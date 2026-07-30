package validate

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/mdscan"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// citationRe matches a codewiki citation: `[internal/cli/cli.go:48-64]()`, or a single
// line, `[internal/cli/cli.go:48]()`.
var citationRe = regexp.MustCompile(`^(.+?):(\d+)(?:-(\d+))?$`)

// Codewiki validates docs/codewiki/.
//
// This is the heaviest validator and the one most likely to be wrong, because it is the
// only one that resolves anything against the live checkout. It still reads no code: a
// citation names a file and a line range, and whether that file has that many lines is
// arithmetic, not comprehension.
//
// What it holds pages to:
//
//   - every citation resolves — the file exists and actually has those lines
//   - every section cites something, because a section that cites nothing is prose that
//     has drifted free of the code it describes
//   - headings are unique within a page, so a slug identifies one section
//
// The `Structure` tree check named in the design is deliberately absent: the design has
// not settled what that tree looks like on disk, and a check written against a guess
// would be the false positive that costs the user's trust in the other three.
func Codewiki(root string) (*finding.Set, error) {
	set := &finding.Set{}
	files, err := markdownFiles(paths.Codewiki(root))
	if err != nil {
		return nil, err
	}
	for _, path := range files {
		if err := codewikiPage(set, root, path); err != nil {
			return nil, err
		}
	}
	return set, nil
}

func codewikiPage(set *finding.Set, root, path string) error {
	file := rel(root, path)
	doc, err := read(root, path)
	if err != nil {
		if doc == nil {
			return err
		}
		set.Addf(file, 1, "codewiki.frontmatter-unreadable", "%v", err)
		return nil
	}

	checkCodewikiHeadings(set, file, doc)

	// Citations, and which section each one belongs to, so "no section cites anything"
	// can be answered per section rather than per page.
	cited := map[int]bool{} // heading line -> saw a citation under it
	for _, link := range doc.Links {
		if link.Target != "" {
			continue // an ordinary link, not a citation
		}
		m := citationRe.FindStringSubmatch(link.Text)
		if m == nil {
			continue
		}
		cited[sectionOf(doc, link.Line)] = true
		checkCitation(set, root, file, link.Line, m)
	}

	for _, h := range doc.Headings {
		// Level 1 is the page title, which is not a section.
		if h.Level < 2 || cited[h.Line] {
			continue
		}
		set.Addf(file, h.Line, "codewiki.section-cites-nothing",
			"%q cites no code; a section with no citation is prose that has drifted free of what it describes", h.Text)
	}
	return nil
}

// checkCitation resolves one citation against the checkout.
func checkCitation(set *finding.Set, root, file string, line int, m []string) {
	target, startText, endText := m[1], m[2], m[3]
	start, _ := strconv.Atoi(startText)
	end := start
	if endText != "" {
		end, _ = strconv.Atoi(endText)
	}

	if start < 1 {
		set.Addf(file, line, "codewiki.citation-invalid", "%s: line numbers start at 1", target)
		return
	}
	if end < start {
		set.Addf(file, line, "codewiki.citation-invalid",
			"%s:%d-%d ends before it starts", target, start, end)
		return
	}

	resolved := filepath.Join(root, filepath.FromSlash(target))
	if !isFile(resolved) {
		set.Addf(file, line, "codewiki.citation-unresolved", "%s does not exist", target)
		return
	}
	b, err := os.ReadFile(resolved)
	if err != nil {
		// Unreadable is not the same as wrong, and scc will not report a finding it
		// cannot stand behind.
		return
	}
	if lines := countLines(b); end > lines {
		set.Addf(file, line, "codewiki.citation-out-of-range",
			"%s:%d-%d, but the file has %d lines — the code moved and the citation did not",
			target, start, end, lines)
	}
}

// checkCodewikiHeadings holds slugs to being unique and derived from their headings. A
// duplicated heading gives two sections one anchor, and a link to it lands on whichever
// came first.
func checkCodewikiHeadings(set *finding.Set, file string, doc *mdscan.Document) {
	seen := map[string]int{}
	for _, h := range doc.Headings {
		if h.Text == "" {
			set.Addf(file, h.Line, "codewiki.empty-heading", "a heading with no text has no slug to link to")
			continue
		}
		slug := mdscan.Slug(h.Text)
		if slug == "" {
			set.Addf(file, h.Line, "codewiki.unslugged-heading",
				"%q yields no anchor; a heading needs at least one letter or digit", h.Text)
			continue
		}
		if prior, dup := seen[slug]; dup {
			set.Addf(file, h.Line, "codewiki.duplicate-heading",
				"%q repeats the heading on line %d, so both sections share one anchor", h.Text, prior)
			continue
		}
		seen[slug] = h.Line
	}
}

// sectionOf returns the line of the nearest heading at or above line, or 0.
func sectionOf(doc *mdscan.Document, line int) int {
	best := 0
	for _, h := range doc.Headings {
		if h.Line <= line && h.Line > best {
			best = h.Line
		}
	}
	return best
}

// countLines counts the lines in a file the way an editor numbers them: a trailing
// newline does not add a line, so the last line of a well-formed file is its content
// rather than the empty string after it.
func countLines(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	n := bytes.Count(b, []byte("\n"))
	if !strings.HasSuffix(string(b), "\n") {
		n++
	}
	return n
}
