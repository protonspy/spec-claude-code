package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/paths"
)

// A plan is one file, not a directory: it holds no state beyond its own checklist,
// and where an item references a spec the state lives in that spec.
func TestPlanNewWritesOneFile(t *testing.T) {
	root := initWorkspace(t)
	if _, stderr, code := run(t, "plan", "new", "checkout-revamp", "--root", root); code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	path := paths.Plan(root, "checkout-revamp")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Error("a plan was created as something other than a regular file")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(b), "Checkout revamp") || strings.Contains(string(b), "{{") {
		t.Errorf("plan was not rendered:\n%s", b)
	}
	// A plan is work, not knowledge, so it sits beside specs/ and never under docs/.
	if _, err := os.Stat(filepath.Join(paths.Docs(root), "plans")); err == nil {
		t.Error("a plans/ directory appeared under docs/")
	}
}

func TestPlanNewRecordsTheKickoffAnswers(t *testing.T) {
	root := initWorkspace(t)
	if _, stderr, code := run(t, "plan", "new", "tidy-up", "--root", root,
		"--autonomy", "gated", "--ci", "no-wait"); code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	stdout, _, code := run(t, "plan", "list", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("list: exit = %d", code)
	}
	var got struct {
		Plans []struct {
			Name     string `json:"name"`
			Path     string `json:"path"`
			Autonomy string `json:"autonomy"`
			CI       string `json:"ci"`
		} `json:"plans"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if got.Count != 1 {
		t.Fatalf("count = %d, want 1", got.Count)
	}
	p := got.Plans[0]
	if p.Autonomy != "gated" || p.CI != "no-wait" {
		t.Errorf("answers not read back: %+v", p)
	}
	// Paths in output are slash-separated so they match the manifest and read the
	// same on every platform.
	if p.Path != "plans/tidy-up.md" {
		t.Errorf("path = %q, want %q", p.Path, "plans/tidy-up.md")
	}
}

func TestPlanRejectsHostileNames(t *testing.T) {
	root := initWorkspace(t)
	for _, name := range []string{"..", ".", "a/b", "/abs", "Not-Kebab", ""} {
		for _, verb := range []string{"new", "delete"} {
			args := []string{"plan", verb, name, "--root", root}
			if verb == "delete" {
				args = append(args, "--force")
			}
			if _, _, code := run(t, args...); code != ExitError {
				t.Errorf("plan %s %q: exit = %d, want %d", verb, name, code, ExitError)
			}
		}
	}
	if _, err := os.Stat(paths.Plans(root)); err != nil {
		t.Fatalf("plans/ was damaged by a rejected name: %v", err)
	}
}

func TestPlanNewRefusesToClobberWithoutForce(t *testing.T) {
	root := initWorkspace(t)
	if _, _, code := run(t, "plan", "new", "sweep", "--root", root); code != ExitOK {
		t.Fatal("setup failed")
	}
	mine := "# my plan\n"
	if err := os.WriteFile(paths.Plan(root, "sweep"), []byte(mine), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, _, code := run(t, "plan", "new", "sweep", "--root", root); code != ExitError {
		t.Error("plan new over an existing plan did not fail")
	}
	if b, _ := os.ReadFile(paths.Plan(root, "sweep")); string(b) != mine {
		t.Error("a refused `plan new` still overwrote the file")
	}
	if _, _, code := run(t, "plan", "new", "sweep", "--root", root, "--force"); code != ExitOK {
		t.Error("plan new --force failed")
	}
}

func TestPlanDeleteRequiresForce(t *testing.T) {
	root := initWorkspace(t)
	if _, _, code := run(t, "plan", "new", "doomed", "--root", root); code != ExitOK {
		t.Fatal("setup failed")
	}
	if _, _, code := run(t, "plan", "delete", "doomed", "--root", root); code != ExitError {
		t.Error("delete without --force did not fail")
	}
	if _, err := os.Stat(paths.Plan(root, "doomed")); err != nil {
		t.Fatalf("the plan was deleted without --force: %v", err)
	}
	if _, _, code := run(t, "plan", "delete", "doomed", "--root", root, "--force"); code != ExitOK {
		t.Error("delete --force failed")
	}
	if _, err := os.Stat(paths.Plan(root, "doomed")); err == nil {
		t.Error("delete --force left the plan behind")
	}
}

func TestPlanListOnAnEmptyWorkspace(t *testing.T) {
	root := initWorkspace(t)
	stdout, _, code := run(t, "plan", "list", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout, `"plans": []`) || !strings.Contains(stdout, `"count": 0`) {
		t.Errorf("empty list = %s, want an empty array and a zero count", stdout)
	}
}

// A directory that happens to sit in plans/ is not a plan, and neither is a
// non-Markdown file.
func TestPlanListIgnoresNonPlans(t *testing.T) {
	root := initWorkspace(t)
	if err := os.MkdirAll(filepath.Join(paths.Plans(root), "not-a-plan.md"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.Plans(root), "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	stdout, _, code := run(t, "plan", "list", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout, `"count": 0`) {
		t.Errorf("list picked up something that is not a plan: %s", stdout)
	}
}

func TestPlanCommandsRequireAWorkspace(t *testing.T) {
	bare := t.TempDir()
	for _, args := range [][]string{
		{"plan", "new", "x", "--root", bare},
		{"plan", "list", "--root", bare},
		{"plan", "delete", "x", "--root", bare, "--force"},
	} {
		_, stderr, code := run(t, args...)
		if code != ExitError {
			t.Errorf("%v: exit = %d, want %d", args, code, ExitError)
		}
		if !strings.Contains(stderr, "not an scc workspace") {
			t.Errorf("%v: stderr = %q, want it to say to run init", args, stderr)
		}
	}
}

func TestPlanUsageAndUnknownSubcommand(t *testing.T) {
	if _, stderr, code := run(t, "plan"); code != ExitError || !strings.Contains(stderr, "Usage:") {
		t.Errorf("bare `plan` = (%d, %q), want a usage error", code, stderr)
	}
	if _, stderr, code := run(t, "plan", "nope"); code != ExitError || !strings.Contains(stderr, "unknown plan subcommand") {
		t.Errorf("`plan nope` = (%d, %q), want an unknown-subcommand error", code, stderr)
	}
	if _, _, code := run(t, "plan", "help"); code != ExitOK {
		t.Error("`plan help` did not exit 0")
	}
}
