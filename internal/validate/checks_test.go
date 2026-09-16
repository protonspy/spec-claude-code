package validate

import (
	"os"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/assets"
	"github.com/protonspy/spec-claude-code/internal/gate"
	sccmanifest "github.com/protonspy/spec-claude-code/internal/manifest"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

func gateWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(paths.Claude.Config(root), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := sccmanifest.Save(root, paths.Claude, sccmanifest.New(assets.Version, paths.Claude)); err != nil {
		t.Fatalf("manifest.Save: %v", err)
	}
	return root
}

// The findings are deliberately distinct rather than one "the gate failed". A
// command that did not run, a test command whose report could not be read, a suite
// with no tests, and one that came up short are four different things to go and do,
// and a gate that called them all one thing would send the reader to the same dead
// end four times.
func TestCheckFindingsTellsTheFailuresApart(t *testing.T) {
	root := gateWorkspace(t)

	for _, tc := range []struct {
		name string
		res  gate.Result
		rule string
		says []string
	}{
		{
			name: "timed out",
			res:  gate.Result{Kind: gate.Test, Command: "go test ./...", TimedOut: true},
			rule: "check.timed-out",
			says: []string{"go test ./...", "still running"},
		},
		{
			name: "failed",
			res:  gate.Result{Kind: gate.Build, Command: "go build ./...", ExitCode: 2, Tail: "undefined: x"},
			rule: "check.failed",
			// The tail is there because "the gate failed" with no excerpt sends the
			// reader off to re-run it by hand.
			says: []string{"go build ./...", "exited 2", "undefined: x"},
		},
		{
			name: "no report",
			res:  gate.Result{Kind: gate.Test, Command: "go test ./...", ExitCode: 0},
			rule: "tests.no-report",
			says: []string{"total", "coverage", "stdout"},
		},
		{
			name: "no tests",
			res: gate.Result{Kind: gate.Test, Command: "go test ./...", Reported: true,
				Report: gate.Report{Total: 0, Coverage: 100}},
			rule: "tests.none",
			says: []string{"0 tests"},
		},
		{
			name: "below floor",
			res: gate.Result{Kind: gate.Test, Command: "go test ./...", Reported: true,
				Report: gate.Report{Total: 800, Coverage: 73.8}, Floor: 86},
			rule: "tests.below-floor",
			says: []string{"73.8", "86"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := CheckFindings(root, tc.res)
			if set.Empty() {
				t.Fatalf("no finding for %s", tc.name)
			}
			var found bool
			var msg string
			for _, f := range set.Sorted() {
				if f.Rule == tc.rule {
					found, msg = true, f.Message
				}
			}
			if !found {
				t.Fatalf("no %s finding: %+v", tc.rule, set.Sorted())
			}
			for _, want := range tc.says {
				if !strings.Contains(msg, want) {
					t.Errorf("%s message does not mention %q:\n%s", tc.rule, want, msg)
				}
			}
		})
	}

	// A passing build says nothing, and a passing test run over the floor says
	// nothing either — the other three gates are judged by their exit status alone.
	for _, res := range []gate.Result{
		{Kind: gate.Build, Command: "go build ./...", ExitCode: 0},
		{Kind: gate.Lint, Command: "golangci-lint run", ExitCode: 0, Tail: "0 issues."},
		{Kind: gate.Test, Command: "go test ./...", Reported: true,
			Report: gate.Report{Total: 800, Coverage: 91}, Floor: 86},
	} {
		if set := CheckFindings(root, res); !set.Empty() {
			t.Errorf("a passing %s gate reported %+v", res.Kind, set.Sorted())
		}
	}
}

// A gate nobody has decided on is a finding, and the message says what to write
// rather than only what is wrong — these are the only findings in the product
// whose fix is a command line rather than an edit to the file they point at. A
// gate somebody declined is silence: a language with no formatter is a real
// answer, and a project that recorded it has already had this conversation.
func TestChecksTellsAnUndecidedGateFromADeclinedOne(t *testing.T) {
	root := gateWorkspace(t)

	set, err := Checks(root)
	if err != nil {
		t.Fatalf("Checks: %v", err)
	}
	if set.Empty() {
		t.Fatal("a workspace that has decided nothing reported no findings")
	}
	first := set.Sorted()[0]
	if first.Rule != "check.not-configured" {
		t.Fatalf("first finding = %+v, want check.not-configured", first)
	}
	// Build first, because the four presuppose each other and a lint report over a
	// broken build is derived noise.
	if !strings.Contains(first.Message, string(gate.Build)) {
		t.Errorf("the first finding is about %q, want the build gate", first.Message)
	}
	for _, want := range []string{"scc check set", "scc check skip"} {
		if !strings.Contains(first.Message, want) {
			t.Errorf("the message offers no way out via %q:\n%s", want, first.Message)
		}
	}

	// Decline all four, and the validator goes quiet about every one of them.
	cfg := gate.Config{Commands: map[gate.Kind]string{}}
	for _, k := range gate.Kinds() {
		cfg.Commands[k] = gate.Skipped
	}
	if _, err := gate.Save(root, cfg); err != nil {
		t.Fatalf("gate.Save: %v", err)
	}
	set, err = Checks(root)
	if err != nil {
		t.Fatalf("Checks: %v", err)
	}
	if !set.Empty() {
		t.Errorf("a workspace that declined every gate reported %+v", set.Sorted())
	}
}

// The set is what the aggregate command runs, and the two options are behind a
// flag for one reason: cost. Everything in base() reads files already on disk;
// these run a compiler and go to the network.
func TestTheValidatorSetIsAssembledByCost(t *testing.T) {
	names := func(all []Validator) []string {
		var out []string
		for _, v := range all {
			out = append(out, v.Name)
		}
		return out
	}

	def := names(All())
	if len(def) == 0 {
		t.Fatal("All() returned no validators")
	}
	for _, off := range []string{"checks", "pr"} {
		if contains(def, off) {
			t.Errorf("%q runs by default, and it is behind a flag for cost", off)
		}
	}
	if !contains(def, "attribution") {
		t.Error("attribution does not run by default")
	}

	// The Stop hook may only carry what the next turn can resolve, and a signature
	// in a commit already made comes out with a rebase rather than an edit — so it
	// would re-appear at the end of every turn for the life of the branch.
	if quiet := names(All(WithoutAttribution())); contains(quiet, "attribution") {
		t.Error("WithoutAttribution left the history check in the run")
	} else if len(quiet) != len(def)-1 {
		t.Errorf("WithoutAttribution changed the set by more than the one check: %v", quiet)
	}

	if withPR := names(All(WithPR())); !contains(withPR, "pr") {
		t.Errorf("WithPR did not add the pull-request check: %v", withPR)
	}

	full := names(All(WithChecks(), WithPR()))
	if !contains(full, "checks") || !contains(full, "pr") {
		t.Errorf("both options together produced %v", full)
	}
	// Checks before pr: they are the slow ones, and a run that is going to fail on
	// a broken build should say so before it spends a round trip on the forge.
	if indexOf(full, "checks") > indexOf(full, "pr") {
		t.Errorf("pr runs before checks: %v", full)
	}
	// And the artifact checks come first, so a scan of the report finds them
	// together with the history check after them.
	if indexOf(full, "checks") < indexOf(full, "attribution") {
		t.Errorf("the delivery gate runs before the artifact checks: %v", full)
	}

	for _, v := range full {
		if v == "" {
			t.Error("a validator has no name, so no finding can be traced to it")
		}
	}
}

func indexOf(all []string, want string) int {
	for i, s := range all {
		if s == want {
			return i
		}
	}
	return -1
}

// The plan's shape is a closed set of six sections, and PlanSections is that
// contract in the order the template writes it — the one thing that ever capped a
// plan's size, because there is deliberately nowhere for prose to go.
func TestPlanSectionsIsTheContractAndACopy(t *testing.T) {
	all := PlanSections()
	if len(all) != 6 {
		t.Fatalf("PlanSections returned %d sections, want the six the contract has: %+v", len(all), all)
	}
	var required int
	for _, s := range all {
		if s.Title == "" || s.Slug == "" {
			t.Errorf("a section is not fully named: %+v", s)
		}
		if s.Required {
			required++
		}
	}
	if required == 0 {
		t.Error("no section is required, so an empty file satisfies the contract")
	}

	// A copy, not the table: a caller that sorted or truncated what it got back
	// would change what every later plan is held to.
	all[0].Title = "Mutated"
	if PlanSections()[0].Title == "Mutated" {
		t.Error("PlanSections handed out the contract itself")
	}
}

// The pair has to travel together: a `status: approved` with no checksum is a plan
// claiming to be sealed by a seal that is not there, and every read of it would
// silently skip the check it was approved to get.
func TestAnApprovedPlanWithNoChecksumIsAFinding(t *testing.T) {
	root := t.TempDir()
	const task = "- [ ] 1.1 (Unit) Do the thing\n"

	// A plan with no status is never checked, which is what makes every plan
	// written before the seal shipped keep passing.
	writePlan(t, root, "none", plan("", task))
	set, err := Plans(root)
	if err != nil || !set.Empty() {
		t.Errorf("a plan with no status: %v, %+v", err, set.Sorted())
	}

	writePlan(t, root, "none", plan("---\nstatus: approved\n---\n\n", task))
	set, err = Plans(root)
	if err != nil {
		t.Fatalf("Plans: %v", err)
	}
	if !hasRule(set, "plan.unsealed") {
		t.Errorf("an approved plan with no checksum reported %+v", set.Sorted())
	}

	// A status outside the vocabulary is its own finding, and the checksum is not
	// then also demanded — one finding that names the real problem beats two.
	writePlan(t, root, "none", plan("---\nstatus: finished\n---\n\n", task))
	set, err = Plans(root)
	if err != nil {
		t.Fatalf("Plans: %v", err)
	}
	if !hasRule(set, "plan.status-invalid") {
		t.Errorf("an unknown status reported %+v", set.Sorted())
	}
	if hasRule(set, "plan.unsealed") {
		t.Errorf("an unknown status also demanded a checksum: %+v", set.Sorted())
	}

	// Draft is the other half of the vocabulary and needs no seal.
	writePlan(t, root, "none", plan("---\nstatus: draft\n---\n\n", task))
	set, err = Plans(root)
	if err != nil {
		t.Fatalf("Plans: %v", err)
	}
	if !set.Empty() {
		t.Errorf("a draft plan reported %+v", set.Sorted())
	}
}

// The pull request is the half no hook can reach: a commit message passes through
// commit-msg on the machine that wrote it, and a PR body is typed straight into
// the forge, which is why the footer that dies everywhere else survives there.
//
// Like its sibling it never returns an error. No gh, no authentication, no pull
// request for this branch: all of them mean there is nothing to check, and none of
// them is this validator failing.
func TestAttributionPRIsSilentWhenThereIsNothingToAsk(t *testing.T) {
	// Not a repository at all.
	set, err := AttributionPR(t.TempDir())
	if err != nil {
		t.Fatalf("AttributionPR outside a repository: %v", err)
	}
	if !set.Empty() {
		t.Errorf("a directory that is not a repository reported %+v", set.Sorted())
	}

	// A repository with no gh on PATH: the question cannot be asked, which is not
	// the same as an answer of "clean" and is certainly not an error.
	t.Setenv("PATH", t.TempDir())
	set, err = AttributionPR(t.TempDir())
	if err != nil {
		t.Fatalf("AttributionPR with no gh: %v", err)
	}
	if !set.Empty() {
		t.Errorf("a workspace with no gh reported %+v", set.Sorted())
	}
}

// clipSubject names the commit in words next to the short sha a finding is filed
// under: the sha is what `git show` takes, the subject is what the reader
// recognizes, and neither on its own finds the commit again after a rebase.
func TestClipSubjectKeepsTheLineReadable(t *testing.T) {
	short := "feat(cli): return exit 2 on findings"
	if got := clipSubject(short); got != short {
		t.Errorf("clipSubject shortened a subject that fits: %q", got)
	}
	long := strings.Repeat("x", 200)
	got := clipSubject(long)
	if len(got) >= len(long) {
		t.Errorf("clipSubject did not shorten a %d-character subject", len(long))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("a clipped subject does not say it was clipped: %q", got)
	}
}
