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

// The block scc ships has to carry both markers, or nothing — not scc and not RTK
// itself — can ever replace it in place.
func TestShippedBlockIsMarkerDelimited(t *testing.T) {
	b := block(t)
	if !strings.HasPrefix(b, openPrefix) {
		t.Errorf("the block does not open with %q: %q", openPrefix, firstLine(b))
	}
	if !strings.Contains(b, closeTag) {
		t.Errorf("the block does not carry %q", closeTag)
	}
	if !strings.Contains(b, "Prefix EVERY command with `rtk`") {
		t.Error("the block lost the instruction it exists for")
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
	if !strings.HasSuffix(got, closeTag+"\n") {
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

// The block between the markers is RTK's: `rtk init` writes it and stamps its own
// version into the opening marker. scc inserts one only where there is none, so a
// block this build does not recognize — a newer one, or one the user edited —
// survives contact.
func TestSpliceLeavesAnExistingBlockAlone(t *testing.T) {
	doc := "# CLAUDE.md\n\n<!-- rtk-instructions v9 -->\n## RTK\nnewer text\n<!-- /rtk-instructions -->\n"
	got, action, err := Splice(doc, block(t), false)
	if err != nil {
		t.Fatalf("Splice: %v", err)
	}
	if action != Present {
		t.Errorf("action = %q, want %q", action, Present)
	}
	if got != doc {
		t.Errorf("an existing block was rewritten without force:\n%s", got)
	}
	if v := BlockVersion(doc); v != "v9" {
		t.Errorf("BlockVersion = %q, want %q", v, "v9")
	}
	if v := BlockVersion("# CLAUDE.md\n\nno block here.\n"); v != "" {
		t.Errorf("BlockVersion of a blockless document = %q, want empty", v)
	}
}

// force is the explicit "use the one this scc ships". It replaces the block where
// it stands, between the markers and nowhere else.
func TestSpliceReplacesAnOlderBlockInPlaceWithForce(t *testing.T) {
	doc := "# CLAUDE.md\n\nAbove.\n\n<!-- rtk-instructions v1 -->\n## RTK\nold text\n<!-- /rtk-instructions -->\n\nBelow.\n"
	got, action, err := Splice(doc, block(t), true)
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
	if strings.Count(got, openPrefix) != 1 {
		t.Errorf("the document carries %d opening markers, want 1", strings.Count(got, openPrefix))
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
	if !strings.HasPrefix(got, openPrefix) {
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
