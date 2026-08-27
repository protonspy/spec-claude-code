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
		if _, err := run(Bin, dir, "rev-parse", "--verify", "--quiet", candidate); err == nil {
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
type PR struct {
	Number int    `json:"number"`
	State  string `json:"state"` // OPEN | MERGED | CLOSED, as gh spells them
	Branch string `json:"branch"`
	URL    string `json:"url"`
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
	out, err := run(GHBin, dir, "pr", "view", strconv.Itoa(number),
		"--json", "number,state,headRefName,url")
	if err != nil {
		return PR{Number: number}, err
	}
	var raw struct {
		Number      int    `json:"number"`
		State       string `json:"state"`
		HeadRefName string `json:"headRefName"`
		URL         string `json:"url"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return PR{Number: number}, err
	}
	return PR{Number: raw.Number, State: strings.ToUpper(raw.State), Branch: raw.HeadRefName, URL: raw.URL}, nil
}
