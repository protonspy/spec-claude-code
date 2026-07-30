package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/assets"
	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/mdscan"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/validate"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// runPlan dispatches `scc plan <subcommand>`.
//
// A plan is everything that is not worth a spec, and it covers both ends of that:
// an initiative too large for one spec, decomposed into specs it references, and a
// change small enough that the *what* was never in doubt, recorded as a checklist.
// It is one file, because it holds no state beyond its own checklist — where an item
// references a spec, the state lives in that spec and is read from there.
func runPlan(args []string) int {
	if len(args) == 0 {
		planUsage()
		return ExitError
	}
	switch args[0] {
	case "new":
		return runPlanNew(args[1:])
	case "list":
		return runPlanList(args[1:])
	case "delete":
		return runPlanDelete(args[1:])
	case "validate":
		return runPlanValidate(args[1:])
	case "help", "-h", "--help":
		planUsage()
		return ExitOK
	default:
		render.Err(fmt.Sprintf("unknown plan subcommand %q", args[0]))
		planUsage()
		return ExitError
	}
}

func runPlanNew(args []string) int {
	fs := flag.NewFlagSet("plan new", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	kick := addKickoff(fs)
	force := fs.Bool("force", false, "overwrite an existing plan")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !kick.validate() {
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
	if _, err := os.Stat(path); err == nil && !*force {
		render.Err(fmt.Sprintf("plan %q %v: %s", name, workspace.ErrAlreadyExists, relPath(target, path)))
		render.Detail("  pass --force to overwrite it, or pick another name")
		return ExitError
	}
	content, err := assets.Artifact("plan.md", assets.ArtifactData{
		Name:     name,
		Title:    assets.Title(name),
		Autonomy: *kick.autonomy,
		CI:       *kick.ci,
	})
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}
	if err := workspace.WriteFile(path, content, *force); err != nil {
		render.Err(err.Error())
		return ExitError
	}

	if *jsonOut {
		return emitJSON(struct {
			Plan     string `json:"plan"`
			Path     string `json:"path"`
			Autonomy string `json:"autonomy"`
			CI       string `json:"ci"`
		}{name, relPath(target, path), *kick.autonomy, *kick.ci})
	}
	render.OK(relPath(target, path))
	render.Info(fmt.Sprintf("autonomy: %s · ci: %s — recorded in the frontmatter", *kick.autonomy, *kick.ci))
	render.Info("one source of truth per item: a checkbox, or a spec reference — never both")
	return ExitOK
}

// runPlanValidate checks one plan, or every plan when no name is given.
func runPlanValidate(args []string) int {
	return runValidation("plan", args, func(root string, rest []string) (*finding.Set, error) {
		switch len(rest) {
		case 0:
			return validate.Plans(root)
		case 1:
			name, ok := artifactName(rest, "plan")
			if !ok {
				return nil, errUsage
			}
			return validate.Plan(root, name)
		default:
			render.Err(fmt.Sprintf("plan validate takes at most one plan name, got %d", len(rest)))
			return nil, errUsage
		}
	})
}

type planEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Autonomy string `json:"autonomy,omitempty"`
	CI       string `json:"ci,omitempty"`
}

func runPlanList(args []string) int {
	fs := flag.NewFlagSet("plan list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !noPositionals(rest, "plan list") {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}

	entries, err := os.ReadDir(paths.Plans(target))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		render.Err(err.Error())
		return ExitError
	}
	plans := []planEntry{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		p := planEntry{Name: name, Path: relPath(target, filepath.Join(paths.Plans(target), e.Name()))}
		if b, err := os.ReadFile(filepath.Join(paths.Plans(target), e.Name())); err == nil {
			if fm, err := mdscan.ParseFrontmatter(string(b)); err == nil {
				p.Autonomy, _ = fm.Get("autonomy")
				p.CI, _ = fm.Get("ci")
			}
		}
		plans = append(plans, p)
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].Name < plans[j].Name })

	if *jsonOut {
		return emitJSON(struct {
			Plans []planEntry `json:"plans"`
			Count int         `json:"count"`
		}{plans, len(plans)})
	}
	if len(plans) == 0 {
		render.Info(fmt.Sprintf("no plans yet — `%s plan new <name>`", prog()))
		return ExitOK
	}
	for _, p := range plans {
		render.Info(fmt.Sprintf("%s  %s", p.Name, p.Path))
	}
	return ExitOK
}

func runPlanDelete(args []string) int {
	fs := flag.NewFlagSet("plan delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	force := fs.Bool("force", false, "required: deleting a plan discards its checklist and decomposition")
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
	if !*force {
		render.Err(fmt.Sprintf("refusing to delete plan %q without --force", name))
		return ExitError
	}
	if err := os.Remove(path); err != nil {
		render.Err(err.Error())
		return ExitError
	}
	if *jsonOut {
		return emitJSON(struct {
			Deleted string `json:"deleted"`
			Path    string `json:"path"`
		}{name, relPath(target, path)})
	}
	render.OK(fmt.Sprintf("deleted %s", relPath(target, path)))
	return ExitOK
}

func planUsage() {
	fmt.Fprintf(os.Stderr, `Usage:
  %s plan new <name> [--autonomy=auto|gated] [--ci=wait|no-wait] [--force]
  %s plan list
  %s plan delete <name> --force
  %s plan validate [<name>]

A plan is everything that is not worth a spec: plans/<name>.md, holding a checklist
of tasks, references to the specs it decomposes into, or both.
`, prog(), prog(), prog(), prog())
}
