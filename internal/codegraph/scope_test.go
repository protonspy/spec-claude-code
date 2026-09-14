package codegraph

import (
	"os"
	"path/filepath"
	"testing"
)

// tree makes the directories a scope test needs and returns the workspace root.
func tree(t *testing.T, dirs ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", d, err)
		}
	}
	return root
}

func rels(roots []Root) []string {
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		out = append(out, r.Rel)
	}
	return out
}

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// No scope is one graph at the root — what every workspace had before scoping
// existed, and what most should keep.
func TestNoScopeIsTheWholeWorkspace(t *testing.T) {
	root := tree(t, "backend/src")
	roots, missing := Roots(root, nil)
	if len(roots) != 1 || roots[0].Rel != "." || len(missing) != 0 {
		t.Fatalf("roots = %+v, missing = %v, want one root at .", roots, missing)
	}
	if Scoped(roots) {
		t.Error("an empty scope reports as scoped")
	}
	if roots[0].Dir != root {
		t.Errorf("dir = %q, want the workspace root %q", roots[0].Dir, root)
	}
}

// What people write, and what it has to mean. A trailing /* is how "everything
// under here" gets typed; handing that to a glob would mean one graph per child,
// which is nobody's intent and an expensive way to discover that.
func TestScopeNormalizesAndExpands(t *testing.T) {
	root := tree(t, "backend/src", "frontend/src", "packages/a/src", "packages/b/src", "vendor")
	cases := []struct {
		name  string
		scope []string
		want  []string
	}{
		{"plain directories", []string{"backend/src", "frontend/src"}, []string{"backend/src", "frontend/src"}},
		{"trailing star", []string{"backend/src/*"}, []string{"backend/src"}},
		{"trailing globstar", []string{"backend/src/**"}, []string{"backend/src"}},
		{"trailing slash", []string{"backend/src/"}, []string{"backend/src"}},
		{"interior glob expands", []string{"packages/*/src"}, []string{"packages/a/src", "packages/b/src"}},
		{"backslashes are normalized", []string{`backend\src`}, []string{"backend/src"}},
		// Recorded order is kept: somebody chose it, and a report reads better in
		// the order they wrote.
		{"order is the recorded one", []string{"frontend/src", "backend/src"}, []string{"frontend/src", "backend/src"}},
		{"duplicates collapse", []string{"backend/src", "backend/src/*"}, []string{"backend/src"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			roots, missing := Roots(root, c.scope)
			if len(missing) != 0 {
				t.Fatalf("missing = %v, want none", missing)
			}
			if got := rels(roots); !same(got, c.want) {
				t.Errorf("roots = %v, want %v", got, c.want)
			}
			if !Scoped(roots) {
				t.Error("a real scope reports as unscoped")
			}
		})
	}
}

// A pattern that matches nothing is named and skipped, not fatal. A scope is
// committed and branches differ, so a directory this checkout does not have is
// worth one line and is no reason to refuse to index the ones it does.
func TestScopeReportsWhatItCannotFind(t *testing.T) {
	root := tree(t, "backend/src")
	roots, missing := Roots(root, []string{"backend/src", "frontend/src"})
	if got := rels(roots); !same(got, []string{"backend/src"}) {
		t.Errorf("roots = %v, want the one that exists", got)
	}
	if !same(missing, []string{"frontend/src"}) {
		t.Errorf("missing = %v, want the one that does not", missing)
	}
}

// Every pattern missing is a different answer: no roots at all, so the caller
// stops. Falling back to the whole workspace would be the opposite of what the
// scope asked for — indexing everything because a path was misspelled is an
// expensive way to learn about the typo.
func TestScopeThatMatchesNothingYieldsNoRoots(t *testing.T) {
	root := tree(t, "backend/src")
	roots, missing := Roots(root, []string{"nope", "also/nope"})
	if len(roots) != 0 {
		t.Errorf("roots = %+v, want none rather than a fallback to the whole workspace", roots)
	}
	if len(missing) != 2 {
		t.Errorf("missing = %v, want both patterns named", missing)
	}
}

// A file is not a tree. A glob that catches one would otherwise hand CodeGraph a
// path it cannot index and turn one typo into an error per file.
func TestScopeKeepsOnlyDirectories(t *testing.T) {
	root := tree(t, "pkg-a", "pkg-b")
	if err := os.WriteFile(filepath.Join(root, "pkg-notes.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	roots, _ := Roots(root, []string{"pkg-*"})
	if got := rels(roots); !same(got, []string{"pkg-a", "pkg-b"}) {
		t.Errorf("roots = %v, want the two directories and not the file", got)
	}
}

// `backend/*` is "everything under backend", which is the directory itself — one
// graph. Expanding it as a glob would mean one graph per child of backend, which
// is not what anybody writing that means.
func TestTrailingStarIsNotChildExpansion(t *testing.T) {
	root := tree(t, "backend/src", "backend/cmd")
	roots, _ := Roots(root, []string{"backend/*"})
	if got := rels(roots); !same(got, []string{"backend"}) {
		t.Errorf("roots = %v, want backend itself rather than one graph per child", got)
	}
}

// A manifest arrives with a clone, so `..` in a recorded path is the same hostile
// input the rule about SafeName exists for — here it would mean indexing, and
// writing a .codegraph into, a directory outside the repository.
func TestScopeRefusesPathsThatEscape(t *testing.T) {
	root := tree(t, "backend/src")
	for _, bad := range []string{"../elsewhere", "backend/../../elsewhere", "/etc", "."} {
		roots, _ := Roots(root, []string{bad})
		for _, r := range roots {
			if r.Rel != "." || len(roots) != 1 {
				t.Errorf("%q resolved to %+v, want nothing", bad, roots)
			}
		}
		if len(roots) != 0 {
			t.Errorf("%q resolved to %+v, want no root", bad, roots)
		}
	}
}

// A scope means the same thing on every machine, which is why the backslash is
// handled here rather than by filepath.ToSlash.
//
// ToSlash is the host's conversion and a no-op on Linux, so a scope recorded as
// `backend\src` on Windows would name a directory there and nothing anywhere
// else — in a file that is committed and read by the whole team. This test is
// the guard, and it only fails on the platforms where the old code was wrong.
func TestScopeIsSeparatorIndependent(t *testing.T) {
	root := tree(t, "backend/src")
	for _, pattern := range []string{`backend\src`, "backend/src", `backend\src\`, `backend\src\*`} {
		roots, missing := Roots(root, []string{pattern})
		if len(missing) != 0 || len(roots) != 1 || roots[0].Rel != "backend/src" {
			t.Errorf("%q resolved to %v (missing %v), want backend/src on every platform",
				pattern, rels(roots), missing)
		}
	}
}
