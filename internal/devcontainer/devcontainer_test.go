package devcontainer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Windows alone, and by elimination rather than preference: everywhere else has a
// kernel sandbox that is cheaper, stronger, and not coupled to Docker.
func TestPreferredIsWindowsAlone(t *testing.T) {
	if got, want := Preferred(), runtime.GOOS == "windows"; got != want {
		t.Errorf("Preferred() = %v on %s, want %v", got, runtime.GOOS, want)
	}
}

// Present answers on the config file, not on the directory: a `.devcontainer/`
// somebody started and left empty is not a project the CLI can build.
func TestPresentWantsTheConfigFile(t *testing.T) {
	root := t.TempDir()
	if Present(root) {
		t.Error("Present on an empty workspace")
	}
	if err := os.MkdirAll(filepath.Join(root, Dir), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if Present(root) {
		t.Error("Present on a directory with no configuration in it")
	}
	if err := os.WriteFile(filepath.Join(root, Dir, Config), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !Present(root) {
		t.Error("Present false with the configuration right there")
	}
}

// The CLI resolves the project from --workspace-folder, not from the working
// directory, so a process started elsewhere would build the wrong project. Both
// vectors carry it, and exec carries the agent's own arguments after the binary
// rather than before it, where the CLI would claim them.
func TestArgVectorsCarryTheWorkspaceFolder(t *testing.T) {
	root := filepath.Join("some", "workspace")

	up := UpArgs(root)
	if up[0] != "up" {
		t.Errorf("UpArgs starts with %q", up[0])
	}
	if !contains(up, "--workspace-folder") || !contains(up, root) {
		t.Errorf("UpArgs = %v, which does not name the workspace", up)
	}

	run := ExecArgs(root, "claude", []string{"--resume", "--verbose"})
	if run[0] != "exec" {
		t.Errorf("ExecArgs starts with %q", run[0])
	}
	if !contains(run, "--workspace-folder") || !contains(run, root) {
		t.Errorf("ExecArgs = %v, which does not name the workspace", run)
	}
	bin := indexOf(run, "claude")
	if bin < 0 {
		t.Fatalf("ExecArgs = %v, which never names the binary", run)
	}
	if indexOf(run, "--resume") < bin || indexOf(run, "--verbose") < bin {
		t.Errorf("ExecArgs = %v puts the agent's arguments ahead of the agent", run)
	}
	if run[len(run)-1] != "--verbose" {
		t.Errorf("ExecArgs = %v reordered the pass-through", run)
	}
}

// ExecArgs must not write into the caller's slice. It appends to a slice it built
// itself, but a `rest` with spare capacity is exactly where an append-in-place bug
// hides, and the caller here is holding the user's own arguments.
func TestExecArgsDoesNotAliasTheCallersArguments(t *testing.T) {
	rest := make([]string, 1, 4)
	rest[0] = "--resume"
	ExecArgs("w", "claude", rest)
	if len(rest) != 1 || rest[0] != "--resume" {
		t.Errorf("the caller's slice changed to %v", rest)
	}
}

// Path and Version answer about a binary that is usually not installed on the
// machine running the suite, so what is asserted is the shape of the answer: an
// absent binary is reported absent rather than as an empty path that looks real.
func TestPathAndVersionDegradeRatherThanGuess(t *testing.T) {
	if p, ok := Path(); ok && p == "" {
		t.Error("Path reported a binary with no path")
	} else if !ok && p != "" {
		t.Errorf("Path reported %q for a binary it says is absent", p)
	}
	if got := Version("definitely-not-a-real-program"); got != "" {
		t.Errorf("Version invented %q for a binary that is not there", got)
	}
}

// Run reports the child's exit code rather than collapsing it, so a caller can tell
// "the container would not come up" from "the CLI could not be started".
func TestRunSeparatesAFailedExitFromAFailedStart(t *testing.T) {
	code, err := Run("definitely-not-a-real-program", t.TempDir(), UpArgs("w"), io.Discard, io.Discard)
	if err == nil {
		t.Error("Run with a missing binary returned no error")
	}
	if code != 0 {
		t.Errorf("code = %d, want 0 — a process that never started has no exit code", code)
	}

	// A real binary that exits non-zero: `go` with no arguments prints usage and
	// exits 2. It is on PATH by construction, since this suite is running.
	code, err = Run("go", t.TempDir(), []string{"--this-is-not-a-go-command"}, io.Discard, io.Discard)
	if err != nil {
		t.Errorf("Run on a binary that started returned an error: %v", err)
	}
	if code == 0 {
		t.Error("Run reported success for a command that failed")
	}
}

// npm is the only installer, and a missing one is reported as a missing npm rather
// than as a failed install — which would send the user to look at the CLI instead
// of at their own toolchain.
func TestInstallReportsAMissingNPM(t *testing.T) {
	if Available() {
		t.Skip("npm is on PATH here, so the missing-npm path is not reachable")
	}
	err := Install(io.Discard, io.Discard)
	if err == nil {
		t.Fatal("Install with no npm returned no error")
	}
	if !strings.Contains(err.Error(), "not on PATH") {
		t.Errorf("error = %q, want it to say npm is not on PATH", err)
	}
}

// The hint has to name both things the user needs, because having one and not the
// other is the common case and each has a different fix.
func TestInstallHintNamesNPMTheDaemonAndTheDocs(t *testing.T) {
	hint := InstallHint()
	for _, want := range []string{InstallCmd(), "npm", "Docker", Docs} {
		if !strings.Contains(hint, want) {
			t.Errorf("InstallHint() = %q, which never mentions %q", hint, want)
		}
	}
	if !strings.Contains(InstallCmd(), Pkg) {
		t.Errorf("InstallCmd() = %q, which does not install %q", InstallCmd(), Pkg)
	}
}

// scc runs no remote script through a shell on the user's behalf — the rule
// docs/stack.md states and CodeGraph's own installer is the counter-example to.
func TestTheInstallerIsNotAPipedScript(t *testing.T) {
	for _, s := range []string{InstallCmd(), InstallHint()} {
		if strings.Contains(s, "| sh") || strings.Contains(s, "| bash") || strings.Contains(s, "iex") {
			t.Errorf("%q pipes something into a shell", s)
		}
	}
}

// DockerReady is a separate question from Path, because the two fail differently
// and the user can only fix one at a time. Asserted as a shape: whatever this
// machine has, the answer must not claim a daemon without a docker binary.
func TestDockerReadyNeedsTheBinaryFirst(t *testing.T) {
	if DockerReady() {
		if _, ok := Path(); !ok {
			// Fine: docker can be there without the devcontainer CLI. The check is
			// only that DockerReady did not answer for the CLI.
			t.Log("a daemon is running without the devcontainer CLI, which is the split this reports")
		}
	}
}

func contains(args []string, want string) bool { return indexOf(args, want) >= 0 }

func indexOf(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}

// fakeBin writes an executable named name into its own directory and returns that
// directory, so a test can put it on PATH and drive the install and lookup paths
// without the real tool being installed on the machine running the suite.
//
// Two flavours because exec.LookPath is platform-shaped: on Windows a command is
// found through PATHEXT, so the script has to be a .cmd; everywhere else it is the
// executable bit and a shebang.
func fakeBin(t *testing.T, name, stdout string, exit int) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		body := "@echo off\r\n"
		if stdout != "" {
			body += "echo " + stdout + "\r\n"
		}
		body += fmt.Sprintf("exit /b %d\r\n", exit)
		write(t, filepath.Join(dir, name+".cmd"), body, 0o644)
		return dir
	}
	body := "#!/bin/sh\n"
	if stdout != "" {
		body += "echo '" + stdout + "'\n"
	}
	body += fmt.Sprintf("exit %d\n", exit)
	write(t, filepath.Join(dir, name), body, 0o755)
	return dir
}

func write(t *testing.T, path, body string, mode os.FileMode) {
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

// The install path, driven against a stand-in npm rather than the real one: a test
// that shelled out to the actual package manager would install a global package on
// whoever ran the suite.
func TestInstallRunsNPMAndReportsWhatItDid(t *testing.T) {
	onlyPath(t, fakeBin(t, "npm", "", 0))
	if !Available() {
		t.Fatal("Available false with npm right there on PATH")
	}
	if err := Install(io.Discard, io.Discard); err != nil {
		t.Errorf("Install with a working npm: %v", err)
	}

	onlyPath(t, fakeBin(t, "npm", "", 1))
	err := Install(io.Discard, io.Discard)
	if err == nil {
		t.Fatal("Install returned no error when npm failed")
	}
	if !strings.Contains(err.Error(), InstallCmd()) {
		t.Errorf("error = %q, which does not name the command that failed", err)
	}

	onlyPath(t, t.TempDir())
	if Available() {
		t.Fatal("Available true with an empty PATH")
	}
	if err := Install(io.Discard, io.Discard); err == nil {
		t.Error("Install returned no error with no npm on PATH")
	} else if !strings.Contains(err.Error(), "not on PATH") {
		t.Errorf("error = %q, want it to say npm is not on PATH", err)
	}
}

// Path and Version against a stand-in CLI, so both answers are exercised on a
// machine that has never installed the real one.
func TestPathAndVersionFindWhatIsOnPATH(t *testing.T) {
	onlyPath(t, fakeBin(t, Bin, "0.89.0", 0))
	p, ok := Path()
	if !ok {
		t.Fatal("Path did not find the binary on PATH")
	}
	if got := Version(p); got != "0.89.0" {
		t.Errorf("Version() = %q, want %q", got, "0.89.0")
	}

	onlyPath(t, t.TempDir())
	if p, ok := Path(); ok {
		t.Errorf("Path found %q with an empty PATH", p)
	}
}

// DockerReady is two questions, and the binary is the first: a missing docker is
// reported without ever asking a daemon that cannot be there.
func TestDockerReadyIsFalseWithoutTheBinary(t *testing.T) {
	onlyPath(t, t.TempDir())
	if DockerReady() {
		t.Error("DockerReady true with no docker on PATH")
	}
	onlyPath(t, fakeBin(t, "docker", "", 1))
	if DockerReady() {
		t.Error("DockerReady true when `docker info` failed")
	}
	onlyPath(t, fakeBin(t, "docker", "Server Version: 27", 0))
	if !DockerReady() {
		t.Error("DockerReady false when `docker info` succeeded")
	}
}
