// Package devcontainer wires the Dev Containers CLI into scc, as the sandbox for
// the platform ai-jail cannot reach.
//
// It exists for one reason and it is worth stating plainly: **ai-jail has no
// Windows backend and is unlikely to get one.** It stands on Linux namespaces and
// Apple's sandbox interface, so a Windows user running `scc launch` today gets an
// agent with the run of their home directory — the `~/.aws` and `~/.ssh` that the
// jail exists to hide. WSL2 remains the better answer where it is available, and
// scc still says so; this is what the rest get.
//
// **A container is a weaker boundary than the jail, and scc does not pretend
// otherwise.** ai-jail hides `.ssh`, `.aws` and `.gnupg` outright and keeps no
// escape hatch. A dev container isolates the filesystem at the Docker boundary,
// but Anthropic's own documentation warns that under `--dangerously-skip-permissions`
// it "does not prevent a malicious project from exfiltrating anything accessible
// inside the container, including the Claude Code credentials stored in
// `~/.claude`" — which are mounted in precisely so a rebuild does not log you out.
// So this is an alternative, not an equal, and every message here says which one
// the user is getting.
//
// scc composes command lines and nothing else, the same contract every other
// integration package here keeps. The image, the features and the firewall are the
// project's to own — scaffolded once as a starting point and never updated,
// because a Dockerfile is a build environment and that belongs to whoever has to
// debug it at three in the morning.
package devcontainer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Bin is the executable's name, as it appears on PATH.
const Bin = "devcontainer"

// Pkg is the npm package that carries the CLI.
const Pkg = "@devcontainers/cli"

// Docs is where the spec lives, for the error that has to send somebody somewhere.
const Docs = "https://containers.dev"

// Dir is the directory the configuration lives in, at the project root.
const Dir = ".devcontainer"

// Config is the file whose presence means this project has one.
const Config = "devcontainer.json"

// Preferred reports whether this platform should reach for a dev container rather
// than for ai-jail.
//
// Windows alone, and by elimination rather than by preference: everywhere else has
// a kernel sandbox that is cheaper, stronger and not coupled to Docker. A Linux
// user with a `.devcontainer/` is free to use it — scc simply does not choose it
// for them.
func Preferred() bool { return runtime.GOOS == "windows" }

// Present reports whether root carries a dev container configuration.
func Present(root string) bool {
	_, err := os.Stat(filepath.Join(root, Dir, Config))
	return err == nil
}

// Path reports where the devcontainer binary is, and whether it is on PATH at all.
func Path() (string, bool) {
	p, err := exec.LookPath(Bin)
	if err != nil {
		return "", false
	}
	return p, true
}

// Version reports what the CLI says about itself, or "" when it cannot answer.
// Advisory only: printed, never branched on.
func Version(bin string) string {
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// DockerReady reports whether a daemon is actually there to build into.
//
// Checked separately from the CLI because the two fail differently and the user
// can only fix one of them at a time: `devcontainer` on PATH with no daemon
// running is the common Windows morning, and "install the CLI" is the wrong thing
// to tell somebody who needs to start Docker Desktop.
func DockerReady() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	return exec.Command("docker", "info").Run() == nil
}

// UpArgs builds or starts the container for this workspace.
//
// `--workspace-folder` rather than a working directory, because the CLI resolves
// the configuration from that flag and a process started elsewhere would build
// the wrong project.
func UpArgs(root string) []string {
	return []string{"up", "--workspace-folder", root}
}

// ExecArgs runs the agent inside the container that `up` left running.
func ExecArgs(root, bin string, rest []string) []string {
	args := []string{"exec", "--workspace-folder", root, bin}
	return append(args, rest...)
}

// Run starts the CLI with the given streams and returns its exit code. An error is
// returned only when the process could not be started at all.
func Run(bin, dir string, args []string, stdout, stderr io.Writer) (int, error) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
}

// Available reports whether npm is here to install the CLI with.
func Available() bool {
	_, err := exec.LookPath("npm")
	return err == nil
}

// InstallCmd is the one command that puts the CLI on PATH.
func InstallCmd() string { return "npm install -g " + Pkg }

// InstallHint is what to tell somebody who has neither the CLI nor npm.
func InstallHint() string {
	return fmt.Sprintf("%s (needs npm and a running Docker daemon — %s)", InstallCmd(), Docs)
}

// Install runs the install with its output where the caller says.
func Install(stdout, stderr io.Writer) error {
	npm, err := exec.LookPath("npm")
	if err != nil {
		return fmt.Errorf("npm is not on PATH, so %s cannot be installed", Bin)
	}
	cmd := exec.Command(npm, "install", "-g", Pkg)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", InstallCmd(), err)
	}
	return nil
}
