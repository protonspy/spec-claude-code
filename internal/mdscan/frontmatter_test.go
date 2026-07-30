package mdscan

import "testing"

func TestParseFrontmatterReadsTheKickoffAnswers(t *testing.T) {
	fm, err := ParseFrontmatter("---\nautonomy: auto\nci: no-wait\n---\n\n# Heading\n")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if !fm.Present {
		t.Fatal("Present = false for a file that opens with a block")
	}
	if got, _ := fm.Get("autonomy"); got != "auto" {
		t.Errorf("autonomy = %q, want auto", got)
	}
	if got, _ := fm.Get("ci"); got != "no-wait" {
		t.Errorf("ci = %q, want no-wait", got)
	}
	if fm.Lines != 4 {
		t.Errorf("Lines = %d, want 4", fm.Lines)
	}
}

// Frontmatter is optional everywhere scc reads it: absent means "not recorded",
// never "wrong". A run that predates the convention is not a finding.
func TestNoFrontmatterIsNotAnError(t *testing.T) {
	fm, err := ParseFrontmatter("# Heading\n\nprose\n")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if fm.Present {
		t.Error("Present = true for a file with no block")
	}
	if _, ok := fm.Get("autonomy"); ok {
		t.Error("a key appeared out of nowhere")
	}
}

// A `---` that is not on the very first line is a horizontal rule, not frontmatter.
func TestDelimiterMustBeTheFirstLine(t *testing.T) {
	fm, err := ParseFrontmatter("\n---\nautonomy: auto\n---\n")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if fm.Present {
		t.Error("a mid-file --- was read as frontmatter")
	}
}

func TestQuotedValuesAndComments(t *testing.T) {
	fm, err := ParseFrontmatter("---\n# a comment\nname: \"user auth\"\nother: 'single'\n\n---\n")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if got, _ := fm.Get("name"); got != "user auth" {
		t.Errorf("name = %q, want unquoted", got)
	}
	if got, _ := fm.Get("other"); got != "single" {
		t.Errorf("other = %q, want unquoted", got)
	}
}

func TestInlineLists(t *testing.T) {
	fm, err := ParseFrontmatter("---\ntools: [Read, Grep, \"Bash\"]\nempty: []\n---\n")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	got := fm.Lists["tools"]
	if len(got) != 3 || got[0] != "Read" || got[2] != "Bash" {
		t.Errorf("tools = %v, want [Read Grep Bash]", got)
	}
	if l, ok := fm.Lists["empty"]; !ok || len(l) != 0 {
		t.Errorf("empty = (%v, %v), want an empty list", l, ok)
	}
}

// The parser refuses what it cannot read rather than guessing. A partial YAML
// parser that silently mis-reads a nested key produces a finding about a file that
// was fine — and one false positive costs more than a miss.
func TestParserRejectsWhatItCannotRead(t *testing.T) {
	cases := map[string]string{
		"two levels of nesting":   "---\nmetadata:\n  owner:\n    name: me\n---\n",
		"block list":              "---\ntools:\n  - Read\n---\n",
		"inconsistent indent":     "---\nmetadata:\n  a: 1\n    b: 2\n---\n",
		"indented under a scalar": "---\nname: x\n  nested: y\n---\n",
		"not a pair":              "---\njust a sentence\n---\n",
		"empty key":               "---\n: value\n---\n",
		"unterminated block":      "---\nautonomy: auto\n",
		"unterminated list":       "---\ntools: [Read, Grep\n---\n",
		"duplicate key":           "---\nci: wait\nci: no-wait\n---\n",
		"empty item in list":      "---\ntools: [Read, , Grep]\n---\n",
		"duplicate list key":      "---\ntools: [Read]\ntools: [Grep]\n---\n",
		"scalar after a list":     "---\ntools: [Read]\ntools: x\n---\n",
	}
	for name, in := range cases {
		if _, err := ParseFrontmatter(in); err == nil {
			t.Errorf("%s: parsed without error, want a refusal", name)
		}
	}
}

// One level of nesting is accepted because the Agent Skills spec's optional
// `metadata` field is a mapping, and refusing it would mean reporting a finding on a
// skill that is valid by the very standard the validator enforces.
func TestOneLevelOfNestingIsAccepted(t *testing.T) {
	fm, err := ParseFrontmatter("---\nname: pdf-processing\nmetadata:\n  author: example-org\n  version: \"1.0\"\nlicense: Apache-2.0\n---\n")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if got := fm.Maps["metadata"]["author"]; got != "example-org" {
		t.Errorf("metadata.author = %q, want example-org", got)
	}
	if got := fm.Maps["metadata"]["version"]; got != "1.0" {
		t.Errorf("metadata.version = %q, want 1.0 unquoted", got)
	}
	// The keys around the mapping still parse as scalars.
	if got, _ := fm.Get("license"); got != "Apache-2.0" {
		t.Errorf("license = %q, want Apache-2.0", got)
	}
	if _, isScalar := fm.Get("metadata"); isScalar {
		t.Error("the mapping head was also recorded as an empty scalar")
	}
}

// A key left blank is a key the user did not fill in — different from a key that
// heads a mapping, and it must not be mistaken for one.
func TestValuelessKeyIsAnEmptyScalar(t *testing.T) {
	fm, err := ParseFrontmatter("---\nautonomy:\nci: wait\n---\n")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	v, ok := fm.Get("autonomy")
	if !ok || v != "" {
		t.Errorf("autonomy = (%q, %v), want an empty scalar", v, ok)
	}
	if len(fm.Maps) != 0 {
		t.Errorf("a blank key became a mapping: %+v", fm.Maps)
	}
}

// A blank key as the last line before the delimiter still resolves.
func TestValuelessKeyAtTheEndOfTheBlock(t *testing.T) {
	fm, err := ParseFrontmatter("---\nci: wait\nautonomy:\n---\n")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if _, ok := fm.Get("autonomy"); !ok {
		t.Error("a trailing blank key was dropped")
	}
}

// CRLF input behaves identically to LF: the working tree's line endings are not
// part of the contract.
func TestCRLFParsesIdentically(t *testing.T) {
	lf, err := ParseFrontmatter("---\nautonomy: auto\n---\n")
	if err != nil {
		t.Fatalf("LF: %v", err)
	}
	crlf, err := ParseFrontmatter("---\r\nautonomy: auto\r\n---\r\n")
	if err != nil {
		t.Fatalf("CRLF: %v", err)
	}
	if lf.Lines != crlf.Lines || lf.Values["autonomy"] != crlf.Values["autonomy"] {
		t.Errorf("CRLF parsed differently: %+v vs %+v", crlf, lf)
	}
}

// A BOM-prefixed file (Windows Notepad) still has its frontmatter read.
func TestBOMDoesNotHideTheBlock(t *testing.T) {
	fm, err := ParseFrontmatter(string(rune(0xFEFF)) + "---\nci: wait\n---\n")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if !fm.Present {
		t.Error("a BOM hid the frontmatter block")
	}
}
