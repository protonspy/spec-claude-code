package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// The UserPromptSubmit stage: what to say when a request arrives, which is almost
// always nothing.
//
// This is ponytail's anti-drift stage, and porting it needed an answer to a
// question ponytail does not have to ask. ponytail re-asserts a disposition, so
// firing on every prompt is the only thing it *can* do. scc's rules are routed by
// moment — a rule fires when its concern goes live — so a stage that fired on
// every prompt would not be re-asserting anything, it would be interrupting.
//
// So it is bounded the hard way: **it speaks only when the prompt names an
// artifact this workspace actually has**, and it says the one thing that is
// cheapest to say at that exact moment — how to read that artifact without
// reading it. Everywhere else it is silent.
//
// That bound is what makes the cost defensible. The stage runs on every prompt of
// every session, which is the worst cost profile of the three; it answers with
// two directory listings and a string scan, no subprocess and no file parsing,
// and on the overwhelming majority of prompts it returns nothing at all.
//
// **The honest caveat, recorded rather than smoothed over:** this was planned as
// a decision to be made on benchmark evidence, and the benchmark was skipped. So
// the design is conservative by construction instead — it can only ever fire on a
// prompt that named something real, which is the narrowest version that still
// does anything. Whether it earns its place is still unmeasured, and
// TestThePromptStageIsSilentUnlessAnArtifactIsNamed is what keeps the bound from
// quietly widening in the meantime.

// promptNameFloor is the shortest artifact name that may be matched in prose.
//
// Short names are the whole risk here. A spec called `ui` would fire on "build
// the ui", on "guide", and on anything else containing those two letters even
// with word boundaries enforced — and a stage that fires wrongly on ordinary
// English is worse than one that never fires. Anything shorter than this is
// reachable by its path (`specs/ui/`), which is unambiguous because nobody types
// it by accident.
const promptNameFloor = 4

// promptContext is what the arriving request has to say for itself, or nothing.
func promptContext(root, prompt string) []string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || !workspace.IsWorkspace(root) {
		return nil
	}

	named := namedArtifacts(root, prompt)
	if len(named) == 0 {
		return nil
	}

	out := []string{
		"scc: the request names " + strings.Join(quoted(named), ", ") +
			". Read the artifact through the map, not with Read — the file is far larger than the answer:",
	}
	for _, a := range named {
		out = append(out, "  "+a.how())
	}
	return out
}

// artifactRef is one artifact a prompt named, and how to open it cheaply.
type artifactRef struct {
	// Name is what the prompt matched, and what the reader recognizes.
	Name string
	// Addr is what `scc map` takes.
	Addr string
	// Spec says which reading surface applies: a spec has phases, a plan has a
	// header and a checklist.
	Spec bool
}

func (a artifactRef) how() string {
	if a.Spec {
		return fmt.Sprintf("%s — `%s map outline %s` for the shape, `%s map show %s#<section>` for one part",
			a.Name, prog(), a.Addr, prog(), a.Addr)
	}
	return fmt.Sprintf("%s — `%s map brief %s` is the header, `%s map tasks %s --next` is the task to do",
		a.Name, prog(), a.Addr, prog(), a.Addr)
}

// namedArtifacts is every artifact this workspace has that the prompt mentions.
//
// Read from the directory rather than from an index, because the whole stage has
// to cost less than the latency a person would notice: two ReadDir calls on
// directories that hold tens of entries, and no file is opened. An artifact
// created since the session started is found, which an index built at
// SessionStart would have missed.
func namedArtifacts(root, prompt string) []artifactRef {
	lower := strings.ToLower(prompt)
	var out []artifactRef

	for _, feature := range childDirs(paths.Specs(root)) {
		if mentions(lower, feature, paths.SpecsSeg) {
			out = append(out, artifactRef{Name: feature, Addr: paths.SpecsSeg + "/" + feature, Spec: true})
		}
	}
	for _, plan := range planNames(root) {
		if mentions(lower, plan, paths.PlansSeg) {
			out = append(out, artifactRef{Name: plan, Addr: paths.PlansSeg + "/" + plan + ".md"})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Addr < out[j].Addr })
	// Capped, because this is context paid for on a turn that has not started
	// yet. A prompt naming six artifacts is a prompt about the workspace, and the
	// answer to that one is `scc map index` rather than six rows.
	const most = 3
	if len(out) > most {
		return out[:most]
	}
	return out
}

// mentions reports whether the prompt names this artifact.
//
// Two ways, and the second is what makes short names safe. The bare name counts
// only at a word boundary and only past promptNameFloor, so a spec called `api`
// cannot fire on "rapid". The path form — `specs/api` — counts at any length,
// because nobody types that by accident.
func mentions(lowerPrompt, name, seg string) bool {
	if strings.Contains(lowerPrompt, strings.ToLower(seg+"/"+name)) {
		return true
	}
	if len(name) < promptNameFloor {
		return false
	}
	return containsWord(lowerPrompt, strings.ToLower(name))
}

// containsWord finds a name only where it stands as a word.
//
// Hyphens and dots are *inside* a name rather than boundaries around it, because
// artifact names are kebab-case: treating `-` as a boundary would make `auth`
// match the spec `auth-flow`, and then naming one artifact would cite two.
func containsWord(haystack, needle string) bool {
	for at := 0; ; {
		i := strings.Index(haystack[at:], needle)
		if i < 0 {
			return false
		}
		i += at
		if nameBoundary(haystack, i-1) && nameBoundary(haystack, i+len(needle)) {
			return true
		}
		at = i + 1
	}
}

func nameBoundary(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return true
	}
	c := s[i]
	return !(c == '-' || c == '.' || c == '_' ||
		c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z')
}

// childDirs is the directory names directly under a path, or none.
func childDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

// planNames is every plan in the workspace, without its extension.
//
// ReadDir and skip directories, the same way the plan scanner does — `migrate`
// puts archived notes in `plans/archive/`, and a directory read as a plan would
// be a plan nothing can open.
func planNames(root string) []string {
	entries, err := os.ReadDir(paths.Plans(root))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
	}
	return out
}

func quoted(all []artifactRef) []string {
	out := make([]string, 0, len(all))
	for _, a := range all {
		out = append(out, "`"+a.Name+"`")
	}
	return out
}

// A requirement id is deliberately *not* a trigger, though it is the most
// artifact-shaped thing a prompt can carry. `R1.2` says which requirement without
// saying which spec, and a real workspace has that id defined in nine of them —
// so firing on it would either pick one at random or print the list, and the
// list is what `scc map trace` already returns, better, when actually asked.
