package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/assets"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// withPrompt makes the process look interactive and answers whatever is asked
// with input. Without it the tests inherit whatever stdin the runner attached,
// which differs between a terminal, a CI job, and an IDE — and a prompt is not
// testable at all if it reads the real one.
func withPrompt(t *testing.T, input string) {
	t.Helper()
	origIn, origTTY := promptIn, interactive
	promptIn, interactive = strings.NewReader(input), func() bool { return true }
	t.Cleanup(func() { promptIn, interactive = origIn, origTTY })
}

// withoutTerminal is the unattended case: an agent or a CI job, where nobody can
// answer a prompt.
func withoutTerminal(t *testing.T) {
	t.Helper()
	origIn, origTTY := promptIn, interactive
	promptIn, interactive = strings.NewReader(""), func() bool { return false }
	t.Cleanup(func() { promptIn, interactive = origIn, origTTY })
}

// Each harness flag scaffolds that harness's whole tree, entry file included.
func TestInitScaffoldsTheChosenHarness(t *testing.T) {
	for _, h := range paths.Harnesses() {
		root := t.TempDir()
		if _, stderr, code := run(t, "init", "--"+h.ID, "--root", root); code != ExitOK {
			t.Fatalf("%s: exit = %d (%s)", h.ID, code, stderr)
		}
		if !workspace.IsWorkspace(root) {
			t.Fatalf("%s: no marker", h.ID)
		}
		for _, f := range assets.Workspace(h) {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(f.Rel))); err != nil {
				t.Errorf("%s: %s was not written: %v", h.ID, f.Rel, err)
			}
		}
		got := workspace.Harnesses(root)
		if len(got) != 1 || got[0].ID != h.ID {
			t.Errorf("%s: initialized harnesses = %v", h.ID, got)
		}
	}
}

// No flag and no terminal is the CI and agent case, and it must stay silent and
// deterministic rather than blocking on a prompt nobody will answer.
func TestInitWithoutAFlagDefaultsToClaude(t *testing.T) {
	withoutTerminal(t)
	root := t.TempDir()
	if _, stderr, code := run(t, "init", "--root", root); code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, stderr)
	}
	got := workspace.Harnesses(root)
	if len(got) != 1 || got[0].ID != paths.Claude.ID {
		t.Errorf("initialized harnesses = %v, want claude", got)
	}
}

// With a person at the terminal and no flag, init asks — and scaffolds what they
// picked.
func TestInitAsksWhenNobodyPassedAFlag(t *testing.T) {
	withPrompt(t, "2\n")
	root := t.TempDir()
	stdout, stderr, code := run(t, "init", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, stderr)
	}
	if !strings.Contains(stdout, "Which harness") {
		t.Errorf("init did not ask:\n%s", stdout)
	}
	got := workspace.Harnesses(root)
	if len(got) != 1 || got[0].ID != paths.Codex.ID {
		t.Errorf("initialized harnesses = %v, want codex", got)
	}
}

// --json is a machine reading the output, so the prompt must not appear even on a
// terminal: it would land in the middle of the document.
func TestInitDoesNotAskUnderJSON(t *testing.T) {
	withPrompt(t, "2\n")
	root := t.TempDir()
	stdout, stderr, code := run(t, "init", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, stderr)
	}
	if strings.Contains(stdout, "Which harness") {
		t.Errorf("init prompted under --json:\n%s", stdout)
	}
	got := workspace.Harnesses(root)
	if len(got) != 1 || got[0].ID != paths.Claude.ID {
		t.Errorf("initialized harnesses = %v, want claude", got)
	}
}

// Two harnesses in one run would have to pick which one the shared AGENTS.md
// describes. Refusing is better than guessing.
func TestInitRefusesTwoHarnessesAtOnce(t *testing.T) {
	root := t.TempDir()
	_, stderr, code := run(t, "init", "--codex", "--opencode", "--root", root)
	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "--codex") || !strings.Contains(stderr, "--opencode") {
		t.Errorf("stderr does not name both flags: %q", stderr)
	}
	if workspace.IsWorkspace(root) {
		t.Error("a refused init still wrote a workspace")
	}
}

// The picker: a number, a name, or Enter for the default, and an unusable answer
// asks again rather than picking something the user did not choose.
func TestPromptHarnessReadsAChoice(t *testing.T) {
	cases := map[string]string{
		"":             paths.Claude.ID, // EOF
		"\n":           paths.Claude.ID, // Enter
		"1\n":          paths.Claude.ID,
		"2\n":          paths.Codex.ID,
		"3\n":          paths.OpenCode.ID,
		"opencode\n":   paths.OpenCode.ID,
		"  Codex  \n":  paths.Codex.ID,
		"9\nnope\n2\n": paths.Codex.ID,
	}
	for in, want := range cases {
		var got paths.Harness
		_, _, _ = capture(t, func() int {
			got = promptHarness(strings.NewReader(in))
			return 0
		})
		if got.ID != want {
			t.Errorf("promptHarness(%q) = %q, want %q", in, got.ID, want)
		}
	}
}

// The prompt lists every supported harness with Claude Code first, so the choice
// is visible rather than something the user has to know to ask about.
func TestPromptHarnessListsEveryHarness(t *testing.T) {
	stdout, _, _ := capture(t, func() int {
		promptHarness(strings.NewReader("\n"))
		return 0
	})
	for _, h := range paths.Harnesses() {
		if !strings.Contains(stdout, h.Label) || !strings.Contains(stdout, h.Dir+"/") {
			t.Errorf("the prompt does not offer %s:\n%s", h.Label, stdout)
		}
	}
	if i, j := strings.Index(stdout, paths.Claude.Label), strings.Index(stdout, paths.Codex.Label); i > j {
		t.Error("Claude Code is not listed first")
	}
}
