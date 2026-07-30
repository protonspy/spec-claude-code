package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/render"
)

// errUsage marks a failure the handler has already reported to the user, so
// runValidation returns the usage exit code without printing a second message.
var errUsage = errors.New("usage")

// runValidation is the shape of every validate command: bind --root and --json,
// resolve the workspace, run the checks, report, and exit on the contract.
//
// One function rather than one per validator, because the part users depend on is
// identical across all of them — the flags, the stdout/stderr split, and above all
// the exit code. A finding is a legitimate answer to a lint question, so it exits
// `2` and not `1`: CI and agents branch on "ran and found problems" separately from
// "could not run", and collapsing the two would break every job that does.
func runValidation(subject string, args []string, fn func(root string, rest []string) (*finding.Set, error)) int {
	fs := flag.NewFlagSet(subject+" validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}

	set, err := fn(target, rest)
	if err != nil {
		if !errors.Is(err, errUsage) {
			render.Err(fmt.Sprintf("%s validate: %v", subject, err))
		}
		return ExitError
	}

	if *jsonOut {
		if code := emitJSON(set.Document()); code != ExitOK {
			return code
		}
		return set.ExitCode()
	}
	set.Report(subject)
	return set.ExitCode()
}
