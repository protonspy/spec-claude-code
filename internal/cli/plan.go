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
	case "approve":
		return runPlanApprove(args[1:])
	case "reseal":
		return runPlanReseal(args[1:])
	case "migrate":
		return runPlanMigrate(args[1:])
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

// runPlanApprove closes authorship and opens execution.
//
// It refuses a plan with findings, and that refusal is the point of the command: an
// approved plan is one nothing may rewrite, so approving one that is already wrong
// would freeze the defect and make fixing it require `--force`. Everything after this
// is `patch check`, `patch fm`, and discovery.
func runPlanApprove(args []string) int {
	fs := flag.NewFlagSet("plan approve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
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

	set, err := validate.Plan(target, name)
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}
	if !set.Empty() {
		if *jsonOut {
			emitJSON(set.Document())
			return ExitFindings
		}
		set.Report(relPath(target, path))
		render.Detail("  an approved plan is one nothing may rewrite; fix these first")
		return ExitFindings
	}

	content, err := os.ReadFile(path)
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}
	sealed := withTrailingNewline(artifact.Approve(string(content)))
	if err := workspace.AtomicWrite(path, []byte(sealed), 0o644); err != nil {
		render.Err(err.Error())
		return ExitError
	}
	sum := artifact.Seal(sealed)
	if *jsonOut {
		return emitJSON(struct {
			Plan     string `json:"plan"`
			Path     string `json:"path"`
			Status   string `json:"status"`
			Checksum string `json:"checksum"`
		}{name, relPath(target, path), artifact.StatusApproved, sum})
	}
	render.OK(fmt.Sprintf("%s — approved", relPath(target, path)))
	render.Info("seal: " + sum)
	render.Info("its content is fixed now: tick boxes with `" + prog() + " patch check`, and discover with `" +
		prog() + " patch add|rm --reason`")
	return ExitOK
}

// runPlanReseal records a legitimate edit made outside the cycle — a merge conflict
// resolved by hand is the case it was written for.
//
// It demands --force and names both hashes, because the honest description of what it
// does is "erase the evidence that this file was edited outside scc". A command that
// did that quietly would make the seal worth nothing.
func runPlanReseal(args []string) int {
	fs := flag.NewFlagSet("plan reseal", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	force := fs.Bool("force", false, "required: reselling accepts an edit made outside scc as intended")
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
	if !a.Approved() {
		render.Err(fmt.Sprintf("%s is not approved, so it carries no seal to recompute", relPath(target, path)))
		render.Detail(fmt.Sprintf("  `%s plan approve %s` is what seals it", prog(), name))
		return ExitError
	}
	recorded, actual, drifted := a.Drift()
	if !drifted {
		if *jsonOut {
			return emitJSON(struct {
				Plan     string `json:"plan"`
				Checksum string `json:"checksum"`
				Changed  bool   `json:"changed"`
			}{name, recorded, false})
		}
		render.OK(relPath(target, path) + " — the seal already matches; nothing to do")
		return ExitOK
	}
	if !*force {
		render.Err(fmt.Sprintf("%s has drifted; reselling accepts that edit as intended", relPath(target, path)))
		render.Detail("  seal:    " + recorded)
		render.Detail("  actual:  " + actual)
		render.Detail("  → `git diff " + relPath(target, path) + "` is the change you are about to bless")
		render.Detail("  → pass --force once you have read it, or revert it with git")
		return ExitError
	}

	content, err := os.ReadFile(path)
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}
	sealed := withTrailingNewline(artifact.Reseal(string(content)))
	if err := workspace.AtomicWrite(path, []byte(sealed), 0o644); err != nil {
		render.Err(err.Error())
		return ExitError
	}
	if *jsonOut {
		return emitJSON(struct {
			Plan     string `json:"plan"`
			Path     string `json:"path"`
			Was      string `json:"was"`
			Checksum string `json:"checksum"`
			Changed  bool   `json:"changed"`
		}{name, relPath(target, path), recorded, artifact.Seal(sealed), true})
	}
	render.OK(relPath(target, path) + " — resealed")
	render.Info("was:  " + recorded)
	render.Info("now:  " + artifact.Seal(sealed))
	return ExitOK
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

// withTrailingNewline is how every file scc writes ends. A sealed plan that lost its
// final newline would hash differently from the same plan a text editor saved.
func withTrailingNewline(s string) string {
	return strings.TrimRight(s, "\n") + "\n"
}

func planUsage() {
	fmt.Fprintf(os.Stderr, `Usage:
  %s plan new <name> [--autonomy=auto|gated] [--ci=wait|no-wait] [--force]
  %s plan list
  %s plan delete <name> --force
  %s plan validate [<name>]
  %s plan approve <name>          validate, then fix the content and seal it
  %s plan reseal <name> --force   accept an edit made outside scc, and re-seal
  %s plan migrate <name>          move a plan onto the closed-section contract

A plan is everything that is not worth a spec: plans/<name>.md, holding a short
header and a checklist. Its sections are closed — Why, Paths, References, Out of
scope, Tasks, Done when — which is the only thing that caps its size.

Approving it makes the content fixed: after that, `+"`%s patch check`"+` moves the boxes and
discovery adds or strikes tasks with a reason, and anything that would rewrite the
work is refused.
`, prog(), prog(), prog(), prog(), prog(), prog(), prog(), prog())
}
