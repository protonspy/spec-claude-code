package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/attribution"
	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/gate"
	"github.com/protonspy/spec-claude-code/internal/hooks"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/validate"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// runHooks is `scc hooks`: the git hooks that run the validators without anybody
// remembering to.
//
// install | check | remove wire them up and report; run is what the hook scripts
// themselves call, so every decision a hook makes is Go that this repository
// tests, rather than shell in somebody's `.git` that nothing does.
//
// **These are the only commands here that do not require a workspace**, and that
// is deliberate. A repository with no `specs/` still has a record of its changes,
// and the rule about what that record may say binds it exactly as hard — scc's own
// repository is the worked example, since it is not an scc workspace and is the
// first place that rule has to hold. What a hook checks therefore scales with what
// is there: the artifacts when there are artifacts, the record always.
func runHooks(args []string) int {
	if len(args) == 0 {
		hooksUsage()
		return ExitError
	}
	switch args[0] {
	case "install":
		return runHooksInstall(args[1:])
	case "check", "status":
		return runHooksCheck(args[1:])
	case "remove", "uninstall":
		return runHooksRemove(args[1:])
	case "run":
		return runHooksRun(args[1:])
	case "help", "-h", "--help":
		hooksUsage()
		return ExitOK
	default:
		render.Err(fmt.Sprintf("unknown hooks command %q", args[0]))
		hooksUsage()
		return ExitError
	}
}

func hooksUsage() {
	fmt.Fprintf(os.Stderr, `%s hooks — run the validators from git, not from memory

Usage:
  %s hooks install [--force]   Write the hooks, or bring them onto this build
  %s hooks check               Report what is installed; exit 2 when anything is not
  %s hooks remove              Take scc's block back out
  %s hooks run <stage> [args]  What the hook scripts call; you do not type this

Git stages — these refuse:
  pre-commit     %s
  commit-msg     %s
  pre-push       %s

Harness stages — these only report, and never publish:
  session-start  %s
  stop           %s

The git hooks need a git repository; outside a workspace the artifact validators
have nothing to read and the checks on the record still run. A hook scc did not
write is reported and left alone; --force appends to it, and only when it is a
POSIX sh script. Skip every scc git hook for one command with %s=1, or use git's
--no-verify where that applies.

The harness stages are entries in the harness's own settings file, spliced in
beside whatever else is there and removed the same way. Only a harness with a
hook surface gets them — Claude Code today. They hand text to the agent and
never block a turn, and they never push or open a pull request: delivery is an
act the agent takes in the open, where you can stop it.
`, prog(), prog(), prog(), prog(), prog(),
		hooks.PreCommit.Why(), hooks.CommitMsg.Why(), hooks.PrePush.Why(),
		hooks.SessionStart.Why(), hooks.Stop.Why(), hooks.SkipEnv)
}

// hooksReport is the frozen JSON shape for install, check and remove: one entry
// per stage, in the order git runs them, plus the harness registrations.
//
// Two lists rather than one, because they are two mechanisms that happen to share
// a word. A git hook is a script in `.git` that can refuse a commit; a harness
// hook is an entry in a settings file that hands text to an agent. Flattening
// them would hide which of the two is missing when one is.
type hooksReport struct {
	Dir     string                `json:"dir"`
	Stages  []hooks.Status        `json:"stages"`
	Harness []hooks.HarnessStatus `json:"harness,omitempty"`
	// Note is why one half is absent — no git repository, usually. A field rather
	// than an error, because the other half may still be wired.
	Note string `json:"note,omitempty"`
	OK   bool   `json:"ok"`
	Skip string `json:"skip_env"`
}

func runHooksInstall(args []string) int {
	fs := flag.NewFlagSet("hooks install", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	force := fs.Bool("force", false, "append scc's block to a hook somebody else wrote, when it is a POSIX sh script")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !noPositionals(rest, "hooks install") {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok {
		return ExitError
	}
	all, note := gitStages(func() ([]hooks.Status, error) { return hooks.Install(target, *force) })
	agent := hooks.InstallHarness(target)
	if !anyHooks(all, agent, note, "hooks install") {
		return ExitError
	}
	return reportHooks(all, agent, note, *jsonOut)
}

// gitStages runs one of the git-side operations and turns "there is no git
// repository here" into a note rather than a failure.
//
// The two halves are independent and have to degrade independently: a harness
// hook lives in the workspace's own settings file, so a directory that is not a
// git repository can still have one, and a run that failed outright over the
// missing half would take the half that works with it. The reverse is already
// true — a repository whose harness has no hook surface still gets its git hooks.
func gitStages(fn func() ([]hooks.Status, error)) ([]hooks.Status, string) {
	all, err := fn()
	if err != nil {
		return nil, err.Error()
	}
	return all, ""
}

// anyHooks reports whether there was anything at all to act on, and says so when
// there was not. Both halves absent is the one case worth an error: the user
// asked for hooks in a directory with nowhere to put any.
func anyHooks(all []hooks.Status, agent []hooks.HarnessStatus, note, cmd string) bool {
	if len(all) > 0 || len(agent) > 0 {
		return true
	}
	if note == "" {
		note = "no git repository and no harness with a hook surface"
	}
	render.Err(fmt.Sprintf("%s: %s", cmd, note))
	return false
}

func runHooksCheck(args []string) int {
	fs := flag.NewFlagSet("hooks check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !noPositionals(rest, "hooks check") {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok {
		return ExitError
	}
	all, note := gitStages(func() ([]hooks.Status, error) { return hooks.Look(target) })
	agent := hooks.LookHarness(target)
	if !anyHooks(all, agent, note, "hooks check") {
		return ExitError
	}
	if code := reportHooks(all, agent, note, *jsonOut); code != ExitOK {
		return code
	}
	// A missing gate is a finding, not a failure to run: the same 2 every
	// validator returns, so a CI job asserting the hooks are wired branches on it
	// exactly as it branches on `scc validate`.
	if !hooks.OK(all) || !hooks.HarnessOK(agent) {
		return ExitFindings
	}
	return ExitOK
}

func runHooksRemove(args []string) int {
	fs := flag.NewFlagSet("hooks remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !noPositionals(rest, "hooks remove") {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok {
		return ExitError
	}
	all, note := gitStages(func() ([]hooks.Status, error) { return hooks.Remove(target) })
	agent := hooks.RemoveHarness(target)
	if !anyHooks(all, agent, note, "hooks remove") {
		return ExitError
	}
	return reportHooks(all, agent, note, *jsonOut)
}

// reportHooks prints one line per stage, or the document. ExitOK unless the
// output itself failed — whether the state is *good* is the caller's question.
func reportHooks(all []hooks.Status, agent []hooks.HarnessStatus, note string, jsonOut bool) int {
	if jsonOut {
		dir := ""
		if len(all) > 0 {
			dir = finding.Rel(filepath.Dir(all[0].Path))
		}
		return emitJSON(hooksReport{Dir: dir, Stages: all, Harness: agent, Note: note,
			OK: hooks.OK(all) && hooks.HarnessOK(agent), Skip: hooks.SkipEnv})
	}
	if note != "" {
		render.Info("no git hooks: " + note)
	}
	for _, s := range all {
		hookLine(string(s.Stage), stateOrAction(s), s.Path, s.Note, s.State)
	}
	for _, s := range agent {
		hookLine(string(s.Event), harnessStateOrAction(s), s.Path, s.Note, s.State)
	}
	return ExitOK
}

// hookLine is one row of the report, in one place, so the two families line up in
// the same columns. They are different mechanisms and the reader is asking one
// question of both: is this wired.
func hookLine(name, what, path, note string, state hooks.State) {
	line := fmt.Sprintf("%-13s %-10s %s", name, what, hookPath(path))
	switch {
	case note != "":
		render.Warn(line)
		render.Detail("  " + note)
	case state == hooks.Installed:
		render.OK(line)
	default:
		render.Warn(line)
	}
}

func harnessStateOrAction(s hooks.HarnessStatus) string {
	if s.Action != "" {
		return s.Action
	}
	return string(s.State)
}

// hookPath prints a hook where the reader can find it: relative to the directory
// they are standing in, since that is what a shell completes and an absolute path
// under somebody's temp directory says nothing. It falls back to the absolute
// path rather than to nothing.
func hookPath(p string) string {
	cwd := mustCwd()
	if cwd == "" {
		return finding.Rel(p)
	}
	return finding.Rel(workspace.Relative(cwd, p))
}

// stateOrAction says what a run did when it did something, and what is there when
// it did not. Two columns saying "installed / present" on every line would push
// the path off the terminal to report nothing.
func stateOrAction(s hooks.Status) string {
	if s.Action != "" {
		return s.Action
	}
	return string(s.State)
}

// runHooksRun is the hook itself. It takes the stage git is in and returns the
// exit code git reads: 0 goes ahead, anything else stops.
//
// Nothing here is meant to be typed by a person, and it is a command rather than
// three flags on `scc validate` for one reason — a hook is written once into a
// file scc may not get to edit again, so what it calls has to be a name whose
// meaning can change underneath it. `hooks run pre-commit` will always mean
// "whatever this build gates a commit on".
func runHooksRun(args []string) int {
	fs := flag.NewFlagSet("hooks run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if len(rest) == 0 {
		render.Err(fmt.Sprintf("hooks run: name a stage (%s)", strings.Join([]string{
			string(hooks.PreCommit), string(hooks.CommitMsg), string(hooks.PrePush)}, ", ")))
		return ExitError
	}
	stage, err := hooks.ParseStage(rest[0])
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok {
		return ExitError
	}

	switch stage {
	case hooks.CommitMsg:
		return runCommitMsg(rest[1:], *jsonOut)
	case hooks.PrePush:
		return runGate(target, *jsonOut, true)
	case hooks.StageSessionStart, hooks.StageStop:
		// A different protocol downstream of the same command: the harness hands
		// the event in on stdin and reads a document back, where git reads an exit
		// code. --json is ignored here because the output is already the harness's
		// JSON and there is no second shape to ask for.
		return runAgentStage(target, stage, os.Stdin)
	default:
		return runGate(target, *jsonOut, false)
	}
}

// runGate is the pre-commit and pre-push check: the same run `scc validate` does,
// reported the same way, exiting on the same contract.
func runGate(root string, jsonOut, pr bool) int {
	set, err := gateFindings(root, pr)
	if err != nil {
		render.Err(fmt.Sprintf("validate: %v", err))
		return ExitError
	}
	if jsonOut {
		if code := emitJSON(set.Document()); code != ExitOK {
			return code
		}
		return set.ExitCode()
	}
	set.Report("validate")
	if !set.Empty() {
		render.Detail(fmt.Sprintf("  fix these, or pass %s=1 for this one command", hooks.SkipEnv))
	}
	return set.ExitCode()
}

// gateFindings runs everything that applies here: every validator in a workspace,
// and the checks on the record alone outside one.
//
// The narrowing is not a courtesy. It is what lets the hooks be installed in a
// repository that follows scc's rules without being one of its workspaces — scc's
// own, for one — where the artifact validators have nothing to read and the rule
// about what a commit may say still binds.
func gateFindings(root string, pr bool) (*finding.Set, error) {
	if workspace.IsWorkspace(root) {
		var opts []validate.Option
		if pr {
			opts = append(opts, validate.WithPR())
			// The suite runs here and only here. A push is where a branch becomes a
			// pull request, which is where this methodology says the claim about tests
			// is actually made — and it is the one moment in the cycle where waiting a
			// minute for a real answer is proportionate. On every commit it would not
			// be, and a pre-commit hook somebody uninstalls takes the other ten
			// validators with it.
			//
			// Only when a command is recorded, which is what keeps the gate from
			// failing every push in a workspace that never wired its tests up. Under
			// `scc validate --tests` the same state is a finding, because there the
			// gate was asked for by name; here it is a default, and a default that
			// blocked work over configuration nobody chose would be scc deciding how
			// somebody else's project is tested.
			if cfg, err := gate.Load(root); err == nil && cfg.Any() {
				opts = append(opts, validate.WithChecks())
			}
		}
		set, _, err := validate.Everything(root, opts...)
		return set, err
	}
	set := &finding.Set{}
	one, err := validate.Attribution(root)
	if err != nil {
		return nil, err
	}
	set.Extend(one)
	if !pr {
		return set, nil
	}
	two, err := validate.AttributionPR(root)
	if err != nil {
		return nil, err
	}
	set.Extend(two)
	return set, nil
}

// runCommitMsg checks the message git is about to commit.
//
// This is the only moment the check is worth anything: the attribution validator
// can report a signature that is already in a commit, but taking it out then means
// rewriting history. Here the message is still a file, and a non-zero exit leaves
// it in `.git/COMMIT_EDITMSG` for the user to fix.
func runCommitMsg(rest []string, jsonOut bool) int {
	if len(rest) == 0 {
		render.Err("hooks run commit-msg: git passes the message file as an argument, and none arrived")
		return ExitError
	}
	path := rest[0]
	raw, err := os.ReadFile(path)
	if err != nil {
		render.Err(fmt.Sprintf("hooks run commit-msg: %v", err))
		return ExitError
	}
	set := &finding.Set{}
	for _, hit := range attribution.Scan(commitMessage(string(raw))) {
		set.Addf(finding.Rel(path), hit.Line, "attribution."+hit.Rule,
			"%s — the work is the user's, and the record says so", hit.Match)
	}
	if jsonOut {
		if code := emitJSON(set.Document()); code != ExitOK {
			return code
		}
		return set.ExitCode()
	}
	set.Report("commit-msg")
	if !set.Empty() {
		render.Detail("  the message is still in .git/COMMIT_EDITMSG — rewrite it and commit again")
		render.Detail(fmt.Sprintf("  or pass %s=1 for this one commit", hooks.SkipEnv))
	}
	return set.ExitCode()
}

// commitMessage is the part of the file git will keep: comments are stripped
// before the message is stored, and the scissors line ends it.
//
// Both matter to the check rather than to tidiness. git's own template sits under
// the comment character and a verbose commit pastes the entire diff below the
// scissors — a check reading either would report findings on text that never
// reaches a commit, and report them on every commit.
func commitMessage(raw string) string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, scissors) {
			break
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// scissors is what `git commit --verbose` puts above the diff it appends.
const scissors = "# ------------------------ >8 ------------------------"
