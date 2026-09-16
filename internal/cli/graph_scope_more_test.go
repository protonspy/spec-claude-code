package cli

import (
	"bytes"
	"os"
	"path/filepath"
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
