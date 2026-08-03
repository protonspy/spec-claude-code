package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/paths"
)

// Every test here passes --no-install or puts a stub on PATH. A test that let the
// real path run would shell out to `cargo install` and spend minutes of CI on a
// network build — and would behave differently depending on whether the machine
// running it already had RTK.
func readEntry(t *testing.T, root, entry string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, entry))
	if err != nil {
		t.Fatalf("ReadFile %s: %v", entry, err)
	}
	return string(b)
}

// stubRTK puts a do-nothing `rtk` on PATH so the install branch is never taken.
func stubRTK(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script, name := "#!/bin/sh\necho 'rtk 0.0.0-stub'\n", "rtk"
	if runtime.GOOS == "windows" {
		script, name = "@echo rtk 0.0.0-stub\r\n", "rtk.bat"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatalf("stub rtk: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRTKAddsTheBlockToTheEntryFile(t *testing.T) {
	root := initWorkspace(t)
	before := readEntry(t, root, paths.Claude.EntryFile)

	stdout, stderr, code := run(t, "rtk", "--root", root, "--no-install")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, ExitOK, stderr)
	}
	got := readEntry(t, root, paths.Claude.EntryFile)
	if !strings.HasPrefix(got, before) {
		t.Error("the scaffolded entry file was rewritten rather than appended to")
	}
	if !strings.Contains(got, "<!-- rtk-instructions") || !strings.Contains(got, "<!-- /rtk-instructions -->") {
		t.Errorf("%s did not get the block:\n%s", paths.Claude.EntryFile, got)
	}
	if !strings.Contains(stdout, paths.Claude.EntryFile) {
		t.Errorf("stdout does not name the file it changed: %q", stdout)
	}
}

// The command an agent runs at the top of a session has to be free to re-run: a
// second pass reports the block as current and leaves the file byte-identical.
func TestRTKIsIdempotent(t *testing.T) {
	root := initWorkspace(t)
	if _, stderr, code := run(t, "rtk", "--root", root, "--no-install"); code != ExitOK {
		t.Fatalf("first run: exit = %d (stderr: %s)", code, stderr)
	}
	once := readEntry(t, root, paths.Claude.EntryFile)

	stdout, stderr, code := run(t, "rtk", "--root", root, "--no-install", "--json")
	if code != ExitOK {
		t.Fatalf("second run: exit = %d (stderr: %s)", code, stderr)
	}
	if got := readEntry(t, root, paths.Claude.EntryFile); got != once {
		t.Error("a second run changed the entry file")
	}
	var report rtkReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if report.Changed != 0 {
		t.Errorf("changed = %d, want 0", report.Changed)
	}
	if len(report.Files) != 1 || report.Files[0].Action != "present" {
		t.Errorf("files = %+v, want one file reported as present", report.Files)
	}
	if report.Files[0].Block == "" {
		t.Error("the report does not say which block version is in the file")
	}
}

// The block between RTK's markers is RTK's. scc inserts one where there is none and
// otherwise keeps its hands off — a newer block, or one the user edited, survives
// every re-run. --force is the separate, explicit decision.
func TestRTKLeavesAnExistingBlockAlone(t *testing.T) {
	root := initWorkspace(t)
	entry := filepath.Join(root, paths.Claude.EntryFile)
	base := readEntry(t, root, paths.Claude.EntryFile)
	theirs := base + "\n<!-- rtk-instructions v9 -->\n## RTK\nnewer text\n<!-- /rtk-instructions -->\n"
	if err := os.WriteFile(entry, []byte(theirs), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	stdout, stderr, code := run(t, "rtk", "--root", root, "--no-install", "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if got := readEntry(t, root, paths.Claude.EntryFile); got != theirs {
		t.Errorf("a newer block was rewritten:\n%s", got)
	}
	var report rtkReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if len(report.Files) != 1 || report.Files[0].Block != "v9" {
		t.Errorf("files = %+v, want the v9 block reported as found", report.Files)
	}
	// And --check calls it wired, because it is: the version is RTK's to judge.
	if _, _, code := run(t, "rtk", "--root", root, "--check"); code != ExitOK {
		t.Errorf("--check exit = %d, want %d with a block already in place", code, ExitOK)
	}

	// --force is the explicit "use the one this scc ships".
	if _, stderr, code := run(t, "rtk", "--root", root, "--no-install", "--force"); code != ExitOK {
		t.Fatalf("--force exit = %d (stderr: %s)", code, stderr)
	}
	got := readEntry(t, root, paths.Claude.EntryFile)
	if strings.Contains(got, "newer text") || strings.Contains(got, "v9") {
		t.Error("--force did not replace the block")
	}
	if !strings.HasPrefix(got, base) {
		t.Error("--force rewrote the document above the block")
	}
}

// --check is the CI question: is this workspace's entry file wired for RTK. It
// answers with the findings code and writes nothing.
func TestRTKCheckReportsFindingsAndWritesNothing(t *testing.T) {
	root := initWorkspace(t)
	before := readEntry(t, root, paths.Claude.EntryFile)

	if _, _, code := run(t, "rtk", "--root", root, "--check"); code != ExitFindings {
		t.Errorf("exit = %d, want %d on a workspace with no block", code, ExitFindings)
	}
	if got := readEntry(t, root, paths.Claude.EntryFile); got != before {
		t.Error("--check wrote to the entry file")
	}

	if _, stderr, code := run(t, "rtk", "--root", root, "--no-install"); code != ExitOK {
		t.Fatalf("apply: exit = %d (stderr: %s)", code, stderr)
	}
	if _, stderr, code := run(t, "rtk", "--root", root, "--check"); code != ExitOK {
		t.Errorf("exit = %d, want %d once the block is in place (stderr: %s)", code, ExitOK, stderr)
	}
}

// A repo worked on from Codex and opencode has two manifests and one AGENTS.md.
// Splicing per harness rather than per file would append the block twice.
func TestRTKWritesOneBlockForASharedEntryFile(t *testing.T) {
	root := t.TempDir()
	for _, h := range []string{"--codex", "--opencode"} {
		if _, stderr, code := run(t, "init", h, "--root", root); code != ExitOK {
			t.Fatalf("init %s: exit = %d (stderr: %s)", h, code, stderr)
		}
	}
	stdout, stderr, code := run(t, "rtk", "--root", root, "--no-install", "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	var report rtkReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if len(report.Files) != 1 {
		t.Errorf("files = %+v, want the shared %s reported once", report.Files, paths.Codex.EntryFile)
	}
	got := readEntry(t, root, paths.Codex.EntryFile)
	if n := strings.Count(got, "<!-- rtk-instructions"); n != 1 {
		t.Errorf("%s carries %d blocks, want 1", paths.Codex.EntryFile, n)
	}
}

// stdout carries the JSON document and nothing else, even on the run that warns
// about a missing binary.
func TestRTKJSONKeepsDiagnosticsOffStdout(t *testing.T) {
	root := initWorkspace(t)
	stdout, _, code := run(t, "rtk", "--root", root, "--no-install", "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	var report rtkReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if report.Install == "" {
		t.Error("the report does not say what happened to the binary")
	}
}

// Outside a workspace there is no entry file to splice into, and the walk would
// otherwise fall back to whatever directory the user happened to be in.
func TestRTKRequiresAWorkspace(t *testing.T) {
	_, stderr, code := run(t, "rtk", "--root", t.TempDir(), "--no-install")
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "not an scc workspace") {
		t.Errorf("stderr = %q, want it to say the directory is not a workspace", stderr)
	}
}

// A workspace whose entry file was deleted has nowhere to put the block. Saying so
// beats exiting 0 having written nothing.
func TestRTKReportsAMissingEntryFile(t *testing.T) {
	root := initWorkspace(t)
	if err := os.Remove(filepath.Join(root, paths.Claude.EntryFile)); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	_, stderr, code := run(t, "rtk", "--root", root, "--no-install")
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, paths.Claude.EntryFile) {
		t.Errorf("stderr does not name the missing file: %q", stderr)
	}
	if _, _, code := run(t, "rtk", "--root", root, "--check"); code != ExitFindings {
		t.Errorf("--check exit = %d, want %d", code, ExitFindings)
	}
}

func TestRTKRejectsPositionals(t *testing.T) {
	if _, _, code := run(t, "rtk", "install", "--root", t.TempDir()); code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
}

// init --rtk is the one-command setup: scaffold, then wire RTK in. Opt-in, because
// the block tells the agent to prefix every command with a binary the machine may
// not have.
func TestInitWithRTKSplicesTheBlock(t *testing.T) {
	stubRTK(t)
	root := t.TempDir()
	if _, stderr, code := run(t, "init", "--claude", "--rtk", "--root", root); code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if got := readEntry(t, root, paths.Claude.EntryFile); !strings.Contains(got, "<!-- rtk-instructions") {
		t.Errorf("init --rtk left no block in %s", paths.Claude.EntryFile)
	}
}

// Without the flag, init writes exactly what it wrote before RTK existed.
func TestInitWithoutRTKLeavesTheEntryFileAlone(t *testing.T) {
	root := initWorkspace(t)
	if got := readEntry(t, root, paths.Claude.EntryFile); strings.Contains(got, "rtk-instructions") {
		t.Error("init wired in RTK without being asked")
	}
}

// The scaffold result keeps its shape so anything already parsing init --json is
// unaffected, and "rtk" appears only when the flag was passed.
func TestInitJSONCarriesTheRTKReportOnlyWithTheFlag(t *testing.T) {
	stubRTK(t)
	var withFlag struct {
		Root string     `json:"root"`
		RTK  *rtkReport `json:"rtk"`
	}
	stdout, stderr, code := run(t, "init", "--claude", "--rtk", "--json", "--root", t.TempDir())
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &withFlag); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if withFlag.Root == "" {
		t.Error("the scaffold result lost its own fields")
	}
	if withFlag.RTK == nil || len(withFlag.RTK.Files) != 1 {
		t.Errorf("rtk report = %+v, want one file", withFlag.RTK)
	}

	stdout, stderr, code = run(t, "init", "--claude", "--json", "--root", t.TempDir())
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if strings.Contains(stdout, `"rtk"`) {
		t.Errorf("init --json carries an rtk key without the flag: %q", stdout)
	}
}
