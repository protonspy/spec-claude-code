package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/hooks"
)

// driftRepo is a workspace on a base branch with one commit, pushed to a remote.
//
// All three parts are load-bearing rather than ceremony. A commit on main is what
// gives base..HEAD anything to mean. The *remote* is what makes "sitting on the
// base branch" answerable at all: standing on main, the honest comparison is
// against origin/main — comparing main to itself is empty, and a check run that
// way would silently pass on every commit in the repository. A repository with no
// remote genuinely has nothing to say there, which is the same "absence is a
// normal answer" line internal/git holds everywhere else.
func driftRepo(t *testing.T) string {
	t.Helper()
	root := gitWorkspace(t)
	gitDo(t, root, "config", "user.email", "t@example.invalid")
	gitDo(t, root, "config", "user.name", "t")
	gitDo(t, root, "checkout", "-q", "-B", "main")
	gitDo(t, root, "add", "-A")
	gitDo(t, root, "commit", "-q", "-m", "chore: scaffold")

	remote := filepath.Join(t.TempDir(), "origin.git")
	cmd := exec.Command("git", "init", "-q", "--bare", remote)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, out)
	}
	gitDo(t, root, "remote", "add", "origin", remote)
	gitDo(t, root, "push", "-q", "-u", "origin", "main")
	return root
}

// gitDo runs one git command in the fixture, with scc's own git hooks skipped.
//
// The fixture is a scaffolded workspace, and `scc init` installs those hooks by
// default — so a plain `git commit` here fires `pre-commit`, which fails closed
// when `scc` is not on PATH. That is the hook behaving exactly as designed: a
// gate that passed silently when its checker is missing would report success it
// did not verify. It is still the wrong thing to run *inside a test*, where the
// subject is drift detection and the binary under test has not been installed
// anywhere.
//
// Caught by CI rather than locally, which is the whole lesson: this passed on a
// machine that happens to have `scc` on PATH and failed on every machine that
// does not. SCC_SKIP_HOOKS is the documented escape hatch and this is what it is
// for.
func gitDo(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), hooks.SkipEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", rel, err)
	}
}

// marker builds one of the words the rule forbids without this file containing
// it as a word, so the check cannot report the test that exercises it.
func marker(s string) string { return s }

// TestTheDriftStageIsSilentOnACleanBranch is the property this whole stage rests
// on, and the one least likely to survive a later edit unnoticed.
//
// A stage that speaks every turn is a stage the reader learns to skip, and then
// the turn it had something real to say is the turn nobody read it. That failure
// is invisible in review — every individual line looks useful — so it is pinned
// here rather than left to inspection. Three shapes of quiet branch, because each
// one is a different way the check could start chattering.
func TestTheDriftStageIsSilentOnACleanBranch(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(t *testing.T, root string)
	}{
		{"a branch with nothing on it", func(*testing.T, string) {}},
		{"source changed and a box ticked", func(t *testing.T, root string) {
			write(t, root, "main.go", "package main\n\nfunc main() {}\n")
			write(t, root, "plans/p.md", "# P\n\n## Tasks\n\n- [x] 1.1 (Unit) done\n")
		}},
		{"only artifacts changed", func(t *testing.T, root string) {
			write(t, root, "docs/glossary.md", "# Glossary\n\n- **Thing** — a thing.\n")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := driftRepo(t)
			gitDo(t, root, "checkout", "-q", "-b", "feat/x")
			tc.set(t, root)
			if got := driftLines(root); len(got) != 0 {
				t.Errorf("a clean branch drew %d drift line(s): %v", len(got), got)
			}
		})
	}
}

// Source changed and nothing ticked is the shape of code written outside the
// methodology: no task cited, and nothing that will read as progress when
// somebody asks what this branch did.
func TestDriftReportsSourceChangedWithNoBoxTicked(t *testing.T) {
	root := driftRepo(t)
	gitDo(t, root, "checkout", "-q", "-b", "feat/x")
	write(t, root, "internal/thing/thing.go", "package thing\n\nfunc New() {}\n")

	got := strings.Join(driftLines(root), "\n")
	if !strings.Contains(got, "ticked no box") {
		t.Fatalf("no unticked line: %q", got)
	}
	if !strings.Contains(got, "thing/thing.go") {
		t.Errorf("the line names no file: %q", got)
	}

	// And it goes quiet the moment a box is ticked, without the file being
	// committed — the whole point of comparing against the working tree is that
	// the turn which did the work is the turn that gets told.
	write(t, root, "plans/p.md", "# P\n\n## Tasks\n\n- [x] 1.1 (Unit) build the thing\n")
	if got := strings.Join(driftLines(root), "\n"); strings.Contains(got, "ticked no box") {
		t.Errorf("still reporting an unticked branch after a box was ticked: %q", got)
	}
}

// A marker in code is the one signal with no validator behind it and no way to
// get one: every validator reads an artifact, and this is about source.
func TestDriftReportsAMarkerLeftInCode(t *testing.T) {
	root := driftRepo(t)
	gitDo(t, root, "checkout", "-q", "-b", "feat/x")
	write(t, root, "internal/thing/thing.go",
		"package thing\n\n// "+marker("TO")+marker("DO")+": handle the empty case\nfunc New() {}\n")
	write(t, root, "plans/p.md", "# P\n\n## Tasks\n\n- [x] 1.1 (Unit) build it\n")

	got := strings.Join(driftLines(root), "\n")
	if !strings.Contains(got, "marker") {
		t.Fatalf("no marker line: %q", got)
	}
	if !strings.Contains(got, "notes add") {
		t.Errorf("the line says what is wrong but not where the note goes: %q", got)
	}
}

// TestDriftIgnoresAMarkerThatIsNotAnAnnotation is what keeps the marker scan
// worth acting on.
//
// The rule forbids a marker left as an annotation, and an annotation lives in a
// comment. A word inside a string literal is data — a linter's configuration, a
// fixture, or the very list this check is built from, which would otherwise make
// the check report the file that defines it. Markdown is excluded for the
// neighbouring reason: a marker in prose is prose.
func TestDriftIgnoresAMarkerThatIsNotAnAnnotation(t *testing.T) {
	root := driftRepo(t)
	gitDo(t, root, "checkout", "-q", "-b", "feat/x")
	write(t, root, "internal/thing/thing.go",
		"package thing\n\nvar markers = []string{\""+marker("TO")+marker("DO")+"\"}\n")
	write(t, root, "docs/wiki/pages/x.md",
		"# X\n\n"+marker("TO")+marker("DO")+": write this page.\n")
	write(t, root, "plans/p.md", "# P\n\n## Tasks\n\n- [x] 1.1 (Unit) build it\n")

	if got := strings.Join(driftLines(root), "\n"); strings.Contains(got, "marker") {
		t.Errorf("reported a marker that is not an annotation: %q", got)
	}
}

// Work on the base branch is what delivery.md says does not happen, and the fix
// has the shortest window of the three: `git switch -c` is a different act once
// the commits have been pushed.
func TestDriftReportsCommitsOnTheBaseBranch(t *testing.T) {
	root := driftRepo(t)
	write(t, root, "internal/thing/thing.go", "package thing\n")
	gitDo(t, root, "add", "-A")
	gitDo(t, root, "commit", "-q", "-m", "feat: a thing")

	got := strings.Join(driftLines(root), "\n")
	if !strings.Contains(got, "work does not happen on") {
		t.Fatalf("no base-branch line: %q", got)
	}
	if !strings.Contains(got, "git switch -c") {
		t.Errorf("the line names the problem but not the move out of it: %q", got)
	}
}

// TestDriftReachesTheAgentThroughTheStopStage closes the loop: the lines are
// worth nothing if the stage does not carry them, and the stage is worth nothing
// if carrying them lets it block a turn.
func TestDriftReachesTheAgentThroughTheStopStage(t *testing.T) {
	root := driftRepo(t)
	gitDo(t, root, "checkout", "-q", "-b", "feat/x")
	write(t, root, "internal/thing/thing.go", "package thing\n\nfunc New() {}\n")

	stdout, _, code := run(t, "hooks", "run", "stop", "--root", root)
	if code != ExitOK {
		t.Fatalf("the Stop stage exited %d; it reports and never refuses, because a hook "+
			"that can block a turn can loop one", code)
	}
	if ctx := additionalContext(t, stdout); !strings.Contains(ctx, "ticked no box") {
		t.Errorf("the drift line never reached the agent: %q", ctx)
	}
}

// TestDriftKeepsNoStateBetweenTurns pins the stateless half.
//
// A session snapshot would need a file to live in, and this workspace has one
// file per harness on purpose. The branch is also the better unit: delivery.md
// already makes one branch one piece of work. So two runs of the same stage over
// an unchanged tree must answer identically — a stage that went quiet the second
// time would be remembering something, and a stage that said more would be
// accumulating.
func TestDriftKeepsNoStateBetweenTurns(t *testing.T) {
	root := driftRepo(t)
	gitDo(t, root, "checkout", "-q", "-b", "feat/x")
	write(t, root, "internal/thing/thing.go", "package thing\n")

	first := strings.Join(driftLines(root), "\n")
	second := strings.Join(driftLines(root), "\n")
	if first != second {
		t.Errorf("two runs over an unchanged tree disagreed:\n first: %q\nsecond: %q", first, second)
	}
	if first == "" {
		t.Fatal("the fixture drew no drift line, so this proves nothing")
	}
	// And nothing was written to say so. The whole workspace is one manifest per
	// harness; a stage that had started keeping a baseline would show up here.
	for _, seg := range []string{".scc", ".scc-state", "scc-drift.json"} {
		if _, err := os.Stat(filepath.Join(root, seg)); err == nil {
			t.Errorf("the drift stage left %s behind; the baseline is the branch, not a file", seg)
		}
	}
}

// TestMarkerInPinsWhereACommentMayStart is the unit behind the marker scan, and
// the shapes here are the ones that decide whether it is worth reading.
//
// A trailing `//` or `#` comment is the case the rule is actually about. `*`,
// `--` and `;` are not: taken anywhere on a line they turn a pointer, a
// command-line flag and a statement terminator into comment openers, and the
// scan starts reporting test code that passes a marker as an argument. One false
// positive is enough to teach a reader to skip the line every time.
func TestMarkerInPinsWhereACommentMayStart(t *testing.T) {
	td := marker("TO") + marker("DO")
	for _, tc := range []struct {
		line string
		want bool
	}{
		{"// " + td + ": handle this", true},
		{"x := 1 // " + td + " later", true},
		{"    # " + td + ": python style", true},
		{" * " + td + ": a block comment continuation", true},
		{"<!-- " + td + ": in markup -->", true},

		{"markers := []string{\"" + td + "\"}", false},
		{"exec.Command(scc, \"--tag\", \"" + td + "\")", false},
		{"n := a * b // nothing here", false},
		{"want := count; got := " + td + "Count", false},
		{"path := \"docs/" + td + "S.md\"", false},
		{"// nothing to see", false},
		{"", false},
	} {
		if got := markerIn(tc.line) != ""; got != tc.want {
			t.Errorf("markerIn(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

// TestTheUntickedSignalIsSilentWhereThereIsNowhereToTick is the bound that came
// out of running the stage against scc's own repository instead of a fixture.
//
// scc's repo is not an scc workspace: no `plans/`, no `specs/`. So "source
// changed and nothing was ticked" is true of every turn, forever, and the line
// was wallpaper on its first real outing — which is precisely the failure the
// stage's whole design is organized against, arriving through the one case no
// fixture had. A repository with no artifacts is not drifting from a methodology
// it never adopted.
func TestTheUntickedSignalIsSilentWhereThereIsNowhereToTick(t *testing.T) {
	root := driftRepo(t)
	gitDo(t, root, "checkout", "-q", "-b", "feat/x")
	// Remove what `scc init` scaffolded, leaving a plain git repository that
	// happens to be where an agent is working.
	for _, seg := range []string{"plans", "specs"} {
		if err := os.RemoveAll(filepath.Join(root, seg)); err != nil {
			t.Fatalf("RemoveAll %s: %v", seg, err)
		}
	}
	write(t, root, "internal/thing/thing.go", "package thing\n\nfunc New() {}\n")

	if got := strings.Join(driftLines(root), "\n"); strings.Contains(got, "ticked no box") {
		t.Errorf("a repository with nowhere to tick a box was told it ticked no box: %q", got)
	}

	// And it comes back the moment the project has artifacts, because then the
	// signal means something again.
	write(t, root, "plans/p.md", "# P\n\n## Tasks\n\n- [ ] 1.1 (Unit) not done\n")
	if got := strings.Join(driftLines(root), "\n"); !strings.Contains(got, "ticked no box") {
		t.Errorf("a workspace with an untouched plan drew no unticked line: %q", got)
	}
}
