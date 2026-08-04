package codegraph

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The graph directory is the whole test for "this workspace is indexed", so it
// has to be a directory rather than anything that merely shares the name: a file
// called .codegraph is not a graph, and treating it as one would send every later
// command into CodeGraph's error instead of scc's.
func TestIndexedWantsADirectory(t *testing.T) {
	root := t.TempDir()
	if Indexed(root) {
		t.Error("an empty workspace reported a graph")
	}

	file := filepath.Join(root, Dir)
	if err := os.WriteFile(file, []byte("not a graph"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if Indexed(root) {
		t.Error("a regular file named .codegraph reported a graph")
	}

	if err := os.Remove(file); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := os.Mkdir(file, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if !Indexed(root) {
		t.Error("a workspace with .codegraph/ reported no graph")
	}
}

// The argument vectors are the whole integration, and every flag in them has to
// be one CodeGraph actually defines — a wrap that invents a flag turns a working
// command into a usage error.
func TestArgVectors(t *testing.T) {
	cases := []struct {
		name string
		got  []string
		want string
	}{
		{"init", InitArgs(), "init"},
		{"sync", SyncArgs(), "sync"},
		{"index", IndexArgs(), "index --force"},
		{"status", StatusArgs(false), "status"},
		{"status json", StatusArgs(true), "status --json"},
		{"query", QueryArgs("UserService", "", 0, false), "query UserService"},
		{"query full", QueryArgs("UserService", "class", 10, true), "query UserService --kind class --limit 10 --json"},
		{"explore", ExploreArgs("how does login work"), "explore how does login work"},
	}
	for _, c := range cases {
		if got := strings.Join(c.got, " "); got != c.want {
			t.Errorf("%s = %q, want %q", c.name, got, c.want)
		}
	}
}

// explore is the CLI face of the MCP tool and emits the same agent-shaped text,
// so it defines no --json. Passing one would fail the command.
func TestExploreTakesNoJSONFlag(t *testing.T) {
	for _, a := range ExploreArgs("anything") {
		if a == "--json" {
			t.Error("ExploreArgs passes --json, which explore does not define")
		}
	}
}

// A remote script piped into a shell is a fine thing for a person to type and not
// a thing scc runs for them: it executes whatever the URL serves at that moment.
// The hint may name it; the installer list may not.
func TestInstallersNeverPipeAScriptIntoAShell(t *testing.T) {
	for _, i := range Installers() {
		if i.Prog == "curl" || i.Prog == "sh" || i.Prog == "powershell" || i.Prog == "irm" {
			t.Errorf("installer %q runs a remote script", i.Prog)
		}
		if strings.Contains(i.Cmd, "|") {
			t.Errorf("installer command pipes: %q", i.Cmd)
		}
	}
	// But somebody without npm still needs a way through, and that decision is
	// theirs to make.
	if !strings.Contains(InstallHint(), Repo) {
		t.Errorf("the hint does not point at the standalone bundle: %q", InstallHint())
	}
}

func TestInstallHintNamesEveryInstaller(t *testing.T) {
	hint := InstallHint()
	for _, i := range Installers() {
		if !strings.Contains(hint, i.Cmd) {
			t.Errorf("the hint omits %s: %q", i.Prog, hint)
		}
	}
}

// A missing npm is reported as a missing npm. Calling it a failed install would
// send the user looking at CodeGraph instead of at their own toolchain.
func TestInstallReportsAMissingProgram(t *testing.T) {
	err := Install(Installer{Prog: "definitely-not-a-real-program", Cmd: "nope"}, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("Install with a missing program returned no error")
	}
	if !strings.Contains(err.Error(), "not on PATH") {
		t.Errorf("error = %q, want it to say the program is not on PATH", err)
	}
}

// Run reports the child's exit code rather than collapsing it, so a caller can
// tell "the graph has no answer" from "the command could not run".
func TestRunSeparatesAFailedExitFromAFailedStart(t *testing.T) {
	code, err := Run("definitely-not-a-real-program", t.TempDir(), []string{"status"}, io.Discard, io.Discard)
	if err == nil {
		t.Error("Run with a missing binary returned no error")
	}
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
}
