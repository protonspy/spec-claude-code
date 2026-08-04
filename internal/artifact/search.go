package artifact

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

// Search over the artifacts, and why it is not a search engine.
//
// The obvious reach here is an inverted-index library. It is the wrong tool at this
// size: a plan measured at 56KB and a workspace of twenty-seven specs is under a
// megabyte of Markdown, which a linear pass reads faster than an index can be opened,
// and every real engine is a dependency — a CGO surface, or a second binary to
// install — against a go.mod that is stdlib-only and a build that cross-compiles to
// six platforms. The cost would be paid on every platform to save microseconds.
//
// What precision actually needs here is not a better index but a better unit. Lines
// are the wrong thing to retrieve: a line has no name, so a hit on one still leaves
// the caller reading around it to find out what it belongs to. So the unit indexed
// is the addressable one — a task, a requirement, a leaf, a paragraph — and a hit
// comes back as an address the caller can hand straight to `show`. Ranking is BM25,
// with the terms ANDed by default, because a reader asking for two words means both.
//
// If the corpus ever outgrows this, the seam is here: Search's signature takes
// artifacts and returns hits, and nothing outside this file knows how it found them.

// Hit is one match, already addressed.
type Hit struct {
	Path    string  `json:"path"`
	Ref     string  `json:"ref"`
	Kind    string  `json:"kind"`
	Label   string  `json:"label"`
	Line    int     `json:"line"`
	End     int     `json:"end"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

// SearchOpts narrows a search before it ranks anything.
type SearchOpts struct {
	Any    bool   // match any term rather than all of them
	Regex  bool   // treat the query as a regular expression over each unit
	Kind   string // restrict to one target kind: task, requirement, leaf, block, section
	Limit  int    // 0 means every hit
	Fields bool   // also match against the file's own path and title
}

// unit is one indexed thing: an addressable region and the text it holds.
type unit struct {
	art   *Artifact
	ref   string
	kind  TargetKind
	label string
	line  int
	end   int
	lead  string
	text  string
	terms map[string]int
	len   int
}

// Search ranks every addressable unit in arts against query.
func Search(arts []*Artifact, query string, opts SearchOpts) ([]Hit, error) {
	units := index(arts, opts.Kind)
	if opts.Regex {
		return searchRegex(units, query, opts)
	}
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil, nil
	}
	return rank(units, terms, opts), nil
}

// index flattens the artifacts into searchable units. A section is indexed by its
// title alone rather than by its whole subtree: its content is already indexed as
// the paragraphs and tasks inside it, and indexing both would make every hit inside
// a long section also a hit on the section.
func index(arts []*Artifact, kind string) []unit {
	var units []unit
	want := func(k TargetKind) bool { return kind == "" || kind == string(k) }
	for _, a := range arts {
		if want(TargetSection) {
			for _, s := range a.Sections {
				units = append(units, mk(a, s.Slug, TargetSection, s.Title, s.Line, s.End, s.Title, s.Title))
			}
		}
		if want(TargetTask) {
			for _, t := range a.Tasks {
				body := strings.Join([]string{t.Number, t.Methodology, t.Text, t.Detail,
					strings.Join(t.Requirements, " ")}, " ")
				units = append(units, mk(a, t.Number, TargetTask, t.Summary(72), t.Line, t.End, t.Text, body))
			}
		}
		if want(TargetRequirement) {
			for _, r := range a.Requirements {
				units = append(units, mk(a, r.ID, TargetRequirement, clip(r.Text, 72), r.Line, r.End,
					r.Text, r.ID+" "+r.Text+" "+r.Pattern))
			}
		}
		if want(TargetLeaf) {
			for _, l := range a.Leaves {
				body := l.Feature + " " + joinRaw(a, l.Line, l.End)
				units = append(units, mk(a, l.Ref, TargetLeaf, clip(l.Text, 72), l.Line, l.End, l.Text, body))
			}
		}
		if want(TargetBlock) {
			for _, b := range a.Blocks() {
				ref := b.Section + ":" + itoa(b.Index)
				units = append(units, mk(a, ref, TargetBlock, clip(b.Lead, 72), b.Line, b.End,
					b.Lead, joinRaw(a, b.Line, b.End)))
			}
		}
	}
	return units
}

func mk(a *Artifact, ref string, kind TargetKind, label string, line, end int, lead, text string) unit {
	u := unit{art: a, ref: ref, kind: kind, label: label, line: line, end: end, lead: lead, text: text}
	u.terms = map[string]int{}
	for _, t := range tokenize(text) {
		u.terms[t]++
		u.len++
	}
	// The lead counts twice. A paragraph's opening sentence is what it is about —
	// measured on a real plan, every note in `## Notes` opened with a bolded thesis —
	// so a term there is a stronger signal than the same term buried in the argument.
	for _, t := range tokenize(lead) {
		u.terms[t]++
		u.len++
	}
	return u
}

// rank scores the units that satisfy the term requirement, using BM25 with the
// usual constants.
func rank(units []unit, terms []string, opts SearchOpts) []Hit {
	df := map[string]int{}
	total := 0
	for _, u := range units {
		total += u.len
		for t := range u.terms {
			df[t]++
		}
	}
	if len(units) == 0 {
		return nil
	}
	avg := float64(total) / float64(len(units))
	const k1, b = 1.2, 0.75

	var hits []Hit
	for _, u := range units {
		matched := 0
		score := 0.0
		for _, t := range terms {
			f := float64(u.terms[t])
			if f == 0 {
				// A term absent as a whole token may still be a prefix of one, which is
				// what makes searching for "pixel" find "pixelart".
				if f = float64(prefixCount(u.terms, t)); f == 0 {
					continue
				}
				f *= 0.5
			}
			matched++
			idf := math.Log(1 + (float64(len(units))-float64(df[t])+0.5)/(float64(df[t])+0.5))
			score += idf * (f * (k1 + 1)) / (f + k1*(1-b+b*float64(u.len)/avg))
		}
		if matched == 0 || (!opts.Any && matched < len(terms)) {
			continue
		}
		hits = append(hits, u.hit(score, terms))
	}
	return finish(hits, opts.Limit)
}

func searchRegex(units []unit, pattern string, opts SearchOpts) ([]Hit, error) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, err
	}
	var hits []Hit
	for _, u := range units {
		if loc := re.FindStringIndex(u.text); loc != nil {
			h := u.hit(1, nil)
			h.Snippet = around(u.text, loc[0], loc[1])
			hits = append(hits, h)
		}
	}
	return finish(hits, opts.Limit), nil
}

func (u unit) hit(score float64, terms []string) Hit {
	h := Hit{
		Path: u.art.Path, Ref: u.ref, Kind: string(u.kind), Label: u.label,
		Line: u.line, End: u.end, Score: score,
	}
	h.Snippet = snippet(u.text, terms)
	return h
}

// finish sorts by score, then by position, so a run over an unchanged workspace
// returns the same order every time — a ranking that reshuffles ties is a diff
// nobody made.
func finish(hits []Hit, limit int) []Hit {
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].Path != hits[j].Path {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Line < hits[j].Line
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// snippet is the window around the first term that hit, so a caller can tell a real
// match from a coincidence without opening the file.
func snippet(text string, terms []string) string {
	lower := strings.ToLower(text)
	best := -1
	for _, t := range terms {
		if i := strings.Index(lower, t); i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	if best < 0 {
		return clip(text, 120)
	}
	return around(text, best, best+1)
}

func around(text string, from, to int) string {
	const pad = 60
	start := from - pad
	if start < 0 {
		start = 0
	}
	end := to + pad
	if end > len(text) {
		end = len(text)
	}
	out := strings.TrimSpace(text[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(text) {
		out += "…"
	}
	return strings.Join(strings.Fields(out), " ")
}

func prefixCount(terms map[string]int, prefix string) int {
	if len(prefix) < 3 {
		return 0
	}
	n := 0
	for t, c := range terms {
		if t != prefix && strings.HasPrefix(t, prefix) {
			n += c
		}
	}
	return n
}

var tokenSplit = regexp.MustCompile(`[^\p{L}\p{N}_.\-/]+`)

// tokenize lowercases and splits, keeping the punctuation that lives inside the
// names these artifacts are full of — `meta.json`, `--dry-run`, `specs/job-store` —
// and then also emitting the pieces, so a search for "json" still finds the first.
func tokenize(s string) []string {
	var out []string
	for _, raw := range tokenSplit.Split(strings.ToLower(s), -1) {
		t := strings.Trim(raw, ".-/_")
		if t == "" {
			continue
		}
		out = append(out, t)
		if strings.ContainsAny(t, "./-_") {
			for _, part := range strings.FieldsFunc(t, func(r rune) bool {
				return r == '.' || r == '/' || r == '-' || r == '_'
			}) {
				if len(part) > 1 {
					out = append(out, part)
				}
			}
		}
	}
	return out
}

func joinRaw(a *Artifact, from, to int) string {
	return strings.Join(strings.Fields(a.Text(from, to)), " ")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
