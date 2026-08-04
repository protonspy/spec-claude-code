package headroom

import (
	"io"
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
	got := WrapArgs("claude", rest)

	want := []string{"wrap", "claude", "--resume", "--model", "opus"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("WrapArgs = %v, want %v", got, want)
	}
	got[2] = "clobbered"
	if rest[0] != "--resume" {
		t.Errorf("WrapArgs wrote through to the caller's slice: %v", rest)
	}

	if got := WrapArgs("codex", nil); strings.Join(got, " ") != "wrap codex" {
		t.Errorf("WrapArgs with no pass-through = %v", got)
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
