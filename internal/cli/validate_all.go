package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/validate"
)

// runValidateAll is `scc validate`: every applicable validator, one exit code, one
// JSON document.
//
// It exists because a user who has to invoke eight validators separately has eight
// chances to skip one, and because CI wants a single gate. Validators whose subject is
// absent contribute nothing rather than complaining — a workspace with no skills is not
// a workspace with findings.
func runValidateAll(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	// The pull request is the half of the record no hook can reach: a commit
	// message passes through commit-msg, a PR body is typed into the forge. It is
	// off by default because it is the one check here that goes to the network, and
	// `scc validate` runs on the pre-commit path.
	withPR := fs.Bool("pr", false, "also check the pull request open on this branch — its title and body, read through gh")
	// The other check a default run leaves off, for the same reason at a larger
	// scale: this one runs the project's compiler, linter and suite. It is the
	// delivery gate, so the pre-push hook passes it — the moment a branch becomes a
	// pull request — and a bare pre-commit run never waits on a build.
	withChecks := fs.Bool("checks", false, "also run this workspace's build, format, lint and test commands")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, helpWord(args))
	if err != nil {
		return exitFor(err)
	}
	if !noPositionals(rest, "validate") {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}

	var opts []validate.Option
	if *withPR {
		opts = append(opts, validate.WithPR())
	}
	if *withChecks {
		opts = append(opts, validate.WithChecks())
	}
	set, results, err := validate.Everything(target, opts...)
	if err != nil {
		render.Err(fmt.Sprintf("validate: %v", err))
		return ExitError
	}

	if *jsonOut {
		if code := emitJSON(struct {
			finding.Document
			Validators []validate.Result `json:"validators"`
		}{set.Document(), results}); code != ExitOK {
			return code
		}
		return set.ExitCode()
	}

	// Counts per validator first, then the findings themselves. "Few findings, each
	// fixable" is easiest to violate right here, where every check reports at once —
	// so the shape of the output has to carry the summary before the detail.
	//
	// Only the validators that found something, though. A clean run printed twelve
	// lines to say nothing happened, eleven of them a validator's name beside a
	// zero, and this is the command the pre-commit hook runs and an agent reads back
	// on every commit. What a reader needs from a clean run is the one line saying
	// it was clean; what they need from a dirty one is which check fired, and that
	// line is still here.
	for _, r := range results {
		if r.Findings > 0 {
			render.Warn(fmt.Sprintf("%-12s %d", r.Name, r.Findings))
		}
	}
	if set.Empty() {
		render.Info(fmt.Sprintf("%d validators · 0 findings", len(results)))
	}
	set.Report("validate")
	return set.ExitCode()
}
