package cli

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/git"
	"github.com/protonspy/spec-claude-code/internal/mdscan"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// Methodology drift, reported at the end of a turn.
//
// This is the one place scc improves on the thing it was ported from. ponytail
// assumes an agent drifts away from its instructions and re-asserts them on every
// prompt and every subagent; that works because its instruction is a disposition —
// reach for the smallest thing that works — and a disposition has no shape, so
// re-asserting is the only move available. scc's rules are not dispositions. They
// say a task is a box in a plan, that a marker does not go in code, that work does
// not happen on the base branch — and every one of those leaves evidence on disk,
// in a range internal/git already computes. So the answer is not to re-assert the
// rule at somebody who has already read it. It is to say what actually happened.
//
// Three signals, and the discipline that matters most is the one that keeps them
// worth reading:
//
// **Each is a line only when it is true.** A stage that speaks every turn is a
// stage the reader learns to skip, and then the turn it had something to say is
// the turn nobody read it. A clean branch draws nothing at all —
// TestTheDriftStageIsSilentOnACleanBranch is what holds that, because silence is
// the property least likely to survive a later edit and least likely to be
// noticed when it stops being true.
//
// **The baseline is the branch, never a session snapshot.** Nothing here
// remembers anything between turns: a snapshot would need a file to live in, and
// scc has one file per harness on purpose. The branch is also the better unit —
// `delivery.md` already makes one branch one piece of work, so `base..HEAD` plus
// the working tree is exactly the thing a reader is asking about.
//
// **It reports and never refuses**, like everything else on this stage. Exit 2
// would block the turn, and a hook that can block a turn can loop one: the agent
// fixes the finding, the hook fires again on the turn that fixed it.

// driftMarkers is what `notes.md` forbids in code and no validator reads, because
// none of them reads source.
//
// Four of the rule's five. The fifth is the one that is also an ordinary English
// word in an ordinary sentence, and a scan that matched it would report a
// perfectly legal comment on every turn — which costs more than the one real
// instance it would catch. The rule is still the rule; this is the part of it that
// can be checked without crying wolf.
var driftMarkers = []string{"TODO", "FIXME", "HACK", "XXX"}

// The comment openers, split by where they may legitimately appear — which is
// the whole of what makes this scan safe to run over source.
//
// A marker matters when it was left as an annotation, and an annotation lives in
// a comment. Requiring an opener earlier on the line separates the thing the rule
// forbids from a string literal that merely contains the word: a linter's
// configuration, a test fixture, or driftMarkers above, which would otherwise
// make this file report itself on the branch that introduced it.
//
// The split is the second half, and it was not in the first cut. `//` and `#`
// open a trailing comment, so they count anywhere on a line — a marker after one
// of those, mid-line, is exactly what the rule is about. The rest do not: `*` is
// a pointer and a multiplication far more often than a block-comment
// continuation, `--` is a command-line flag, and `;` ends a statement in most of
// the languages that have one. Taken anywhere, those three report a test that
// passes a marker word to a command as an argument — a false positive in test
// code, which is the fastest way to teach a reader to ignore the whole line.
var (
	inlineOpeners = []string{"//", "#", "/*", "<!--"}
	lineOpeners   = []string{"*", "--", ";", "%"}
)

// driftLines is what the end of a turn has to say about the methodology, or
// nothing.
func driftLines(root string) []string {
	base := git.Base(root)
	changes, err := git.Changed(root, base)
	if err != nil {
		// No git, no repository, a shallow clone with no base ref. Absence is a
		// normal answer here, and a line about plumbing at the end of a turn is
		// context the agent pays for and cannot act on.
		return nil
	}

	var out []string
	if line := onBaseLine(root, base); line != "" {
		out = append(out, line)
	}
	if line := untickedLine(root, changes); line != "" {
		out = append(out, line)
	}
	if line := markerLine(changes); line != "" {
		out = append(out, line)
	}
	return out
}

// onBaseLine reports work sitting on the branch it is supposed to be delivered
// into.
//
// It is the cheapest of the three and the one with the shortest window: the fix
// is `git switch -c`, and it is a different act once the work has been pushed.
// Said while the branch is still local, or not at all — a repository whose author
// genuinely works on the base branch would otherwise get this line every turn
// forever, which is how a signal becomes wallpaper.
func onBaseLine(root, base string) string {
	u := git.UndeliveredWork(root)
	if u.Branch == "" || u.Branch != base || u.Ahead > 0 {
		// Ahead > 0 cannot happen on the base branch — UndeliveredWork returns
		// early there — and is tested anyway, because this line is the one that
		// would fire on every turn if that ever changed.
		return ""
	}
	commits, err := git.Commits(root, base)
	if err != nil || len(commits) == 0 {
		return ""
	}
	return fmt.Sprintf("scc: %d commit%s on `%s`, which delivery.md says work does not happen on — "+
		"`git switch -c <type>/<slug>` moves them onto a branch before they are pushed.",
		len(commits), plural(len(commits)), base)
}

// untickedLine reports source changed on this branch with no box ticked anywhere.
//
// The two halves are what make it worth saying. Source alone is ordinary work;
// a ticked box alone is a plan being kept. Source with nothing ticked is the
// shape of code written outside the methodology entirely — no task cited, nothing
// to review the change against, and nothing that will show up as progress when
// somebody asks what this branch did.
//
// It is deliberately not a finding. Plenty of legitimate branches look like this
// for a while, which is exactly why no validator owns it and why this says it at
// the end of a turn instead of refusing a commit.
//
// **It is silent where there is nowhere to tick a box**, and that bound was found
// by running the stage against scc's own repository rather than against a
// fixture. This repo is not an scc workspace — no `plans/`, no `specs/` — so the
// condition "source changed and nothing was ticked" is true of literally every
// turn, forever, and the line became wallpaper on its first real outing. A
// repository with no artifacts is not drifting from a methodology it never
// adopted; it is just a repository.
func untickedLine(root string, changes []git.Change) string {
	if !hasArtifacts(root) {
		return ""
	}
	var source []string
	ticked := false
	for _, c := range changes {
		switch {
		case artifactPath(c.Path):
			for _, line := range c.Added {
				if mdscan.Ticked(line) {
					ticked = true
				}
			}
		case sourcePath(c.Path):
			source = append(source, c.Path)
		}
	}
	if ticked || len(source) == 0 {
		return ""
	}
	return fmt.Sprintf("scc: this branch changed %d source file%s and ticked no box — "+
		"`%s map tasks <plan>` finds the task this belongs to, or it is work no plan asked for. (%s)",
		len(source), plural(len(source)), prog(), sample(source))
}

// markerLine reports the markers this branch added to code.
//
// The one signal here with no validator behind it and no way to get one: every
// other validator reads an artifact, and this is about source, which scc does not
// parse and has no business parsing. The rule exists because a marker in code
// reaches exactly one reader — the one already looking at that line — and dies
// with the file. `scc notes add` is where it goes instead, which is why the line
// names that command rather than only the problem.
func markerLine(changes []git.Change) string {
	seen := map[string]bool{}
	var where []string
	for _, c := range changes {
		if !sourcePath(c.Path) {
			continue
		}
		for _, line := range c.Added {
			m := markerIn(line)
			if m == "" || seen[c.Path+" "+m] {
				continue
			}
			seen[c.Path+" "+m] = true
			where = append(where, c.Path+" ("+m+")")
		}
	}
	if len(where) == 0 {
		return ""
	}
	return fmt.Sprintf("scc: this branch added %d marker%s to code, which notes.md forbids — "+
		"`%s notes add \"…\" --tag <tag> --path <path>` is where that goes. (%s)",
		len(where), plural(len(where)), prog(), sample(where))
}

// markerIn is the marker a line left in a comment, or "".
//
// The opener has to come first on the line. That is what keeps a word inside a
// string literal from counting, and it is the difference between a check the
// agent acts on and one it learns to ignore.
func markerIn(line string) string {
	at := openerAt(line)
	if at < 0 {
		return ""
	}
	rest := line[at:]
	for _, m := range driftMarkers {
		i := strings.Index(rest, m)
		if i < 0 {
			continue
		}
		// A whole word, so `TODOS.md` in a path and `xxx` in a hex dump are not
		// annotations somebody left behind.
		if boundary(rest, i-1) && boundary(rest, i+len(m)) {
			return m
		}
	}
	return ""
}

// openerAt is where a comment starts on this line, or -1.
func openerAt(line string) int {
	best := -1
	for _, o := range inlineOpeners {
		if i := strings.Index(line, o); i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	trimmed := strings.TrimLeft(line, " \t")
	for _, o := range lineOpeners {
		if !strings.HasPrefix(trimmed, o) {
			continue
		}
		if at := len(line) - len(trimmed); best < 0 || at < best {
			best = at
		}
	}
	return best
}

// boundary reports whether the character at i ends a word, treating both ends of
// the string as boundaries.
func boundary(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return true
	}
	c := s[i]
	return !(c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z')
}

// artifactPath reports whether a path is one the methodology tracks progress in:
// a plan, or a spec's tasks. Those are where a box can be ticked.
func artifactPath(p string) bool {
	return underSeg(p, paths.PlansSeg) || underSeg(p, paths.SpecsSeg)
}

// hasArtifacts reports whether this workspace has anywhere to tick a box.
//
// A directory rather than a parsed plan, deliberately: the question is whether
// this project keeps its work in artifacts at all, and a workspace whose only
// plan is malformed is still one where the answer is yes.
func hasArtifacts(root string) bool {
	for _, seg := range []string{paths.PlansSeg, paths.SpecsSeg} {
		if info, err := os.Stat(filepath.Join(root, seg)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// sourcePath reports whether a path is code this project is responsible for.
//
// Defined by exclusion, and it has to be: scc governs four trees and knows
// nothing about the fifth, which is the project itself. So everything that is not
// an artifact, not the knowledge base, not a harness's own directory and not the
// container definition is source — which is the right default, because a new
// language arriving in a repository must not silently stop being checked.
//
// Markdown anywhere is excluded on top of that. A marker in prose is prose, and
// the rule that forbids one is about code.
func sourcePath(p string) bool {
	if strings.HasSuffix(strings.ToLower(p), ".md") {
		return false
	}
	if underSeg(p, paths.SpecsSeg) || underSeg(p, paths.PlansSeg) ||
		underSeg(p, paths.DocsSeg) || underSeg(p, paths.DevcontainerSeg) {
		return false
	}
	for _, h := range paths.Harnesses() {
		if underSeg(p, h.Dir) {
			return false
		}
	}
	return p != ""
}

// underSeg reports whether a slash-separated repo-relative path is inside seg.
func underSeg(p, seg string) bool {
	return p == seg || strings.HasPrefix(p, seg+"/")
}

// sample names a few of what was found and counts the rest.
//
// Capped because this is context the agent pays for on its next turn, and the
// whole list is a command away. The same reasoning caps the finding lines beside
// it, for the same reason.
func sample(all []string) string {
	const most = 3
	if len(all) <= most {
		return strings.Join(shortened(all), ", ")
	}
	return fmt.Sprintf("%s, +%d more", strings.Join(shortened(all[:most]), ", "), len(all)-most)
}

// shortened prints a path the way a reader recognizes it: the last two segments,
// which is enough to place a file without spending a line on a deep tree.
func shortened(in []string) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		dir, file := path.Split(p)
		if dir == "" {
			out = append(out, p)
			continue
		}
		out = append(out, path.Base(strings.TrimSuffix(dir, "/"))+"/"+file)
	}
	return out
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
