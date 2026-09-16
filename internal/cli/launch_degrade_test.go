package cli

import (
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/paths"
)

// Every way of not getting Headroom degrades to starting the agent bare, and each
// says why: a launch that refused to run because a compression proxy was missing
// would be scc putting its own preference above the thing the user asked for.
//
// The reason is the whole value — silence is how somebody spends a month
// wondering why Headroom never seemed to help. Driven against resolveHeadroom
// directly because `--json` implies "install nothing", so the install-shaped
// branches are not reachable through a plan-only run.
func TestHeadroomDegradesWithAReasonEachTime(t *testing.T) {
	for name, tc := range map[string]struct {
		bins  []string
		opts  headroomOptions
		says  string
		wraps bool
	}{
		"no installer at all": {
			bins: nil,
			opts: headroomOptions{},
			says: "neither uv nor pip",
		},
		"an installer, but nobody to ask": {
			bins: []string{"uv"},
			opts: headroomOptions{quiet: true},
			says: "nobody is here",
		},
		"--no-install with an installer right there": {
			bins: []string{"uv"},
			opts: headroomOptions{noInstall: true},
			says: "not on PATH",
		},
		"present, so it wraps": {
			bins:  []string{"headroom"},
			opts:  headroomOptions{quiet: true},
			wraps: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			isolatedPath(t, tc.bins...)
			report := resolveHeadroom(paths.Claude, tc.opts)
			if report == nil {
				t.Fatal("the run said nothing about Headroom at all")
			}
			if report.Wrapping != tc.wraps {
				t.Errorf("wrapping = %v, want %v (reason: %q)", report.Wrapping, tc.wraps, report.Reason)
			}
			if tc.wraps {
				if report.Reason != "" {
					t.Errorf("a wrapping run gave a reason for not wrapping: %q", report.Reason)
				}
				if report.Install != installPresent {
					t.Errorf("install = %q, want %q", report.Install, installPresent)
				}
				return
			}
			if !strings.Contains(report.Reason, tc.says) {
				t.Errorf("reason = %q, want it to mention %q", report.Reason, tc.says)
			}
		})
	}

	// Headroom does not wrap every harness, and one it cannot is reported rather
	// than silently skipped — the user asked for compression and is not getting it.
	isolatedPath(t, "headroom")
	if report := resolveHeadroom(paths.Codex, headroomOptions{quiet: true}); report == nil {
		t.Error("a harness Headroom may not wrap reported nothing at all")
	}

	// And disabled is the one outcome with no report: nobody asked, so there is
	// nothing to explain.
	if report := resolveHeadroom(paths.Claude, headroomOptions{disabled: true}); report != nil {
		t.Errorf("a disabled step still reported %+v", report)
	}
}

// The same contract for the graph, which degrades the same way and for the same
// reason: the agent can still read files, so a missing binary, a declined install
// or a failed index all end in the agent starting anyway with a line saying what
// happened.
func TestTheGraphDegradesWithAReasonEachTime(t *testing.T) {
	for name, tc := range map[string]struct {
		bins []string
		opts graphOptions
		says string
	}{
		"no npm to install with": {
			bins: nil,
			opts: graphOptions{quiet: true},
			says: "npm is not on PATH",
		},
		"npm, but nobody to ask": {
			bins: []string{"npm"},
			opts: graphOptions{quiet: true},
			says: "nobody is here",
		},
		"--no-install with npm right there": {
			bins: []string{"npm"},
			opts: graphOptions{noInstall: true, quiet: true},
			says: "not on PATH",
		},
		"plan-only, which installs and indexes nothing": {
			bins: []string{"npm"},
			opts: graphOptions{plan: true, quiet: true},
			says: "not on PATH",
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := initWorkspace(t)
			isolatedPath(t, tc.bins...)
			report := resolveGraph(root, tc.opts)
			if report == nil {
				t.Fatal("the run said nothing about the graph at all")
			}
			if report.Action != graphSkipped {
				t.Errorf("action = %q, want %q", report.Action, graphSkipped)
			}
			if !strings.Contains(report.Reason, tc.says) {
				t.Errorf("reason = %q, want it to mention %q", report.Reason, tc.says)
			}
			// No binary means no usage block either: guidance naming a command the
			// machine cannot run is worse than none, because an agent that tries it
			// and watches it fail discounts the whole file.
			if len(report.Blocks) != 0 {
				t.Errorf("a run with no CodeGraph wrote a usage block anyway: %+v", report.Blocks)
			}
		})
	}

	if report := resolveGraph(t.TempDir(), graphOptions{disabled: true}); report != nil {
		t.Errorf("a disabled step still reported %+v", report)
	}
}

// --no-graph and --no-rtk drop their step entirely rather than reporting a skipped
// one: a flag that turned into a report would be scc explaining a decision the
// user already made.
func TestTheOptOutsRemoveTheirStepEntirely(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "claude")

	cmd := launchJSON(t, "launch", "claude", "--root", root, "--json", "--no-graph", "--no-rtk")
	if cmd.Graph != nil {
		t.Errorf("--no-graph still reported a graph step: %+v", cmd.Graph)
	}
	if cmd.RTK != nil {
		t.Errorf("--no-rtk still reported an RTK step: %+v", cmd.RTK)
	}
	// Headroom is off by default, so it is absent without being asked to be.
	if cmd.Headroom != nil && cmd.Headroom.Wrapping {
		t.Errorf("a default run wrapped with Headroom: %+v", cmd.Headroom)
	}
}
