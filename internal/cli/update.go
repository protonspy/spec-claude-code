package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/assets"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/scaffold"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// runUpdate brings a workspace's managed files onto this build's templates.
//
// It reports before it acts, always. The plan is computed by hashing — every
// managed rule, agent, skill, and command is compared against what this version
// renders and against what the manifest says scc last wrote — so the three
// outcomes are distinguishable and are named separately in the summary: a file
// scc may replace because nothing was authored into it, a file the user changed,
// and a file this version no longer ships.
//
// Nothing is written until the user agrees. --yes is that agreement given up
// front, which is also what a non-interactive caller must pass: a command that
// blocked on a read nobody will answer would break the contract that an agent or
// a CI job drives scc exactly as well as a person does.
func runUpdate(args []string) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	yes := fs.Bool("yes", false, "apply without asking (required when stdin is not a terminal)")
	force := fs.Bool("force", false, "also overwrite files you have edited, naming each one")
	dryRun := fs.Bool("dry-run", false, "report the plan and write nothing")
	picks := map[string]*bool{}
	for _, h := range paths.Harnesses() {
		picks[h.ID] = fs.Bool(h.ID, false, "update only the "+h.ID+" tree")
	}
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !noPositionals(rest, "update") {
		return ExitError
	}

	target, ok := resolveRoot(*root)
	if !ok {
		return ExitError
	}

	harnesses, err := updateTargets(target, picks)
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}

	plans := make([]*scaffold.UpdatePlan, 0, len(harnesses))
	for _, h := range harnesses {
		plan, err := scaffold.PlanUpdate(target, h)
		if err != nil {
			render.Err(fmt.Sprintf("update failed: %v", err))
			return ExitError
		}
		plans = append(plans, plan)
	}

	if *jsonOut && *dryRun {
		return emitJSON(struct {
			Plans []*scaffold.UpdatePlan `json:"plans"`
		}{plans})
	}
	if !*jsonOut {
		reportPlans(harnesses, plans, *force)
	}

	pending := 0
	writes := false
	for _, p := range plans {
		pending += len(p.Pending())
		writes = writes || p.Writes(*force)
	}
	// The three ways an update ends without writing. Under --json they all emit the
	// same document the applying path does, with an empty result list: one command,
	// one output shape, whatever it decided to do.
	if pending == 0 {
		if *jsonOut {
			return emitResults(nil)
		}
		render.OK(fmt.Sprintf("already on template version %s — nothing to do", assets.Version))
		return ExitOK
	}
	if *dryRun {
		if *jsonOut {
			return emitResults(nil)
		}
		render.Info("dry run: nothing was written")
		return ExitOK
	}
	if !writes {
		if *jsonOut {
			return emitResults(nil)
		}
		render.Info("nothing to apply — the differences are all in files scc will not touch")
		return ExitOK
	}

	if !*yes {
		// --json has no way to ask: the prompt and the "nothing was written" line
		// both go to stdout, which is the document the caller is piping into jq.
		// Refuse the combination rather than emitting something that is not JSON.
		if *jsonOut {
			render.Err("--json cannot ask: pass --yes to apply, or --dry-run to see the plan")
			return ExitError
		}
		if !interactive() {
			render.Err("stdin is not a terminal: re-run with --yes to apply, or --dry-run to see the plan")
			return ExitError
		}
		if !confirm(promptIn, "Apply these changes?") {
			render.Info("nothing was written")
			return ExitOK
		}
	}

	results := make([]*scaffold.UpdateResult, 0, len(harnesses))
	for i, h := range harnesses {
		res, err := scaffold.ApplyUpdate(target, h, plans[i], scaffold.UpdateOptions{
			SCCVersion: version,
			Force:      *force,
		})
		if err != nil {
			render.Err(fmt.Sprintf("update failed: %v", err))
			return ExitError
		}
		results = append(results, res)
	}

	if *jsonOut {
		return emitResults(results)
	}
	applied := 0
	for _, res := range results {
		for _, it := range res.Applied {
			render.OK(fmt.Sprintf("%-8s %s", it.Action, it.Path))
			applied++
		}
	}
	render.OK(fmt.Sprintf("%d file(s) updated to template version %s", applied, assets.Version))
	for _, res := range results {
		for _, it := range res.Kept {
			if it.Action == scaffold.UpConflict {
				render.Warn(fmt.Sprintf("kept your edited %s — pass --force to take the new version", it.Path))
			}
			if it.Action == scaffold.UpOwned {
				render.Warn(fmt.Sprintf("%s is yours and was left alone; a newer template exists", it.Path))
			}
		}
	}
	return ExitOK
}

// emitResults writes the one document `update --json` ever emits. Never null: a
// caller iterating `.results` must not have to special-case the no-op run.
func emitResults(results []*scaffold.UpdateResult) int {
	if results == nil {
		results = []*scaffold.UpdateResult{}
	}
	return emitJSON(struct {
		Results []*scaffold.UpdateResult `json:"results"`
	}{results})
}

// updateTargets resolves which trees to update: the ones named by flag, or every
// harness the workspace was initialized for. Updating all of them by default is
// the honest reading of "bring this workspace up to date" — a repo worked on from
// two tools has two managed trees, and leaving one behind would quietly let them
// drift apart.
func updateTargets(root string, picks map[string]*bool) ([]paths.Harness, error) {
	initialized := workspace.Harnesses(root)
	if len(initialized) == 0 {
		return nil, fmt.Errorf("%s is not an scc workspace: run `%s init` first", root, prog())
	}
	var wanted []paths.Harness
	for _, h := range paths.Harnesses() {
		if p, ok := picks[h.ID]; ok && *p {
			wanted = append(wanted, h)
		}
	}
	if len(wanted) == 0 {
		return initialized, nil
	}
	have := map[string]bool{}
	for _, h := range initialized {
		have[h.ID] = true
	}
	for _, h := range wanted {
		if !have[h.ID] {
			return nil, fmt.Errorf("this workspace has no %s tree: run `%s init --%s` to create one",
				h.Label, prog(), h.ID)
		}
	}
	return wanted, nil
}

// reportPlans prints the summary the user is agreeing to. Grouped by what happens
// to the file rather than by directory, because the question being answered is
// "what does this cost me", and the answer is entirely in the conflict group.
func reportPlans(harnesses []paths.Harness, plans []*scaffold.UpdatePlan, force bool) {
	groups := []struct {
		action scaffold.UpdateAction
		label  string
	}{
		{scaffold.UpCreate, "create"},
		{scaffold.UpUpdate, "update"},
		{scaffold.UpDelete, "delete"},
		{scaffold.UpOrphan, "no longer managed, left on disk"},
		{scaffold.UpConflict, "you edited these"},
		{scaffold.UpOwned, "yours — never overwritten"},
	}
	for i, h := range harnesses {
		plan := plans[i]
		if len(plan.Pending()) == 0 {
			render.Info(fmt.Sprintf("%s: already on template version %s", h.Label, assets.Version))
			continue
		}
		fmt.Println()
		render.Info(fmt.Sprintf("%s — %s/ and %s", render.Bold(h.Label), h.Dir, h.EntryFile))
		for _, g := range groups {
			var listed []string
			for _, it := range plan.Items {
				if it.Action == g.action {
					listed = append(listed, it.Path)
				}
			}
			if len(listed) == 0 {
				continue
			}
			head := fmt.Sprintf("  %s (%d)", g.label, len(listed))
			switch g.action {
			case scaffold.UpConflict:
				if force {
					head += render.Yellow("  — --force will overwrite them")
				} else {
					head += render.Yellow("  — kept as they are")
				}
			}
			fmt.Println(head)
			for _, rel := range listed {
				fmt.Println("      " + rel)
			}
		}
	}
	fmt.Println()
}

// confirm asks a yes/no question, defaulting to no. Anything other than an
// explicit yes leaves the workspace untouched: the cost of a wrong "no" is
// running the command again, and the cost of a wrong "yes" is somebody's work.
func confirm(in io.Reader, question string) bool {
	render.Ask(question + " [y/N]: ")
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		fmt.Println()
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes"
}
