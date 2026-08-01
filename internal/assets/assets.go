// Package assets is the template set compiled into the binary: the rules, the
// review agents, the knowledge-base skills and their commands, and the artifact
// templates, all of it plain Markdown embedded with //go:embed.
//
// There are two kinds of template here and the difference matters:
//
//   - **Workspace files** — what `scc init` writes and the manifest tracks. These
//     are DATA-FREE: no project name, no date, no path is interpolated into any of
//     them. That is what makes an upgrade possible without storing rendered text
//     anywhere. A given template version renders byte-identically in every
//     workspace on earth, so its content hash identifies it globally and the
//     three-way merge can reconstruct the old side from the version alone. The two
//     files that genuinely want per-project content — CLAUDE.md and the project
//     rule — are exactly the files an upgrade excludes because the user owns them,
//     so the two rules agree instead of fighting.
//
//   - **Artifact templates** — what `spec new` and `plan new` render. These take
//     data, because they are authored from birth: the user owns the result
//     immediately, nothing tracks them in the manifest, and no upgrade ever touches
//     them.
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
const Version = "3"

// The embedded tree. "all:" so nothing is silently dropped for having a name the
// default embed pattern skips.
//
//go:embed all:templates
var files embed.FS

const (
	root         = "templates"
	artifactsDir = "artifacts"
)

// File is one workspace file scc scaffolds and then tracks.
type File struct {
	// Name is the path inside the embedded tree, e.g. "claude/rules/routing.md".
	Name string

	// Rel is where it goes: slash-separated and relative to the workspace root.
	// This is also verbatim what the manifest records, so the layout crosses
	// machines as slashes and never as the host's separator.
	Rel string

	// Owned marks a file the user owns from their first edit. scc writes it once
	// and records it, then leaves it alone: an upgrade reports that a new version
	// exists rather than merging into it. CLAUDE.md and the project rule are the
	// two files whose whole purpose is to be edited, and merging a new template
	// into someone's own prose produces a mess no one asked for.
	Owned bool
}

// Workspace returns every file `scc init` writes, sorted by destination so init's
// output and the manifest are in the same order on every run.
func Workspace() []File {
	claude := func(seg ...string) string {
		return path.Join(append([]string{paths.ClaudeDir}, seg...)...)
	}
	set := []File{
		{Name: "CLAUDE.md", Rel: paths.EntryFile, Owned: true},
		{Name: "claude/rules/project.md", Rel: claude(paths.RulesSeg, "project.md"), Owned: true},
	}
	// The methodology. Every one of these is scc's own content: an upgrade should
	// deliver improvements to them, so none is Owned.
	for _, rule := range []string{
		"routing.md",
		"autonomy.md",
		"methodology.md",
		"tasks.md",
		"verification.md",
		"delivery.md",
		"specs.md",
		"knowledge-base.md",
	} {
		set = append(set, File{
			Name: "claude/rules/" + rule,
			Rel:  claude(paths.RulesSeg, rule),
		})
	}
	for _, agent := range []string{"code-review.md", "security-review.md"} {
		set = append(set, File{
			Name: "claude/agents/" + agent,
			Rel:  claude(paths.AgentsSeg, agent),
		})
	}
	// The knowledge base's authors. Every one of these produces an artifact
	// `scc validate` already checks — a workspace that ships the eight validators
	// and no skill teaching the formats would demand conformance to documents
	// nobody was told how to write.
	for _, skill := range KnowledgeSkills {
		set = append(set, File{
			Name: "claude/skills/" + skill + "/SKILL.md",
			Rel:  claude(paths.SkillsSeg, skill, "SKILL.md"),
		})
		// One command per skill, so the human has an explicit entry point where the
		// model has a description. Namespaced, because slash commands share a flat
		// namespace with every other source Claude Code loads them from and `/adr`
		// would collide on contact.
		cmd := commandPrefix + skill + ".md"
		set = append(set, File{
			Name: "claude/commands/" + cmd,
			Rel:  claude(paths.CommandsSeg, cmd),
		})
	}
	sort.Slice(set, func(i, j int) bool { return set[i].Rel < set[j].Rel })
	return set
}

// KnowledgeSkills names the skills scc ships, each one the author of a `docs/`
// artifact a validator checks. Both the skill directory and its slash command are
// derived from this list, so the two cannot drift apart.
//
// The methodology is deliberately absent from it: the cycles, verification, and
// delivery are rules under `.claude/rules/`, read when the concern is live. A skill
// restating a rule is a second copy of one fact, and the copy goes stale.
var KnowledgeSkills = []string{"adr", "codewiki", "glossary", "prd", "stack", "wiki"}

// commandPrefix namespaces the scaffolded slash commands.
const commandPrefix = "scc-"

// Dirs returns the directories `scc init` creates even when it has no file to put
// in them. An agent that can see specs/, plans/, and docs/wiki/ knows where its
// output goes; one that has to infer the layout from a rule file guesses.
func Dirs() []string {
	return []string{
		path.Join(paths.ClaudeDir, paths.RulesSeg),
		path.Join(paths.ClaudeDir, paths.AgentsSeg),
		path.Join(paths.ClaudeDir, paths.SkillsSeg),
		path.Join(paths.ClaudeDir, paths.CommandsSeg),
		paths.SpecsSeg,
		paths.PlansSeg,
		path.Join(paths.DocsSeg, paths.WikiSeg),
		path.Join(paths.DocsSeg, paths.RawSeg),
		path.Join(paths.DocsSeg, paths.ADRSeg),
		path.Join(paths.DocsSeg, paths.CodewikiSeg),
	}
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
