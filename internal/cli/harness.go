package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/render"
)

// chooseHarness resolves which harness `scc init` scaffolds for, in this order:
// an explicit flag, then a prompt, then the default.
//
// The prompt only happens when a person is actually there to answer it — stdin
// is a terminal and the caller did not ask for JSON. That condition is the whole
// design: scc's contract is that an agent or a CI job drives it exactly as well
// as a human does, and a command that blocks on a read no one will answer breaks
// that contract far worse than a wrong default would. Non-interactive callers
// get Claude Code silently, which is what every existing script already assumed.
func chooseHarness(picks map[string]*bool, jsonOut bool) (paths.Harness, error) {
	var chosen []paths.Harness
	for _, h := range paths.Harnesses() {
		if p, ok := picks[h.ID]; ok && *p {
			chosen = append(chosen, h)
		}
	}
	switch len(chosen) {
	case 1:
		return chosen[0], nil
	case 0:
		if jsonOut || !interactive() {
			return paths.Claude, nil
		}
		return promptHarness(promptIn), nil
	default:
		// Exclusive rather than additive because each run writes one manifest and
		// one entry file: two harnesses in one invocation would have to pick which
		// one the shared AGENTS.md describes, and guessing that is worse than
		// asking for a second run.
		names := make([]string, 0, len(chosen))
		for _, h := range chosen {
			names = append(names, "--"+h.ID)
		}
		return paths.Harness{}, fmt.Errorf(
			"%s: pick one harness per run, and run init again for the other",
			strings.Join(names, " and "))
	}
}

// promptHarness shows the supported harnesses and reads a choice. Claude Code is
// first and is the default, because it is the harness the methodology was
// designed against and the only one whose subagent, skill, and slash-command
// surfaces all exist at project scope.
//
// A numbered list read line by line, not a full-screen selector: scc is a
// headless CLI that happens to be talkative when a human is present, and a raw
// terminal mode would be a second interaction model to maintain — and to get
// wrong on Windows — for one question asked once per repo.
func promptHarness(in io.Reader) paths.Harness {
	all := paths.Harnesses()

	fmt.Println()
	render.Ask("Which harness is this workspace for?\n")
	fmt.Println()
	width := 0
	for _, h := range all {
		if len(h.Label) > width {
			width = len(h.Label)
		}
	}
	for i, h := range all {
		line := fmt.Sprintf("  %s  %-*s  %s · %s/",
			render.Bold(strconv.Itoa(i+1)+")"), width, h.Label, h.EntryFile, h.Dir)
		if i == 0 {
			line += render.Cyan("  (default)")
		}
		fmt.Println(line)
	}
	fmt.Println()

	scanner := bufio.NewScanner(in)
	for {
		render.Ask(fmt.Sprintf("Pick 1-%d, or Enter for %s: ", len(all), all[0].Label))
		if !scanner.Scan() {
			// EOF: the input ended without an answer, so take the default rather
			// than looping forever on a closed pipe.
			fmt.Println()
			return all[0]
		}
		answer := strings.TrimSpace(scanner.Text())
		if answer == "" {
			return all[0]
		}
		if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(all) {
			return all[n-1]
		}
		// A name works too — somebody who typed "codex" meant it.
		if h, err := paths.ParseHarness(strings.ToLower(answer)); err == nil {
			return h
		}
		render.Warn(fmt.Sprintf("not one of the choices: %q", answer))
	}
}

// Where an interactive prompt reads from, and whether anybody is there to answer
// it. Two variables rather than a direct os.Stdin read, so the tests can drive
// both halves: the prompts are a real part of the surface and would otherwise be
// checkable only by hand, or worse, would behave differently under `go test`
// depending on whether the runner attached a console.
var (
	promptIn    io.Reader = os.Stdin
	interactive           = func() bool { return isTerminal(os.Stdin) }
)

// isTerminal reports whether f is a character device, i.e. a real terminal rather
// than a pipe or a file. Same test render uses to decide on color, and stdlib
// only — the binary ships to six platforms and every dependency is a supply-chain
// surface.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
