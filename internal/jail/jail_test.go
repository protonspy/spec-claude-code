package jail

import (
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
