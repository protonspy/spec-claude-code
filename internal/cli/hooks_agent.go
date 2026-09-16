package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/codegraph"
	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/gate"
	"github.com/protonspy/spec-claude-code/internal/git"
	"github.com/protonspy/spec-claude-code/internal/hooks"
	"github.com/protonspy/spec-claude-code/internal/validate"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// The harness side of `scc hooks run`: the stages Claude Code runs inside a
// session rather than the ones git runs around a commit.
//
// The protocol is the harness's, and it is small. The event arrives as JSON on
// stdin, and what scc prints on stdout comes back to the agent as context for its
// next turn. Everything else about these two stages follows from one decision:
//
// **They never refuse — but on Stop, speaking is itself a continuation.** This
// was written down wrong once and shipped as a loop, so it is worth stating
// precisely: the harness treats `additionalContext` on Stop as guidance the
// conversation continues for, under the same loop protections as an outright
// block. Returning 0 is not a passive report there. Two rules follow, and both
// are load-bearing:
//
//   - `stop_hook_active` is honoured unconditionally. Once the harness says it is
//     already continuing because of this hook, the hook has been heard.
//   - Stop may only carry what the next turn can *resolve*. A finding clears when
//     it is fixed; unpushed work clears when it is pushed. A fact about the
//     branch clears for nobody, which is why drift is reported at SessionStart.
//
// The git hooks are where refusal belongs, because a commit is a decision with a
// natural place to stand in front of.
//
// **They never publish.** The end of a turn is a plausible moment to `git push`,
// and scc does not: that sends work on an event nobody is watching. What the Stop
// stage does instead is say the branch has commits that never left the machine,
// so the agent pushes in its next turn — in the open, where a person can still
// stop it.
//
// **And they cost nothing a turn cannot afford.** No network: the pull-request
// check and the test suite are both off here, and undelivered work is answered
// from refs already in the repository. A check that cost a fetch per turn is a
// check somebody turns off, and it would take the cheap ones with it.
func runAgentStage(root string, stage hooks.Stage, in io.Reader) int {
	// The event is read whole either way. Only one stage needs a field from it,
	// and the harness is entitled to a reader that consumes its input rather than
	// one that leaves a pipe half-full.
	event := readEvent(in)

	// The prompt stage returns before the sync, and that is the point rather than
	// an oversight. SessionStart and Stop bookend a task and run while nobody is
	// waiting; UserPromptSubmit sits between the person pressing enter and the
	// agent starting, on every prompt, where latency is felt directly. Spending a
	// CodeGraph subprocess there would make every turn slower to save a read the
	// turn may not even do.
	if stage == hooks.StageUserPrompt {
		return emitHook(stage, promptContext(root, event.Prompt))
	}

	// **Stop is a continuation, not a report, and getting that wrong shipped a
	// loop.**
	//
	// The harness treats `additionalContext` on Stop as guidance it should act on:
	// the conversation *continues* so the agent can, under the same loop
	// protections as an outright block. This package's comments claimed the
	// opposite — that returning 0 with context was a passive report and only exit
	// 2 could hold a turn open — and on that reading it never read
	// `stop_hook_active`. The result, in a real session: a branch-scoped finding
	// that could never clear re-fired on every continuation until the harness
	// overrode it at the consecutive-block cap, with the agent answering "." to
	// itself nine times.
	//
	// So the flag is honoured first and unconditionally. When the harness is
	// already continuing because of this hook, the hook has nothing further to
	// add: it has been heard, and saying it again is the loop.
	if stage == hooks.StageStop && event.StopHookActive {
		return ExitOK
	}

	// The other two, before anything else: the graph is brought current at the two
	// moments that bookend a task. See syncGraph — it is silent, and it is the one
	// thing here that changes the workspace rather than reporting on it.
	syncGraph(root)

	var lines []string
	switch stage {
	case hooks.StageSessionStart:
		lines = sessionStartContext(root)
	default:
		lines = stopContext(root)
	}
	return emitHook(stage, lines)
}

// hookEvent is the part of the harness's event this package reads.
//
// One field today. Parsed leniently on purpose: this is the harness's schema on
// the harness's release schedule, and a stage whose whole job is to be silent
// most of the time must degrade to silence rather than to an error when the
// document is not what it expected.
type hookEvent struct {
	Prompt string `json:"prompt"`
	// StopHookActive is true when the harness is *already* continuing because of
	// a Stop hook. Reading it is not optional, and this package learned that the
	// hard way — see runAgentStage.
	StopHookActive bool `json:"stop_hook_active"`
}

func readEvent(in io.Reader) hookEvent {
	var e hookEvent
	raw, err := io.ReadAll(in)
	if err != nil {
		return e
	}
	_ = json.Unmarshal(raw, &e)
	return e
}

// hookOutput is the harness's frozen shape. `additionalContext` is the field that
// reaches the agent; the wrapper object is what the protocol asks for.
type hookOutput struct {
	Specific hookSpecific `json:"hookSpecificOutput"`
}

type hookSpecific struct {
	EventName         string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// emitHook writes the document, or nothing at all.
//
// Nothing at all is the common case and the important one: a workspace with no
// findings and nothing undelivered has no business adding a line to every turn of
// every session. Silence is what keeps the line that does appear worth reading.
func emitHook(stage hooks.Stage, lines []string) int {
	if len(lines) == 0 {
		return ExitOK
	}
	event := hooks.SessionStart
	switch stage {
	case hooks.StageStop:
		event = hooks.Stop
	case hooks.StageUserPrompt:
		event = hooks.UserPromptSubmit
	}
	out := hookOutput{Specific: hookSpecific{
		EventName:         string(event),
		AdditionalContext: strings.Join(lines, "\n"),
	}}
	b, err := json.Marshal(out)
	if err != nil {
		// A hook that cannot write its own document says nothing and gets out of
		// the way. Exiting non-zero would put an error notice in front of the user
		// for a failure that cost them nothing.
		return ExitOK
	}
	// Ignored deliberately: the harness reads this on a pipe it owns, and a write
	// that failed leaves nothing scc could usefully do about it at the end of a turn.
	_, _ = fmt.Fprintln(os.Stdout, string(b))
	return ExitOK
}

// sessionStartContext is what a workspace says about itself before any work is
// done on it.
//
// One thing, and only when it is true: this workspace has decided nothing about
// its own commands. That is the gap the agent is the only one who can close — the
// commands depend on this project's toolchain, scc cannot derive them, and the
// moment to say so is before the session picks up a task rather than at the push
// that fails.
func sessionStartContext(root string) []string {
	if !workspace.IsWorkspace(root) {
		return nil
	}
	// First, because it changes what the agent does with everything below it.
	out := unattendedLines()
	// Drift is reported here rather than at the end of a turn, and the move was
	// paid for in a broken session. It is a fact about the branch — source changed
	// with no box ticked, a marker in code, commits on the base branch — so no
	// action in the next turn makes it false, and at Stop, where context continues
	// the conversation, a fact that cannot be resolved is a fact that repeats
	// until the harness cuts it off. Said once, at the start, where it is
	// information rather than an instruction the agent cannot carry out.
	out = append(out, driftLines(root)...)

	cfg, err := gate.Load(root)
	if err != nil || cfg.Any() {
		return out
	}
	return append(out,
		"scc: this workspace records no build, format, lint or test command, so the delivery gate has nothing to run.",
		fmt.Sprintf("Record them with `%s check set <gate> \"<command>\"`, built from this project's own toolchain;", prog()),
		fmt.Sprintf("`%s check skip <gate>` is the answer for a step this project genuinely does not have.", prog()),
		`The test gate has to print {"total": N, "coverage": P} on stdout and keep the suite's exit status.`,
		fmt.Sprintf("`%s check help` has the rest.", prog()),
	)
}

// unattendedLines says, once, that nobody is going to read a question asked this
// turn.
//
// `rules/autonomy.md` opens every piece of work with three questions, and that is
// right whenever there is somebody to answer them. In a headless run there is not,
// and the rule alone cannot tell the difference: from inside the turn an unattended
// session looks exactly like an attended one. Measured, on a pinned ticket at n=3:
// the scaffolded harness ended every run holding the kickoff questions and wrote no
// code at all, while the same agent with no methodology shipped the feature every
// time. Adding the branch to the rule's prose changed nothing — 0 of 3 again —
// because the agent was being asked to act on a condition it cannot observe. With
// this line in front of it, the same arm delivered 6 of 6, with the plan, the
// ticked box, the branch and a clean `scc validate` the methodology promises.
//
// **The signal is undocumented and is treated that way.** `CLAUDE_CODE_SESSION_
// ATTENDED=0` is in the environment of a hook during `claude -p`, verified against
// 2.1.273; Claude Code's own documentation describes neither the variable nor its
// values, so this reads exactly one spelling and says nothing for anything else,
// unset included. That default is the safe one in the only direction that matters:
// a missed line leaves today's behaviour, while a line in front of an attended
// session would talk somebody out of a question they were right to ask.
//
// **The value is set by the harness rather than inherited**, which is what makes it
// worth reading at all: an environment variable carries no provenance on its own, so
// a hostile `.envrc` or `devcontainer.json` in a clone would otherwise be able to
// tell an attended session that nobody is watching — and the one checkpoint a person
// gets is `gated`. Measured: exported as `1` into the shell that launched `claude
// -p`, the hook still saw `0`. That is one direction of a symmetric mechanism and
// not a proof of both, so the last line emitted says the conversation wins. Live
// user text is the one input no file in the repository can forge, and somebody who
// asks for a gated run gets one whatever this variable says.
func unattendedLines() []string {
	if os.Getenv("CLAUDE_CODE_SESSION_ATTENDED") != "0" {
		return nil
	}
	return []string{
		"scc: this session is unattended, so a question asked this turn reaches nobody.",
		"Take the kickoff answers as `autonomy: auto`, `ci: no-wait` and the user's own language,",
		"record them in the artifact's frontmatter as assumed rather than answered, say so in one line, and build.",
		"If the user does ask for something else in this conversation, they are here after all and what they say wins.",
	}
}

// stopContext is what the end of a turn has to say: what the validators found,
// and whether finished work is still sitting on this machine.
//
// **Only things the agent can resolve now.** Anything here continues the
// conversation, so whatever it says has to be something the next turn can act on
// and thereby make go away — otherwise the same line comes back on the next stop,
// and the next, until the harness overrides the hook at its consecutive-block
// cap. A finding clears when it is fixed; unpushed work clears when it is pushed.
// Both terminate.
//
// Drift used to be here and is not any more, for exactly that reason: "source
// changed on this branch with no box ticked" is a fact about the branch, not
// about the turn, and no action the agent takes in the next turn can make it
// false. Measured in a real session, it re-fired on every continuation while the
// agent — correctly — kept answering that the work was not orphaned. It moved to
// SessionStart, which is said once and cannot hold a turn open.
//
// Both halves are silent when they have nothing to say, and that is the property
// to protect: sections that each speak sometimes are a report worth reading, and
// sections that always speak are a banner.
func stopContext(root string) []string {
	var out []string
	if set := stopFindings(root); set != nil && !set.Empty() {
		out = append(out, fmt.Sprintf("scc validate: %s.", finding.Count(set.Len())))
		out = append(out, stopFindingLines(set)...)
		out = append(out, fmt.Sprintf("Run `%s validate` for the rest, and fix these before the commit.", prog()))
	}
	if line := undeliveredLine(root); line != "" {
		out = append(out, line)
	}
	return out
}

// stopFindings runs what a turn can afford and reports what a turn can act on.
//
// What it can afford: every validator that reads a file, and neither of the two
// that do not. The pull request is a network call and the test suite is a test
// suite — both belong at the push, which is where the hook that runs them is.
//
// What it can act on is the narrower half, and it is the Stop contract rather
// than a nicety, because speaking here continues the conversation. Two kinds of
// finding can never clear in the next turn and so would re-fire at the end of
// every turn for the life of the branch:
//
//   - **a signature in a commit that is already made** — a rebase, not an edit,
//     so `attribution` comes out of this run entirely.
//   - **a finding in a file this branch never touched** — a migrated plan, a
//     stale ADR, whatever the workspace was carrying before this work started.
//     The agent is told to go and fix a file its task has nothing to do with,
//     every turn, and correctly declines.
//
// Both are reported in full by `scc validate`, which is where somebody asking
// about the workspace gets the whole answer. Here the question is narrower: what
// did this turn just break.
func stopFindings(root string) *finding.Set {
	set, err := gateFindings(root, false, validate.WithoutAttribution())
	if err != nil {
		return nil
	}
	return touchedHere(root, set)
}

// touchedHere keeps the findings that sit in a file this branch has changed.
//
// Its answer when git cannot say — no repository, an unborn HEAD, a base that
// will not resolve — is the whole set, unfiltered. That direction is deliberate:
// the filter exists to keep the stage from nagging about work nobody here did,
// and a workspace where the question cannot be asked is one where every finding
// is as likely as not to be this session's.
func touchedHere(root string, set *finding.Set) *finding.Set {
	if set == nil || set.Empty() {
		return set
	}
	changes, err := git.Changed(root, "")
	if err != nil || len(changes) == 0 {
		return set
	}
	touched := make(map[string]bool, len(changes))
	for _, c := range changes {
		touched[finding.Rel(c.Path)] = true
	}
	out := &finding.Set{}
	for _, f := range set.Sorted() {
		if touched[finding.Rel(f.File)] {
			out.Add(f)
		}
	}
	return out
}

// stopFindingLines is the findings themselves, capped.
//
// Capped because this is context the agent pays for on its next turn, and a
// workspace mid-migration can report dozens. The first few plus a count is enough
// to act on; the whole list is what `scc validate` is for, and the line above
// says so.
func stopFindingLines(set *finding.Set) []string {
	const most = 5
	var out []string
	all := set.Sorted()
	for i, f := range all {
		if i == most {
			out = append(out, fmt.Sprintf("  … and %d more", len(all)-most))
			break
		}
		where := f.File
		if f.Line > 0 {
			where = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		// One line each, and the message clipped: a finding whose message runs to a
		// paragraph is still identified by its rule and its file.
		out = append(out, fmt.Sprintf("  %s [%s] %s", where, f.Rule, firstLine(f.Message)))
	}
	return out
}

// undeliveredLine says that finished work has not left the machine, and stops
// there. It names the commands rather than running them: publishing is the
// agent's act, in its next turn, where a person can see it happen.
//
// **Finished is the word that does the work, and a dirty tree is not it.** Said on
// the first commit of a unit of work, this asks for a push in the middle of it —
// and because Stop is a continuation, the ask is acted on: a push per turn, each
// one paying for the pre-push gate's build, lint and suite, and a pull request
// opened on a third of a feature. Uncommitted changes are the cheapest available
// evidence that the work is still in hand, so the line waits for them to be gone.
// A clean tree with commits the remote does not have is the state this is for.
func undeliveredLine(root string) string {
	if git.Dirty(root) {
		return ""
	}
	u := git.UndeliveredWork(root)
	if !u.Any() {
		return ""
	}
	what := fmt.Sprintf("%d commit", u.Unpushed)
	if u.Unpushed != 1 {
		what += "s"
	}
	if !u.Pushed {
		return fmt.Sprintf("scc: `%s` has %s and has never been pushed — `git push -u origin %s`, then open the PR per delivery.md.",
			u.Branch, what, u.Branch)
	}
	return fmt.Sprintf("scc: `%s` has %s that origin does not have — `git push`, then check the PR per delivery.md.",
		u.Branch, what)
}

// firstLine clips a message to its first line, so a finding that quotes a test
// log contributes one row to the report rather than twelve.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// stale reports whether dir's graph is older than something inside it.
//
// It exists because this runs at the end of every turn, and CodeGraph is a node
// process: an unconditional sync spends one on a turn that answered a question and
// wrote nothing, and on Windows that is most of a second of the user waiting for a
// re-index of a tree nobody edited. The comparison is a stat per changed file
// rather than a walk, which is what keeps the check cheaper than the thing it is
// avoiding.
//
// The answer when it cannot tell — no `.codegraph` to stat, no list of changed
// files, a file that has since been deleted — is stale. Re-indexing needlessly
// costs a subprocess; skipping a sync that was needed leaves the agent with an
// index that answers confidently about code that changed, and the rule this
// implements says that is the worse of the two.
func stale(dir string, changed []git.Change) bool {
	info, err := os.Stat(filepath.Join(dir, codegraph.Dir))
	if err != nil {
		return true
	}
	if len(changed) == 0 {
		return true
	}
	indexed := info.ModTime()
	for _, c := range changed {
		p := c.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, filepath.FromSlash(p))
		}
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		if fi.ModTime().After(indexed) {
			return true
		}
	}
	return false
}

// syncGraph brings every scoped graph up to date, silently.
//
// This is the deterministic half of the staleness problem, and it is deliberately
// not a rule. A stale graph answers confidently about code that changed, which is
// worse than no graph — and "remember to sync" is exactly the kind of instruction
// this product exists to stop relying on. So it runs where scc controls the
// moment rather than where the agent has to remember: at the top of a session,
// and at the end of every turn that just wrote code.
//
// The two moments are the bookends of a task. SessionStart covers the session
// that did not come through `scc launch` — the one `scc launch` was already
// handling, and the gap it left. Stop covers the rest: the agent has just edited
// the tree, and the next turn is exactly when a query about what it wrote would
// otherwise be answered from an index taken before the edit.
//
// **Silent on every outcome.** There is no binary, there is no graph, CodeGraph
// failed — all of them end in a session that carries on, and a line about
// plumbing at the end of every turn is context the agent pays for and cannot act
// on. What a stale or missing graph costs is already in the rule; what this does
// is make it rare.
func syncGraph(root string) {
	if !workspace.IsWorkspace(root) {
		return
	}
	bin, ok := codegraph.Path()
	if !ok {
		return
	}
	scope, err := scopeOf(root)
	if err != nil {
		return
	}
	// What this branch has touched, asked once and reused per root. It is what
	// makes the sync conditional: an index is re-run when a file it covers is
	// newer than it is, and skipped otherwise.
	changed, _ := git.Changed(root, "")

	roots, _ := codegraph.Roots(root, scope)
	for _, r := range roots {
		// Only a tree that already has a graph. Building one from a hook would turn
		// the end of a turn into the first full index of a large repository, which
		// is a minute nobody asked for — `scc launch` and `scc graph build` are
		// where that decision is made deliberately.
		if !r.Indexed() {
			continue
		}
		if !stale(r.Dir, changed) {
			continue
		}
		// Output discarded rather than forwarded: stdout belongs to the hook's own
		// JSON document, and CodeGraph's progress bars are not an event the agent
		// needs. A failure is a stale graph, which the rule already covers.
		_, _ = codegraph.Run(bin, r.Dir, codegraph.SyncArgs(), io.Discard, io.Discard)
	}
}
