// Package jail is the sandbox an agent runs inside: [ai-jail], Fábio Akita's
// wrapper over bubblewrap on Linux and sandbox-exec on macOS.
//
// It is the fifth integration package, on the same terms as rtk, headroom,
// codegraph and git — scc composes a command line for a binary somebody else
// ships and never reimplements what it does. That restraint matters more here than
// anywhere else in the tree: a sandbox is security-critical C and kernel interfaces
// (namespaces, Landlock, seccomp), and a half-copy of one is worse than none,
// because it produces the confidence of containment without the containment.
//
// # Why an agent wants one
//
// An agent needs filesystem access to do its job, and the same access lets it run
// `rm -rf`, read `~/.aws`, or ship a private key to a paste site — by accident, on a
// poisoned instruction in a file it read, or through a dependency it installed. This
// workspace's own methodology makes that sharper rather than softer: `autonomy: auto`
// means nobody is watching the step where noticing was still possible.
//
// # What scc decides, and what it does not
//
// scc passes exactly the flags that make an agent able to run at all — a network it
// can reach its model through, the credential state it authenticates with, and
// read-only mounts for the handful of binaries scc's own guidance tells it to use
// (see [FlagMap]) — and nothing else. Every other question (lockdown, denied paths,
// the browser, Docker) is *policy*, and policy belongs in ai-jail's own `~/.ai-jail`
// and `./.ai-jail`, which it reads by itself and scc never writes. A launcher that
// quietly loosened somebody's sandbox policy would be the worst kind of helpful.
//
// All three are read off `ai-jail --help` rather than compiled in, which is the
// lesson internal/headroom already paid for: a flag name hardcoded here turns a
// rename in somebody else's release into a launch that dies on "no such option".
//
// [ai-jail]: https://github.com/akitaonrails/ai-jail
package jail

import (
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// Repo is where ai-jail lives, printed whenever scc suggests installing it.
const Repo = "https://github.com/akitaonrails/ai-jail"

// Bin is the executable, as it appears on PATH.
const Bin = "ai-jail"

// Crate is the name ai-jail publishes under, which is what cargo installs.
const Crate = "ai-jail"

// Supported reports whether ai-jail runs on this platform at all.
//
// Linux (bubblewrap) and macOS (sandbox-exec) are the two backends it ships. There
// is no Windows backend and there is unlikely to be one: the sandbox is built out of
// Linux namespaces and Apple's sandbox interface, neither of which has a Windows
// equivalent ai-jail could stand on. WSL2 is the answer there, and it is a real one —
// scc inside WSL2 is scc on Linux.
func Supported() bool {
	return runtime.GOOS == "linux" || runtime.GOOS == "darwin"
}

// Backend names what would do the sandboxing here, for a report that says why.
func Backend() string {
	switch runtime.GOOS {
	case "linux":
		return "bubblewrap"
	case "darwin":
		return "sandbox-exec"
	default:
		return ""
	}
}

// Unsupported is the sentence a caller prints when Supported is false. It names the
// way out rather than only the wall: WSL2 is a supported Linux, not a workaround.
func Unsupported() string {
	return fmt.Sprintf("%s has no %s backend — it sandboxes with bubblewrap on Linux and sandbox-exec on macOS; on Windows, run scc inside WSL2",
		Bin, runtime.GOOS)
}

// Path reports where the ai-jail binary is, and whether it is on PATH at all.
func Path() (string, bool) {
	p, err := exec.LookPath(Bin)
	if err != nil {
		return "", false
	}
	return p, true
}

// Version reports what `ai-jail --version` says, or "" when the binary cannot
// answer. Advisory only: printed, never branched on.
func Version(bin string) string {
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// InstallCmd is the command Install runs, named before it runs so the user can see
// what they are agreeing to, and printed verbatim when cargo is missing and they
// have to do it themselves.
//
// cargo, of the several ways ai-jail is distributed — Homebrew, an AUR package, Nix,
// signed release archives — because it is the one that works the same on both
// supported platforms and needs no tap, channel, or manual download. The others are
// better if you already use them, which is why InstallHint names them.
func InstallCmd() string { return "cargo install --locked " + Crate }

// InstallHint is what to tell somebody who has no cargo: the platform-native routes,
// in the order they are worth trying.
func InstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew tap akitaonrails/tap && brew install " + Bin + ", or " + InstallCmd()
	default:
		return InstallCmd() + " (or the AUR, Nix, and signed release archives at " + Repo + ")"
	}
}

// Available says whether the toolchain Install needs is on PATH — the same question
// rtk answers about cargo, for the same reason: offering to install without asking
// it first is a prompt whose only possible answer is no.
func Available() bool {
	_, err := exec.LookPath("cargo")
	return err == nil
}

// Install builds and installs ai-jail with cargo, streaming the build's output: it
// takes minutes, and a silent command that long reads as a hang.
func Install(stdout, stderr io.Writer) error {
	cargo, err := exec.LookPath("cargo")
	if err != nil {
		return fmt.Errorf("cargo is not on PATH; install a Rust toolchain (https://rustup.rs), then run: %s", InstallCmd())
	}
	cmd := exec.Command(cargo, "install", "--locked", Crate)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", InstallCmd(), err)
	}
	return nil
}

var longFlag = regexp.MustCompile(`--[a-z0-9][a-z0-9-]*`)

// Help is what `ai-jail --help` prints, or "" when the binary cannot answer.
// Combined output, because a build that routes help to stderr still answers the
// question and reading only stdout would silently disable every flag below.
func Help(bin string) string {
	out, err := exec.Command(bin, "--help").CombinedOutput()
	if err != nil {
		return ""
	}
	return string(out)
}

// HelpFlags is the set of long options a help text advertises.
func HelpFlags(help string) map[string]bool {
	found := map[string]bool{}
	for _, f := range longFlag.FindAllString(help, -1) {
		found[f] = true
	}
	return found
}

// The two flags scc asks for, and the whole of what it asks for.
//
// ai-jail defaults both to off, which is the right default for a sandbox and the
// wrong one for a launcher: an agent with no network cannot reach the model it is,
// and one with no credential state cannot authenticate, so `ai-jail claude` with
// neither is a jail that starts nothing. They are function rather than policy, which
// is exactly why scc is willing to name these two and nothing else.
const (
	FlagNetwork = "--network"
	FlagState   = "--agent-state"
)

// Needed returns the flags this build advertises out of the two above, and the ones
// it does not.
//
// A missing flag is reported rather than substituted. If a future ai-jail spells
// network access differently, scc passing nothing means the user's own `.ai-jail`
// still governs and the agent may simply fail to connect — which is a visible,
// diagnosable outcome. Guessing at a replacement spelling is how a sandbox ends up
// opened by a tool that was only trying to help.
func Needed(flags map[string]bool) (have, missing []string) {
	for _, f := range []string{FlagNetwork, FlagState} {
		if flags[f] {
			have = append(have, f)
			continue
		}
		missing = append(missing, f)
	}
	return have, missing
}

// Args is the full command line: ai-jail's own options, then `--`, then the agent
// and everything meant for it.
//
// The terminator is not optional. ai-jail parses flags out of its tail the way
// `headroom wrap` does, so an agent argument that collides with one of ai-jail's —
// `--network` is plausible for any tool — would otherwise be eaten by the sandbox
// instead of reaching the agent. `--` ends that ambiguity for good.
//
// extra comes last among the options, so a flag the user passed deliberately wins
// over the two scc supplies.
func Args(opts, extra []string, bin string, rest []string) []string {
	args := make([]string, 0, len(opts)+len(extra)+len(rest)+2)
	args = append(args, opts...)
	args = append(args, extra...)
	args = append(args, "--", bin)
	return append(args, rest...)
}
