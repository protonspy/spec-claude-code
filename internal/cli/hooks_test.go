package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/hooks"
)

// gitWorkspace is a scaffolded workspace that is also a git repository, which is
// what the hooks need: git decides where they go, so a temp directory with no
// repository has nowhere to put them.
// gitInit is a temp directory that is a git repository and nothing else — the
// half of gitWorkspace a test needs when it wants to choose its own harness.
func gitInit(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-q", ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return root
}

func gitWorkspace(t *testing.T) string {
	t.Helper()
	root := gitInit(t)
	if _, stderr, code := run(t, "init", "--no-rtk", "--claude", "--root", root); code != ExitOK {
		t.Fatalf("scc init: exit = %d (stderr: %s)", code, stderr)
	}
	return root
}

// TestInitInstallsTheHooks — they are on by default, because a gate somebody has
// to opt into is a gate the one session that skipped it never had.
func TestInitInstallsTheHooks(t *testing.T) {
	root := gitWorkspace(t)
	stdout, stderr, code := run(t, "hooks", "check", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("hooks check: exit = %d, want %d (stderr: %s)", code, ExitOK, stderr)
	}
	var doc struct {
		Stages []hooks.Status `json:"stages"`
		OK     bool           `json:"ok"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if !doc.OK || len(doc.Stages) != len(hooks.Stages()) {
		t.Errorf("init left the hooks at %+v", doc.Stages)
	}
}

// TestInitCanBeToldNotTo: --no-hooks is the whole opt-out, and a workspace
// scaffolded with it reports the gate as missing rather than pretending.
func TestInitCanBeToldNotTo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-q", ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if _, stderr, code := run(t, "init", "--no-rtk", "--claude", "--no-hooks", "--root", root); code != ExitOK {
		t.Fatalf("scc init: exit = %d (stderr: %s)", code, stderr)
	}
	// Missing hooks are a finding, not a failure to run: the same 2 the validators
	// return, so CI branches on it the same way.
	if _, _, code := run(t, "hooks", "check", "--root", root); code != ExitFindings {
		t.Errorf("hooks check exit = %d, want %d", code, ExitFindings)
	}
}

// TestInitOutsideAGitRepoStillScaffolds. A directory with no repository has
// nowhere to put a hook, which is not a reason to refuse to set up a workspace.
func TestInitOutsideAGitRepoStillScaffolds(t *testing.T) {
	root := t.TempDir()
	if _, stderr, code := run(t, "init", "--no-rtk", "--claude", "--root", root); code != ExitOK {
		t.Fatalf("scc init: exit = %d (stderr: %s)", code, stderr)
	}
}

// TestCommitMsgRejectsTheHarnessFooter is the gate this whole path exists for:
// the message is still a file here, so the signature can be caught before it is in
// a commit rather than reported after it is in history.
func TestCommitMsgRejectsTheHarnessFooter(t *testing.T) {
	root := gitWorkspace(t)
	for _, tc := range []struct {
		name string
		msg  string
		want int
	}{
		{"co-author trailer", "fix: thing\n\nCo-Authored-By: Claude <noreply@anthropic.com>\n", ExitFindings},
		{"generated with", "fix: thing\n\n\U0001F916 Generated with [Claude Code](https://claude.com/claude-code)\n", ExitFindings},
		{"badge alone", "fix: thing\n\n\U0001F916 [Claude Code](https://example.invalid)\n", ExitFindings},
		{"bare name", "fix: thing\n\n\U0001F916 Claude Code\n", ExitFindings},
		{"ordinary message", "feat(harness): add opencode support\n\nCloses #4\n", ExitOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
			if err := os.WriteFile(path, []byte(tc.msg), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			_, stderr, code := run(t, "hooks", "run", "commit-msg", path, "--root", root)
			if code != tc.want {
				t.Errorf("exit = %d, want %d (stderr: %s)", code, tc.want, stderr)
			}
		})
	}
}

// TestCommitMsgIgnoresWhatGitStrips. git's own template sits under the comment
// character and `commit --verbose` pastes the diff below the scissors — a check
// reading either would report on text that never reaches a commit.
func TestCommitMsgIgnoresWhatGitStrips(t *testing.T) {
	root := gitWorkspace(t)
	path := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
	msg := "feat: a real subject\n" +
		"# Co-Authored-By: Claude <noreply@anthropic.com>\n" +
		scissors + "\n" +
		"diff --git a/x b/x\n+\U0001F916 Generated with [Claude Code](https://claude.com/claude-code)\n"
	if err := os.WriteFile(path, []byte(msg), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, stderr, code := run(t, "hooks", "run", "commit-msg", path, "--root", root); code != ExitOK {
		t.Errorf("exit = %d, want %d — the finding is in text git throws away (stderr: %s)", code, ExitOK, stderr)
	}
}

// TestInstalledHooksSurviveARemove, and the workspace says so at every step. The
// point is that check and remove agree with install about what is there.
func TestHooksInstallCheckRemoveAgree(t *testing.T) {
	root := gitWorkspace(t)
	if _, _, code := run(t, "hooks", "remove", "--root", root); code != ExitOK {
		t.Fatalf("hooks remove: exit = %d", code)
	}
	if _, _, code := run(t, "hooks", "check", "--root", root); code != ExitFindings {
		t.Error("check says the hooks are there after remove took them out")
	}
	stdout, _, code := run(t, "hooks", "install", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("hooks install: exit = %d", code)
	}
	if !strings.Contains(stdout, `"`+hooks.Added+`"`) {
		t.Errorf("install did not report adding anything: %s", stdout)
	}
	if _, _, code := run(t, "hooks", "check", "--root", root); code != ExitOK {
		t.Error("check says the hooks are missing right after install wrote them")
	}
}

// TestHooksRunNeedsAStageItInstalls — a typo in a hook script must fail loudly
// rather than pass silently, because a gate that exits 0 on a name it does not
// know is a gate that reports success it never verified.
func TestHooksRunNeedsAStageItInstalls(t *testing.T) {
	root := gitWorkspace(t)
	if _, _, code := run(t, "hooks", "run", "post-merge", "--root", root); code != ExitError {
		t.Error("hooks run accepted a stage scc does not install")
	}
	if _, _, code := run(t, "hooks", "run", "--root", root); code != ExitError {
		t.Error("hooks run accepted no stage at all")
	}
	if _, _, code := run(t, "hooks", "run", "commit-msg", "--root", root); code != ExitError {
		t.Error("commit-msg ran without the message file git always passes")
	}
}

// TestHooksWorkOutsideAWorkspace is the case scc's own repository is: a git
// repository that follows these rules without being a workspace. The artifact
// validators have nothing to read there; the rule about what a commit message may
// say binds exactly as hard, so the gate has to install and has to fire.
func TestHooksWorkOutsideAWorkspace(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-q", ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if _, stderr, code := run(t, "hooks", "install", "--root", root); code != ExitOK {
		t.Fatalf("hooks install outside a workspace: exit = %d (stderr: %s)", code, stderr)
	}
	if _, _, code := run(t, "hooks", "check", "--root", root); code != ExitOK {
		t.Error("check does not see the hooks install just wrote")
	}
	// pre-commit has no artifacts to read and must still succeed rather than
	// refusing: a gate that errors where it has nothing to check is a gate removed.
	if _, stderr, code := run(t, "hooks", "run", "pre-commit", "--root", root); code != ExitOK {
		t.Errorf("pre-commit exit = %d, want %d (stderr: %s)", code, ExitOK, stderr)
	}
	path := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
	if err := os.WriteFile(path, []byte("fix: thing\n\nCo-Authored-By: Claude <x@y.com>\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, code := run(t, "hooks", "run", "commit-msg", path, "--root", root); code != ExitFindings {
		t.Error("commit-msg let a signature through outside a workspace")
	}
}

// TestHooksInstallRefusesOutsideARepository — a hook written where git will not
// read it is a failure that looks like success, so it is an error and not a shrug.
func TestHooksInstallRefusesOutsideARepository(t *testing.T) {
	if _, _, code := run(t, "hooks", "install", "--root", t.TempDir()); code != ExitError {
		t.Error("hooks install answered for a directory that is not a git repository")
	}
}

// The suite runs at pre-push and nowhere else. A push is where a branch becomes a
// pull request, which is where this methodology says the claim about tests is
// made — and it is the one moment in the cycle where waiting for a real answer is
// proportionate. On every commit it is not, and a pre-commit hook somebody
// uninstalls takes the other ten validators with it.
func TestPrePushRunsTheSuiteAndPreCommitDoesNot(t *testing.T) {
	root := gitWorkspace(t)
	marker := filepath.Join(root, "ran.txt")
	setTest(t, root, "echo ran > "+filepath.ToSlash(marker))

	if _, stderr, code := run(t, "hooks", "run", "pre-commit", "--root", root); code != ExitOK {
		t.Fatalf("pre-commit: exit = %d (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the pre-commit hook ran the test suite")
	}

	// pre-push does run it — and this command prints no report, so the gate fires
	// rather than passing on a suite it could not read.
	_, stderr, code := run(t, "hooks", "run", "pre-push", "--root", root)
	if code != ExitFindings {
		t.Fatalf("pre-push: exit = %d, want %d (stderr: %s)", code, ExitFindings, stderr)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the pre-push hook did not run the test suite: %v", err)
	}
}

// A workspace that never wired its tests up pushes exactly as it did before. The
// gate is a default here rather than something asked for by name, and a default
// that blocked every push over configuration nobody chose would be scc deciding
// how somebody else's project is tested.
func TestPrePushIsSilentWithoutATestCommand(t *testing.T) {
	root := gitWorkspace(t)
	stdout, stderr, code := run(t, "hooks", "run", "pre-push", "--root", root)
	if code != ExitOK {
		t.Fatalf("pre-push: exit = %d, want %d (stderr: %s)", code, ExitOK, stderr)
	}
	if strings.Contains(stdout+stderr, "not-configured") {
		t.Errorf("the hook demanded a test command nobody asked it to check: %q", stdout+stderr)
	}
}
