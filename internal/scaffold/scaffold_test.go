package scaffold

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/assets"
	"github.com/protonspy/spec-claude-code/internal/manifest"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

func apply(t *testing.T, root string, force bool) *Result {
	t.Helper()
	return applyTo(t, root, paths.Claude, force)
}

func applyTo(t *testing.T, root string, h paths.Harness, force bool) *Result {
	t.Helper()
	res, err := Apply(root, Options{SCCVersion: "v0.0.0-test", Harness: h, Force: force})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return res
}

// renderRel returns the bytes the template set produces for a destination, which
// is what "still pristine" means: the manifest hashes the render, not the
// template file.
func renderRel(t *testing.T, rel string) string {
	t.Helper()
	return renderRelFor(t, paths.Claude, rel)
}

// renderRelFor is renderRel for a harness other than Claude Code.
func renderRelFor(t *testing.T, h paths.Harness, rel string) string {
	t.Helper()
	for _, f := range assets.Workspace(h) {
		if f.Rel == rel {
			out, err := assets.Render(h, f)
			if err != nil {
				t.Fatalf("Render %s: %v", rel, err)
			}
			return out
		}
	}
	t.Fatalf("no managed file at %s", rel)
	return ""
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(b)
}

func TestApplyWritesTheWholeSet(t *testing.T) {
	root := t.TempDir()
	res := apply(t, root, false)

	if res.AlreadyPresent {
		t.Error("AlreadyPresent = true for a fresh directory")
	}
	for _, f := range assets.Workspace(paths.Claude) {
		want, err := assets.Render(paths.Claude, f)
		if err != nil {
			t.Fatalf("Content: %v", err)
		}
		if got := read(t, root, f.Rel); got != want {
			t.Errorf("%s: content does not match the template", f.Rel)
		}
	}
	for _, s := range assets.Seeds() {
		want, err := assets.Content(s.Name)
		if err != nil {
			t.Fatalf("Content: %v", err)
		}
		if got := read(t, root, s.Rel); got != want {
			t.Errorf("%s: content does not match the seed", s.Rel)
		}
	}
	if want := len(assets.Workspace(paths.Claude)) + len(assets.Seeds()); res.Created != want {
		t.Errorf("created = %d, want %d", res.Created, want)
	}
	for _, dir := range assets.Dirs(paths.Claude) {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil || !info.IsDir() {
			t.Errorf("directory %s was not created: %v", dir, err)
		}
	}
}

// The marker is what makes every other command able to find the root, from any
// depth, which is the reason it is a file rather than the .claude/ directory.
func TestApplyMakesTheDirectoryAWorkspace(t *testing.T) {
	root := t.TempDir()
	apply(t, root, false)

	if !workspace.IsWorkspace(root) {
		t.Fatal("root is not a workspace after Apply")
	}
	deep := filepath.Join(root, paths.SpecsSeg, "user-auth", "nested")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// t.TempDir() can sit under a symlink (/var -> /private/var on macOS) and
	// Windows reports 8.3 short names, so compare identities rather than strings.
	found, err := os.Stat(workspace.Find(deep))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	want, err := os.Stat(root)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !os.SameFile(found, want) {
		t.Errorf("Find(%q) did not resolve back to the root", deep)
	}
}

func TestApplyRecordsEveryManagedFile(t *testing.T) {
	root := t.TempDir()
	apply(t, root, false)

	m, found, err := manifest.Load(root, paths.Claude)
	if err != nil || !found {
		t.Fatalf("Load = (%v, %v), want found", found, err)
	}
	if m.SCC != "v0.0.0-test" {
		t.Errorf("scc stamp = %q, want the version passed in", m.SCC)
	}
	if len(m.Files) != len(assets.Workspace(paths.Claude)) {
		t.Fatalf("entries = %d, want %d", len(m.Files), len(assets.Workspace(paths.Claude)))
	}
	for _, f := range assets.Workspace(paths.Claude) {
		e, ok := m.Get(f.Rel)
		if !ok {
			t.Errorf("%s is not in the manifest", f.Rel)
			continue
		}
		if e.Version != assets.Version {
			t.Errorf("%s: version = %q, want %q", f.Rel, e.Version, assets.Version)
		}
		status, err := m.Status(root, e)
		if err != nil || status != manifest.Pristine {
			t.Errorf("%s: status = (%v, %v) right after writing it, want pristine", f.Rel, status, err)
		}
	}
}

// Spec and plan artifacts are authored from birth and owned immediately, so they
// are deliberately absent from the manifest — which is what keeps an upgrade from
// ever touching a requirement.
func TestManifestRecordsOnlySccManagedFiles(t *testing.T) {
	root := t.TempDir()
	apply(t, root, false)
	m, _, err := manifest.Load(root, paths.Claude)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, e := range m.Files {
		if len(e.Path) >= 6 && (e.Path[:6] == "specs/" || e.Path[:6] == "plans/") {
			t.Errorf("manifest records a user artifact: %s", e.Path)
		}
	}
}

// A second run writes nothing — not one file, and not the manifest. Re-running
// init has to be free, or the "just run init again" upgrade story costs a diff.
func TestApplyIsIdempotent(t *testing.T) {
	root := t.TempDir()
	apply(t, root, false)
	before, err := os.ReadFile(paths.Claude.Manifest(root))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	res := apply(t, root, false)
	if !res.AlreadyPresent {
		t.Error("AlreadyPresent = false on a second run")
	}
	if res.Created != 0 || res.Replaced != 0 {
		t.Errorf("second run wrote files: created=%d replaced=%d", res.Created, res.Replaced)
	}
	if want := len(assets.Workspace(paths.Claude)) + len(assets.Seeds()); res.Skipped != want {
		t.Errorf("skipped = %d, want %d", res.Skipped, want)
	}
	if res.ManifestWritten {
		t.Error("second run rewrote the manifest")
	}
	after, err := os.ReadFile(paths.Claude.Manifest(root))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(before) != string(after) {
		t.Error("the manifest changed across two identical runs")
	}
}

// Never author what the user owns: an edited file survives init, and it keeps the
// version the manifest recorded for it — that version is the base revision a later
// three-way merge starts from.
func TestApplyNeverOverwritesWithoutForce(t *testing.T) {
	root := t.TempDir()
	apply(t, root, false)

	target := ".claude/rules/methodology.md"
	mine := "# mine\n\nI rewrote this rule.\n"
	if err := workspace.AtomicWrite(filepath.Join(root, filepath.FromSlash(target)), []byte(mine), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	res := apply(t, root, false)
	if got := read(t, root, target); got != mine {
		t.Errorf("init overwrote an edited file:\n%s", got)
	}
	if len(res.Clobbered) != 0 {
		t.Errorf("clobbered = %v on a run without --force", res.Clobbered)
	}
	m, _, err := manifest.Load(root, paths.Claude)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	e, ok := m.Get(target)
	if !ok {
		t.Fatal("the edited file left the manifest")
	}
	status, err := m.Status(root, e)
	if err != nil || status != manifest.Edited {
		t.Errorf("status = (%v, %v), want edited", status, err)
	}
}

// --force overwrites, and has to name what it destroyed. A silent clobber is the
// one outcome that would make the tool untrustworthy.
func TestForceNamesEveryEditedFileItClobbers(t *testing.T) {
	root := t.TempDir()
	apply(t, root, false)

	edited := ".claude/rules/routing.md"
	if err := workspace.AtomicWrite(filepath.Join(root, filepath.FromSlash(edited)), []byte("# mine\n"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	res := apply(t, root, true)
	if len(res.Clobbered) != 1 || res.Clobbered[0] != edited {
		t.Errorf("clobbered = %v, want exactly [%s]", res.Clobbered, edited)
	}
	want := renderRel(t, ".claude/rules/routing.md")
	if got := read(t, root, edited); got != want {
		t.Error("--force did not restore the template")
	}
	// Every other file was already identical, so --force had nothing to write to
	// them: an overwrite that changes nothing is not an overwrite.
	if res.Replaced != 1 {
		t.Errorf("replaced = %d, want 1", res.Replaced)
	}
}

// A pre-existing file scc has no record of — someone else's CLAUDE.md, adopting scc
// into a live repo — is reported as clobbered too. scc cannot tell it from an edit,
// and guessing in the direction of "it was probably fine to destroy" is the wrong
// default.
func TestForceReportsAdoptedFilesAsClobbered(t *testing.T) {
	root := t.TempDir()
	if err := workspace.AtomicWrite(paths.Claude.Entry(root), []byte("# their own CLAUDE.md\n"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	res := apply(t, root, true)
	if len(res.Clobbered) != 1 || res.Clobbered[0] != paths.Claude.EntryFile {
		t.Errorf("clobbered = %v, want [%s]", res.Clobbered, paths.Claude.EntryFile)
	}
}

// Adopting a directory that already has content must not lose it, and the file that
// was there keeps being the user's.
func TestApplyAdoptsAnExistingFileWithoutLosingIt(t *testing.T) {
	root := t.TempDir()
	theirs := "# their own CLAUDE.md\n"
	if err := workspace.AtomicWrite(paths.Claude.Entry(root), []byte(theirs), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	apply(t, root, false)

	if got := read(t, root, paths.Claude.EntryFile); got != theirs {
		t.Errorf("adoption overwrote their file:\n%s", got)
	}
	m, _, err := manifest.Load(root, paths.Claude)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	e, ok := m.Get(paths.Claude.EntryFile)
	if !ok {
		t.Fatal("the adopted file is not in the manifest")
	}
	// Recorded at the current template version: today's template is the only honest
	// merge base when scc has no record of where the file came from.
	if e.Version != assets.Version {
		t.Errorf("version = %q, want %q", e.Version, assets.Version)
	}
	status, _ := m.Status(root, e)
	if status != manifest.Edited {
		t.Errorf("status = %v, want edited", status)
	}
}

// A CRLF checkout of an untouched file is untouched. Without normalization every
// managed file on a Windows working tree would read as edited.
func TestCRLFOnDiskIsNotAnEdit(t *testing.T) {
	root := t.TempDir()
	apply(t, root, false)

	rel := ".claude/rules/tasks.md"
	pristine := read(t, root, rel)
	crlf := ""
	for _, line := range splitLines(pristine) {
		crlf += line + "\r\n"
	}
	if err := workspace.AtomicWrite(filepath.Join(root, filepath.FromSlash(rel)), []byte(crlf), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	res := apply(t, root, true)
	if len(res.Clobbered) != 0 {
		t.Errorf("a CRLF checkout was reported as edited: %v", res.Clobbered)
	}
	if got := read(t, root, rel); got != crlf {
		t.Error("--force rewrote a file whose content was equivalent")
	}
}

// A manifest entry for a template this version no longer ships is dropped rather
// than left to send a later upgrade looking for a template that does not exist.
func TestStaleManifestEntriesAreDropped(t *testing.T) {
	root := t.TempDir()
	apply(t, root, false)

	m, _, err := manifest.Load(root, paths.Claude)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	m.Set(".claude/rules/removed-in-a-later-version.md", "deadbeef", "0")
	if err := manifest.Save(root, paths.Claude, m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	res := apply(t, root, false)
	var unmanaged []string
	for _, c := range res.Changes {
		if c.Action == Unmanaged {
			unmanaged = append(unmanaged, c.Path)
		}
	}
	if len(unmanaged) != 1 || unmanaged[0] != ".claude/rules/removed-in-a-later-version.md" {
		t.Errorf("unmanaged = %v, want the stale entry", unmanaged)
	}
	after, _, err := manifest.Load(root, paths.Claude)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := after.Get(".claude/rules/removed-in-a-later-version.md"); ok {
		t.Error("the stale entry survived")
	}
}

// A missing file is restored: nothing is lost by deleting a managed file, which is
// what makes "delete it and re-run init" a safe instruction.
func TestApplyRestoresADeletedFile(t *testing.T) {
	root := t.TempDir()
	apply(t, root, false)
	rel := ".claude/agents/code-review.md"
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	res := apply(t, root, false)
	if res.Created != 1 {
		t.Errorf("created = %d, want 1", res.Created)
	}
	want := renderRel(t, ".claude/agents/code-review.md")
	if got := read(t, root, rel); got != want {
		t.Error("the deleted file was not restored from the template")
	}
}

// Changes are reported in one deterministic order so a --json consumer and a human
// reading two runs see the same sequence.
func TestChangesAreSortedByPath(t *testing.T) {
	root := t.TempDir()
	res := apply(t, root, false)
	for i := 1; i < len(res.Changes); i++ {
		if res.Changes[i-1].Path >= res.Changes[i].Path {
			t.Fatalf("changes not sorted: %q before %q", res.Changes[i-1].Path, res.Changes[i].Path)
		}
	}
}

// Every harness gets a complete, self-contained tree: its own manifest, its own
// rules, and an entry file its own tool loads.
func TestApplyScaffoldsEveryHarness(t *testing.T) {
	for _, h := range paths.Harnesses() {
		root := t.TempDir()
		res := applyTo(t, root, h, false)
		if res.Harness != h.ID {
			t.Errorf("%s: result harness = %q", h.ID, res.Harness)
		}
		if !workspace.IsWorkspace(root) {
			t.Errorf("%s: no marker written", h.ID)
		}
		m, found, err := manifest.Load(root, h)
		if err != nil || !found {
			t.Fatalf("%s: Load = (%v, %v)", h.ID, found, err)
		}
		if m.Harness != h.ID {
			t.Errorf("%s: manifest records harness %q", h.ID, m.Harness)
		}
		if len(m.Files) != len(assets.Workspace(h)) {
			t.Errorf("%s: %d entries, want %d", h.ID, len(m.Files), len(assets.Workspace(h)))
		}
		read(t, root, h.EntryFile)
		read(t, root, h.Dir+"/"+h.RulesSeg+"/routing.md")
	}
}

// Running init twice for two harnesses is how a repo worked on from two tools
// gets both trees. The second run must not touch the first's files — and Codex
// and opencode share one entry file, which is exactly where an overwrite would
// destroy the user's own prose.
func TestASecondHarnessLeavesTheFirstAlone(t *testing.T) {
	root := t.TempDir()
	applyTo(t, root, paths.Codex, false)

	mine := read(t, root, paths.Codex.EntryFile) + "\n## my own section\n"
	if err := workspace.AtomicWrite(paths.Codex.Entry(root), []byte(mine), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	res := applyTo(t, root, paths.OpenCode, false)
	if got := read(t, root, paths.OpenCode.EntryFile); got != mine {
		t.Error("the second harness overwrote the shared entry file")
	}
	for _, c := range res.Changes {
		if c.Path == paths.OpenCode.EntryFile && c.Action != Skipped {
			t.Errorf("entry file action = %q, want %q", c.Action, Skipped)
		}
	}
	// Both trees are present and each is findable on its own manifest.
	if got := workspace.Harnesses(root); len(got) != 2 {
		t.Errorf("initialized harnesses = %v, want two", got)
	}
	read(t, root, ".codex/agents/code-review.toml")
	read(t, root, ".opencode/agent/code-review.md")
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
