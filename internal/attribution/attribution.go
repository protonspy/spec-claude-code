// Package attribution knows the shapes an assistant's signature takes in the
// record of a change — a commit message, a pull request title or body.
//
// It exists because "the work is the user's, and the record says so" is a rule
// every harness's defaults work against: Claude Code, Codex and opencode all
// append a co-author trailer or a generated-with footer unless told not to, and a
// rule that is only ever read is a rule that holds until the one session that did
// not read it. A deterministic check is what turns the instruction into a gate.
//
// It knows nothing about findings, exit codes or git — the same seam
// internal/notes and internal/artifact keep with their own grammars. Scan takes
// text and returns what it matched; deciding that a match is a finding belongs to
// the validator, and deciding where the text came from belongs to the caller.
//
// **A signature is a shape, never a word.** Nothing here fires on a vendor's name
// alone: `chore(deps): bump the anthropic SDK to 0.40` and `feat(harness): add
// opencode support` are ordinary commits, and a check that flagged them would be
// suppressed within a day — which costs more than every signature it would ever
// catch. Every rule below is a trailer, a footer phrase, or a link, and the
// vocabulary only decides whether that shape names an assistant.
package attribution

import (
	"regexp"
	"strings"
)

// Hit is one signature found in the text, at the line it sits on.
//
// Rule is the stable half of the identity and carries no prefix: the validator
// namespaces it, so this package never has to know what a rule slug looks like
// elsewhere in scc.
type Hit struct {
	Line  int    `json:"line"`
	Rule  string `json:"rule"`
	Match string `json:"match"`
}

// The rules, named so a caller never matches on a string literal.
const (
	// RuleTrailer is a `Co-Authored-By:`-shaped trailer whose value names an
	// assistant. The trailer is what git itself reads as authorship, so this is
	// the one that actually rewrites who the history says did the work.
	RuleTrailer = "co-authored-by"
	// RuleFooter is a "generated with"/"created by" line naming a model, vendor
	// or harness — the footer the harnesses append by default.
	RuleFooter = "generated-with"
	// RuleLink is a session link or a vendor no-reply address. It is separate
	// from the other two because it survives them: a footer rewritten by hand
	// often keeps the URL.
	RuleLink = "assistant-link"
)

// The three shapes. Compiled once, applied per line, and deliberately anchored:
// a pattern that could match mid-sentence would fire on a commit message that
// merely quotes one of these lines, which is exactly what a fix for this finding
// looks like.
var (
	// A git trailer: a known key at the start of a line, with a value.
	trailer = regexp.MustCompile(`(?i)^[ \t]*(co-?authored-by|assisted-by|generated-by|signed-off-by)[ \t]*:[ \t]*(\S.*?)[ \t]*$`)
	// The footer phrase. Both halves have to be present on the line — the phrase
	// and a name — or it is prose.
	footer = regexp.MustCompile(`(?i)\b(?:generated|created|authored|written|made|built)\s+(?:with|by|using)\b`)
	// A session link or a vendor no-reply address, which is a signature on its
	// own: nobody cites either of these to explain a change.
	link = regexp.MustCompile(`(?i)\b(?:https?://)?(?:claude\.ai|claude\.com/claude-code|chatgpt\.com|chat\.openai\.com|cursor\.(?:com|sh)/|copilot\.microsoft\.com)\S*|\b[\w.+-]+@(?:anthropic|openai)\.com\b`)
)

// assistants is the closed vocabulary a trailer or a footer is measured against.
//
// Closed on purpose, and matched as a substring so "Claude Opus 5 (1M context)"
// and "claude-sonnet-5" both land. It is safe to be this broad only because every
// rule gates it behind a shape first — the list never decides on its own that a
// line is a signature, it decides that a signature names an assistant.
//
// A name not on it is a miss rather than a false positive, which is the trade
// this package is built to make: one wrong finding teaches the user to disbelieve
// every validator scc ships.
var assistants = []string{
	"claude", "anthropic", "chatgpt", "openai", "gpt", "copilot", "codex",
	"cursor", "gemini", "devin", "aider", "opencode", "windsurf", "sourcegraph",
	"cody", "codeium", "tabnine",
}

// Two names are deliberately absent, and both were on the list first. "bot"
// would fire on `dependabot[bot]`, which is a real author of the commit it signs
// rather than somebody signing the user's work — the rule is about a record that
// lies, not about every non-human. And "amp" is a substring of "example",
// "amplify" and "champion", so a trailer naming a person called Champion would
// have been reported. Both are the same mistake: a vocabulary entry short enough
// to appear inside an innocent word costs more than the signature it catches.

// Names reports whether s mentions something on the vocabulary. Exported because
// the vocabulary is the part a caller might reasonably want to ask about.
func Names(s string) bool {
	low := strings.ToLower(s)
	for _, a := range assistants {
		if strings.Contains(low, a) {
			return true
		}
	}
	return false
}

// Scan returns every signature in text, in the order the lines appear.
//
// One line can carry at most one hit, and the rules are tried in the order they
// are declared. That is not a shortcut: the harness default footer is a single
// line carrying a footer phrase *and* a link, and reporting it twice would make
// one mistake look like two — the shape of a validator nobody trusts the count of.
func Scan(text string) []Hit {
	var out []Hit
	for i, line := range strings.Split(normalize(text), "\n") {
		if h, ok := scanLine(line); ok {
			h.Line = i + 1
			out = append(out, h)
		}
	}
	return out
}

func scanLine(line string) (Hit, bool) {
	if m := trailer.FindStringSubmatch(line); m != nil && Names(m[2]) {
		return Hit{Rule: RuleTrailer, Match: strings.TrimSpace(line)}, true
	}
	if footer.MatchString(line) && Names(line) {
		return Hit{Rule: RuleFooter, Match: strings.TrimSpace(line)}, true
	}
	if m := link.FindString(line); m != "" {
		return Hit{Rule: RuleLink, Match: m}, true
	}
	return Hit{}, false
}

// normalize folds CRLF so a message written on Windows scans identically to the
// same message written anywhere else. textutil owns this for files scc writes;
// here the text arrives from git, so it is done at the door.
func normalize(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}
