package workspace

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/paths"
)

func TestKebabCheck(t *testing.T) {
	ok := []string{"a", "spec", "user-auth", "oauth2-login", "a1-b2"}
	bad := []string{"", "A", "User-Auth", "user_auth", "-user", "user-", "user--auth", "1user", "user auth", "café"}
	for _, n := range ok {
		if err := KebabCheck(n, "spec"); err != nil {
			t.Errorf("KebabCheck(%q) = %v, want nil", n, err)
		}
	}
	for _, n := range bad {
		if err := KebabCheck(n, "spec"); err == nil {
			t.Errorf("KebabCheck(%q) = nil, want an error", n)
		}
	}
}

// SafeName is the guard standing between a CLI arg and filepath.Join. These are
// the inputs that turn `delete <name>` into deleting the workspace.
func TestSafeNameRejectsTraversal(t *testing.T) {
	bad := []string{"", ".", "..", "../x", "a/b", `a\b`, "/abs", "./x"}
	for _, n := range bad {
		if err := SafeName(n, "spec"); err == nil {
			t.Errorf("SafeName(%q) = nil, want an error", n)
		}
	}
	for _, n := range []string{"user-auth", "spec.md", "a"} {
		if err := SafeName(n, "spec"); err != nil {
			t.Errorf("SafeName(%q) = %v, want nil", n, err)
		}
	}
}

// filepath.IsLocal only rejects reserved device names on Windows, so assert the
// platform-specific half of the contract where it actually applies.
func TestSafeNameRejectsWindowsDeviceNames(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("reserved device names are a Windows-only concern")
	}
	for _, n := range []string{"CON", "nul", "COM1"} {
		if err := SafeName(n, "spec"); err == nil {
			t.Errorf("SafeName(%q) = nil, want an error", n)
		}
	}
}

func TestAtomicWriteCreatesParentsAndLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "nested", "deep", "file.json")
	if err := AtomicWrite(target, []byte("payload"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("content = %q, want %q", got, "payload")
	}
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("temp file left behind: %v", entries)
	}
}

func TestAtomicWriteOverwrites(t *testing.T) {
	target := filepath.Join(t.TempDir(), "file")
	for _, want := range []string{"first", "second"} {
		if err := AtomicWrite(target, []byte(want), 0o644); err != nil {
			t.Fatalf("AtomicWrite(%q): %v", want, err)
		}
		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(got) != want {
			t.Errorf("content = %q, want %q", got, want)
		}
	}
}

// markWorkspace creates the .claude/scc-manifest.json marker under root.
func markWorkspace(t *testing.T, root string) {
	t.Helper()
	if err := AtomicWrite(paths.Claude.Manifest(root), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("writing marker: %v", err)
	}
}

func TestFindWalksUpToMarker(t *testing.T) {
	root := t.TempDir()
	markWorkspace(t, root)
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// t.TempDir() can sit under a symlinked path (/var -> /private/var on macOS),
	// which Find resolves differently than the raw name; compare identities.
	assertSameDir(t, Find(deep), root)
	if !IsWorkspace(root) {
		t.Errorf("IsWorkspace(%q) = false, want true", root)
	}
}

// A bare .claude/ must NOT be treated as a marker, for two reasons: it is Claude
// Code's global config at $HOME, so honoring it would resolve the root to the
// user's home for any command run outside a workspace; and it exists in every
// repo that merely uses Claude Code, where scc was never initialized.
func TestFindIgnoresClaudeDirWithoutMarker(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(paths.Claude.Config(root), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	deep := filepath.Join(root, "a")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	assertSameDir(t, Find(deep), deep)
	if IsWorkspace(root) {
		t.Errorf("IsWorkspace(%q) = true for a .claude-only dir, want false", root)
	}
}

// The marker must be a regular file. A directory carrying the manifest's name is
// not a workspace, and must not be silently accepted as one.
func TestFindRejectsMarkerThatIsADirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(paths.Claude.Manifest(root), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	assertSameDir(t, Find(root), root) // falls back to start, not "found"
	if IsWorkspace(root) {
		t.Error("IsWorkspace = true for a directory named like the manifest, want false")
	}
}

// A symlink is not a marker. The marker's whole claim is "scc wrote this here",
// and a link pointing at somebody else's manifest — the user's global one, say —
// satisfies Stat while making that claim false.
func TestFindRejectsASymlinkedMarker(t *testing.T) {
	elsewhere := filepath.Join(t.TempDir(), "real-manifest.json")
	if err := AtomicWrite(elsewhere, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	root := t.TempDir()
	marker := paths.Claude.Manifest(root)
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(elsewhere, marker); err != nil {
		// Unprivileged Windows without developer mode cannot make one; the
		// behavior under test is unreachable there rather than wrong.
		t.Skipf("symlinks unavailable: %v", err)
	}
	if IsWorkspace(root) {
		t.Error("IsWorkspace = true for a symlinked marker, want false")
	}
	if len(Harnesses(root)) != 0 {
		t.Error("a symlinked marker was reported as an initialized harness")
	}
}

// The nearest marker wins, so a workspace nested inside another resolves to
// itself rather than to its parent.
func TestFindStopsAtNearestMarker(t *testing.T) {
	outer := t.TempDir()
	markWorkspace(t, outer)
	inner := filepath.Join(outer, "sub", "project")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	markWorkspace(t, inner)
	assertSameDir(t, Find(filepath.Join(inner, "deep")), inner)
}

func TestFindWithoutMarkerReturnsStart(t *testing.T) {
	dir := t.TempDir()
	assertSameDir(t, Find(dir), dir)
}

// A workspace is a workspace whichever harness scaffolded it: specs/, plans/, and
// docs/ sit at that root either way, so every command has to resolve it.
func TestFindAcceptsAnyHarnessMarker(t *testing.T) {
	for _, h := range paths.Harnesses() {
		root := t.TempDir()
		if err := AtomicWrite(h.Manifest(root), []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("%s: writing marker: %v", h.ID, err)
		}
		deep := filepath.Join(root, "a", "b")
		if err := os.MkdirAll(deep, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		assertSameDir(t, Find(deep), root)
		got := Harnesses(root)
		if len(got) != 1 || got[0].ID != h.ID {
			t.Errorf("Harnesses = %v, want just %s", got, h.ID)
		}
	}
}

// Running init twice for two harnesses leaves two managed trees in one repo, and
// both have to be visible: each is upgraded on its own manifest.
func TestHarnessesReportsEveryInitializedTree(t *testing.T) {
	root := t.TempDir()
	for _, h := range []paths.Harness{paths.Claude, paths.OpenCode} {
		if err := AtomicWrite(h.Manifest(root), []byte("{}\n"), 0o644); err != nil {
			t.Fatalf("%s: writing marker: %v", h.ID, err)
		}
	}
	got := Harnesses(root)
	if len(got) != 2 || got[0].ID != paths.Claude.ID || got[1].ID != paths.OpenCode.ID {
		t.Errorf("Harnesses = %v, want [claude opencode] in that order", got)
	}
}

func TestResolveRejectsMissingRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	if _, err := Resolve(missing); err == nil {
		t.Errorf("Resolve(%q) = nil error, want one", missing)
	}
}

func TestResolveAcceptsExistingRoot(t *testing.T) {
	dir := t.TempDir()
	got, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertSameDir(t, got, dir)
}

func TestSafeWriteDoesNotClobber(t *testing.T) {
	target := filepath.Join(t.TempDir(), "file")
	created, err := SafeWrite(target, "original")
	if err != nil || !created {
		t.Fatalf("SafeWrite = (%v, %v), want (true, nil)", created, err)
	}
	created, err = SafeWrite(target, "replacement")
	if err != nil {
		t.Fatalf("SafeWrite: %v", err)
	}
	if created {
		t.Error("SafeWrite reported a create over an existing file")
	}
	got, _ := os.ReadFile(target)
	if string(got) != "original" {
		t.Errorf("content = %q, want %q", got, "original")
	}
}

func TestWriteFileRequiresOverwrite(t *testing.T) {
	target := filepath.Join(t.TempDir(), "file")
	if err := WriteFile(target, "a", false); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := WriteFile(target, "b", false); err == nil {
		t.Error("WriteFile overwrote an existing file without overwrite=true")
	}
	if err := WriteFile(target, "b", true); err != nil {
		t.Fatalf("WriteFile(overwrite): %v", err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "b" {
		t.Errorf("content = %q, want %q", got, "b")
	}
}

// assertSameDir compares two paths by the identity the filesystem reports, so a
// symlinked temp dir or Windows 8.3 short name doesn't fail a correct result.
func assertSameDir(t *testing.T, got, want string) {
	t.Helper()
	gi, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat %q: %v", got, err)
	}
	wi, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat %q: %v", want, err)
	}
	if !os.SameFile(gi, wi) {
		t.Errorf("got %q, want %q", got, want)
	}
}
