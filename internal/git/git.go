// Package git answers one question for scc: what happened to the branch this spec
// says it is being built on.
//
// It is the fourth integration package, on the same terms as rtk, headroom and
// codegraph — it composes command lines for a binary somebody else ships and reads
// what comes back. Two binaries rather than one, because the question has two halves
// that no caller should have to route between: `git` knows whether a branch exists
// and whether it has landed, and `gh` knows whether a pull request is open, merged,
// or closed unmerged. A caller asking "is this work finished?" would otherwise have
// to know which of the two could answer today.
//
// **Nothing here installs anything, and nothing here writes.** scc will not install
// git, and it runs no command that changes a repository: every call below is a query.
// That is what makes it safe to run this over every spec in a workspace on somebody's
// behalf — the worst outcome of a wrong answer is a frontmatter line that says the
// wrong thing, and `scc spec sync` can be run again.
//
// Absence is a normal answer, never an error to propagate. A workspace with no git,
// no remote, or no `gh` still has specs, and the caller reports what it could not
// determine rather than failing.
package git

import (
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Bin and GHBin are the executables, named here so a caller never spells them.
const (
	Bin   = "git"
	GHBin = "gh"
)

// DefaultBase is the branch a repository is assumed to merge into when nothing says
// otherwise. It is a fallback for a query that failed, not a preference: every path
// below asks the repository first.
const DefaultBase = "main"

// ErrUnavailable is what every query returns when the binary it needs is not on PATH.
// Callers test for it to say "could not determine" instead of "failed".
var ErrUnavailable = errors.New("not available")

// Found reports whether a binary is on PATH.
func Found(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

// run executes a query in dir and returns its trimmed stdout.
//
// Stderr is deliberately dropped. Every caller here treats failure as "could not
// determine", and git writes advice to stderr on perfectly ordinary misses — a
// caller that surfaced it would turn "this branch is gone, as expected after a
// merge" into something that reads like a malfunction.
func run(bin, dir string, args ...string) (string, error) {
	if !Found(bin) {
		return "", ErrUnavailable
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// IsRepo reports whether dir is inside a git work tree.
func IsRepo(dir string) bool {
	out, err := run(Bin, dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// CurrentBranch is the checked-out branch, or "" in a detached head.
func CurrentBranch(dir string) (string, error) {
	out, err := run(Bin, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	if out == "HEAD" {
		return "", nil
	}
	return out, nil
}

// Base is the branch this repository merges into.
//
// It is read from the remote's own HEAD rather than guessed, because "main" has been
// wrong for every repository created before 2020 and for plenty created since. A
// repository with no remote falls back to whichever of main and master exists, and
// then to DefaultBase — which is a guess, and is why callers say what base they used.
func Base(dir string) string {
	if out, err := run(Bin, dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if _, name, found := strings.Cut(out, "/"); found && name != "" {
			return name
		}
	}
	for _, name := range []string{DefaultBase, "master"} {
		if ref(dir, name) != "" {
			return name
		}
	}
	return DefaultBase
}

// ref resolves the first ref that exists for a branch name: the local branch, then
// the remote-tracking one. The remote half is what keeps a branch findable after a
// local checkout has been deleted, which is most of them.
func ref(dir, branch string) string {
	for _, candidate := range []string{"refs/heads/" + branch, "refs/remotes/origin/" + branch} {
		if verify(dir, candidate) {
			return candidate
		}
	}
	return ""
}

// Branch is what git knows about one branch.
type Branch struct {
	Name string `json:"name"`
	// Local and Remote say where it still exists. Both false means the branch is
	// gone, which on its own means nothing: it is equally the shape of a branch
	// deleted after a clean merge and of one abandoned.
	Local  bool `json:"local"`
	Remote bool `json:"remote"`
	// Ahead is the commits on this branch that are not on Base; Behind is the
	// reverse. Both are 0 when the two refs are the same commit.
	Ahead  int `json:"ahead"`
	Behind int `json:"behind"`
	// Merged is Ahead == 0 with Behind > 0: everything this branch had is on the
	// base, and the base has moved on since. That second half is what stops a
	// freshly created branch — which is trivially an ancestor of its base, having
	// added nothing — from being read as delivered work.
	//
	// The one case it gets wrong is a fast-forward merge that nothing has advanced
	// past, where the two refs are identical and no ref can tell "just branched"
	// from "just landed". It resolves that as *not* merged, deliberately: this
	// record exists to surface unfinished work, so the error that leaves a loose end
	// visible is the one to make.
	Merged bool   `json:"merged"`
	Base   string `json:"base"`
}

// Exists reports whether git can still see the branch at all.
func (b Branch) Exists() bool { return b.Local || b.Remote }

// Look reports what git knows about a branch.
func Look(dir, branch, base string) (Branch, error) {
	if !Found(Bin) {
		return Branch{Name: branch}, ErrUnavailable
	}
	if base == "" {
		base = Base(dir)
	}
	b := Branch{Name: branch, Base: base}
	_, localErr := run(Bin, dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	b.Local = localErr == nil
	_, remoteErr := run(Bin, dir, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+branch)
	b.Remote = remoteErr == nil
	if !b.Exists() {
		return b, nil
	}
	// Counted against the base ref that exists: a workspace with no remote has no
	// origin/main to compare against, and a count nobody can take is not a zero.
	baseRef := ref(dir, base)
	if baseRef == "" {
		return b, nil
	}
	b.Behind, b.Ahead = counts(dir, baseRef, ref(dir, branch))
	b.Merged = b.Ahead == 0 && b.Behind > 0
	return b, nil
}

// counts is how far two refs have diverged: commits on left only, then on right only.
//
// `--left-right --count a...b` is one call for both halves, and it is the honest test
// for "did this land" — an ancestor check alone answers yes for a branch that has
// never had a commit of its own.
func counts(dir, left, right string) (int, int) {
	out, err := run(Bin, dir, "rev-list", "--left-right", "--count", left+"..."+right)
	if err != nil {
		return 0, 0
	}
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return 0, 0
	}
	l, err1 := strconv.Atoi(fields[0])
	r, err2 := strconv.Atoi(fields[1])
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	return l, r
}

// PR is what the forge knows about a pull request.
//
// Title and Body are here because the pull request is half of the record of a
// change and the half no hook can reach: a commit message passes through
// commit-msg on the machine that wrote it, and a PR body is typed straight into
// the forge. Whatever rule binds a commit message binds this text too, so a caller
// checking one has to be able to ask for the other.
type PR struct {
	Number int    `json:"number"`
	State  string `json:"state"` // OPEN | MERGED | CLOSED, as gh spells them
	Branch string `json:"branch"`
	URL    string `json:"url"`
	Title  string `json:"title,omitempty"`
	Body   string `json:"body,omitempty"`
}

// The states gh reports, named so a caller never matches on a string literal.
const (
	StateOpen   = "OPEN"
	StateMerged = "MERGED"
	StateClosed = "CLOSED"
)

// LookPR asks gh about one pull request.
//
// This is the only query that can tell a merged branch from an abandoned one after
// the branch itself is gone, which is the common case and the reason gh is worth
// shelling out to at all. Without it, a spec whose branch has vanished is reported as
// undetermined rather than guessed at.
func LookPR(dir string, number int) (PR, error) {
	pr, err := viewPR(dir, strconv.Itoa(number))
	if err != nil {
		return PR{Number: number}, err
	}
	return pr, nil
}

// CurrentPR asks gh about the pull request for the checked-out branch, if there
// is one.
//
// A separate entry point rather than a zero value for LookPR's number, because
// the two answer different questions: one is "what happened to the PR this spec
// recorded", the other is "what is open in front of me right now". A branch with
// no pull request is an error from gh and a normal answer here — the caller has
// nothing to check, which is not the same as a failure.
func CurrentPR(dir string) (PR, error) { return viewPR(dir, "") }

// viewPR is the one `gh pr view` both entry points share. An empty arg means the
// current branch, which is gh's own default.
func viewPR(dir, arg string) (PR, error) {
	args := []string{"pr", "view"}
	if arg != "" {
		args = append(args, arg)
	}
	out, err := run(GHBin, dir, append(args, "--json", "number,state,headRefName,url,title,body")...)
	if err != nil {
		return PR{}, err
	}
	var raw struct {
		Number      int    `json:"number"`
		State       string `json:"state"`
		HeadRefName string `json:"headRefName"`
		URL         string `json:"url"`
		Title       string `json:"title"`
		Body        string `json:"body"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return PR{}, err
	}
	return PR{
		Number: raw.Number,
		State:  strings.ToUpper(raw.State),
		Branch: raw.HeadRefName,
		URL:    raw.URL,
		Title:  raw.Title,
		Body:   raw.Body,
	}, nil
}

// Commit is one commit's identity and its message in full.
//
// Message is the whole thing — subject, body and trailers — because the trailers
// are the point: a caller asking what this history says about who did the work is
// asking about the lines git itself reads as authorship, and those sit at the end.
type Commit struct {
	SHA     string `json:"sha"`
	Short   string `json:"short"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

// The record and field separators used to read `git log`. ASCII 0x1e and 0x1f
// exist for exactly this and cannot appear in a commit message, which newlines,
// tabs and every printable delimiter can — a message quoting a delimiter would
// otherwise split into two commits, and the reader would blame the wrong sha.
const (
	recordSep = "\x1e"
	fieldSep  = "\x1f"
)

// Commits lists the commits that are on HEAD and not on base, newest first.
//
// "Not on base" is the whole definition of the work in front of the user: on a
// feature branch it is the branch, and on the base branch itself it is whatever
// has not been pushed yet. Anything already on the base is somebody else's
// history, and a caller that reported on it would report the same thing on every
// branch in the repository forever.
//
// Absence is a normal answer here as it is everywhere else in this package: no
// git, no repository, an unborn HEAD, a shallow clone with no base ref, or a
// branch that is exactly its base all return no commits and no error. A caller
// checking a property of this branch's history has nothing to check, which is a
// different thing from a failure and must not read as one.
func Commits(dir, base string) ([]Commit, error) {
	if !Found(Bin) {
		return nil, ErrUnavailable
	}
	if base == "" {
		base = Base(dir)
	}
	against := compareRef(dir, base)
	if against == "" {
		return nil, nil
	}
	format := strings.Join([]string{"%H", "%h", "%s", "%B"}, fieldSep) + recordSep
	out, err := run(Bin, dir, "log", "--no-merges", "--format="+format, against+"..HEAD")
	if err != nil {
		// An unborn HEAD, a range git cannot resolve, a repository mid-rebase.
		// Nothing to report, and nothing worth failing a validation run over.
		return nil, nil
	}
	return parseLog(out), nil
}

// compareRef is what "not on base" is measured against, or "" when no ref can
// answer.
//
// Standing on the base branch is the case worth spelling out: refs/heads/main
// compared against itself is empty, so a check run there would silently pass on
// every commit in the repository. The remote's copy is the honest comparison —
// what is here and not yet pushed — and a base branch with no remote at all
// leaves genuinely nothing to compare against.
func compareRef(dir, base string) string {
	branch, err := CurrentBranch(dir)
	if err == nil && branch == base {
		remote := "refs/remotes/origin/" + base
		if verify(dir, remote) {
			return remote
		}
		return ""
	}
	return ref(dir, base)
}

// verify reports whether a ref resolves.
func verify(dir, r string) bool {
	_, err := run(Bin, dir, "rev-parse", "--verify", "--quiet", r)
	return err == nil
}

// parseLog turns the separated log stream back into commits. A trailing record
// separator leaves an empty final field, which is dropped rather than reported as
// a commit with no sha.
func parseLog(out string) []Commit {
	var commits []Commit
	for _, record := range strings.Split(out, recordSep) {
		record = strings.Trim(record, "\n")
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, fieldSep, 4)
		if len(fields) != 4 {
			continue
		}
		commits = append(commits, Commit{
			SHA:     fields[0],
			Short:   fields[1],
			Subject: fields[2],
			Message: strings.TrimRight(fields[3], "\n"),
		})
	}
	return commits
}

// HooksDir is the directory this repository runs its hooks from.
//
// Asked of git rather than assumed, because `.git/hooks` is only the default: a
// repository with core.hooksPath set runs from somewhere else entirely, and a
// worktree or a submodule keeps its git directory outside the checkout. Writing a
// hook into a path that git does not read is the kind of failure that looks like
// success for weeks.
//
// The configured path wins, resolved against the work tree the way git resolves
// it. Absence is a normal answer here as everywhere else in this package: a
// directory that is not a repository has no hooks directory, which is not an
// error to propagate.
func HooksDir(dir string) (string, error) {
	if p, err := run(Bin, dir, "config", "--get", "core.hooksPath"); err == nil && p != "" {
		return absolute(dir, p), nil
	}
	out, err := run(Bin, dir, "rev-parse", "--git-path", "hooks")
	if err != nil {
		return "", err
	}
	if out == "" {
		return "", ErrUnavailable
	}
	return absolute(dir, out), nil
}

// absolute resolves a path git reported relative to the directory it was asked
// in. git answers with forward slashes on every platform, which filepath handles.
func absolute(dir, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(dir, p)
}

// Undelivered is what git alone can say about work that is finished and still on
// this machine.
//
// Alone is the operative word. Everything here is answered from refs that are
// already in the repository — no fetch, no `gh`, no network — because the caller
// is a hook that runs at the end of every turn, and a check that cost a network
// round trip per turn is a check somebody turns off. That bounds what it can
// know: `origin/<branch>` is as current as the last fetch, so this reports what
// this checkout has not sent, which is the question, rather than what the forge
// has, which is not.
type Undelivered struct {
	Branch string `json:"branch"`
	Base   string `json:"base"`
	// Ahead is the commits this branch has added to its base.
	Ahead int `json:"ahead"`
	// Unpushed is how many of those `origin/<branch>` does not have. Equal to
	// Ahead when the branch has never been pushed at all.
	Unpushed int `json:"unpushed"`
	// Pushed says a remote-tracking ref exists, which is what separates "nothing
	// to push" from "never left this machine".
	Pushed bool `json:"pushed"`
}

// Any reports whether there is work here worth saying anything about.
func (u Undelivered) Any() bool { return u.Ahead > 0 && u.Unpushed > 0 }

// Undelivered answers "is there finished work that has not left this machine".
//
// Absence is a normal answer, never an error to propagate — the same line this
// package holds everywhere else. No git, no repository, a detached HEAD, or a
// checkout sitting on its own base branch all come back as an empty answer,
// because each of those is a real state and none of them is a failure of the
// caller that asked.
func UndeliveredWork(dir string) Undelivered {
	if !Found(Bin) || !IsRepo(dir) {
		return Undelivered{}
	}
	branch, err := CurrentBranch(dir)
	if err != nil || branch == "" {
		return Undelivered{}
	}
	base := Base(dir)
	// Standing on the base branch is not a branch of work. Reporting it would put
	// the same line at the end of every turn in a repository whose author does not
	// use branches, which is how a nudge becomes wallpaper.
	if branch == base {
		return Undelivered{Branch: branch, Base: base}
	}
	u := Undelivered{Branch: branch, Base: base}
	if baseRef := ref(dir, base); baseRef != "" {
		_, u.Ahead = counts(dir, baseRef, ref(dir, branch))
	}
	remote := "refs/remotes/origin/" + branch
	if _, err := run(Bin, dir, "rev-parse", "--verify", "--quiet", remote); err != nil {
		// Never pushed: everything this branch added is still here. A branch with no
		// upstream and no commits of its own is still nothing to report, which Any
		// takes care of.
		u.Unpushed = u.Ahead
		return u
	}
	u.Pushed = true
	_, u.Unpushed = counts(dir, remote, ref(dir, branch))
	return u
}
