package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/rtk"
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

// `headroom wrap --rtk` appends the same guidance to the same entry file behind
// its own marker pair, and neither marker is a substring of the other — so both
// tools' idempotency checks pass and the file ends up telling the agent the same
// thing twice. scc cannot address a block it does not own, so it does the one
// thing left: names it, says how to remove it, and touches nothing.
func TestRTKReportsHeadroomsCompetingBlock(t *testing.T) {
	root := initWorkspace(t)
	entry := filepath.Join(root, paths.Claude.EntryFile)
	foreign := "\n<!-- headroom:rtk-instructions -->\nPrefix every command with rtk.\n<!-- /headroom:rtk-instructions -->\n"
	before := readEntry(t, root, paths.Claude.EntryFile) + foreign
	if err := os.WriteFile(entry, []byte(before), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	stdout, stderr, code := run(t, "rtk", "--root", root, "--no-install", "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, ExitOK, stderr)
	}
	if !strings.Contains(stderr, "Headroom") || !strings.Contains(stderr, "headroom unwrap") {
		t.Errorf("stderr does not name the competing block and its fix: %q", stderr)
	}

	// Reported, never touched: that block is Headroom's to rewrite and remove.
	if !strings.Contains(readEntry(t, root, paths.Claude.EntryFile), foreign) {
		t.Error("scc modified Headroom's block")
	}

	var report rtkReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if len(report.Files) == 0 || report.Files[0].Foreign != "Headroom" {
		t.Errorf("files = %+v, want the foreign block named", report.Files)
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

// The block scc ships wins by default. `rtk init` writes the same v2 instruction
// in roughly five times the bytes, and the entry file is preloaded into every
// request of the session, so leaving the larger one in place because it got there
// first is a standing cost rather than deference.
func TestRTKReplacesAnExistingBlockWithItsOwn(t *testing.T) {
	root := initWorkspace(t)
	entry := filepath.Join(root, paths.Claude.EntryFile)
	base := readEntry(t, root, paths.Claude.EntryFile)
	fat := "\n<!-- rtk-instructions v2 -->\n## RTK\n" + strings.Repeat("verbose guidance line\n", 200) + "<!-- /rtk-instructions -->\n"
	if err := os.WriteFile(entry, []byte(base+fat), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	stdout, stderr, code := run(t, "rtk", "--root", root, "--no-install", "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	got := readEntry(t, root, paths.Claude.EntryFile)
	if strings.Contains(got, "verbose guidance line") {
		t.Error("the larger block survived")
	}
	// Only the region between the markers: everything above it is the user's.
	if !strings.HasPrefix(got, base) {
		t.Error("the document above the block was rewritten")
	}

	var report rtkReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	f := report.Files[0]
	if f.Action != "replaced" {
		t.Errorf("action = %q, want replaced", f.Action)
	}
	if f.WasBytes <= f.Bytes {
		t.Errorf("bytes = %d, was_bytes = %d, want the replacement to be the smaller one", f.Bytes, f.WasBytes)
	}
	// Same version on both sides, so there is nothing to warn about.
	if f.Was != "" {
		t.Errorf("was = %q, want it silent when the versions match", f.Was)
	}

	// Re-running changes nothing: the block is now byte-identical to scc's.
	if _, _, code := run(t, "rtk", "--root", root, "--no-install"); code != ExitOK {
		t.Fatalf("second pass exit = %d", code)
	}
	if again := readEntry(t, root, paths.Claude.EntryFile); again != got {
		t.Error("a second pass rewrote the file")
	}
}

// Preferring scc's block costs something exactly once: when the block replaced
// claimed a newer version. That is a downgrade, so it is said out loud rather
// than left to be inferred, and --keep is the standing answer.
func TestRTKNamesTheVersionItDowngradesAndKeepCanRefuse(t *testing.T) {
	root := initWorkspace(t)
	entry := filepath.Join(root, paths.Claude.EntryFile)
	base := readEntry(t, root, paths.Claude.EntryFile)
	theirs := base + "\n<!-- rtk-instructions v9 -->\n## RTK\nnewer text\n<!-- /rtk-instructions -->\n"
	if err := os.WriteFile(entry, []byte(theirs), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// --keep leaves it exactly as it was.
	if _, stderr, code := run(t, "rtk", "--root", root, "--no-install", "--keep"); code != ExitOK {
		t.Fatalf("--keep exit = %d (stderr: %s)", code, stderr)
	}
	if got := readEntry(t, root, paths.Claude.EntryFile); got != theirs {
		t.Errorf("--keep rewrote the block:\n%s", got)
	}

	// Without it, scc's block wins — and the run says which version it displaced.
	stdout, stderr, code := run(t, "rtk", "--root", root, "--no-install", "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if strings.Contains(readEntry(t, root, paths.Claude.EntryFile), "newer text") {
		t.Error("the v9 block survived without --keep")
	}
	if !strings.Contains(stderr, "v9") || !strings.Contains(stderr, "--keep") {
		t.Errorf("stderr does not name the downgrade and its escape: %q", stderr)
	}
	var report rtkReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if report.Files[0].Was != "v9" {
		t.Errorf("was = %q, want v9", report.Files[0].Was)
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
		if _, stderr, code := run(t, "init", "--no-rtk", h, "--root", root); code != ExitOK {
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

// init --rtk is the one-command setup: scaffold, then wire RTK in without putting
// the install question.
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

// With the binary already there, a bare init wires the block in: the install was
// the part that needed asking, and it has nothing to ask.
//
// This is what makes a workspace wired whichever way its first session starts. `scc
// launch` has always written the block, but an agent started by typing `claude`
// reads an entry file that never mentions the prefix it is supposed to be using.
func TestInitSplicesTheBlockWhenRTKIsAlreadyThere(t *testing.T) {
	stubRTK(t)
	root := t.TempDir()
	if _, stderr, code := run(t, "init", "--claude", "--root", root); code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if got := readEntry(t, root, paths.Claude.EntryFile); !strings.Contains(got, "<!-- rtk-instructions") {
		t.Errorf("init left no block in %s with rtk on PATH", paths.Claude.EntryFile)
	}
}

// Nothing is written when the binary is absent and nobody can be asked about
// building it. Guidance naming a command the machine cannot run is worse than no
// guidance: the agent tries the prefix, watches it fail, and discounts the rest of
// the file with it.
func TestInitWritesNoBlockWithoutTheBinary(t *testing.T) {
	isolatedPath(t)
	root := t.TempDir()
	stdout, stderr, code := run(t, "init", "--claude", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d, want a scaffolded workspace anyway (stderr: %s)", code, stderr)
	}
	if got := readEntry(t, root, paths.Claude.EntryFile); strings.Contains(got, "rtk-instructions") {
		t.Error("init wrote a block naming a binary that is not there")
	}
	// Said once, and it names the way to wire it in later: a step that was skipped
	// in silence is one nobody knows to take.
	if !strings.Contains(stdout+stderr, "rtk") {
		t.Errorf("init never said why RTK was skipped: %q", stdout+stderr)
	}
}

// --no-rtk is the explicit "scaffold and nothing else": no lookup, no block, no
// report — and it contradicts --rtk rather than quietly outranking it, because a
// flag that is accepted and ignored is how somebody spends a session believing they
// configured something.
func TestInitNoRTKSkipsItEntirely(t *testing.T) {
	stubRTK(t)
	root := t.TempDir()
	if _, stderr, code := run(t, "init", "--claude", "--no-rtk", "--root", root); code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if got := readEntry(t, root, paths.Claude.EntryFile); strings.Contains(got, "rtk-instructions") {
		t.Error("--no-rtk still wired RTK in")
	}
	if _, _, code := run(t, "init", "--claude", "--rtk", "--no-rtk", "--root", t.TempDir()); code != ExitError {
		t.Errorf("exit = %d, want %d for two flags that contradict each other", code, ExitError)
	}
}

// A re-run leaves a block somebody else put there exactly as it is. Replacing one
// is a real trade-off with a real cost, and `scc rtk` is where it is made
// deliberately — scaffolding is not the moment to make it as a side effect.
func TestInitKeepsAnRTKBlockThatIsAlreadyThere(t *testing.T) {
	stubRTK(t)
	root := t.TempDir()
	if _, stderr, code := run(t, "init", "--claude", "--no-rtk", "--root", root); code != ExitOK {
		t.Fatalf("init: exit = %d (stderr: %s)", code, stderr)
	}
	mine := rtk.Markers.Open + " v9 -->\nmine, not scc's\n" + rtk.Markers.Close + "\n"
	entry := filepath.Join(root, paths.Claude.EntryFile)
	raw, err := os.ReadFile(entry)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := os.WriteFile(entry, []byte(string(raw)+"\n"+mine), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, stderr, code := run(t, "init", "--claude", "--root", root); code != ExitOK {
		t.Fatalf("second init: exit = %d (stderr: %s)", code, stderr)
	}
	if got := readEntry(t, root, paths.Claude.EntryFile); !strings.Contains(got, "mine, not scc's") {
		t.Errorf("init replaced a block it did not write:\n%s", got)
	}
}

// The scaffold result keeps its shape so anything already parsing init --json is
// unaffected, and "rtk" appears on the runs that actually wired it in.
func TestInitJSONCarriesTheRTKReport(t *testing.T) {
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

	// And it is absent from the run that did not: the missing key is the answer to
	// "was this step taken", which a zero-valued report would not be.
	stdout, stderr, code = run(t, "init", "--claude", "--no-rtk", "--json", "--root", t.TempDir())
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if strings.Contains(stdout, `"rtk"`) {
		t.Errorf("init --json carries an rtk key under --no-rtk: %q", stdout)
	}
}

// --rtk named the step, so a machine that cannot take it says so in the exit code
// rather than scaffolding and moving on. Without the flag the same situation is a
// status line: the workspace is finished, and RTK is the part that is not wired.
func TestInitWithRTKFailsWhenItCannotBeBuilt(t *testing.T) {
	isolatedPath(t)
	_, stderr, code := run(t, "init", "--claude", "--rtk", "--root", t.TempDir())
	if code != ExitError {
		t.Fatalf("exit = %d, want %d with neither rtk nor cargo on PATH", code, ExitError)
	}
	if !strings.Contains(stderr, "cargo") {
		t.Errorf("stderr does not say what is missing: %q", stderr)
	}
}
