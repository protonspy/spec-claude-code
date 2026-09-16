package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withGraphCapture stands in for the capturing runner, so a fan-out test can state
// what each root printed instead of needing CodeGraph installed.
func withGraphCapture(t *testing.T, out string, code int) *[]string {
	t.Helper()
	var dirs []string
	orig := graphCapture
	graphCapture = func(bin, dir string, args []string, buf *bytes.Buffer) int {
		dirs = append(dirs, dir)
		buf.WriteString(out)
		return code
	}
	t.Cleanup(func() { graphCapture = orig })
	return &dirs
}

// A scope is one graph per directory rather than one graph filtered, so every
// command answers once per root — and N JSON documents concatenated are not a
// document, so --json comes back as an array of sections.
func TestGraphJSONOverAScopeIsAnArrayOfSections(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "codegraph")
	for _, dir := range []string{"backend", "frontend"} {
		if err := os.MkdirAll(filepath.Join(root, dir, "src", ".codegraph"), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	if _, stderr, code := run(t, "graph", "scope", "set", "backend/src", "frontend/src", "--root", root); code != ExitOK {
		t.Fatalf("scope set: exit %d (%s)", code, stderr)
	}

	dirs := withGraphCapture(t, `{"symbols": 3}`, ExitOK)
	stdout, _, code := run(t, "graph", "status", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("graph status --json: exit %d", code)
	}
	if len(*dirs) != 2 {
		t.Fatalf("the command ran in %d roots, want one per scoped tree: %v", len(*dirs), *dirs)
	}

	var doc []struct {
		Path   string         `json:"path"`
		Output map[string]int `json:"output"`
		Raw    string         `json:"raw"`
		Exit   int            `json:"exit"`
	}
	decode(t, stdout, &doc)
	if len(doc) != 2 {
		t.Fatalf("the document has %d sections, want one per root: %+v", len(doc), doc)
	}
	for _, s := range doc {
		if s.Path == "" {
			t.Errorf("a section does not say which root it came from: %+v", s)
		}
		// The root's own document is passed through rather than re-encoded:
		// nesting a document is not reading one.
		if s.Output["symbols"] != 3 {
			t.Errorf("a section did not carry the root's own output: %+v", s)
		}
	}

	// A root whose output is not JSON after all degrades to a string rather than
	// breaking the array — a CodeGraph that prints a warning above its document is
	// still something a consumer can read.
	withGraphCapture(t, "warning: something\n", ExitOK)
	stdout, _, code = run(t, "graph", "status", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("graph status --json: exit %d", code)
	}
	doc = doc[:0]
	decode(t, stdout, &doc)
	for _, s := range doc {
		if s.Raw == "" {
			t.Errorf("output that is not JSON did not come back as a string: %+v", s)
		}
	}

	// A failing root carries its exit code into its own section, and the command's
	// exit is the failure rather than the last root's.
	withGraphCapture(t, "", ExitError)
	if _, _, code := run(t, "graph", "status", "--root", root, "--json"); code == ExitOK {
		t.Error("a failing root reported success")
	}
}

// Scoped or not, the scope is reported: "the graph is current" means something
// different when it is two graphs, so a run that scoped says so.
func TestGraphScopeShowSaysWhatIsRecorded(t *testing.T) {
	root := initWorkspace(t)

	stdout, _, code := run(t, "graph", "scope", "--root", root)
	if code != ExitOK {
		t.Fatalf("graph scope: exit %d", code)
	}
	if stdout == "" {
		t.Error("an empty scope said nothing, so nobody learns it is the whole workspace")
	}

	var doc struct {
		Scope []string `json:"scope"`
	}
	stdout, _, code = run(t, "graph", "scope", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("graph scope --json: exit %d", code)
	}
	decode(t, stdout, &doc)
	// [] rather than null: the field is a list of directories, and an empty scope
	// is still a list.
	if doc.Scope == nil {
		t.Error("an empty scope came back as null rather than as an empty list")
	}
}

// A pattern that matches nothing is named and skipped; every pattern matching
// nothing stops the run instead. Falling back to the whole workspace there would
// be the opposite of what the scope asked for, and an expensive way to find out
// about a typo.
func TestAScopeThatMatchesNothingStopsTheRun(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "codegraph")
	if err := os.MkdirAll(filepath.Join(root, "backend", "src"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// One good and one missing: the missing one is named, and the run goes on with
	// what is there.
	if _, stderr, code := run(t, "graph", "scope", "set", "backend/src", "--root", root); code != ExitOK {
		t.Fatalf("scope set: exit %d (%s)", code, stderr)
	}
	if err := os.WriteFile(filepath.Join(root, ".claude", "scc-manifest.json"),
		[]byte(scopedManifest(t, root, "backend/src", "frontend/src")), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	withGraphExec(t, ExitOK)
	stdout, stderr, _ := run(t, "graph", "sync", "--root", root)
	if !strings.Contains(stdout+stderr, "frontend/src") {
		t.Errorf("the missing tree was not named:\n%s%s", stdout, stderr)
	}

	// Every pattern missing: the run stops, and says how to fix or clear it.
	if err := os.WriteFile(filepath.Join(root, ".claude", "scc-manifest.json"),
		[]byte(scopedManifest(t, root, "nowhere/at-all")), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	stdout, stderr, code := run(t, "graph", "sync", "--root", root)
	if code == ExitOK {
		t.Error("a scope that matches nothing indexed the whole workspace anyway")
	}
	if !strings.Contains(stdout+stderr, "scope set") {
		t.Errorf("the error does not say how to fix it:\n%s%s", stdout, stderr)
	}
}

// scopedManifest is this workspace's manifest with the graph scope replaced, so a
// test can state a scope that names trees which are not there — which `scope set`
// itself refuses, and rightly.
func scopedManifest(t *testing.T, root string, dirs ...string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".claude", "scc-manifest.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	list := make([]any, 0, len(dirs))
	for _, d := range dirs {
		list = append(list, d)
	}
	doc["codegraph"] = list
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return string(out)
}

// A scope command with nothing recorded and one with a scope both have to answer,
// and `clear` has to put it back to the whole workspace rather than to an empty
// list nothing indexes.
func TestGraphScopeSetAndClearRoundTrip(t *testing.T) {
	root := initWorkspace(t)
	for _, dir := range []string{"backend/src", "frontend/src"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}

	if _, stderr, code := run(t, "graph", "scope", "set", "backend/src/*", "frontend/src", "--root", root); code != ExitOK {
		t.Fatalf("scope set: exit %d (%s)", code, stderr)
	}
	var doc struct {
		Scope []string `json:"scope"`
	}
	stdout, _, code := run(t, "graph", "scope", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("scope --json: exit %d", code)
	}
	decode(t, stdout, &doc)
	if len(doc.Scope) != 2 {
		t.Fatalf("scope = %v, want both trees", doc.Scope)
	}
	// What people write is `backend/src/*` and what that means is the directory:
	// the pattern is recorded as typed and normalized when the roots are resolved,
	// so a run indexes one tree rather than failing to match a directory.
	isolatedPath(t, "codegraph")
	for _, dir := range []string{"backend/src", "frontend/src"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir), ".codegraph"), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	dirs := withGraphCapture(t, `{"symbols": 1}`, ExitOK)
	if _, stderr, code := run(t, "graph", "status", "--root", root, "--json"); code != ExitOK {
		t.Fatalf("graph status: exit %d (%s)", code, stderr)
	}
	if len(*dirs) != 2 {
		t.Errorf("the trailing glob did not resolve to a directory: %v", *dirs)
	}

	if _, stderr, code := run(t, "graph", "scope", "clear", "--root", root); code != ExitOK {
		t.Fatalf("scope clear: exit %d (%s)", code, stderr)
	}
	stdout, _, code = run(t, "graph", "scope", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("scope --json: exit %d", code)
	}
	cleared := doc
	decode(t, stdout, &cleared)
	if len(cleared.Scope) != 0 {
		t.Errorf("clear left %v recorded", cleared.Scope)
	}
	// Empty is a list, not null: the field is a list of directories.
	if cleared.Scope == nil {
		t.Error("a cleared scope came back as null")
	}
}
