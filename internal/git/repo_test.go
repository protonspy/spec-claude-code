package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A repository built for one test, isolated from whoever is running the suite:
// GIT_CONFIG_GLOBAL and GIT_CONFIG_SYSTEM point at nothing, so a developer's own
// commit.gpgsign, core.hooksPath or diff.noprefix cannot reach in and change what
// these tests measure — which is the same class of defect Changed's pinned
// prefixes exist for.
func repo(t *testing.T) string {
	t.Helper()
	if !Found(Bin) {
		t.Skip("git is not on PATH")
	}
	empty := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_SYSTEM", empty)

	dir := t.TempDir()
	git(t, dir, "init", "-b", DefaultBase)
	git(t, dir, "config", "user.name", "Test")
	git(t, dir, "config", "user.email", "test@example.invalid")
	git(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(Bin, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit writes path and commits it, so a test reads as the history it is
// building rather than as four git calls.
func commit(t *testing.T, dir, path, body, message string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	git(t, dir, "add", "--", path)
	git(t, dir, "commit", "-m", message)
}

func containsLine(lines []string, want string) bool {
	for _, l := range lines {
		if strings.TrimSpace(l) == want {
			return true
		}
	}
	return false
}

// fakeBin writes an executable named name into its own directory and returns that
// directory, so a test can put it on PATH and drive a query without the real tool.
func fakeBin(t *testing.T, name, stdout string, exit int) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		body := "@echo off\r\n"
		if stdout != "" {
			body += "echo " + stdout + "\r\n"
		}
		body += fmt.Sprintf("exit /b %d\r\n", exit)
		writeFile(t, filepath.Join(dir, name+".cmd"), body, 0o644)
		return dir
	}
	body := "#!/bin/sh\n"
	if stdout != "" {
		body += "printf '%s\n' " + shellQuote(stdout) + "\n"
	}
	body += fmt.Sprintf("exit %d\n", exit)
	writeFile(t, filepath.Join(dir, name), body, 0o755)
	return dir
}

// shellQuote wraps s for /bin/sh, which has one escape for a single-quoted string
// and it is to leave the string, quote a quote, and go back in.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func writeFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func TestIsRepoAndCurrentBranch(t *testing.T) {
	dir := repo(t)
	if !IsRepo(dir) {
		t.Fatal("IsRepo false inside a repository")
	}
	commit(t, dir, "a.txt", "one\n", "feat: one")

	got, err := CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if got != DefaultBase {
		t.Errorf("CurrentBranch = %q, want %q", got, DefaultBase)
	}

	// A detached head has no branch, and "" is the answer rather than the literal
	// "HEAD" git prints — a caller comparing against a base would otherwise match
	// a branch nobody is on.
	git(t, dir, "checkout", "--detach", "HEAD")
	if got, err := CurrentBranch(dir); err != nil || got != "" {
		t.Errorf("detached CurrentBranch = %q, %v — want the empty answer", got, err)
	}

	outside := t.TempDir()
	if IsRepo(outside) {
		t.Error("IsRepo true outside any repository")
	}
	if _, err := CurrentBranch(outside); err == nil {
		t.Error("CurrentBranch outside a repository returned no error")
	}
}

// "main" has been wrong for every repository created before 2020 and plenty
// since, so the remote's own HEAD is asked first and the fallback is a guess the
// caller is told about.
func TestBaseAsksTheRemoteBeforeGuessing(t *testing.T) {
	dir := repo(t)
	commit(t, dir, "a.txt", "one\n", "feat: one")
	if got := Base(dir); got != DefaultBase {
		t.Errorf("Base = %q on a main-only repository, want %q", got, DefaultBase)
	}

	old := repo(t)
	// The initial branch has no commits yet, so switching leaves no main behind at
	// all — which is the shape of a repository that only ever had master.
	git(t, old, "checkout", "-b", "master")
	commit(t, old, "a.txt", "one\n", "feat: one")
	if got := Base(old); got != "master" {
		t.Errorf("Base = %q on a master-only repository, want master", got)
	}

	remote := t.TempDir()
	git(t, remote, "init", "--bare", "-b", "trunk")
	named := repo(t)
	git(t, named, "checkout", "-b", "trunk")
	commit(t, named, "a.txt", "one\n", "feat: one")
	git(t, named, "remote", "add", "origin", remote)
	git(t, named, "push", "-u", "origin", "trunk")
	git(t, named, "remote", "set-head", "origin", "trunk")
	if got := Base(named); got != "trunk" {
		t.Errorf("Base = %q, want the remote's own HEAD, trunk", got)
	}
}

// Merged is ahead == 0 AND behind > 0. A branch created ten seconds ago is
// trivially an ancestor of its base, and reading that as delivered work is the
// defect this spelling exists to prevent.
func TestLookTellsAFreshBranchFromADeliveredOne(t *testing.T) {
	dir := repo(t)
	commit(t, dir, "a.txt", "one\n", "feat: one")

	git(t, dir, "checkout", "-b", "feat/fresh")
	fresh, err := Look(dir, "feat/fresh", DefaultBase)
	if err != nil {
		t.Fatalf("Look: %v", err)
	}
	if !fresh.Local || fresh.Remote {
		t.Errorf("fresh branch = %+v, want local only", fresh)
	}
	if fresh.Merged {
		t.Error("a branch with no commits of its own was reported merged")
	}

	commit(t, dir, "b.txt", "two\n", "feat: two")
	ahead, _ := Look(dir, "feat/fresh", DefaultBase)
	if ahead.Ahead != 1 || ahead.Behind != 0 || ahead.Merged {
		t.Errorf("branch with one commit = %+v, want ahead 1 and not merged", ahead)
	}

	git(t, dir, "checkout", DefaultBase)
	git(t, dir, "merge", "--no-ff", "-m", "merge", "feat/fresh")
	landed, _ := Look(dir, "feat/fresh", DefaultBase)
	if landed.Ahead != 0 || landed.Behind == 0 || !landed.Merged {
		t.Errorf("landed branch = %+v, want ahead 0, behind > 0, merged", landed)
	}

	git(t, dir, "branch", "-D", "feat/fresh")
	gone, _ := Look(dir, "feat/fresh", DefaultBase)
	if gone.Exists() || gone.Merged {
		t.Errorf("deleted branch = %+v, want neither existing nor merged", gone)
	}

	filled, _ := Look(dir, DefaultBase, "")
	if filled.Base == "" {
		t.Error("Look left the base empty instead of asking the repository")
	}
}

// Commits is "on HEAD and not on base", and the message comes back whole —
// trailers included, because the trailers are what a caller asking about
// authorship is asking about.
func TestCommitsAreWhatThisBranchAdded(t *testing.T) {
	dir := repo(t)
	commit(t, dir, "a.txt", "one\n", "feat: one")
	git(t, dir, "checkout", "-b", "feat/x")
	commit(t, dir, "b.txt", "two\n", "feat: two\n\nA body.\n\nCo-Authored-By: Someone <s@example.invalid>")

	got, err := Commits(dir, DefaultBase)
	if err != nil {
		t.Fatalf("Commits: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Commits returned %d, want the one this branch added: %+v", len(got), got)
	}
	c := got[0]
	if c.Subject != "feat: two" {
		t.Errorf("subject = %q", c.Subject)
	}
	if !strings.Contains(c.Message, "Co-Authored-By:") {
		t.Errorf("message lost its trailers: %q", c.Message)
	}
	if c.SHA == "" || c.Short == "" || !strings.HasPrefix(c.SHA, c.Short) {
		t.Errorf("sha = %q, short = %q", c.SHA, c.Short)
	}

	// Standing on the base branch with no remote leaves genuinely nothing to
	// compare against, and that is an empty answer rather than an error.
	git(t, dir, "checkout", DefaultBase)
	if got, err := Commits(dir, DefaultBase); err != nil || len(got) != 0 {
		t.Errorf("on the base branch: %d commits, %v — want none and no error", len(got), err)
	}
}

// On the base branch the comparison is against the remote's copy, so unpushed
// commits are still checked rather than silently passing.
func TestCommitsOnTheBaseBranchCompareAgainstTheRemote(t *testing.T) {
	remote := t.TempDir()
	dir := repo(t)
	git(t, remote, "init", "--bare", "-b", DefaultBase)
	commit(t, dir, "a.txt", "one\n", "feat: one")
	git(t, dir, "remote", "add", "origin", remote)
	git(t, dir, "push", "-u", "origin", DefaultBase)

	if got, _ := Commits(dir, DefaultBase); len(got) != 0 {
		t.Errorf("a pushed base branch reported %d commits", len(got))
	}
	commit(t, dir, "b.txt", "two\n", "feat: unpushed")
	got, err := Commits(dir, DefaultBase)
	if err != nil {
		t.Fatalf("Commits: %v", err)
	}
	if len(got) != 1 || got[0].Subject != "feat: unpushed" {
		t.Errorf("unpushed work on the base = %+v, want the one commit", got)
	}
}

func TestDirtyAndCommentChar(t *testing.T) {
	dir := repo(t)
	commit(t, dir, "a.txt", "one\n", "feat: one")
	if Dirty(dir) {
		t.Error("Dirty on a clean tree")
	}
	writeFile(t, filepath.Join(dir, "a.txt"), "changed\n", 0o644)
	if !Dirty(dir) {
		t.Error("Dirty false with a modified file")
	}
	git(t, dir, "checkout", "--", "a.txt")
	writeFile(t, filepath.Join(dir, "new.txt"), "x\n", 0o644)
	if !Dirty(dir) {
		t.Error("Dirty false with an untracked file")
	}

	if got := CommentChar(dir); got != "" {
		t.Errorf("CommentChar = %q on a repository that sets none", got)
	}
	git(t, dir, "config", "core.commentChar", ";")
	if got := CommentChar(dir); got != ";" {
		t.Errorf("CommentChar = %q, want %q", got, ";")
	}
	// "auto" comes back as itself: git resolves it against the message being
	// written, and the caller holding the message decides.
	git(t, dir, "config", "core.commentChar", "auto")
	if got := CommentChar(dir); got != "auto" {
		t.Errorf("CommentChar = %q, want auto verbatim", got)
	}
}

// Asked of git rather than assumed, because writing a hook where git will not
// read it is the kind of failure that looks like success for weeks.
func TestHooksDirHonoursTheConfiguredPath(t *testing.T) {
	dir := repo(t)
	commit(t, dir, "a.txt", "one\n", "feat: one")

	got, err := HooksDir(dir)
	if err != nil {
		t.Fatalf("HooksDir: %v", err)
	}
	if filepath.Base(got) != "hooks" || !filepath.IsAbs(got) {
		t.Errorf("HooksDir = %q, want an absolute path ending in hooks", got)
	}

	elsewhere := filepath.Join(t.TempDir(), "githooks")
	git(t, dir, "config", "core.hooksPath", filepath.ToSlash(elsewhere))
	got, err = HooksDir(dir)
	if err != nil {
		t.Fatalf("HooksDir with core.hooksPath: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean(elsewhere) {
		t.Errorf("HooksDir = %q, want the configured %q", got, elsewhere)
	}

	if _, err := HooksDir(t.TempDir()); err == nil {
		t.Error("HooksDir outside a repository returned no error")
	}
}

// Everything here is answered from refs already in the repository — no fetch, no
// gh — because the caller is a hook that runs at the end of every turn.
func TestUndeliveredWork(t *testing.T) {
	dir := repo(t)
	commit(t, dir, "a.txt", "one\n", "feat: one")

	u := UndeliveredWork(dir)
	if u.Any() || u.Branch != DefaultBase {
		t.Errorf("on the base branch = %+v, want nothing to report", u)
	}

	git(t, dir, "checkout", "-b", "feat/x")
	commit(t, dir, "b.txt", "two\n", "feat: two")
	u = UndeliveredWork(dir)
	if !u.Any() || u.Ahead != 1 || u.Unpushed != 1 || u.Pushed {
		t.Errorf("never-pushed branch = %+v, want one unpushed commit and Pushed false", u)
	}

	remote := t.TempDir()
	git(t, remote, "init", "--bare", "-b", DefaultBase)
	git(t, dir, "remote", "add", "origin", remote)
	git(t, dir, "push", "-u", "origin", "feat/x")
	u = UndeliveredWork(dir)
	if u.Any() || !u.Pushed {
		t.Errorf("pushed branch = %+v, want Pushed and nothing outstanding", u)
	}

	commit(t, dir, "c.txt", "three\n", "feat: three")
	u = UndeliveredWork(dir)
	if !u.Any() || u.Unpushed != 1 || !u.Pushed {
		t.Errorf("branch with one commit past the remote = %+v", u)
	}

	git(t, dir, "checkout", "--detach", "HEAD")
	if got := UndeliveredWork(dir); got.Any() {
		t.Errorf("detached head = %+v, want nothing", got)
	}
	if got := UndeliveredWork(t.TempDir()); got.Any() {
		t.Errorf("outside a repository = %+v, want nothing", got)
	}
}

// Changed reads against the working tree rather than HEAD, because at the end of
// a turn the agent has usually written code and not yet committed it.
func TestChangedSeesCommittedUncommittedAndUntracked(t *testing.T) {
	dir := repo(t)
	commit(t, dir, "a.txt", "one\n", "feat: one")
	git(t, dir, "checkout", "-b", "feat/x")

	commit(t, dir, "tracked.txt", "committed line\n", "feat: tracked")
	writeFile(t, filepath.Join(dir, "a.txt"), "one\nuncommitted line\n", 0o644)
	writeFile(t, filepath.Join(dir, "new.txt"), "untracked line\n", 0o644)

	changes, err := Changed(dir, DefaultBase)
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	added := map[string][]string{}
	for _, c := range changes {
		added[filepath.ToSlash(c.Path)] = c.Added
	}
	for path, want := range map[string]string{
		"tracked.txt": "committed line",
		"a.txt":       "uncommitted line",
		"new.txt":     "untracked line",
	} {
		lines, ok := added[path]
		if !ok {
			t.Errorf("%s is missing from %v", path, added)
			continue
		}
		if !containsLine(lines, want) {
			t.Errorf("%s added = %v, want it to carry %q", path, lines, want)
		}
	}

	// A binary blob is not read: scanning one finds nothing and costs everything.
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), []byte{'a', 0, 'b'}, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// And a gitignored file is git's own business.
	writeFile(t, filepath.Join(dir, ".gitignore"), "ignored.txt\n", 0o644)
	writeFile(t, filepath.Join(dir, "ignored.txt"), "hidden\n", 0o644)

	changes, _ = Changed(dir, DefaultBase)
	for _, c := range changes {
		switch filepath.ToSlash(c.Path) {
		case "blob.bin":
			t.Errorf("a binary file was read as text: %+v", c)
		case "ignored.txt":
			t.Error("an ignored file was reported as changed")
		}
	}
}

// Absence is a normal answer, never an error to propagate — the line this package
// holds everywhere. With no git on PATH every query says so, so a caller can tell
// "could not determine" from "determined nothing".
func TestEveryQueryDegradesWithoutGit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	if Found(Bin) {
		t.Skip("PATH could not be emptied on this platform")
	}

	if IsRepo(dir) || Dirty(dir) || CommentChar(dir) != "" {
		t.Error("a query answered as though git were here")
	}
	if _, err := CurrentBranch(dir); err != ErrUnavailable {
		t.Errorf("CurrentBranch err = %v, want ErrUnavailable", err)
	}
	if _, err := Look(dir, "x", DefaultBase); err != ErrUnavailable {
		t.Errorf("Look err = %v, want ErrUnavailable", err)
	}
	if _, err := Commits(dir, DefaultBase); err != ErrUnavailable {
		t.Errorf("Commits err = %v, want ErrUnavailable", err)
	}
	if _, err := Changed(dir, DefaultBase); err != ErrUnavailable {
		t.Errorf("Changed err = %v, want ErrUnavailable", err)
	}
	if _, err := HooksDir(dir); err == nil {
		t.Error("HooksDir returned no error with no git")
	}
	if got := UndeliveredWork(dir); got.Any() {
		t.Errorf("UndeliveredWork = %+v with no git", got)
	}
	if got := Base(dir); got != DefaultBase {
		t.Errorf("Base = %q with no git, want the documented fallback", got)
	}
	if _, err := LookPR(dir, 1); err != ErrUnavailable {
		t.Errorf("LookPR err = %v, want ErrUnavailable", err)
	}
	if _, err := CurrentPR(dir); err != ErrUnavailable {
		t.Errorf("CurrentPR err = %v, want ErrUnavailable", err)
	}
}

// The PR is the half no hook can reach, so the title and body come back with it.
// Driven against a stand-in gh: the real one would need a network and an account.
func TestViewPRReadsWhatTheForgeSays(t *testing.T) {
	const body = `{"number":7,"state":"open","headRefName":"feat/x","url":"https://example.invalid/pr/7","title":"feat: x","body":"why"}`
	t.Setenv("PATH", fakeBin(t, GHBin, body, 0))

	pr, err := LookPR(t.TempDir(), 7)
	if err != nil {
		t.Fatalf("LookPR: %v", err)
	}
	if pr.Number != 7 || pr.Branch != "feat/x" || pr.Title != "feat: x" || pr.Body != "why" {
		t.Errorf("pr = %+v", pr)
	}
	// gh spells the state however it likes; scc's constants are upper case, so the
	// comparison a caller writes against StateOpen has to hold.
	if pr.State != StateOpen {
		t.Errorf("state = %q, want %q", pr.State, StateOpen)
	}
	if _, err := CurrentPR(t.TempDir()); err != nil {
		t.Errorf("CurrentPR: %v", err)
	}

	// A branch with no pull request is gh exiting non-zero, and that is an answer
	// rather than a malfunction.
	t.Setenv("PATH", fakeBin(t, GHBin, "", 1))
	if _, err := CurrentPR(t.TempDir()); err == nil {
		t.Error("CurrentPR returned no error when gh reported none")
	}
	// And output that is not JSON is reported rather than half-read.
	t.Setenv("PATH", fakeBin(t, GHBin, "not json", 0))
	if _, err := LookPR(t.TempDir(), 7); err == nil {
		t.Error("LookPR accepted output that is not JSON")
	}
}

// A comment is the half of the pull request that nothing else can reach: no hook
// runs when a tool posts one. The query reads the issue comments and the review
// bodies as one list, and drops a review that is an approval click with nothing
// written in it.
func TestPRCommentsReadsWhatWasWrittenAfterwards(t *testing.T) {
	const body = `{"comments":[{"author":{"login":"someone"},"url":"https://example.invalid/pr/7#c1","body":"looks good"}],` +
		`"reviews":[{"author":{"login":"other"},"url":"https://example.invalid/pr/7#r1","body":"one nit"},` +
		`{"author":{"login":"third"},"url":"https://example.invalid/pr/7#r2","body":"   "}]}`
	t.Setenv("PATH", fakeBin(t, GHBin, body, 0))

	got, err := PRComments(t.TempDir(), 7)
	if err != nil {
		t.Fatalf("PRComments: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d comments, want 2 (the empty review is an approval click): %+v", len(got), got)
	}
	// Issue comments first, then reviews, so a reader meets them in the order the
	// pull request page shows them.
	if got[0].Author != "someone" || got[0].Body != "looks good" {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].Author != "other" || got[1].URL != "https://example.invalid/pr/7#r1" {
		t.Errorf("second = %+v", got[1])
	}

	// The current branch's pull request is the same query with no number, which is
	// gh's own default.
	if _, err := PRComments(t.TempDir(), 0); err != nil {
		t.Errorf("PRComments(0): %v", err)
	}

	// And every way of having no answer is an error the caller turns into silence,
	// never a half-read list.
	t.Setenv("PATH", fakeBin(t, GHBin, "not json", 0))
	if _, err := PRComments(t.TempDir(), 7); err == nil {
		t.Error("PRComments accepted output that is not JSON")
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := PRComments(t.TempDir(), 7); err != ErrUnavailable {
		t.Errorf("err = %v, want ErrUnavailable when gh is not on PATH", err)
	}
}
