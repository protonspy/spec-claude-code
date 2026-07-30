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
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !noPositionals(rest, "validate") {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}

	set, results, err := validate.Everything(target)
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
	for _, r := range results {
		line := fmt.Sprintf("%-9s %d", r.Name, r.Findings)
		if r.Findings == 0 {
			render.Info(line)
			continue
		}
		render.Warn(line)
	}
	set.Report("validate")
	return set.ExitCode()
}
