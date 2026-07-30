package cli

import (
	"fmt"
	"os"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/validate"
)

// runSkill dispatches `scc skill <subcommand>`.
//
// This validator is the only one in scc that enforces somebody else's standard: the
// Agent Skills spec is published and maintained outside this project, and tools
// across competing vendors read the same format. So a finding here is not scc's
// opinion about how a skill should look — it is non-conformance to a contract, which
// is the most defensible kind of check the tool can ship.
func runSkill(args []string) int {
	if len(args) == 0 {
		skillUsage()
		return ExitError
	}
	switch args[0] {
	case "validate":
		return runValidation("skill", args[1:], func(root string, rest []string) (*finding.Set, error) {
			if !noPositionals(rest, "skill validate") {
				return nil, errUsage
			}
			return validate.Skills(root)
		})
	case "help", "-h", "--help":
		skillUsage()
		return ExitOK
	default:
		render.Err(fmt.Sprintf("unknown skill subcommand %q", args[0]))
		skillUsage()
		return ExitError
	}
}

func skillUsage() {
	fmt.Fprintf(os.Stderr, `Usage:
  %s skill validate

Checks every skill under .claude/skills/ against the published Agent Skills
specification: name charset and length, name matching its directory, description,
body budget, and references that resolve one level deep.

Exits 2 when it finds something.
`, prog())
}
