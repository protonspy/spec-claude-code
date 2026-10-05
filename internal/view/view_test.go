package view

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fixture lays out a workspace's readable tree: one page in each docs group, a
// plan with tasks, a spec, and source files the codewiki could cite. The manifest
// is not needed — the server is handed a root, it never looks for one.
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"docs/wiki/index.md":         "# Wiki\n\n- [[alpha]]\n",
		"docs/wiki/changelog.md":     "# Changelog\n",
		"docs/wiki/pages/alpha.md":   "# Alpha\n\nSee adr:0001-first.\n",
		"docs/adr/0001-first.md":     "---\nstatus: accepted\n---\n\n# First\n\n## Context\n\nWhy.\n",
		"docs/codewiki/packages.md":  "# Packages\n\n## Main\n\n[cmd/main.go:1-2]()\n",
		"docs/glossary.md":           "# Glossary\n",
		"docs/raw/.hidden/secret.md": "# Hidden\n",
		"plans/ship.md":              "---\nautonomy: auto\n---\n\n# Ship\n\n## Tasks\n\n- [x] 1.1 (Unit) One\n- [ ] 1.2 (Unit) Two\n",
		"specs/feat/requirements.md": "# Feat — requirements\n\n- **R1.1** The system shall work.\n",
		"specs/feat/design.md":       "# Feat — design\n",
		"specs/feat/tasks.md":        "# Feat — tasks\n\n- [x] 1.1 (Unit) Do — R1.1\n",
		"cmd/main.go":                "package main\r\n\r\nfunc main() {}\r\n",
		".env":                       "SECRET=1\n",
		"internal/.cache/x.go":       "package x\n",
		"bin/blob":                   "ab\x00cd",
		"notes.txt":                  "outside docs\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	r.Host = "127.0.0.1:7777"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
}

func TestTreeGroupsEveryPageByKind(t *testing.T) {
	var tree Tree
	decodeBody(t, get(t, Handler(fixture(t)), "/api/tree"), &tree)

	byID := map[string][]Item{}
	var ids []string
	for _, g := range tree.Groups {
		ids = append(ids, g.ID)
		byID[g.ID] = g.Items
	}
	if got := strings.Join(ids, ","); got != "wiki,adr,codewiki,docs,plans,specs" {
		t.Fatalf("groups = %s", got)
	}
	paths := func(id string) string {
		var out []string
		for _, it := range byID[id] {
			out = append(out, it.Path)
		}
		return strings.Join(out, ",")
	}
	want := map[string]string{
		"wiki":     "docs/wiki/index.md,docs/wiki/changelog.md,docs/wiki/pages/alpha.md",
		"adr":      "docs/adr/0001-first.md",
		"codewiki": "docs/codewiki/packages.md",
		"docs":     "docs/glossary.md",
		"plans":    "plans/ship.md",
		"specs":    "specs/feat/requirements.md,specs/feat/design.md,specs/feat/tasks.md",
	}
	for id, w := range want {
		if got := paths(id); got != w {
			t.Errorf("%s = %s, want %s", id, got, w)
		}
	}
	plan := byID["plans"][0]
	if plan.Title != "Ship" || plan.Done != 1 || plan.Total != 2 {
		t.Errorf("plan item = %+v, want Ship 1/2", plan)
	}
}

func TestTreeReadsTheDiskOnEveryRequest(t *testing.T) {
	root := fixture(t)
	h := Handler(root)
	get(t, h, "/api/tree")
	if err := os.WriteFile(filepath.Join(root, "plans", "later.md"), []byte("# Later\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var tree Tree
	decodeBody(t, get(t, h, "/api/tree"), &tree)
	for _, g := range tree.Groups {
		if g.ID == "plans" && len(g.Items) == 2 {
			return
		}
	}
	t.Fatal("a plan written after the first request is missing from the second")
}

func TestTreeKeepsEmptyGroups(t *testing.T) {
	var tree Tree
	decodeBody(t, get(t, Handler(t.TempDir()), "/api/tree"), &tree)
	if len(tree.Groups) != 6 {
		t.Fatalf("groups = %d, want 6 even with nothing on disk", len(tree.Groups))
	}
	for _, g := range tree.Groups {
		if g.Items == nil {
			t.Errorf("%s items is null, want []", g.ID)
		}
	}
}

func TestPageSplitsFrontmatterFromBody(t *testing.T) {
	var p Page
	decodeBody(t, get(t, Handler(fixture(t)), "/api/page?path=docs/adr/0001-first.md"), &p)
	if p.Frontmatter["status"] != "accepted" {
		t.Errorf("frontmatter = %v", p.Frontmatter)
	}
	if strings.Contains(p.Body, "status:") || !strings.HasPrefix(strings.TrimSpace(p.Body), "# First") {
		t.Errorf("body = %q, want the Markdown after the frontmatter", p.Body)
	}
	if p.Title != "First" || len(p.Sections) == 0 {
		t.Errorf("title %q, sections %v", p.Title, p.Sections)
	}
}

func TestPageCarriesTaskProgress(t *testing.T) {
	var p Page
	decodeBody(t, get(t, Handler(fixture(t)), "/api/page?path=plans/ship.md"), &p)
	if p.Done != 1 || p.Total != 2 || len(p.Tasks) != 2 || !p.Tasks[0].Checked {
		t.Errorf("progress %d/%d, tasks %+v", p.Done, p.Total, p.Tasks)
	}
}

func TestPageRefusesAnythingButMarkdownInTheThreeTrees(t *testing.T) {
	h := Handler(fixture(t))
	for _, rel := range []string{
		"",
		"notes.txt",
		"cmd/main.go",
		"docs/../plans/ship.md",
		"../outside.md",
		"/etc/passwd",
		"docs\\glossary.md",
		"C:/Windows/win.ini",
		"docs/missing.md",
		"docs/wiki",
		"docs/raw/.hidden/secret.md",
	} {
		if w := get(t, h, "/api/page?path="+rel); w.Code != http.StatusNotFound {
			t.Errorf("page %q = %d, want 404", rel, w.Code)
		}
	}
}

func TestPageRefusesASymlinkOutOfTheWorkspace(t *testing.T) {
	root := fixture(t)
	outside := filepath.Join(t.TempDir(), "x.md")
	if err := os.WriteFile(outside, []byte("# Outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "docs", "link.md")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	if w := get(t, Handler(root), "/api/page?path=docs/link.md"); w.Code != http.StatusNotFound {
		t.Errorf("symlink out of the workspace = %d, want 404", w.Code)
	}
}

// A symlink committed inside the workspace passes the containment check, so the
// dot rule has to hold on where it points, not only on what was asked for.
func TestSourceRefusesASymlinkToADotPath(t *testing.T) {
	root := fixture(t)
	if err := os.Symlink(filepath.Join(root, ".env"), filepath.Join(root, "cmd", "env.txt")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	if w := get(t, Handler(root), "/api/source?path=cmd/env.txt"); w.Code != http.StatusNotFound {
		t.Errorf("symlink to .env = %d, want 404", w.Code)
	}
}

// On Windows a dot-named directory usually has an 8.3 alias — .git is GIT~1 —
// which carries no dot, and reaches the same file.
func TestSourceRefusesAShortNameForADotPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("8.3 short names are a Windows filesystem feature")
	}
	root := fixture(t)
	if _, err := os.Stat(filepath.Join(root, "internal", "CACHE~1", "x.go")); err != nil {
		t.Skipf("no 8.3 alias on this volume: %v", err)
	}
	if w := get(t, Handler(root), "/api/source?path=internal/CACHE~1/x.go"); w.Code != http.StatusNotFound {
		t.Errorf("short name for internal/.cache = %d, want 404", w.Code)
	}
}

func TestTreeLeavesOutAnOversizedPage(t *testing.T) {
	root := fixture(t)
	big := "# Big\n" + strings.Repeat("a", maxFile)
	if err := os.WriteFile(filepath.Join(root, "docs", "big.md"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	h := Handler(root)
	if w := get(t, h, "/api/page?path=docs/big.md"); w.Code != http.StatusNotFound {
		t.Errorf("oversized page = %d, want 404", w.Code)
	}
	if strings.Contains(get(t, h, "/api/tree").Body.String(), "docs/big.md") {
		t.Error("oversized page is listed in the tree")
	}
}

func TestSourceReturnsLines(t *testing.T) {
	var s Source
	decodeBody(t, get(t, Handler(fixture(t)), "/api/source?path=cmd/main.go"), &s)
	if len(s.Lines) != 3 || s.Lines[0] != "package main" || s.Lines[2] != "func main() {}" {
		t.Errorf("lines = %q", s.Lines)
	}
}

func TestSourceRefusesDotSegmentsBinariesAndEscapes(t *testing.T) {
	root := fixture(t)
	big := filepath.Join(root, "big.txt")
	if err := os.WriteFile(big, []byte(strings.Repeat("a", maxFile+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	h := Handler(root)
	for _, rel := range []string{".env", "internal/.cache/x.go", "bin/blob", "big.txt", "../x", "cmd/../.env", "cmd"} {
		if w := get(t, h, "/api/source?path="+rel); w.Code != http.StatusNotFound {
			t.Errorf("source %q = %d, want 404", rel, w.Code)
		}
	}
}

func TestGuardRefusesForeignHosts(t *testing.T) {
	h := Handler(fixture(t))
	for host, want := range map[string]int{
		"127.0.0.1:7777":    http.StatusOK,
		"localhost:7777":    http.StatusOK,
		"[::1]:7777":        http.StatusOK,
		"LOCALHOST":         http.StatusOK,
		"evil.example:7777": http.StatusForbidden,
		"127.0.0.1.nip.io":  http.StatusForbidden,
		"":                  http.StatusForbidden,
	} {
		r := httptest.NewRequest(http.MethodGet, "/api/tree", nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Errorf("Host %q = %d, want %d", host, w.Code, want)
		}
	}
}

func TestGuardRefusesWrites(t *testing.T) {
	h := Handler(fixture(t))
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		r := httptest.NewRequest(m, "/api/tree", nil)
		r.Host = "localhost"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s = %d, want 405", m, w.Code)
		}
	}
}

func TestEveryResponseCarriesTheContentSecurityPolicy(t *testing.T) {
	h := Handler(fixture(t))
	for _, target := range []string{"/", "/api/tree", "/api/page?path=nope.md"} {
		got := get(t, h, target).Header().Get("Content-Security-Policy")
		if !strings.Contains(got, "default-src 'self'") || strings.Contains(got, "script-src 'unsafe-inline'") {
			t.Errorf("%s CSP = %q", target, got)
		}
	}
}

func TestAppFallsBackToIndex(t *testing.T) {
	h := Handler(fixture(t))
	for _, target := range []string{"/", "/some/client/route"} {
		w := get(t, h, target)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "<!doctype html>") {
			t.Errorf("%s = %d %q, want index.html", target, w.Code, w.Body.String())
		}
	}
	if w := get(t, h, "/assets/gone-1234.js"); w.Code != http.StatusNotFound {
		t.Errorf("missing asset = %d, want 404 rather than the app", w.Code)
	}
	if w := get(t, h, "/api/unknown"); w.Code != http.StatusNotFound {
		t.Errorf("/api/unknown = %d, want 404 rather than the app", w.Code)
	}
}

func TestServeStopsWhenTheContextIsCancelled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	root := fixture(t)
	go func() { done <- Serve(ctx, ln, root) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/api/tree")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("live server answered %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v on cancel, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}
