package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/artifact"
	"github.com/protonspy/spec-claude-code/internal/git"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// reconcile is the whole policy, and it is a pure function precisely so the rules can
// be read here without a repository. Every row is a state somebody's workspace is
// actually in.
func TestReconcile(t *testing.T) {
	rec := func(branch string, pr int, state string) artifact.Delivery {
		return artifact.Delivery{Branch: branch, PR: pr, State: state}
	}
	alive := func(ahead int) evidence {
		return evidence{branch: git.Branch{Name: "feat/x", Local: true, Base: "main", Ahead: ahead}}
	}
	landed := evidence{branch: git.Branch{Name: "feat/x", Local: true, Base: "main", Behind: 2, Merged: true}}
	gone := evidence{branch: git.Branch{Name: "feat/x", Base: "main"}}
	pr := func(state string) evidence {
		return evidence{askedPR: true, pr: git.PR{Number: 28, State: state}}
	}

	for _, tc := range []struct {
		name      string
		was       artifact.Delivery
		ev        evidence
		want      string
		changed   bool
		undecided bool
	}{
		{"a branch with work on it is in progress",
			rec("feat/x", 0, ""), alive(3), artifact.DeliveryInProgress, true, false},
		{"a branch level with the base has not delivered anything",
			rec("feat/x", 0, artifact.DeliveryInProgress), alive(0), artifact.DeliveryInProgress, false, false},
		{"a landed branch is merged",
			rec("feat/x", 0, artifact.DeliveryInProgress), landed, artifact.DeliveryMerged, true, false},
		{"a deleted branch with nothing to ask is undecided, not guessed",
			rec("feat/x", 0, artifact.DeliveryInProgress), gone, artifact.DeliveryInProgress, false, true},
		{"a settled record does not re-report the ambiguity",
			rec("feat/x", 0, artifact.DeliveryMerged), gone, artifact.DeliveryMerged, false, false},
		{"an open PR is in review",
			rec("feat/x", 28, artifact.DeliveryInProgress), pr(git.StateOpen), artifact.DeliveryInReview, true, false},
		{"a merged PR settles it even with the branch gone",
			rec("feat/x", 28, artifact.DeliveryInReview), pr(git.StateMerged), artifact.DeliveryMerged, true, false},
		{"a PR closed unmerged is abandoned",
			rec("feat/x", 28, artifact.DeliveryInReview), pr(git.StateClosed), artifact.DeliveryAbandoned, true, false},
		{"a recorded PR with no gh is undecided rather than downgraded",
			rec("", 28, artifact.DeliveryInReview), evidence{}, artifact.DeliveryInReview, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := reconcile("feature", tc.was, tc.ev)
			if got.Now.State != tc.want {
				t.Errorf("state = %q, want %q (why: %q, undecided: %q)",
					got.Now.State, tc.want, got.Why, got.Undecided)
			}
			if got.Changed != tc.changed {
				t.Errorf("changed = %v, want %v", got.Changed, tc.changed)
			}
			if (got.Undecided != "") != tc.undecided {
				t.Errorf("undecided = %q, want present=%v", got.Undecided, tc.undecided)
			}
		})
	}
}

// The PR wins over the branch whenever it answered: a merged PR whose branch is still
// sitting there locally is merged, not in progress.
func TestReconcilePrefersThePullRequest(t *testing.T) {
	was := artifact.Delivery{Branch: "feat/x", PR: 28, State: artifact.DeliveryInReview}
	ev := evidence{
		branch:  git.Branch{Name: "feat/x", Local: true, Base: "main", Ahead: 4},
		askedPR: true,
		pr:      git.PR{Number: 28, State: git.StateMerged},
	}
	if got := reconcile("feature", was, ev); got.Now.State != artifact.DeliveryMerged {
		t.Errorf("state = %q, want %q — the forge stated the outcome outright",
			got.Now.State, artifact.DeliveryMerged)
	}
}

func trackedSpec(t *testing.T) (root string) {
	t.Helper()
	root = initWorkspace(t)
	if _, stderr, code := run(t, "spec", "new", "user-auth", "--root", root); code != ExitOK {
		t.Fatalf("spec new: %d (%s)", code, stderr)
	}
	return root
}

func requirements(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(paths.Requirements(root, "user-auth"))
	if err != nil {
		t.Fatalf("read requirements: %v", err)
	}
	return string(b)
}

func TestSpecTrackWritesTheRecord(t *testing.T) {
	root := trackedSpec(t)

	if _, stderr, code := run(t, "spec", "track", "user-auth", "--branch", "feat/user-auth", "--root", root); code != ExitOK {
		t.Fatalf("track: %d (%s)", code, stderr)
	}
	body := requirements(t, root)
	for _, want := range []string{"branch: feat/user-auth", "delivery: " + artifact.DeliveryInProgress} {
		if !strings.Contains(body, want) {
			t.Errorf("requirements.md does not record %q:\n%s", want, body)
		}
	}
	// The kickoff answers are still there: the record is written into the block, not
	// over it.
	if !strings.Contains(body, "autonomy: auto") {
		t.Error("writing the record dropped the kickoff answers")
	}

	// A PR moves it on, and does not have to restate the branch.
	if _, stderr, code := run(t, "spec", "track", "user-auth", "--pr", "28", "--root", root); code != ExitOK {
		t.Fatalf("track --pr: %d (%s)", code, stderr)
	}
	body = requirements(t, root)
	for _, want := range []string{"branch: feat/user-auth", "pr: 28", "delivery: " + artifact.DeliveryInReview} {
		if !strings.Contains(body, want) {
			t.Errorf("requirements.md does not record %q:\n%s", want, body)
		}
	}
	if _, _, code := run(t, "validate", "--root", root); code != ExitOK {
		t.Error("a tracked spec does not pass validation")
	}
}

func TestSpecTrackRejectsWhatSyncCouldNotUse(t *testing.T) {
	root := trackedSpec(t)
	for _, args := range [][]string{
		{"spec", "track", "user-auth"},
		{"spec", "track", "user-auth", "--delivery", "shipping-soon"},
		{"spec", "track", "user-auth", "--pr", "-3"},
		{"spec", "track", "user-auth", "--branch", "two words"},
	} {
		if _, _, code := run(t, append(args, "--root", root)...); code != ExitError {
			t.Errorf("%v exited %d, want %d", args, code, ExitError)
		}
	}
	if strings.Contains(requirements(t, root), "delivery:") {
		t.Error("a rejected track still wrote something")
	}
}

func TestSpecTrackIsIdempotent(t *testing.T) {
	root := trackedSpec(t)
	run(t, "spec", "track", "user-auth", "--branch", "feat/x", "--root", root)
	first := requirements(t, root)
	stdout, _, code := run(t, "spec", "track", "user-auth", "--branch", "feat/x", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "already records") {
		t.Errorf("stdout = %q, want it to say nothing changed", stdout)
	}
	if requirements(t, root) != first {
		t.Error("re-recording the same values rewrote the file")
	}
}

func TestSpecTrackDryRunWritesNothing(t *testing.T) {
	root := trackedSpec(t)
	before := requirements(t, root)
	if _, _, code := run(t, "spec", "track", "user-auth", "--branch", "feat/x", "--dry-run", "--root", root); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if requirements(t, root) != before {
		t.Error("--dry-run wrote to the file")
	}
}

// The record has to reach `spec list` and `spec show`, because "what is still open"
// is asked of the listing and never of one spec's frontmatter.
func TestTheRecordShowsUpInListAndShow(t *testing.T) {
	root := trackedSpec(t)
	run(t, "spec", "track", "user-auth", "--branch", "feat/user-auth", "--pr", "28", "--root", root)

	stdout, _, _ := run(t, "spec", "list", "--root", root)
	for _, want := range []string{artifact.DeliveryInReview, "feat/user-auth", "#28"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("spec list = %q, want it to carry %q", stdout, want)
		}
	}

	stdout, _, _ = run(t, "spec", "show", "user-auth", "--json", "--root", root)
	var got struct {
		Delivery artifact.Delivery `json:"delivery"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("show --json is not JSON (%v): %q", err, stdout)
	}
	if got.Delivery.Branch != "feat/user-auth" || got.Delivery.PR != 28 ||
		got.Delivery.State != artifact.DeliveryInReview {
		t.Errorf("delivery = %+v", got.Delivery)
	}
}

// An untracked spec says so once, where somebody can act on it, and stays silent in
// the listing: a column of "not started" is a column nobody reads.
func TestAnUntrackedSpecIsNotAFinding(t *testing.T) {
	root := trackedSpec(t)
	if _, _, code := run(t, "validate", "--root", root); code != ExitOK {
		t.Error("an untracked spec produced findings")
	}
	stdout, _, _ := run(t, "spec", "show", "user-auth", "--root", root)
	if !strings.Contains(stdout, "not recorded") {
		t.Errorf("spec show = %q, want it to say the record is missing", stdout)
	}
}

func TestSpecSyncDegradesOutsideARepository(t *testing.T) {
	root := trackedSpec(t)
	run(t, "spec", "track", "user-auth", "--branch", "feat/x", "--root", root)
	// t.TempDir() is not a git repository, and a workspace outside one is legitimate.
	stdout, stderr, code := run(t, "spec", "sync", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit %d (%s)", code, stderr)
	}
	var got struct {
		Git bool `json:"git"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("not JSON (%v): %q", err, stdout)
	}
	if got.Git {
		t.Error("sync claimed a git repository where there is none")
	}
}

// The lifecycle against a real repository: branched, worked on, merged. Skipped where
// git is not installed rather than failing — this asserts scc's reading of git, and a
// machine without git has nothing to read.
func TestSpecSyncFollowsABranchToMerged(t *testing.T) {
	if !git.Found(git.Bin) {
		t.Skip("git is not on PATH")
	}
	root := trackedSpec(t)
	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command(git.Bin, args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	gitRun("init", "-q", "-b", "main", ".")
	gitRun("config", "user.email", "t@example.com")
	gitRun("config", "user.name", "t")
	gitRun("add", "-A")
	gitRun("commit", "-qm", "init")
	gitRun("switch", "-qc", "feat/user-auth")

	if _, stderr, code := run(t, "spec", "track", "user-auth", "--here", "--root", root); code != ExitOK {
		t.Fatalf("track --here: %d (%s)", code, stderr)
	}
	if !strings.Contains(requirements(t, root), "branch: feat/user-auth") {
		t.Fatal("--here did not read the checked-out branch")
	}

	// Freshly branched: nothing has landed, and the ancestor test alone would have
	// called this merged.
	if _, _, code := run(t, "spec", "sync", "--root", root); code != ExitOK {
		t.Fatal("sync failed")
	}
	if got := requirements(t, root); !strings.Contains(got, "delivery: "+artifact.DeliveryInProgress) {
		t.Errorf("a freshly branched spec is not in progress:\n%s", got)
	}

	if err := os.WriteFile(root+"/work.txt", []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	gitRun("add", "-A")
	gitRun("commit", "-qm", "work")
	run(t, "spec", "sync", "--root", root)
	if got := requirements(t, root); !strings.Contains(got, "delivery: "+artifact.DeliveryInProgress) {
		t.Errorf("a branch with unmerged work is not in progress:\n%s", got)
	}

	gitRun("switch", "-q", "main")
	gitRun("merge", "-q", "--no-ff", "feat/user-auth", "-m", "merge")
	stdout, _, code := run(t, "spec", "sync", "--root", root)
	if code != ExitOK {
		t.Fatal("sync failed after the merge")
	}
	if !strings.Contains(stdout, artifact.DeliveryMerged) {
		t.Errorf("sync = %q, want it to report the merge", stdout)
	}
	if got := requirements(t, root); !strings.Contains(got, "delivery: "+artifact.DeliveryMerged) {
		t.Errorf("the merge was not written back:\n%s", got)
	}

	// A branch deleted after the merge leaves the record settled and quiet.
	gitRun("branch", "-qD", "feat/user-auth")
	stdout, stderr, _ := run(t, "spec", "sync", "--root", root)
	if strings.Contains(stderr, "look the same") {
		t.Errorf("a settled spec re-reported the deleted-branch ambiguity: %q", stderr)
	}
	if !strings.Contains(stdout, "0 still open") {
		t.Errorf("sync = %q, want nothing left open", stdout)
	}
}
