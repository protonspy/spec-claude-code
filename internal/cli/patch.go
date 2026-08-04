package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/artifact"
	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/validate"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// runPatch dispatches `scc patch <subcommand>` — changing an artifact at an address,
// without having read it.
//
// The guard that reading-first was providing is replaced by three that are stronger
// for a structured file: the address is resolved by the parser rather than matched as
// a string, so a miss is an error and never a write to the wrong place; the file is
// re-validated afterwards and the write is rolled back if it introduced a finding;
// and the lines that changed are printed back, which is the confirmation the read was
// standing in for.
func runPatch(args []string) int {
	if len(args) == 0 {
		patchUsage()
		return ExitError
	}
	switch args[0] {
	case "check":
		return runPatchCheck(args[1:], true)
	case "uncheck":
		return runPatchCheck(args[1:], false)
	case "task":
		return runPatchTask(args[1:])
	case "add":
		return runPatchAdd(args[1:])
	case "rm":
		return runPatchRemove(args[1:])
	case "append", "prepend", "replace":
		return runPatchText(args[0], args[1:])
	case "fm":
		return runPatchFrontmatter(args[1:])
	case "help", "-h", "--help":
		patchUsage()
		return ExitOK
	default:
		render.Err(fmt.Sprintf("unknown patch subcommand %q", args[0]))
		patchUsage()
		return ExitError
	}
}

// patchFlags is the set every patch subcommand shares, so the safety valves are
// spelled and behave identically across the surface.
type patchFlags struct {
	root    *string
	dry     *bool
	force   *bool
	jsonOut *bool
}

func addPatchFlags(fs *flag.FlagSet) patchFlags {
	return patchFlags{
		root: addRoot(fs),
		dry:  fs.Bool("dry-run", false, "show the change and write nothing"),
		force: fs.Bool("force", false,
			"write even when the change introduces a validation finding"),
		jsonOut: addJSON(fs),
	}
}

func runPatchCheck(args []string, done bool) int {
	name := "patch uncheck"
	if done {
		name = "patch check"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pf := addPatchFlags(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if len(rest) < 2 {
		render.Err(name + " needs an artifact and at least one task number")
		return ExitError
	}
	return withEditor(pf, rest[0], func(e *artifact.Editor) {
		for _, number := range rest[1:] {
			e.Check(number, done)
		}
	})
}

func runPatchTask(args []string) int {
	fs := flag.NewFlagSet("patch task", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pf := addPatchFlags(fs)
	text := fs.String("text", "", "replace the description")
	method := fs.String("method", "", "replace the methodology: `Unit` or TDD")
	req := fs.String("req", "", "replace the citations: a comma-separated `list` of ids")
	number := fs.String("number", "", "renumber the task")
	state := fs.String("state", "", "set the box: `open` or done")
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if len(rest) != 2 {
		render.Err("patch task needs an artifact and one task number")
		return ExitError
	}
	edit := artifact.TaskEdit{}
	if isSet(fs, "text") {
		edit.Text = text
	}
	if isSet(fs, "method") {
		if !validMethod(*method) {
			return ExitError
		}
		edit.Methodology = method
	}
	if isSet(fs, "req") {
		ids := splitList(*req)
		edit.Requirements = &ids
	}
	if isSet(fs, "number") {
		edit.Number = number
	}
	if isSet(fs, "state") {
		switch *state {
		case "done":
			t := true
			edit.Checked = &t
		case "open":
			f := false
			edit.Checked = &f
		default:
			render.Err(fmt.Sprintf("--state is `open` or `done`, got %q", *state))
			return ExitError
		}
	}
	if edit.Text == nil && edit.Methodology == nil && edit.Requirements == nil &&
		edit.Number == nil && edit.Checked == nil {
		render.Err("patch task changes nothing: pass --text, --method, --req, --number, or --state")
		return ExitError
	}
	return withEditor(pf, rest[0], func(e *artifact.Editor) { e.SetTask(rest[1], edit) })
}

func runPatchAdd(args []string) int {
	fs := flag.NewFlagSet("patch add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pf := addPatchFlags(fs)
	section := fs.String("section", "", "the `section` slug whose task list this joins")
	number := fs.String("number", "", "the task's `number`")
	method := fs.String("method", "Unit", "`Unit` or TDD")
	text := fs.String("text", "", "the description")
	req := fs.String("req", "", "the requirements it satisfies, comma-separated")
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if len(rest) != 1 {
		render.Err("patch add needs an artifact")
		return ExitError
	}
	if *section == "" || *number == "" || strings.TrimSpace(*text) == "" {
		render.Err("patch add needs --section, --number and --text")
		return ExitError
	}
	if !validMethod(*method) {
		return ExitError
	}
	t := artifact.NewTask{
		Section: *section, Number: *number, Methodology: *method,
		Text: *text, Requirements: splitList(*req),
	}
	return withEditor(pf, rest[0], func(e *artifact.Editor) { e.AddTask(t) })
}

func runPatchRemove(args []string) int {
	fs := flag.NewFlagSet("patch rm", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pf := addPatchFlags(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if len(rest) != 2 {
		render.Err("patch rm needs an artifact and one task number")
		return ExitError
	}
	return withEditor(pf, rest[0], func(e *artifact.Editor) { e.RemoveTask(rest[1]) })
}

func runPatchText(op string, args []string) int {
	fs := flag.NewFlagSet("patch "+op, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pf := addPatchFlags(fs)
	text := fs.String("text", "", "the text to write; `-` reads stdin, which is how multi-line prose gets in")
	file := fs.String("file", "", "read the text from this `path` instead")
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if len(rest) != 2 {
		render.Err(fmt.Sprintf("patch %s needs an artifact and one address", op))
		return ExitError
	}
	body, ok := readText(*text, *file)
	if !ok {
		return ExitError
	}
	if strings.TrimSpace(body) == "" {
		render.Err("nothing to write: pass --text, --text - to read stdin, or --file")
		return ExitError
	}
	return withEditor(pf, rest[0], func(e *artifact.Editor) {
		switch op {
		case "append":
			e.Append(rest[1], body)
		case "prepend":
			e.Prepend(rest[1], body)
		case "replace":
			e.Replace(rest[1], body)
		}
	})
}

func runPatchFrontmatter(args []string) int {
	fs := flag.NewFlagSet("patch fm", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pf := addPatchFlags(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if len(rest) < 2 {
		render.Err("patch fm needs an artifact and at least one key=value")
		return ExitError
	}
	pairs := make([][2]string, 0, len(rest)-1)
	for _, arg := range rest[1:] {
		k, v, ok := strings.Cut(arg, "=")
		if !ok || strings.TrimSpace(k) == "" {
			render.Err(fmt.Sprintf("expected key=value, got %q", arg))
			return ExitError
		}
		pairs = append(pairs, [2]string{strings.TrimSpace(k), strings.TrimSpace(v)})
	}
	return withEditor(pf, rest[0], func(e *artifact.Editor) {
		for _, p := range pairs {
			e.SetFrontmatter(p[0], p[1])
		}
	})
}

// withEditor is the whole write path: load, apply, verify, and roll back.
//
// The verification is the part that earns the right to skip reading. A patch is
// checked against the same validators `scc validate` runs, before and after, and a
// finding the file did not already have means the edit is undone and reported rather
// than left on disk. An artifact scc has no validator for — a wiki page — is written
// and said to be unverified, because claiming a check that did not happen is worse
// than not checking.
func withEditor(pf patchFlags, target string, apply func(*artifact.Editor)) int {
	root, ok := resolveRoot(*pf.root)
	if !ok || !requireWorkspace(root) {
		return ExitError
	}
	a, ok := loadOne(root, target)
	if !ok {
		return ExitError
	}

	e := a.Edit()
	apply(e)
	content, err := e.Content()
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}
	changes := e.Changes()

	if e.Empty() {
		if *pf.jsonOut {
			return emitJSON(patchReport{Path: a.Path, Changes: nil, Written: false, Verified: "unchanged"})
		}
		render.Info("already in that state — nothing to write")
		return ExitOK
	}
	if *pf.dry {
		if *pf.jsonOut {
			return emitJSON(patchReport{Path: a.Path, Changes: changes, Written: false, Verified: "not-run"})
		}
		printChanges(a.Path, changes)
		render.Info(fmt.Sprintf("--dry-run: nothing written (%s)", scaleOf(changes)))
		return ExitOK
	}
	if lost := deleted(changes); lost > destructiveLines && !*pf.force {
		// The failure mode a caller cannot see coming. An address is a name, not a
		// span, so `replace #notes` reads as one small edit and resolves to four
		// hundred lines — and the whole point of this command is that nobody looked at
		// the file first. So a change that destroys more than a screenful stops and
		// shows what it was about to remove, and --force is the separate decision.
		if *pf.jsonOut {
			emitJSON(patchReport{Path: a.Path, Changes: changes, Written: false, Verified: "refused"})
			return ExitError
		}
		printChanges(a.Path, changes)
		render.Err(fmt.Sprintf("refusing to remove %d lines from %s without --force", lost, a.Path))
		render.Detail("  the address resolved to more than it looks like: check it with")
		render.Detail(fmt.Sprintf("    %s map show %s %s", prog(), a.Name, changes[0].Ref))
		return ExitError
	}

	original, err := os.ReadFile(a.Abs)
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}
	before, checkable := validateArtifact(root, a)
	if err := workspace.AtomicWrite(a.Abs, []byte(content), 0o644); err != nil {
		render.Err(err.Error())
		return ExitError
	}

	report := patchReport{Path: a.Path, Changes: changes, Written: true, Verified: "clean"}
	if !checkable {
		report.Verified = "no-validator"
	} else {
		after, _ := validateArtifact(root, a)
		if introduced := newFindings(before, after); len(introduced) > 0 {
			report.Introduced = introduced
			if !*pf.force {
				if writeErr := workspace.AtomicWrite(a.Abs, original, 0o644); writeErr != nil {
					render.Err("the edit introduced findings and the rollback failed: " + writeErr.Error())
					return ExitError
				}
				report.Written = false
				report.Verified = "rolled-back"
			} else {
				report.Verified = "forced"
			}
		}
	}

	if *pf.jsonOut {
		code := ExitOK
		if len(report.Introduced) > 0 {
			code = ExitFindings
		}
		if emitJSON(report) != ExitOK {
			return ExitError
		}
		return code
	}
	return printPatchReport(report)
}

// patchReport is what a patch says it did, in the one shape both renderers read.
type patchReport struct {
	Path       string            `json:"path"`
	Changes    []artifact.Change `json:"changes"`
	Written    bool              `json:"written"`
	Verified   string            `json:"verified"` // clean | rolled-back | forced | refused | no-validator | not-run | unchanged
	Introduced []finding.Finding `json:"introduced,omitempty"`
}

func printPatchReport(r patchReport) int {
	printChanges(r.Path, r.Changes)
	switch r.Verified {
	case "rolled-back":
		render.Err(fmt.Sprintf("rolled back: the change introduced %d finding(s)", len(r.Introduced)))
		for _, f := range r.Introduced {
			render.Detail(fmt.Sprintf("  %s:%d  %s  %s", f.File, f.Line, f.Rule, f.Message))
		}
		render.Detail("  fix the input, or pass --force to write it anyway")
		return ExitFindings
	case "forced":
		render.Warn(fmt.Sprintf("written with %d new finding(s) because --force was given", len(r.Introduced)))
		for _, f := range r.Introduced {
			render.Detail(fmt.Sprintf("  %s:%d  %s  %s", f.File, f.Line, f.Rule, f.Message))
		}
		return ExitFindings
	case "no-validator":
		render.OK(r.Path + " — written; scc has no validator for this artifact, so nothing was checked")
		return ExitOK
	default:
		render.OK(fmt.Sprintf("%s — written and re-validated clean", r.Path))
		return ExitOK
	}
}

// destructiveLines is where a patch stops being an edit and starts being a
// deletion. A screenful: below it a caller can see the whole change in the report
// the command prints back, and above it they cannot.
const destructiveLines = 12

// deleted is how many lines the change set removes on net — what the caller loses if
// the address resolved to more than they meant.
func deleted(changes []artifact.Change) int {
	lost := 0
	for _, c := range changes {
		if n := len(c.Before) - len(c.After); n > 0 {
			lost += n
		}
	}
	return lost
}

func scaleOf(changes []artifact.Change) string {
	added, removed := 0, 0
	for _, c := range changes {
		added += len(c.After)
		removed += len(c.Before)
	}
	return fmt.Sprintf("+%d −%d lines", added, removed)
}

// printChanges shows the displaced and the written lines. It is the confirmation
// that replaces having read the file: a caller sees exactly what its address
// resolved to.
func printChanges(path string, changes []artifact.Change) {
	for _, c := range changes {
		render.Info(fmt.Sprintf("%s %s  %s:%d", c.Op, render.Bold(c.Ref), path, c.Line))
		printSide(c.Before, render.Red("  - "))
		printSide(c.After, render.Green("  + "))
	}
}

// printSide prints one side of a change, elided in the middle.
//
// The elision is not cosmetic. This command exists so a caller never has to hold the
// file, and a confirmation that echoed four hundred displaced lines would put the
// file in their context by the back door — while reporting a refusal to touch it.
func printSide(lines []string, marker string) {
	const shown = 3
	if len(lines) <= 2*shown+1 {
		for _, line := range lines {
			render.Detail(marker + line)
		}
		return
	}
	for _, line := range lines[:shown] {
		render.Detail(marker + line)
	}
	render.Detail(fmt.Sprintf("%s… %d more lines", marker, len(lines)-2*shown))
	for _, line := range lines[len(lines)-shown:] {
		render.Detail(marker + line)
	}
}

// validateArtifact runs the validator that owns this file. The second return is
// false when scc has none, which is not a failure — the knowledge base is Markdown
// too, and refusing to edit it would be the wrong lesson to draw from having no rule
// for it.
func validateArtifact(root string, a *artifact.Artifact) ([]finding.Finding, bool) {
	var set *finding.Set
	var err error
	switch a.Kind {
	case artifact.KindPlan:
		set, err = validate.Plan(root, a.Name)
	case artifact.KindRequirements, artifact.KindDesign, artifact.KindTasks:
		set, err = validate.Spec(root, a.Spec)
	default:
		return nil, false
	}
	if err != nil || set == nil {
		return nil, false
	}
	return set.Sorted(), true
}

// newFindings is what the edit caused: findings whose rule and message are not in
// the set the file already had.
//
// Line numbers are deliberately not part of the identity. An insertion moves every
// finding below it, and comparing on line would report the whole tail of a
// pre-existing problem as newly caused by a task added above it.
func newFindings(before, after []finding.Finding) []finding.Finding {
	had := map[string]int{}
	for _, f := range before {
		had[f.Rule+"\x00"+f.Message]++
	}
	var out []finding.Finding
	for _, f := range after {
		key := f.Rule + "\x00" + f.Message
		if had[key] > 0 {
			had[key]--
			continue
		}
		out = append(out, f)
	}
	return out
}

// readText resolves the three ways prose gets in: inline, from stdin, or from a
// file. Stdin exists because a note is paragraphs, and a shell argument is a bad
// place to put paragraphs.
func readText(text, file string) (string, bool) {
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			render.Err(err.Error())
			return "", false
		}
		return string(b), true
	}
	if text == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			render.Err(err.Error())
			return "", false
		}
		return string(b), true
	}
	return text, true
}

func validMethod(m string) bool {
	if m == "Unit" || m == "TDD" {
		return true
	}
	render.Err(fmt.Sprintf("a methodology is `Unit` or `TDD`, got %q", m))
	return false
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// isSet reports whether the caller actually passed a flag, which is what separates
// "clear the citations" from "leave them alone".
func isSet(fs *flag.FlagSet, name string) bool {
	seen := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			seen = true
		}
	})
	return seen
}

func patchUsage() {
	fmt.Fprintf(os.Stderr, `Usage:
  %s patch check   <artifact> <number>…        mark tasks done
  %s patch uncheck <artifact> <number>…        mark tasks not done
  %s patch task    <artifact> <number> [--text|--method|--req|--number|--state]
  %s patch add     <artifact> --section <slug> --number N --text "…" [--method|--req]
  %s patch rm      <artifact> <number>
  %s patch append  <artifact> <address> --text "…"|--text -|--file <path>
  %s patch prepend <artifact> <address> …
  %s patch replace <artifact> <address> …
  %s patch fm      <artifact> key=value…       write the frontmatter

Changes an artifact at an address — the same addresses "%s map" prints — so the file
never has to be read first. Every address is resolved by the parser, so a miss is an
error and never a write to the wrong place.

After writing, the file is re-validated. A change that introduces a finding is rolled
back and reported; --force writes it anyway. --dry-run shows the lines and writes
nothing.

Exit codes: 0 written · 1 usage or runtime error · 2 rolled back, or forced with findings.
`, prog(), prog(), prog(), prog(), prog(), prog(), prog(), prog(), prog(), prog())
}
