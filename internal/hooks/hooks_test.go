package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/paths"
)

// repo is a throwaway git repository. Every test here needs one, because the
// hooks directory is git's answer and not a path scc gets to assume.
func repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q", ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return dir
}

func TestInstallWritesEveryStage(t *testing.T) {
	dir := repo(t)
	all, err := Install(dir, false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(all) != len(Stages()) {
		t.Fatalf("got %d stages, want %d", len(all), len(Stages()))
	}
	for _, s := range all {
		if s.State != Installed || s.Action != Added {
			t.Errorf("%s: state=%q action=%q, want installed/added (%s)", s.Stage, s.State, s.Action, s.Note)
		}
		raw, err := os.ReadFile(s.Path)
		if err != nil {
			t.Fatalf("%s: %v", s.Stage, err)
		}
		body := string(raw)
		if !strings.HasPrefix(body, shebang) {
			t.Errorf("%s does not start with a shebang:\n%s", s.Stage, body)
		}
		if !strings.Contains(body, Prog+" hooks run "+string(s.Stage)) {
			t.Errorf("%s never calls back into %s:\n%s", s.Stage, Prog, body)
		}
		// The escape hatch is part of the contract: a gate with no documented way
		// past it is a gate people delete rather than skip.
		if !strings.Contains(body, SkipEnv) {
			t.Errorf("%s never names %s", s.Stage, SkipEnv)
		}
	}
	if !OK(all) {
		t.Error("OK says the hooks are not installed right after installing them")
	}
}

// TestInstallIsIdempotent — running init twice is normal, and the second run must
// report "present" rather than rewriting a file that is already right.
func TestInstallIsIdempotent(t *testing.T) {
	dir := repo(t)
	if _, err := Install(dir, false); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	all, err := Install(dir, false)
	if err != nil {
		t.Fatalf("second Install: %v", err)
	}
	for _, s := range all {
		if s.Action != Present {
			t.Errorf("%s: action=%q on the second run, want %q", s.Stage, s.Action, Present)
		}
	}
}

// TestAStaleBlockIsBroughtCurrent: a workspace wired by an older build picks up
// this one's script, because scc owns what is between its own markers.
func TestAStaleBlockIsBroughtCurrent(t *testing.T) {
	dir := repo(t)
	hooksDir, err := Dir(dir)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	path := filepath.Join(hooksDir, string(PreCommit))
	old := shebang + "\n\n" + Markers.Open + " v0 — from an older build\necho hi\n" + Markers.Close + "\n"
	if err := os.WriteFile(path, []byte(old), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if got := state(path, PreCommit); got != Stale {
		t.Fatalf("state = %q, want %q", got, Stale)
	}
	all, err := Install(dir, false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if all[0].Action != Replaced {
		t.Errorf("action = %q, want %q", all[0].Action, Replaced)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "echo hi") {
		t.Error("the old block survived the update")
	}
}

// TestAForeignHookIsLeftAlone is the one that matters most: a repository already
// running husky or lefthook has a pre-commit of its own, and scc appending shell
// to it would be scc authoring what the user owns.
func TestAForeignHookIsLeftAlone(t *testing.T) {
	dir := repo(t)
	hooksDir, _ := Dir(dir)
	path := filepath.Join(hooksDir, string(PreCommit))
	theirs := "#!/bin/sh\nnpx lint-staged\n"
	if err := os.WriteFile(path, []byte(theirs), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	all, err := Install(dir, false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if all[0].State != Foreign || all[0].Action != Skipped {
		t.Errorf("state=%q action=%q, want foreign/skipped", all[0].State, all[0].Action)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != theirs {
		t.Errorf("the hook was rewritten:\n%s", raw)
	}
	if OK(all) {
		t.Error("OK says everything is installed while a stage is not")
	}

	// --force appends, and keeps every line of theirs.
	forced, err := Install(dir, true)
	if err != nil {
		t.Fatalf("forced Install: %v", err)
	}
	if forced[0].State != Installed {
		t.Errorf("state = %q after --force, want %q (%s)", forced[0].State, Installed, forced[0].Note)
	}
	raw, _ = os.ReadFile(path)
	if !strings.Contains(string(raw), "npx lint-staged") {
		t.Errorf("--force dropped the user's own hook:\n%s", raw)
	}
}

// TestAHookInAnotherLanguageIsNotAppendedTo — appending sh to a Python hook is
// not a forced decision, it is a broken repository.
func TestAHookInAnotherLanguageIsNotAppendedTo(t *testing.T) {
	dir := repo(t)
	hooksDir, _ := Dir(dir)
	path := filepath.Join(hooksDir, string(CommitMsg))
	theirs := "#!/usr/bin/env python3\nprint('hi')\n"
	if err := os.WriteFile(path, []byte(theirs), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	all, err := Install(dir, true)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	for _, s := range all {
		if s.Stage != CommitMsg {
			continue
		}
		if s.Action != Skipped || s.Note == "" {
			t.Errorf("action=%q note=%q, want skipped with a reason", s.Action, s.Note)
		}
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != theirs {
		t.Errorf("a Python hook was appended to:\n%s", raw)
	}
}

// TestRemoveTakesTheFileWhenTheBlockWasAllOfIt, and leaves the file when it was
// not — the second half is what keeps `remove` safe to run in a repository that
// forced the block into somebody else's script.
func TestRemoveTakesTheFileWhenTheBlockWasAllOfIt(t *testing.T) {
	dir := repo(t)
	if _, err := Install(dir, false); err != nil {
		t.Fatalf("Install: %v", err)
	}
	hooksDir, _ := Dir(dir)
	theirs := filepath.Join(hooksDir, string(PrePush))
	if err := os.WriteFile(theirs, []byte("#!/bin/sh\necho theirs\n"+script(PrePush)+"\n"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}

	all, err := Remove(dir)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	for _, s := range all {
		if s.Action != Removed {
			t.Errorf("%s: action=%q, want %q (%s)", s.Stage, s.Action, Removed, s.Note)
		}
	}
	if _, err := os.Stat(filepath.Join(hooksDir, string(PreCommit))); !os.IsNotExist(err) {
		t.Error("a hook scc created whole was left behind")
	}
	raw, err := os.ReadFile(theirs)
	if err != nil {
		t.Fatalf("the user's own pre-push was deleted: %v", err)
	}
	if !strings.Contains(string(raw), "echo theirs") {
		t.Errorf("the user's own lines went with the block:\n%s", raw)
	}
	if strings.Contains(string(raw), Markers.Open) {
		t.Errorf("the block survived remove:\n%s", raw)
	}
}

// TestDirRefusesOutsideARepository: writing a hook where git will not read it is
// a failure that looks like success, so it is an error rather than a shrug.
func TestDirRefusesOutsideARepository(t *testing.T) {
	if _, err := Dir(t.TempDir()); err == nil {
		t.Error("Dir answered for a directory that is not a git repository")
	}
}

// TestParseStage takes what a hook script passes and nothing else.
func TestParseStage(t *testing.T) {
	for _, name := range []string{"pre-commit", "PRE-COMMIT", " pre-push "} {
		if _, err := ParseStage(name); err != nil {
			t.Errorf("ParseStage(%q): %v", name, err)
		}
	}
	if _, err := ParseStage("post-merge"); err == nil {
		t.Error("ParseStage accepted a stage scc does not install")
	}
}

// TestNoHarnessNeedsASubagentStage is the gate on a stage scc deliberately does
// not have.
//
// ponytail re-asserts its skill on every subagent start, because it assumes the
// methodology does not survive the boundary. scc asked the same question and got
// the opposite answer for the only harness that could host the stage: Claude Code
// hands a non-fork subagent the whole CLAUDE.md hierarchy, project rules
// included, so a SubagentStart hook would re-send ~26KB the agent already has —
// the exact cost TestRulesStayShortEnoughToBePreloaded exists to prevent, paid
// once per subagent instead of once per request.
//
// So the stage is absent on evidence rather than on appetite, and this is what
// keeps the evidence load-bearing. Two ways to fail, and both are real:
//
//   - A harness with a hook surface stops delivering the rules to its subagents.
//     That is the hole ponytail's stage fills, and it would now be open here.
//   - A harness that does not deliver them grows a hook surface, which is the
//     same hole arriving from the other direction.
//
// Either way the answer is to build the stage, and the failure says so rather
// than leaving the next session to rediscover the question.
func TestNoHarnessNeedsASubagentStage(t *testing.T) {
	for _, h := range paths.Harnesses() {
		if h.SettingsSeg == "" || h.SubagentsInheritRules {
			continue
		}
		t.Errorf("%s has a hook surface (%s) and does not give its subagents the rules: "+
			"a subagent there writes code under none of the methodology, and nothing reports it. "+
			"Register a SubagentStart stage that emits the entry file's routing table — not the "+
			"rules themselves, which is the cost the preload budget exists to prevent.",
			h.ID, h.SettingsSeg)
	}

	// And the one stage this test is actually about stays absent. Named rather
	// than inferred from the set's size: the set is allowed to grow, and what it
	// is not allowed to grow is a stage that re-sends rules the agent already
	// holds.
	for _, e := range Events() {
		if strings.Contains(string(e), "Subagent") {
			t.Errorf("Events() registers %q. Every harness here hands its subagents the rules, "+
				"so this re-sends ~26KB the agent already has — the cost the preload budget exists "+
				"to prevent, paid once per subagent instead of once per request", e)
		}
	}
}

// TestTheHarnessStagesOnlyEverReport pins the contract both stages hold, and the
// one a third would have to hold too.
//
// Exit 2 would block the turn, and a hook that can block a turn can loop one: the
// agent fixes the finding, the hook fires again on the turn that fixed it, and a
// bad comparison spends a session. Refusal belongs at the commit, which is a
// decision with a natural place to stand in front of — which is why the git
// stages are allowed to refuse and these are not.
func TestTheHarnessStagesOnlyEverReport(t *testing.T) {
	for _, s := range AgentStages() {
		if s.Git() {
			t.Errorf("%s is a harness stage but reports itself as one git runs; "+
				"only the git side reads an exit code, and only the git side may refuse", s)
		}
		if s.Why() == "" {
			t.Errorf("%s says nothing about itself, so `scc hooks check` has a blank row", s)
		}
	}
	// Timeouts are bounded on both: a harness stage that hangs holds up the turn
	// it was meant to make cheaper.
	for _, e := range Events() {
		if e.Timeout() <= 0 || e.Timeout() > 300 {
			t.Errorf("%s has a %ds timeout; a stage that can hang a turn is worse than one that says nothing",
				e, e.Timeout())
		}
	}
}

// A shebang carries arguments, and an interpreter with one is still that
// interpreter.
//
// posix matched the tail of the shebang line, so `#!/bin/sh -e` — an ordinary
// hardening flag on an ordinary hook — read as a script written in something scc
// must not append shell to, and `--force` refused it. The test states the
// interpreters rather than the spellings, because the spellings are what was
// wrong.
func TestPosixReadsTheInterpreterNotTheLineEnding(t *testing.T) {
	for _, tc := range []struct {
		line string
		want bool
	}{
		{"#!/bin/sh", true},
		{"#!/bin/sh -e", true},
		{"#!/bin/sh -eu", true},
		{"#!/usr/bin/env sh", true},
		{"#!/usr/bin/env bash", true},
		{"#!/usr/bin/env bash -eu", true},
		{"#!/usr/bin/env -S bash -eu", true},
		{"#!/bin/bash", true},
		{"#!/usr/local/bin/zsh -f", true},
		{"", true}, // no shebang: git runs it with sh
		{"#!/usr/bin/env python3", false},
		{"#!/usr/bin/python", false},
		{"#!/usr/bin/env node", false},
		{"#!/usr/bin/perl -w", false},
	} {
		if got := posix(tc.line + "\necho hi\n"); got != tc.want {
			t.Errorf("posix(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

// Look changes nothing, which is the whole of what `scc hooks check` needs and
// the one thing a report must never get wrong: a check that installed what it
// was asked to report on would make the answer true by writing it.
func TestLookReportsWithoutWriting(t *testing.T) {
	dir := repo(t)

	all, err := Look(dir)
	if err != nil {
		t.Fatalf("Look: %v", err)
	}
	if len(all) != len(Stages()) {
		t.Fatalf("got %d stages, want %d", len(all), len(Stages()))
	}
	for _, s := range all {
		if s.State != Missing {
			t.Errorf("%s: state=%q on a repository with no hooks", s.Stage, s.State)
		}
		if _, err := os.Stat(s.Path); !os.IsNotExist(err) {
			t.Errorf("Look created %s", s.Path)
		}
	}
	if OK(all) {
		t.Error("OK true for a repository with no hooks installed")
	}

	if _, err := Install(dir, false); err != nil {
		t.Fatalf("Install: %v", err)
	}
	all, err = Look(dir)
	if err != nil {
		t.Fatalf("Look: %v", err)
	}
	if !OK(all) {
		t.Errorf("OK false right after a successful install: %+v", all)
	}

	// OK over nothing is false, and that is deliberate: an empty report means the
	// question could not be answered, not that everything is fine.
	if OK(nil) {
		t.Error("OK(nil) is true, so an unanswerable check reads as a pass")
	}
	if _, err := Look(t.TempDir()); err == nil {
		t.Error("Look outside a repository returned no error")
	}
}

// A hook scc appended to keeps everything of theirs when scc's block comes back
// out — the other half of the promise --force makes.
func TestRemoveLeavesAForeignHookItAppendedTo(t *testing.T) {
	dir := repo(t)
	hooks, err := Dir(dir)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	path := filepath.Join(hooks, string(PreCommit))
	const theirs = "#!/bin/sh\necho theirs\n"
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(theirs), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := Install(dir, true); err != nil {
		t.Fatalf("Install --force: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "echo theirs") {
		t.Fatalf("--force overwrote the existing hook:\n%s", raw)
	}

	all, err := Remove(dir)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	for _, s := range all {
		if s.Stage != PreCommit {
			continue
		}
		if s.Action != Removed || s.State != Foreign {
			t.Errorf("pre-commit: state=%q action=%q, want the foreign script left behind", s.State, s.Action)
		}
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "echo theirs") {
		t.Errorf("remove took the user's own hook with it:\n%s", raw)
	}
	if strings.Contains(string(raw), Prog+" hooks run") {
		t.Errorf("remove left scc's block behind:\n%s", raw)
	}

	// A second remove has nothing of scc's to take out.
	all, _ = Remove(dir)
	for _, s := range all {
		if s.Action != Skipped {
			t.Errorf("second remove: %s action=%q, want %q", s.Stage, s.Action, Skipped)
		}
	}
}
