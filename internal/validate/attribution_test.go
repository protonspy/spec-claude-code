package validate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repo builds a throwaway git repository with one commit on the base branch, and
// returns its root. A test that wants a feature branch makes one on top.
//
// It configures identity and disables signing locally, because a machine whose
// global git config demands a GPG key would otherwise fail every commit here for
// a reason that has nothing to do with what is being tested.
func repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	root := t.TempDir()
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "dev@example.com"},
		{"config", "user.name", "A Developer"},
		{"config", "commit.gpgsign", "false"},
	} {
		gitIn(t, root, args...)
	}
	commit(t, root, "chore: first")
	return root
}

func gitIn(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// commit writes a file nobody reads and records it under the given message. The
// file changes every time so git always has something to commit; the message is
// the only part any test here cares about.
func commit(t *testing.T, root, message string) {
	t.Helper()
	name := filepath.Join(root, "file.txt")
	prior, _ := os.ReadFile(name)
	if err := os.WriteFile(name, append(prior, 'x'), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-m", message)
}

// The signature this whole check exists for, verbatim.
const signed = "fix(cli): return exit 2 on findings\n\nCo-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"

func TestAttributionReportsASignatureOnThisBranch(t *testing.T) {
	root := repo(t)
	gitIn(t, root, "switch", "-c", "feat/thing")
	commit(t, root, signed)

	got := runValidator(t, Attribution, root)
	if len(got) != 1 || got[0] != "attribution.co-authored-by" {
		t.Fatalf("rules = %v, want [attribution.co-authored-by]", got)
	}
}

func TestAttributionLeavesOrdinaryCommitsAlone(t *testing.T) {
	root := repo(t)
	gitIn(t, root, "switch", "-c", "feat/thing")
	commit(t, root, "chore(deps): bump the anthropic SDK to 0.40")
	commit(t, root, "feat(harness): add opencode support")

	if got := runValidator(t, Attribution, root); len(got) != 0 {
		t.Errorf("findings on ordinary commits: %v", got)
	}
}

// The base's own history is somebody else's. A check that read it would report
// the same finding on every branch in the repository, forever.
func TestAttributionIgnoresWhatIsAlreadyOnTheBase(t *testing.T) {
	root := repo(t)
	commit(t, root, signed)
	gitIn(t, root, "switch", "-c", "feat/thing")
	commit(t, root, "feat: something clean")

	if got := runValidator(t, Attribution, root); len(got) != 0 {
		t.Errorf("findings from the base branch's history: %v", got)
	}
}

// A local main nobody pulled is the common shape once pull requests merge on the
// forge. A signed commit that already reached origin/main through somebody else's
// merge is not this branch's to fix, and blocking the push over it blocks every
// branch cut from the remote until the local copy is pulled.
func TestAttributionIgnoresWhatIsAlreadyOnTheRemoteBase(t *testing.T) {
	root := repo(t)
	remote := t.TempDir()
	gitIn(t, remote, "init", "--bare", "--initial-branch=main")
	gitIn(t, root, "remote", "add", "origin", remote)
	gitIn(t, root, "push", "-u", "origin", "main")
	commit(t, root, signed)
	gitIn(t, root, "push", "origin", "main")
	gitIn(t, root, "switch", "-c", "fix/thing")
	gitIn(t, root, "branch", "-f", "main", "HEAD~1")
	commit(t, root, "fix: something clean")

	if got := runValidator(t, Attribution, root); len(got) != 0 {
		t.Errorf("findings from a commit already on origin/main: %v", got)
	}
}

// Standing on the base with no remote leaves nothing to compare against, which is
// silence rather than a failure — and the common shape of a fresh local repo.
func TestAttributionIsSilentOnTheBaseWithNoRemote(t *testing.T) {
	root := repo(t)
	commit(t, root, signed)

	if got := runValidator(t, Attribution, root); len(got) != 0 {
		t.Errorf("findings with no branch to compare: %v", got)
	}
}

// The property the user asked for by name: a project with no git configured must
// not blow up. `Everything` turns a validator error into exit 1, so returning one
// here would break `scc validate` in every workspace that is not a repository —
// over a rule about git.
func TestAttributionNeverErrorsWithoutGit(t *testing.T) {
	for _, root := range []string{t.TempDir(), filepath.Join(t.TempDir(), "does-not-exist")} {
		set, err := Attribution(root)
		if err != nil {
			t.Fatalf("Attribution(%q) errored: %v", root, err)
		}
		if !set.Empty() {
			t.Errorf("Attribution(%q) reported %d findings outside a repository", root, set.Len())
		}
	}
}

// An unborn HEAD — `git init` and nothing since — is a repository with no history
// at all, and the range git is asked for cannot resolve.
func TestAttributionIsSilentOnAnUnbornHead(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	root := t.TempDir()
	gitIn(t, root, "init", "--initial-branch=main")

	set, err := Attribution(root)
	if err != nil {
		t.Fatalf("Attribution errored on an unborn HEAD: %v", err)
	}
	if !set.Empty() {
		t.Errorf("findings on an unborn HEAD: %d", set.Len())
	}
}

// A finding has to be actionable: the short sha is what `git show` takes, and the
// subject is what the reader recognizes in a rebase.
func TestAttributionNamesTheCommit(t *testing.T) {
	root := repo(t)
	gitIn(t, root, "switch", "-c", "feat/thing")
	commit(t, root, signed)

	set, err := Attribution(root)
	if err != nil {
		t.Fatalf("Attribution: %v", err)
	}
	f := set.Sorted()[0]
	if len(f.File) < 7 {
		t.Errorf("file = %q, want a short sha", f.File)
	}
	if f.Line != 3 {
		t.Errorf("line = %d, want 3 — the line of the trailer in the message", f.Line)
	}
	if !strings.Contains(f.Message, "fix(cli): return exit 2 on findings") {
		t.Errorf("message does not name the commit: %q", f.Message)
	}
	if !strings.Contains(f.Message, "Co-Authored-By: Claude") {
		t.Errorf("message does not quote the offending line: %q", f.Message)
	}
}
