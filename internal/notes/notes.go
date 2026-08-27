// Package notes is docs/notes.md — the project's note log, and the grammar that
// makes it answerable without being read.
//
// A note is the small durable observation that has nowhere else to live: the
// gotcha, the why-not, the "careful, this looks wrong and is not". Before this
// file existed that text went into a comment beside the code, where exactly one
// reader ever found it — the one already looking at that line — and where it was
// invisible to anybody asking "what do we know about this area?".
//
// The whole design follows from one requirement: a reader must be able to ask the
// log a question without loading it. So a note is ONE LINE, self-contained, with
// its index fields up front:
//
//   - n-0042 2026-08-27 #gotcha @internal/cli/launch.go — wrap writes MCP config
//
// One line is what makes grep exact rather than approximate: a hit is a whole
// note, never a fragment of one, so `grep ' #gotcha ' docs/notes.md` and
// `scc notes find --tag gotcha` return the same thing and neither has to reason
// about where a record ends. It is also the boundary against this file becoming
// the place prose goes to hide — the failure mode that ended `## Notes` in the v1
// plan format, where nothing forbade growth and half the file became one section.
// A thought that needs a second line is not a note; it is a wiki page, an ADR, or
// a task, and docs/ already has all three.
//
// Nothing here reads a note's meaning. This package parses, formats, filters, and
// splices; turning a fact about a line into a finding is internal/validate's job,
// the same seam internal/artifact keeps with the plan grammar.
package notes

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/protonspy/spec-claude-code/internal/mdscan"
	"github.com/protonspy/spec-claude-code/internal/textutil"
)

// Section is the heading notes live under. Everything above it is guidance the
// user owns; everything below it is the log.
//
// A heading rather than an HTML-comment marker because the region has to be
// visible to the person reading the file, and because mdscan already reports
// headings — a marker would be a second parser for a job the first one does.
const Section = "Log"

// IDPrefix and idWidth spell a note's identity: n-0001, zero-padded so the log
// sorts the way it reads, and free to grow past four digits rather than wrapping.
//
// An id exists so a note can be cited — from a spec, from a commit message, from
// the one place a docstring legitimately mentions one ("see n-0042"). Without it
// the only way to point at a note is to quote it, and a quotation goes stale
// silently.
const (
	IDPrefix = "n-"
	idWidth  = 4
)

// The grammar. Field order is index-first and deliberate: id, date, then the tags
// and paths a query filters on, and only then the prose. A line whose fixed
// fields come first can be matched by a reader that stops caring at the em dash.
var (
	noteRe = regexp.MustCompile(`^-[ \t]+(` + IDPrefix + `[0-9]{4,})[ \t]+([0-9]{4}-[0-9]{2}-[0-9]{2})[ \t]+((?:[#@][^\s]+[ \t]+)*)—[ \t]+(\S.*)$`)
	itemRe = regexp.MustCompile(`^-[ \t]+\S`)
	tagRe  = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	idRe   = regexp.MustCompile(`^` + IDPrefix + `([0-9]{4,})$`)
	tombRe = regexp.MustCompile(`^<!--\s+(` + IDPrefix + `[0-9]{4,}) removed\s+-->$`)
)

// DateLayout is the only date a note carries: ISO, so it sorts as a string and a
// `--since` filter is a comparison rather than a parse.
const DateLayout = "2006-01-02"

// Note is one line of the log.
type Note struct {
	// Line is 1-based in the file it was parsed from, or 0 for a note being built.
	Line int    `json:"line,omitempty"`
	ID   string `json:"id"`
	Num  int    `json:"-"`
	Date string `json:"date"`
	// Tags are the index, without their leading '#'. At least one, because a note
	// nobody can filter for is one only full-text search will ever surface.
	Tags []string `json:"tags"`
	// Paths are what the note is about, without their leading '@': repo-relative,
	// slash-separated. This is the field that replaces the comment in the file —
	// it is how a note stays attached to code without living inside it.
	Paths []string `json:"paths,omitempty"`
	Text  string   `json:"text"`
}

// Loose is a line in the log region that is not a note. It is reported rather
// than ignored: a hand-written note that missed the grammar is invisible to every
// query, which is the one failure this file cannot tolerate quietly.
type Loose struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// File is a parsed docs/notes.md.
type File struct {
	Path    string
	Lines   []string
	Doc     *mdscan.Document
	Notes   []Note
	Loose   []Loose
	Outside []Note // well-formed notes sitting outside the log region

	// Retired is the numbers `Remove` has spent, by id. A removed note leaves an
	// HTML-comment tombstone where it stood, which mdscan blanks — so it costs
	// nothing to every reader of the log, and Next can still refuse to hand its
	// number out twice.
	Retired map[string]int

	// Section is the 1-based line of the `## Log` heading, or 0 when the file has
	// none — which is a finding, and the reason Append refuses.
	Section int
}

// Parse reads a notes file. Content is normalized first, so a CRLF checkout and a
// LF one produce identical notes and identical line numbers.
func Parse(p, content string) (*File, error) {
	doc, err := mdscan.Parse(p, textutil.NormalizeNewlines(content))
	if err != nil {
		return nil, err
	}
	f := &File{Path: p, Lines: doc.Lines, Doc: doc, Retired: map[string]int{}}
	// Tombstones are read from the raw lines rather than from Body, because Body is
	// exactly where an HTML comment has been blanked out.
	for i, raw := range doc.Lines {
		if m := tombRe.FindStringSubmatch(strings.TrimSpace(raw)); m != nil {
			f.Retired[m[1]] = i + 1
		}
	}

	start, end := 0, len(doc.Lines)+1
	for _, h := range doc.Headings {
		if start == 0 {
			if h.Level >= 2 && strings.EqualFold(h.Text, Section) {
				start, f.Section = h.Line, h.Line
			}
			continue
		}
		if h.Level <= 2 {
			end = h.Line
			break
		}
	}

	for i, body := range doc.Body {
		line := i + 1
		if strings.TrimSpace(body) == "" {
			continue
		}
		inRegion := start > 0 && line > start && line < end
		if m := noteRe.FindStringSubmatch(body); m != nil {
			n := parseNote(line, m)
			if inRegion {
				f.Notes = append(f.Notes, n)
			} else {
				f.Outside = append(f.Outside, n)
			}
			continue
		}
		// Only inside the region, and only for list items: the guidance above the
		// heading is prose the user owns, and a check that reported on it would
		// fire on the file scc itself seeds.
		if inRegion && itemRe.MatchString(body) {
			f.Loose = append(f.Loose, Loose{Line: line, Text: strings.TrimSpace(body)})
		}
	}
	return f, nil
}

func parseNote(line int, m []string) Note {
	n := Note{Line: line, ID: m[1], Date: m[2], Text: strings.TrimSpace(m[4])}
	if d := idRe.FindStringSubmatch(n.ID); d != nil {
		n.Num, _ = strconv.Atoi(d[1])
	}
	for _, tok := range strings.Fields(m[3]) {
		switch tok[0] {
		case '#':
			n.Tags = append(n.Tags, tok[1:])
		case '@':
			n.Paths = append(n.Paths, tok[1:])
		}
	}
	return n
}

// Format renders a note as the line the file holds. It is the only writer, so
// every note scc adds is one a query can find.
func (n Note) Format() string {
	var b strings.Builder
	b.WriteString("- " + n.ID + " " + n.Date)
	for _, t := range n.Tags {
		b.WriteString(" #" + t)
	}
	for _, p := range n.Paths {
		b.WriteString(" @" + p)
	}
	b.WriteString(" — " + n.Text)
	return b.String()
}

// String is Format: what a reader greps for and what a reader is shown must not
// be two formats to learn.
func (n Note) String() string { return n.Format() }

// ID formats a note number.
func ID(num int) string { return fmt.Sprintf("%s%0*d", IDPrefix, idWidth, num) }

// Next is the number a new note takes: the high-water mark plus one, counting
// notes wherever they sit. Numbers are never reused — a citation to n-0042 must
// not silently come to mean a different note — so a deleted note's number stays
// spent, which falls out of using the maximum rather than the count.
func (f *File) Next() int {
	high := 0
	for _, set := range [][]Note{f.Notes, f.Outside} {
		for _, n := range set {
			if n.Num > high {
				high = n.Num
			}
		}
	}
	for id := range f.Retired {
		if m := idRe.FindStringSubmatch(id); m != nil {
			if num, err := strconv.Atoi(m[1]); err == nil && num > high {
				high = num
			}
		}
	}
	return high + 1
}

// Get returns the note with this id.
func (f *File) Get(id string) (Note, bool) {
	for _, set := range [][]Note{f.Notes, f.Outside} {
		for _, n := range set {
			if n.ID == id {
				return n, true
			}
		}
	}
	return Note{}, false
}

// Append splices a note in at the end of the log region and returns the new
// content.
//
// At the end rather than the top: the log is append-only and reads oldest first,
// so a diff of this file is one added line at a known place, and two sessions
// each adding a note conflict on nothing.
func (f *File) Append(n Note) (string, error) {
	if f.Section == 0 {
		return "", fmt.Errorf("%s has no `## %s` heading — nowhere to put a note", f.Path, Section)
	}
	end := len(f.Lines)
	for _, h := range f.Doc.Headings {
		if h.Level <= 2 && h.Line > f.Section {
			end = h.Line - 1
			break
		}
	}
	// Back up over the blank lines that separate the region from what follows, so
	// the note lands against the last note rather than after a gap.
	for end > f.Section && strings.TrimSpace(f.Lines[end-1]) == "" {
		end--
	}
	out := make([]string, 0, len(f.Lines)+1)
	out = append(out, f.Lines[:end]...)
	out = append(out, n.Format())
	out = append(out, f.Lines[end:]...)
	return strings.Join(out, "\n"), nil
}

// Remove takes a note out of the log, leaving a tombstone where it stood.
//
// The note's text goes — a wrong note is worse than none, and unlike a struck-out
// plan task it is not a commitment whose absence needs explaining. The number
// stays: an id is what a commit message or a spec cites, and handing it out again
// would make an old citation point at a new note, silently. The tombstone is an
// HTML comment, so it is invisible to a rendered read, to a grep for a tag, and to
// every parser here except Next.
func (f *File) Remove(id string) (string, Note, error) {
	n, ok := f.Get(id)
	if !ok {
		if line, was := f.Retired[id]; was {
			return "", Note{}, fmt.Errorf("%s was already removed (line %d)", id, line)
		}
		return "", Note{}, fmt.Errorf("no note %q in %s", id, f.Path)
	}
	out := make([]string, 0, len(f.Lines))
	out = append(out, f.Lines[:n.Line-1]...)
	out = append(out, Tombstone(n.ID))
	out = append(out, f.Lines[n.Line:]...)
	return strings.Join(out, "\n"), n, nil
}

// Tombstone renders the marker a removed note leaves behind.
func Tombstone(id string) string { return "<!-- " + id + " removed -->" }

// Query is a filter over the log. Every field is an AND, and a repeated field is
// an OR within itself: `--tag a --tag b --path p` is "(a or b) and p", which is
// what a reader narrowing a search actually means.
type Query struct {
	Tags  []string
	Paths []string
	// Terms are matched case-insensitively against the whole rendered line, so a
	// term can be a word in the prose, a tag, or a path fragment without the
	// caller having to say which.
	Terms []string
	// Since is an ISO date; a note older than it is dropped. Comparison is on the
	// string, which is exactly right for ISO and needs no clock.
	Since string
}

// Match returns the notes this query selects, in file order.
func (f *File) Match(q Query) []Note {
	out := []Note{}
	for _, n := range f.Notes {
		if n.matches(q) {
			out = append(out, n)
		}
	}
	return out
}

func (n Note) matches(q Query) bool {
	if q.Since != "" && n.Date < q.Since {
		return false
	}
	if len(q.Tags) > 0 && !anyOf(n.Tags, q.Tags, false) {
		return false
	}
	// A path filter matches the path itself and anything under it, so asking about
	// a directory answers for the files in it — the question a reader means when
	// they name a package rather than a file.
	if len(q.Paths) > 0 && !anyOf(n.Paths, q.Paths, true) {
		return false
	}
	line := strings.ToLower(n.Format())
	for _, t := range q.Terms {
		if !strings.Contains(line, strings.ToLower(t)) {
			return false
		}
	}
	return true
}

func anyOf(have, want []string, prefix bool) bool {
	for _, w := range want {
		for _, h := range have {
			if h == w || (prefix && strings.HasPrefix(h, w+"/")) {
				return true
			}
		}
	}
	return false
}

// Count is one index entry — a tag or a path — and how many notes carry it.
type Count struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Tags is the tag index, most used first and alphabetical within a count.
//
// It exists to be read before a tag is coined. An open vocabulary drifts the way
// domain language does — three tags for one concern inside a week — and the cheap
// defense is not a closed list scc would have to guess at, but making the existing
// tags one command away at the moment somebody is about to invent a fourth.
func (f *File) Tags() []Count {
	return tally(f.Notes, func(n Note) []string { return n.Tags })
}

// Paths is every path the log mentions, most noted first. Same purpose as Tags:
// it answers "what does this project already know things about".
func (f *File) Paths() []Count {
	return tally(f.Notes, func(n Note) []string { return n.Paths })
}

func tally(notes []Note, of func(Note) []string) []Count {
	seen := map[string]int{}
	for _, n := range notes {
		for _, v := range of(n) {
			seen[v]++
		}
	}
	out := make([]Count, 0, len(seen))
	for v, c := range seen {
		out = append(out, Count{Name: v, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Today is the date a new note takes when the caller names none.
func Today() string { return time.Now().Format(DateLayout) }

// CheckTag rejects a tag the grammar cannot round-trip. Kebab-case for the same
// reason every other name in an scc workspace is: one spelling per concept, so
// `--tag Gotcha` and `--tag gotcha` cannot become two entries in the index.
func CheckTag(t string) error {
	if !tagRe.MatchString(t) {
		return fmt.Errorf("tag %q is not kebab-case (a-z, 0-9, single hyphens)", t)
	}
	return nil
}

// CheckDate rejects anything the grammar would not parse back.
func CheckDate(d string) error {
	if _, err := time.Parse(DateLayout, d); err != nil {
		return fmt.Errorf("date %q is not YYYY-MM-DD", d)
	}
	return nil
}

// CheckPath normalizes a scope and rejects one the log could not hold.
//
// It is deliberately strict about the spelling and silent about what exists: a
// note may legitimately name a file this branch has not created yet, and refusing
// that would make the log unusable at the moment it is most worth writing. Whether
// the path still resolves is the validator's question, asked later and repeatedly.
func CheckPath(p string) (string, error) {
	clean := strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if clean == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.ContainsAny(clean, " \t") {
		return "", fmt.Errorf("path %q contains whitespace; a note's fields are space-separated", p)
	}
	if strings.HasPrefix(clean, "/") || (len(clean) > 1 && clean[1] == ':') {
		return "", fmt.Errorf("path %q is absolute; a note cites a repo-relative path", p)
	}
	clean = path.Clean(clean)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path %q escapes the workspace", p)
	}
	return clean, nil
}

// CheckText rejects a note the file could not hold as one line, which is the whole
// contract: a hit has to be a whole note.
func CheckText(s string) (string, error) {
	t := strings.TrimSpace(textutil.NormalizeNewlines(s))
	if t == "" {
		return "", fmt.Errorf("a note needs text")
	}
	if strings.Contains(t, "\n") {
		return "", fmt.Errorf("a note is one line; this is %d — if it needs more, it is a wiki page or an ADR",
			strings.Count(t, "\n")+1)
	}
	return t, nil
}
