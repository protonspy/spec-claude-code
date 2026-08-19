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
const Version = "18"

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
)

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
	for _, rule := range []string{
		"caveman.md",
		"routing.md",
		"autonomy.md",
		"prior-art.md",
		"methodology.md",
		"tasks.md",
		"verification.md",
		"delivery.md",
		"specs.md",
		"knowledge-base.md",
		"code-search.md",
		"artifacts.md",
	} {
		set = append(set, File{
			Name: "rules/" + rule,
			Rel:  under(h.RulesSeg, rule),
			Kind: Plain,
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
		// One command per skill, so the human has an explicit entry point where the
		// model has a description. Namespaced, because slash commands share a flat
		// namespace with every other source the harness loads them from and `/adr`
		// would collide on contact.
		//
		// Codex is the exception and gets none: its custom prompts live in the
		// user's home directory and are deprecated in favor of skills, so there is
		// nothing project-scoped to write. The skills above are the whole surface
		// there, which is what Codex itself now recommends.
		if h.CommandsSeg == "" {
			continue
		}
		cmd := commandPrefix + skill + ".md"
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
// four fixed documents, each holding the format its validator checks and nothing
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
// Four, and the rule that picks them is the same one that picks the skills: a seed
// for each fixed `docs/` document a validator checks. The per-concept pages, the
// ADRs, and the codewiki pages have no fixed name, so they have no anchor — an
// empty directory is the honest scaffold for those.
//
// Seeding is what makes the knowledge base's own rules discoverable at the moment
// somebody first opens the file, rather than only after a validator has already
// fired on the missing document.
func Seeds() []Seed {
	return []Seed{
		{Name: "docs/glossary.md", Rel: path.Join(paths.DocsSeg, paths.GlossarySeg)},
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
var ReviewAgents = []string{"code-review", "security-review"}

// KnowledgeSkills names the skills that author a `docs/` artifact a validator
// checks — one per artifact, so a workspace shipping the eight validators never
// demands conformance to a document nobody was told how to write.
//
// The methodology is deliberately absent from it: the cycles, verification, and
// delivery are rules under the harness's rules directory, read when the concern
// is live. A skill restating a rule is a second copy of one fact, and the copy
// goes stale.
var KnowledgeSkills = []string{"adr", "codewiki", "glossary", "prd", "stack", "wiki"}

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
var WorkflowSkills = []string{"init", "plan-run"}

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

// commandPrefix namespaces the scaffolded slash commands.
const commandPrefix = "scc-"

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
	Manifest    string
	// RulesPreloaded says the harness already put Rules in the agent's context,
	// so a template can stop telling it to go and read them. See
	// paths.Harness.PreloadsRules.
	RulesPreloaded bool

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
		Harness:        h.ID,
		Label:          h.Label,
		Dir:            h.Dir,
		Entry:          h.EntryFile,
		Rules:          path.Join(h.Dir, h.RulesSeg),
		Skills:         path.Join(h.Dir, h.SkillsSeg),
		Agents:         path.Join(h.Dir, h.AgentsSeg),
		Manifest:       path.Join(h.Dir, paths.ManifestSeg),
		RulesPreloaded: h.PreloadsRules,
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
	default:
		return expanded, nil
	}
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
