package validate

import (
	"sort"

	"github.com/protonspy/spec-claude-code/internal/finding"
)

// Validator is one named check over a workspace.
type Validator struct {
	Name string
	Run  func(root string) (*finding.Set, error)
}

// All is every validator that runs under `scc validate`, in a fixed order so two runs
// over one workspace report identically.
//
// Each one is silent when its subject is absent: a workspace with no skills is not a
// workspace with findings. That is what lets the aggregate command run unconditionally
// instead of asking the user which validators apply.
func All() []Validator {
	return []Validator{
		{Name: "skill", Run: Skills},
		{Name: "spec", Run: Specs},
		{Name: "plan", Run: Plans},
		{Name: "wiki", Run: Wiki},
		{Name: "adr", Run: ADR},
		{Name: "glossary", Run: Glossary},
		{Name: "stack", Run: Stack},
		{Name: "codewiki", Run: Codewiki},
	}
}

// Result is one validator's outcome, kept separate from the merged set so the report
// can say which check found what.
type Result struct {
	Name     string `json:"validator"`
	Findings int    `json:"findings"`
}

// Everything runs every validator and merges the findings into one set.
//
// One exit code and one document, because eight validators the user has to invoke
// separately is eight chances to skip one. The per-validator counts come back
// alongside so the report can group by check without re-running anything.
func Everything(root string) (*finding.Set, []Result, error) {
	set := &finding.Set{}
	var results []Result
	for _, v := range All() {
		one, err := v.Run(root)
		if err != nil {
			return nil, nil, err
		}
		results = append(results, Result{Name: v.Name, Findings: one.Len()})
		set.Extend(one)
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	return set, results, nil
}
