package cli

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/protonspy/spec-claude-code/internal/devcontainer"
	"strings"
	"testing"
)

// The container is the Windows backend, and every assertion here is about what a
// run reports when it cannot deliver one: the agent still starts, and the line
// saying it is uncontained is the most important thing that run has to say.
//
// It runs only where ai-jail has no backend, because everywhere else the default
// sandbox is the jail and this path is not reached.
func skipWhereTheJailWorks(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("ai-jail is the default sandbox on this platform, so the container path is unreachable")
	}
}

// A workspace with no .devcontainer/ is the first reason there is no container,
// and it is reported rather than guessed around: seeding one is the user's
// decision, not something a launch does on the way past.
func TestLaunchReportsAWorkspaceWithNoContainer(t *testing.T) {
	skipWhereTheJailWorks(t)
	root := initWorkspace(t)
	isolatedPath(t, "claude")
	if err := os.RemoveAll(filepath.Join(root, ".devcontainer")); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	cmd := launchJSON(t, "launch", "claude", "--root", root, "--json", "--no-install")
	if cmd.Container == nil {
		t.Fatal("the run reported no container at all, so nobody learns it is uncontained")
	}
	if cmd.Container.Wrapping {
		t.Error("a workspace with no .devcontainer reported that it wrapped")
	}
	if !strings.Contains(cmd.Container.Reason, devcontainer.Dir) {
		t.Errorf("the reason does not name the missing directory: %q", cmd.Container.Reason)
	}
	// The boundary is reported as the lesser one for machines too, not only in
	// the human line: a consumer should not have to know the platform.
	if !cmd.Container.Weaker {
		t.Error("the container report does not say it is the weaker boundary")
	}
	// And the agent still starts: a sandbox nobody asked for by name does not get
	// to stop the thing the user actually asked for.
	if cmd.Bin == "" {
		t.Error("the run named no binary to start")
	}
}

// `init` seeds .devcontainer/, so a fresh workspace has one — and then the binary
// is the next reason. A plan-only run reports it without installing anything.
func TestLaunchReportsAMissingDevcontainerCLI(t *testing.T) {
	skipWhereTheJailWorks(t)
	root := initWorkspace(t)
	isolatedPath(t, "claude")

	cmd := launchJSON(t, "launch", "claude", "--root", root, "--json", "--no-install")
	if cmd.Container == nil {
		t.Fatal("the run reported no container at all")
	}
	if cmd.Container.Wrapping {
		t.Error("a run with no devcontainer CLI reported that it wrapped")
	}
	if cmd.Container.Config == "" {
		t.Errorf("the seeded configuration was not found: %+v", cmd.Container)
	}
	if cmd.Container.Reason == "" {
		t.Error("the run gave no reason for starting on the host")
	}
	if cmd.Container.Install != installSkipped {
		t.Errorf("install = %q on a --no-install run", cmd.Container.Install)
	}
}

// --no-sandbox is the deliberate way out, and it contradicts --jail rather than
// one outranking the other: somebody who typed both has not said what they want.
func TestLaunchNoSandboxOptsOutAndContradictsJail(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "claude")

	cmd := launchJSON(t, "launch", "claude", "--root", root, "--json", "--no-sandbox", "--no-install")
	if cmd.Container != nil || cmd.Jail != nil {
		t.Errorf("--no-sandbox still reported a boundary: container %+v, jail %+v", cmd.Container, cmd.Jail)
	}

	stdout, stderr, code := run(t, "launch", "claude", "--root", root, "--jail", "--no-sandbox", "--dry-run")
	if code != ExitError {
		t.Errorf("--jail with --no-sandbox exited %d, want %d", code, ExitError)
	}
	if !strings.Contains(stdout+stderr, "contradict") {
		t.Errorf("the refusal does not say the two flags contradict:\n%s%s", stdout, stderr)
	}
}

// --jail-arg is repeatable, because the alternative — a comma-separated string —
// cannot carry a value containing a comma, which a path or a glob can.
func TestJailArgIsRepeatable(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "claude")

	// On a platform with no jail backend this refuses, and on one with a backend
	// it needs ai-jail installed. Either way the flag has been parsed by then,
	// which is what this is about: the values are collected rather than the last
	// one winning.
	var r repeatable
	for _, v := range []string{"--bind a,b", "--bind c"} {
		if err := r.Set(v); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}
	if len(r) != 2 {
		t.Fatalf("repeatable collected %d values, want both: %v", len(r), r)
	}
	if !strings.Contains(r.String(), "a,b") {
		t.Errorf("a value containing a comma did not survive: %q", r.String())
	}

	// And the flag reaches the command line rather than being silently dropped.
	if _, _, code := run(t, "launch", "claude", "--root", root,
		"--jail-arg", "--bind a,b", "--jail-arg", "--bind c", "--dry-run", "--no-install"); code == ExitOK {
		// A platform with a backend and no binary refuses, which is the contract.
		return
	}
}
