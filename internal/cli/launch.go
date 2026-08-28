package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/assets"
	"github.com/protonspy/spec-claude-code/internal/codegraph"
	"github.com/protonspy/spec-claude-code/internal/headroom"
	"github.com/protonspy/spec-claude-code/internal/jail"
	"github.com/protonspy/spec-claude-code/internal/mdblock"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/rtk"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// runLaunch starts a harness in this workspace, through Headroom's compression
// proxy when it can.
//
// It exists because "which agent, from which directory, with what in front of it"
// is a question scc already knows the answer to. The workspace walk finds the root
// whatever subdirectory the shell is in, the manifest says which harnesses were
// scaffolded, and the harness profile names the binary — so `scc launch` is one
// word where the alternative is remembering to cd first and to spell the wrapper
// right.
//
// What a bare `scc launch` does, and does not, is settled by one question: does
// this leave anything behind after the session ends?
//
// RTK is on by default because what it leaves behind is what the agent needs.
// Its usage block in the entry file is the whole point — an agent that has not
// read it does not prefix anything, so the binary alone buys nothing. And the
// step is bounded by that block: it runs when an entry file has none and does
// nothing when they all do, so it fires once per workspace rather than once per
// session, and never rewrites a block somebody already has. `--no-rtk` opts out.
//
// Headroom is off by default because what *it* leaves behind is not: `headroom
// wrap` registers MCP servers into the agent's own config, and those
// registrations outlive the session that made them — which is why Headroom ships
// `unwrap` at all. Compression for one session is not worth a config edit the
// user did not ask for and will not see. `--headroom` asks for it, and then
// --headroom-mcp=none is still the default so the wrap is the proxy and nothing
// else; `--headroom-mcp retrieve` hands back the tool that makes the proxy's
// compression markers actionable, and `--headroom-context-tool` lets Headroom
// append its own RTK guidance to the entry file — off because `scc rtk` owns
// that block here, behind markers Headroom's do not match.
//
// The agent's own exit code is passed straight through, which is the one place
// scc's 0/1/2 contract does not apply — and it has to be. A launcher that
// flattened the exit status of what it launched would be unusable in the scripts
// people actually write. scc's own failures, before anything is started, still
// report 1.
func runLaunch(args []string) int {
	// Split on `--` before the flag package sees it, so `scc launch claude --
	// --resume` can tell scc's flags from the agent's. Doing this by hand rather
	// than leaning on flag's own terminator is what keeps the harness name a
	// positional while everything after `--` stays untouched.
	own, passthrough := splitPassthrough(args)

	fs := flag.NewFlagSet("launch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	headroomFlag := fs.Bool("headroom", false, "start the agent behind Headroom's compression proxy (it registers MCP servers that outlive the session)")
	mcp := fs.String("headroom-mcp", headroom.MCPNone.String(),
		"which MCP servers Headroom may register: all | retrieve (its own only) | none")
	contextTool := fs.Bool("headroom-context-tool", false,
		"let Headroom set up its own CLI context tool (RTK or lean-ctx) and append its guidance to the entry file")
	jailFlag := fs.Bool("jail", false, "start the agent inside ai-jail's sandbox; refuses to start it outside one")
	var jailArgs repeatable
	fs.Var(&jailArgs, "jail-arg", "an extra `flag` for ai-jail itself, repeatable (policy otherwise lives in .ai-jail)")
	noGraph := fs.Bool("no-graph", false, "start the agent without building or refreshing the symbol graph")
	noRTK := fs.Bool("no-rtk", false, "start the agent without setting up RTK's binary or its usage block in the entry file")
	noInstall := fs.Bool("no-install", false, "never install anything; use ai-jail, Headroom, CodeGraph and RTK only if they are already on PATH")
	yes := fs.Bool("yes", false, "answer the install prompts with yes, for an unattended run")
	dryRun := fs.Bool("dry-run", false, "print the command this would run, and run nothing")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, own)
	if err != nil {
		return ExitError
	}
	mcpMode, err := headroom.ParseMCPMode(*mcp)
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}
	// Any --headroom-* flag is itself a request for Headroom. The alternative is
	// accepting a flag and then ignoring it, which is how somebody spends a session
	// believing they configured something.
	wantHeadroom := *headroomFlag
	fs.Visit(func(f *flag.Flag) {
		if strings.HasPrefix(f.Name, "headroom-") {
			wantHeadroom = true
		}
	})

	target, ok := resolveRoot(*root)
	if !ok {
		return ExitError
	}
	if !requireWorkspace(target) {
		return ExitError
	}

	harness, err := launchHarness(target, rest, *jsonOut)
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}

	// The jail is settled first, and it is the one thing here that refuses. Every
	// other integration degrades because it is an enhancement; a sandbox is a
	// containment boundary, and a launcher that quietly started the agent outside the
	// boundary somebody asked for would hand them the confidence of containment
	// without the containment. So a run that cannot jail starts nothing — and it
	// decides that before the graph is built or the entry file is touched, so a
	// refusal leaves the workspace exactly as it found it.
	var jailed *jailReport
	if *jailFlag {
		jailed = resolveJail(jailOptions{
			noInstall: *noInstall,
			yes:       *yes,
			quiet:     *jsonOut,
			extra:     jailArgs,
		})
		if jailed == nil {
			return ExitError
		}
	}

	// --json and --dry-run both report the plan and start nothing. For --json that
	// is not a shortcut but the only coherent answer: the agent inherits this
	// terminal and writes to the same stdout, so a launched session and a clean
	// JSON document on stdout cannot both exist.
	plan := *jsonOut || *dryRun
	opts := headroomOptions{
		disabled:    !wantHeadroom,
		noInstall:   *noInstall || plan,
		yes:         *yes,
		quiet:       *jsonOut,
		mcp:         mcpMode,
		contextTool: *contextTool,
	}

	cmd := launchCommand{Harness: harness.ID, Dir: target, Bin: harness.Bin, Args: passthrough}
	if hr := resolveHeadroom(harness, opts); hr != nil {
		cmd.Headroom = hr
		if hr.Wrapping {
			cmd.Bin = headroom.Bin
			cmd.Args = headroom.WrapArgs(hr.Agent, hr.Options, passthrough)
		}
	}
	cmd.Graph = resolveGraph(target, graphOptions{
		disabled:  *noGraph,
		noInstall: *noInstall,
		yes:       *yes,
		plan:      plan,
		quiet:     *jsonOut,
	})
	// RTK last of the three, because it is the only one whose *install* is a
	// decision — a Rust toolchain and a build that takes minutes — and a run the user
	// aborts at the Headroom or CodeGraph prompt should not already have edited their
	// entry file. Both it and CodeGraph write a usage block there, and both do so only
	// once their binary is known to be present: guidance naming a command the machine
	// cannot run is worse than no guidance, because it costs the file its credibility.
	cmd.RTK = resolveRTK(target, rtkLaunchOptions{
		disabled:  *noRTK,
		noInstall: *noInstall,
		yes:       *yes,
		plan:      plan,
		quiet:     *jsonOut,
	})

	// Outermost, after Headroom: the jail contains the whole session, wrapper
	// included. Anything Headroom registers from inside it lands in the sandbox's
	// own transient home rather than the real one, which is a change in what `wrap`
	// leaves behind and worth knowing before combining the two.
	//
	// Last of everything, rather than beside resolveJail where the refusal happens,
	// because the toolchain it maps in is not settled until the two steps above have
	// run: a launch that just installed rtk has to map the binary it installed, and
	// one composed before that would sandbox the agent away from it.
	if jailed != nil {
		jailToolchain(jailed, jail.HiddenRoot(), jailTools(), *jsonOut)
		opts := append(append([]string{}, jailed.Options...), jailed.mapArgs...)
		cmd.Args = jail.Args(opts, jailed.Extra, cmd.Bin, cmd.Args)
		cmd.Bin = jail.Bin
		cmd.Jail = jailed
	}
	if cmd.Args == nil {
		// A JSON consumer gets [] rather than null: the field is a command line,
		// and an empty one is still a list.
		cmd.Args = []string{}
	}

	if *jsonOut {
		return emitJSON(cmd)
	}
	if *dryRun {
		render.Info(cmd.String())
		return ExitOK
	}

	if _, err := exec.LookPath(cmd.Bin); err != nil {
		render.Err(fmt.Sprintf("%s is not on PATH", cmd.Bin))
		render.Detail(fmt.Sprintf("  install %s, or run `%s launch --dry-run` to see the command", harness.Label, prog()))
		return ExitError
	}
	render.Info(cmd.String())
	code, err := launchExec(cmd)
	if err != nil {
		render.Err(fmt.Sprintf("could not start %s: %v", cmd.Bin, err))
		return ExitError
	}
	return code
}

// launchCommand is both the frozen JSON shape and what the human line is printed
// from, so the two cannot describe different commands.
type launchCommand struct {
	Harness  string           `json:"harness"`
	Dir      string           `json:"dir"`
	Bin      string           `json:"bin"`
	Args     []string         `json:"args"`
	Jail     *jailReport      `json:"jail,omitempty"`
	Headroom *headroomReport  `json:"headroom,omitempty"`
	Graph    *graphReport     `json:"graph,omitempty"`
	RTK      *rtkLaunchReport `json:"rtk,omitempty"`
}

// String is the command as a person would type it. Not shell-quoted, because it
// is a status line rather than something to paste: scc execs the argv directly
// and no shell is ever involved.
func (c launchCommand) String() string {
	return strings.TrimSpace(c.Bin + " " + strings.Join(c.Args, " "))
}

// headroomReport says what happened on the Headroom side of a launch — reported
// on every run rather than only the ones that wrapped, because "started without
// compression" is exactly the outcome somebody would otherwise not notice.
type headroomReport struct {
	// Wrapping is whether this launch actually goes through `headroom wrap`.
	Wrapping bool `json:"wrapping"`
	// Agent is the slug Headroom knows this harness by.
	Agent   string `json:"agent,omitempty"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	// Install is what happened to the binary: present | installed | skipped |
	// failed. The same vocabulary `scc rtk` uses, for the same reason — it
	// describes the binary, which is a different question from what the command
	// ended up doing.
	Install string `json:"install"`
	// MCP is the mode asked for: all | retrieve | none.
	MCP string `json:"mcp"`
	// ContextTool is whether Headroom was left free to set up its own CLI
	// context tool — RTK or lean-ctx — and append its guidance to the entry file.
	ContextTool bool `json:"context_tool"`
	// Options is how this Headroom build spells everything above, reported apart
	// from the rest of the command line because scc put it there and the user did
	// not. Empty — with a warning on the human path — when the build advertises no
	// way to decline what was declined.
	Options []string `json:"options,omitempty"`
	// Reason names why a launch is not wrapping, for the run where that is a
	// surprise. Empty when it is.
	Reason string `json:"reason,omitempty"`
}

type headroomOptions struct {
	disabled  bool
	noInstall bool
	yes       bool
	quiet     bool
	mcp       headroom.MCPMode
	// contextTool lets Headroom set up RTK or lean-ctx and write its guidance
	// into the entry file. Off by default: `scc rtk` owns that block here.
	contextTool bool
}

// resolveHeadroom decides whether this launch goes through Headroom, installing
// it first if the user says so. It returns nil only when the user asked for no
// Headroom at all — every other outcome is reportable.
//
// Nothing here returns an exit code, and that is the design: Headroom is an
// enhancement, so every way of not getting it degrades to starting the agent bare
// rather than to failing. A launch that refused to run because a compression
// proxy was missing would be scc putting its own preference above the thing the
// user actually asked for.
func resolveHeadroom(h paths.Harness, opts headroomOptions) *headroomReport {
	if opts.disabled {
		return nil
	}

	agent, wraps := headroom.Agent(h)
	if !wraps {
		return &headroomReport{Install: installSkipped, Reason: "Headroom does not wrap " + h.Label}
	}
	report := &headroomReport{Agent: agent, MCP: opts.mcp.String(), ContextTool: opts.contextTool}

	if p, ok := headroom.Path(); ok {
		report.Path, report.Version, report.Install = p, headroom.Version(p), installPresent
		wrapWith(report, opts)
		return report
	}

	report.Install = installSkipped
	installer, available := headroom.Available()
	switch {
	case opts.noInstall:
		report.Reason = headroom.Bin + " is not on PATH"
	case !available:
		report.Reason = fmt.Sprintf("neither uv nor pip is on PATH, so %s cannot be installed", headroom.Bin)
	case opts.yes:
		// Asked for by flag; no question to put.
	case opts.quiet || !interactive():
		// Unattended. Installing a Python distribution without being asked, in a
		// CI job or under an agent, is not a decision scc gets to make silently.
		report.Reason = fmt.Sprintf("%s is not on PATH, and nobody is here to answer the install prompt", headroom.Bin)
	default:
		render.Warn(fmt.Sprintf("%s is not on PATH — %s compresses the agent's context before it reaches the model", headroom.Bin, headroom.Bin))
		render.Detail("  " + headroom.Repo)
		if !confirmInstall(promptIn, fmt.Sprintf("Install it now with `%s`?", installer.Cmd)) {
			report.Reason = "install declined"
		}
	}
	if report.Reason != "" {
		warnUnwrapped(report, opts)
		return report
	}

	render.Info(fmt.Sprintf("installing %s: %s — this takes a few minutes", headroom.Bin, installer.Cmd))
	// The installer's own output goes to stderr in both streams when the caller is
	// emitting JSON, because stdout carries the document and nothing else.
	out := os.Stdout
	if opts.quiet {
		out = os.Stderr
	}
	if err := headroom.Install(installer, out, os.Stderr); err != nil {
		report.Install, report.Reason = installFailed, err.Error()
		warnUnwrapped(report, opts)
		return report
	}
	p, ok := headroom.Path()
	if !ok {
		report.Install = installFailed
		report.Reason = fmt.Sprintf("%s reported success but %s is still not on PATH", installer.Prog, headroom.Bin)
		warnUnwrapped(report, opts)
		return report
	}
	report.Path, report.Version, report.Install = p, headroom.Version(p), installInstalled
	render.OK(strings.TrimSpace(headroom.Bin + " installed: " + p + " " + report.Version))
	wrapWith(report, opts)
	return report
}

// wrapWith settles the wrap: this launch is going through Headroom, and these are
// the options it goes through with.
//
// The opt-out flags are read off `headroom wrap <agent> --help` rather than
// compiled into scc, because they are Headroom's vocabulary and it has already
// renamed this one once — `--no-serena` became `--code-memory none`. A name
// hardcoded here would have turned that release into a launch that dies on "no
// such option", which is a strictly worse outcome than a launch that registers
// one MCP server too many.
func wrapWith(report *headroomReport, opts headroomOptions) {
	report.Wrapping = true
	if opts.mcp == headroom.MCPAll && opts.contextTool {
		return
	}
	flags := headroom.HelpFlags(headroom.WrapHelp(report.Path, report.Agent))

	declined, got := 0, 0
	if opts.mcp != headroom.MCPAll {
		declined++
		if args := headroom.MCPOffArgs(flags, opts.mcp); len(args) > 0 {
			report.Options, got = append(report.Options, args...), got+1
		}
	}
	if !opts.contextTool {
		declined++
		if args := headroom.ContextToolOffArgs(flags); len(args) > 0 {
			report.Options, got = append(report.Options, args...), got+1
		}
	}

	if got == declined || opts.quiet {
		return
	}
	// Something was declined and this build offers no way to decline it. Say so:
	// the alternative is a user who set the flag, saw no error, and assumes it
	// took.
	render.Warn(fmt.Sprintf("this %s build advertises no way to decline everything scc asked it to; run it with --help to see what it takes", headroom.Bin))
	render.Detail("  `" + headroom.Bin + " wrap " + report.Agent + " --help`, and `" + headroom.Bin + " unwrap " + report.Agent + "` to undo what it registered")
}

// rtkLaunchReport says what happened to RTK on the way to starting the agent. It is
// deliberately not cli.rtkReport: that one is `scc rtk`'s frozen shape, describing a
// run of that command, and a launch answers a narrower question — is the binary there,
// and does the entry file carry the block.
type rtkLaunchReport struct {
	// Install is what happened to the binary, in the same vocabulary the other two
	// integrations use: present | installed | skipped | failed.
	Install string `json:"install"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	// Block is what happened to the usage block in the entry file: added | present |
	// replaced | missing | skipped.
	Block string `json:"block"`
	// Reason names why RTK was not set up, for the run where that is a surprise.
	Reason string `json:"reason,omitempty"`
}

type rtkLaunchOptions struct {
	disabled  bool
	noInstall bool
	yes       bool
	plan      bool
	quiet     bool
}

// resolveRTK makes sure the entry file the agent is about to load actually tells it
// about RTK, and that the binary that guidance names is there.
//
// The trigger is the block, not the binary: this runs when an entry file carries no
// RTK block, and does nothing at all when every one of them already does. That
// bound is what lets it be the default. A workspace wired on a previous launch pays
// nothing at the top of the next session — no read of substance, no cargo prompt,
// and above all no edit — while a fresh workspace gets the one thing that makes RTK
// work at all, since an agent that never read the block never types the prefix.
//
// It also splices with keep, so a block that is already there is left exactly as it
// is even when scc ships a different one. Replacing somebody's block is a real
// decision with a real trade-off, and `scc rtk` is where it is made deliberately;
// starting an agent is not the moment to make it as a side effect.
//
// It degrades the way the other two do: RTK is an enhancement, so a missing cargo, a
// declined install, or a failed build all end in the agent starting anyway.
func resolveRTK(root string, opts rtkLaunchOptions) *rtkLaunchReport {
	if opts.disabled {
		return nil
	}
	report := &rtkLaunchReport{Install: installSkipped, Block: "skipped"}

	if rtkBlockPresent(root) {
		report.Block = string(mdblock.Present)
		p, ok := rtk.Path()
		if ok {
			report.Install, report.Path, report.Version = installPresent, p, rtk.Version(p)
			return report
		}
		// The file tells the agent to prefix every command with a binary this machine
		// does not have. Said once, and not acted on: the block is wired, so this is a
		// PATH problem on the user's side rather than something for scc to fix mid-launch.
		report.Reason = rtk.Bin + " is not on PATH, and the entry file already tells the agent to use it"
		if !opts.quiet && !opts.plan {
			render.Warn(report.Reason)
			render.Detail("  install it with: " + rtk.InstallCmd())
		}
		return report
	}

	if _, ok := rtk.Path(); !ok {
		switch {
		case opts.noInstall || opts.plan:
			report.Reason = rtk.Bin + " is not on PATH"
		case !rtk.Available():
			report.Reason = fmt.Sprintf("cargo is not on PATH, so %s cannot be built", rtk.Bin)
		case opts.yes:
			// Asked for by flag; no question to put.
		case opts.quiet || !interactive():
			report.Reason = fmt.Sprintf("%s is not on PATH, and nobody is here to answer the install prompt", rtk.Bin)
		default:
			render.Warn(fmt.Sprintf("%s is not on PATH — it filters command output before it reaches the model", rtk.Bin))
			render.Detail("  " + rtk.Repo)
			// The question names the file, because consenting to an install is not
			// consenting to an edit and the user cannot see the second one coming.
			if !confirmInstall(promptIn, fmt.Sprintf("Build it with `%s` and add its usage block to the entry file?", rtk.InstallCmd())) {
				report.Reason = "install declined"
			}
		}
		if report.Reason != "" {
			if !opts.quiet && !opts.plan {
				render.Warn("starting without " + rtk.Bin + ": " + report.Reason)
			}
			return report
		}
	}

	// applyRTK owns both halves and is what `scc rtk` runs, so a launch cannot drift
	// from the command. noInstall is false here only because the prompt above already
	// settled it — this call is what actually builds the binary and writes the block.
	//
	// check is how a plan-only run reports the splice without performing it: --json
	// has to leave stdout clean for the document, and a --dry-run that edited the
	// entry file would be the one flag nobody expects to change anything doing
	// exactly that.
	sub, _ := applyRTK(root, rtkOptions{check: opts.plan, keep: true, quiet: opts.quiet})
	report.Install, report.Path, report.Version = sub.Install, sub.Path, sub.Version
	if sub.Error != "" {
		report.Reason = sub.Error
	}
	if len(sub.Files) > 0 {
		report.Block = sub.Files[0].Action
	}
	return report
}

// rtkBlockPresent reports whether every entry file in this workspace already carries
// an RTK block — the question `scc launch` decides its whole RTK step on.
//
// Every, not any: a workspace scaffolded for two harnesses has two entry files, and
// one of them being wired says nothing about the other. A workspace with no entry
// file at all answers no, so the run goes down the normal path and reports the file
// as missing rather than quietly calling an absent file done.
func rtkBlockPresent(root string) bool {
	entries := entryFiles(root)
	if len(entries) == 0 {
		return false
	}
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join(root, entry))
		if err != nil || rtk.Markers.Block(string(raw)) == "" {
			return false
		}
	}
	return true
}

// graphReport says what happened to the symbol graph on the way to starting the
// agent.
type graphReport struct {
	// Action is what scc did: built | synced | current | skipped | failed.
	Action string `json:"action"`
	// Indexed is whether the workspace had a graph before this launch.
	Indexed bool   `json:"indexed"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	// Blocks is what happened to the CodeGraph usage block in each entry file:
	// added | present | replaced | missing. Empty when nothing was written, which
	// is every run where the binary is not there — a block telling the agent to run
	// `scc graph explore` in a workspace with no CodeGraph is guidance that fails on
	// first use, and an agent that has been lied to once discounts the whole file.
	Blocks []blockFile `json:"blocks,omitempty"`
	// Reason names why nothing was built, for the run where that is a surprise.
	Reason string `json:"reason,omitempty"`
}

// The values graphReport.Action takes.
const (
	graphBuilt   = "built"
	graphSynced  = "synced"
	graphSkipped = "skipped"
	graphFailed  = "failed"
)

type graphOptions struct {
	disabled  bool
	noInstall bool
	yes       bool
	plan      bool
	quiet     bool
}

// resolveGraph brings the workspace's symbol graph up to date before the agent
// starts, which is the only moment where doing so is free: the session is about
// to begin, nobody is waiting on a prompt yet, and the alternative is an agent
// whose first graph query answers out of an index from last week.
//
// It degrades exactly the way the Headroom path does, and for the same reason. A
// graph is an enhancement — the agent can still read files — so a missing binary,
// a declined install, or a failed index all end in the agent starting anyway with
// a line saying what happened. `scc launch` that refused to run because an index
// was stale would be scc putting its own tidiness above the thing the user asked
// for.
func resolveGraph(root string, opts graphOptions) *graphReport {
	if opts.disabled {
		return nil
	}
	report := &graphReport{Indexed: codegraph.Indexed(root)}

	bin, ok := codegraph.Path()
	if !ok {
		report.Action = graphSkipped
		installer, available := codegraph.Available()
		switch {
		case opts.noInstall || opts.plan:
			report.Reason = codegraph.Bin + " is not on PATH"
		case !available:
			report.Reason = fmt.Sprintf("npm is not on PATH, so %s cannot be installed", codegraph.Bin)
		case opts.yes:
			// Asked for by flag; no question to put.
		case opts.quiet || !interactive():
			report.Reason = fmt.Sprintf("%s is not on PATH, and nobody is here to answer the install prompt", codegraph.Bin)
		default:
			render.Warn(fmt.Sprintf("%s is not on PATH — it gives the agent a symbol graph instead of file-by-file reading", codegraph.Bin))
			render.Detail("  " + codegraph.Repo)
			if !confirmInstall(promptIn, fmt.Sprintf("Install it now with `%s`?", installer.Cmd)) {
				report.Reason = "install declined"
			}
		}
		if report.Reason != "" {
			warnNoGraph(report, opts)
			return report
		}
		render.Info(fmt.Sprintf("installing %s: %s", codegraph.Bin, installer.Cmd))
		out := os.Stdout
		if opts.quiet {
			out = os.Stderr
		}
		if err := codegraph.Install(installer, out, os.Stderr); err != nil {
			report.Action, report.Reason = graphFailed, err.Error()
			warnNoGraph(report, opts)
			return report
		}
		if bin, ok = codegraph.Path(); !ok {
			report.Action = graphFailed
			report.Reason = fmt.Sprintf("%s reported success but %s is still not on PATH", installer.Prog, codegraph.Bin)
			warnNoGraph(report, opts)
			return report
		}
	}
	report.Path, report.Version = bin, codegraph.Version(bin)

	// A plan-only run reports what it would do and indexes nothing: --json has to
	// leave stdout clean for the document, and --dry-run means what it says. It
	// writes no block either — --dry-run that edited the entry file would be the one
	// flag nobody expects to change anything doing exactly that.
	if opts.plan {
		report.Action = graphSkipped
		report.Reason = "plan-only run"
		return report
	}

	// The block before the index rather than after it, so a failed index still
	// leaves the agent knowing the command that rebuilds one.
	report.Blocks = spliceGraphBlock(root, opts)

	args, action, doing := codegraph.InitArgs(), graphBuilt, "building the symbol graph — the first index takes a while"
	if report.Indexed {
		args, action, doing = codegraph.SyncArgs(), graphSynced, "refreshing the symbol graph"
	}
	if !opts.quiet {
		render.Info(codegraph.Bin + " " + doing)
	}
	// The indexer's own output goes to stderr in both streams when the caller is
	// emitting JSON, because stdout carries the document and nothing else.
	out := os.Stdout
	if opts.quiet {
		out = os.Stderr
	}
	code, err := codegraph.Run(bin, root, args, out, os.Stderr)
	switch {
	case err != nil:
		report.Action, report.Reason = graphFailed, err.Error()
	case code != 0:
		report.Action = graphFailed
		report.Reason = fmt.Sprintf("%s %s exited %d", codegraph.Bin, args[0], code)
	default:
		report.Action = action
		return report
	}
	warnNoGraph(report, opts)
	return report
}

// spliceGraphBlock keeps the CodeGraph usage block current in every entry file this
// workspace has, and returns what that took.
//
// It runs at launch and nowhere else, which is the same reasoning that puts the
// index here: this is the moment the guidance is about to be read, and the binary
// has just been proven to exist. `scc init` cannot write it — CodeGraph may not be
// installed then, and often is not — and a block promising `scc graph explore` in a
// workspace where that command fails teaches the agent to distrust the whole file.
//
// A write failure is not fatal. The agent is about to start either way, and the
// block is an enhancement exactly as the graph is; refusing to launch over a
// Markdown splice would be scc putting its own tidiness above what was asked for.
func spliceGraphBlock(root string, opts graphOptions) []blockFile {
	block, err := assets.CodeGraphBlock()
	if err != nil {
		if !opts.quiet {
			render.Warn("could not read the CodeGraph usage block: " + err.Error())
		}
		return nil
	}
	var out []blockFile
	for _, entry := range entryFiles(root) {
		// keep=false: scc owns this block behind its own markers, so replacing it is
		// how a workspace picks up a newer one. There is no other author to defer to.
		file, err := spliceEntryBlock(root, entry, codegraph.Markers, block, false, false)
		if err != nil {
			if !opts.quiet {
				render.Warn(fmt.Sprintf("%s: %v", entry, err))
			}
			continue
		}
		out = append(out, file)
		if opts.quiet || file.Action == string(mdblock.Present) {
			continue
		}
		if file.Action == blockMissing {
			render.Warn(fmt.Sprintf("%s does not exist; run `%s init` first", entry, prog()))
			continue
		}
		render.OK(fmt.Sprintf("%s — CodeGraph block %s", entry, file.Action))
	}
	return out
}

// warnNoGraph says, once, why the agent is starting without a fresh graph.
func warnNoGraph(report *graphReport, opts graphOptions) {
	if opts.quiet {
		return
	}
	render.Warn(fmt.Sprintf("starting without a fresh symbol graph: %s", report.Reason))
	if report.Action != graphFailed {
		render.Detail("  install it with: " + codegraph.InstallHint())
	}
}

// warnUnwrapped says, once, why the agent is starting without compression. It is
// a warning rather than a status line because the run is about to do less than
// the user asked for, and silence there is how somebody spends a month wondering
// why Headroom never seemed to help.
func warnUnwrapped(report *headroomReport, opts headroomOptions) {
	if opts.quiet {
		return
	}
	render.Warn(fmt.Sprintf("starting without %s: %s", headroom.Bin, report.Reason))
	if report.Install != installFailed {
		render.Detail("  install it with: " + headroom.InstallHint())
	}
}

// launchHarness picks which harness to start.
//
// An explicit name wins, and must be one this workspace was actually scaffolded
// for — starting Codex in a Claude-only repo would hand the user an agent with
// none of the methodology loaded, which is the exact failure scc exists to
// prevent, and it would do it while looking like it worked. Otherwise: the only
// harness here, or a picker when a person is at the terminal, or an error naming
// the choices when nobody is.
func launchHarness(root string, positionals []string, jsonOut bool) (paths.Harness, error) {
	here := workspace.Harnesses(root)

	if len(positionals) > 1 {
		return paths.Harness{}, fmt.Errorf("expected at most one harness name, got %d: %s",
			len(positionals), strings.Join(positionals, " "))
	}
	if len(positionals) == 1 {
		h, err := paths.ParseHarness(strings.ToLower(positionals[0]))
		if err != nil {
			return paths.Harness{}, err
		}
		for _, in := range here {
			if in.ID == h.ID {
				return h, nil
			}
		}
		return paths.Harness{}, fmt.Errorf("%s is not scaffolded here (found %s); run `%s init --%s` first",
			h.ID, harnessIDs(here), prog(), h.ID)
	}

	switch len(here) {
	case 1:
		return here[0], nil
	case 0:
		// requireWorkspace has already run, so this is unreachable through the CLI.
		return paths.Harness{}, fmt.Errorf("no harness is scaffolded here; run `%s init` first", prog())
	default:
		if jsonOut || !interactive() {
			return paths.Harness{}, fmt.Errorf("this workspace has %s; name the one to start, e.g. `%s launch %s`",
				harnessIDs(here), prog(), here[0].ID)
		}
		return promptHarness(promptIn, "Which harness do you want to start?", here), nil
	}
}

// harnessIDs lists a set for an error message, in the order paths declares them.
func harnessIDs(all []paths.Harness) string {
	ids := make([]string, 0, len(all))
	for _, h := range all {
		ids = append(ids, h.ID)
	}
	return strings.Join(ids, ", ")
}

// splitPassthrough divides scc's own arguments from the agent's at the first bare
// `--`. Everything after it is passed through untouched, so `scc launch claude --
// --dangerously-skip-permissions` reaches Claude Code rather than being rejected
// here as an unknown flag.
//
// "Untouched by scc" is the whole promise, and it is worth being precise about
// what it is not: when the launch is wrapped, those arguments land after `wrap
// <agent>`, and `headroom wrap` takes every flag it recognizes for itself before
// forwarding the rest. A pass-through argument that collides with one of
// Headroom's — `--verbose` is defined by both — is eaten there, not here. A second
// terminator forces the issue: `scc launch claude -- -- -p`.
func splitPassthrough(args []string) (own, rest []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

// launchExec starts the resolved command with this terminal attached and returns
// its exit code.
//
// A child process rather than an exec(2) replacement, because Windows is a
// first-class target and has no execve: one code path that behaves identically
// everywhere beats a faster one on two platforms out of three. It is a package
// var so the tests can drive the whole command without starting a real agent.
var launchExec = func(cmd launchCommand) (int, error) {
	c := exec.Command(cmd.Bin, cmd.Args...)
	c.Dir = cmd.Dir
	// The agent owns this terminal for the length of its session: it is
	// interactive, and anything scc interposed here would break its rendering.
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), nil
		}
		return ExitError, err
	}
	return ExitOK, nil
}

// repeatable is a flag that may be given more than once, collecting its values.
// The flag package has no such type, and the alternative — a comma-separated
// string — cannot carry a value containing a comma, which a path or a glob can.
type repeatable []string

func (r *repeatable) String() string { return strings.Join(*r, " ") }

func (r *repeatable) Set(v string) error {
	*r = append(*r, v)
	return nil
}

// jailReport says what happened on the sandbox side of a launch.
//
// Unlike the other three it never describes a launch that went ahead without what
// was asked for: when this is present at all, the agent is inside the sandbox.
// There is no "started unjailed" outcome to report, because that outcome does not
// exist — see resolveJail.
type jailReport struct {
	// Wrapping is always true when the report exists, and it is written down
	// anyway: a JSON consumer checking `jail.wrapping` should get the same answer
	// as one checking whether the key is there.
	Wrapping bool   `json:"wrapping"`
	Path     string `json:"path,omitempty"`
	Version  string `json:"version,omitempty"`
	Backend  string `json:"backend,omitempty"`
	// Install is what happened to the binary: present | installed. The vocabulary
	// the other integrations use, minus the outcomes that end in starting anyway.
	Install string `json:"install"`
	// Options is what scc asked for, kept apart from Extra because scc put these
	// there and the user did not.
	Options []string `json:"options,omitempty"`
	// Maps is the toolchain scc mounted back in read-only: the binaries its own
	// guidance names, which the sandbox's private home would otherwise take away
	// along with the rest of $HOME.
	Maps []jail.Mapping `json:"maps,omitempty"`
	// Missing names anything scc asked for that this build does not advertise.
	// Reported rather than substituted: a sandbox opened by a guess is worse than
	// an agent that fails to connect.
	Missing []string `json:"missing,omitempty"`
	// Extra is what the user passed with --jail-arg, verbatim and last.
	Extra []string `json:"extra,omitempty"`

	// flags is what this build's own help advertises, read once in resolveJail and
	// consulted again when the toolchain is mapped. mapArgs is Maps as this build
	// spells them. Neither is reported: flags is an implementation detail of asking,
	// and mapArgs would say what Maps already says, in a second vocabulary.
	flags   map[string]bool
	mapArgs []string
}

type jailOptions struct {
	noInstall bool
	yes       bool
	quiet     bool
	extra     []string
}

// resolveJail puts this launch inside ai-jail, or returns nil having said why it
// could not.
//
// nil means the caller stops. That is the whole difference between this and
// resolveHeadroom, and it is deliberate: Headroom missing costs a session of
// compression, while a sandbox missing costs the property the user asked for by
// name. Somebody who typed --jail and watched an agent start believes they are
// contained, and a false belief about containment is worse than a known absence of
// it — they would not have run the thing at all.
func resolveJail(opts jailOptions) *jailReport {
	if !jail.Supported() {
		render.Err(jail.Unsupported())
		render.Detail("  " + jail.Repo)
		return nil
	}
	report := &jailReport{Wrapping: true, Backend: jail.Backend(), Extra: opts.extra}

	p, ok := jail.Path()
	if !ok {
		if !installJail(opts) {
			return nil
		}
		if p, ok = jail.Path(); !ok {
			render.Err(fmt.Sprintf("cargo reported success but %s is still not on PATH", jail.Bin))
			return nil
		}
		report.Install = installInstalled
		render.OK(strings.TrimSpace(jail.Bin + " installed: " + p))
	} else {
		report.Install = installPresent
	}
	report.Path, report.Version = p, jail.Version(p)

	// The two flags an agent needs to function, read off this build's own help
	// rather than compiled in. Everything else is policy and stays in .ai-jail,
	// which ai-jail reads by itself.
	report.flags = jail.HelpFlags(jail.Help(p))
	report.Options, report.Missing = jail.Needed(report.flags)
	if len(report.Missing) > 0 && !opts.quiet {
		render.Warn(fmt.Sprintf("this %s build advertises no %s — the agent may not reach its API or its credentials",
			jail.Bin, strings.Join(report.Missing, " or ")))
		render.Detail("  scc passes no substitute rather than guessing; set it in ~/.ai-jail, or pass --jail-arg")
	}
	return report
}

// installJail offers the install, and reports rather than proceeds when it cannot.
// Every path out of here that is not an installed binary ends the launch.
func installJail(opts jailOptions) bool {
	switch {
	case opts.noInstall:
		render.Err(fmt.Sprintf("%s is not on PATH, and --no-install was passed", jail.Bin))
	case !jail.Available():
		render.Err(fmt.Sprintf("%s is not on PATH and cargo is not either, so it cannot be installed here", jail.Bin))
		render.Detail("  " + jail.InstallHint())
		return false
	case opts.yes:
		return runJailInstall(opts)
	case opts.quiet || !interactive():
		// Unattended. Building a Rust binary without being asked is not a decision
		// scc makes silently, and a sandbox is not a thing to install by surprise.
		render.Err(fmt.Sprintf("%s is not on PATH, and nobody is here to answer the install prompt", jail.Bin))
	default:
		render.Warn(fmt.Sprintf("%s is not on PATH — it is the sandbox --jail asks for (%s)", jail.Bin, jail.Backend()))
		render.Detail("  " + jail.Repo)
		if confirmInstall(promptIn, fmt.Sprintf("Install it now with `%s`?", jail.InstallCmd())) {
			return runJailInstall(opts)
		}
		render.Err("install declined, so there is no sandbox to start the agent in")
		return false
	}
	render.Detail("  " + jail.InstallHint())
	render.Detail("  or drop --jail to start the agent without a sandbox, deliberately")
	return false
}

// jailToolchain mounts the binaries scc's own guidance names back into the
// sandbox, read-only.
//
// This is the third thing scc asks the jail for, and it is asked for the same
// reason as the first two. ai-jail's private home replaces $HOME with a fresh
// tmpfs and binds only the command it was handed, so `rtk` in ~/.cargo/bin and an
// npm-installed `scc` vanish with the rest of the home, and PATH is then pruned
// to what survived. The agent is left in front of an entry file telling it to
// prefix every command with a binary that is not there, and rules telling it to
// answer questions with a command that is not there either — which it discovers
// one failed command at a time, and works around by reading whole files instead.
//
// Mapping is a real loosening and it is kept as narrow as the tools allow: three
// binaries scc can name, read-only, every one of them reported in Maps and
// printed in the command line. Everything else stays policy, and policy stays in
// .ai-jail.
// root and tools are parameters rather than calls, so a test can state a
// toolchain and a hidden region instead of depending on how the machine running
// the suite installed its own.
func jailToolchain(report *jailReport, root string, tools []jailTool, quiet bool) {
	if root == "" {
		return
	}
	var maps []jail.Mapping
	for _, t := range tools {
		maps = append(maps, jail.Needs(root, t.entry, t.real)...)
	}
	report.Maps = jail.Dedupe(maps)
	args, ok := jail.MapArgs(report.flags, report.Maps)
	if !ok {
		// Reported, never substituted. The launch goes ahead — the sandbox is what
		// was asked for and it is intact — but the agent will find its own toolchain
		// missing, and that is worth hearing before it does.
		report.Missing = append(report.Missing, jail.FlagMap)
		report.Maps = nil
		if !quiet {
			render.Warn(fmt.Sprintf("this %s build advertises no %s — %s, %s and %s will not exist inside the sandbox",
				jail.Bin, jail.FlagMap, prog(), rtk.Bin, codegraph.Bin))
			render.Detail("  map them from ~/.ai-jail instead, under ro_maps")
		}
		return
	}
	report.mapArgs = args
}

// jailTool is one binary to keep reachable: where PATH finds it, and the file scc
// already knows sits behind that name.
type jailTool struct {
	entry string
	real  string
}

// jailTools is the toolchain, and the list is closed on purpose: scc, because
// every rule it scaffolds answers questions with it; rtk and codegraph, because
// scc wrote the blocks in the entry file that tell the agent to use them. A tool
// scc never mentioned is a tool the user can map themselves with --jail-arg.
func jailTools() []jailTool {
	tools := []jailTool{sccTool()}
	if p, ok := rtk.Path(); ok {
		tools = append(tools, jailTool{entry: p})
	}
	if p, ok := codegraph.Path(); ok {
		tools = append(tools, jailTool{entry: p})
	}
	return tools
}

// sccTool names the running binary rather than whatever PATH resolves to, which
// is what makes the npm distribution work inside a sandbox at all: `scc` there is
// a node shim that spawns the Go binary out of a platform package, and
// os.Executable is that binary. Mapping it at the name PATH knows replaces a
// launcher with the thing it launches, and takes node out of the picture.
func sccTool() jailTool {
	real, err := os.Executable()
	if err != nil {
		real = ""
	}
	entry, err := exec.LookPath("scc")
	if err != nil {
		// Not on PATH — run through npx, or from a build directory. Mount it where
		// it stands: an agent that cannot find it is a smaller problem than one
		// whose `scc` is a path that does not exist.
		entry = real
	}
	return jailTool{entry: entry, real: real}
}

func runJailInstall(opts jailOptions) bool {
	render.Info(fmt.Sprintf("installing %s: %s — this takes a few minutes", jail.Bin, jail.InstallCmd()))
	out := os.Stdout
	if opts.quiet {
		out = os.Stderr
	}
	if err := jail.Install(out, os.Stderr); err != nil {
		render.Err(err.Error())
		return false
	}
	return true
}
