package validate

import (
	"os"
	"path/filepath"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/notes"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// Notes validates docs/notes.md — the log of small durable observations that used
// to be comments nobody outside one file ever read.
//
// Two of these checks are the whole reason the validator exists. A line that
// misses the grammar is invisible to every query, so a note somebody wrote by hand
// and got slightly wrong is a note the project has already lost — silently, which
// is the one way this file can fail without anybody noticing. And a `@path` that
// no longer resolves is the notes half of the codewiki citation check: a note
// about deleted code is worse than no note, because it is read as current.
//
// Everything else here is a spelling check on fields the CLI writes correctly by
// construction. They matter for the same reason: `scc notes add` is not the only
// way a line gets into this file, and the grammar has to hold either way.
func Notes(root string) (*finding.Set, error) {
	set := &finding.Set{}
	path := paths.Notes(root)
	if !isFile(path) {
		return set, nil
	}
	file := rel(root, path)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := notes.Parse(file, string(b))
	if err != nil {
		return nil, err
	}

	if f.Section == 0 {
		set.Addf(file, 1, "notes.no-log-section",
			"no `## %s` heading: notes have nowhere to go, and `scc notes add` will refuse", notes.Section)
	}
	for _, l := range f.Loose {
		set.Addf(file, l.Line, "notes.malformed",
			"not a note, so no query will ever return it: `- %s0001 YYYY-MM-DD #tag @path — text`", notes.IDPrefix)
	}
	for _, n := range f.Outside {
		set.Addf(file, n.Line, "notes.outside-log",
			"%s sits outside `## %s`, where `notes find` does not look", n.ID, notes.Section)
	}

	seen := map[string]int{}
	for _, n := range append(append([]notes.Note{}, f.Notes...), f.Outside...) {
		if prior, dup := seen[n.ID]; dup {
			set.Addf(file, n.Line, "notes.duplicate-id",
				"%s is already used on line %d; an id is how a note is cited, so it names one note", n.ID, prior)
		} else {
			seen[n.ID] = n.Line
		}
		if err := notes.CheckDate(n.Date); err != nil {
			set.Addf(file, n.Line, "notes.bad-date", "%s: %v", n.ID, err)
		}
		if len(n.Tags) == 0 {
			set.Addf(file, n.Line, "notes.untagged",
				"%s carries no #tag, so only a full-text search will ever surface it", n.ID)
		}
		for _, t := range n.Tags {
			if err := notes.CheckTag(t); err != nil {
				set.Addf(file, n.Line, "notes.bad-tag", "%s: %v", n.ID, err)
			}
		}
		for _, p := range n.Paths {
			clean, err := notes.CheckPath(p)
			if err != nil {
				set.Addf(file, n.Line, "notes.bad-path", "%s: %v", n.ID, err)
				continue
			}
			// The stale check, and the reason a note names a path at all: a note about
			// code that no longer exists is read as current, which is worse than the
			// comment it replaced — that one at least died with the file.
			if !exists(filepath.Join(root, filepath.FromSlash(clean))) {
				set.Addf(file, n.Line, "notes.stale-path",
					"%s is about %s, which no longer exists; repoint the note or remove it", n.ID, clean)
			}
		}
	}
	return set, nil
}
