// Package paths centralizes the on-disk layout of an scc workspace. Every
// directory and file name the CLI reads or writes is defined here so the layout
// lives in exactly one place — never hardcode ".claude" or "specs" elsewhere.
//
// The layout is Claude Code-native: no conversion layer, no parallel config
// format, and no directory of its own. Everything scc keeps lives under
// .claude/ — Claude Code's own configuration directory — alongside the agents,
// skills, and commands Claude Code already reads from there.
//
// Every segment the product governs is defined here now: Claude Code's own
// directories, scc's manifest, the rules directory the scaffolded CLAUDE.md
// points at, the two work trees (specs/, plans/), and the knowledge base under
// docs/.
package paths

import "path/filepath"

// ClaudeDir is Claude Code's own configuration directory at the project root.
// scc writes into it — agents, skills, commands, hooks, and its own two files —
// but it does not own it.
const ClaudeDir = ".claude"

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
// The marker is deliberately this FILE and not .claude/ itself. ~/.claude is
// Claude Code's global configuration directory and exists on every machine that
// runs Claude Code, so an upward walk for the directory resolves the root to
// $HOME for any command run outside a workspace — and every command would then
// read and write the user's global configuration. .claude/ also exists in every
// repo that merely *uses* Claude Code, where scc was never initialized. See
// workspace.Find.
//
// There is no scc config file. scc runs no tests and no linters (see the design
// doc's §5 and §7 — that is the orchestrator's job), so it has nothing to
// configure; a project's test and lint commands belong in a rule under
// .claude/rules/, which is Markdown the orchestrator already reads.
const ManifestSeg = "scc-manifest.json"

// Entry-point files at the project root.
const (
	// EntryFile is the agent entry point. Keeping it small is a product
	// requirement, not a preference: every Claude Code session pays for it in
	// context, so it links out to steering/knowledge rather than inlining them.
	EntryFile = "CLAUDE.md"
)

// Directory segments under .claude/. All but RulesSeg are Claude Code
// conventions.
const (
	AgentsSeg   = "agents"        // subagent definitions (review, security)
	SkillsSeg   = "skills"        // model-invoked skills
	CommandsSeg = "commands"      // slash commands
	HooksSeg    = "hooks"         // deterministic automation scripts
	SettingsSeg = "settings.json" // hooks + permissions

	// RulesSeg holds the methodology, one file per concern. It is where scc's
	// product actually lives: CLAUDE.md stays small and links here, because every
	// session pays for CLAUDE.md in context and a rule only needs reading when it
	// is relevant. Length has a measured cost in accuracy, not just in tokens.
	RulesSeg = "rules"
)

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
	WikiSeg     = "wiki"         // the graph of durable knowledge
	ADRSeg      = "adr"          // architecture decision records, numbered
	RawSeg      = "raw"          // sources dropped in to be processed into the wiki
	CodewikiSeg = "codewiki"     // narrated code, citing [path:start-end]()
	GlossarySeg = "glossary.md"  // one canonical term per concept
	StackSeg    = "stack.md"     // adopted technology; unlisted means undecided
	WikiIndex   = "index.md"     // the wiki's entry point; an unreachable page is an orphan
	WikiLog     = "changelog.md" // what changed in the wiki, and when
)

// Claude returns the .claude/ directory under root.
func Claude(root string) string { return filepath.Join(root, ClaudeDir) }

// Agents returns .claude/agents/.
func Agents(root string) string { return filepath.Join(root, ClaudeDir, AgentsSeg) }

// Skills returns .claude/skills/.
func Skills(root string) string { return filepath.Join(root, ClaudeDir, SkillsSeg) }

// Commands returns .claude/commands/.
func Commands(root string) string { return filepath.Join(root, ClaudeDir, CommandsSeg) }

// Hooks returns .claude/hooks/.
func Hooks(root string) string { return filepath.Join(root, ClaudeDir, HooksSeg) }

// Settings returns .claude/settings.json.
func Settings(root string) string { return filepath.Join(root, ClaudeDir, SettingsSeg) }

// Manifest returns .claude/scc-manifest.json, scc's only file and the workspace
// marker.
func Manifest(root string) string { return filepath.Join(root, ClaudeDir, ManifestSeg) }

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

// Rules returns .claude/rules/.
func Rules(root string) string { return filepath.Join(root, ClaudeDir, RulesSeg) }

// Rule returns .claude/rules/<name>.md. Callers must pass name through
// workspace.SafeName first when it arrives from CLI args.
func Rule(root, name string) string {
	return filepath.Join(root, ClaudeDir, RulesSeg, name+".md")
}

// Docs returns the project-root docs/ knowledge base.
func Docs(root string) string { return filepath.Join(root, DocsSeg) }

// Wiki returns docs/wiki/.
func Wiki(root string) string { return filepath.Join(root, DocsSeg, WikiSeg) }

// ADR returns docs/adr/.
func ADR(root string) string { return filepath.Join(root, DocsSeg, ADRSeg) }

// Raw returns docs/raw/, the drop box for sources awaiting processing. A file
// still sitting here is a wiki finding: it was collected and never read.
func Raw(root string) string { return filepath.Join(root, DocsSeg, RawSeg) }

// Codewiki returns docs/codewiki/.
func Codewiki(root string) string { return filepath.Join(root, DocsSeg, CodewikiSeg) }

// Glossary returns docs/glossary.md.
func Glossary(root string) string { return filepath.Join(root, DocsSeg, GlossarySeg) }

// Stack returns docs/stack.md. Technology absent from it is an open decision,
// never something adopted silently — and because dependency manifests are
// structured data, that rule is checkable without reading any source.
func Stack(root string) string { return filepath.Join(root, DocsSeg, StackSeg) }

// Entry returns the project-root CLAUDE.md.
func Entry(root string) string { return filepath.Join(root, EntryFile) }
