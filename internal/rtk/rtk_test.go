package rtk

import (
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/assets"
)

func block(t *testing.T) string {
	t.Helper()
	b, err := assets.RTKBlock()
	if err != nil {
		t.Fatalf("RTKBlock: %v", err)
	}
	return b
}

// The block scc ships has to carry both markers, or nothing â€” not scc and not RTK
// itself â€” can ever replace it in place.
func TestShippedBlockIsMarkerDelimited(t *testing.T) {
	b := block(t)
	if !strings.HasPrefix(b, Markers.Open) {
		t.Errorf("the block does not open with %q: %q", Markers.Open, firstLine(b))
	}
	if !strings.Contains(b, Markers.Close) {
		t.Errorf("the block does not carry %q", Markers.Close)
	}
	if !strings.Contains(b, "Prefix EVERY command with `rtk`") {
		t.Error("the block lost the instruction it exists for")
	}
}

// scc shares RTK's markers rather than namespacing its own, and that is what
// makes `rtk init` and `scc rtk` converge on one copy: `rtk init` writes this
// exact pair into the project's entry file. A marker of scc's own would make each
// tool blind to the other's block and leave the file carrying both.
func TestTheMarkersAreRTKsOwn(t *testing.T) {
	if Markers.Open != "<!-- rtk-instructions" || Markers.Close != "<!-- /rtk-instructions -->" {
		t.Errorf("markers are %q / %q, which is not what `rtk init` writes", Markers.Open, Markers.Close)
	}
	// And the namespaced variant somebody will eventually propose must not match,
	// or scc would claim a block it does not own.
	if _, _, err := Splice("<!-- scc:rtk-instructions -->\nx\n<!-- /scc:rtk-instructions -->\n", block(t), false); err != nil {
		t.Errorf("a namespaced block confused the splice: %v", err)
	}
}

// Headroom writes the same guidance behind its own marker pair, into the same
// entry file. scc cannot address that block â€” it is Headroom's â€” but a file
// carrying both tells the agent the same thing twice in every request, so the one
// thing scc must not do is fail to notice.
func TestForeignBlockFindsHeadroomsCopy(t *testing.T) {
	doc := "# CLAUDE.md\n\n<!-- headroom:rtk-instructions -->\nuse rtk\n<!-- /headroom:rtk-instructions -->\n"
	f, ok := ForeignBlock(doc)
	if !ok {
		t.Fatal("Headroom's block went undetected")
	}
	if f.Tool != "Headroom" || f.Fix == "" {
		t.Errorf("foreign = %+v, want it named with a way to remove it", f)
	}

	// scc's own block is not foreign, and neither is a document with no block.
	if _, ok := ForeignBlock(block(t)); ok {
		t.Error("scc's own block was reported as another tool's")
	}
	if _, ok := ForeignBlock("# CLAUDE.md\n\nnothing here\n"); ok {
		t.Error("a blockless document reported a foreign block")
	}
}

// Neither marker is a substring of the other, which is why both tools' idempotency
// checks pass and both append. Splice must leave Headroom's block exactly where it
// is and add scc's alongside â€” anything else would be scc editing a document it
// does not own.
func TestSpliceLeavesAForeignBlockAlone(t *testing.T) {
	foreign := "<!-- headroom:rtk-instructions -->\nuse rtk\n<!-- /headroom:rtk-instructions -->"
	doc := "# CLAUDE.md\n\n" + foreign + "\n"

	got, action, err := Splice(doc, block(t), false)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	if action != Added {
		t.Errorf("action = %q, want %q", action, Added)
	}
	if !strings.Contains(got, foreign) {
		t.Error("Splice modified Headroom's block")
	}
	if !strings.Contains(got, Markers.Open) {
		t.Error("Splice did not add scc's own block")
	}
}

func TestSpliceAppendsToADocumentWithoutABlock(t *testing.T) {
	doc := "# CLAUDE.md\n\nSome rules.\n"
	got, action, err := Splice(doc, block(t), false)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	if action != Added {
		t.Errorf("action = %q, want %q", action, Added)
	}
	if !strings.HasPrefix(got, doc) {
		t.Error("splicing rewrote the document above the block")
	}
	if !strings.Contains(got, "## RTK") {
		t.Error("the block was not appended")
	}
	if !strings.HasSuffix(got, Markers.Close+"\n") {
		t.Errorf("the result does not end with the closing marker and one newline: %q", tail(got))
	}
}

// Re-running has to be free. `scc rtk` is the kind of command an agent runs at the
// top of a session, and a splice that appended every time would grow the entry file
// without bound.
func TestSpliceIsIdempotent(t *testing.T) {
	b := block(t)
	once, _, err := Splice("# CLAUDE.md\n\nSome rules.\n", b, false)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	twice, action, err := Splice(once, b, false)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	if action != Present {
		t.Errorf("action = %q, want %q", action, Present)
	}
	if twice != once {
		t.Error("a second splice changed the document")
	}
}

// The block scc ships wins by default, replacing whatever is between the markers
// â€” in place, between the markers and nowhere else.
func TestSpliceReplacesAnExistingBlockInPlace(t *testing.T) {
	doc := "# CLAUDE.md\n\nAbove.\n\n<!-- rtk-instructions v1 -->\n## RTK\nold text\n<!-- /rtk-instructions -->\n\nBelow.\n"
	got, action, err := Splice(doc, block(t), false)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	if action != Replaced {
		t.Errorf("action = %q, want %q", action, Replaced)
	}
	if strings.Contains(got, "old text") || strings.Contains(got, "v1") {
		t.Error("the old block survived")
	}
	if !strings.Contains(got, "# CLAUDE.md\n\nAbove.\n") || !strings.HasSuffix(got, "\nBelow.\n") {
		t.Errorf("the user's own prose did not survive: %q", got)
	}
	if strings.Count(got, Markers.Open) != 1 {
		t.Errorf("the document carries %d opening markers, want 1", strings.Count(got, Markers.Open))
	}
}

// keep is the standing "leave whatever is already there", for a block somebody
// curated on purpose â€” or one whose version is ahead of what this build ships.
func TestSpliceKeepsAnExistingBlockWhenAsked(t *testing.T) {
	doc := "# CLAUDE.md\n\n<!-- rtk-instructions v9 -->\n## RTK\nnewer text\n<!-- /rtk-instructions -->\n"
	got, action, err := Splice(doc, block(t), true)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	if action != Present {
		t.Errorf("action = %q, want %q", action, Present)
	}
	if got != doc {
		t.Errorf("keep rewrote the block:\n%s", got)
	}
	if v := BlockVersion(doc); v != "v9" {
		t.Errorf("BlockVersion = %q, want %q", v, "v9")
	}
	if v := BlockVersion("# CLAUDE.md\n\nno block here.\n"); v != "" {
		t.Errorf("BlockVersion of a blockless document = %q, want empty", v)
	}
}

// Block is what lets a caller measure what is there against what it would write,
// which is the whole basis for replacing it: `rtk init` and scc both stamp v2 and
// say the same thing, but one spends roughly five times the bytes.
func TestBlockExtractsWhatIsThere(t *testing.T) {
	inner := "<!-- rtk-instructions v1 -->\nold text\n<!-- /rtk-instructions -->"
	doc := "# CLAUDE.md\n\nAbove.\n\n" + inner + "\n\nBelow.\n"
	if got := Block(doc); got != inner {
		t.Errorf("Block = %q, want %q", got, inner)
	}
	if got := Block("# CLAUDE.md\n\nnothing here.\n"); got != "" {
		t.Errorf("Block of a blockless document = %q, want empty", got)
	}
	// A half block is not a block: the opening marker without its close.
	if got := Block("<!-- rtk-instructions v1 -->\ndangling\n"); got != "" {
		t.Errorf("Block of a half block = %q, want empty", got)
	}
}

// scc is a guest in this file. A CRLF entry file stays CRLF, so `scc rtk` shows up
// in the diff as the block it added and not as every line of the document.
func TestSplicePreservesCRLF(t *testing.T) {
	doc := "# CLAUDE.md\r\n\r\nSome rules.\r\n"
	got, _, err := Splice(doc, block(t), false)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Error("the spliced block carries bare LF inside a CRLF document")
	}
	// And it is recognized on the way back in, rather than appended a second time.
	again, action, err := Splice(got, block(t), false)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	if action != Present || again != got {
		t.Errorf("a CRLF document was not recognized as current: action = %q", action)
	}
}

func TestSpliceIntoAnEmptyDocument(t *testing.T) {
	got, action, err := Splice("", block(t), false)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	if action != Added {
		t.Errorf("action = %q, want %q", action, Added)
	}
	if !strings.HasPrefix(got, Markers.Open) {
		t.Errorf("an empty document got leading blank lines: %q", firstLine(got))
	}
}

// A half-block is malformed, not blockless. Appending a second one there would
// leave two openings and one close, which nothing could update afterwards.
func TestSpliceRefusesAHalfBlock(t *testing.T) {
	for name, doc := range map[string]string{
		"no close": "# CLAUDE.md\n\n<!-- rtk-instructions v2 -->\n## RTK\nsomething\n",
		"no open":  "# CLAUDE.md\n\n## RTK\nsomething\n<!-- /rtk-instructions -->\n",
	} {
		if _, _, err := Splice(doc, block(t), false); err == nil {
			t.Errorf("%s: Splice returned no error", name)
		}
	}
}

func TestInstallCmdNamesTheRepo(t *testing.T) {
	if !strings.Contains(InstallCmd(), Repo) {
		t.Errorf("InstallCmd() = %q, want it to name %s", InstallCmd(), Repo)
	}
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i]
	}
	return s
}

func tail(s string) string {
	if len(s) > 60 {
		return s[len(s)-60:]
	}
	return s
}
