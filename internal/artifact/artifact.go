// Package artifact is the navigable model of one scc artifact — a plan, or one of
// a spec's three files.
//
// It exists for a cost that is paid on every request rather than once: an agent
// that has to answer "what is the next open task?" reads the whole file to find
// out, and a plan that decomposes into twenty-seven specs is tens of kilobytes of
// prose wrapped around thirteen checkboxes. The model turns that file into
// addressable pieces — sections, tasks, requirements, decomposition leaves — each
// with a stable name and a line range, so a caller can ask for exactly the piece it
// needs and read nothing else.
//
// The addresses are the point. A task is `1.2`, a requirement is `R1.2`, a section
// is its GitHub anchor slug, a leaf is `specs/<feature>/`. None of them is a line
// number, because a line number stops being true the moment anything above it
// moves — and an editor that addresses by line number is an editor that has to read
// the file first, which is the cost this package exists to remove.
//
// Parsing is layered on internal/mdscan, so fenced blocks and HTML comments are
// excluded here exactly as they are for the validators. That is not a convenience:
// scc's own templates carry their instructions in HTML comments, examples included,
// and a model that saw those examples as content would report tasks nobody wrote.
package artifact

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/mdscan"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// Kind is which of scc's artifacts a file is. It is derived from the path rather
// than from the content, because the layout is the contract: specs/<feature>/tasks.md
// is a task list by virtue of where it sits.
type Kind string

const (
	KindPlan         Kind = "plan"
	KindRequirements Kind = "requirements"
	KindDesign       Kind = "design"
	KindTasks        Kind = "tasks"
	KindDoc          Kind = "doc"
)

// Artifact is one parsed file.
//
// Lines is the raw text, kept because every edit in this package is a line splice
// against it and because a caller asking for a range wants what is actually in the
// file — comments and fences included — rather than the blanked-out Body the
// grammars are matched against.
type Artifact struct {
	Path  string `json:"path"` // workspace-relative, slash-separated
	Abs   string `json:"-"`    // absolute, for reading and writing
	Kind  Kind   `json:"kind"` // plan | requirements | design | tasks | doc
	Spec  string `json:"spec,omitempty"`
	Name  string `json:"name"`  // the plan's name, or the spec's feature name
	Title string `json:"title"` // the H1, or the name when there is none

	Frontmatter  map[string]string `json:"frontmatter,omitempty"`
	Sections     []Section         `json:"sections,omitempty"`
	Tasks        []Task            `json:"tasks,omitempty"`
	Requirements []Requirement     `json:"requirements,omitempty"`
	Leaves       []Leaf            `json:"leaves,omitempty"`

	Lines []string `json:"-"`
	Bytes int      `json:"bytes"`

	doc *mdscan.Document
}

// LineCount is the file's length, reported next to Bytes so a caller can see at a
// glance what an outline saved it from reading.
func (a *Artifact) LineCount() int { return len(a.Lines) }

// Doc exposes the underlying scan for callers that need the blanked-out Body — the
// searcher, which must not match inside a comment or a fence.
func (a *Artifact) Doc() *mdscan.Document { return a.doc }

// Section is one heading and the region it governs.
//
// Two ends, because "the section" means two different things depending on who is
// asking. End covers the subtree: everything under this heading including its
// children, which is what a reader asking to see `## Decomposition` wants. BodyEnd
// stops at the first child heading, which is where an appended line has to land so
// it does not fall into the last subsection.
type Section struct {
	Slug    string `json:"slug"`
	Level   int    `json:"level"`
	Title   string `json:"title"`
	Line    int    `json:"line"`
	End     int    `json:"end"`
	BodyEnd int    `json:"-"`
	Words   int    `json:"words"`
	Tasks   int    `json:"tasks,omitempty"`
	Done    int    `json:"done,omitempty"`
	Leaves  int    `json:"leaves,omitempty"`
}

// Leaf is a decomposition item: a plan's reference to a spec that carries the work.
// It has no checkbox on purpose — the state lives in that spec and is read from
// there — so its progress is a roll-up rather than a field.
type Leaf struct {
	Ref     string `json:"ref"` // "specs/<feature>/"
	Feature string `json:"feature"`
	Text    string `json:"text"`
	Line    int    `json:"line"`
	End     int    `json:"end"`
	Section string `json:"section,omitempty"`
}

// Load reads and parses one artifact. abs must already be an existing file.
func Load(root, abs string) (*Artifact, error) {
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	doc, err := mdscan.Parse(abs, string(b))
	if err != nil && doc == nil {
		return nil, err
	}
	a := &Artifact{
		Abs:   abs,
		Path:  relPath(root, abs),
		Lines: doc.Lines,
		Bytes: len(b),
		doc:   doc,
	}
	a.classify(root)
	a.readFrontmatter()
	a.buildSections()
	a.Tasks = parseTasks(doc)
	a.Requirements = parseRequirements(doc)
	a.Leaves = parseLeaves(doc)
	a.attribute()
	return a, nil
}

// classify decides the artifact's kind and name from where it sits. A file outside
// specs/ and plans/ is a doc: the knowledge base is Markdown too, and a caller that
// points this at docs/wiki/index.md should get a map of it rather than an error.
func (a *Artifact) classify(root string) {
	parts := strings.Split(a.Path, "/")
	base := parts[len(parts)-1]
	switch {
	case len(parts) == 2 && parts[0] == paths.PlansSeg:
		a.Kind = KindPlan
		a.Name = strings.TrimSuffix(base, ".md")
	case len(parts) == 3 && parts[0] == paths.SpecsSeg:
		a.Spec, a.Name = parts[1], parts[1]
		switch base {
		case paths.RequirementsSeg:
			a.Kind = KindRequirements
		case paths.DesignSeg:
			a.Kind = KindDesign
		case paths.TasksSeg:
			a.Kind = KindTasks
		default:
			a.Kind = KindDoc
		}
	default:
		a.Kind = KindDoc
		a.Name = strings.TrimSuffix(base, ".md")
	}
	a.Title = a.Name
	for _, h := range a.doc.Headings {
		if h.Level == 1 {
			a.Title = h.Text
			break
		}
	}
}

func (a *Artifact) readFrontmatter() {
	if len(a.doc.Frontmatter.Values) == 0 {
		return
	}
	a.Frontmatter = make(map[string]string, len(a.doc.Frontmatter.Values))
	for k, v := range a.doc.Frontmatter.Values {
		a.Frontmatter[k] = v
	}
}

// buildSections computes each heading's two ends in one backward pass, then counts
// the words in its body. A heading's subtree ends where the next heading of the same
// or a shallower level begins; its body ends where the next heading of any level
// begins.
func (a *Artifact) buildSections() {
	hs := a.doc.Headings
	last := len(a.Lines)
	a.Sections = make([]Section, 0, len(hs))
	for i, h := range hs {
		s := Section{Slug: h.Slug, Level: h.Level, Title: h.Text, Line: h.Line, End: last, BodyEnd: last}
		for j := i + 1; j < len(hs); j++ {
			if hs[j].Level <= h.Level {
				s.End = hs[j].Line - 1
				break
			}
		}
		if i+1 < len(hs) {
			s.BodyEnd = hs[i+1].Line - 1
		}
		for n := h.Line; n <= s.End && n <= len(a.doc.Body); n++ {
			s.Words += len(strings.Fields(a.doc.Body[n-1]))
		}
		a.Sections = append(a.Sections, s)
	}
}

// attribute assigns every task, requirement and leaf to the deepest heading that
// contains it, and rolls the counts back up into every ancestor. A count that
// stopped at the deepest heading would make `## Tasks` report zero in a plan whose
// tasks all sit under `### 1 · …`.
func (a *Artifact) attribute() {
	for i := range a.Tasks {
		a.Tasks[i].Section = a.sectionAt(a.Tasks[i].Line)
	}
	for i := range a.Requirements {
		a.Requirements[i].Section = a.sectionAt(a.Requirements[i].Line)
	}
	for i := range a.Leaves {
		a.Leaves[i].Section = a.sectionAt(a.Leaves[i].Line)
	}
	for i := range a.Sections {
		s := &a.Sections[i]
		for _, t := range a.Tasks {
			if t.Line >= s.Line && t.Line <= s.End {
				s.Tasks++
				if t.Checked {
					s.Done++
				}
			}
		}
		for _, l := range a.Leaves {
			if l.Line >= s.Line && l.Line <= s.End {
				s.Leaves++
			}
		}
	}
}

// sectionAt is the slug of the deepest heading whose subtree contains line.
func (a *Artifact) sectionAt(line int) string {
	slug := ""
	for _, s := range a.Sections {
		if s.Line <= line && line <= s.End {
			slug = s.Slug
		}
	}
	return slug
}

// Section finds one section by slug, or by a title the caller typed instead.
func (a *Artifact) Section(ref string) (Section, bool) {
	want := strings.TrimPrefix(ref, "#")
	for _, s := range a.Sections {
		if s.Slug == want {
			return s, true
		}
	}
	slug := mdscan.Slug(want)
	for _, s := range a.Sections {
		if s.Slug == slug || strings.EqualFold(s.Title, want) {
			return s, true
		}
	}
	return Section{}, false
}

// Task finds one task by its number.
func (a *Artifact) Task(number string) (Task, bool) {
	for _, t := range a.Tasks {
		if t.Number == number {
			return t, true
		}
	}
	return Task{}, false
}

// Requirement finds one requirement by its id.
func (a *Artifact) Requirement(id string) (Requirement, bool) {
	for _, r := range a.Requirements {
		if strings.EqualFold(r.ID, id) {
			return r, true
		}
	}
	return Requirement{}, false
}

// Leaf finds one decomposition leaf by feature name or by the reference as written.
func (a *Artifact) Leaf(ref string) (Leaf, bool) {
	feature := strings.Trim(strings.TrimPrefix(ref, paths.SpecsSeg+"/"), "/")
	for _, l := range a.Leaves {
		if l.Feature == feature {
			return l, true
		}
	}
	return Leaf{}, false
}

// Done reports how many tasks are checked.
func (a *Artifact) Done() (done, total int) {
	for _, t := range a.Tasks {
		if t.Checked {
			done++
		}
	}
	return done, len(a.Tasks)
}

// Text returns lines [from, to] inclusive, 1-based and clamped, as they appear in
// the file.
func (a *Artifact) Text(from, to int) string {
	if from < 1 {
		from = 1
	}
	if to > len(a.Lines) {
		to = len(a.Lines)
	}
	if from > to {
		return ""
	}
	return strings.Join(a.Lines[from-1:to], "\n")
}

// Resolve turns whatever the caller typed into the artifact files it names.
//
// It accepts, in order: a path that exists relative to the workspace root, a path
// that exists relative to the working directory, a bare plan name, and a bare
// feature name — which resolves to all three of that spec's files, because a spec
// is three files and asking to map one is asking to map the spec.
func Resolve(root, arg string) ([]string, error) {
	if arg == "" {
		return nil, fmt.Errorf("no artifact named")
	}
	clean := filepath.FromSlash(strings.TrimSuffix(arg, "/"))
	for _, cand := range []string{filepath.Join(root, clean), clean} {
		if abs, err := filepath.Abs(cand); err == nil {
			if info, err := os.Stat(abs); err == nil && info.Mode().IsRegular() {
				return []string{abs}, nil
			}
		}
	}
	if err := workspace.SafeName(arg, "artifact"); err == nil {
		if p := paths.Plan(root, arg); isFile(p) {
			return []string{p}, nil
		}
		if dir := paths.Spec(root, arg); isDir(dir) {
			var out []string
			for _, seg := range []string{paths.RequirementsSeg, paths.DesignSeg, paths.TasksSeg} {
				if p := filepath.Join(dir, seg); isFile(p) {
					out = append(out, p)
				}
			}
			if len(out) > 0 {
				return out, nil
			}
		}
	}
	// A spec directory named the long way: specs/<feature>/ or specs/<feature>.
	slashed := filepath.ToSlash(clean)
	if strings.HasPrefix(slashed, paths.SpecsSeg+"/") {
		return Resolve(root, strings.TrimPrefix(slashed, paths.SpecsSeg+"/"))
	}
	// And one file of a spec named without the specs/ prefix — `<feature>/tasks.md`,
	// which is what a caller types after `map` printed the feature name at them.
	if feature, seg, ok := strings.Cut(slashed, "/"); ok && !strings.Contains(seg, "/") {
		if workspace.SafeName(feature, "artifact") == nil && workspace.SafeName(seg, "artifact") == nil {
			if p := filepath.Join(paths.Spec(root, feature), seg); isFile(p) {
				return []string{p}, nil
			}
		}
	}
	return nil, fmt.Errorf("no artifact %q under %s/ or %s/", arg, paths.PlansSeg, paths.SpecsSeg)
}

// Scan loads every artifact in the workspace: each plan, then each spec's three
// files in phase order. It is the index every other read is a narrowing of.
func Scan(root string) ([]*Artifact, error) {
	var out []*Artifact
	for _, p := range planFiles(root) {
		a, err := Load(root, p)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	for _, feature := range specNames(root) {
		for _, seg := range []string{paths.RequirementsSeg, paths.DesignSeg, paths.TasksSeg} {
			p := filepath.Join(paths.Spec(root, feature), seg)
			if !isFile(p) {
				continue
			}
			a, err := Load(root, p)
			if err != nil {
				return nil, err
			}
			out = append(out, a)
		}
	}
	return out, nil
}

func planFiles(root string) []string {
	entries, err := os.ReadDir(paths.Plans(root))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		out = append(out, filepath.Join(paths.Plans(root), e.Name()))
	}
	sort.Strings(out)
	return out
}

func specNames(root string) []string {
	entries, err := os.ReadDir(paths.Specs(root))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func relPath(root, target string) string {
	return filepath.ToSlash(workspace.Relative(root, target))
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
