package jail

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The terminator is the whole reason Args exists rather than a slice literal at the
// call site: ai-jail parses flags out of its tail, so an agent argument that collides
// with one of its own would be eaten by the sandbox instead of reaching the agent.
func TestArgsTerminatesTheSandboxOptions(t *testing.T) {
	got := Args([]string{FlagNetwork, FlagState}, nil, "claude", []string{"--resume", "-p"})
	want := "--network --agent-state -- claude --resume -p"
	if strings.Join(got, " ") != want {
		t.Errorf("Args = %q, want %q", strings.Join(got, " "), want)
	}
}

// A colliding agent flag reaches the agent, because everything after `--` does.
func TestArgsKeepsACollidingAgentFlagOnTheAgentSide(t *testing.T) {
	got := Args([]string{FlagNetwork}, nil, "claude", []string{"--network"})
	term := -1
	for i, a := range got {
		if a == "--" {
			term = i
			break
		}
	}
	if term < 0 {
		t.Fatalf("no terminator in %q", got)
	}
	if strings.Join(got[term+1:], " ") != "claude --network" {
		t.Errorf("after the terminator: %q, want %q", got[term+1:], "claude --network")
	}
}

// What the user passed deliberately comes after what scc supplies, so it wins.
func TestArgsPutsTheUsersFlagsLast(t *testing.T) {
	got := Args([]string{FlagNetwork}, []string{"--lockdown", "--no-network"}, "claude", nil)
	if strings.Join(got, " ") != "--network --lockdown --no-network -- claude" {
		t.Errorf("Args = %q", got)
	}
}

func TestNeededReadsTheBuildsOwnHelp(t *testing.T) {
	full := HelpFlags(`
  --network / --no-network      enable network
  --agent-state                 mount credential state
  --lockdown                    strict read-only
`)
	have, missing := Needed(full)
	if len(have) != 2 || len(missing) != 0 {
		t.Errorf("have = %v, missing = %v, want both flags found", have, missing)
	}

	// A build that spells one of them differently is reported, never substituted:
	// scc guessing at a replacement is how a sandbox gets opened by a helper.
	partial := HelpFlags("  --agent-state   mount credential state\n")
	have, missing = Needed(partial)
	if strings.Join(have, " ") != FlagState || strings.Join(missing, " ") != FlagNetwork {
		t.Errorf("have = %v, missing = %v", have, missing)
	}

	// No help at all is every flag missing, which is the honest reading of an
	// answer nobody could get.
	if _, missing := Needed(HelpFlags("")); len(missing) != 2 {
		t.Errorf("missing = %v, want both", missing)
	}
}

func TestHelpFlagsIgnoresProse(t *testing.T) {
	flags := HelpFlags("Usage: ai-jail [OPTIONS] [--] [COMMAND]\n  --network  run with a network\n")
	if !flags["--network"] {
		t.Error("--network not found")
	}
	if flags["--"] || flags["-"] {
		t.Errorf("a bare terminator was read as a flag: %v", flags)
	}
}

// Supported and Backend have to agree, or a report says the platform is fine and
// then names no sandbox to do it with.
func TestSupportedAgreesWithBackend(t *testing.T) {
	if Supported() != (Backend() != "") {
		t.Errorf("Supported() = %v but Backend() = %q", Supported(), Backend())
	}
	switch runtime.GOOS {
	case "linux":
		if Backend() != "bubblewrap" {
			t.Errorf("linux backend = %q", Backend())
		}
	case "darwin":
		if Backend() != "sandbox-exec" {
			t.Errorf("darwin backend = %q", Backend())
		}
	default:
		if Supported() {
			t.Errorf("%s reported as supported", runtime.GOOS)
		}
		// The message names the way out, not just the wall.
		if !strings.Contains(Unsupported(), "WSL2") {
			t.Errorf("Unsupported() = %q, want it to name WSL2", Unsupported())
		}
	}
}

func TestInstallHintNamesSomethingToRun(t *testing.T) {
	if !strings.Contains(InstallCmd(), Crate) {
		t.Errorf("InstallCmd() = %q", InstallCmd())
	}
	if !strings.Contains(InstallHint(), "install") {
		t.Errorf("InstallHint() = %q", InstallHint())
	}
}

// The binary half, driven against stand-ins on a replaced PATH: the real install
// is a Rust build that takes minutes.
func TestTheBinaryHalfAnswersAboutWhatIsOnPATH(t *testing.T) {
	onlyPath(t, t.TempDir())
	if Available() {
		t.Error("Available true with no cargo on PATH")
	}
	if p, ok := Path(); ok {
		t.Errorf("Path found %q with an empty PATH", p)
	}
	if got := Version("definitely-not-a-real-program"); got != "" {
		t.Errorf("Version invented %q for a binary that is not there", got)
	}
	// A build that cannot answer means scc asks for nothing rather than guessing
	// — a sandbox opened by a guess is the failure this whole feature prevents.
	if got := Help("definitely-not-a-real-program"); got != "" {
		t.Errorf("Help invented %q", got)
	}
	// Missing cargo is reported as itself: a Rust toolchain is a different problem
	// from a build that broke.
	err := Install(io.Discard, io.Discard)
	if err == nil {
		t.Fatal("Install with no cargo returned no error")
	}
	if !strings.Contains(err.Error(), "cargo is not on PATH") {
		t.Errorf("error = %q, want it to name the missing toolchain", err)
	}

	onlyPath(t, fakeBin(t, "cargo", "", 0), fakeBin(t, Bin, "ai-jail 0.4.0", 0))
	if !Available() {
		t.Error("Available false with cargo right there")
	}
	p, ok := Path()
	if !ok {
		t.Fatal("Path did not find the binary on PATH")
	}
	if got := Version(p); got != "ai-jail 0.4.0" {
		t.Errorf("Version = %q", got)
	}
	if got := Help(p); !strings.Contains(got, "ai-jail 0.4.0") {
		t.Errorf("Help = %q, want whatever the build printed", got)
	}
	if err := Install(io.Discard, io.Discard); err != nil {
		t.Errorf("Install with a working cargo: %v", err)
	}

	onlyPath(t, fakeBin(t, "cargo", "", 1))
	if err := Install(io.Discard, io.Discard); err == nil {
		t.Error("Install returned no error when cargo failed")
	} else if !strings.Contains(err.Error(), InstallCmd()) {
		t.Errorf("error = %q, which does not name the command that failed", err)
	}
}

// Every platform answers all three consistently: a platform with a backend names
// it and hides something, and one without names neither and says why.
func TestThePlatformAnswersAreConsistent(t *testing.T) {
	if Supported() {
		if Backend() == "" {
			t.Error("a supported platform names no backend")
		}
		if HiddenRoot() == "" {
			t.Error("a supported platform hides nothing, so no toolchain would ever be mapped")
		}
		if !strings.Contains(InstallHint(), InstallCmd()) && runtime.GOOS != "darwin" {
			t.Errorf("InstallHint = %q, which does not carry the install command", InstallHint())
		}
		return
	}
	if Backend() != "" {
		t.Errorf("an unsupported platform names the backend %q", Backend())
	}
	if HiddenRoot() != "" {
		t.Errorf("an unsupported platform hides %q", HiddenRoot())
	}
	// Somebody on Windows has to be told where the answer is, and WSL2 is a real
	// answer rather than a workaround.
	if !strings.Contains(Unsupported(), "WSL2") {
		t.Errorf("Unsupported() = %q, which does not name the way forward", Unsupported())
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
		writeBin(t, filepath.Join(dir, name+".cmd"), body, 0o644)
		return dir
	}
	body := "#!/bin/sh\n"
	if stdout != "" {
		body += "printf '%s\n' '" + strings.ReplaceAll(stdout, "'", `'\''`) + "'\n"
	}
	body += fmt.Sprintf("exit %d\n", exit)
	writeBin(t, filepath.Join(dir, name), body, 0o755)
	return dir
}

func writeBin(t *testing.T, path, body string, mode os.FileMode) {
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
