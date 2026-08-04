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
	"regexp"
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

// WrapArgs is the whole argument vector: `wrap <agent>`, then scc's own options
// to wrap, then whatever is passing through to the agent itself.
//
// The two argument lists are separate parameters rather than one because they are
// not interchangeable, and the command line hides that. `headroom wrap` parses
// every flag it recognizes out of the tail and forwards only the rest to the
// agent, so an agent flag that collides with one of Headroom's — `--verbose`,
// which both Claude Code and `wrap` define — is silently eaten. Keeping the two
// apart here means scc's options go first, where a colliding pass-through
// argument still lands last and wins.
func WrapArgs(agent string, opts, rest []string) []string {
	args := make([]string, 0, len(opts)+len(rest)+2)
	args = append(args, "wrap", agent)
	args = append(args, opts...)
	return append(args, rest...)
}

// MCPMode says which of the MCP servers `headroom wrap` would register scc
// actually wants registered.
//
// scc names the intent; which flag expresses it is discovered from the binary
// (see WrapHelp). That split is the whole point: Headroom has already renamed
// this control once — `--no-serena` became `--code-memory none` — and a flag name
// compiled into scc would have turned that release into a launch that dies on
// "no such option" instead of one that starts an agent.
type MCPMode int

const (
	// MCPAll leaves Headroom's own defaults alone.
	MCPAll MCPMode = iota
	// MCPRetrieve keeps Headroom's retrieve tool, which its proxy needs to make
	// compression markers actionable, and drops the code-memory server it would
	// otherwise install into the user's agent config.
	MCPRetrieve
	// MCPNone asks for no MCP server at all, retrieve included.
	MCPNone
)

// ParseMCPMode reads the mode from the spelling a user types.
func ParseMCPMode(s string) (MCPMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "all":
		return MCPAll, nil
	case "retrieve":
		return MCPRetrieve, nil
	case "none":
		return MCPNone, nil
	default:
		return MCPAll, fmt.Errorf("unknown mcp mode %q: want all, retrieve, or none", s)
	}
}

func (m MCPMode) String() string {
	switch m {
	case MCPRetrieve:
		return "retrieve"
	case MCPNone:
		return "none"
	default:
		return "all"
	}
}

// longFlag matches an option as `--help` prints it. Deliberately loose: a flag
// named anywhere in the help text — in the options list or in an example — is a
// flag this build accepts, and that is the only question being asked.
var longFlag = regexp.MustCompile(`--[a-z0-9][a-z0-9-]*`)

// WrapHelp is what `headroom wrap <agent> --help` prints, or "" when the binary
// cannot answer. Advisory: an empty answer means scc passes no options rather
// than guessing at them.
func WrapHelp(bin, agent string) string {
	// Combined, because a build that routes help to stderr still answers the
	// question, and getting this wrong would silently disable every opt-out.
	out, err := exec.Command(bin, "wrap", agent, "--help").CombinedOutput()
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

// MCPOffArgs is how this build spells mode, given the flags it advertises.
//
// Empty is a legitimate answer, and the caller has to treat it as one: a build
// that offers no way to decline an MCP server has told scc that the answer is no,
// which is a thing to report rather than to force.
func MCPOffArgs(flags map[string]bool, mode MCPMode) []string {
	if mode == MCPAll {
		return nil
	}
	var args []string
	// The code-memory server, under whichever name this build knows it by. Newest
	// spelling first so a build that still accepts an old alias is driven by the
	// one its own help leads with.
	switch {
	case flags["--code-memory"]:
		args = append(args, "--code-memory", "none")
	case flags["--no-serena"]:
		args = append(args, "--no-serena")
	}
	// tokensave was a separate server in the builds between those two spellings,
	// with its own opt-out and no entry in --code-memory.
	if flags["--no-tokensave"] {
		args = append(args, "--no-tokensave")
	}
	if mode == MCPNone && flags["--no-mcp"] {
		args = append(args, "--no-mcp")
	}
	return args
}

// ContextToolOffArgs is how this build spells "set up no CLI context tool", or
// empty when it offers no way to say it.
//
// scc passes this by default, and the reason is a collision rather than a
// preference. Headroom's context-tool setup appends RTK guidance to
// `$PWD/CLAUDE.md` or `$PWD/AGENTS.md` — the same entry file `scc rtk` splices —
// behind its own marker pair, `<!-- headroom:rtk-instructions -->`. Neither
// marker is a substring of the other, so each tool's idempotency check passes and
// both append: an entry file carrying the same RTK instructions twice, which is
// pure wasted context in every request of the session.
//
// Headroom already gates that injection behind HEADROOM_RTK, so it is off unless
// asked for. Passing the flag anyway is what makes it off *here*: an environment
// that exports HEADROOM_RTK=1 for other reasons would otherwise turn every
// `scc launch` into a second copy of a block the workspace already has.
//
// --no-context-tool first: it is the primary spelling, and it covers lean-ctx as
// well as RTK. --no-rtk is the same option's older alias.
func ContextToolOffArgs(flags map[string]bool) []string {
	switch {
	case flags["--no-context-tool"]:
		return []string{"--no-context-tool"}
	case flags["--no-rtk"]:
		return []string{"--no-rtk"}
	default:
		return nil
	}
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
