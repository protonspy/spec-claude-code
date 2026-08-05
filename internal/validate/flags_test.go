package validate

import "testing"

// Each of these is a defect a loop cannot recover from on its own: it either runs on
// a fact nobody wrote or waits forever for one that will never arrive.
func TestTaskFlagFindings(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{
			"a flag nobody defined is a typo, not prose",
			"- [ ] 1.1 (Unit) Do it\n  _Depend 1.0_\n",
			"task.unknown-flag",
		},
		{
			"two of the same flag are two answers to one question",
			"- [ ] 1.1 (Unit) Do it\n  _Priority 1_\n  _Priority 2_\n",
			"task.duplicate-flag",
		},
		{
			"a priority that is not a positive whole number orders nothing",
			"- [ ] 1.1 (Unit) Do it\n  _Priority soon_\n",
			"task.invalid-priority",
		},
		{
			"the box is the state, so a status that restates it is two records of one fact",
			"- [ ] 1.1 (Unit) Do it\n  _Status open_\n",
			"task.status-duplicates-box",
		},
		{
			"and any other status is simply not one",
			"- [ ] 1.1 (Unit) Do it\n  _Status parked_\n",
			"task.invalid-status",
		},
		{
			"a removal with no reason is a line nobody can explain later",
			"- [ ] 1.1 (Unit) Do it\n  _Status removed_\n",
			"task.removed-without-reason",
		},
		{
			"removed and ticked at once",
			"- [x] 1.1 (Unit) Do it\n  _Status removed_\n  _Reason it went away_\n",
			"task.removed-but-checked",
		},
		{
			"a dependency on a task that is not here never resolves",
			"- [ ] 1.1 (Unit) Do it\n  _Depends 9.9_\n",
			"task.unknown-dependency",
		},
		{
			"a task waiting for itself",
			"- [ ] 1.1 (Unit) Do it\n  _Depends 1.1_\n",
			"task.self-dependency",
		},
		{
			"a dependency on a removed task will never be ticked",
			"- [ ] 1.1 (Unit) Do it\n  _Depends 1.2_\n" +
				"- [ ] 1.2 (Unit) Dropped\n  _Status removed_\n  _Reason not needed_\n",
			"task.depends-on-removed",
		},
		{
			"a cycle is a deadlock --next cannot explain",
			"- [ ] 1.1 (Unit) One\n  _Depends 1.2_\n- [ ] 1.2 (Unit) Two\n  _Depends 1.1_\n",
			"task.dependency-cycle",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writePlan(t, root, "sweep", plan("", tc.body))
			if got := planFindings(t, root, "sweep"); !contains(got, tc.want) {
				t.Errorf("rules = %v, want %s", got, tc.want)
			}
		})
	}
}

// The flags are additive: a plan that carries none is exactly as valid as it was
// before they existed, which is what makes this half of the change safe to ship.
func TestPlansWithoutFlagsAreUnaffected(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "sweep", plan("", "- [ ] 1.1 (Unit) Do it\n- [ ] 1.2 (TDD) Do the other thing\n"))
	if got := planFindings(t, root, "sweep"); len(got) != 0 {
		t.Errorf("a plan with no flags reported %v", got)
	}
}

// Italic prose that is not sitting where a flag sits is prose. The parser only looks
// directly under a task, which is what keeps emphasis in a `## Why` paragraph from
// becoming a finding.
func TestEmphasisElsewhereIsNotAFlag(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "sweep", plan("", "- [ ] 1.1 (Unit) Do it\n\nSome prose.\n\n_An emphasized aside._\n"))
	if got := planFindings(t, root, "sweep"); contains(got, "task.unknown-flag") {
		t.Errorf("emphasis away from a task was read as a flag: %v", got)
	}
}
