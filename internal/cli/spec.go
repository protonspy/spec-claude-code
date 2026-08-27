package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/artifact"
	"github.com/protonspy/spec-claude-code/internal/assets"
	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/mdscan"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/validate"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// runSpec dispatches `scc spec <subcommand>`. A spec is the vehicle for work whose
// *what* and *how* need settling before code; everything else is a plan.
func runSpec(args []string) int {
	if len(args) == 0 {
		specUsage()
		return ExitError
	}
	switch args[0] {
	case "new":
		return runSpecNew(args[1:])
	case "list":
		return runSpecList(args[1:])
	case "show":
		return runSpecShow(args[1:])
	case "delete":
		return runSpecDelete(args[1:])
	case "track":
		return runSpecTrack(args[1:])
	case "sync":
		return runSpecSync(args[1:])
	case "validate":
		return runSpecValidate(args[1:])
	case "help", "-h", "--help":
		specUsage()
		return ExitOK
	default:
		render.Err(fmt.Sprintf("unknown spec subcommand %q", args[0]))
		specUsage()
		return ExitError
	}
}

// specArtifacts pairs each of the three files with the template that renders it.
// Order is the order the phases happen in, which is also the order init reports.
var specArtifacts = []struct {
	seg      string
	template string
}{
	{paths.RequirementsSeg, "requirements.md"},
	{paths.DesignSeg, "design.md"},
	{paths.TasksSeg, "tasks.md"},
}

func runSpecNew(args []string) int {
	fs := flag.NewFlagSet("spec new", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	kick := addKickoff(fs)
	force := fs.Bool("force", false, "overwrite an existing spec")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !kick.validate() {
		return ExitError
	}
	name, ok := artifactName(rest, "spec")
	if !ok {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}

	dir := paths.Spec(target, name)
	if _, err := os.Stat(dir); err == nil && !*force {
		render.Err(fmt.Sprintf("spec %q %v: %s", name, workspace.ErrAlreadyExists, workspace.Relative(target, dir)))
		render.Detail("  pass --force to overwrite it, or pick another name")
		return ExitError
	}

	data := assets.ArtifactData{
		Name:     name,
		Title:    assets.Title(name),
		Autonomy: *kick.autonomy,
		CI:       *kick.ci,
	}
	var written []string
	for _, a := range specArtifacts {
		content, err := assets.Artifact(a.template, data)
		if err != nil {
			render.Err(err.Error())
			return ExitError
		}
		path := filepath.Join(dir, a.seg)
		if err := workspace.WriteFile(path, content, *force); err != nil {
			render.Err(err.Error())
			return ExitError
		}
		written = append(written, relPath(target, path))
	}

	if *jsonOut {
		return emitJSON(struct {
			Spec     string   `json:"spec"`
			Path     string   `json:"path"`
			Files    []string `json:"files"`
			Autonomy string   `json:"autonomy"`
			CI       string   `json:"ci"`
		}{name, relPath(target, dir), written, *kick.autonomy, *kick.ci})
	}
	for _, w := range written {
		render.OK(w)
	}
	render.Info(fmt.Sprintf("autonomy: %s · ci: %s — recorded in requirements.md", *kick.autonomy, *kick.ci))
	render.Info("write requirements first: EARS, numbered R1.1, all five patterns valid")
	return ExitOK
}

// runSpecValidate checks one spec, or every spec when no name is given. The default
// is every spec: a workspace-wide answer is what a CI job wants, and naming one
// feature is the exception rather than the common case.
func runSpecValidate(args []string) int {
	return runValidation("spec", args, func(root string, rest []string) (*finding.Set, error) {
		switch len(rest) {
		case 0:
			return validate.Specs(root)
		case 1:
			name, ok := artifactName(rest, "spec")
			if !ok {
				return nil, errUsage
			}
			return validate.Spec(root, name)
		default:
			render.Err(fmt.Sprintf("spec validate takes at most one feature name, got %d", len(rest)))
			return nil, errUsage
		}
	})
}

// specEntry is what list and show report about one spec. The three booleans are the
// honest answer to "how far along is this?" without reading the files: a spec with
// tasks.md missing has not been decomposed yet.
type specEntry struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	Requirements bool   `json:"requirements"`
	Design       bool   `json:"design"`
	Tasks        bool   `json:"tasks"`
	Autonomy     string `json:"autonomy,omitempty"`
	CI           string `json:"ci,omitempty"`
	// Delivery is where this spec is being built and how far that has got. It is
	// listed beside the phases because they answer one question between them: the
	// phases say whether the spec is written, this says whether the work shipped.
	Delivery artifact.Delivery `json:"delivery,omitempty"`
}

func runSpecList(args []string) int {
	fs := flag.NewFlagSet("spec list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !noPositionals(rest, "spec list") {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}

	entries, err := os.ReadDir(paths.Specs(target))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		render.Err(err.Error())
		return ExitError
	}
	specs := []specEntry{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		specs = append(specs, describeSpec(target, e.Name()))
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })

	if *jsonOut {
		return emitJSON(struct {
			Specs []specEntry `json:"specs"`
			Count int         `json:"count"`
		}{specs, len(specs)})
	}
	if len(specs) == 0 {
		render.Info(fmt.Sprintf("no specs yet — `%s spec new <feature>`", prog()))
		return ExitOK
	}
	for _, s := range specs {
		render.Info(fmt.Sprintf("%s  %s%s", s.Name, phases(s), deliveryNote(s.Delivery)))
	}
	return ExitOK
}

// deliveryNote is the record on one line, or nothing at all when there is none. A
// spec nobody has started should not have to say so on every listing — the absence is
// already the answer, and a column of "not started" is a column nobody reads.
func deliveryNote(d artifact.Delivery) string {
	if !d.Tracked() {
		return ""
	}
	parts := []string{}
	if d.State != "" {
		parts = append(parts, d.State)
	}
	if d.Branch != "" {
		parts = append(parts, d.Branch)
	}
	if d.PR != 0 {
		parts = append(parts, fmt.Sprintf("#%d", d.PR))
	}
	return "  " + strings.Join(parts, " · ")
}

func runSpecShow(args []string) int {
	fs := flag.NewFlagSet("spec show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	name, ok := artifactName(rest, "spec")
	if !ok {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	if _, err := os.Stat(paths.Spec(target, name)); err != nil {
		render.Err(fmt.Sprintf("no spec %q under %s", name, paths.SpecsSeg))
		return ExitError
	}

	s := describeSpec(target, name)
	if *jsonOut {
		return emitJSON(s)
	}
	render.Info(render.Bold(s.Name) + "  " + s.Path)
	render.Info("phases: " + phases(s))
	if s.Delivery.Tracked() {
		render.Info("delivery:" + deliveryNote(s.Delivery))
	} else {
		render.Info(fmt.Sprintf("delivery: not recorded — `%s spec track %s --here` once you branch",
			prog(), s.Name))
	}
	if s.Autonomy != "" || s.CI != "" {
		render.Info(fmt.Sprintf("kickoff: autonomy=%s ci=%s", orUnset(s.Autonomy), orUnset(s.CI)))
	} else {
		// A missing answer means the run predates the convention, not that the user
		// did something wrong. Say what to do about it, not that it is a problem.
		render.Info("kickoff: not recorded — ask once, then write it into the frontmatter")
	}
	return ExitOK
}

func runSpecDelete(args []string) int {
	fs := flag.NewFlagSet("spec delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	force := fs.Bool("force", false, "required: deleting a spec discards requirements, design, and tasks")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	name, ok := artifactName(rest, "spec")
	if !ok {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	dir := paths.Spec(target, name)
	if _, err := os.Stat(dir); err != nil {
		render.Err(fmt.Sprintf("no spec %q under %s", name, paths.SpecsSeg))
		return ExitError
	}
	// --force is required rather than merely available. A spec is the record of what
	// a feature was supposed to do; deleting one is a decision, and the flag is what
	// makes it a deliberate one.
	if !*force {
		render.Err(fmt.Sprintf("refusing to delete spec %q without --force", name))
		return ExitError
	}
	if err := os.RemoveAll(dir); err != nil {
		render.Err(err.Error())
		return ExitError
	}
	if *jsonOut {
		return emitJSON(struct {
			Deleted string `json:"deleted"`
			Path    string `json:"path"`
		}{name, relPath(target, dir)})
	}
	render.OK(fmt.Sprintf("deleted %s", relPath(target, dir)))
	return ExitOK
}

// describeSpec reports what is on disk for one spec, including the kickoff answers
// when requirements.md records them. A frontmatter block scc cannot parse is left
// empty rather than guessed at — `spec validate` is where a malformed artifact
// gets reported, not here.
func describeSpec(root, name string) specEntry {
	s := specEntry{Name: name, Path: relPath(root, paths.Spec(root, name))}
	s.Requirements = isFile(paths.Requirements(root, name))
	s.Design = isFile(paths.Design(root, name))
	s.Tasks = isFile(paths.Tasks(root, name))
	if b, err := os.ReadFile(paths.Requirements(root, name)); err == nil {
		if fm, err := mdscan.ParseFrontmatter(string(b)); err == nil {
			s.Autonomy, _ = fm.Get("autonomy")
			s.CI, _ = fm.Get("ci")
			s.Delivery = artifact.ReadDelivery(fm.Values)
		}
	}
	return s
}

func phases(s specEntry) string {
	out := ""
	for _, p := range []struct {
		present bool
		label   string
	}{{s.Requirements, "requirements"}, {s.Design, "design"}, {s.Tasks, "tasks"}} {
		mark := render.Red("✗")
		if p.present {
			mark = render.Green("✓")
		}
		if out != "" {
			out += " "
		}
		out += mark + " " + p.label
	}
	return out
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func orUnset(s string) string {
	if s == "" {
		return "unset"
	}
	return s
}

func specUsage() {
	fmt.Fprintf(os.Stderr, `Usage:
  %s spec new <feature> [--autonomy=auto|gated] [--ci=wait|no-wait] [--force]
  %s spec list
  %s spec show <feature>
  %s spec delete <feature> --force
  %s spec track <feature> [--here|--branch <b>] [--pr <n>] [--delivery <state>]
  %s spec sync [<feature>…] [--base <b>] [--dry-run]
  %s spec validate [<feature>]

A spec is for work whose *what* and *how* need settling before code:
specs/<feature>/requirements.md, design.md, tasks.md. Everything else is a plan.

The delivery record — %s: · %s: · %s: — says where a spec is being built and how far
that has got, in requirements.md beside the kickoff answers. "track" writes what you
know the moment you know it; "sync" asks git and the forge and writes back what they
say, so a branch cannot go quietly unfinished. Nothing is guessed: a deleted branch
with no PR to ask about is reported undetermined and left alone, because merged and
abandoned look identical once a branch is gone.

  %s: %s
`, prog(), prog(), prog(), prog(), prog(), prog(), prog(),
		artifact.KeyBranch, artifact.KeyPR, artifact.KeyDelivery,
		artifact.KeyDelivery, strings.Join(artifact.DeliveryStates(), " · "))
}
