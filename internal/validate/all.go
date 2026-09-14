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
// attribution comes last because it is the only one whose subject is not a file: it
// reads the commits this branch has added rather than anything on disk, so a reader
// scanning the list finds every artifact check together and the history check after
// them.
//
// Each one is silent when its subject is absent: a workspace with no skills is not a
// workspace with findings. That is what lets the aggregate command run unconditionally
// instead of asking the user which validators apply.
func All(opts ...Option) []Validator {
	return append(base(), extra(opts)...)
}

// Option turns on a check that a default run leaves off.
//
// There are two, and both are behind it for the same reason: cost. `scc validate`
// sits on the pre-commit path, where a gate that costs a second per commit is a
// gate somebody turns off and then none of the others run either. WithPR goes to
// the network; WithTests runs the project's whole suite. Everything else here
// reads files that are already on disk, and the bar for adding a third option is
// exactly that — a check that is cheap belongs in base(), where nobody has to
// remember it.
type Option func(*options)

type options struct {
	pr    bool
	tests bool
}

// WithPR adds the pull-request check: the title and body of the PR open on this
// branch, read through `gh`. Off by default, on in the pre-push hook and under
// `scc validate --pr`, which are the two moments the pull request exists.
func WithPR() Option { return func(o *options) { o.pr = true } }

// WithTests adds the delivery gate's numeric half: the project's own test command,
// run, with its coverage held against the floor. Off by default, on under `scc
// validate --tests` and in the pre-push hook of a workspace that recorded a
// command — the moment a branch becomes a pull request.
func WithTests() Option { return func(o *options) { o.tests = true } }

func extra(opts []Option) []Validator {
	var o options
	for _, fn := range opts {
		fn(&o)
	}
	var out []Validator
	// Tests before pr: it is the slow one, and a run that is going to fail on
	// coverage should say so before it spends a network round trip on the forge.
	if o.tests {
		out = append(out, Validator{Name: "tests", Run: Tests})
	}
	if o.pr {
		out = append(out, Validator{Name: "pr", Run: AttributionPR})
	}
	return out
}

func base() []Validator {
	return []Validator{
		{Name: "skill", Run: Skills},
		{Name: "spec", Run: Specs},
		{Name: "plan", Run: Plans},
		{Name: "wiki", Run: Wiki},
		{Name: "adr", Run: ADR},
		{Name: "glossary", Run: Glossary},
		{Name: "stack", Run: Stack},
		{Name: "codewiki", Run: Codewiki},
		{Name: "notes", Run: Notes},
		{Name: "attribution", Run: Attribution},
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
// One exit code and one document, because ten validators the user has to invoke
// separately is nine chances to skip one. The per-validator counts come back
// alongside so the report can group by check without re-running anything.
func Everything(root string, opts ...Option) (*finding.Set, []Result, error) {
	set := &finding.Set{}
	var results []Result
	for _, v := range All(opts...) {
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
