// Package hooks is scc's git hooks: the gate that runs `scc validate` before a
// commit is made and before a branch is pushed.
//
// It exists for the reason the attribution validator exists, one step further on.
// A rule in `rules/delivery.md` holds until the session that does not re-read it;
// a validator turns the rule into a check, and a check holds until the session
// that does not run it. A hook is the same check with the "remembering to run it"
// taken out — git runs it, on every commit, whoever or whatever typed the command.
//
// Three stages, and each is where its question can first be answered:
//
//   - **pre-commit** runs the workspace validators, so a spec or a plan is in
//     shape before it enters history rather than after.
//   - **commit-msg** reads the message being written, which is the only moment an
//     assistant's signature can be caught *before* it is in a commit. The
//     validator can report one after the fact; nothing can take it out of a
//     pushed history without a rewrite.
//   - **pre-push** runs the validators again with the pull request included, so
//     "only what passes gets pushed" is true of the branch as a whole.
//
// **The scripts are three lines and call back into scc.** Everything a hook
// decides lives in Go, where it is tested and where `scc update` can change it
// without rewriting a file in somebody's `.git`. A hook script that carried the
// logic would be a fourth copy of the exit-code contract, in the one language
// this project does not otherwise write.
//
// **A hook scc did not write is never touched.** A repository already running
// husky or lefthook has a pre-commit of its own, and appending shell to it would
// be scc authoring what the user owns. It is reported as foreign, with `--force`
// as the separate decision — and even then only into a script whose shebang says
// it is POSIX sh.
package hooks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/mdblock"
	"github.com/protonspy/spec-claude-code/internal/textutil"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// Stage is one git hook scc installs, named as git names it.
type Stage string

// The three stages, in the order git runs them across a unit of work.
const (
	PreCommit Stage = "pre-commit"
	CommitMsg Stage = "commit-msg"
	PrePush   Stage = "pre-push"
)

// The stages the harness runs rather than git. They are Stage values so that
// `scc hooks run <name>` stays one command with one vocabulary — a hook script
// and a settings entry both call it, and which of the two is calling is not
// something the caller should have to spell differently.
const (
	StageSessionStart Stage = "session-start"
	StageStop         Stage = "stop"
)

// Stages is the git set, in the order git runs them. Adding one means adding a
// case to Run in the CLI and a line here — nothing is registered dynamically.
func Stages() []Stage { return []Stage{PreCommit, CommitMsg, PrePush} }

// AgentStages is the harness set, in the order a session meets them.
func AgentStages() []Stage { return []Stage{StageSessionStart, StageStop} }

// Git reports whether this stage is one git runs. The two families differ in
// everything downstream of the name: git reads an exit code, the harness reads a
// JSON document on stdout, and only the git side is allowed to refuse.
func (s Stage) Git() bool {
	for _, st := range Stages() {
		if st == s {
			return true
		}
	}
	return false
}

// ParseStage resolves a name from the command line, from either family.
func ParseStage(s string) (Stage, error) {
	for _, st := range append(Stages(), AgentStages()...) {
		if string(st) == strings.ToLower(strings.TrimSpace(s)) {
			return st, nil
		}
	}
	return "", fmt.Errorf("unknown hook stage %q (want %s)", s, strings.Join(names(), ", "))
}

func names() []string {
	all := append(Stages(), AgentStages()...)
	out := make([]string, 0, len(all))
	for _, s := range all {
		out = append(out, string(s))
	}
	return out
}

// Why is the one line the hook prints about itself, and the one line the report
// prints beside each stage.
func (s Stage) Why() string {
	switch s {
	case PreCommit:
		return "runs `" + Prog + " validate` before the commit is made"
	case CommitMsg:
		return "checks the message being written for an assistant's signature"
	case PrePush:
		return "runs `" + Prog + " validate --pr --checks` before the branch is pushed"
	case StageSessionStart:
		return SessionStart.Why()
	case StageStop:
		return Stop.Why()
	}
	return ""
}

// Markers delimit scc's block inside a hook script. Namespaced as scc's, the way
// the CodeGraph block is and unlike the RTK one: no other tool writes this block,
// so there is nothing to converge with and every reason to leave a husky or
// lefthook script free to keep its own.
var Markers = mdblock.Markers{
	Open:  "# >>> scc:hooks",
	Close: "# <<< scc:hooks",
}

// Version stamps the block, so a workspace wired by an older build can be brought
// current by re-running the install. Bump it whenever script changes.
const Version = "v1"

// Prog is the binary the hook calls. It is a constant rather than the running
// executable's path: a hook is checked into nobody's repository but it does
// outlive this process, and burning an absolute path from one machine into it
// would break the moment scc is reinstalled or the checkout moves.
const Prog = "scc"

// SkipEnv turns every scc hook off for one command. It exists because `--no-verify`
// does not reach every git invocation that runs hooks — a rebase, a merge driver, a
// tool committing on the user's behalf — and a gate with no documented way past it
// is a gate people remove instead of skipping.
const SkipEnv = "SCC_SKIP_HOOKS"

// State is what a hook file is, from scc's point of view.
type State string

const (
	// Installed: the file carries this build's block.
	Installed State = "installed"
	// Stale: the file carries an scc block from another build.
	Stale State = "stale"
	// Missing: there is no hook file at all.
	Missing State = "missing"
	// Foreign: a hook is there and scc did not write it. Reported, never touched.
	Foreign State = "foreign"
)

// Status is one stage's hook as it stands on disk.
type Status struct {
	Stage Stage  `json:"stage"`
	Path  string `json:"path"`
	State State  `json:"state"`
	// Action is what an install or a remove did: added | replaced | present |
	// removed | skipped. Empty on a plain look.
	Action string `json:"action,omitempty"`
	// Note names why an action did not happen, for the run where that is a surprise.
	Note string `json:"note,omitempty"`
}

// The actions a run reports.
const (
	Added    = "added"
	Replaced = "replaced"
	Present  = "present"
	Removed  = "removed"
	Skipped  = "skipped"
)

// Look reports the state of every stage without changing anything.
func Look(dir string) ([]Status, error) {
	return each(dir, func(st Stage, path string) Status {
		return Status{Stage: st, Path: path, State: state(path, st)}
	})
}

// Install writes scc's hooks, or brings them current.
//
// force is the separate decision for a hook somebody else wrote: without it a
// foreign script is reported and left exactly as it is. With it, the block is
// appended to the existing script — but only when the shebang says POSIX sh,
// because appending shell to a Python hook is not a forced decision, it is a
// broken repository.
func Install(dir string, force bool) ([]Status, error) {
	return each(dir, func(st Stage, path string) Status {
		s := Status{Stage: st, Path: path, State: state(path, st)}
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			if err := write(path, shebang+"\n\n"+script(st)+"\n"); err != nil {
				s.Note = err.Error()
				return s
			}
			s.State, s.Action = Installed, Added
			return s
		}
		if err != nil {
			s.Note = err.Error()
			return s
		}
		doc := string(raw)
		if s.State == Foreign {
			if !force {
				s.Action = Skipped
				s.Note = "a hook scc did not write is already here; --force appends to it"
				return s
			}
			if !posix(doc) {
				s.Action = Skipped
				s.Note = "the hook here is not a POSIX sh script, so there is nothing safe to append to"
				return s
			}
		}
		out, action, err := Markers.Splice(doc, script(st), false)
		if err != nil {
			s.Note = err.Error()
			return s
		}
		if err := write(path, out); err != nil {
			s.Note = err.Error()
			return s
		}
		s.State, s.Action = Installed, string(action)
		return s
	})
}

// Remove takes scc's block back out, and the file with it when the block was all
// it held. A hook somebody else wrote that scc appended to keeps everything of
// theirs.
func Remove(dir string) ([]Status, error) {
	return each(dir, func(st Stage, path string) Status {
		s := Status{Stage: st, Path: path, State: state(path, st)}
		if s.State == Missing || s.State == Foreign {
			s.Action = Skipped
			return s
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			s.Note = err.Error()
			return s
		}
		out, found := Markers.Remove(string(raw))
		if !found {
			s.Action = Skipped
			return s
		}
		if strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), shebang)) == "" {
			if err := os.Remove(path); err != nil {
				s.Note = err.Error()
				return s
			}
			s.State, s.Action = Missing, Removed
			return s
		}
		if err := write(path, out); err != nil {
			s.Note = err.Error()
			return s
		}
		s.State, s.Action = Foreign, Removed
		return s
	})
}

// OK reports whether every stage is installed and current — the question
// `scc hooks --check` and the init report both ask.
func OK(all []Status) bool {
	for _, s := range all {
		if s.State != Installed {
			return false
		}
	}
	return len(all) > 0
}

// each resolves the hooks directory once and applies fn to every stage.
func each(dir string, fn func(Stage, string) Status) ([]Status, error) {
	hooks, err := Dir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Status, 0, len(Stages()))
	for _, st := range Stages() {
		out = append(out, fn(st, filepath.Join(hooks, string(st))))
	}
	return out, nil
}

// state reads one hook file and says what it is.
func state(path string, st Stage) State {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Missing
	}
	doc := string(raw)
	block := Markers.Block(doc)
	switch {
	case block == "":
		return Foreign
	case block == script(st):
		return Installed
	}
	return Stale
}

// write puts a hook on disk executable. 0755 rather than 0644 because a hook git
// cannot execute is a hook git skips — silently, which is the one failure mode
// this package exists to remove. Windows ignores the bit and runs hooks through
// the sh that ships with git.
func write(path, body string) error {
	return workspace.AtomicWrite(path, []byte(textutil.NormalizeNewlines(body)), 0o755)
}

// posix reports whether a script scc is about to append to is one git will run
// with a POSIX shell.
func posix(doc string) bool {
	line, _, _ := strings.Cut(strings.TrimSpace(doc), "\n")
	if !strings.HasPrefix(line, "#!") {
		// No shebang: git runs it with sh, which is what the block expects.
		return true
	}
	for _, sh := range []string{"sh", "bash", "dash", "zsh", "ksh"} {
		if strings.HasSuffix(strings.TrimSpace(line), "/"+sh) || strings.HasSuffix(strings.TrimSpace(line), " "+sh) {
			return true
		}
	}
	return false
}
