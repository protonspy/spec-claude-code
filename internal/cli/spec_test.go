package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/paths"
)

func TestSpecNewWritesTheThreeArtifacts(t *testing.T) {
	root := initWorkspace(t)
	_, stderr, code := run(t, "spec", "new", "user-auth", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	for _, path := range []string{
		paths.Requirements(root, "user-auth"),
		paths.Design(root, "user-auth"),
		paths.Tasks(root, "user-auth"),
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if !strings.Contains(string(b), "User auth") {
			t.Errorf("%s: the feature title was not rendered", path)
		}
		if strings.Contains(string(b), "{{") {
			t.Errorf("%s: an unrendered template action shipped into the artifact", path)
		}
	}
}

// The kickoff answers are recorded on the artifact so the run is reproducible from
// the file and nobody is asked twice.
func TestSpecNewRecordsTheKickoffAnswers(t *testing.T) {
	root := initWorkspace(t)
	if _, stderr, code := run(t, "spec", "new", "billing", "--root", root,
		"--autonomy", "gated", "--ci", "no-wait"); code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	b, err := os.ReadFile(paths.Requirements(root, "billing"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(b), "autonomy: gated") || !strings.Contains(string(b), "ci: no-wait") {
		t.Errorf("frontmatter did not record the answers:\n%s", b)
	}

	stdout, _, code := run(t, "spec", "show", "billing", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("show: exit = %d", code)
	}
	var got struct {
		Name         string `json:"name"`
		Autonomy     string `json:"autonomy"`
		CI           string `json:"ci"`
		Requirements bool   `json:"requirements"`
		Design       bool   `json:"design"`
		Tasks        bool   `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if got.Autonomy != "gated" || got.CI != "no-wait" {
		t.Errorf("show did not read the answers back: %+v", got)
	}
	if !got.Requirements || !got.Design || !got.Tasks {
		t.Errorf("show did not see all three artifacts: %+v", got)
	}
}

// A typo'd answer recorded verbatim would read as an answer nobody gave.
func TestSpecNewRejectsAnUnknownKickoffAnswer(t *testing.T) {
	root := initWorkspace(t)
	for _, args := range [][]string{
		{"spec", "new", "a", "--root", root, "--autonomy", "automatic"},
		{"spec", "new", "a", "--root", root, "--ci", "later"},
	} {
		_, stderr, code := run(t, args...)
		if code != ExitError {
			t.Errorf("%v: exit = %d, want %d", args, code, ExitError)
		}
		if !strings.Contains(stderr, "must be") {
			t.Errorf("%v: stderr = %q, want it to name the valid values", args, stderr)
		}
		if _, err := os.Stat(paths.Spec(root, "a")); err == nil {
			t.Errorf("%v: the spec was created despite the invalid flag", args)
		}
	}
}

// SafeName stands between a CLI argument and filepath.Join. Without it
// `scc spec delete ..` resolves to the workspace root and takes the project with it.
func TestSpecRejectsHostileNames(t *testing.T) {
	root := initWorkspace(t)
	marker := filepath.Join(root, filepath.FromSlash(".claude/rules/routing.md"))
	for _, name := range []string{"..", ".", "../escape", "a/b", "/abs", "Not-Kebab", "under_score", ""} {
		for _, verb := range []string{"new", "show", "delete"} {
			args := []string{"spec", verb, name, "--root", root}
			if verb == "delete" {
				args = append(args, "--force")
			}
			if _, _, code := run(t, args...); code != ExitError {
				t.Errorf("spec %s %q: exit = %d, want %d", verb, name, code, ExitError)
			}
		}
	}
	// The workspace is still there: no traversal reached anything.
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("a rejected name still damaged the workspace: %v", err)
	}
	if _, err := os.Stat(paths.Specs(root)); err != nil {
		t.Fatalf("specs/ was removed by a rejected name: %v", err)
	}
}

func TestSpecNewRefusesToClobberWithoutForce(t *testing.T) {
	root := initWorkspace(t)
	if _, _, code := run(t, "spec", "new", "cart", "--root", root); code != ExitOK {
		t.Fatal("setup failed")
	}
	mine := "# my requirements\n"
	if err := os.WriteFile(paths.Requirements(root, "cart"), []byte(mine), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, stderr, code := run(t, "spec", "new", "cart", "--root", root)
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "already exists") {
		t.Errorf("stderr = %q, want it to say the spec already exists", stderr)
	}
	b, _ := os.ReadFile(paths.Requirements(root, "cart"))
	if string(b) != mine {
		t.Error("a refused `spec new` still overwrote the file")
	}

	if _, stderr, code := run(t, "spec", "new", "cart", "--root", root, "--force"); code != ExitOK {
		t.Fatalf("--force: exit = %d (stderr: %s)", code, stderr)
	}
	b, _ = os.ReadFile(paths.Requirements(root, "cart"))
	if string(b) == mine {
		t.Error("--force did not overwrite")
	}
}

func TestSpecListIsSortedAndJSON(t *testing.T) {
	root := initWorkspace(t)
	for _, name := range []string{"zebra", "alpha", "middle"} {
		if _, _, code := run(t, "spec", "new", name, "--root", root); code != ExitOK {
			t.Fatalf("creating %s failed", name)
		}
	}
	stdout, _, code := run(t, "spec", "list", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	var got struct {
		Specs []struct{ Name string } `json:"specs"`
		Count int                     `json:"count"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Count != 3 {
		t.Fatalf("count = %d, want 3", got.Count)
	}
	if got.Specs[0].Name != "alpha" || got.Specs[2].Name != "zebra" {
		t.Errorf("list is not sorted: %+v", got.Specs)
	}
}

// An empty workspace is not an error, and the empty JSON document still has both
// keys so a caller can index the array without a nil check.
func TestSpecListOnAnEmptyWorkspace(t *testing.T) {
	root := initWorkspace(t)
	stdout, _, code := run(t, "spec", "list", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout, `"specs": []`) || !strings.Contains(stdout, `"count": 0`) {
		t.Errorf("empty list = %s, want an empty array and a zero count", stdout)
	}
}

func TestSpecDeleteRequiresForce(t *testing.T) {
	root := initWorkspace(t)
	if _, _, code := run(t, "spec", "new", "doomed", "--root", root); code != ExitOK {
		t.Fatal("setup failed")
	}
	if _, _, code := run(t, "spec", "delete", "doomed", "--root", root); code != ExitError {
		t.Error("delete without --force did not fail")
	}
	if _, err := os.Stat(paths.Spec(root, "doomed")); err != nil {
		t.Fatalf("the spec was deleted without --force: %v", err)
	}
	if _, stderr, code := run(t, "spec", "delete", "doomed", "--root", root, "--force"); code != ExitOK {
		t.Fatalf("delete --force: exit = %d (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(paths.Spec(root, "doomed")); err == nil {
		t.Error("delete --force left the spec behind")
	}
}

func TestSpecDeleteUnknownFeature(t *testing.T) {
	root := initWorkspace(t)
	_, stderr, code := run(t, "spec", "delete", "ghost", "--root", root, "--force")
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "ghost") {
		t.Errorf("stderr = %q, want it to name the missing spec", stderr)
	}
}

// Outside a workspace these commands would otherwise act on whatever directory the
// upward walk fell back to — for a command run from $HOME, the user's own tree.
func TestSpecCommandsRequireAWorkspace(t *testing.T) {
	bare := t.TempDir()
	for _, args := range [][]string{
		{"spec", "new", "x", "--root", bare},
		{"spec", "list", "--root", bare},
		{"spec", "show", "x", "--root", bare},
		{"spec", "delete", "x", "--root", bare, "--force"},
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

func TestSpecUsageAndUnknownSubcommand(t *testing.T) {
	if _, stderr, code := run(t, "spec"); code != ExitError || !strings.Contains(stderr, "Usage:") {
		t.Errorf("bare `spec` = (%d, %q), want a usage error", code, stderr)
	}
	if _, stderr, code := run(t, "spec", "nope"); code != ExitError || !strings.Contains(stderr, "unknown spec subcommand") {
		t.Errorf("`spec nope` = (%d, %q), want an unknown-subcommand error", code, stderr)
	}
	if _, _, code := run(t, "spec", "help"); code != ExitOK {
		t.Error("`spec help` did not exit 0")
	}
}
