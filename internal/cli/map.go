package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/artifact"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/render"
)

// runMap dispatches `scc map <subcommand>`, the read half of navigating an artifact
// without loading it.
//
// The bare form takes a path rather than a subcommand — `scc map plans/x.md` — because
// the outline is what a caller wants nine times out of ten and making them type the
// verb for the common case is a cost paid on every invocation. A first argument that
// matches a known verb is that verb; anything else is a path.
func runMap(args []string) int {
	if len(args) == 0 {
		return runMapIndex(nil)
	}
	switch args[0] {
	case "index":
		return runMapIndex(args[1:])
	case "outline":
		return runMapOutline(args[1:])
	case "tasks":
		return runMapTasks(args[1:])
	case "show":
		return runMapShow(args[1:])
	case "blocks":
		return runMapBlocks(args[1:])
	case "find":
		return runMapFind(args[1:])
	case "trace":
		return runMapTrace(args[1:])
	case "help", "-h", "--help":
		mapUsage()
		return ExitOK
	default:
		if strings.HasPrefix(args[0], "-") {
			return runMapIndex(args)
		}
		return runMapOutline(args)
	}
}

// loadOne resolves a positional to exactly one artifact. Naming a spec resolves to
// its three files, which is right for an outline and ambiguous for everything else,
// so the commands that need one target say which file they want.
func loadOne(root, arg string) (*artifact.Artifact, bool) {
	paths_, err := artifact.Resolve(root, arg)
	if err != nil {
		render.Err(err.Error())
		return nil, false
	}
	if len(paths_) > 1 {
		render.Err(fmt.Sprintf("%q is a spec, which is %d files; name one of them", arg, len(paths_)))
		for _, p := range paths_ {
			render.Detail("  " + p)
		}
		return nil, false
	}
	a, err := artifact.Load(root, paths_[0])
	if err != nil {
		render.Err(err.Error())
		return nil, false
	}
	return a, true
}

func loadMany(root string, args []string) ([]*artifact.Artifact, bool) {
	if len(args) == 0 {
		all, err := artifact.Scan(root)
		if err != nil {
			render.Err(err.Error())
			return nil, false
		}
		return all, true
	}
	var out []*artifact.Artifact
	for _, arg := range args {
		files, err := artifact.Resolve(root, arg)
		if err != nil {
			render.Err(err.Error())
			return nil, false
		}
		for _, f := range files {
			a, err := artifact.Load(root, f)
			if err != nil {
				render.Err(err.Error())
				return nil, false
			}
			out = append(out, a)
		}
	}
	return out, true
}

// indexEntry is one artifact in the workspace index: enough to decide whether to
// open it, and nothing more.
type indexEntry struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Title  string `json:"title"`
	Tasks  int    `json:"tasks"`
	Done   int    `json:"done"`
	Leaves int    `json:"leaves"`
	Reqs   int    `json:"requirements"`
	Lines  int    `json:"lines"`
	Bytes  int    `json:"bytes"`
}

func runMapIndex(args []string) int {
	fs := flag.NewFlagSet("map index", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !noPositionals(rest, "map index") {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	arts, err := artifact.Scan(target)
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}

	entries := make([]indexEntry, 0, len(arts))
	totalBytes := 0
	for _, a := range arts {
		done, total := a.Done()
		entries = append(entries, indexEntry{
			Path: a.Path, Kind: string(a.Kind), Name: a.Name, Title: a.Title,
			Tasks: total, Done: done, Leaves: len(a.Leaves),
			Reqs: len(a.Requirements), Lines: a.LineCount(), Bytes: a.Bytes,
		})
		totalBytes += a.Bytes
	}
	if *jsonOut {
		return emitJSON(struct {
			Artifacts []indexEntry `json:"artifacts"`
			Count     int          `json:"count"`
			Bytes     int          `json:"bytes"`
		}{entries, len(entries), totalBytes})
	}
	if len(entries) == 0 {
		render.Info(fmt.Sprintf("no plans or specs yet — `%s plan new <name>`", prog()))
		return ExitOK
	}
	for _, e := range entries {
		render.Info(fmt.Sprintf("%-44s %s", e.Path, indexFacts(e)))
	}
	render.Info(fmt.Sprintf("%d artifacts · %s", len(entries), sizeOf(totalBytes)))
	return ExitOK
}

func indexFacts(e indexEntry) string {
	var parts []string
	if e.Tasks > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d tasks", e.Done, e.Tasks))
	}
	if e.Leaves > 0 {
		parts = append(parts, fmt.Sprintf("%d leaves", e.Leaves))
	}
	if e.Reqs > 0 {
		parts = append(parts, fmt.Sprintf("%d reqs", e.Reqs))
	}
	parts = append(parts, fmt.Sprintf("%dL", e.Lines))
	return strings.Join(parts, " · ")
}

func runMapOutline(args []string) int {
	fs := flag.NewFlagSet("map outline", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	depth := fs.Int("depth", 6, "deepest heading level to print")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if len(rest) == 0 {
		render.Err("map outline needs an artifact: a path, a plan name, or a feature name")
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	arts, ok := loadMany(target, rest)
	if !ok {
		return ExitError
	}
	if *jsonOut {
		return emitJSON(struct {
			Artifacts []*artifact.Artifact `json:"artifacts"`
		}{arts})
	}
	for i, a := range arts {
		if i > 0 {
			render.Info("")
		}
		printOutline(a, *depth)
	}
	return ExitOK
}

// printOutline is the whole point of the command: the shape of the file, with the
// prose replaced by its size. The closing line states what was skipped, because a
// summary that hides how much it summarized is a summary you cannot calibrate.
func printOutline(a *artifact.Artifact, depth int) {
	head := render.Bold(a.Title)
	if fm := frontmatterLine(a); fm != "" {
		head += "   " + fm
	}
	render.Info(head)
	render.Info(fmt.Sprintf("%s · %s · %d lines · %s", a.Path, a.Kind, a.LineCount(), sizeOf(a.Bytes)))

	blocks := map[string]int{}
	for _, b := range a.Blocks() {
		blocks[b.Section]++
	}
	for _, s := range a.Sections {
		if s.Level > depth || s.Level == 1 {
			continue
		}
		indent := strings.Repeat("  ", s.Level-1)
		var facts []string
		if s.Tasks > 0 {
			facts = append(facts, fmt.Sprintf("%d/%d tasks", s.Done, s.Tasks))
		}
		if s.Leaves > 0 {
			facts = append(facts, fmt.Sprintf("%d leaves", s.Leaves))
		}
		if n := blocks[s.Slug]; n > 0 {
			facts = append(facts, fmt.Sprintf("%d notes", n))
		}
		facts = append(facts, fmt.Sprintf("L%d-%d", s.Line, s.End))
		render.Info(fmt.Sprintf("%s%-32s %s", indent, s.Title, strings.Join(facts, " · ")))
	}
	if done, total := a.Done(); total > 0 {
		render.Info(fmt.Sprintf("tasks: %d/%d done · open: %s", done, total, openNumbers(a)))
	}
}

func openNumbers(a *artifact.Artifact) string {
	var open []string
	for _, t := range a.Tasks {
		if !t.Checked {
			open = append(open, t.Number)
		}
	}
	if len(open) == 0 {
		return "none"
	}
	if len(open) > 12 {
		return strings.Join(open[:12], " ") + fmt.Sprintf(" … (%d more)", len(open)-12)
	}
	return strings.Join(open, " ")
}

func frontmatterLine(a *artifact.Artifact) string {
	var parts []string
	for _, k := range []string{"autonomy", "ci", "pr", "worktree", "merge"} {
		if v, ok := a.Frontmatter[k]; ok {
			parts = append(parts, k+":"+v)
		}
	}
	return strings.Join(parts, " · ")
}

func runMapTasks(args []string) int {
	fs := flag.NewFlagSet("map tasks", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	open := fs.Bool("open", false, "only tasks that are not done")
	done := fs.Bool("done", false, "only tasks that are done")
	group := fs.String("group", "", "only tasks in this numbering `group` (1, or 1.2)")
	req := fs.String("req", "", "only tasks citing this `requirement` (R1.2)")
	method := fs.String("method", "", "only tasks annotated `Unit` or TDD")
	next := fs.Bool("next", false, "only the first open task — what a loop asks for, and it implies --open")
	width := fs.Int("width", 96, "clip each description to this many `runes`")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if *open && *done {
		render.Err("--open and --done ask for opposite things")
		return ExitError
	}
	// --next means the next task to work on, which is the first one nobody has done.
	// Without this it would mean "the first task", and answer a question nobody asked.
	if *next {
		if *done {
			render.Err("--next asks for the first open task; --done asks for finished ones")
			return ExitError
		}
		*open = true
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	arts, ok := loadMany(target, rest)
	if !ok {
		return ExitError
	}

	type row struct {
		Path string `json:"path"`
		artifact.Task
	}
	rows := []row{}
	for _, a := range arts {
		for _, t := range a.Tasks {
			switch {
			case *open && t.Checked, *done && !t.Checked:
				continue
			case *group != "" && t.Group() != *group && !strings.HasPrefix(t.Number, *group+"."):
				continue
			case *method != "" && !strings.EqualFold(t.Methodology, *method):
				continue
			}
			if *req != "" && !cites(t, *req) {
				continue
			}
			rows = append(rows, row{a.Path, t})
			if *next {
				break
			}
		}
		if *next && len(rows) > 0 {
			break
		}
	}

	if *jsonOut {
		return emitJSON(struct {
			Tasks []row `json:"tasks"`
			Count int   `json:"count"`
		}{rows, len(rows)})
	}
	if len(rows) == 0 {
		render.Info("no tasks match")
		return ExitOK
	}
	for _, r := range rows {
		box := "[ ]"
		if r.Checked {
			box = render.Green("[x]")
		}
		cite := ""
		if len(r.Requirements) > 0 {
			cite = " — " + strings.Join(r.Requirements, ", ")
		}
		render.Info(fmt.Sprintf("%s %-6s %-6s %s%s  %s", box, r.Number, r.Methodology,
			r.Summary(*width), cite, render.Cyan(fmt.Sprintf("%s:%d", r.Path, r.Line))))
	}
	return ExitOK
}

func cites(t artifact.Task, id string) bool {
	for _, r := range t.Requirements {
		if strings.EqualFold(r, id) {
			return true
		}
	}
	return false
}

func runMapShow(args []string) int {
	fs := flag.NewFlagSet("map show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	numbers := fs.Bool("numbers", false, "prefix each line with its line number")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if len(rest) < 2 {
		render.Err("map show needs an artifact and at least one address: `map show plans/x.md 1.2`")
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	a, ok := loadOne(target, rest[0])
	if !ok {
		return ExitError
	}

	type piece struct {
		artifact.Target
		Text string `json:"text"`
	}
	pieces := make([]piece, 0, len(rest)-1)
	for _, ref := range rest[1:] {
		t, err := a.Find(ref)
		if err != nil {
			render.Err(err.Error())
			return ExitError
		}
		pieces = append(pieces, piece{t, a.Text(t.Line, t.End)})
	}

	if *jsonOut {
		return emitJSON(struct {
			Path   string  `json:"path"`
			Pieces []piece `json:"pieces"`
		}{a.Path, pieces})
	}
	for i, p := range pieces {
		if i > 0 {
			fmt.Println()
		}
		render.Info(fmt.Sprintf("%s %s  %s:%d-%d", p.Kind, render.Bold(p.Ref), a.Path, p.Line, p.End))
		if *numbers {
			for n, line := range strings.Split(p.Text, "\n") {
				fmt.Printf("%5d  %s\n", p.Line+n, line)
			}
			continue
		}
		fmt.Println(p.Text)
	}
	return ExitOK
}

// runMapBlocks prints the paragraph index — the lead sentence of every paragraph in
// a section, with its address. It is the answer to a section that is half the file
// and carries no headings to navigate by.
func runMapBlocks(args []string) int {
	fs := flag.NewFlagSet("map blocks", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	section := fs.String("section", "", "only paragraphs under this `section` slug")
	width := fs.Int("width", 96, "clip each lead to this many `runes`")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if len(rest) == 0 {
		render.Err("map blocks needs an artifact")
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	a, ok := loadOne(target, rest[0])
	if !ok {
		return ExitError
	}
	if len(rest) > 1 && *section == "" {
		*section = rest[1]
	}

	blocks := []artifact.Block{}
	for _, b := range a.Blocks() {
		if *section != "" && b.Section != strings.TrimPrefix(*section, "#") {
			continue
		}
		blocks = append(blocks, b)
	}
	if *jsonOut {
		return emitJSON(struct {
			Path   string           `json:"path"`
			Blocks []artifact.Block `json:"blocks"`
			Count  int              `json:"count"`
		}{a.Path, blocks, len(blocks)})
	}
	if len(blocks) == 0 {
		render.Info("no paragraphs match")
		return ExitOK
	}
	for _, b := range blocks {
		ref := fmt.Sprintf("%s:%d", b.Section, b.Index)
		render.Info(fmt.Sprintf("%-14s %s  %s", render.Bold(ref),
			clipRunes(b.Lead, *width), render.Cyan(fmt.Sprintf("L%d-%d", b.Line, b.End))))
	}
	return ExitOK
}

func runMapFind(args []string) int {
	fs := flag.NewFlagSet("map find", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	in := fs.String("in", "", "restrict to one `artifact` (a path, plan name, or feature)")
	kind := fs.String("kind", "", "restrict to one `kind`: task, requirement, leaf, block, section")
	limit := fs.Int("limit", 10, "how many hits to `n`ame")
	any := fs.Bool("any", false, "match any term rather than all of them")
	rex := fs.Bool("regex", false, "treat the query as a regular expression")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if len(rest) == 0 {
		render.Err("map find needs something to look for")
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	var scope []string
	if *in != "" {
		scope = []string{*in}
	}
	arts, ok := loadMany(target, scope)
	if !ok {
		return ExitError
	}

	hits, err := artifact.Search(arts, strings.Join(rest, " "), artifact.SearchOpts{
		Any: *any, Regex: *rex, Kind: *kind, Limit: *limit,
	})
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}
	if *jsonOut {
		return emitJSON(struct {
			Query string         `json:"query"`
			Hits  []artifact.Hit `json:"hits"`
			Count int            `json:"count"`
		}{strings.Join(rest, " "), hits, len(hits)})
	}
	if len(hits) == 0 {
		render.Info("nothing matched")
		return ExitOK
	}
	for _, h := range hits {
		render.Info(fmt.Sprintf("%s %s  %s",
			render.Bold(h.Ref), render.Cyan(fmt.Sprintf("%s:%d-%d", h.Path, h.Line, h.End)), h.Kind))
		render.Detail("    " + clipRunes(h.Snippet, 150))
	}
	render.Info(fmt.Sprintf("%d hits — `%s map show <artifact> <ref>` for the whole of one", len(hits), prog()))
	return ExitOK
}

// runMapTrace answers "what else knows about this?" across files: a requirement and
// the tasks that cite it, or a spec and the plan leaf that carries it.
func runMapTrace(args []string) int {
	fs := flag.NewFlagSet("map trace", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	in := fs.String("in", "", "the `feature` whose numbering a requirement id belongs to")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if len(rest) != 1 {
		render.Err("map trace takes one reference: a requirement (R1.2) or a spec (specs/<feature>/)")
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	arts, err := artifact.Scan(target)
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}

	ref := rest[0]
	// A requirement id is scoped to its spec: R2.5 in one feature is a different
	// requirement from R2.5 in another, and this workspace has both. So the id can be
	// written with its scope — specs/<feature>/R2.5 — and an unscoped one that is
	// defined in more than one place is answered with the list rather than with every
	// spec's trace concatenated, which is the same mistake as reading all of them.
	scope := *in
	if i := strings.LastIndex(ref, "/"); i >= 0 && artifact.RequirementIDRe.MatchString(ref[i+1:]) {
		scope = strings.Trim(strings.TrimPrefix(ref[:i], paths.SpecsSeg+"/"), "/")
		ref = ref[i+1:]
	}

	type site struct {
		Path  string `json:"path"`
		Role  string `json:"role"`
		Ref   string `json:"ref"`
		Line  int    `json:"line"`
		Label string `json:"label"`
	}
	sites := []site{}

	if artifact.RequirementIDRe.MatchString(ref) && scope == "" {
		var defined []string
		for _, a := range arts {
			if _, ok := a.Requirement(ref); ok {
				defined = append(defined, a.Spec)
			}
		}
		if len(defined) > 1 {
			if *jsonOut {
				return emitJSON(struct {
					Ref     string   `json:"ref"`
					Scoped  bool     `json:"scoped"`
					Defined []string `json:"defined_in"`
				}{ref, false, defined})
			}
			render.Warn(fmt.Sprintf("%s is defined in %d specs — a requirement id is scoped to its own", ref, len(defined)))
			for _, f := range defined {
				render.Info(fmt.Sprintf("  %s/%s/%s", paths.SpecsSeg, f, ref))
			}
			render.Detail(fmt.Sprintf("  name one: `%s map trace %s/<feature>/%s`", prog(), paths.SpecsSeg, ref))
			return ExitOK
		}
		if len(defined) == 1 {
			scope = defined[0]
		}
	}
	if scope != "" {
		var narrowed []*artifact.Artifact
		for _, a := range arts {
			if a.Spec == scope || a.Kind == artifact.KindPlan {
				narrowed = append(narrowed, a)
			}
		}
		arts = narrowed
	}

	if strings.HasPrefix(ref, paths.SpecsSeg+"/") || !artifact.RequirementIDRe.MatchString(ref) {
		feature := strings.Trim(strings.TrimPrefix(ref, paths.SpecsSeg+"/"), "/")
		for _, a := range arts {
			for _, l := range a.Leaves {
				if l.Feature == feature {
					sites = append(sites, site{a.Path, "decomposed-by", l.Ref, l.Line, clipRunes(l.Text, 88)})
				}
			}
			if a.Spec == feature {
				done, total := a.Done()
				var label string
				switch {
				case total > 0:
					label = fmt.Sprintf("%d/%d tasks done", done, total)
				case len(a.Requirements) > 0:
					label = fmt.Sprintf("%d requirements", len(a.Requirements))
				default:
					label = fmt.Sprintf("%d sections · %d lines", len(a.Sections), a.LineCount())
				}
				sites = append(sites, site{a.Path, string(a.Kind), feature, 1, label})
			}
		}
	} else {
		for _, a := range arts {
			if r, ok := a.Requirement(ref); ok {
				sites = append(sites, site{a.Path, "defined", r.ID, r.Line, clipRunes(r.Text, 88)})
			}
			for _, t := range a.Tasks {
				if cites(t, ref) {
					sites = append(sites, site{a.Path, "implemented-by", t.Number, t.Line, t.Summary(88)})
				}
			}
			if a.Kind == artifact.KindDesign {
				for i, line := range a.Doc().Body {
					for _, id := range artifact.RequirementIDRe.FindAllString(line, -1) {
						if strings.EqualFold(id, ref) {
							sites = append(sites, site{a.Path, "designed", id, i + 1, clipRunes(line, 88)})
						}
					}
				}
			}
		}
	}

	if *jsonOut {
		return emitJSON(struct {
			Ref   string `json:"ref"`
			Sites []site `json:"sites"`
			Count int    `json:"count"`
		}{ref, sites, len(sites)})
	}
	if len(sites) == 0 {
		render.Info(fmt.Sprintf("nothing in this workspace mentions %s", ref))
		return ExitOK
	}
	for _, s := range sites {
		render.Info(fmt.Sprintf("%-16s %-10s %s  %s", render.Bold(s.Ref), s.Role,
			render.Cyan(fmt.Sprintf("%s:%d", s.Path, s.Line)), s.Label))
	}
	return ExitOK
}

func clipRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return strings.TrimRight(string(r[:n-1]), " ") + "…"
}

func sizeOf(b int) string {
	if b < 1024 {
		return fmt.Sprintf("%dB", b)
	}
	return fmt.Sprintf("%.1fKB", float64(b)/1024)
}

func mapUsage() {
	fmt.Fprintf(os.Stderr, `Usage:
  %s map                                 every plan and spec, one line each
  %s map <artifact>                      the shape of one file: sections, counts, open tasks
  %s map tasks [<artifact>…] [filters]   --open --done --next --group N --req R1.2 --method TDD
  %s map show <artifact> <address>…      exactly that piece, and nothing else
  %s map blocks <artifact> [<section>]   the lead sentence of every paragraph, with its address
  %s map find <query> [--in <artifact>]  ranked search over addressable units
  %s map trace <R1.2|specs/<feature>/>   everything in the workspace that mentions it

An <artifact> is a path (plans/x.md), a plan name, or a feature name.

Addresses, none of which is a line number — which is why one survives an edit above it:

  1.2            a task, by its number
  R1.2           a requirement, by its id
  specs/foo/     a decomposition leaf
  #notes         a section, by anchor slug (or by its title as written)
  notes:7        the 7th paragraph of that section
  L120-160       an explicit line range, the escape hatch

Use "%s patch" to change what "%s map" found, without reading the file first.
`, prog(), prog(), prog(), prog(), prog(), prog(), prog(), prog(), prog())
}
