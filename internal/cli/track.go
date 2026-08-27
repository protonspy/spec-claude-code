package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/artifact"
	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/git"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// This file is the delivery record: `scc spec track` writes it and `scc spec sync`
// reconciles it with git.
//
// The problem it closes is that a branch leaves no trace in the artifacts. A spec says
// what the feature does and which boxes are ticked; git says a branch has been sitting
// unmerged for three weeks. Nothing joined the two, so "which of these is actually
// finished" was answerable only by a person holding both halves — and under
// `autonomy: auto` nobody is holding either.
//
// The split between the two commands is the one the rest of scc makes: `track` records
// what the caller knows and cannot be got wrong, `sync` derives what git knows and can.
// Neither guesses. A branch that no longer exists, with no pull request to ask about,
// is reported as undetermined and left exactly as it was — because a merged branch and
// an abandoned one look identical once they are deleted, and writing either one would
// be inventing the answer this record exists to stop people inventing.

// runSpecTrack records where a spec is being built. Every field is optional and each
// is written only when passed, so `--pr 28` after the PR opens does not have to
// restate the branch.
func runSpecTrack(args []string) int {
	fs := flag.NewFlagSet("spec track", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	branch := fs.String("branch", "", "the `branch` this spec is being built on")
	pr := fs.Int("pr", 0, "the pull request's `number`")
	state := fs.String("delivery", "", "how far it has got: `"+strings.Join(artifact.DeliveryStates(), "|")+"`")
	here := fs.Bool("here", false, "take the branch from the checkout you are in")
	dry := fs.Bool("dry-run", false, "show what would be written, and write nothing")
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

	pairs := [][2]string{}
	if *here {
		// The branch you are on is the branch the work is on, and asking scc for it
		// beats retyping it — a mistyped branch name is a record that points at
		// nothing, which is worse than no record because it reads as one.
		current, err := git.CurrentBranch(target)
		if err != nil || current == "" {
			render.Err("--here: could not read the current branch (no git, or a detached HEAD)")
			return ExitError
		}
		pairs = append(pairs, [2]string{artifact.KeyBranch, current})
	}
	if *branch != "" {
		if strings.ContainsAny(*branch, " \t") {
			render.Err(fmt.Sprintf("branch %q contains whitespace", *branch))
			return ExitError
		}
		pairs = append(pairs, [2]string{artifact.KeyBranch, *branch})
	}
	if *pr != 0 {
		if *pr < 0 {
			render.Err("a pull request number is a positive whole number")
			return ExitError
		}
		pairs = append(pairs, [2]string{artifact.KeyPR, strconv.Itoa(*pr)})
	}
	if *state != "" {
		if !artifact.ValidDelivery(*state) {
			render.Err(fmt.Sprintf("--delivery %q is not one of %s",
				*state, strings.Join(artifact.DeliveryStates(), ", ")))
			return ExitError
		}
		pairs = append(pairs, [2]string{artifact.KeyDelivery, *state})
	}
	if len(pairs) == 0 {
		render.Err("nothing to record: pass --branch, --here, --pr, or --delivery")
		render.Detail(fmt.Sprintf("  %s spec show %s   shows what is recorded now", prog(), name))
		return ExitError
	}

	// A branch is recorded because work has started on it, so the state follows from
	// it unless the caller said otherwise. Inferring the obvious here is what keeps
	// the common call one flag long; anything less obvious is `sync`'s job.
	if *state == "" && hasKey(pairs, artifact.KeyBranch) {
		next := artifact.DeliveryInProgress
		if *pr != 0 {
			next = artifact.DeliveryInReview
		}
		pairs = append(pairs, [2]string{artifact.KeyDelivery, next})
	} else if *state == "" && *pr != 0 {
		pairs = append(pairs, [2]string{artifact.KeyDelivery, artifact.DeliveryInReview})
	}

	res, err := writeDelivery(target, name, pairs, *dry)
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}
	if *jsonOut {
		code := ExitOK
		if len(res.Introduced) > 0 {
			code = ExitFindings
		}
		if emitJSON(res) != ExitOK {
			return ExitError
		}
		return code
	}
	return reportWrite(res, *dry)
}

func hasKey(pairs [][2]string, key string) bool {
	for _, p := range pairs {
		if p[0] == key {
			return true
		}
	}
	return false
}

// writeResult is what one spec's write did, in the shape both renderers read.
type writeResult struct {
	Spec       string            `json:"spec"`
	Path       string            `json:"path"`
	Changes    []string          `json:"changes"`
	Written    bool              `json:"written"`
	Verified   string            `json:"verified"` // clean | rolled-back | unchanged | not-run
	Introduced []finding.Finding `json:"introduced,omitempty"`
	Delivery   artifact.Delivery `json:"record"`
}

// writeDelivery sets the record on a spec's requirements.md, under the contract every
// scc write obeys: the file is re-validated afterwards and the write is undone when it
// introduced a finding the file did not already have.
//
// It goes through the artifact editor rather than through the frontmatter by hand, so
// a spec that has no frontmatter block at all gets one written correctly instead of
// growing a second.
func writeDelivery(root, name string, pairs [][2]string, dry bool) (writeResult, error) {
	rel := paths.SpecsSeg + "/" + name + "/" + paths.RequirementsSeg
	abs := paths.Requirements(root, name)
	if !isFile(abs) {
		return writeResult{}, fmt.Errorf("no spec %q: %s does not exist", name, rel)
	}
	// Load takes the absolute path; rel is what the report prints, so the same
	// artifact reads the same way on every platform.
	a, err := artifact.Load(root, abs)
	if err != nil {
		return writeResult{}, err
	}

	e := a.Edit()
	for _, p := range pairs {
		e.SetFrontmatter(p[0], p[1])
	}
	content, err := e.Content()
	if err != nil {
		return writeResult{}, err
	}
	res := writeResult{Spec: name, Path: rel, Verified: "unchanged"}
	for _, p := range pairs {
		res.Changes = append(res.Changes, p[0]+": "+p[1])
	}
	if e.Empty() {
		res.Delivery = artifact.ReadDelivery(a.Frontmatter)
		return res, nil
	}
	if dry {
		res.Verified = "not-run"
		res.Delivery = plannedDelivery(a.Frontmatter, pairs)
		return res, nil
	}

	original, err := os.ReadFile(a.Abs)
	if err != nil {
		return writeResult{}, err
	}
	before, checkable := validateArtifact(root, a)
	if err := workspace.AtomicWrite(a.Abs, []byte(withTrailingNewline(content)), 0o644); err != nil {
		return writeResult{}, err
	}
	res.Written, res.Verified = true, "clean"
	if !checkable {
		res.Verified = "no-validator"
	} else {
		after, _ := validateArtifact(root, a)
		if introduced := newFindings(before, after); len(introduced) > 0 {
			res.Introduced = introduced
			if writeErr := workspace.AtomicWrite(a.Abs, original, 0o644); writeErr != nil {
				return res, fmt.Errorf("the edit introduced findings and the rollback failed: %w", writeErr)
			}
			res.Written, res.Verified = false, "rolled-back"
		}
	}
	res.Delivery = plannedDelivery(a.Frontmatter, pairs)
	if !res.Written {
		res.Delivery = artifact.ReadDelivery(a.Frontmatter)
	}
	return res, nil
}

// plannedDelivery is the record as it will read once these pairs land.
func plannedDelivery(fm map[string]string, pairs [][2]string) artifact.Delivery {
	next := map[string]string{}
	for k, v := range fm {
		next[k] = v
	}
	for _, p := range pairs {
		next[p[0]] = p[1]
	}
	return artifact.ReadDelivery(next)
}

func reportWrite(res writeResult, dry bool) int {
	switch {
	case res.Verified == "unchanged":
		render.Info(fmt.Sprintf("%s already records that — nothing to write", res.Spec))
		return ExitOK
	case dry:
		render.Info(fmt.Sprintf("--dry-run: %s would record %s", res.Path, strings.Join(res.Changes, " · ")))
		return ExitOK
	case res.Verified == "rolled-back":
		render.Err(fmt.Sprintf("not written: the edit would introduce %d finding(s) in %s",
			len(res.Introduced), res.Path))
		for _, f := range res.Introduced {
			render.Detail(fmt.Sprintf("  %s  %s", f.Rule, f.Message))
		}
		return ExitFindings
	}
	render.OK(fmt.Sprintf("%s  %s", res.Spec, strings.Join(res.Changes, " · ")))
	return ExitOK
}

// syncResult is one spec's reconciliation.
type syncResult struct {
	Spec      string            `json:"spec"`
	Was       artifact.Delivery `json:"was"`
	Now       artifact.Delivery `json:"now"`
	Changed   bool              `json:"changed"`
	Written   bool              `json:"written"`
	Why       string            `json:"why"`
	Undecided string            `json:"undecided,omitempty"`
}

// runSpecSync reads git back into the specs: the command that makes the record true
// without anybody remembering to keep it true.
//
// With no feature named it walks every spec, which is the shape the question actually
// takes — "what is still open around here" is never asked about one spec.
func runSpecSync(args []string) int {
	fs := flag.NewFlagSet("spec sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	base := fs.String("base", "", "the `branch` work merges into (default: the repository's own)")
	dry := fs.Bool("dry-run", false, "report what git says, and write nothing")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	if !git.Found(git.Bin) || !git.IsRepo(target) {
		// Degrades rather than fails, the way every integration here does: a
		// workspace outside a repository is a legitimate workspace, and the record
		// still holds whatever `spec track` put in it.
		render.Warn("no git repository here — nothing to sync against")
		if *jsonOut {
			return emitJSON(struct {
				Specs []syncResult `json:"specs"`
				Count int          `json:"count"`
				Git   bool         `json:"git"`
			}{[]syncResult{}, 0, false})
		}
		return ExitOK
	}

	names, ok := syncTargets(target, rest)
	if !ok {
		return ExitError
	}
	if *base == "" {
		*base = git.Base(target)
	}
	hasGH := git.Found(git.GHBin)

	results := []syncResult{}
	for _, name := range names {
		d, err := readSpecDelivery(target, name)
		if err != nil {
			render.Err(err.Error())
			return ExitError
		}
		if !d.Tracked() {
			continue
		}
		r := reconcile(name, d, lookUp(target, d, *base, hasGH))
		if r.Changed && !*dry {
			res, err := writeDelivery(target, name,
				[][2]string{{artifact.KeyDelivery, r.Now.State}}, false)
			if err != nil {
				render.Err(err.Error())
				return ExitError
			}
			r.Written = res.Written
		}
		results = append(results, r)
	}

	if *jsonOut {
		return emitJSON(struct {
			Specs []syncResult `json:"specs"`
			Count int          `json:"count"`
			Base  string       `json:"base"`
			GH    bool         `json:"gh"`
			Git   bool         `json:"git"`
		}{results, len(results), *base, hasGH, true})
	}
	return reportSync(results, *base, hasGH, *dry)
}

// syncTargets is the specs to reconcile: the ones named, or all of them.
func syncTargets(root string, rest []string) ([]string, bool) {
	if len(rest) > 0 {
		for _, name := range rest {
			if err := workspace.SafeName(name, "spec"); err != nil {
				render.Err(err.Error())
				return nil, false
			}
		}
		return rest, true
	}
	entries, err := os.ReadDir(paths.Specs(root))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		render.Err(err.Error())
		return nil, false
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, true
}

func readSpecDelivery(root, name string) (artifact.Delivery, error) {
	abs := paths.Requirements(root, name)
	if !isFile(abs) {
		return artifact.Delivery{}, fmt.Errorf("no spec %q under %s", name, paths.SpecsSeg)
	}
	a, err := artifact.Load(root, abs)
	if err != nil {
		return artifact.Delivery{}, err
	}
	return artifact.ReadDelivery(a.Frontmatter), nil
}

// evidence is what the forge and the repository said about one spec's work.
type evidence struct {
	branch  git.Branch
	pr      git.PR
	askedPR bool
	prErr   error
}

func lookUp(root string, d artifact.Delivery, base string, hasGH bool) evidence {
	var ev evidence
	if d.Branch != "" {
		ev.branch, _ = git.Look(root, d.Branch, base)
	}
	if d.PR != 0 && hasGH {
		ev.askedPR = true
		ev.pr, ev.prErr = git.LookPR(root, d.PR)
	}
	return ev
}

// reconcile is the whole policy, kept as one pure function so the rules can be read
// and tested without a repository.
//
// The order is by strength of evidence. A pull request states the outcome outright, so
// it wins whenever it answered. Git can only prove the positive — this branch's commits
// are on the base, so it landed — and the negative case is genuinely ambiguous: a
// branch that is gone is equally one deleted after a clean merge and one abandoned.
// That ambiguity is reported, never resolved by preference.
func reconcile(name string, was artifact.Delivery, ev evidence) syncResult {
	r := syncResult{Spec: name, Was: was, Now: was}
	next, why, undecided := "", "", ""

	switch {
	case ev.askedPR && ev.prErr == nil:
		switch ev.pr.State {
		case git.StateMerged:
			next, why = artifact.DeliveryMerged, fmt.Sprintf("PR #%d is merged", ev.pr.Number)
		case git.StateClosed:
			next, why = artifact.DeliveryAbandoned, fmt.Sprintf("PR #%d was closed unmerged", ev.pr.Number)
		case git.StateOpen:
			next, why = artifact.DeliveryInReview, fmt.Sprintf("PR #%d is open", ev.pr.Number)
		default:
			undecided = fmt.Sprintf("gh reported PR #%d as %q", ev.pr.Number, ev.pr.State)
		}
	case ev.askedPR:
		undecided = fmt.Sprintf("gh could not read PR #%d", ev.pr.Number)
	case was.PR != 0:
		undecided = fmt.Sprintf("PR #%d is recorded and gh is not installed", was.PR)
	}

	if next == "" && was.Branch != "" {
		switch {
		case ev.branch.Merged:
			next, why = artifact.DeliveryMerged,
				fmt.Sprintf("%s has landed on %s", was.Branch, ev.branch.Base)
		case ev.branch.Exists():
			// A branch that exists and has not landed is in progress — unless a PR was
			// already recorded, in which case in-review is the more specific truth and
			// nothing here is evidence against it.
			next = artifact.DeliveryInProgress
			why = branchWhy(ev.branch)
			if was.PR != 0 && was.State == artifact.DeliveryInReview {
				next, why = was.State, why+", and a PR is recorded"
			}
		default:
			undecided = fmt.Sprintf("%s is gone; merged and abandoned look the same once a branch is deleted",
				was.Branch)
		}
	}

	if next != "" && next != was.State {
		r.Now.State, r.Changed, r.Why = next, true, why
		return r
	}
	if next != "" {
		r.Why = why
	}
	// A settled record has nothing left to decide, so the ambiguity of a deleted
	// branch is not worth reporting: that is what a merged branch is supposed to look
	// like. Reporting it anyway would put a warning on every finished spec forever,
	// which is how a report stops being read.
	if !artifact.Settled(was.State) {
		r.Undecided = undecided
	}
	return r
}

func reportSync(results []syncResult, base string, hasGH, dry bool) int {
	if len(results) == 0 {
		render.Info(fmt.Sprintf("no spec records a branch or a PR — `%s spec track <feature> --here`", prog()))
		return ExitOK
	}
	changed := 0
	for _, r := range results {
		switch {
		case r.Changed && dry:
			changed++
			render.Info(fmt.Sprintf("%-24s %s → %s  (%s)", r.Spec, orUnset(r.Was.State), r.Now.State, r.Why))
		case r.Changed:
			changed++
			render.OK(fmt.Sprintf("%-24s %s → %s  (%s)", r.Spec, orUnset(r.Was.State), r.Now.State, r.Why))
		case r.Undecided != "":
			render.Warn(fmt.Sprintf("%-24s %s — %s", r.Spec, orUnset(r.Was.State), r.Undecided))
		default:
			render.Info(fmt.Sprintf("%-24s %s", r.Spec, orUnset(r.Was.State)))
		}
	}
	// What is still open is the answer the command was run for, so it is said rather
	// than left to be counted off the rows above.
	open := 0
	for _, r := range results {
		if !artifact.Settled(r.Now.State) {
			open++
		}
	}
	render.Info(fmt.Sprintf("%d tracked · %d changed · %d still open · base %s%s",
		len(results), changed, open, base, ghNote(hasGH)))
	if dry && changed > 0 {
		render.Info("--dry-run: nothing was written")
	}
	return ExitOK
}

func ghNote(hasGH bool) string {
	if hasGH {
		return ""
	}
	// Said once per run rather than per spec: without gh, a deleted branch cannot be
	// told from a merged one, and the reader should know that is why some rows say so.
	return " · no gh, so a deleted branch cannot be resolved"
}

// branchWhy says what git found, in the terms a reader is actually asking about:
// how much work is sitting there unmerged.
func branchWhy(b git.Branch) string {
	switch {
	case b.Ahead == 1:
		return fmt.Sprintf("%s has 1 commit not on %s", b.Name, b.Base)
	case b.Ahead > 1:
		return fmt.Sprintf("%s has %d commits not on %s", b.Name, b.Ahead, b.Base)
	default:
		// Level with the base: branched and nothing done, or fast-forwarded and
		// nothing has advanced past it. Neither is delivered work.
		return fmt.Sprintf("%s is level with %s", b.Name, b.Base)
	}
}
