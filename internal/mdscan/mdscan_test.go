package mdscan

import (
	"strings"
	"testing"
)

func parse(t *testing.T, content string) *Document {
	t.Helper()
	doc, err := Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return doc
}

func TestHeadings(t *testing.T) {
	doc := parse(t, "# Title\n\ntext\n\n## Sub section\n\n### Deep ##\n")
	if len(doc.Headings) != 3 {
		t.Fatalf("headings = %d, want 3: %+v", len(doc.Headings), doc.Headings)
	}
	want := []Heading{
		{Line: 1, Level: 1, Text: "Title", Slug: "title"},
		{Line: 5, Level: 2, Text: "Sub section", Slug: "sub-section"},
		{Line: 7, Level: 3, Text: "Deep", Slug: "deep"}, // the closing sequence is not text
	}
	for i, w := range want {
		if doc.Headings[i] != w {
			t.Errorf("heading %d = %+v, want %+v", i, doc.Headings[i], w)
		}
	}
}

// Two headings with the same text get distinct slugs, the way GitHub numbers them —
// the codewiki validator checks slug uniqueness, and it has to agree with what an
// anchor actually resolves to.
func TestDuplicateHeadingsGetNumberedSlugs(t *testing.T) {
	doc := parse(t, "## Notes\n\n## Notes\n\n## Notes\n")
	var slugs []string
	for _, h := range doc.Headings {
		slugs = append(slugs, h.Slug)
	}
	if strings.Join(slugs, ",") != "notes,notes-1,notes-2" {
		t.Errorf("slugs = %v, want notes,notes-1,notes-2", slugs)
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Simple":                     "simple",
		"With Spaces":                "with-spaces",
		"Punctuation! (and) more?":   "punctuation-and-more",
		"R1.2 · the numbered clause": "r12--the-numbered-clause",
		"under_score-and-hyphen":     "under_score-and-hyphen",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCheckboxes(t *testing.T) {
	doc := parse(t, "- [ ] 1.1 (Unit) first\n- [x] 1.2 (TDD) second\n  - [X] 1.3 nested\n* [ ] star bullet\n- not a checkbox\n")
	if len(doc.Checkboxes) != 4 {
		t.Fatalf("checkboxes = %d, want 4: %+v", len(doc.Checkboxes), doc.Checkboxes)
	}
	if doc.Checkboxes[0].Checked || !doc.Checkboxes[1].Checked || !doc.Checkboxes[2].Checked {
		t.Errorf("checked flags wrong: %+v", doc.Checkboxes)
	}
	if doc.Checkboxes[2].Indent != 2 {
		t.Errorf("indent = %d, want 2", doc.Checkboxes[2].Indent)
	}
	if doc.Checkboxes[0].Text != "1.1 (Unit) first" {
		t.Errorf("text = %q, want the item without the box", doc.Checkboxes[0].Text)
	}
}

// The single most important property in this package: a task-syntax example inside a
// fenced block is documentation, not a task. Without it every rule file that
// documents the grammar would produce findings.
func TestFencedBlocksAreNotContent(t *testing.T) {
	doc := parse(t, strings.Join([]string{
		"# Real heading",
		"```",
		"# not a heading",
		"- [ ] not a task",
		"[[not a wikilink]]",
		"[not a link](target.md)",
		"```",
		"- [ ] 1.1 (Unit) a real task",
		"",
	}, "\n"))
	if len(doc.Headings) != 1 || doc.Headings[0].Text != "Real heading" {
		t.Errorf("headings = %+v, want only the real one", doc.Headings)
	}
	if len(doc.Checkboxes) != 1 || doc.Checkboxes[0].Line != 8 {
		t.Errorf("checkboxes = %+v, want only the one after the fence", doc.Checkboxes)
	}
	if len(doc.Wikilinks) != 0 || len(doc.Links) != 0 {
		t.Errorf("links inside the fence were collected: %+v %+v", doc.Links, doc.Wikilinks)
	}
}

// scc's own templates carry their instructions in HTML comments, examples included.
// A scanner that missed that would ship a tool whose first act is to report findings
// on the file it just generated.
func TestHTMLCommentsAreNotContent(t *testing.T) {
	doc := parse(t, strings.Join([]string{
		"<!-- Grammar:",
		"     - [ ] <number> (Unit|TDD) <description>",
		"     ## not a heading",
		"     Delete this comment. -->",
		"",
		"- [ ] 1.1 (Unit) the only task here",
		"prose <!-- an aside --> continues",
		"",
	}, "\n"))
	if len(doc.Checkboxes) != 1 || doc.Checkboxes[0].Line != 6 {
		t.Errorf("checkboxes = %+v, want only the real one at line 6", doc.Checkboxes)
	}
	if len(doc.Headings) != 0 {
		t.Errorf("a commented heading was collected: %+v", doc.Headings)
	}
}

// A fence inside a comment is commented out; a comment marker inside a fence is code.
func TestFencesAndCommentsDoNotConfuseEachOther(t *testing.T) {
	doc := parse(t, strings.Join([]string{
		"```",
		"<!-- this is code, and it never opens a comment",
		"```",
		"## a real heading",
		"<!--",
		"```",
		"## commented out",
		"```",
		"-->",
		"## another real heading",
		"",
	}, "\n"))
	var texts []string
	for _, h := range doc.Headings {
		texts = append(texts, h.Text)
	}
	if strings.Join(texts, "|") != "a real heading|another real heading" {
		t.Errorf("headings = %v, want the two real ones", texts)
	}
}

func TestTabIndentedFenceStillCloses(t *testing.T) {
	doc := parse(t, "\t```\n- [ ] inside\n\t```\n- [ ] 1.1 outside\n")
	if len(doc.Checkboxes) != 1 || doc.Checkboxes[0].Text != "1.1 outside" {
		t.Errorf("checkboxes = %+v, want only the one outside the fence", doc.Checkboxes)
	}
}

// A longer closing run closes the block; a shorter one does not, so a nested
// three-backtick example inside a four-backtick block stays inside it.
func TestFenceLengthsMustMatch(t *testing.T) {
	doc := parse(t, "````\n```\n- [ ] still inside\n```\n````\n- [ ] 1.1 out\n")
	if len(doc.Checkboxes) != 1 || doc.Checkboxes[0].Text != "1.1 out" {
		t.Errorf("checkboxes = %+v, want only the one after the outer fence", doc.Checkboxes)
	}
}

func TestTildeFences(t *testing.T) {
	doc := parse(t, "~~~go\n- [ ] inside\n~~~\n- [ ] 1.1 out\n")
	if len(doc.Checkboxes) != 1 {
		t.Errorf("checkboxes = %+v, want only the one outside", doc.Checkboxes)
	}
}

// An inline code span is an example. `[[link]]` in backticks refers to nothing.
func TestCodeSpansAreNotLinks(t *testing.T) {
	doc := parse(t, "See `[[example]]` and `[text](target.md)`, but [[real]] and [here](real.md) count.\n")
	if len(doc.Wikilinks) != 1 || doc.Wikilinks[0].Target != "real" {
		t.Errorf("wikilinks = %+v, want only [[real]]", doc.Wikilinks)
	}
	if len(doc.Links) != 1 || doc.Links[0].Target != "real.md" {
		t.Errorf("links = %+v, want only real.md", doc.Links)
	}
}

// A single backtick that never closes is literal text, not the start of a span that
// swallows the rest of the line.
func TestUnterminatedCodeSpanDoesNotSwallowTheLine(t *testing.T) {
	doc := parse(t, "a ` stray backtick and [[real]]\n")
	if len(doc.Wikilinks) != 1 {
		t.Errorf("wikilinks = %+v, want the one after the stray backtick", doc.Wikilinks)
	}
}

func TestWikilinkLabels(t *testing.T) {
	doc := parse(t, "[[page-one]] and [[page-two|a nicer label]]\n")
	if len(doc.Wikilinks) != 2 {
		t.Fatalf("wikilinks = %+v, want 2", doc.Wikilinks)
	}
	if doc.Wikilinks[0].Target != "page-one" || doc.Wikilinks[0].Label != "" {
		t.Errorf("first = %+v", doc.Wikilinks[0])
	}
	if doc.Wikilinks[1].Target != "page-two" || doc.Wikilinks[1].Label != "a nicer label" {
		t.Errorf("second = %+v", doc.Wikilinks[1])
	}
}

// A codewiki citation is a link with no destination on purpose, so an empty target
// must survive parsing rather than being discarded as malformed.
func TestEmptyLinkTargetIsPreserved(t *testing.T) {
	doc := parse(t, "as written in [internal/cli/cli.go:48-64]()\n")
	if len(doc.Links) != 1 {
		t.Fatalf("links = %+v, want 1", doc.Links)
	}
	if doc.Links[0].Text != "internal/cli/cli.go:48-64" || doc.Links[0].Target != "" {
		t.Errorf("link = %+v, want the citation with an empty target", doc.Links[0])
	}
}

// Frontmatter is skipped rather than scanned: a `#` in a value is not a heading, and
// line numbers after the block still have to be right.
func TestFrontmatterIsSkippedButCounted(t *testing.T) {
	doc := parse(t, "---\nautonomy: auto\n---\n\n# Title\n")
	if got, _ := doc.Frontmatter.Get("autonomy"); got != "auto" {
		t.Errorf("frontmatter not parsed: %+v", doc.Frontmatter)
	}
	if len(doc.Headings) != 1 || doc.Headings[0].Line != 5 {
		t.Errorf("heading = %+v, want one at line 5", doc.Headings)
	}
}

// Malformed frontmatter is returned as an error along with a usable document, so a
// validator can report the frontmatter problem without losing the rest of the file.
func TestParseReturnsBothErrorAndDocument(t *testing.T) {
	doc, err := Parse("test.md", "---\ntools:\n  - Read\n---\n# Title\n")
	if err == nil {
		t.Error("malformed frontmatter parsed without an error")
	}
	if doc == nil || len(doc.Lines) == 0 {
		t.Error("no document was returned alongside the error")
	}
}

// The working tree's line endings are not part of the contract.
func TestCRLFScansIdentically(t *testing.T) {
	body := "# Title\n\n- [ ] 1.1 (Unit) task\n\n[[link]]\n"
	lf := parse(t, body)
	crlf := parse(t, strings.ReplaceAll(body, "\n", "\r\n"))
	if len(lf.Headings) != len(crlf.Headings) ||
		len(lf.Checkboxes) != len(crlf.Checkboxes) ||
		len(lf.Wikilinks) != len(crlf.Wikilinks) {
		t.Errorf("CRLF scanned differently: %+v vs %+v", crlf, lf)
	}
	if lf.Checkboxes[0].Line != crlf.Checkboxes[0].Line {
		t.Errorf("line numbers differ under CRLF: %d vs %d", crlf.Checkboxes[0].Line, lf.Checkboxes[0].Line)
	}
}

// Body gives every validator fence- and comment-awareness for free. A validator that
// re-derived it would get it wrong in a different way each time.
func TestBodyBlanksOutEverythingThatIsNotContent(t *testing.T) {
	doc := parse(t, strings.Join([]string{
		"---",
		"autonomy: auto",
		"---",
		"real prose",
		"```",
		"code line",
		"```",
		"<!-- commented -->",
		"more prose <!-- with an aside --> continues",
		"",
	}, "\n"))
	want := map[int]string{
		2: "",           // frontmatter
		4: "real prose", // content
		6: "",           // inside the fence
		8: "",           // whole-line comment
		9: "more prose  continues",
	}
	for line, expect := range want {
		if got := doc.Content(line); got != expect {
			t.Errorf("Content(%d) = %q, want %q", line, got, expect)
		}
	}
	if len(doc.Body) != len(doc.Lines) {
		t.Errorf("Body has %d lines, Lines has %d — indices must line up", len(doc.Body), len(doc.Lines))
	}
	if doc.Content(0) != "" || doc.Content(999) != "" {
		t.Error("out-of-range Content() did not return empty")
	}
}

func TestLineIsOneBasedAndSafe(t *testing.T) {
	doc := parse(t, "first\nsecond\n")
	if doc.Line(1) != "first" || doc.Line(2) != "second" {
		t.Errorf("Line() is not 1-based: %q %q", doc.Line(1), doc.Line(2))
	}
	if doc.Line(0) != "" || doc.Line(999) != "" {
		t.Error("out-of-range Line() did not return empty")
	}
}
