package codegraph

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

// The binary half, driven against a stand-in npm and a stand-in codegraph on a
// replaced PATH: npm is the only installer scc will run, and a test that ran it
// would install a global package on whoever runs the suite.
func TestTheBinaryHalfAnswersAboutWhatIsOnPATH(t *testing.T) {
	onlyPath(t, t.TempDir())
	if i, ok := Available(); ok {
		t.Errorf("Available returned %+v with an empty PATH", i)
	}
	if p, ok := Path(); ok {
		t.Errorf("Path found %q with an empty PATH", p)
	}
	if got := Version("definitely-not-a-real-program"); got != "" {
		t.Errorf("Version invented %q for a binary that is not there", got)
	}

	onlyPath(t, fakeBin(t, "npm", "", 0), fakeBin(t, Bin, "1.5.0", 0))
	i, ok := Available()
	if !ok {
		t.Fatal("Available false with npm right there")
	}
	p, ok := Path()
	if !ok {
		t.Fatal("Path did not find the binary on PATH")
	}
	if got := Version(p); got != "1.5.0" {
		t.Errorf("Version = %q", got)
	}
	if err := Install(i, io.Discard, io.Discard); err != nil {
		t.Errorf("Install with a working npm: %v", err)
	}

	onlyPath(t, fakeBin(t, "npm", "", 1))
	i, _ = Available()
	if err := Install(i, io.Discard, io.Discard); err == nil {
		t.Error("Install returned no error when npm failed")
	}
}

// Indexed is what tells `init` from `sync`, and it answers on the directory the
// tool actually writes rather than on the workspace having been seen before.
func TestIndexedAnswersOnTheGraphDirectory(t *testing.T) {
	root := t.TempDir()
	if Indexed(root) {
		t.Error("Indexed on a workspace with no graph")
	}
	if err := os.MkdirAll(filepath.Join(root, Dir), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if !Indexed(root) {
		t.Error("Indexed false with the graph directory right there")
	}
}

// fakeBin writes an executable named name into its own directory and returns that
// directory, so a test can put it on PATH and drive a lookup or an install without
// the real tool being installed on the machine running the suite.
func fakeBin(t *testing.T, name, stdout string, exit int) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		body := "@echo off\r\n"
		if stdout != "" {
			body += "echo " + stdout + "\r\n"
		}
		body += fmt.Sprintf("exit /b %d\r\n", exit)
		writeFile(t, filepath.Join(dir, name+".cmd"), body, 0o644)
		return dir
	}
	body := "#!/bin/sh\n"
	if stdout != "" {
		body += "printf '%s\n' '" + strings.ReplaceAll(stdout, "'", `'\''`) + "'\n"
	}
	body += fmt.Sprintf("exit %d\n", exit)
	writeFile(t, filepath.Join(dir, name), body, 0o755)
	return dir
}

func writeFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

// onlyPath replaces PATH with dirs for the length of the test, so a lookup finds
// exactly what the test put there and nothing the machine happens to have.
func onlyPath(t *testing.T, dirs ...string) {
	t.Helper()
	t.Setenv("PATH", strings.Join(dirs, string(os.PathListSeparator)))
}
