package validate

import (
	"strconv"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/attribution"
	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/git"
)

// Attribution reports an assistant's signature in the commits this branch has
// added — the `Co-Authored-By` trailer, the "generated with" footer, the badge,
// the link, the bare name on a line of its own.
//
// It is the one validator whose subject is not a file, and it is here rather than
// in a rule for the reason every gate in this product exists: `rules/delivery.md`
// has said "no attribution in the record" since the first release, and every
// harness's defaults append one anyway unless the session overrides them. An
// instruction that has to be re-read and re-obeyed on every commit holds until the
// one session that does not, and under `autonomy: auto` nobody is watching that
// session. A check that runs the same way every time is what turns the rule into
// something the workspace enforces instead of something it asks for.
//
// **It never returns an error, and that is a contract rather than an oversight.**
// `Everything` turns a validator's error into exit 1 — "could not run" — and a
// project with no git, an unborn HEAD, a shallow CI clone, or a checkout that is
// exactly its base branch is not a project that failed a check. It is a project
// with nothing on this branch to check, which is what a clean pass means. The
// alternative would break `scc validate` in every workspace that is not a git
// repository, over a rule about git.
//
// What it deliberately does not check is the branch name, which the same rule
// covers. `feat/claude-api-client` is an ordinary branch for anyone building on
// that API, and there is no shape to gate the vocabulary behind the way a trailer
// or a footer gates it — so the check would rest on the vendor's name alone, which
// is the one thing internal/attribution refuses to do.
func Attribution(root string) (*finding.Set, error) {
	set := &finding.Set{}
	if !git.IsRepo(root) {
		return set, nil
	}
	commits, err := git.Commits(root, "")
	if err != nil {
		return set, nil
	}
	for _, c := range commits {
		report(set, c.Short, clipSubject(c.Subject), c.Message)
	}
	return set, nil
}

// AttributionPR reports the same signatures in the pull request this branch has
// open — its title and its body.
//
// It is the other half of the record and the half no hook can reach. A commit
// message passes through `commit-msg` on the machine that wrote it; a PR body is
// typed straight into the forge, which is why the footer that survives everywhere
// else survives there. `scc validate --pr` is where it gets checked, and the
// pre-push hook is where that runs on its own.
//
// It is **not** part of the default run, and the reason is cost rather than
// doctrine: this is a network call to `gh`, and `scc validate` is on the
// pre-commit path where a second of latency is paid on every commit. A check
// nobody can afford to leave on is a check that gets turned off.
//
// Like its sibling it never returns an error. No gh, no authentication, no
// network, no pull request for this branch: all of them mean there is nothing to
// check, and none of them is this validator failing.
func AttributionPR(root string) (*finding.Set, error) {
	set := &finding.Set{}
	if !git.IsRepo(root) || !git.Found(git.GHBin) {
		return set, nil
	}
	pr, err := git.CurrentPR(root)
	if err != nil || pr.Number == 0 {
		return set, nil
	}
	// Title first and body after, as one text, so a finding's line number reads
	// against what a person sees on the pull request page: line 1 is the title.
	report(set, prName(pr), clipSubject(pr.Title), pr.Title+"\n"+pr.Body)
	return set, nil
}

// prName is what a pull-request finding is filed under: the number, because it is
// what `gh pr edit <n>` takes and the report is read in a terminal.
func prName(pr git.PR) string { return "PR #" + strconv.Itoa(pr.Number) }

// report turns one piece of the record — a commit message, a pull request — into
// findings. One function so the two callers cannot describe the same signature in
// two different ways.
func report(set *finding.Set, where, subject, text string) {
	for _, hit := range attribution.Scan(text) {
		set.Addf(where, hit.Line, "attribution."+hit.Rule,
			"%s in %q — %s", clip(hit.Match), subject, advice[hit.Rule])
	}
}

// advice is what to do about each shape, which is the difference between a
// finding and a complaint. They all end in the same place — the text has to be
// rewritten — so each says only what is wrong with the line it is on.
var advice = map[string]string{
	attribution.RuleTrailer: "an assistant is not a co-author; git reads this trailer as authorship",
	attribution.RuleFooter:  "the record says what changed, not what typed it",
	attribution.RuleLink:    "a session link is not part of the record",
	attribution.RuleBadge:   "the badge is the footer with the words taken off",
	attribution.RuleMention: "a line that says nothing but the name is a signature",
}

// clip keeps a finding to one readable line. A trailer is short, but a footer can
// carry a full URL and a message that wrapped the terminal would defeat the
// grouping the report is built around.
func clip(s string) string {
	const max = 64
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// clipSubject names the commit in words, next to the short sha the finding is
// filed under. The sha is what `git show` takes; the subject is what the reader
// recognizes, and neither on its own is enough to find the commit in a rebase.
func clipSubject(s string) string {
	const max = 48
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
