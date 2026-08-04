package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withGraphExec replaces the CodeGraph runner with a recorder, so the whole
// subcommand surface is drivable without CodeGraph installed.
func withGraphExec(t *testing.T, code int) *[]string {
	t.Helper()
	var got []string
	orig := graphExec
	graphExec = func(bin, root string, args []string) int {
		got = append([]string{bin, root}, args...)
		return code
	}
	t.Cleanup(func() { graphExec = orig })
	return &got
}

// graphArgs is what CodeGraph was asked to do, without the binary and root.
func graphArgs(got []string) string {
	if len(got) < 2 {
		return ""
	}
	return strings.Join(got[2:], " ")
}

func indexWorkspace(t *testing.T, root string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(root, ".codegraph"), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
}

// `build` is two different requests wearing one word. The first time is setup;
// every later time is somebody saying the graph has gone wrong, which is a full
// re-index and has to be asked for. Ordinary staleness is neither.
func TestGraphBuildInitializesOnceThenRefreshes(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "codegraph")
	got := withGraphExec(t, ExitOK)

	if _, stderr, code := run(t, "graph", "build", "--root", root); code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if graphArgs(*got) != "init" {
		t.Errorf("build on a fresh workspace = %q, want init", graphArgs(*got))
	}

	indexWorkspace(t, root)
	if _, stderr, code := run(t, "graph", "build", "--root", root); code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if graphArgs(*got) != "sync" {
		t.Errorf("build on an indexed workspace = %q, want sync", graphArgs(*got))
	}

	if _, stderr, code := run(t, "graph", "build", "--root", root, "--force"); code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if graphArgs(*got) != "index --force" {
		t.Errorf("--force = %q, want a full re-index", graphArgs(*got))
	}
}

// The workspace root, not the shell's directory: `scc graph build` from specs/
// has to index the repository rather than a subtree, which is the whole reason
// this command exists rather than people typing `codegraph init`.
func TestGraphBuildsAtTheWorkspaceRoot(t *testing.T) {
	root := initWorkspace(t)
	sub := filepath.Join(root, "specs", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	isolatedPath(t, "codegraph")
	got := withGraphExec(t, ExitOK)
	t.Chdir(sub)

	if _, stderr, code := run(t, "graph", "build"); code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if !sameDir(t, (*got)[1], root) {
		t.Errorf("indexed %q, want the workspace root %q", (*got)[1], root)
	}
}

// --check answers one question — does this workspace have a graph — with the
// findings code, so CI branches on it the way it branches on `scc validate`. It
// deliberately runs no subprocess: a CI runner without CodeGraph installed still
// has a correct answer.
func TestGraphStatusCheckReportsFindingsWithoutTheBinary(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t)

	stdout, _, code := run(t, "graph", "status", "--root", root, "--check", "--json")
	if code != ExitFindings {
		t.Errorf("exit = %d, want %d for a workspace with no graph", code, ExitFindings)
	}
	var report struct {
		Indexed bool `json:"indexed"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if report.Indexed {
		t.Error("an unindexed workspace reported a graph")
	}

	indexWorkspace(t, root)
	if _, _, code := run(t, "graph", "status", "--root", root, "--check"); code != ExitOK {
		t.Errorf("exit = %d, want %d once the graph is there", code, ExitOK)
	}
}

// CodeGraph's own error for an unindexed project is about a missing database,
// which is true and unhelpful. scc names the command that fixes it instead.
func TestGraphQueryRefusesAnUnindexedWorkspace(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "codegraph")
	got := withGraphExec(t, ExitOK)

	_, stderr, code := run(t, "graph", "query", "UserService", "--root", root)
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "graph build") {
		t.Errorf("stderr does not name the fix: %q", stderr)
	}
	if len(*got) != 0 {
		t.Errorf("scc ran codegraph anyway: %v", *got)
	}
}

// Every flag scc forwards has to be one CodeGraph defines, and the ones it does
// not set stay CodeGraph's own defaults rather than becoming scc's.
func TestGraphQueryForwardsOnlyWhatWasAsked(t *testing.T) {
	root := initWorkspace(t)
	indexWorkspace(t, root)
	isolatedPath(t, "codegraph")
	got := withGraphExec(t, ExitOK)

	if _, _, code := run(t, "graph", "query", "UserService", "--root", root); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if graphArgs(*got) != "query UserService" {
		t.Errorf("bare query = %q, want no invented flags", graphArgs(*got))
	}

	if _, _, code := run(t, "graph", "query", "UserService", "--root", root, "--kind", "class", "--limit", "10", "--json"); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if got, want := graphArgs(*got), "query UserService --kind class --limit 10 --json"; got != want {
		t.Errorf("query = %q, want %q", got, want)
	}
}

// An unquoted question reaches CodeGraph as the sentence it was typed as, rather
// than being rejected as too many positionals.
func TestGraphExploreTakesAnUnquotedQuestion(t *testing.T) {
	root := initWorkspace(t)
	indexWorkspace(t, root)
	isolatedPath(t, "codegraph")
	got := withGraphExec(t, ExitOK)

	if _, stderr, code := run(t, "graph", "explore", "--root", root, "how", "does", "login", "work"); code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if got, want := graphArgs(*got), "explore how does login work"; got != want {
		t.Errorf("explore = %q, want %q", got, want)
	}
}

func TestGraphNeedsSomethingToLookFor(t *testing.T) {
	root := initWorkspace(t)
	indexWorkspace(t, root)
	isolatedPath(t, "codegraph")
	withGraphExec(t, ExitOK)

	for _, sub := range []string{"query", "explore"} {
		_, stderr, code := run(t, "graph", sub, "--root", root)
		if code != ExitError {
			t.Errorf("%s with no query exited %d, want %d", sub, code, ExitError)
		}
		if !strings.Contains(stderr, sub) {
			t.Errorf("%s: stderr does not name the subcommand: %q", sub, stderr)
		}
	}
}

// The whole command is the binary, so a missing CodeGraph is a hard error here
// rather than the degraded run `scc launch` gives it: there is nothing left to
// fall back to.
func TestGraphReportsAMissingBinary(t *testing.T) {
	root := initWorkspace(t)
	indexWorkspace(t, root)
	isolatedPath(t)
	withoutTerminal(t)

	_, stderr, code := run(t, "graph", "sync", "--root", root)
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "codegraph") || !strings.Contains(stderr, "not on PATH") {
		t.Errorf("stderr = %q, want it to name the missing binary", stderr)
	}
}

func TestGraphRequiresAWorkspace(t *testing.T) {
	isolatedPath(t, "codegraph")
	withGraphExec(t, ExitOK)

	_, stderr, code := run(t, "graph", "status", "--root", t.TempDir())
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "not an scc workspace") {
		t.Errorf("stderr = %q, want it to say the directory is not a workspace", stderr)
	}
}

func TestGraphRejectsAnUnknownSubcommand(t *testing.T) {
	_, stderr, code := run(t, "graph", "nope")
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "nope") {
		t.Errorf("stderr does not name the unknown subcommand: %q", stderr)
	}
}
