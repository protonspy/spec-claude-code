// Package headroom wires Headroom — the context-compression layer that sits
// between a coding agent and its model — into the way scc starts a harness.
//
// Three things, kept apart because they fail differently: naming the agent slug
// `headroom wrap` takes for a given harness, which is pure data; finding the
// binary, which is a PATH lookup; and installing it, which needs a Python
// toolchain and a network and can take minutes.
//
// scc only ever composes the command line. The proxy Headroom starts, the config
// it injects, and the agent's own lifetime are Headroom's, and scc deliberately
// knows nothing about them — `headroom wrap claude` is one process to launch, not
// a protocol to implement.
package headroom

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/paths"
)

// Repo is where Headroom is developed, for the error that has to send somebody
// somewhere.
const Repo = "https://github.com/headroomlabs-ai/headroom"

// Bin is the executable's name, as it appears on PATH.
const Bin = "headroom"

// Dist is the distribution that carries the CLI.
//
// The [all] extra is load-bearing: it is what pulls in the proxy and the
// code-aware compressors alongside the library. Installing the bare package
// would leave `headroom` either absent or unable to wrap anything.
const Dist = "headroom-ai[all]"

// agents maps a paths.Harness ID to the slug `headroom wrap` takes.
//
// A map here rather than a field on paths.Harness, and the reason is ownership:
// which agents Headroom wraps is Headroom's vocabulary, published in Headroom's
// README and changing on Headroom's schedule. Putting those slugs in the on-disk
// profile would make every harness scc adds look like it already had an answer
// here. The three agree with scc's own IDs today; this indirection is what keeps
// that a coincidence rather than a coupling.
var agents = map[string]string{
	paths.Claude.ID:   "claude",
	paths.Codex.ID:    "codex",
	paths.OpenCode.ID: "opencode",
}

// Agent reports the slug for h, and whether Headroom wraps that harness at all.
// A false here is a fact about Headroom's support, not an error: the caller's
// answer is to start the agent unwrapped.
func Agent(h paths.Harness) (string, bool) {
	slug, ok := agents[h.ID]
	return slug, ok
}

// WrapArgs is the whole argument vector: `wrap <agent>`, then whatever the caller
// is passing straight through to the agent itself.
func WrapArgs(agent string, rest []string) []string {
	args := make([]string, 0, len(rest)+2)
	args = append(args, "wrap", agent)
	return append(args, rest...)
}

// Path reports where the headroom binary is, and whether it is on PATH at all.
func Path() (string, bool) {
	p, err := exec.LookPath(Bin)
	if err != nil {
		return "", false
	}
	return p, true
}

// Version reports what `headroom --version` says, or "" when the binary cannot
// answer. Advisory only: it is printed, never branched on, so a build that words
// its version differently costs nothing.
func Version(bin string) string {
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Installer is one way to get the CLI onto PATH.
type Installer struct {
	// Prog is the program that does the installing, looked up on PATH.
	Prog string
	// Args is the argument vector passed to Prog, unquoted — exec takes no shell,
	// so the extras bracket needs no escaping here.
	Args []string
	// Cmd is the same command written the way a shell takes it, for printing. It
	// is not the argv form: `headroom-ai[all]` is a glob in zsh, so a line a human
	// copy-pastes has to carry the quotes that a line scc execs must not.
	Cmd string
}

// Installers are the ways to install the CLI, in preference order.
//
// uv first because it is what Headroom's own docs lead with and because it puts
// the tool in an isolated environment, which is the right default for a CLI that
// happens to be written in Python. pip second, for a machine that has no uv.
//
// npm is deliberately absent even though Headroom publishes there:
// `npm install headroom-ai` ships the TypeScript SDK and no CLI, so installing it
// would report success and leave `headroom` still missing.
func Installers() []Installer {
	return []Installer{
		{
			Prog: "uv",
			Args: []string{"tool", "install", "--python", "3.13", Dist},
			Cmd:  `uv tool install --python 3.13 "` + Dist + `"`,
		},
		{
			Prog: "pip",
			Args: []string{"install", Dist},
			Cmd:  `pip install "` + Dist + `"`,
		},
	}
}

// Available returns the first installer whose program is actually on this machine.
func Available() (Installer, bool) {
	for _, i := range Installers() {
		if _, err := exec.LookPath(i.Prog); err == nil {
			return i, true
		}
	}
	return Installer{}, false
}

// InstallHint is what to tell somebody who has to do it themselves: every way,
// in preference order, written the way a shell takes it.
func InstallHint() string {
	all := Installers()
	lines := make([]string, 0, len(all))
	for _, i := range all {
		lines = append(lines, i.Cmd)
	}
	return strings.Join(lines, "\n  or: ")
}

// Install runs i, streaming its output — resolving and building a Python
// distribution takes a while, and a silent command that long reads as a hang.
//
// A missing program is reported as itself rather than as a failed install: the
// user has to get uv or pip first, which is a different problem from an install
// that broke, and "install failed" would send them looking in the wrong place.
func Install(i Installer, stdout, stderr io.Writer) error {
	prog, err := exec.LookPath(i.Prog)
	if err != nil {
		return fmt.Errorf("%s is not on PATH; install it, then run: %s", i.Prog, i.Cmd)
	}
	cmd := exec.Command(prog, i.Args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", i.Cmd, err)
	}
	return nil
}
