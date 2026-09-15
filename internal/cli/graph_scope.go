package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/codegraph"
	"github.com/protonspy/spec-claude-code/internal/manifest"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// The graph's scope: which trees of this workspace get indexed.
//
// It is recorded in the manifest beside the delivery gate's commands, for the
// same reason and on the same terms — it is *input*, the project telling scc
// something scc cannot derive, in the one file scc already has.
//
// What it cannot be is a filter. CodeGraph indexes one root at a time: `init`
// takes a single optional path and refuses two, no command has an include or
// exclude flag, and the only exclusion it honors is `.gitignore`. So a scope is N
// graphs, one per directory, and every command here fans out over them. That is
// stated wherever a user meets it, because "the graph is scoped to backend/src
// and frontend/src" and "there are two graphs" are the same fact and only the
// second one explains why `status` answers twice.

// loadScope reads the recorded scope and resolves it to the trees to index.
//
// A pattern that matches nothing is named and skipped rather than fatal: a scope
// is committed and branches differ, so a directory this checkout does not have is
// worth one line and is no reason to refuse to index the ones it does.
//
// Every pattern missing is different, and the caller stops. Falling back to the
// whole workspace there would be the opposite of what the scope asked for —
// indexing everything because a path was misspelled is an expensive way to find
// out about the typo.
func loadScope(root string, quiet bool) ([]codegraph.Root, bool) {
	scope, err := scopeOf(root)
	if err != nil {
		render.Err(fmt.Sprintf("graph: %v", err))
		return nil, false
	}
	roots, missing := codegraph.Roots(root, scope)
	if !quiet {
		for _, m := range missing {
			render.Warn(fmt.Sprintf("%s matches nothing in this checkout — recorded in %s", m, manifestName(root)))
		}
	}
	if len(roots) == 0 {
		render.Err("graph: every recorded scope matches nothing here")
		render.Detail(fmt.Sprintf("  fix it with `%s graph scope set <dir>…`, or clear it to index the whole workspace", prog()))
		return nil, false
	}
	return roots, true
}

// scopeOf reads the scope out of this workspace's manifests, taking the first
// harness that records one — the same rule the delivery gate's commands follow,
// and for the same reason: the scope is a property of the project, not of the
// tool it is being edited from.
func scopeOf(root string) ([]string, error) {
	for _, h := range workspace.Harnesses(root) {
		m, found, err := manifest.Load(root, h)
		if err != nil {
			return nil, err
		}
		if found && len(m.Codegraph) > 0 {
			return m.Codegraph, nil
		}
	}
	return nil, nil
}

// saveScope records the scope in every harness manifest this workspace has.
func saveScope(root string, scope []string) ([]string, error) {
	all := workspace.Harnesses(root)
	if len(all) == 0 {
		return nil, fmt.Errorf("%s is not an scc workspace", root)
	}
	var wrote []string
	for _, h := range all {
		m, found, err := manifest.Load(root, h)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		m.Codegraph = scope
		if err := manifest.Save(root, h, m); err != nil {
			return nil, err
		}
		wrote = append(wrote, h.Manifest(root))
	}
	return wrote, nil
}

// manifestName is the file a message about the scope points at.
func manifestName(root string) string {
	all := workspace.Harnesses(root)
	if len(all) == 0 {
		return "the manifest"
	}
	return workspace.Relative(mustCwd(), all[0].Manifest(root))
}

// runGraphScope is `scc graph scope`: show it, set it, or clear it.
//
// A command rather than an instruction to edit JSON, for the reason `scc check
// set` is one: a format nobody can be made to type is a format that decays, and
// this one has a normalization step — a trailing `/*` stripped, a glob expanded —
// that a person editing the file by hand would not perform and would then be
// surprised by.
func runGraphScope(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "set":
			return runGraphScopeSet(args[1:])
		case "clear", "unset":
			return runGraphScopeSet(append([]string{"--clear"}, args[1:]...))
		case "show":
			args = args[1:]
		}
	}
	fs := flag.NewFlagSet("graph scope", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return exitFor(err)
	}
	if !noPositionals(rest, "graph scope") {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	scope, err := scopeOf(target)
	if err != nil {
		render.Err(fmt.Sprintf("graph scope: %v", err))
		return ExitError
	}
	roots, missing := codegraph.Roots(target, scope)

	if *jsonOut {
		return emitJSON(struct {
			Scope   []string         `json:"scope"`
			Roots   []codegraph.Root `json:"roots"`
			Missing []string         `json:"missing,omitempty"`
			Scoped  bool             `json:"scoped"`
		}{orEmpty(scope), roots, missing, codegraph.Scoped(roots)})
	}
	if len(scope) == 0 {
		render.Info("no scope: the whole workspace is one graph")
		render.Detail(fmt.Sprintf("  narrow it with: %s graph scope set backend/src frontend/src", prog()))
		return ExitOK
	}
	for _, r := range roots {
		state := "no graph yet"
		if r.Indexed() {
			state = "indexed"
		}
		render.OK(fmt.Sprintf("%-28s %s", r.Rel, state))
	}
	for _, m := range missing {
		render.Warn(fmt.Sprintf("%-28s matches nothing here", m))
	}
	// Said once, wherever somebody meets the scope: N directories is N graphs, and
	// that is why every other command answers in sections.
	render.Info(fmt.Sprintf("%d graph(s) — CodeGraph indexes one root at a time, so a scope is one graph per directory", len(roots)))
	return ExitOK
}

func runGraphScopeSet(args []string) int {
	fs := flag.NewFlagSet("graph scope set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	clear := fs.Bool("clear", false, "record no scope, so the whole workspace is one graph again")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return exitFor(err)
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	if *clear {
		if len(rest) > 0 {
			render.Err("graph scope: --clear takes no directories")
			return ExitError
		}
		return writeScope(target, nil, *jsonOut)
	}
	if len(rest) == 0 {
		render.Err(fmt.Sprintf("graph scope set: name the directories to index, e.g. `%s graph scope set backend/src frontend/src`", prog()))
		render.Detail(fmt.Sprintf("  or `%s graph scope clear` to go back to one graph for the whole workspace", prog()))
		return ExitError
	}
	// Resolved before it is written, so a typo is an error now rather than an
	// empty index later. Every pattern has to name something: a scope is small and
	// hand-written, and a silent miss in it is a tree nobody notices is unindexed.
	roots, missing := codegraph.Roots(target, rest)
	if len(missing) > 0 {
		render.Err(fmt.Sprintf("graph scope set: %s matches no directory here", strings.Join(missing, ", ")))
		render.Detail("  paths are relative to the workspace root; a trailing /* is fine and means the same thing")
		return ExitError
	}
	if !*jsonOut {
		for _, r := range roots {
			render.OK(r.Rel)
		}
	}
	return writeScope(target, rest, *jsonOut)
}

func writeScope(root string, scope []string, jsonOut bool) int {
	wrote, err := saveScope(root, scope)
	if err != nil {
		render.Err(fmt.Sprintf("graph scope: %v", err))
		return ExitError
	}
	if jsonOut {
		return emitJSON(struct {
			Scope []string `json:"scope"`
			Files []string `json:"files"`
		}{orEmpty(scope), relAll(root, wrote)})
	}
	said := fmt.Sprintf("scope: %s", strings.Join(scope, ", "))
	if len(scope) == 0 {
		said = "scope cleared — the whole workspace is one graph"
	}
	for _, f := range wrote {
		render.Info(fmt.Sprintf("%s — %s", workspace.Relative(mustCwd(), f), said))
	}
	return ExitOK
}

// orEmpty keeps a JSON consumer from having to tell null from []: the field is a
// list of directories, and an empty scope is still a list.
func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// graphFan runs one CodeGraph command in every root of the scope.
//
// The command is the same for all of them: every CodeGraph command discovers its
// project from the working directory, which is what lets the argument vectors in
// internal/codegraph stay unaware that scoping exists.
//
// Sections are labelled only when there is more than one root, so an unscoped
// workspace — still the common case — reads exactly as it did before.
func graphFan(bin string, roots []codegraph.Root, args []string) int {
	code := ExitOK
	for _, r := range roots {
		if len(roots) > 1 {
			render.Info(fmt.Sprintf("── %s ──", r.Rel))
		}
		if c := graphExec(bin, r.Dir, args); c != ExitOK {
			code = c
		}
	}
	return code
}

// graphFanJSON is the same fan-out for a machine reader: one document per root,
// in an array, each tagged with the directory it came from.
//
// The per-root output is passed through as raw JSON rather than parsed. scc does
// not know CodeGraph's schema and has no business learning it — nesting a
// document is not reading one. A root whose output is not JSON after all comes
// back as a string, so a CodeGraph that prints a warning above its document
// degrades to something a consumer can still read instead of breaking the whole
// array.
func graphFanJSON(bin string, roots []codegraph.Root, args []string) int {
	type section struct {
		Path   string          `json:"path"`
		Output json.RawMessage `json:"output,omitempty"`
		Raw    string          `json:"raw,omitempty"`
		Exit   int             `json:"exit"`
	}
	out := make([]section, 0, len(roots))
	code := ExitOK
	for _, r := range roots {
		var buf bytes.Buffer
		c := graphCapture(bin, r.Dir, args, &buf)
		if c != ExitOK {
			code = c
		}
		s := section{Path: r.Rel, Exit: c}
		if trimmed := bytes.TrimSpace(buf.Bytes()); json.Valid(trimmed) && len(trimmed) > 0 {
			s.Output = append(json.RawMessage(nil), trimmed...)
		} else {
			s.Raw = buf.String()
		}
		out = append(out, s)
	}
	if c := emitJSON(out); c != ExitOK {
		return c
	}
	return code
}

// graphCapture is graphExec with stdout collected instead of streamed, which is
// what the JSON fan-out needs and nothing else does. A package var for the same
// reason graphExec is one: the tests drive this surface without CodeGraph
// installed.
var graphCapture = func(bin, dir string, args []string, out *bytes.Buffer) int {
	code, err := codegraph.Run(bin, dir, args, out, os.Stderr)
	if err != nil {
		render.Err(fmt.Sprintf("could not run %s: %v", bin, err))
		return ExitError
	}
	if code != 0 {
		return ExitError
	}
	return ExitOK
}

// requireGraphs refuses when any root in the scope has no graph yet, and names
// the ones that do not.
//
// Any rather than all: a scoped workspace whose second tree was never indexed
// would otherwise answer every question from the first alone and look complete
// while being half blind, which is the failure mode a scope makes possible and an
// unscoped workspace never had.
func requireGraphs(roots []codegraph.Root) bool {
	var missing []string
	for _, r := range roots {
		if !r.Indexed() {
			missing = append(missing, r.Rel)
		}
	}
	if len(missing) == 0 {
		return true
	}
	render.Err(fmt.Sprintf("no graph in %s", strings.Join(missing, ", ")))
	render.Detail(fmt.Sprintf("  build it with: %s graph build", prog()))
	return false
}
