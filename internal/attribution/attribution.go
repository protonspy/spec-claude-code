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
// appearing in a sentence: `chore(deps): bump the anthropic SDK to 0.40` and
// `feat(harness): add opencode support` are ordinary commits, and a check that
// flagged them would be suppressed within a day — which costs more than every
// signature it would ever catch. Every rule below is a trailer, a footer phrase, a
// link, a badge, or a line that says nothing but the name; the vocabulary only
// ever decides whether that shape names an assistant.
package attribution

import (
	"regexp"
	"strings"
	"unicode"
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
	// RuleLink is a session link, a vendor's own page, or a vendor no-reply
	// address. It is separate from the other two because it survives them: a
	// footer rewritten by hand usually keeps the URL.
	//
	// It fires wherever the URL sits — inside prose, inside a parenthesis, inside
	// an HTML comment — and the one thing that buys an exemption is being
	// *documentation*. That split replaces an earlier one that asked whether the
	// URL was alone on its line, and it was measured rather than argued: a harness
	// told not to sign its work wrote the product link into a sentence, and a rule
	// that only looked at lines standing by themselves read it as prose. A product
	// page is nobody's citation, so the position it sits in says nothing.
	//
	// Documentation keeps the old gate. `code.claude.com/docs/…` is a source, and
	// a technical argument that cannot cite the vendor's own documentation is one
	// nobody can check — so a docs URL is reported only when it stands alone,
	// where it is the footer's surviving half rather than a reference.
	RuleLink = "assistant-link"
	// RuleBadge is a Markdown link or image whose label or target names an
	// assistant — the `[Claude Code](…)` half of the footer the harnesses append.
	// It is its own rule because the phrase in front of it is the part a session
	// rewrites: a badge with no footer phrase in front of it and no vendor host
	// behind it is still a signature.
	RuleBadge = "assistant-badge"
	// RuleMention is a line that is nothing but an assistant's name, sitting
	// where a signature sits — "Claude Code" behind a robot emoji, "— Claude Opus
	// 5", a last line reading "via Codex". It is what a stripped-down footer
	// collapses to once the phrase, the badge and the link are all gone.
	//
	// It is the one rule where the vocabulary comes closest to doing the naming,
	// so it is gated twice: the line has to say nothing else, *and* it has to be
	// in a signature's position — under a symbol, behind a dash, or at the end of
	// the message. Without the second gate it reports "Claude Code, Codex, and
	// opencode", which is a sentence this project's own pull requests write.
	RuleMention = "assistant-mention"
)

// The shapes. Compiled once, applied per line, and deliberately anchored: a
// pattern that could match mid-sentence would fire on a commit message that
// merely quotes one of these lines, which is exactly what a fix for this finding
// looks like.
var (
	// A git trailer: a known key at the start of a line, with a value.
	trailer = regexp.MustCompile(`(?i)^[ \t]*(co-?authored-by|assisted-by|generated-by|signed-off-by)[ \t]*:[ \t]*(\S.*?)[ \t]*$`)
	// The footer phrase, and every other way a line hands this change to whoever
	// it names next. It is only half a shape: the caller requires the name to
	// follow it on the same line, so a sentence that happens to say "written by"
	// about something else stays prose.
	//
	// Three families, and the second and third are what a session writes once it
	// has been told not to write the first. "Generated with" is the harness
	// default; "with the help of" and "thanks to" are what it collapses to when
	// somebody rephrases it in their own words; and the verb list is wider than
	// the footer's because the sentence being rewritten is about this work, so any
	// verb for making it will do.
	footer = regexp.MustCompile(`(?i)\b(?:` +
		`(?:generated|created|authored|written|made|built|produced|drafted|implemented|developed|refactored|scaffolded|assembled|composed|prepared|completed|reviewed)\s+(?:with|by|using|via)` +
		`|(?:with|using)\s+(?:the\s+)?(?:help|assistance|support|aid)\s+(?:of|from)` +
		`|thanks\s+to|powered\s+by|driven\s+by|came\s+from|courtesy\s+of` +
		`|pair(?:ed|[- ]programmed)?\s+with|in\s+(?:collaboration|partnership)\s+with` +
		`)\b`)
	// The other half of crediting: the assistant as the instrument rather than the
	// agent — "used Claude Code to draft the tests". The name has to be what
	// follows the verb, not merely somewhere after it, because the passive reading
	// of the same words is ordinary technical prose: "the settings format used by
	// Claude Code" describes a product, it does not hand it the work.
	useVerb = regexp.MustCompile(`(?i)\b(?:used|using|ran)\s+(?:the\s+|a\s+|an\s+|my\s+|our\s+)?`)
	// A session link, a vendor's own page, or a vendor no-reply address.
	//
	// Whole hosts, rather than the one path today's footer happens to use.
	// claude.com/claude-code is the spelling that walked past a rule written
	// around claude.ai, and the next rewording moves the path again.
	link = regexp.MustCompile(`(?i)\b(?:https?://)?(?:[\w-]+\.)*(?:claude\.(?:ai|com)|anthropic\.com|chatgpt\.com|openai\.com|cursor\.(?:com|sh)|copilot\.microsoft\.com|codeium\.com|windsurf\.com|sourcegraph\.com/cody)(?:/\S*)?|\b[\w.+-]+@(?:anthropic|openai)\.com\b`)
	// What makes a vendor URL citable: a documentation host, or a path whose first
	// meaningful segment is documentation. Everything else on a vendor host is a
	// product page, and nobody cites a product page to explain a change.
	docHost = regexp.MustCompile(`(?i)^(?:https?://)?(?:docs?|developers?|platform|api|help|support|code)\.`)
	docPath = regexp.MustCompile(`(?i)^(?:https?://)?[^/\s]+/(?:[\w-]+/){0,3}?(?:docs?|reference|api|guides?|blog|news|engineering|research|help|support|changelog|papers?)(?:[/#?]|$)`)
	// A Markdown link or image: a label in brackets and a target in parentheses,
	// with an optional leading bang. Both halves are measured, so a badge survives
	// having either one of them rewritten.
	badge = regexp.MustCompile(`!?\[([^\]\n]{0,160})\]\(([^)\n]{0,400})\)`)
	// What a line carries besides its words: the robot, a rule, a dash, emphasis,
	// a bullet, a trailing stop — everything a footer keeps once the phrase in
	// front of it is gone.
	decoration = regexp.MustCompile(`^[\s\pP\pS]+|[\s\pP\pS]+$`)
	// A session or a share link: a vendor URL whose path names one conversation.
	//
	// This is the half of the link rule that stays unconditional, wherever on the
	// line it sits. Documentation on a vendor host is cited by ordinary technical
	// writing — `code.claude.com/docs/…` is a source, and an argument that cannot
	// cite the vendor's own documentation is one nobody can check. A link to
	// somebody's session is not a source; it is the footer's surviving half.
	session = regexp.MustCompile(`(?i)\b(?:https?://)?(?:[\w-]+\.)*(?:claude\.(?:ai|com)|chatgpt\.com|openai\.com)/(?:code/)?(?:session|share|chat|c)[/_-]\S+`)
	// One word of a line, for the mention rule.
	word = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{N}.+-]*`)
)

// assistants is the closed vocabulary a trailer, a footer, a badge or a mention
// is measured against.
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

// filler is what a bare mention may say around the name: a product word, a model,
// a size, a connective. The list is short on purpose — every entry widens what
// counts as "the line says nothing else", and the mention rule is sound only
// while that phrase stays true.
var filler = map[string]bool{
	"code": true, "cli": true, "ai": true, "agent": true, "app": true,
	"model": true, "opus": true, "sonnet": true, "haiku": true, "fable": true,
	"gpt": true, "pro": true, "max": true, "mini": true, "turbo": true,
	"preview": true, "context": true, "via": true, "with": true, "by": true,
	"using": true, "and": true, "the": true, "powered": true, "assisted": true,
	"help": true, "from": true, "sdk": true, "api": true, "v": true,
}

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
// line carrying a footer phrase, a badge *and* a link, and reporting it three
// times would make one mistake look like three — the shape of a validator nobody
// trusts the count of.
func Scan(text string) []Hit {
	lines := strings.Split(normalize(text), "\n")
	last := lastWritten(lines)
	var out []Hit
	for i, line := range lines {
		if h, ok := scanLine(line, i == last); ok {
			h.Line = i + 1
			out = append(out, h)
		}
	}
	return out
}

// lastWritten is the index of the final non-blank line — where a footer lands,
// and the position the mention rule needs to know about. -1 when the text is
// blank.
func lastWritten(lines []string) int {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return i
		}
	}
	return -1
}

func scanLine(line string, last bool) (Hit, bool) {
	if m := trailer.FindStringSubmatch(line); m != nil && Names(m[2]) {
		return Hit{Rule: RuleTrailer, Match: strings.TrimSpace(line)}, true
	}
	if credited(line) || instrumental(line) {
		return Hit{Rule: RuleFooter, Match: strings.TrimSpace(line)}, true
	}
	if m := badge.FindStringSubmatch(line); m != nil && (Names(m[1]) || Names(m[2])) && signsAlone(line, m[0]) {
		return Hit{Rule: RuleBadge, Match: strings.TrimSpace(m[0])}, true
	}
	if m := link.FindString(line); m != "" && linkSigns(line, m) {
		return Hit{Rule: RuleLink, Match: m}, true
	}
	if m, ok := mention(line, last); ok {
		return Hit{Rule: RuleMention, Match: m}, true
	}
	return Hit{}, false
}

// credited reports a line that hands this change to whoever it names: the footer
// phrase, or one of the ways somebody rephrases it.
//
// The name has to come *after* the phrase, not merely somewhere on the line.
// Measured on this project's own history: "opencode has one AGENTS.md written by
// whichever ran first" carries the phrase and the vocabulary and is a sentence
// about opencode, not a line signed by it.
//
// And the occurrence must not be quoted, for the reason instrumental gives below:
// a commit explaining this rule quotes the phrases it catches, and the rule that
// reported that would fire on the fix for its own finding.
func credited(line string) bool {
	for _, at := range footer.FindAllStringIndex(line, -1) {
		if Names(line[at[1]:]) && !quoted(line[:at[0]]) {
			return true
		}
	}
	return false
}

// instrumental reports a line that uses an assistant to do this work — "used
// Claude Code to draft the tests", "ran Codex over the diff".
//
// It is the footer rule's other voice, and it is separated from it because it
// needs a stricter test. The footer phrase can look anywhere after itself for the
// name, since "generated with" is already about this change. "Used" is not: the
// same verb in the passive describes a product rather than crediting one, and
// this repository's own history writes that sentence — "the settings format used
// by Claude Code" is prose about a harness scc supports.
//
// So the name has to be the verb's own object — the very next word, article
// aside — and it must not be possessive. "Used by Claude Code" puts a preposition
// there and is the passive; "using Claude Code's hook surface" is a sentence
// about a hook surface; "using Claude Code to write this" hands over the work.
// Those two tests are cheap, and they stand in for a distinction that would
// otherwise need a parser.
// A quoted occurrence is skipped, which is the anchoring the other rules get from
// their shape and this one has nowhere else to get. Measured on this package's own
// history: a commit explaining this very rule quotes the line it now catches, and
// a check that reported that would fire on the fix for its own finding — the
// failure the package comment names at the top of this file.
func instrumental(line string) bool {
	for _, at := range useVerb.FindAllStringIndex(line, -1) {
		rest := strings.Fields(line[at[1]:])
		switch {
		case len(rest) == 0, !Names(rest[0]):
		case strings.ContainsAny(firstWords(rest, 3), "'’"):
		case quoted(line[:at[0]]):
		default:
			return true
		}
	}
	return false
}

// quoted reports whether what follows this much of a line sits inside a
// quotation: an odd number of straight quotes or backticks behind it, or a
// typographic quotation still open.
//
// The apostrophe is deliberately not counted. It is a possessive in ordinary
// English far more often than it opens anything, and instrumental already reads
// one three words along to tell a credit from a description.
func quoted(before string) bool {
	return strings.Count(before, `"`)%2 == 1 ||
		strings.Count(before, "`")%2 == 1 ||
		strings.Count(before, "“") > strings.Count(before, "”")
}

// firstWords is the opening of a fragment, punctuation kept. Kept rather than
// stripped because the punctuation is what instrumental reads: an apostrophe
// three words in is the difference between a credit and a description.
func firstWords(f []string, n int) string {
	if len(f) > n {
		f = f[:n]
	}
	return strings.Join(f, " ")
}

// linkSigns decides whether a vendor URL on this line is a signature.
//
// A session link and a no-reply address always are: nobody cites either to
// explain a change. Documentation is the one exemption, and it keeps the older
// "alone on its line" gate, because a docs URL in a sentence is a source and the
// same URL by itself at the end of a message is the footer with the words taken
// off. Everything else on a vendor host is a product page, and a product page is
// a signature wherever it sits — in prose, in a parenthesis, or inside an HTML
// comment, which is where one was measured getting through.
func linkSigns(line, m string) bool {
	switch {
	case session.MatchString(m), strings.Contains(m, "@"):
		return true
	case docHost.MatchString(m), docPath.MatchString(m):
		return signsAlone(line, m)
	default:
		return true
	}
}

// mention reports a line that is nothing but an assistant's name, in the place a
// signature goes.
//
// Two gates, and both are needed. The line has to say nothing else: strip the
// decoration a footer keeps, then require every remaining word to be the name
// itself or one of a short list of product words, under a cap — "fix(cli): stop
// claude-sonnet-5 being spelled two ways" names an assistant in a sentence this
// will never accept.
//
// And it has to sit where a signature sits: behind a symbol or a dash, or at the
// end of the message. That is what separates a robot emoji followed by "Claude
// Code" from "Claude Code, Codex, and opencode" — a list this project writes in
// its own pull requests, and the false positive the first gate alone would
// produce. A Markdown bullet is deliberately not a trigger, for the same reason.
func mention(line string, last bool) (string, bool) {
	s := strings.TrimSpace(decoration.ReplaceAllString(line, ""))
	if s == "" || !Names(s) || !(last || signed(line)) {
		return "", false
	}
	words := word.FindAllString(s, -1)
	if len(words) == 0 || len(words) > 6 {
		return "", false
	}
	named := false
	for _, w := range words {
		low := strings.ToLower(w)
		switch {
		// A file is not a name, and this repository is the reason the distinction
		// had to be drawn: `CLAUDE.md` is a path the vocabulary matches on, so a
		// last line naming the file — which a review bot writes and a commit body
		// listing what it touched writes too — was read as somebody signing it.
		// The line says something else, so it is not a bare mention.
		case isFilename(low):
			return "", false
		case Names(low):
			named = true
		case filler[low], isVersion(low):
		default:
			return "", false
		}
	}
	if !named {
		return "", false
	}
	return strings.TrimSpace(line), true
}

// signed reports whether a line opens the way a signature opens: a symbol — the
// robot, a sparkle, a trademark — or a typographic dash.
//
// The hyphen, the asterisk and the angle bracket are all absent, and that is the
// point: they open a Markdown bullet and a quote, which is how an ordinary list
// of harnesses starts. A signature's decoration and a list's bullet look alike
// enough that guessing between them is how this rule would earn its first false
// positive.
//
// Saying that was not enough to make it true, and the gap was measured rather
// than argued. `unicode.IsSymbol` returns true for `>`, which is Sm — so every
// blockquoted line opened a signature, and `> Claude Code, Codex, and opencode`
// was reported: the very sentence the rule above promises never to report, quoted
// back. A review bot's comment is written entirely in blockquotes, and `--pr` now
// reads comments, so the paragraph and the code had to be made to agree.
//
// The symbol has to be non-ASCII. That is what a signature's decoration actually
// is — the robot, a sparkle, a trademark — while ASCII's symbol block is Markdown
// punctuation: `>`, `|`, `=`, `+`, `^`, the backtick, `~`. The one ASCII
// character kept is the tilde, named above and never a Markdown opener at the
// start of a line.
func signed(line string) bool {
	for _, r := range strings.TrimLeft(line, " \t") {
		return (unicode.IsSymbol(r) && r > unicode.MaxASCII) || r == '—' || r == '–' || r == '~'
	}
	return false
}

// isFilename reports a token that is a file rather than a name: a dot with a
// short alphabetic tail, which is an extension.
//
// It sits beside isVersion because it is the same kind of test — a shape rather
// than a word — and it is deliberately narrow. A model name keeps its dots
// ("claude-3.5-sonnet" ends in "5-sonnet", which is not an extension), and a
// vendor host ending in one ("claude.ai") is the link rule's to report and is
// reached before this.
func isFilename(w string) bool {
	i := strings.LastIndexByte(w, '.')
	if i <= 0 || i == len(w)-1 {
		return false
	}
	ext := w[i+1:]
	if len(ext) > 4 {
		return false
	}
	return strings.IndexFunc(ext, func(r rune) bool { return !unicode.IsLetter(r) }) < 0
}

// isVersion accepts the scraps of a model name that are not words: "5", "4.5",
// "v2", "1m". It sits beside filler rather than in it because it is a shape too.
func isVersion(w string) bool {
	w = strings.TrimPrefix(w, "v")
	w = strings.TrimSuffix(w, "m")
	if w == "" {
		return false
	}
	return strings.IndexFunc(w, func(r rune) bool {
		return (r < '0' || r > '9') && r != '.'
	}) < 0
}

// normalize folds CRLF so a message written on Windows scans identically to the
// same message written anywhere else. textutil owns this for files scc writes;
// here the text arrives from git, so it is done at the door.
func normalize(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

// badgeSigns gates the badge rule the way mention is gated: the badge has to be
// the line, not a link inside a sentence.
//
// The rule exists for the half of the harness footer that survives when a session
// is told not to write one — `Generated with [Claude Code](…)` loses the phrase
// and keeps the badge, alone on its own line at the end. What it must not do is
// fire on a **citation**, and the two are the same shape with different
// neighbours: a signature stands by itself, a citation is embedded in prose.
//
// Measured on this project's own pull request, which is the worst place to find
// out: "…and [Anthropic's own guidance](https://code.claude.com/docs/en/devcontainer)
// warns that under --dangerously-skip-permissions…" was reported as a signature.
// It is the opposite — it is sourcing a claim, and a technical argument that
// cannot cite the vendor's documentation is one nobody can check. A validator
// that fires on scc's own output is the worst bug in this product, because one
// wrong finding teaches the reader to disbelieve the other ten.
//
// So: strip the badge out of the line, strip the decoration a footer keeps, and
// require nothing else to be left. A bullet, a robot emoji, an em dash and
// trailing punctuation are decoration; a verb is prose.
func signsAlone(line, match string) bool {
	rest := strings.Replace(line, match, "", 1)
	rest = decoration.ReplaceAllString(rest, "")
	return strings.TrimSpace(rest) == ""
}
