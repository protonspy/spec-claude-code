package headroom

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/paths"
)

// Every harness scc scaffolds for is one Headroom wraps, so `scc launch` never
// silently drops compression for a harness that could have had it. A new harness
// arriving here without a slug is a decision to make deliberately — add it, or
// accept that launching it starts the agent bare — not one to discover in the
// field.
func TestEveryHarnessHasAnAgentSlug(t *testing.T) {
	for _, h := range paths.Harnesses() {
		slug, ok := Agent(h)
		if !ok {
			t.Errorf("%s has no headroom agent slug", h.ID)
			continue
		}
		if slug == "" {
			t.Errorf("%s maps to an empty slug", h.ID)
		}
	}
}

func TestAgentRejectsAnUnknownHarness(t *testing.T) {
	if _, ok := Agent(paths.Harness{ID: "nope"}); ok {
		t.Error("an unknown harness reported a slug")
	}
}

// WrapArgs is the whole command line, and it must not alias the caller's slice:
// the pass-through arguments come straight off the command line, and appending
// into their backing array would corrupt what the caller still holds.
func TestWrapArgsPrefixesWithoutAliasing(t *testing.T) {
	rest := []string{"--resume", "--model", "opus"}
	got := WrapArgs("claude", nil, rest)

	want := []string{"wrap", "claude", "--resume", "--model", "opus"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("WrapArgs = %v, want %v", got, want)
	}
	got[2] = "clobbered"
	if rest[0] != "--resume" {
		t.Errorf("WrapArgs wrote through to the caller's slice: %v", rest)
	}

	if got := WrapArgs("codex", nil, nil); strings.Join(got, " ") != "wrap codex" {
		t.Errorf("WrapArgs with no pass-through = %v", got)
	}
}

// scc's own options go in front of the pass-through, so a user who names the same
// flag after `--` is the one who wins: click takes the last occurrence, and the
// argument the user typed has to beat the default scc supplied.
func TestWrapArgsPutsSCCsOptionsBeforeThePassthrough(t *testing.T) {
	got := WrapArgs("claude", []string{"--code-memory", "none"}, []string{"--code-memory", "serena"})
	want := "wrap claude --code-memory none --code-memory serena"
	if strings.Join(got, " ") != want {
		t.Errorf("WrapArgs = %q, want %q", strings.Join(got, " "), want)
	}
}

// The opt-out flags are read off the binary's own help rather than compiled in,
// because Headroom has already renamed this control once. Each spelling has to
// resolve to the vector that build actually accepts.
func TestMCPOffArgsFollowsTheBuildsOwnSpelling(t *testing.T) {
	current := HelpFlags("  --no-mcp   Skip it\n  --code-memory [serena|none]  Code-memory MCP\n")
	if got := strings.Join(MCPOffArgs(current, MCPRetrieve), " "); got != "--code-memory none" {
		t.Errorf("retrieve on a current build = %q", got)
	}
	if got := strings.Join(MCPOffArgs(current, MCPNone), " "); got != "--code-memory none --no-mcp" {
		t.Errorf("none on a current build = %q", got)
	}

	// The older vocabulary, which opencode's wrap still speaks.
	older := HelpFlags("  --no-mcp  Skip\n  --no-serena  Never register Serena\n  --no-tokensave  Skip tokensave\n")
	if got := strings.Join(MCPOffArgs(older, MCPRetrieve), " "); got != "--no-serena --no-tokensave" {
		t.Errorf("retrieve on an older build = %q", got)
	}

	// MCPAll is scc keeping its hands off, whatever the build offers.
	if got := MCPOffArgs(current, MCPAll); len(got) != 0 {
		t.Errorf("all = %v, want no arguments", got)
	}
}

// A build that advertises no opt-out has answered the question, and scc has to
// take the answer: inventing a flag would trade one unwanted MCP server for a
// launch that dies on "no such option".
func TestMCPOffArgsInventsNothing(t *testing.T) {
	for _, mode := range []MCPMode{MCPRetrieve, MCPNone} {
		if got := MCPOffArgs(HelpFlags("Usage: headroom wrap claude [OPTIONS]\n"), mode); len(got) != 0 {
			t.Errorf("%s against a build with no opt-out = %v, want nothing", mode, got)
		}
	}
}

// Headroom's context-tool setup writes RTK guidance into the same entry file
// `scc rtk` splices, behind its own marker pair — so the file ends up carrying
// the instructions twice. --no-context-tool is the primary spelling and covers
// lean-ctx too; --no-rtk is the same option's older alias.
func TestContextToolOffArgsPrefersThePrimarySpelling(t *testing.T) {
	both := HelpFlags("  --no-context-tool, --no-rtk  Skip CLI context-tool setup\n")
	if got := strings.Join(ContextToolOffArgs(both), " "); got != "--no-context-tool" {
		t.Errorf("with both spellings = %q, want --no-context-tool", got)
	}

	older := HelpFlags("  --no-rtk  Skip rtk setup\n")
	if got := strings.Join(ContextToolOffArgs(older), " "); got != "--no-rtk" {
		t.Errorf("with only the alias = %q, want --no-rtk", got)
	}

	if got := ContextToolOffArgs(HelpFlags("Usage: headroom wrap claude\n")); len(got) != 0 {
		t.Errorf("against a build with no opt-out = %v, want nothing", got)
	}
}

func TestParseMCPModeRoundTrips(t *testing.T) {
	for _, want := range []MCPMode{MCPAll, MCPRetrieve, MCPNone} {
		got, err := ParseMCPMode(want.String())
		if err != nil {
			t.Fatalf("ParseMCPMode(%q): %v", want.String(), err)
		}
		if got != want {
			t.Errorf("ParseMCPMode(%q) = %v", want.String(), got)
		}
	}
	if _, err := ParseMCPMode("serena"); err == nil {
		t.Error("ParseMCPMode accepted a mode scc does not define")
	}
}

// uv is what Headroom's docs lead with and what puts the CLI in an isolated
// environment, so it has to be tried first on a machine that has both.
func TestInstallersPreferUV(t *testing.T) {
	all := Installers()
	if len(all) < 2 {
		t.Fatalf("installers = %v, want at least uv and pip", all)
	}
	if all[0].Prog != "uv" {
		t.Errorf("first installer is %q, want uv", all[0].Prog)
	}
	for _, i := range all {
		if !strings.Contains(i.Cmd, Dist) {
			t.Errorf("%s does not install %s: %q", i.Prog, Dist, i.Cmd)
		}
		// The printable form is quoted for a shell; the argv form must not be, or
		// the extras bracket becomes part of the distribution name.
		if !strings.Contains(i.Cmd, `"`+Dist+`"`) {
			t.Errorf("%s's printable command does not quote the extras: %q", i.Prog, i.Cmd)
		}
		found := false
		for _, a := range i.Args {
			if a == Dist {
				found = true
			}
		}
		if !found {
			t.Errorf("%s's argv does not carry %s unquoted: %v", i.Prog, Dist, i.Args)
		}
	}
}

// npm publishes headroom-ai too, but that package is the TypeScript SDK with no
// CLI: installing it would report success and leave `headroom` still missing.
func TestNPMIsNotAnInstaller(t *testing.T) {
	for _, i := range Installers() {
		if i.Prog == "npm" {
			t.Error("npm is listed as an installer, but it ships no CLI")
		}
	}
	if strings.Contains(InstallHint(), "npm") {
		t.Errorf("the install hint offers npm: %q", InstallHint())
	}
}

// The hint is what somebody reads when scc cannot install for them, so it has to
// name every way rather than only the one scc would have picked.
func TestInstallHintNamesEveryInstaller(t *testing.T) {
	hint := InstallHint()
	for _, i := range Installers() {
		if !strings.Contains(hint, i.Cmd) {
			t.Errorf("the hint omits %s: %q", i.Prog, hint)
		}
	}
}

// A missing uv is reported as a missing uv. Calling it a failed install would
// send the user looking at Headroom instead of at their own toolchain.
func TestInstallReportsAMissingProgram(t *testing.T) {
	err := Install(Installer{Prog: "definitely-not-a-real-program", Cmd: "nope"}, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("Install with a missing program returned no error")
	}
	if !strings.Contains(err.Error(), "not on PATH") {
		t.Errorf("error = %q, want it to say the program is not on PATH", err)
	}
}

// The binary half, driven against stand-ins on a replaced PATH: the real install
// resolves and builds a Python distribution, which is not a thing a test suite
// does to whoever runs it.
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
	// A build that cannot answer means scc passes no options rather than guessing.
	if got := WrapHelp("definitely-not-a-real-program", "claude"); got != "" {
		t.Errorf("WrapHelp invented %q", got)
	}
	// A missing program is reported as itself: the user has to get uv or pip
	// first, which is a different problem from an install that broke.
	err := Install(Installer{Prog: "definitely-not-a-real-program", Cmd: "nope"}, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("Install with a missing program returned no error")
	}
	if !strings.Contains(err.Error(), "not on PATH") {
		t.Errorf("error = %q, want it to say the program is not on PATH", err)
	}

	// uv first, because it is what Headroom's own docs lead with — so a machine
	// carrying both has to choose it.
	uv, pip := fakeBin(t, "uv", "", 0), fakeBin(t, "pip", "", 0)
	onlyPath(t, pip, uv)
	i, ok := Available()
	if !ok {
		t.Fatal("Available false with both installers on PATH")
	}
	if i.Prog != "uv" {
		t.Errorf("Available chose %q with uv present", i.Prog)
	}
	if err := Install(i, io.Discard, io.Discard); err != nil {
		t.Errorf("Install with a working uv: %v", err)
	}

	onlyPath(t, pip)
	if i, ok := Available(); !ok || i.Prog != "pip" {
		t.Errorf("Available = %+v, %v on a machine with only pip", i, ok)
	}

	// The binary, and the help a wrap reads its opt-outs out of.
	onlyPath(t, fakeBin(t, Bin, "headroom 1.2.3", 0))
	p, ok := Path()
	if !ok {
		t.Fatal("Path did not find the binary on PATH")
	}
	if got := Version(p); got != "headroom 1.2.3" {
		t.Errorf("Version = %q", got)
	}
	if got := WrapHelp(p, "claude"); !strings.Contains(got, "headroom 1.2.3") {
		t.Errorf("WrapHelp = %q, want whatever the build printed", got)
	}

	// An install that fails is reported as the command that failed, so the user
	// can run it again themselves and see the same output.
	onlyPath(t, fakeBin(t, "uv", "", 1))
	i, _ = Available()
	if err := Install(i, io.Discard, io.Discard); err == nil {
		t.Error("Install returned no error when the installer failed")
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
