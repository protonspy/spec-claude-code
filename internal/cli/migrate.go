package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/artifact"
	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/validate"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// planSectionSlugs and requiredPlanSections read the contract from the validator that
// enforces it. Restating it here would let migration produce a file the validator it
// was written for then rejects.
func planSectionSlugs() []string {
	var out []string
	for _, s := range validate.PlanSections() {
		out = append(out, s.Slug)
	}
	return out
}

func requiredPlanSections() []string {
	var out []string
	for _, s := range validate.PlanSections() {
		if s.Required {
			out = append(out, s.Title)
		}
	}
	return out
}

func planFindingsFor(root, name string) ([]finding.Finding, error) {
	set, err := validate.Plan(root, name)
	if err != nil {
		return nil, err
	}
	return set.Sorted(), nil
}

// Moving a plan onto the v2 contract.
//
// Mechanical where it can be, and never destructive where it cannot. Two rules
// decide everything below:
//
//   - **Nothing is deleted.** A section the contract does not have is moved to
//     plans/archive/<name>-notes.md, not dropped. The plan scanner reads plans/ with
//     ReadDir and skips directories, so the archived file is neither a phantom plan
//     nor something a validator reports on.
//
//   - **No placeholder passes the validator.** A missing required section is created
//     empty and the finding is left to appear. A `<!-- TODO -->` that satisfied the
//     check would be a plan that lies about being complete, which is worse than a
//     plan that says it is not.
//
// Migrating on read was considered and rejected: rewriting a user's file as a side
// effect of `scc map` violates "never author what the user owns" and produces a diff
// nobody asked for.

// archiveSeg is where the sections that are not in the contract go.
const archiveSeg = "archive"

// renames is the one heading whose content is already in the contract under another
// name. `## Decomposition` held list items citing `specs/<feature>/`, and the parser
// recognizes those by the citation rather than by the heading above them — so this is
// a rename and not a move, and not one line of content changes.
var renames = map[string]string{"decomposition": "References"}

type migration struct {
	Plan     string   `json:"plan"`
	Path     string   `json:"path"`
	Renamed  []string `json:"renamed,omitempty"`
	Archived []string `json:"archived,omitempty"`
	Created  []string `json:"created,omitempty"`
	Rewrote  int      `json:"tasks_rewritten"`
	Archive  string   `json:"archive,omitempty"`
	Status   string   `json:"status"`
	Findings []string `json:"findings,omitempty"`
	Written  bool     `json:"written"`
}

func runPlanMigrate(args []string) int {
	fs := flag.NewFlagSet("plan migrate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	dry := fs.Bool("dry-run", false, "report what would change and write nothing")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	name, ok := artifactName(rest, "plan")
	if !ok {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	path := paths.Plan(target, name)
	if !isFile(path) {
		render.Err(fmt.Sprintf("no plan %q under %s", name, paths.PlansSeg))
		return ExitError
	}
	a, err := artifact.Load(target, path)
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}
	if a.Approved() {
		render.Err(fmt.Sprintf("%s is approved; migrating rewrites its content", a.Path))
		render.Detail("  it is already on a contract somebody signed off — there is nothing to migrate")
		return ExitError
	}

	plan, archived, m := migrate(a, name)
	m.Path = a.Path
	if *dry {
		m.Written = false
		return reportMigration(m, *jsonOut, true)
	}

	if archived != "" {
		dir := filepath.Join(paths.Plans(target), archiveSeg)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			render.Err(err.Error())
			return ExitError
		}
		file := filepath.Join(dir, name+"-notes.md")
		if err := workspace.AtomicWrite(file, []byte(archived), 0o644); err != nil {
			render.Err(err.Error())
			return ExitError
		}
		m.Archive = relPath(target, file)
	}
	if err := workspace.AtomicWrite(path, []byte(plan), 0o644); err != nil {
		render.Err(err.Error())
		return ExitError
	}
	m.Written = true

	// The findings that are left are the point of the report: the sections migration
	// created empty are exactly the decisions a person still has to make.
	if set, err := planFindingsFor(target, name); err == nil {
		for _, f := range set {
			m.Findings = append(m.Findings, fmt.Sprintf("%d  %s  %s", f.Line, f.Rule, f.Message))
		}
	}
	return reportMigration(m, *jsonOut, false)
}

// migrate returns the rewritten plan, the archive file (empty when there is nothing
// to archive), and what it did.
func migrate(a *artifact.Artifact, name string) (plan, archive string, m migration) {
	m.Plan = name
	m.Status = artifact.StatusDraft

	known := map[string]bool{}
	for _, s := range planSectionSlugs() {
		known[s] = true
	}

	// Which lines belong to a section the contract does not have. Whole subtrees, so
	// a `### Detail` under `## Notes` travels with it.
	drop := map[int]bool{}
	var archived []artifact.Section
	for _, s := range a.Sections {
		if s.Level != 2 || known[s.Slug] {
			continue
		}
		if _, renamed := renames[s.Slug]; renamed {
			continue
		}
		archived = append(archived, s)
		for n := s.Line; n <= s.End; n++ {
			drop[n] = true
		}
	}

	var body []string
	for i, line := range a.Lines {
		n := i + 1
		if drop[n] {
			continue
		}
		if to, ok := renameAt(a, n); ok {
			m.Renamed = append(m.Renamed, to)
			body = append(body, "## "+to)
			continue
		}
		body = append(body, line)
	}
	for _, s := range archived {
		m.Archived = append(m.Archived, s.Title)
	}

	out := strings.Join(body, "\n")
	out = withStatus(out, artifact.StatusDraft)
	out, created := ensureSections(out)
	m.Created = created
	m.Rewrote = len(a.Tasks)

	if len(archived) == 0 {
		return withTrailingNewline(out), "", m
	}
	var buf strings.Builder
	fmt.Fprintf(&buf, "# %s — sections moved out of the plan\n\n", a.Title)
	buf.WriteString("These were in `" + a.Path + "` before it moved to the v2 contract, where a plan is a\n")
	buf.WriteString("header and a checklist. Nothing here was changed; it was moved so it could be read,\n")
	buf.WriteString("split into ADRs under `" + paths.DocsSeg + "/" + paths.ADRSeg + "/`, or deleted deliberately.\n")
	for _, s := range archived {
		buf.WriteString("\n")
		buf.WriteString(a.Text(s.Line, s.End))
		buf.WriteString("\n")
	}
	return withTrailingNewline(out), withTrailingNewline(buf.String()), m
}

func renameAt(a *artifact.Artifact, line int) (string, bool) {
	for _, s := range a.Sections {
		if s.Line == line && s.Level == 2 {
			if to, ok := renames[s.Slug]; ok {
				return to, true
			}
		}
	}
	return "", false
}

// ensureSections appends the required headings the plan does not have, empty.
func ensureSections(content string) (string, []string) {
	var created []string
	lower := strings.ToLower(content)
	for _, s := range requiredPlanSections() {
		if strings.Contains(lower, "\n## "+strings.ToLower(s)+"\n") ||
			strings.HasPrefix(lower, "## "+strings.ToLower(s)+"\n") {
			continue
		}
		created = append(created, s)
		content = strings.TrimRight(content, "\n") + "\n\n## " + s + "\n"
	}
	return content, created
}

func withStatus(content, status string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	lines, n := artifact.EnsureFrontmatter(lines)
	lines, _ = artifact.SetFrontmatterKey(lines, n, artifact.KeyStatus, status)
	return strings.Join(lines, "\n")
}

func reportMigration(m migration, jsonOut, dry bool) int {
	if jsonOut {
		return emitJSON(m)
	}
	for _, s := range m.Renamed {
		render.Info("renamed  → ## " + s)
	}
	for _, s := range m.Archived {
		render.Info("archived   ## " + s)
	}
	for _, s := range m.Created {
		render.Info("created    ## " + s + "  (empty — fill it in)")
	}
	if m.Archive != "" {
		render.Info("           " + m.Archive)
	}
	if dry {
		render.Info("--dry-run: nothing written")
		return ExitOK
	}
	render.OK(m.Path + " — migrated, status: " + m.Status)
	if len(m.Findings) > 0 {
		render.Warn(fmt.Sprintf("%d finding(s) remain — they are what is left for a person to decide", len(m.Findings)))
		for _, f := range m.Findings {
			render.Detail("  " + f)
		}
		render.Detail(fmt.Sprintf("  fix them, then `%s plan approve %s`", prog(), m.Plan))
		return ExitFindings
	}
	render.Info(fmt.Sprintf("`%s plan approve %s` seals it", prog(), m.Plan))
	return ExitOK
}
