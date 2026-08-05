// Package mdblock is the marker-delimited splice: keeping one block of generated
// guidance current inside a Markdown file that somebody else owns.
//
// It exists because two integrations need the same thing and neither owns it. RTK's
// block is addressed by RTK's own markers, so that `rtk init` and `scc rtk` converge
// on one copy; CodeGraph writes nothing into the entry file, so scc's block there is
// namespaced as scc's. Those are different decisions about *which* markers, and they
// belong in the integration packages. What is identical is everything after the
// markers are chosen — find, compare, replace, append, preserve the line endings —
// and a second copy of that would be a second set of edge cases to get right.
//
// Everything outside the markers is untouched, always. The document belongs to the
// user; scc is a guest in it.
package mdblock

import (
	"fmt"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/textutil"
)

// Markers is the HTML comment pair that delimits one tool's block.
//
// Open is matched by prefix rather than exactly, because the marker carries a
// version: a block stamped v1 or v9 is still the block, and replacing it with the
// one this build ships is exactly what an update means.
type Markers struct {
	// Open is the opening marker up to but not including its version, e.g.
	// "<!-- rtk-instructions".
	Open string
	// Close is the closing marker in full.
	Close string
}

// Action is what splicing the block did to a document.
type Action string

const (
	// Added: the document carried no block, so one was appended.
	Added Action = "added"
	// Present: the block in the document is already the one being written, byte for
	// byte, or the caller asked for an existing block to be kept.
	Present Action = "present"
	// Replaced: the document carried a different block, and this one replaced it.
	Replaced Action = "replaced"
)

// Splice returns doc with block present exactly once, and what it had to do to get
// there.
//
// block wins by default. keep is the standing "leave whatever is already there", for
// a document whose block somebody curated on purpose.
//
// The document's own line endings are preserved: an entry file checked out CRLF
// stays CRLF, because rewriting every line of someone else's file is not a change
// they asked for.
//
// A document carrying an opening marker with no closing one is malformed rather than
// blockless, and it is an error: appending a second block there would leave the file
// with two openings and one close, which no tool could then update.
func (m Markers) Splice(doc, block string, keep bool) (string, Action, error) {
	block = strings.TrimRight(textutil.NormalizeNewlines(block), "\n")
	eol := "\n"
	if strings.Contains(doc, "\r\n") {
		eol = "\r\n"
		block = strings.ReplaceAll(block, "\n", "\r\n")
	}

	start := strings.Index(doc, m.Open)
	if start < 0 {
		if strings.Contains(doc, m.Close) {
			return "", "", fmt.Errorf("found %s with no opening marker", m.Close)
		}
		trimmed := strings.TrimRight(doc, " \t\r\n")
		if trimmed == "" {
			return block + eol, Added, nil
		}
		return trimmed + eol + eol + block + eol, Added, nil
	}

	rest := doc[start:]
	end := strings.Index(rest, m.Close)
	if end < 0 {
		return "", "", fmt.Errorf("found %s with no closing %s", m.Open+" …", m.Close)
	}
	end += len(m.Close)
	if keep || rest[:end] == block {
		return doc, Present, nil
	}
	return doc[:start] + block + doc[start+end:], Replaced, nil
}

// Block returns the marker-delimited block in doc, markers included, or "" when doc
// carries none.
//
// It exists so a caller can measure what is already there against what it would have
// written — the entry file is preloaded into every request of the session, so the
// size of a block is the thing worth comparing when two of them say the same words.
func (m Markers) Block(doc string) string {
	start := strings.Index(doc, m.Open)
	if start < 0 {
		return ""
	}
	rest := doc[start:]
	end := strings.Index(rest, m.Close)
	if end < 0 {
		return ""
	}
	return rest[:end+len(m.Close)]
}

// Version reports what the opening marker in doc claims — "v2" for
// `<!-- rtk-instructions v2 -->` — or "" when doc carries no block.
//
// Advisory: it is printed so a run that left a block alone says which one it left,
// and never compared. Version ordering belongs to whoever defines the block.
func (m Markers) Version(doc string) string {
	start := strings.Index(doc, m.Open)
	if start < 0 {
		return ""
	}
	rest := doc[start+len(m.Open):]
	end := strings.Index(rest, "-->")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}
