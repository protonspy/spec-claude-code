package cli

import (
	"errors"
	"flag"
	"fmt"
	"strings"
	"testing"
)

// `--help` is 0, and it took 52 call sites to be wrong about that at once: every
// handler routed flag.ErrHelp to ExitError, so `scc map tasks --help` reported the
// same code as a command that could not run — to an agent branching on this
// contract, help that says the command is broken.
func TestHelpIsAlwaysZeroAndAlwaysPrintsSomething(t *testing.T) {
	// `check` with no subcommand is deliberately absent from every list here: it
	// runs the gates, which in this repository is the suite running this test.
	commands := [][]string{
		{"map"}, {"map", "index"}, {"map", "outline"}, {"map", "brief"}, {"map", "tasks"},
		{"map", "show"}, {"map", "blocks"}, {"map", "trace"},
		{"patch"}, {"patch", "check"}, {"patch", "uncheck"}, {"patch", "task"},
		{"patch", "add"}, {"patch", "rm"}, {"patch", "append"}, {"patch", "prepend"},
		{"patch", "replace"}, {"patch", "fm"},
		{"notes"}, {"notes", "add"}, {"notes", "find"}, {"notes", "show"},
		{"notes", "tags"}, {"notes", "paths"}, {"notes", "rm"}, {"notes", "validate"},
		{"spec"}, {"spec", "new"}, {"spec", "list"}, {"spec", "track"}, {"spec", "sync"},
		{"plan"}, {"plan", "new"}, {"plan", "approve"}, {"plan", "reseal"}, {"plan", "migrate"},
		{"graph"}, {"graph", "build"}, {"graph", "sync"}, {"graph", "status"},
		{"graph", "query"}, {"graph", "explore"}, {"graph", "scope"},
		{"hooks"}, {"hooks", "install"}, {"hooks", "check"}, {"hooks", "remove"},
		{"check", "set"}, {"check", "skip"}, {"check", "clear"},
		{"init"}, {"update"}, {"validate"}, {"launch"}, {"rtk"},
	}

	for _, cmd := range commands {
		form := append(append([]string{}, cmd...), "--help")
		name := strings.Join(form, " ")
		stdout, stderr, code := run(t, form...)
		if code != ExitOK {
			t.Errorf("`scc %s` exited %d, and help is always 0", name, code)
		}
		if strings.TrimSpace(stdout+stderr) == "" {
			t.Errorf("`scc %s` printed nothing", name)
		}
	}
}

// A bare `help` positional is the same request one word shorter, and it is what
// somebody types after `scc map help`. It is answered on a resource — where there
// is a subcommand list to print — rather than on a leaf, where `help` is a
// perfectly good address, note id or feature name and taking it as a request for
// the manual would make those unaddressable.
func TestHelpAsAPositionalIsAnsweredOnAResource(t *testing.T) {
	for _, resource := range []string{"map", "patch", "notes", "spec", "plan", "graph", "hooks", "check"} {
		stdout, stderr, code := run(t, resource, "help")
		if code != ExitOK {
			t.Errorf("`scc %s help` exited %d", resource, code)
		}
		if !strings.Contains(stdout+stderr, resource) {
			t.Errorf("`scc %s help` does not name the resource:\n%s%s", resource, stdout, stderr)
		}
	}
}

// An unknown subcommand is an error that names what was typed: the caller is often
// an agent that cannot see a manual, so "unknown command" alone costs it a round
// trip to work out which word was wrong.
func TestAnUnknownSubcommandNamesWhatWasTyped(t *testing.T) {
	for _, resource := range []string{"map", "patch", "notes", "spec", "plan", "graph", "hooks", "check"} {
		stdout, stderr, code := run(t, resource, "definitely-not-a-subcommand")
		if code != ExitError {
			t.Errorf("`scc %s definitely-not-a-subcommand` exited %d, want %d", resource, code, ExitError)
		}
		if !strings.Contains(stdout+stderr, "definitely-not-a-subcommand") {
			t.Errorf("`scc %s` with an unknown subcommand did not name it:\n%s%s", resource, stdout, stderr)
		}
	}
}

// The top-level surface: no arguments at all, and a command scc does not have.
func TestTheTopLevelSurfaceExplainsItself(t *testing.T) {
	stdout, stderr, code := run(t)
	if code != ExitError {
		t.Errorf("`scc` with no arguments exited %d", code)
	}
	if !strings.Contains(stdout+stderr, "validate") {
		t.Errorf("`scc` does not list its commands:\n%s%s", stdout, stderr)
	}

	stdout, stderr, code = run(t, "--help")
	if code != ExitOK {
		t.Errorf("`scc --help` exited %d", code)
	}
	if !strings.Contains(stdout+stderr, "validate") {
		t.Errorf("`scc --help` does not list its commands:\n%s%s", stdout, stderr)
	}

	if _, _, code := run(t, "definitely-not-a-command"); code != ExitError {
		t.Errorf("an unknown command exited %d, want %d", code, ExitError)
	}

	// The exit codes are the contract, and they are three distinct things: a
	// version query is not a usage error and findings are not a failure to run.
	if _, _, code := run(t, "version"); code != ExitOK {
		t.Errorf("`scc version` exited %d", code)
	}
}

// `scc validate help` is what somebody types having just typed `scc map help`,
// which works because that command has a subcommand to dispatch on. A leaf command
// has none, so the word arrived as a stray positional and the answer was "validate
// takes no arguments" — correct, exit 1, and no use to anybody.
func TestHelpWordRewritesOnlyALoneHelp(t *testing.T) {
	if got := helpWord([]string{"help"}); len(got) != 1 || got[0] != "-h" {
		t.Errorf("helpWord([help]) = %v, want [-h]", got)
	}
	// Anything else is left exactly as it was: `help` is a perfectly good plan
	// name, note id or search term, and taking it for a request would make those
	// unaddressable.
	for _, args := range [][]string{
		{},
		{"help", "me"},
		{"--json", "help"},
		{"helpful"},
		{"sample"},
	} {
		got := helpWord(args)
		if len(got) != len(args) {
			t.Errorf("helpWord(%v) = %v, want it untouched", args, got)
			continue
		}
		for i := range args {
			if got[i] != args[i] {
				t.Errorf("helpWord(%v) = %v, want it untouched", args, got)
				break
			}
		}
	}
}

// `--help` is not a failure. flag reports it as one because it has nothing else to
// return, but the user got exactly what they asked for — and exit 1 there says the
// command could not run, which an agent branching on the contract reads as a
// broken command rather than as the help it just printed.
func TestExitForSeparatesHelpFromAFailure(t *testing.T) {
	if got := exitFor(flag.ErrHelp); got != ExitOK {
		t.Errorf("exitFor(ErrHelp) = %d, want %d", got, ExitOK)
	}
	if got := exitFor(errors.New("flag provided but not defined: -nope")); got != ExitError {
		t.Errorf("exitFor(a real failure) = %d, want %d", got, ExitError)
	}
	if got := exitFor(fmt.Errorf("wrapped: %w", flag.ErrHelp)); got != ExitOK {
		t.Errorf("exitFor on a wrapped ErrHelp = %d, want %d", got, ExitOK)
	}
}
