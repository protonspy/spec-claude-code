package cli

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// parseFlags parses args and returns the positionals, wherever they sit among the
// flags.
//
// Go's flag package stops parsing at the first non-flag argument, so
// `scc spec new user-auth --json` would otherwise read "--json" as a second
// positional and silently ignore the flag. Every user and every agent writes the
// name first, so collecting the leading positionals and parsing the rest is the
// minimum. It is not enough on its own: a flag *after* a positional stops the
// parse just as dead, and the measured result was `scc notes add --tag gotcha
// "text" --path internal/x` writing "--path internal/x" into the note as prose,
// with no path recorded and exit 0. Silently misfiling an argument is worse than
// rejecting it, so this alternates — peel positionals, parse flags, repeat — until
// nothing is left.
//
// A bare `--` ends scc's own arguments. Everything after it is a positional
// verbatim, even spelled like a flag, which is what makes a literal `--text -x`
// reachable at all.
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var verbatim []string
	for i, a := range args {
		if a == "--" {
			args, verbatim = args[:i], args[i+1:]
			break
		}
	}
	var positionals []string
	for len(args) > 0 {
		i := 0
		for i < len(args) && !isFlag(args[i]) {
			positionals = append(positionals, args[i])
			i++
		}
		if err := fs.Parse(args[i:]); err != nil {
			return nil, err
		}
		// flag stops at the first argument it does not recognize as a flag, which
		// by isFlag's definition is one this loop will consume — so the next pass
		// always makes progress and this terminates.
		args = fs.Args()
	}
	return append(positionals, verbatim...), nil
}

// isFlag reports whether flag would treat this argument as one.
//
// It matches flag's own test rather than approximating it with a "-" prefix,
// because the two disagree on exactly the argument that would hang the loop above:
// a bare "-" looks like a flag and is parsed as a positional.
func isFlag(a string) bool {
	return len(a) > 1 && a[0] == '-'
}

// helpWord rewrites a lone `help` positional into the flag that prints the usage.
//
// `scc validate help` is what somebody types having just typed `scc map help` and
// `scc check help`, which work because those commands have a subcommand to
// dispatch on. A leaf command has none, so the word arrived as a stray positional
// and the answer was "validate takes no arguments, got \"help\"" — correct, exit
// 1, and no use to anybody.
func helpWord(args []string) []string {
	if len(args) == 1 && args[0] == "help" {
		return []string{"-h"}
	}
	return args
}

// exitFor turns a parse failure into a command's exit code.
//
// `--help` is not a failure. flag reports it as one because it has nothing else to
// return, but the user got exactly what they asked for, and exit 1 there says the
// command could not run — which an agent branching on the 0/1/2 contract reads as
// a broken command rather than as the help it just printed.
func exitFor(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return ExitOK
	}
	return ExitError
}

// rootFlagHelp is the single wording every command shows for --root, for the same
// reason addJSON exists: the flag has to read identically across the surface.
const rootFlagHelp = "workspace root (default: the enclosing workspace, else the working directory)"

// addRoot binds --root on fs.
func addRoot(fs *flag.FlagSet) *string {
	return fs.String("root", "", rootFlagHelp)
}

// resolveRoot turns the --root flag into an absolute path, reporting the failure
// on stderr so every command words it the same way.
func resolveRoot(arg string) (string, bool) {
	root, err := workspace.Resolve(arg)
	if err != nil {
		render.Err(err.Error())
		return "", false
	}
	return root, true
}

// requireWorkspace is the guard on every command that reads or writes artifacts.
// Without it those commands would operate on whatever directory the walk fell back
// to, which for a command run outside a workspace is the user's own filesystem.
func requireWorkspace(root string) bool {
	if workspace.IsWorkspace(root) {
		return true
	}
	render.Err(fmt.Sprintf("%s is not an scc workspace", root))
	render.Detail(fmt.Sprintf("  run `%s init` there first", prog()))
	return false
}

// The kickoff answers, recorded on the artifact so the run is reproducible from
// the file and nobody is asked twice. The orchestrator asks the user in
// conversation — that is where the person who has to decide actually is — and
// passes the answer here.
const (
	autonomyAuto  = "auto"
	autonomyGated = "gated"
	ciWait        = "wait"
	ciNoWait      = "no-wait"
)

type kickoff struct {
	autonomy *string
	ci       *string
}

// addKickoff binds --autonomy and --ci.
//
// The defaults match the methodology rather than being neutral: the spec phases
// are autonomous by default, and work is not finished while CI is failing. A
// neutral default would leave the most consequential question in the process
// unanswered in the file, which is the one outcome recording it was meant to
// prevent.
func addKickoff(fs *flag.FlagSet) kickoff {
	return kickoff{
		autonomy: fs.String("autonomy", autonomyAuto,
			"`auto` to run the phases without stopping, or `gated` to review each one"),
		ci: fs.String("ci", ciWait,
			"`wait` to watch CI after the PR opens, or `no-wait` to finish at the PR"),
	}
}

// validate rejects a value outside the two the rule defines. A typo'd
// "--autonomy=automatic" recorded verbatim would read as an answer nobody gave.
func (k kickoff) validate() bool {
	if *k.autonomy != autonomyAuto && *k.autonomy != autonomyGated {
		render.Err(fmt.Sprintf("--autonomy must be %q or %q, got %q", autonomyAuto, autonomyGated, *k.autonomy))
		return false
	}
	if *k.ci != ciWait && *k.ci != ciNoWait {
		render.Err(fmt.Sprintf("--ci must be %q or %q, got %q", ciWait, ciNoWait, *k.ci))
		return false
	}
	return true
}

// artifactName validates a positional that is about to become a path segment.
//
// Order matters: SafeName first, because it is the check that stands between a CLI
// argument and filepath.Join. Without it `scc spec delete ..` resolves to the
// workspace root. KebabCheck second, because it is a convention rather than a
// safety property.
func artifactName(positionals []string, kind string) (string, bool) {
	if len(positionals) != 1 {
		render.Err(fmt.Sprintf("expected exactly one %s name, got %d", kind, len(positionals)))
		return "", false
	}
	name := positionals[0]
	if err := workspace.SafeName(name, kind); err != nil {
		render.Err(err.Error())
		return "", false
	}
	if err := workspace.KebabCheck(name, kind); err != nil {
		render.Err(err.Error())
		return "", false
	}
	return name, true
}

// noPositionals rejects leftover arguments. A command that silently ignores them
// does the wrong thing while reporting success — usually because the user typed a
// name a subcommand expected and this one does not take.
func noPositionals(positionals []string, command string) bool {
	if len(positionals) == 0 {
		return true
	}
	render.Err(fmt.Sprintf("%s takes no arguments, got %q", command, positionals[0]))
	return false
}

// relPath renders a path for output: relative to the workspace root and
// slash-separated, so the same artifact reads the same way on every platform and a
// --json consumer can match it against the manifest.
func relPath(root, target string) string {
	return filepath.ToSlash(workspace.Relative(root, target))
}
