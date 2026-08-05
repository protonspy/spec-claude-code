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
