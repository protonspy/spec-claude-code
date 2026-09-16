package mdblock

import (
	"strings"
	"testing"
)

var (
	alpha = Markers{Open: "<!-- alpha", Close: "<!-- /alpha -->"}
	beta  = Markers{Open: "<!-- beta", Close: "<!-- /beta -->"}
)

const (
	alphaBlock = "<!-- alpha v1 -->\nAlpha says this.\n<!-- /alpha -->"
	betaBlock  = "<!-- beta v1 -->\nBeta says this.\n<!-- /beta -->"
)

// The reason this package exists as its own thing: one entry file now carries two
// blocks written by two integrations, and each has to be able to update its own
// without touching the other. Neither marker is a substring of the other, which is
// the property that makes that true — and the property nobody would notice breaking
// until a real workspace lost a block.
func TestTwoBlocksCoexistInOneDocument(t *testing.T) {
	doc := "# CLAUDE.md\n\nThe user's own prose.\n"

	doc, action, err := alpha.Splice(doc, alphaBlock, false)
	if err != nil || action != Added {
		t.Fatalf("alpha: action = %q, err = %v", action, err)
	}
	doc, action, err = beta.Splice(doc, betaBlock, false)
	if err != nil || action != Added {
		t.Fatalf("beta: action = %q, err = %v", action, err)
	}

	// Now rewrite alpha's, and beta's must come through untouched.
	next := "<!-- alpha v2 -->\nAlpha says something else.\n<!-- /alpha -->"
	doc, action, err = alpha.Splice(doc, next, false)
	if err != nil || action != Replaced {
		t.Fatalf("alpha rewrite: action = %q, err = %v", action, err)
	}
	if !strings.Contains(doc, betaBlock) {
		t.Errorf("rewriting alpha's block damaged beta's:\n%s", doc)
	}
	if !strings.Contains(doc, "The user's own prose.") {
		t.Errorf("the user's own prose did not survive:\n%s", doc)
	}
	if got := alpha.Version(doc); got != "v2" {
		t.Errorf("alpha version = %q, want v2", got)
	}
	if got := beta.Version(doc); got != "v1" {
		t.Errorf("beta version = %q, want v1 — it read alpha's marker", got)
	}
	if got := beta.Block(doc); got != betaBlock {
		t.Errorf("beta block = %q, want its own", got)
	}
}

// An idempotent splice is what lets a launch run the same write on every session
// without the entry file growing a block each time.
func TestSpliceIsIdempotent(t *testing.T) {
	once, _, err := alpha.Splice("# CLAUDE.md\n", alphaBlock, false)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	twice, action, err := alpha.Splice(once, alphaBlock, false)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	if action != Present || twice != once {
		t.Errorf("action = %q and the document changed; want %q and no change", action, Present)
	}
}

// A document with an opening marker and no close is malformed rather than
// blockless: appending would leave two openings and one close, which no tool could
// then update.
func TestSpliceRefusesAnUnclosedBlock(t *testing.T) {
	if _, _, err := alpha.Splice("<!-- alpha v1 -->\ndangling\n", alphaBlock, false); err == nil {
		t.Error("Splice accepted a document whose block never closes")
	}
	if _, _, err := alpha.Splice("<!-- /alpha -->\n", alphaBlock, false); err == nil {
		t.Error("Splice accepted a closing marker with no opening one")
	}
}

// A document that documents the markers is not a document carrying the block. This
// repository's own entry file writes both of RTK's markers in one sentence of prose,
// which made Splice read that sentence as the block and stand ready to replace it,
// and made Block report a block that was never spliced — so `scc launch` skipped the
// RTK wiring it believed was already done.
func TestMarkersInProseAreNotTheBlock(t *testing.T) {
	prose := "scc uses RTK's own markers: `<!-- alpha v1 -->` … `<!-- /alpha -->`, verified.\n"

	if got := alpha.Block(prose); got != "" {
		t.Errorf("Block found a block in prose: %q", got)
	}
	out, action, err := alpha.Splice(prose, alphaBlock, false)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	if action != Added {
		t.Errorf("action = %q, want %q", action, Added)
	}
	if !strings.Contains(out, prose) {
		t.Errorf("Splice overwrote the sentence:\n%s", out)
	}
	if !strings.Contains(out, alphaBlock) {
		t.Errorf("Splice did not append the block:\n%s", out)
	}
	if back, had := alpha.Remove(out); !had || !strings.Contains(back, prose) {
		t.Errorf("Remove took the prose with it (had = %v):\n%s", had, back)
	}
}

// The other half of the same defect, and the one that was actually reported: an entry
// file naming only the opening marker — in a code span, in a sentence about what the
// markers are — was read as malformed, so the block was never written at all.
func TestAnOpeningMarkerInACodeSpanIsNotMalformed(t *testing.T) {
	prose := "Its markers are scc's own — `<!-- alpha v1 -->` — which is deliberate.\n"

	out, action, err := alpha.Splice(prose, alphaBlock, false)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	if action != Added {
		t.Errorf("action = %q, want %q", action, Added)
	}
	if !strings.Contains(out, prose) {
		t.Errorf("Splice overwrote the sentence:\n%s", out)
	}
}

// A fenced example is the same case one construct over: a README showing the block it
// documents must not be mistaken for a workspace carrying it.
func TestAFencedExampleIsNotTheBlock(t *testing.T) {
	doc := "# Docs\n\n```\n" + alphaBlock + "\n```\n"

	if got := alpha.Version(doc); got != "" {
		t.Errorf("Version read a fenced example: %q", got)
	}
	out, action, err := alpha.Splice(doc, alphaBlock, false)
	if err != nil || action != Added {
		t.Fatalf("action = %q, err = %v", action, err)
	}
	if !strings.Contains(out, "```\n"+alphaBlock+"\n```") {
		t.Errorf("Splice rewrote the fenced example:\n%s", out)
	}
}

// Remove is the counterpart to Splice, and its promise is what survives: the file
// that carried the block alongside somebody else's content is still theirs
// afterwards, and the blank lines the block sat between go with it — a file left
// with a growing stack of empty lines records how many times the tool ran.
func TestRemoveTakesTheBlockAndItsBlankLines(t *testing.T) {
	head := "# CLAUDE.md\n\nThe user's own prose.\n"
	tail := "\nAnd more of their prose.\n"

	// Between two pieces of the user's own text.
	doc, _, err := alpha.Splice(head+tail, alphaBlock, false)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	out, found := alpha.Remove(doc)
	if !found {
		t.Fatal("Remove did not find the block it had just spliced")
	}
	if strings.Contains(out, "Alpha says this.") {
		t.Errorf("Remove left the block behind:\n%s", out)
	}
	if !strings.Contains(out, "The user's own prose.") || !strings.Contains(out, "And more of their prose.") {
		t.Errorf("Remove took the user's text with it:\n%s", out)
	}
	if strings.Contains(out, "\n\n\n") {
		t.Errorf("Remove left a stack of blank lines:\n%q", out)
	}

	// A document that is nothing but the block comes back empty rather than as a
	// file holding one newline.
	if out, found := alpha.Remove(alphaBlock + "\n"); !found || strings.TrimSpace(out) != "" {
		t.Errorf("removing the whole document left %q (found = %v)", out, found)
	}

	// A document with no block is returned unchanged, and says so.
	plain := "# CLAUDE.md\n\nNothing here.\n"
	if out, found := alpha.Remove(plain); found || out != plain {
		t.Errorf("Remove on a blockless document returned %q, %v", out, found)
	}
	// An opening marker with no close is malformed, not a block to take out.
	unclosed := "<!-- alpha v1 -->\nno close\n"
	if out, found := alpha.Remove(unclosed); found || out != unclosed {
		t.Errorf("Remove on an unclosed block returned %q, %v", out, found)
	}
	// And a marker written in prose is not a block either, on this path as on
	// every other.
	prose := "documented as `<!-- alpha v1 -->` … `<!-- /alpha -->` in a sentence.\n"
	if out, found := alpha.Remove(prose); found || out != prose {
		t.Errorf("Remove took a sentence out of the document: %q, %v", out, found)
	}
}

// CRLF is preserved on every path, because rewriting every line of somebody
// else's file is not a change they asked for.
func TestSpliceAndRemovePreserveCRLF(t *testing.T) {
	doc := "# CLAUDE.md\r\n\r\nTheir prose.\r\n"

	out, _, err := alpha.Splice(doc, alphaBlock, false)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Errorf("the splice introduced a bare LF into a CRLF document:\n%q", out)
	}
	back, found := alpha.Remove(out)
	if !found {
		t.Fatal("Remove did not find the block in a CRLF document")
	}
	if strings.Contains(strings.ReplaceAll(back, "\r\n", ""), "\n") {
		t.Errorf("the remove introduced a bare LF:\n%q", back)
	}
}
