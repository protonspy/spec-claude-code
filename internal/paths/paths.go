// Package paths centralizes the on-disk layout of an scc workspace. Every
// directory and file name the CLI reads or writes is defined here so the layout
// lives in exactly one place — never hardcode ".claude" or "specs" elsewhere.
//
// The layout is harness-native: no conversion layer, no parallel config format,
// and no directory of scc's own. Everything scc keeps for a harness lives under
// that harness's own configuration directory — .claude/, .codex/, .opencode/ —
// alongside the agents, skills, and commands the harness already reads from
// there. See Harness.
//
// The work trees are the opposite: specs/, plans/, and docs/ are the same in
// every workspace, because they are the product, not the tool. Nothing about a
// requirement or a wiki page changes with the agent reading it, and a repo that
// swapped harnesses would otherwise have to move its own knowledge base.
package paths

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ClaudeDir is Claude Code's own configuration directory at the project root.
// scc writes into it — agents, skills, commands, hooks, and its own two files —
// but it does not own it.
const ClaudeDir = ".claude"

// The other harnesses' configuration directories, on the same terms.
const (
	CodexDir    = ".codex"
	OpenCodeDir = ".opencode"
)

// Harness is one agent harness's on-disk conventions — the whole of what scc has
// to know to scaffold into it.
//
// It exists because the methodology is not Claude Code's. The rules, the review
// agents, and the knowledge-base skills are Markdown that says how to work; what
// differs between harnesses is only where each file goes, what the entry file is
// called, and what dialect of frontmatter the loader parses. Encoding that as
// data — rather than as a second copy of the template set per harness — is what
// keeps one prose source honest across all three.
type Harness struct {
	// ID is the name on the command line and in the manifest: "claude",
	// "codex", "opencode".
	ID string

	// Label is the tool's own name, for anything a person reads. The IDs are
	// flags and JSON; "Claude Code" is what the user actually installed.
	Label string

	// Bin is the harness's own executable, as it appears on PATH — what
	// `scc launch` starts.
	//
	// The one field here that is not about the on-disk layout, and it belongs
	// here anyway: scaffolding for a harness and then starting that harness in
	// the workspace it just configured is one profile's question asked twice. A
	// launcher switching on ID instead would be this profile missing a field.
	Bin string

	// Dir is the harness's configuration directory at the project root.
	Dir string

	// EntryFile is the instruction file the harness loads on its own, at the
	// project root. Claude Code reads CLAUDE.md; Codex and opencode both read
	// AGENTS.md, which is the same idea under the name the wider ecosystem
	// standardized on.
	EntryFile string

	// AgentsSeg is where subagent definitions go under Dir. opencode's loader
	// accepts agent/ and agents/ both; Claude Code and Codex each accept one.
	AgentsSeg string

	// AgentFormat is the dialect a subagent definition is written in:
	// FormatMarkdown for the YAML-frontmatter files Claude Code and opencode
	// read, FormatTOML for Codex's agent role files.
	AgentFormat Format

	// SkillsSeg is where SKILL.md directories go under Dir. All three harnesses
	// implement the same Agent Skills format, which is why `scc validate` can
	// check skills without knowing which harness wrote them.
	SkillsSeg string

	// CommandsSeg is where slash commands go under Dir, or "" when the harness
	// has no project-level command surface. Codex is the "" case: its custom
	// prompts are user-global and deprecated in favor of skills, so scc ships
	// the skills alone there rather than writing into the user's home directory.
	CommandsSeg string

	// RulesSeg is where the methodology goes under Dir. The path is scc's choice
	// in all three, kept parallel so one layout is learned once — but what the
	// harness then does with it is not, which is what PreloadsRules records.
	RulesSeg string

	// PreloadsRules is whether the harness reads every file under RulesSeg into
	// context by itself at session start.
	//
	// It changes what the entry file may truthfully say, which is why it is a
	// field rather than a footnote. Claude Code loads `.claude/rules/*.md` at
	// launch with the same priority as CLAUDE.md, so an entry file telling that
	// agent to "read the rule when the concern is live" describes a mechanism
	// that already ran — it asks for a re-read that puts the same bytes in
	// context twice, and it makes the one document the agent is meant to trust
	// wrong about its own environment. For Codex and opencode, RulesSeg is scc's
	// invention and nothing loads it, so there the instruction is the only thing
	// that gets a rule read at all.
	//
	// Verifiable per harness: `/context` in Claude Code lists what loaded.
	PreloadsRules bool
}

// Format is the dialect a harness's subagent definitions are written in.
type Format string

const (
	FormatMarkdown Format = "md"
	FormatTOML     Format = "toml"
)

// AgentExt is the file extension an agent definition takes in this harness.
func (h Harness) AgentExt() string {
	if h.AgentFormat == FormatTOML {
		return ".toml"
	}
	return ".md"
}

// The harnesses scc can scaffold into.
//
// Claude Code is the default and the reference implementation: it is where the
// methodology was designed, and the one whose subagent, skill, and slash-command
// surfaces all exist at project scope.
var (
	Claude = Harness{
		ID: "claude", Label: "Claude Code", Bin: "claude", Dir: ClaudeDir, EntryFile: "CLAUDE.md",
		AgentsSeg: "agents", AgentFormat: FormatMarkdown,
		SkillsSeg: "skills", CommandsSeg: "commands", RulesSeg: "rules",
		PreloadsRules: true,
	}
	Codex = Harness{
		ID: "codex", Label: "Codex", Bin: "codex", Dir: CodexDir, EntryFile: "AGENTS.md",
		AgentsSeg: "agents", AgentFormat: FormatTOML,
		SkillsSeg: "skills", CommandsSeg: "", RulesSeg: "rules",
	}
	OpenCode = Harness{
		ID: "opencode", Label: "opencode", Bin: "opencode", Dir: OpenCodeDir, EntryFile: "AGENTS.md",
		AgentsSeg: "agent", AgentFormat: FormatMarkdown,
		SkillsSeg: "skills", CommandsSeg: "command", RulesSeg: "rules",
	}
)

// Harnesses returns every supported harness, in a fixed order. The order is the
// probe order for the workspace marker, so it is deliberate rather than
// alphabetical: Claude Code first because it is the default.
func Harnesses() []Harness { return []Harness{Claude, Codex, OpenCode} }

// ParseHarness resolves an ID from the command line.
func ParseHarness(id string) (Harness, error) {
	for _, h := range Harnesses() {
		if h.ID == id {
			return h, nil
		}
	}
	return Harness{}, fmt.Errorf("unknown harness %q: expected one of %s", id, HarnessIDs())
}

// HarnessIDs lists the IDs for help text and errors.
func HarnessIDs() string {
	ids := make([]string, 0, len(Harnesses()))
	for _, h := range Harnesses() {
		ids = append(ids, h.ID)
	}
	sort.Strings(ids)
	return strings.Join(ids, ", ")
}

// ManifestSeg is scc's only file, and it does two jobs.
//
// It records, per scc-managed file, a content hash and the template version that
// produced it. The hash answers "did the user edit this?"; the version answers
// "what did it look like before?" — and an upgrade needs both, because it applies
// a three-way merge (re-render old, re-render new, diff, merge) rather than
// dropping a backup file. A manifest carrying only hashes cannot support that.
//
// Its presence is also the workspace marker: only scc writes it, so it exists
// exactly when scc has scaffolded this directory — the question a marker answers.
//
// The marker is deliberately this FILE and not the harness directory. Every one
// of the three has a global twin in the user's home — ~/.claude, ~/.codex,
// ~/.config/opencode — and exists on every machine that runs that tool, so an
// upward walk accepting the directory would resolve the root to $HOME for any
// command run outside a workspace, and every command would then read and write
// the user's global configuration. The directories also exist in every repo that
// merely *uses* the harness, where scc was never initialized. Only scc writes
// this file. See workspace.Find.
//
// One manifest per harness, in that harness's own directory. A workspace
// scaffolded for two harnesses is two managed trees that happen to share a repo,
// and each has to be upgradable on its own.
//
// There is no scc config file. scc runs no tests and no linters (see the design
// doc's §5 and §7 — that is the orchestrator's job), so it has nothing to
// configure; a project's test and lint commands belong in a rule under the
// harness's rules directory, which is Markdown the orchestrator already reads.
const ManifestSeg = "scc-manifest.json"

// Project-root directory segments that scc governs. The split is strict and each
// name means exactly one thing: specs/ holds features, plans/ holds initiatives,
// docs/ is the knowledge base. csdd kept plans under docs/plans/; a plan is work,
// not knowledge, so it moves out.
const (
	SpecsSeg = "specs" // one directory per feature
	PlansSeg = "plans" // one file per initiative
	DocsSeg  = "docs"  // the knowledge base
)

// The three artifacts of a spec, directly under specs/<feature>/. There is no
// intermediate route directory: a plan lives in plans/ and the Tasks route writes
// nothing at all, so nothing would be distinguished by one.
const (
	RequirementsSeg = "requirements.md"
	DesignSeg       = "design.md"
	TasksSeg        = "tasks.md"
)

// The knowledge base under docs/. It answers *why* — the durable reasoning, the
// decisions, the material read from outside — while a spec answers what one
// feature does now. Neither replaces the other.
const (
	WikiSeg     = "wiki"     // the graph of durable knowledge
	ADRSeg      = "adr"      // architecture decision records, numbered
	RawSeg      = "raw"      // sources dropped in to be processed into the wiki
	CodewikiSeg = "codewiki" // narrated code, citing [path:start-end]()
	GlossarySeg = "glossary.md"
	StackSeg    = "stack.md" // adopted technology; unlisted means undecided
	// NotesSeg is where a small durable observation goes: the gotcha, the why-not,
	// the "careful here" that used to be a comment only the reader of that one file
	// ever saw. One note per line, so a grep returns whole notes rather than
	// fragments — which is what lets an agent query it without loading it.
	NotesSeg = "notes.md"
	// WikiPagesSeg holds the pages themselves, so the wiki's two fixed documents are
	// distinguished from its content by where they sit rather than by their names.
	// The validator used to exclude index.md and changelog.md from the page set by
	// matching those names, which made any other file dropped into wiki/ a page —
	// and usually then an orphan finding for a file nobody meant as a page.
	WikiPagesSeg = "pages"
	WikiIndex    = "index.md"     // the wiki's entry point; an unreachable page is an orphan
	WikiLog      = "changelog.md" // what changed in the wiki, and when
)

// Config returns the harness's configuration directory under root.
func (h Harness) Config(root string) string { return filepath.Join(root, h.Dir) }

// Agents returns the harness's subagent directory.
func (h Harness) Agents(root string) string { return filepath.Join(root, h.Dir, h.AgentsSeg) }

// Skills returns the harness's skills directory.
func (h Harness) Skills(root string) string { return filepath.Join(root, h.Dir, h.SkillsSeg) }

// Commands returns the harness's slash-command directory, or "" when it has no
// project-level command surface. A caller that joins onto "" would write into
// the configuration directory itself, so check before using it.
func (h Harness) Commands(root string) string {
	if h.CommandsSeg == "" {
		return ""
	}
	return filepath.Join(root, h.Dir, h.CommandsSeg)
}

// Rules returns the harness's rules directory.
func (h Harness) Rules(root string) string { return filepath.Join(root, h.Dir, h.RulesSeg) }

// Rule returns <rules>/<name>.md. Callers must pass name through
// workspace.SafeName first when it arrives from CLI args.
func (h Harness) Rule(root, name string) string {
	return filepath.Join(root, h.Dir, h.RulesSeg, name+".md")
}

// Entry returns the harness's project-root instruction file.
func (h Harness) Entry(root string) string { return filepath.Join(root, h.EntryFile) }

// Manifest returns <config>/scc-manifest.json, scc's only file and the
// workspace marker for this harness.
func (h Harness) Manifest(root string) string {
	return filepath.Join(root, h.Dir, ManifestSeg)
}

// SkillDirs returns every harness's skills directory under root, in Harnesses
// order. `scc validate` checks skills wherever they are, because the Agent
// Skills format is one standard and a workspace scaffolded for two harnesses
// has two copies of it to keep conforming.
func SkillDirs(root string) []string {
	dirs := make([]string, 0, len(Harnesses()))
	for _, h := range Harnesses() {
		dirs = append(dirs, h.Skills(root))
	}
	return dirs
}

// Specs returns the project-root specs/ directory.
func Specs(root string) string { return filepath.Join(root, SpecsSeg) }

// Spec returns specs/<feature>/. Callers must pass feature through
// workspace.SafeName before this — the name arrives from CLI args, and without
// that check `..` resolves to the workspace root.
func Spec(root, feature string) string { return filepath.Join(root, SpecsSeg, feature) }

// Requirements returns specs/<feature>/requirements.md.
func Requirements(root, feature string) string {
	return filepath.Join(root, SpecsSeg, feature, RequirementsSeg)
}

// Design returns specs/<feature>/design.md.
func Design(root, feature string) string {
	return filepath.Join(root, SpecsSeg, feature, DesignSeg)
}

// Tasks returns specs/<feature>/tasks.md.
func Tasks(root, feature string) string {
	return filepath.Join(root, SpecsSeg, feature, TasksSeg)
}

// Plans returns the project-root plans/ directory.
func Plans(root string) string { return filepath.Join(root, PlansSeg) }

// Plan returns plans/<name>.md. A plan is one document: its progress is derived
// from the specs it decomposes into, never tracked in the plan itself, so there
// is no per-plan state to give it a directory.
func Plan(root, name string) string {
	return filepath.Join(root, PlansSeg, name+".md")
}

// Docs returns the project-root docs/ knowledge base.
func Docs(root string) string { return filepath.Join(root, DocsSeg) }

// Wiki returns docs/wiki/.
func Wiki(root string) string { return filepath.Join(root, DocsSeg, WikiSeg) }

// WikiPages returns docs/wiki/pages/, where the pages live.
func WikiPages(root string) string { return filepath.Join(Wiki(root), WikiPagesSeg) }

// ADR returns docs/adr/.
func ADR(root string) string { return filepath.Join(root, DocsSeg, ADRSeg) }

// Raw returns docs/raw/, the drop box for sources awaiting processing. A file
// still sitting here is a wiki finding: it was collected and never read.
func Raw(root string) string { return filepath.Join(root, DocsSeg, RawSeg) }

// Codewiki returns docs/codewiki/.
func Codewiki(root string) string { return filepath.Join(root, DocsSeg, CodewikiSeg) }

// Glossary returns docs/glossary.md.
func Glossary(root string) string { return filepath.Join(root, DocsSeg, GlossarySeg) }

// Notes returns docs/notes.md, the project's note log. A missing file is the
// normal state of a young workspace, not an error: `scc notes add` creates it, and
// every reader treats absent as an empty log.
func Notes(root string) string { return filepath.Join(root, DocsSeg, NotesSeg) }

// Stack returns docs/stack.md. Technology absent from it is an open decision,
// never something adopted silently — and because dependency manifests are
// structured data, that rule is checkable without reading any source.
func Stack(root string) string { return filepath.Join(root, DocsSeg, StackSeg) }
