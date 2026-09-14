package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
