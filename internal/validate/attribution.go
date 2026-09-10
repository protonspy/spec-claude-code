package validate

import (
	"strings"

	"github.com/protonspy/spec-claude-code/internal/attribution"
	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/git"
)

// Attribution reports an assistant's signature in the commits this branch has
// added — the `Co-Authored-By` trailer, the "generated with" footer, the session
// link.
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
		for _, hit := range attribution.Scan(c.Message) {
			set.Addf(c.Short, hit.Line, "attribution."+hit.Rule,
				"%s in %q — %s", clip(hit.Match), clipSubject(c.Subject), advice[hit.Rule])
		}
	}
	return set, nil
}

// advice is what to do about each shape, which is the difference between a
// finding and a complaint. All three end in the same place — the message has to
// be rewritten — so each says only what is wrong with the line it is on.
var advice = map[string]string{
	attribution.RuleTrailer: "an assistant is not a co-author; git reads this trailer as authorship",
	attribution.RuleFooter:  "the record says what changed, not what typed it",
	attribution.RuleLink:    "a session link is not part of the record",
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
