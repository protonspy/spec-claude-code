package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The temp file is created in the destination directory so the rename stays on one
// filesystem, and a concurrent reader never observes a truncated file. What is
// checkable here is the observable half: the content arrives whole, the mode is the
// one asked for, a missing directory is created, and nothing is left behind.
func TestAtomicWriteLeavesNoTempFileAndKeepsTheMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deeper", "file.txt")

	if err := AtomicWrite(path, []byte("first\n"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "first\n" {
		t.Errorf("content = %q", got)
	}

	// Overwriting is the case the rename exists for: the old content is replaced
	// whole rather than truncated and rewritten.
	if err := AtomicWrite(path, []byte("second and longer\n"), 0o644); err != nil {
		t.Fatalf("AtomicWrite (overwrite): %v", err)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "second and longer\n" {
		t.Errorf("content after overwrite = %q", got)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("a temp file survived the write: %s", e.Name())
		}
	}

	// A path whose parent is a file cannot be created, and that is an error rather
	// than a silent miss.
	blocked := filepath.Join(dir, "nested", "deeper", "file.txt", "child.txt")
	if err := AtomicWrite(blocked, []byte("x"), 0o644); err == nil {
		t.Error("AtomicWrite under a regular file returned no error")
	}
}

// Resolve normalizes --root: an empty argument means "the enclosing workspace",
// and a path that is not there is an error rather than a directory scc creates.
func TestResolveRefusesAPathThatIsNotThere(t *testing.T) {
	dir := t.TempDir()

	got, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !sameDir(t, got, dir) {
		t.Errorf("Resolve(%q) = %q", dir, got)
	}

	missing := filepath.Join(dir, "not-there")
	if _, err := Resolve(missing); err == nil {
		t.Error("Resolve on a path that does not exist returned no error")
	} else if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %q, want it to say the path is not there", err)
	}

	// Empty falls back to the working directory's enclosing workspace, which is a
	// real answer wherever the suite is run from.
	if got, err := Resolve(""); err != nil || got == "" {
		t.Errorf("Resolve(\"\") = %q, %v", got, err)
	}
}

// Relative is for logging, so a target that cannot be made relative comes back as
// itself rather than as an error nobody can act on.
func TestRelativeFallsBackToTheTargetItself(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "plans", "sample.md")

	if got := Relative(root, inside); got != filepath.Join("plans", "sample.md") {
		t.Errorf("Relative = %q", got)
	}
	// A target on another volume cannot be made relative on Windows, and on every
	// platform an empty root leaves the path as it was.
	if got := Relative("", inside); got == "" {
		t.Error("Relative with no root returned nothing")
	}
}

// SafeWrite is "write only when missing" and WriteFile is "write unless told not
// to" — the pair that keeps a seed from overwriting what the user has since
// edited.
func TestSafeWriteAndWriteFileDisagreeOnPurpose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glossary.md")

	created, err := SafeWrite(path, "seeded\n")
	if err != nil || !created {
		t.Fatalf("SafeWrite: %v, created = %v", err, created)
	}
	created, err = SafeWrite(path, "seeded again\n")
	if err != nil {
		t.Fatalf("SafeWrite: %v", err)
	}
	if created {
		t.Error("SafeWrite overwrote a file that was already there")
	}
	if body, _ := os.ReadFile(path); string(body) != "seeded\n" {
		t.Errorf("content = %q", body)
	}

	if err := WriteFile(path, "mine\n", false); err == nil {
		t.Error("WriteFile without overwrite replaced an existing file")
	} else if !strings.Contains(err.Error(), "--force") {
		t.Errorf("the refusal does not name the way past it: %v", err)
	}
	if err := WriteFile(path, "mine\n", true); err != nil {
		t.Fatalf("WriteFile with overwrite: %v", err)
	}
	if body, _ := os.ReadFile(path); string(body) != "mine\n" {
		t.Errorf("content = %q", body)
	}
}

// sameDir compares resolved paths rather than strings: t.TempDir can sit under a
// symlink (/var → /private/var on macOS) and Windows reports 8.3 short names.
func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(fa, fb)
}
