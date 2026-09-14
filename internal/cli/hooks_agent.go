package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/gate"
	"github.com/protonspy/spec-claude-code/internal/git"
	"github.com/protonspy/spec-claude-code/internal/hooks"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// The harness side of `scc hooks run`: the stages Claude Code runs inside a
// session rather than the ones git runs around a commit.
//
// The protocol is the harness's, and it is small. The event arrives as JSON on
// stdin, and what scc prints on stdout comes back to the agent as context for its
// next turn. Everything else about these two stages follows from one decision:
//
// **They report and never refuse.** Exit 2 would block the turn, and a hook that
// can stop a turn can also loop one — the agent fixes the finding, the hook fires
// again on the turn that fixed it, and a bad comparison spends a session. The git
// hooks are where refusal belongs, because a commit is a decision with a natural
// place to stand in front of; the end of a turn is a moment to *tell* somebody
// something while it is still cheap to act on.
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
	// The event is read and discarded. Nothing here needs a field from it yet, and
	// the harness is entitled to a reader that consumes its input rather than one
	// that leaves a pipe half-full.
	_, _ = io.Copy(io.Discard, in)

	var lines []string
	switch stage {
	case hooks.StageSessionStart:
		lines = sessionStartContext(root)
	default:
		lines = stopContext(root)
	}
	return emitHook(stage, lines)
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
	if stage == hooks.StageStop {
		event = hooks.Stop
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
	cfg, err := gate.Load(root)
	if err != nil || cfg.Any() {
		return nil
	}
	return []string{
		"scc: this workspace records no build, format, lint or test command, so the delivery gate has nothing to run.",
		fmt.Sprintf("Record them with `%s check set <gate> \"<command>\"`, built from this project's own toolchain;", prog()),
		fmt.Sprintf("`%s check skip <gate>` is the answer for a step this project genuinely does not have.", prog()),
		`The test gate has to print {"total": N, "coverage": P} on stdout and keep the suite's exit status.`,
		fmt.Sprintf("`%s check help` has the rest.", prog()),
	}
}

// stopContext is what the end of a turn has to say: what the validators found,
// and whether finished work is still sitting on this machine.
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

// stopFindings runs what a turn can afford: every validator that reads a file,
// and neither of the two that do not. The pull request is a network call and the
// test suite is a test suite — both belong at the push, which is where the hook
// that runs them is.
func stopFindings(root string) *finding.Set {
	set, err := gateFindings(root, false)
	if err != nil {
		return nil
	}
	return set
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
func undeliveredLine(root string) string {
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
