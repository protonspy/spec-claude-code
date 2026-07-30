package mdscan

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/protonspy/spec-claude-code/internal/textutil"
)

// Document is one parsed Markdown artifact. Line numbers throughout are 1-based,
// so a finding can carry one straight to the user's editor.
//
// This type is where false positives are prevented for every validator at once. A
// `- [ ]` inside a fenced block is documentation about task syntax, not a task. A
// `[[link]]` in a code span is an example, not a wikilink. And an example inside an
// HTML comment is not anything at all — which matters more here than in most
// Markdown tooling, because scc's own templates carry their instructions in HTML
// comments, examples included. A scanner that missed that would ship a tool whose
// first act is to report findings on the file it just generated.
type Document struct {
	Path  string
	Lines []string

	// Body is Lines with everything that is not content blanked out: frontmatter,
	// fenced blocks, and HTML comments. Indices line up with Lines, so a validator
	// scanning for its own grammar gets fence- and comment-awareness for free
	// instead of re-deriving it and getting it wrong.
	Body        []string
	Frontmatter Frontmatter
	Headings    []Heading
	Checkboxes  []Checkbox
	Links       []Link
	Wikilinks   []Wikilink
}

// Heading is an ATX heading (`## Title`). Setext headings (underlined with === or
// ---) are deliberately not recognized: the artifacts scc governs are generated from
// its own templates, none of which use them, and `---` is already the frontmatter
// delimiter.
type Heading struct {
	Line  int
	Level int
	Text  string
	Slug  string // GitHub-compatible anchor, de-duplicated within the document
}

// Checkbox is a task-list item. The grammar layered on top of it — the number, the
// methodology annotation, the requirement citations — belongs to the validators,
// because it is scc's grammar rather than Markdown's.
type Checkbox struct {
	Line    int
	Indent  int    // leading spaces, tabs expanded to 4
	Checked bool   // `- [x]`
	Text    string // everything after the box
}

// Link is an inline link. Target may be empty: a codewiki citation is written
// `[internal/cli/cli.go:48-64]()`, which is a link with no destination on purpose.
type Link struct {
	Line   int
	Text   string
	Target string
}

// Wikilink is `[[target]]` or `[[target|label]]`.
type Wikilink struct {
	Line   int
	Target string
	Label  string
}

var (
	headingRe  = regexp.MustCompile(`^ {0,3}(#{1,6})(\s+(.*?))?\s*$`)
	checkboxRe = regexp.MustCompile(`^([ \t]*)[-*+] \[([ xX])\]\s*(.*)$`)
	fenceRe    = regexp.MustCompile("^[ \t]{0,3}(`{3,}|~{3,})(.*)$")
	linkRe     = regexp.MustCompile(`\[([^\[\]]*)\]\(([^()]*)\)`)
	wikilinkRe = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)
	trailingRe = regexp.MustCompile(`\s+#+\s*$`) // closing sequence of an ATX heading
)

// Parse reads content as one of scc's Markdown artifacts.
//
// The only error it can return comes from the frontmatter parser refusing input it
// cannot read; the body is always scannable, because every construct it does not
// recognize is simply prose.
func Parse(path, content string) (*Document, error) {
	text := textutil.NormalizeNewlines(content)
	doc := &Document{Path: path, Lines: strings.Split(text, "\n")}
	doc.Body = make([]string, len(doc.Lines))

	fm, err := ParseFrontmatter(text)
	doc.Frontmatter = fm
	if err != nil {
		return doc, err
	}

	slugs := map[string]int{}
	var fence string // the open fence's delimiter, "" when not in one
	inComment := false

	for i := fm.Lines; i < len(doc.Lines); i++ {
		line := doc.Lines[i]
		num := i + 1

		// Fences first: an HTML comment marker inside a code block is code, and a
		// fence inside a comment is commented out.
		if fence != "" {
			if closesFence(line, fence) {
				fence = ""
			}
			continue
		}
		if !inComment {
			if open, delim := opensFence(line); open {
				fence = delim
				continue
			}
		}

		body, stillInComment := stripComments(line, inComment)
		inComment = stillInComment
		doc.Body[i] = body
		if strings.TrimSpace(body) == "" {
			continue
		}

		if m := headingRe.FindStringSubmatch(body); m != nil {
			text := strings.TrimSpace(trailingRe.ReplaceAllString(m[3], ""))
			h := Heading{Line: num, Level: len(m[1]), Text: text, Slug: Slug(text)}
			if n := slugs[h.Slug]; n > 0 {
				h.Slug = h.Slug + "-" + strconv.Itoa(n)
			}
			slugs[Slug(text)]++
			doc.Headings = append(doc.Headings, h)
			continue
		}

		if m := checkboxRe.FindStringSubmatch(body); m != nil {
			doc.Checkboxes = append(doc.Checkboxes, Checkbox{
				Line:    num,
				Indent:  indentWidth(m[1]),
				Checked: m[2] != " ",
				Text:    strings.TrimSpace(m[3]),
			})
		}

		// Links and wikilinks are read from the line with code spans masked, so an
		// example in backticks is not mistaken for a reference to something real.
		masked := maskCodeSpans(body)
		for _, m := range wikilinkRe.FindAllStringSubmatch(masked, -1) {
			target, label, _ := strings.Cut(m[1], "|")
			doc.Wikilinks = append(doc.Wikilinks, Wikilink{
				Line:   num,
				Target: strings.TrimSpace(target),
				Label:  strings.TrimSpace(label),
			})
		}
		for _, m := range linkRe.FindAllStringSubmatch(masked, -1) {
			doc.Links = append(doc.Links, Link{Line: num, Text: m[1], Target: strings.TrimSpace(m[2])})
		}
	}
	return doc, nil
}

// Content returns the 1-based line n with frontmatter, fences, and comments already
// stripped — the text a validator should apply its grammar to.
func (d *Document) Content(n int) string {
	if n < 1 || n > len(d.Body) {
		return ""
	}
	return d.Body[n-1]
}

// Line returns the 1-based line n, or "" when n is out of range. Callers report
// line numbers they got from this package, so an out-of-range n is a bug rather
// than user input — returning "" keeps it from being a panic in a lint run.
func (d *Document) Line(n int) string {
	if n < 1 || n > len(d.Lines) {
		return ""
	}
	return d.Lines[n-1]
}

func opensFence(line string) (bool, string) {
	m := fenceRe.FindStringSubmatch(line)
	if m == nil {
		return false, ""
	}
	// A tilde fence's info string may contain anything; a backtick fence's may not
	// contain a backtick, which is what keeps `` `code` `` from opening a block.
	if m[1][0] == '`' && strings.Contains(m[2], "`") {
		return false, ""
	}
	return true, m[1]
}

// closesFence reports whether line closes an open fence. The closing run must be at
// least as long as the opening one and carry no info string — and it may be
// tab-indented, which a naive prefix check on spaces misses.
func closesFence(line, open string) bool {
	m := fenceRe.FindStringSubmatch(line)
	if m == nil {
		return false
	}
	if m[1][0] != open[0] || len(m[1]) < len(open) {
		return false
	}
	return strings.TrimSpace(m[2]) == ""
}

// stripComments blanks out the HTML-comment spans in a line and reports whether a
// comment is still open at the end of it.
func stripComments(line string, inComment bool) (string, bool) {
	var out strings.Builder
	rest := line
	for {
		if inComment {
			end := strings.Index(rest, "-->")
			if end < 0 {
				return out.String(), true
			}
			rest = rest[end+3:]
			inComment = false
			continue
		}
		start := strings.Index(rest, "<!--")
		if start < 0 {
			out.WriteString(rest)
			return out.String(), false
		}
		out.WriteString(rest[:start])
		rest = rest[start+4:]
		inComment = true
	}
}

// maskCodeSpans replaces the contents of `backtick spans` with spaces, preserving
// every byte offset so line-local positions still line up.
func maskCodeSpans(line string) string {
	out := []byte(line)
	i := 0
	for i < len(out) {
		if out[i] != '`' {
			i++
			continue
		}
		open := i
		for i < len(out) && out[i] == '`' {
			i++
		}
		delim := string(out[open:i])
		end := strings.Index(line[i:], delim)
		if end < 0 {
			// An unterminated span is literal text, not a span.
			return string(out)
		}
		for j := i; j < i+end; j++ {
			out[j] = ' '
		}
		i += end + len(delim)
	}
	return string(out)
}

// Slug renders heading text as a GitHub-compatible anchor: lowercased, spaces to
// hyphens, everything that is not a letter, digit, hyphen, or underscore dropped.
// Implemented here rather than assumed, because the codewiki validator checks that
// a slug was derived from its heading, and "derived" has to mean one specific thing.
func Slug(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func indentWidth(s string) int {
	n := 0
	for _, r := range s {
		if r == '\t' {
			n += 4
			continue
		}
		n++
	}
	return n
}
