// Package assets is the template set compiled into the binary: the rules, the
// review agents, the knowledge-base skills and their commands, and the artifact
// templates, all of it plain Markdown embedded with //go:embed.
//
// There are two kinds of template here and the difference matters:
//
//   - **Workspace files** — what `scc init` writes and the manifest tracks. These
//     are DATA-FREE except for the harness: no project name, no date, no path
//     from the machine is interpolated into any of them, and the only value a
//     template can reach is the profile in internal/paths that says where this
//     harness keeps its files. That is what makes an upgrade possible without
//     storing rendered text anywhere. A given (template version, harness) pair
//     renders byte-identically in every workspace on earth, so its content hash
//     identifies it globally and the three-way merge can reconstruct the old side
//     from the version and the harness alone — both of which the manifest
//     records. The two files that genuinely want per-project content — the entry
//     file and the project rule — are exactly the files an upgrade excludes
//     because the user owns them, so the two rules agree instead of fighting.
//
//   - **Artifact templates** — what `spec new` and `plan new` render. These take
//     data, because they are authored from birth: the user owns the result
//     immediately, nothing tracks them in the manifest, and no upgrade ever touches
//     them.
//
//   - **Seeds** — the `docs/` anchors init lays down. Untracked like an artifact and
//     data-free like a workspace file: they are the knowledge base's fixed
//     documents, so scc writes each one once, holding the format its validator
//     checks, and never writes it again. See Seed.
//
// One prose source, three harnesses. The methodology does not change with the
// tool reading it, so the rules, the skills, and the review agents are written
// once and re-addressed per harness: paths come from the profile, and the
// frontmatter each loader parses is synthesized at render time. Shipping a copy
// of the template set per harness would guarantee they drift, and the drift would
// be silent.
package assets

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"text/template"

	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/textutil"
)

// Version is the template-set version, recorded per entry in the manifest and
// bumped whenever any workspace template's content changes.
//
// It is not the binary's version. Two scc releases that ship identical templates
// share a template version, which keeps `scc update` a no-op across them instead
// of touching every managed file to record a number nobody read.
//
// 2: the knowledge-base skills and their commands.
// 3: CLAUDE.md documents the npx fallback for running scc uninstalled.
// 4: the review agents pin a model and effort, run gates instead of reading, and
// report in a fixed shape the orchestrator can act on.
// 5: one template set for three harnesses — Claude Code, Codex, opencode.
// 6: the knowledge base states the stack check without naming an ecosystem.
// 7: the plan-run workflow skill and its command — running a whole plan, group by
// group, is a procedure with an entry point rather than a rule read in passing.
// 8: the entry file names *when* to read each rule instead of tabulating all nine
// as equals — five read on their own trigger, four looked up by name — and the two
// review agents are tightened in the same pass.
// 9: the entry file gives
// each rule its own trigger line instead of running four of them together in a
// sentence — project.md above all, since a build command nobody read is guessed —
// and it stops telling a harness that preloads the rules to go and read them.
// 11: artifacts.md — plans and specs are addressable, so `scc map` answers a
// structural question and `scc patch` edits by address, and neither one costs the
// file. The entry file carries the reflex; the rule carries the how.
// 12: the entry file's layout block is one column across all three harnesses. The
// padding is computed from the profile rather than written into the template,
// because a run of spaces that lines up for `.codex/` is ragged for `.opencode/`.
// 13: caveman.md — the output budget belongs to the code, and narration is the part of
// a long run that can be cut without losing a fact. A rule rather than a skill because
// it is on by default, and a default the model has to choose to load is not one. One
// level (ultra) rather than three, because a dial nobody turns is three descriptions of
// the register to keep true instead of one. The language it answers in is the third
// kickoff question in autonomy.md, recorded as `lang:` beside the two that were already
// there — the register is a decision about the whole run, so it belongs where the run's
// other two decisions are and not in a preference asked again every session. In the
// same pass, three places a real workspace showed scc stating a mechanism exactly and
// leaving the judgment unsaid — where an agent fills the gap with the cheapest
// reading. The plan-run command said "read the plan" where the skill it invokes says
// map it; the review agents asked for surrounding code without ever saying the diff
// already held it; and the wiki skill explained how a slug resolves at the moment the
// page is being named, without saying the name has to name the concept.
// Also: the wiki's pages move to docs/wiki/pages/, so index.md and changelog.md are
// told apart from content by where they sit rather than by their names — which is what
// stopped any other .md dropped into wiki/ from becoming a page, and then an orphan.
// 14: the plan is a contract rather than a document. Its sections are closed — a
// header and a checklist, and nowhere for prose to grow, which is the only thing that
// ever capped a plan's size; `## Decomposition` becomes `## References`, since the
// parser recognizes a leaf by the citation and never by the heading above it. A task
// gains four flags and no more (`_Depends_`, `_Priority_`, `_Status removed_`,
// `_Reason_`), so "what do I work on next" has a determined answer instead of a
// file-order one. And the rules stop offering to read the plan at all: `map brief`
// once plus `map tasks --next` per task is the whole reading surface, which is what
// gives "never read the plan" the authority to be a rule.
// 15: delivery is a branch in the checkout you are in, and the worktree is gone from
// the procedure. It was there for one reason — several sessions at once on one repo —
// and that is the user's setup to make, not a step every single-session run pays for:
// a directory to create, one to switch to, and one to remember to remove, with a
// checkout left behind whenever the run dies before the last step. So `plan-run` asks
// three questions instead of four, `worktree:` stops being a frontmatter answer, and
// what survives is the one line the worktree was really carrying — leave the checkout
// back on `main` and clean, because that is where the next unit of work starts.
// 16: prior-art.md — the knowledge base is read before the first artifact is written,
// not consulted once someone is stuck. `docs/` is the constraint set: an ADR binds the
// design, `stack.md` says what may be built on, `glossary.md` says what to call it. It
// is a rule of its own rather than a paragraph in knowledge-base.md because the two
// halves fire at opposite moments — that one is triggered by having learned something,
// this one by being about to write — and a read-side instruction filed under the
// write-side rule is read after the spec exists, which is after it was any use. It
// also has to state the thing no index states: `scc map` covers `plans/` and `specs/`
// and the symbol graph covers code, so `docs/` is the one corpus reached by opening a
// file, and the anchors are what keep that cheap.
// 17: the `init` skill and `/scc-init` — the knowledge base is bootstrapped from a
// repository that already exists. It is the counterpart to the CLI command of the same
// name, which lays the four anchors down empty and can do nothing else: what fills them
// is a survey of the code, and a survey is judgment. The skill holds the order across
// the six knowledge authors (checkable first, interpretive last, `scc validate` between
// stages) and one bar the authors cannot state for themselves — everything written on
// this run is reconstructed rather than remembered, so nothing goes in that cannot be
// pointed at, and what nobody can justify is reported by name instead of filled in. The
// ADRs are last and strictest for that reason, and each says in its `## Context` that it
// was reconstructed after the fact.
// 18: delivery.md stops naming a second-directory setup at all. Version 15 took it out
// of the procedure and left the rule pointing at one anyway, which is the same cost in
// a smaller font: a line preloaded into every request, describing a step nothing in the
// flow performs. How a user arranges several sessions against one repo is theirs, and
// saying so is enough — the rule keeps only what it was ever really carrying, which is
// to leave the checkout back on `main` and clean.
// 19: notes.md — the small durable observation gets one place to live, and the code
// stops being it. A comment says what a thing is and how to use it; the gotcha, the
// why-not and the "careful, this is not what it looks like" go to docs/notes.md as
// one line each, carrying the path they are about. One line is the whole design: a
// match is a whole note, so grep and `scc notes find` answer the same question, and
// the file that centralizes every note is never a file anybody has to read. It is a
// rule of its own because its trigger is a keystroke — you are about to type a
// comment that is not a docstring — and prior-art.md gains the read side, since this
// is now the one corpus under docs/ that does have an index.
// 20: delivery.md — the commit and the PR carry no attribution. No `Co-Authored-By`
// for an assistant, no session link, no generated-with footer, no naming of a model,
// vendor or harness, in a message or a PR body or a branch name. The work is the
// user's: a tool that signs what it did for somebody is claiming a share of it, and
// the honest signature is the diff. It lands in delivery.md at no cost to the budget —
// the opening paragraph's fenced one-liner goes inline, which pays for the paragraph
// exactly.
// 21: delivery.md — the spec records where it is being built. A branch was the one
// part of this methodology that left no trace in the artifacts: the spec said which
// boxes were ticked and git said a branch had been unmerged for three weeks, and
// nothing joined the two, so "which of these actually shipped" was answerable only by
// somebody holding both halves — which under `autonomy: auto` is nobody. `scc spec
// track --here` records the branch and `--pr` the pull request; `scc spec sync` reads
// git and the forge back into every spec. The rule pays for the paragraph by
// tightening §8's argument, which the design doc holds in full anyway.
// 24: delivery.md — the tests become a number. "Full suite + lint" was the one step
// in the sequence that nothing could check: a rule can ask for a green suite, and
// under `autonomy: auto` nobody reads the answer. `scc test` runs the command this
// workspace recorded and reads `{"total": N, "coverage": P}` back out of it, so the
// claim is arithmetic against a floor rather than a sentence. Step 1 and step 2 name
// it, and pre-push runs it, which is the moment a branch becomes a pull request. The
// paragraph is paid for by tightening the sequential-implementation argument and the
// review step — the reasons stay, the repetition goes.
// 25: delivery.md, project.md, the init skill — the gate is four commands, not one.
// Build, format and lint join test, run in that order and stopping at the first
// failure, because a lint report over a broken build is derived noise. The step that
// made it worth generalizing is `skipped`: a language with no formatter is a real
// answer, recorded once, and a gate left unrecorded is a decision nobody has made —
// two states the rule has to keep apart or it nags one project and goes quiet about
// the other. The skill gains the per-language table and the warning that a format
// gate is the check and never the rewrite.
//
// project.md and the init skill move with it, because the command is the agent's to
// write and neither the rule nor the validator can write it: project.md gains the one
// exception to "scc runs none of these", and the init skill gains the stage that
// derives the command from the language the survey just identified. The finding says
// what to do rather than only what is wrong — it is the one finding in the product
// whose fix is a command line rather than an edit to the file it points at.
// 26: code-search.md — the table named three commands that do not exist. `scc graph`
// dispatches build | sync | status | query | scope | explore, and the rule sent the
// agent to `impact`, `callers` and `callees`, every one of which fails on first use.
// That is exactly the failure the CodeGraph usage block is withheld until launch to
// avoid: guidance naming a command the machine cannot run costs the file its
// credibility, and an agent that has watched one line fail discounts the rest of it.
// Relationship questions route to `explore`, the one command that answers them — the
// call paths between the relevant symbols, which is what `impact` was promising — and
// name lookups to `query`, with the flags it actually takes. Four rows before and
// four after, so the budget is untouched.
// 27: ladder.md — a rule for how much code a task gets. Fourteen rules governed which
// vehicle carries the work, how a task is tested, what a finished one owes and what
// gets written down afterwards; none governed the size of the thing being built.
// caveman.md is the nearest neighbour and it says so itself — it is the register the
// answer is written in, not the code. So the ladder: reuse before stdlib before the
// platform before a dependency this project already carries, and two of those rungs
// are a command here rather than a discipline, because `scc graph explore` and
// `docs/stack.md` are already the recorded answer.
//
// Two boundaries are the reason it is safe to ship on by default. The carve-outs —
// trust-boundary validation, data-loss error handling, security, accessibility — are
// the half that separates lazy from careless, and nothing in the set had them. And
// the requirement is not a rung: a spec decides whether a thing exists, so building
// less than R1.2 says is the one failure this rule could cause, and it ships looking
// like a clean diff. A deliberate ceiling is a note with a tag rather than a comment,
// which notes.md had already argued for everything except this case.
// 28: five lines that said something the product does not do. An audit of the set for
// ambiguity, prompted by porting ponytail, and the findings were all of one kind: a
// sentence true of the artifact in front of it, read as a claim about the whole
// product. artifacts.md called References the specs a plan decomposes into, where the
// contract takes ADRs and links too. autonomy.md recorded the kickoff answers in
// "`requirements.md` for a spec" and named no file for a plan, which has the same two
// keys. specs.md said "`scc` never checks that a section is present" while arguing
// against filler headings in a design — true there, false of a plan, where three
// sections are required and the reader who believed it writes an invalid one.
// verification.md said scc "does not own the linter and does not run it", which the
// delivery gate stopped being true of. And ladder.md ran "before the code" without
// saying what else runs before it, leaving two rules claiming the same moment.
//
// None of them is wrong about its own subject, which is why they survived review: a
// rule is read one file at a time and audited the same way. The cost is the same as a
// command that does not exist — the agent does what the line says, once.
//
// 30: the standing cost, measured and cut. What a scaffolded Claude Code workspace
// preloads into every request had grown to 47KB — 15 rules and the entry file — while
// this file's own prose still said nine rules and ~26KB. Nothing about the
// methodology changed here; what changed is when three of its files arrive.
// `specs.md`, `tasks.md` and `knowledge-base.md` carry a `paths:` header, so the
// harness loads them when the agent touches the tree they govern rather than at
// session start: 10.4KB, about a quarter, off every request. The bar for scoping a
// rule is in Rules() and it is mechanical — a validator has to report what the rule
// prevents, so a rule arriving late is a rule whose failure `scc validate` catches
// before the commit. The entry file says which three, because it is the one document
// the agent is meant to trust about its own environment.
//
// The skill descriptions are the other half and were the unmeasured one: a body is
// paid by the run that invokes it, a description by every request whether the skill
// runs or not. Eight came to 4087 characters, mostly enumerating findings the
// validator already names — now 2084, with a budget test so the next one cannot
// quietly restore them.
//
// And the two review agents become `scc-code-review` and `scc-security-review`.
// Claude Code ships its own under the unprefixed names, so `delivery.md` naming them
// in prose resolved to whichever the harness picked — a collision nobody would ever
// see, because both produce a review and the one that ran is the one that answers.
//
// 31: the slash commands come off Claude Code, and the `scc-` prefix moves onto the
// skill. Claude Code registers a skill as `/<name>` by itself and its own
// documentation now says commands and skills are one mechanism — so a command file
// beside each skill was a second entry point in the picker for one thing, and a
// second `description` preloaded into every request whether anybody ran it or not.
// That is the whole cost: ~980 characters of saying twice what the skill says once.
//
// The rename is what makes the removal safe rather than a second collision. With no
// command file the skill's own name is what the user types, and `/init` is a command
// Claude Code already ships — the same failure the review agents had under their
// unprefixed names, one version ago, by the same door. So the skills are `scc-init`,
// `scc-wiki` and the rest, and every reference to `/scc-init` already written keeps
// resolving. opencode still gets command files, because nothing there turns a skill
// into one.
//
// What the command bodies carried and the skills did not is the *default job* — what
// to do when nobody typed an argument, which for five of them is "run `scc validate`
// and clear this artifact's findings". That moved into the skill bodies, where it is
// paid by the run that invokes it instead of by every session; `argument-hint` moved
// with it, since SKILL.md takes the same key.
const Version = "32"

// The embedded tree. "all:" so nothing is silently dropped for having a name the
// default embed pattern skips.
//
//go:embed all:templates
var files embed.FS

const (
	root         = "templates"
	artifactsDir = "artifacts"
)

// Kind says how a template becomes a file in a workspace.
type Kind string

const (
	// Plain is copied through after path expansion: rules, skills, the entry file.
	Plain Kind = "plain"
	// Agent is a subagent definition. Its template carries harness-neutral
	// frontmatter (a name and a description) and the reviewer's prose; the
	// header the harness's loader actually parses is synthesized per harness,
	// because the three disagree on both the dialect and the keys.
	Agent Kind = "agent"
	// Command is a slash command, on the same terms as Agent: shared body,
	// per-harness frontmatter.
	Command Kind = "command"
	// Rule is one file of the methodology. Copied through like Plain, except on a
	// harness that understands a `paths:` header, where a rule carrying one is
	// rendered with it so the harness loads that rule on demand instead of at
	// session start. See Rules for which rules carry one and why.
	Rule Kind = "rule"
)

// rule is one file of the methodology and when the harness should load it.
type rule struct {
	// name is the file under rules/.
	name string
	// scope is the globs this rule governs, or nil for a rule that is always on.
	//
	// A rule with a scope is one a harness may load only when the agent works with
	// a matching file. That is a real change in when the agent has it — it arrives
	// as the file is opened rather than before the decision to open it — so the
	// bar for putting a scope here is stated once, in Rules, and applies to every
	// entry.
	scope []string
}

// Rules is the methodology, in the order a unit of work meets it, and which of
// them the harness may leave until they are needed.
//
// **Everything here is preloaded on Claude Code and paid in every request of the
// session.** That is what the rules are for — a rule you have to decide to read is
// one the session that skips it does not have — and it is also the whole of what
// this product costs to run. Fifteen files come to ~44KB, and nothing about the
// methodology gets cheaper by being explained better.
//
// **So three of them carry a scope, and the bar is mechanical: a rule may be
// scoped only where a validator reports what it prevents.** The failure a scope
// introduces is the rule arriving after the decision it governs, and that is
// survivable exactly when something else catches the result — an EARS clause
// missing a part, a task with no methodology annotation, a broken wikilink, a
// dependency nobody recorded are all `scc validate` exit 2 before the commit. The
// three scoped rules are the three whose subject a validator reads.
//
// Every other rule stays on whatever it costs, and the ones that look most
// temptingly scopable are the clearest cases:
//
//   - `artifacts.md` would scope to `plans/**`, and its instruction is **never
//     open the plan**. A rule that loads when the agent opens the file is a rule
//     that only ever arrives to say the thing it was meant to prevent has happened.
//   - `prior-art.md` and `routing.md` fire before the first artifact exists, so
//     there is no file for a glob to match.
//   - `caveman.md`, `ladder.md`, `methodology.md`, `delivery.md` and
//     `verification.md` govern how the agent works rather than what it edits, and
//     nothing mechanical reports a failure to follow them.
//   - `notes.md` is triggered by writing a comment in *source*, which is every
//     file in the repository — a scope covering everything is a preloaded rule
//     with extra steps.
func Rules() []rule {
	return []rule{
		{name: "caveman.md"},
		{name: "routing.md"},
		{name: "autonomy.md"},
		{name: "prior-art.md"},
		{name: "methodology.md"},
		{name: "ladder.md"},
		// The task grammar: `(Unit)`/`(TDD)`, the number, the citation, the four
		// flags. `spec.task-*` and `plan.task-*` report every part of it.
		{name: "tasks.md", scope: []string{"specs/**", "plans/**"}},
		{name: "verification.md"},
		{name: "delivery.md"},
		// EARS, the delta form, the conditional design sections. The `spec`
		// validator grades the requirement lines and the traceability both ways.
		{name: "specs.md", scope: []string{"specs/**"}},
		// The wiki, ADRs, codewiki, glossary and stack — five validators between
		// them, reporting broken links, orphans, numbering gaps, unresolved
		// citations, avoided synonyms and undocumented dependencies.
		{name: "knowledge-base.md", scope: []string{"docs/**"}},
		{name: "notes.md"},
		{name: "code-search.md"},
		{name: "artifacts.md"},
	}
}

// File is one workspace file scc scaffolds and then tracks.
type File struct {
	// Name is the path inside the embedded tree, e.g. "rules/routing.md".
	Name string

	// Rel is where it goes: slash-separated and relative to the workspace root.
	// This is also verbatim what the manifest records, so the layout crosses
	// machines as slashes and never as the host's separator.
	Rel string

	// Kind selects the rendering.
	Kind Kind

	// Scope is a Rule's `paths:` list, empty for a rule that is always on. It
	// reaches the file only on a harness whose ScopedRules says the header means
	// something there.
	Scope []string

	// Owned marks a file the user owns from their first edit. scc writes it once
	// and records it, then leaves it alone: an upgrade reports that a new version
	// exists rather than merging into it. The entry file and the project rule are
	// the two files whose whole purpose is to be edited, and merging a new
	// template into someone's own prose produces a mess no one asked for.
	Owned bool
}

// Workspace returns every file `scc init` writes for a harness, sorted by
// destination so init's output and the manifest are in the same order on every
// run.
func Workspace(h paths.Harness) []File {
	under := func(seg ...string) string {
		return path.Join(append([]string{h.Dir}, seg...)...)
	}
	set := []File{
		{Name: "entry.md", Rel: h.EntryFile, Kind: Plain, Owned: true},
		{Name: "rules/project.md", Rel: under(h.RulesSeg, "project.md"), Kind: Plain, Owned: true},
	}
	// The methodology. Every one of these is scc's own content: an upgrade should
	// deliver improvements to them, so none is Owned.
	for _, r := range Rules() {
		set = append(set, File{
			Name:  "rules/" + r.name,
			Rel:   under(h.RulesSeg, r.name),
			Kind:  Rule,
			Scope: r.scope,
		})
	}
	for _, agent := range ReviewAgents {
		set = append(set, File{
			Name: "agents/" + agent + ".md",
			Rel:  under(h.AgentsSeg, agent+h.AgentExt()),
			Kind: Agent,
		})
	}
	// The knowledge base's authors, and the workflow skills that run the
	// methodology rather than describe it. Why each list holds what it holds is on
	// the lists themselves; here they are one set of files on identical terms.
	for _, skill := range Skills() {
		set = append(set, File{
			Name: "skills/" + skill + "/SKILL.md",
			Rel:  under(h.SkillsSeg, skill, "SKILL.md"),
			Kind: Plain,
		})
		// A command per skill, for a harness where a skill is not already one.
		//
		// **Where it is, the command file is pure cost.** Claude Code registers
		// every skill as `/<name>` by itself, so shipping `commands/scc-wiki.md`
		// beside `skills/scc-wiki/` put two entry points in the picker for one
		// thing and, worse, a second `description` in front of the model — and a
		// description is the half that is preloaded into every request whether
		// anybody runs it or not. Eight of them came to ~980 characters of saying
		// twice what the skill's own description says once.
		//
		// The prefix moved onto the skill to make that possible, and it is load-
		// bearing rather than tidy: with the command gone the skill's own name is
		// what the user types, and `init` is a slash command Claude Code already
		// ships. `/init` resolving to two different things is the failure the
		// review agents had under their unprefixed names, arriving by the same
		// door — so the names are `scc-init`, `scc-wiki`, and every reference to
		// `/scc-init` that was already written keeps working.
		//
		// Codex gets none for a different reason: its custom prompts live in the
		// user's home directory and are deprecated in favor of skills, so there is
		// nothing project-scoped to write.
		if h.CommandsSeg == "" || h.SkillsAreCommands {
			continue
		}
		cmd := skill + ".md"
		set = append(set, File{
			Name: "commands/" + cmd,
			Rel:  under(h.CommandsSeg, cmd),
			Kind: Command,
		})
	}
	sort.Slice(set, func(i, j int) bool { return set[i].Rel < set[j].Rel })
	return set
}

// Seed is one of the `docs/` anchor files `scc init` lays down: the knowledge base's
// five fixed documents, each holding the format its validator checks and nothing
// else.
//
// A seed is deliberately NOT a managed file, and that is a third category rather
// than an oversight. init writes one only when it is missing, the manifest never
// records it, and no upgrade ever touches it — the same terms artifact templates
// ship on, for the same reason: the moment the file exists it holds project
// knowledge, which scc has nothing to deliver improvements to.
//
// They are also harness-neutral, because `docs/` is. A repo scaffolded for two
// harnesses has one knowledge base, written by whichever init ran first, and a seed
// that varied by harness would make the second init's copy the odd one out.
type Seed struct {
	// Name is the path inside the embedded tree, e.g. "docs/glossary.md".
	Name string
	// Rel is where it goes: slash-separated and relative to the workspace root.
	Rel string
}

// Seeds returns the anchors in destination order.
//
// Five, and the rule that picks them is the same one that picks the skills: a seed
// for each fixed `docs/` document a validator checks. The per-concept pages, the
// ADRs, and the codewiki pages have no fixed name, so they have no anchor — an
// empty directory is the honest scaffold for those.
//
// Seeding is what makes the knowledge base's own rules discoverable at the moment
// somebody first opens the file, rather than only after a validator has already
// fired on the missing document.
func Seeds() []Seed {
	return []Seed{
		// The dev container, on exactly the same terms as the knowledge base's
		// anchors below: written once when absent, recorded nowhere, never updated.
		//
		// It is a seed rather than a managed file because a Dockerfile is a build
		// environment, and that belongs to whoever has to debug it at three in the
		// morning. scc has nothing to deliver to it after the first write — the
		// image, the toolchain and the features are all this project's, and
		// `/scc-init` is what fills them in from the language the survey found.
		//
		// Scaffolded on every platform even though only Windows launches through
		// it, because the files are committed and a team is not one platform: the
		// Linux teammate who wants a container should find one, and the Windows
		// teammate should not have to write it.
		{Name: "devcontainer/Dockerfile", Rel: path.Join(paths.DevcontainerSeg, "Dockerfile")},
		{Name: "devcontainer/devcontainer.json", Rel: path.Join(paths.DevcontainerSeg, "devcontainer.json")},

		{Name: "docs/glossary.md", Rel: path.Join(paths.DocsSeg, paths.GlossarySeg)},
		{Name: "docs/notes.md", Rel: path.Join(paths.DocsSeg, paths.NotesSeg)},
		{Name: "docs/stack.md", Rel: path.Join(paths.DocsSeg, paths.StackSeg)},
		{Name: "docs/wiki/changelog.md", Rel: path.Join(paths.DocsSeg, paths.WikiSeg, paths.WikiLog)},
		{Name: "docs/wiki/index.md", Rel: path.Join(paths.DocsSeg, paths.WikiSeg, paths.WikiIndex)},
	}
}

// RTKTemplate is the embedded name of the RTK usage block.
const RTKTemplate = "rtk.md"

// RTKBlock returns the marker-delimited RTK instructions `scc rtk` splices into
// the entry file.
//
// A fragment, which is a fourth category and the only one that is not a file: it
// lands *inside* a document the user owns, so the markers rather than a path are
// what make it replaceable. RTK stamps its own version into the opening marker and
// rewrites between the two, so scc shipping the block verbatim means `rtk init` and
// `scc rtk` converge on one copy instead of racing to append a second.
//
// Data-free like every workspace template, and for a stronger reason than usual:
// the same block has to be recognizable to a tool that is not scc.
func RTKBlock() (string, error) { return Content(RTKTemplate) }

// CodeGraphTemplate is the embedded name of the CodeGraph usage block.
const CodeGraphTemplate = "codegraph.md"

// CodeGraphBlock returns the marker-delimited CodeGraph instructions `scc launch`
// splices into the entry file.
//
// A fragment like the RTK one, and delimited by markers of scc's own — which is the
// opposite of that decision, for the opposite reason. `rtk init` writes an RTK block
// into this same file, so sharing its markers is what makes the two tools converge
// on one copy. CodeGraph writes nothing into the entry file at all: this block is
// scc's account of `scc graph`, not CodeGraph's account of itself, so namespacing it
// leaves a future CodeGraph release free to add its own without either clobbering
// the other.
func CodeGraphBlock() (string, error) { return Content(CodeGraphTemplate) }

// ReviewAgents names the two subagents scc ships. Both read and neither writes:
// review is where a cold context is worth paying for, and authorship is not.
//
// **Prefixed, because the unprefixed names are taken.** Claude Code ships its own
// `code-review` and `security-review`, so a workspace scaffolded with those names
// hands the model two different things under each one — and `delivery.md` names
// them in prose, which resolves to whichever the harness picked. That is not a
// collision anybody would see: both names produce a review, and the one that ran
// is the one that answers. The prefix is the same `scc-` the commands already
// carry, and it is what makes the rule's instruction name exactly one thing.
//
// Keeping them at all is a separate question with a separate answer: these two
// check what the built-in cannot, because they know the methodology. Gate 1 is
// ticked boxes against the code that was actually written, the standard is the
// artifact rather than taste, and the verdict comes back in a fixed shape the
// orchestrator branches on. Codex and opencode have no built-in either.
var ReviewAgents = []string{"scc-code-review", "scc-security-review"}

// KnowledgeSkills names the skills that author a `docs/` artifact a validator
// checks — one per artifact, so a workspace shipping the eight validators never
// demands conformance to a document nobody was told how to write.
//
// The methodology is deliberately absent from it: the cycles, verification, and
// delivery are rules under the harness's rules directory, read when the concern
// is live. A skill restating a rule is a second copy of one fact, and the copy
// goes stale.
var KnowledgeSkills = []string{"scc-adr", "scc-codewiki", "scc-glossary", "scc-prd", "scc-stack", "scc-wiki"}

// WorkflowSkills names the skills that drive the methodology instead of authoring a
// document. There is one, and the bar for a second is the rule above: if a skill
// would restate what a rule already says, it should be the rule.
//
// plan-run clears that bar because it holds what no rule does — the loop *across*
// units of work. delivery.md ends at one merged pull request, which is exactly where
// one unit of work ends; running a whole plan means picking the next group, branching
// it from the merge the last group produced, and recovering the position from `main`
// when a session dies mid-plan. That is a procedure, and a procedure a person invokes
// needs an entry point they can type.
//
// init clears it for the same reason, one layer up. The knowledge skills each own one
// artifact and are triggered by a concern going live — something was learned, a
// decision was made, a dependency was added. None of them fires when the whole base is
// empty, and no rule can say which artifact comes first, because a rule is read at the
// moment its own concern arrives. Bootstrapping an existing repository is the one job
// that needs the order *across* the six, a survey before any of them, and a bar on
// what may be written when the answer is being reconstructed rather than remembered.
var WorkflowSkills = []string{"scc-init", "scc-plan-run"}

// Skills is every skill scc ships, knowledge first. Both the skill directory and its
// slash command are derived from this one list, so the two cannot drift apart, and a
// skill added to either half above reaches workspaces that already exist through
// `scc update` on the same terms as any other managed file.
//
// The register the agent answers in was briefly a skill here and is now caveman.md,
// because it is on by default: a skill nobody invokes does nothing, and one the model
// must decide to load is not a default. What it costs is what every rule costs — it is
// preloaded where the harness reads rules/ — and that is the price of it being on.
func Skills() []string {
	return append(append([]string{}, KnowledgeSkills...), WorkflowSkills...)
}

// SkillPrefix namespaces everything scc scaffolds that the user invokes by name.
//
// It sits on the skill rather than on a command wrapping it, because on a harness
// that turns a skill into a slash command by itself the skill's name *is* what
// gets typed — and `init` is a command Claude Code already ships. The review
// agents carry it for the same reason under the same names.
const SkillPrefix = "scc-"

// Dirs returns every directory `scc init` creates, including the ones it has no
// file to put in. An agent that can see specs/, plans/, and docs/adr/ knows where
// its output goes; one that has to infer the layout from a rule file guesses.
func Dirs(h paths.Harness) []string {
	dirs := []string{
		path.Join(h.Dir, h.RulesSeg),
		path.Join(h.Dir, h.AgentsSeg),
		path.Join(h.Dir, h.SkillsSeg),
	}
	if h.CommandsSeg != "" {
		dirs = append(dirs, path.Join(h.Dir, h.CommandsSeg))
	}
	return append(dirs,
		paths.SpecsSeg,
		paths.PlansSeg,
		paths.DocsSeg,
		path.Join(paths.DocsSeg, paths.WikiSeg),
		path.Join(paths.DocsSeg, paths.WikiSeg, paths.WikiPagesSeg),
		path.Join(paths.DocsSeg, paths.RawSeg),
		path.Join(paths.DocsSeg, paths.ADRSeg),
		path.Join(paths.DocsSeg, paths.CodewikiSeg),
		paths.DevcontainerSeg,
	)
}

// Content returns an embedded template verbatim, normalized to LF.
//
// Normalizing here rather than at each call site is what makes a manifest hash
// portable: the same template must hash identically no matter how the scc source
// tree was checked out on the machine that built the binary.
func Content(name string) (string, error) {
	b, err := files.ReadFile(path.Join(root, name))
	if err != nil {
		return "", fmt.Errorf("no embedded template %q: %w", name, err)
	}
	return textutil.NormalizeNewlines(string(b)), nil
}

// layout is what a workspace template can address: where this harness keeps
// things, and nothing else. Every field is derived from the profile, so a
// template cannot come to depend on the machine, the project, or the clock.
type layout struct {
	Harness string
	// Label is the tool's own name, for the one sentence a template addresses to
	// the agent about the tool it is running inside.
	Label       string
	Dir         string
	Entry       string
	Rules       string
	Skills      string
	Agents      string
	Commands    string
	HasCommands bool
	// SkillsAreCommands says the harness turns a skill into a slash command by
	// itself, so the layout block names no commands directory — there is not one.
	SkillsAreCommands bool
	Manifest          string
	// RulesPreloaded says the harness already put Rules in the agent's context,
	// so a template can stop telling it to go and read them. See
	// paths.Harness.PreloadsRules.
	RulesPreloaded bool

	// RulesScoped says some of the rules carry a `paths:` header this harness
	// reads, so "everything is already in context" is not true of the whole set.
	//
	// It is a field for the same reason RulesPreloaded is one: the entry file is
	// the document the agent is meant to trust about its own environment, and a
	// blanket "nothing to open" in a workspace where three rules are on demand
	// teaches it that the file is wrong. See paths.Harness.ScopedRules.
	RulesScoped bool

	// The same three paths, trailing slash included, padded to the column the
	// entry file's layout block puts its descriptions in.
	//
	// The padding is computed rather than written into the template because a
	// harness's own directory names differ in width — `.codex/rules/` is three
	// characters shorter than `.opencode/rules/` — so any literal run of spaces
	// would line up in exactly one of the three and read as ragged in the other
	// two. TestEntryLayoutBlockIsAligned is what keeps these and the template's
	// hand-written left column agreeing.
	RulesCol    string
	SkillsCol   string
	CommandsCol string
}

// layoutColumn is where a description starts in the entry file's layout block.
// It has to clear both the widest path any harness produces (`.opencode/command/`,
// 18) and the widest literal in the template (`specs/<feature>/`, 16).
const layoutColumn = 20

// column renders one path as the left half of that block: trailing slash, then
// spaces out to layoutColumn. A path too wide to pad still gets one space, so a
// future harness with a long name degrades to ragged rather than to a run-on line.
func column(p string) string {
	s := p + "/"
	if pad := layoutColumn - len([]rune(s)); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s + " "
}

func layoutOf(h paths.Harness) layout {
	l := layout{
		Harness:           h.ID,
		Label:             h.Label,
		Dir:               h.Dir,
		Entry:             h.EntryFile,
		Rules:             path.Join(h.Dir, h.RulesSeg),
		Skills:            path.Join(h.Dir, h.SkillsSeg),
		Agents:            path.Join(h.Dir, h.AgentsSeg),
		Manifest:          path.Join(h.Dir, paths.ManifestSeg),
		RulesPreloaded:    h.PreloadsRules,
		RulesScoped:       h.ScopedRules,
		SkillsAreCommands: h.SkillsAreCommands,
	}
	l.RulesCol = column(l.Rules)
	l.SkillsCol = column(l.Skills)
	if h.CommandsSeg != "" {
		l.Commands = path.Join(h.Dir, h.CommandsSeg)
		l.CommandsCol = column(l.Commands)
		l.HasCommands = true
	}
	return l
}

// Render produces the exact bytes f takes in a workspace scaffolded for h: paths
// expanded, and for an agent or a command, the frontmatter that harness's loader
// parses.
func Render(h paths.Harness, f File) (string, error) {
	raw, err := Content(f.Name)
	if err != nil {
		return "", err
	}
	expanded, err := expand(f.Name, raw, layoutOf(h))
	if err != nil {
		return "", err
	}
	switch f.Kind {
	case Agent:
		return renderAgent(h, f, expanded)
	case Command:
		return renderCommand(h, expanded)
	case Rule:
		return renderRule(h, f, expanded), nil
	default:
		return expanded, nil
	}
}

// renderRule puts a rule's `paths:` header on where the harness reads one.
//
// Only there, and that is the point of the field rather than a switch: on Codex
// and opencode nothing preloads the rules in the first place, so the header would
// buy nothing and cost every reader of that file the two lines it takes to work
// out that they are not YAML frontmatter it should act on.
func renderRule(h paths.Harness, f File, body string) string {
	if !h.ScopedRules || len(f.Scope) == 0 {
		return body
	}
	var b strings.Builder
	b.WriteString("---\npaths:\n")
	for _, p := range f.Scope {
		fmt.Fprintf(&b, "  - %q\n", p)
	}
	b.WriteString("---\n\n")
	b.WriteString(body)
	return b.String()
}

// expand runs a workspace template through text/template with the layout as its
// only data. missingkey=error turns a typo'd field into a test failure rather
// than the string "<no value>" landing in somebody's rules.
func expand(name, raw string, l layout) (string, error) {
	tpl, err := template.New(name).Option("missingkey=error").Parse(raw)
	if err != nil {
		return "", fmt.Errorf("template %q: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, l); err != nil {
		return "", fmt.Errorf("template %q: %w", name, err)
	}
	return textutil.NormalizeNewlines(buf.String()), nil
}

// meta is the harness-neutral header a shared template carries: what this thing
// is called and when to use it. Everything else in a real header is dialect.
type meta struct {
	name        string
	description string
	extra       map[string]string // keys only some harnesses understand
	body        string
}

// splitMeta reads the leading `---` block as flat `key: value` pairs. This is not
// a YAML parser and does not need to be: scc writes these templates, the tests
// assert the shape, and a template that grows a nested value here would be
// telling us it wants to be per-harness data instead.
func splitMeta(name, raw string) (meta, error) {
	const fence = "---\n"
	if !strings.HasPrefix(raw, fence) {
		return meta{}, fmt.Errorf("template %q: expected a leading --- block", name)
	}
	rest := raw[len(fence):]
	end := strings.Index(rest, "\n"+fence)
	if end < 0 {
		return meta{}, fmt.Errorf("template %q: unterminated --- block", name)
	}
	m := meta{extra: map[string]string{}, body: strings.TrimLeft(rest[end+len("\n"+fence):], "\n")}
	for _, line := range strings.Split(rest[:end], "\n") {
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			return meta{}, fmt.Errorf("template %q: %q is not a `key: value` line", name, line)
		}
		switch key {
		case "name":
			m.name = value
		case "description":
			m.description = value
		default:
			m.extra[key] = value
		}
	}
	if m.description == "" {
		return meta{}, fmt.Errorf("template %q: no description", name)
	}
	return m, nil
}

// The reasoning budget both reviewers run on. Review is chains-of-inference work
// — tracing a value from an argument to a shell, or a ticked box to the code
// behind it — and that is what effort buys. Every harness that expresses it gets
// it; the model tier is pinned only where the harness has a stable alias for one
// (Claude Code's "sonnet"), because a pinned `gpt-5.6` or `anthropic/claude-x`
// would be a guess about a name that churns and a provider the user may not have
// configured.
const reviewEffort = "high"

func renderAgent(h paths.Harness, f File, raw string) (string, error) {
	m, err := splitMeta(f.Name, raw)
	if err != nil {
		return "", err
	}
	if m.name == "" {
		return "", fmt.Errorf("template %q: an agent needs a name", f.Name)
	}
	switch h.ID {
	case paths.Codex.ID:
		// A Codex agent role file: flat TOML, the prose in developer_instructions.
		// A literal string ('''), so no character in the reviewer's prose needs
		// escaping and none is silently mangled — the tests hold the body to that.
		var b strings.Builder
		fmt.Fprintf(&b, "name = %q\n", m.name)
		fmt.Fprintf(&b, "description = %q\n", m.description)
		fmt.Fprintf(&b, "model_reasoning_effort = %q\n", reviewEffort)
		fmt.Fprintf(&b, "developer_instructions = '''\n%s'''\n", m.body)
		return b.String(), nil
	case paths.OpenCode.ID:
		// opencode takes the agent's name from the filename, so the header carries
		// only what it cannot infer. `edit: deny` is the "you do not write code,
		// you report it" rule made mechanical; bash stays allowed because the
		// reviewer has to run this project's tests and lint itself.
		var b strings.Builder
		b.WriteString("---\n")
		fmt.Fprintf(&b, "description: %s\n", m.description)
		b.WriteString("mode: subagent\n")
		b.WriteString("temperature: 0.1\n")
		b.WriteString("permission:\n  edit: deny\n")
		b.WriteString("---\n\n")
		b.WriteString(m.body)
		return b.String(), nil
	default:
		var b strings.Builder
		b.WriteString("---\n")
		fmt.Fprintf(&b, "name: %s\n", m.name)
		fmt.Fprintf(&b, "description: %s\n", m.description)
		b.WriteString("tools: Read, Grep, Glob, Bash\n")
		b.WriteString("model: sonnet\n")
		fmt.Fprintf(&b, "effort: %s\n", reviewEffort)
		b.WriteString("---\n\n")
		b.WriteString(m.body)
		return b.String(), nil
	}
}

func renderCommand(h paths.Harness, raw string) (string, error) {
	m, err := splitMeta("command", raw)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "description: %s\n", m.description)
	// argument-hint is Claude Code's; opencode's command schema does not define it
	// and there is no value in feeding a loader a key it will only ignore.
	if hint, ok := m.extra["argument-hint"]; ok && h.ID == paths.Claude.ID {
		fmt.Fprintf(&b, "argument-hint: %s\n", hint)
	}
	b.WriteString("---\n\n")
	b.WriteString(m.body)
	return b.String(), nil
}

// ArtifactData is what an artifact template can interpolate. Deliberately tiny:
// every field is something the user supplied on the command line, so a template
// cannot come to depend on the machine it was rendered on.
type ArtifactData struct {
	// Name is the kebab-case name as given, e.g. "user-auth".
	Name string
	// Title is Name rendered for a heading, e.g. "User auth".
	Title string
	// Autonomy is "auto" or "gated" — the kickoff answer, recorded so nobody is
	// asked twice and the run stays reproducible from the file.
	Autonomy string
	// CI is "wait" or "no-wait", recorded for the same reason.
	CI string
}

// Artifact renders one of the artifact templates ("requirements.md", "design.md",
// "tasks.md", "plan.md") with data. The result is LF-normalized and ends in
// exactly one newline.
func Artifact(name string, data ArtifactData) (string, error) {
	raw, err := Content(path.Join(artifactsDir, name))
	if err != nil {
		return "", err
	}
	// Option("missingkey=error") makes a typo'd field a build-time-ish failure —
	// caught by this package's tests — instead of the string "<no value>" landing in
	// the user's requirements document.
	tpl, err := template.New(name).Option("missingkey=error").Parse(raw)
	if err != nil {
		return "", fmt.Errorf("template %q: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template %q: %w", name, err)
	}
	out := textutil.NormalizeNewlines(buf.String())
	return strings.TrimRight(out, "\n") + "\n", nil
}

// Title turns a kebab-case name into heading text: "user-auth" -> "User auth".
func Title(name string) string {
	s := strings.ReplaceAll(name, "-", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// walk lists every embedded template path, relative to the tree root. Used by the
// tests that assert the set and the tree agree — an embedded file no File points
// at ships in the binary and reaches no workspace.
func walk() ([]string, error) {
	var out []string
	err := fs.WalkDir(files, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepathRel(p)
		if err != nil {
			return err
		}
		out = append(out, rel)
		return nil
	})
	sort.Strings(out)
	return out, err
}

func filepathRel(p string) (string, error) {
	trimmed := strings.TrimPrefix(p, root+"/")
	if trimmed == p {
		return "", fmt.Errorf("embedded path %q is not under %q", p, root)
	}
	return trimmed, nil
}
