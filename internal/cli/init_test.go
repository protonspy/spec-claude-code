package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/assets"
	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// initWorkspace scaffolds a throwaway workspace and returns its root. Every test
// passes --root explicitly rather than changing directory: os.Chdir is process-wide
// and would make these tests unsafe to run alongside anything else.
func initWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if _, stderr, code := run(t, "init", "--root", root); code != ExitOK {
		t.Fatalf("init: exit = %d (stderr: %s)", code, stderr)
	}
	return root
}

func TestInitCreatesAWorkspace(t *testing.T) {
	root := initWorkspace(t)
	if !workspace.IsWorkspace(root) {
		t.Fatal("init did not leave the workspace marker")
	}
	for _, f := range assets.Workspace() {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(f.Rel))); err != nil {
			t.Errorf("%s missing after init: %v", f.Rel, err)
		}
	}
	for _, dir := range []string{paths.Specs(root), paths.Plans(root), paths.Wiki(root), paths.ADR(root)} {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Errorf("%s was not created: %v", dir, err)
		}
	}
}

func TestInitJSONReportsEveryAction(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := run(t, "init", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	var got struct {
		Root    string `json:"root"`
		Created int    `json:"created"`
		Changes []struct {
			Path   string `json:"path"`
			Action string `json:"action"`
		} `json:"changes"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if got.Created != len(assets.Workspace()) || len(got.Changes) != len(assets.Workspace()) {
		t.Errorf("created = %d, changes = %d, want %d of each", got.Created, len(got.Changes), len(assets.Workspace()))
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty so stdout can be piped", stderr)
	}
}

// Re-running init is the honest upgrade story until `scc update` exists, so it has
// to be free: nothing written, nothing reported as changed, exit 0.
func TestInitIsIdempotent(t *testing.T) {
	root := initWorkspace(t)
	before, err := os.ReadFile(paths.Manifest(root))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	stdout, stderr, code := run(t, "init", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	var got struct {
		Created         int  `json:"created"`
		Skipped         int  `json:"skipped"`
		ManifestWritten bool `json:"manifestWritten"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Created != 0 || got.ManifestWritten {
		t.Errorf("second init wrote something: %+v", got)
	}
	after, err := os.ReadFile(paths.Manifest(root))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(before) != string(after) {
		t.Error("the manifest changed across two identical inits")
	}
}

// Never author what the user owns.
func TestInitLeavesEditedFilesAlone(t *testing.T) {
	root := initWorkspace(t)
	rule := filepath.Join(root, filepath.FromSlash(".claude/rules/methodology.md"))
	mine := "# my own methodology\n"
	if err := workspace.AtomicWrite(rule, []byte(mine), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	if _, stderr, code := run(t, "init", "--root", root); code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	got, err := os.ReadFile(rule)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != mine {
		t.Errorf("init overwrote an edited rule:\n%s", got)
	}
}

// --force overwrites, and says out loud what it destroyed. A silent clobber is the
// outcome that would make the tool untrustworthy.
func TestInitForceNamesWhatItClobbered(t *testing.T) {
	root := initWorkspace(t)
	rule := filepath.Join(root, filepath.FromSlash(".claude/rules/routing.md"))
	if err := workspace.AtomicWrite(rule, []byte("# mine\n"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	_, stderr, code := run(t, "init", "--root", root, "--force")
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, ".claude/rules/routing.md") {
		t.Errorf("stderr did not name the clobbered file: %q", stderr)
	}
	got, err := os.ReadFile(rule)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(got), "# mine") {
		t.Error("--force did not restore the template")
	}
}

func TestInitRejectsAMissingRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	_, stderr, code := run(t, "init", "--root", missing)
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "does not exist") {
		t.Errorf("stderr = %q, want it to say the root does not exist", stderr)
	}
}

// A stray positional is usually a name the user meant for another subcommand.
// Ignoring it would report success for something scc did not do.
func TestInitRejectsPositionals(t *testing.T) {
	root := t.TempDir()
	if _, _, code := run(t, "init", "--root", root, "user-auth"); code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
}

// The scaffolded workspace has to pass scc's own conventions: the manifest is a
// regular file (the marker contract), and every managed path is inside the root.
func TestScaffoldedWorkspaceIsSelfConsistent(t *testing.T) {
	root := initWorkspace(t)
	info, err := os.Stat(paths.Manifest(root))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("the marker is not a regular file: %v", err)
	}
	for _, f := range assets.Workspace() {
		if strings.HasPrefix(f.Rel, "/") || strings.Contains(f.Rel, "..") {
			t.Errorf("%s escapes the workspace root", f.Rel)
		}
	}
}

// finding duplicates the exit-code contract because the dependency runs the other
// way (cli imports finding). This is the guard that keeps the two from drifting.
func TestFindingExitCodesMatchTheCLIContract(t *testing.T) {
	if finding.ExitOK != ExitOK || finding.ExitFindings != ExitFindings {
		t.Errorf("finding exit codes (%d/%d) drifted from the CLI's (%d/%d)",
			finding.ExitOK, finding.ExitFindings, ExitOK, ExitFindings)
	}
}
