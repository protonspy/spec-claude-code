package assets

import (
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/paths"
)

// Every embedded template is checked for the properties a manifest hash depends
// on. A CRLF template would hash differently on a Windows build machine than on a
// Linux one, and the same workspace would then report every managed file as edited
// depending on who built the binary.
func TestEveryTemplateIsLFCleanAndNonEmpty(t *testing.T) {
	names, err := walk()
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("no templates are embedded")
	}
	for _, name := range names {
		raw, err := Content(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if strings.TrimSpace(raw) == "" {
			t.Errorf("%s: empty", name)
		}
		if !strings.HasSuffix(raw, "\n") {
			t.Errorf("%s: does not end in a newline", name)
		}
		if strings.HasSuffix(raw, "\n\n") {
			t.Errorf("%s: ends in more than one newline", name)
		}
	}
}

// Content normalizes, so an embedded file that happened to be checked out CRLF is
// still delivered as LF. Assert on the raw bytes to prove the guarantee is in the
// code and not in the checkout.
func TestContentIsNormalized(t *testing.T) {
	raw, err := Content("entry.md")
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if strings.Contains(raw, "\r") {
		t.Error("entry.md carries CR after Content()")
	}
}

func TestContentRejectsUnknownTemplate(t *testing.T) {
	if _, err := Content("nope.md"); err == nil {
		t.Error("Content of a missing template returned no error")
	}
}

// The workspace set and the embedded tree must agree in both directions: a File
// pointing at nothing breaks init, and an embedded file no harness's File points
// at ships in the binary and reaches no workspace.
func TestWorkspaceSetAndTreeAgree(t *testing.T) {
	embedded, err := walk()
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	inTree := map[string]bool{}
	for _, name := range embedded {
		inTree[name] = true
	}

	referenced := map[string]bool{}
	for _, h := range paths.Harnesses() {
		for _, f := range Workspace(h) {
			if _, err := Content(f.Name); err != nil {
				t.Errorf("Workspace(%s) references %q which is not embedded", h.ID, f.Name)
			}
			referenced[f.Name] = true
		}
	}
	for _, name := range []string{"requirements.md", "design.md", "tasks.md", "plan.md"} {
		referenced["artifacts/"+name] = true
	}
	for _, s := range Seeds() {
		if _, err := Content(s.Name); err != nil {
			t.Errorf("Seeds() references %q which is not embedded", s.Name)
		}
		referenced[s.Name] = true
	}
	for name := range inTree {
		if !referenced[name] {
			t.Errorf("embedded template %q is in no harness's Workspace(), Seeds(), and is not an artifact template", name)
		}
	}
}

// Destinations must be unique and slash-separated: they are written verbatim into
// the manifest, and two Files claiming one path means one silently wins.
func TestWorkspaceDestinationsAreUniqueAndSlashed(t *testing.T) {
	for _, h := range paths.Harnesses() {
		seen := map[string]bool{}
		for _, f := range Workspace(h) {
			if seen[f.Rel] {
				t.Errorf("%s: duplicate destination %q", h.ID, f.Rel)
			}
			seen[f.Rel] = true
			if strings.Contains(f.Rel, `\`) {
				t.Errorf("%s: destination %q is not slash-separated", h.ID, f.Rel)
			}
			if strings.HasPrefix(f.Rel, "/") || strings.Contains(f.Rel, "..") {
				t.Errorf("%s: destination %q escapes the workspace root", h.ID, f.Rel)
			}
		}
	}
}

// Every managed file lands inside the harness it was scaffolded for, except the
// entry file the harness itself loads from the project root. A file written into
// another harness's directory would be invisible to its own tools and would be
// clobbered by that harness's own init.
func TestWorkspaceStaysInsideItsHarness(t *testing.T) {
	for _, h := range paths.Harnesses() {
		for _, f := range Workspace(h) {
			if f.Rel == h.EntryFile {
				continue
			}
			if !strings.HasPrefix(f.Rel, h.Dir+"/") {
				t.Errorf("%s: %q is outside %s/", h.ID, f.Rel, h.Dir)
			}
		}
	}
}

// The set is sorted by destination so init's report and the manifest are in the
// same order on every run and on every platform.
func TestWorkspaceIsSortedByDestination(t *testing.T) {
	for _, h := range paths.Harnesses() {
		set := Workspace(h)
		for i := 1; i < len(set); i++ {
			if set[i-1].Rel >= set[i].Rel {
				t.Fatalf("%s: not sorted: %q before %q", h.ID, set[i-1].Rel, set[i].Rel)
			}
		}
	}
}

// The upgrade path depends on this: a template's only variable is the harness
// profile, which the manifest records, so re-rendering the recorded version
// reconstructs the merge base exactly. A leftover action or a "<no value>" means
// something reached for data that is not the layout, and that data would not be
// reconstructible from anything scc stores.
func TestRenderLeavesNoActionsAndNoMissingKeys(t *testing.T) {
	for _, h := range paths.Harnesses() {
		for _, f := range Workspace(h) {
			out, err := Render(h, f)
			if err != nil {
				t.Fatalf("%s %s: %v", h.ID, f.Name, err)
			}
			if strings.Contains(out, "{{") || strings.Contains(out, "<no value>") {
				t.Errorf("%s %s: unrendered action or missing key", h.ID, f.Rel)
			}
			if strings.Contains(out, "\r") {
				t.Errorf("%s %s: rendered with CR", h.ID, f.Rel)
			}
			// "<harness>" is shorthand this project's own docs use for "whichever
			// of the three". It is prose for a human reading about scc, and it
			// means nothing to an agent reading a scaffolded rule — a template
			// carrying it wanted {{.Dir}} and would ship a path that resolves to
			// nowhere.
			if strings.Contains(out, "<harness>") {
				t.Errorf("%s %s: carries the literal <harness>; use the layout fields", h.ID, f.Rel)
			}
		}
	}
}

// Same inputs, same bytes — the property the manifest hash is built on.
func TestRenderIsDeterministic(t *testing.T) {
	for _, h := range paths.Harnesses() {
		for _, f := range Workspace(h) {
			first, err := Render(h, f)
			if err != nil {
				t.Fatalf("%s %s: %v", h.ID, f.Name, err)
			}
			second, err := Render(h, f)
			if err != nil {
				t.Fatalf("%s %s: %v", h.ID, f.Name, err)
			}
			if first != second {
				t.Errorf("%s %s: two renders differ", h.ID, f.Rel)
			}
		}
	}
}

// No rendered file may name a harness directory other than its own. This is the
// guard against the failure this refactor exists to prevent: a rule that still
// says ".claude/rules/project.md" reads correctly to a Claude Code session and
// sends a Codex session to a path that is not there.
func TestNoRenderedFileNamesAnotherHarnessDirectory(t *testing.T) {
	for _, h := range paths.Harnesses() {
		for _, f := range Workspace(h) {
			out, err := Render(h, f)
			if err != nil {
				t.Fatalf("%s %s: %v", h.ID, f.Name, err)
			}
			for _, other := range paths.Harnesses() {
				if other.Dir == h.Dir {
					continue
				}
				if strings.Contains(out, other.Dir+"/") {
					t.Errorf("%s %s mentions %s/", h.ID, f.Rel, other.Dir)
				}
			}
		}
	}
}

// Exactly the two files whose purpose is to be edited are Owned, in every
// harness. If a third appears, it is either a mistake or a decision that belongs
// in the design.
func TestOnlyTheUserOwnedFilesAreOwned(t *testing.T) {
	for _, h := range paths.Harnesses() {
		var owned []string
		for _, f := range Workspace(h) {
			if f.Owned {
				owned = append(owned, f.Rel)
			}
		}
		want := []string{h.Dir + "/" + h.RulesSeg + "/project.md", h.EntryFile}
		if h.EntryFile < want[0] {
			want = []string{h.EntryFile, h.Dir + "/" + h.RulesSeg + "/project.md"}
		}
		if strings.Join(owned, ",") != strings.Join(want, ",") {
			t.Errorf("%s: owned files = %v, want %v", h.ID, owned, want)
		}
	}
}

// The entry file is paid for in context by every session, and accuracy degrades as
// context grows. The rules exist so it can stay short; this is the guard that keeps
// someone from "helpfully" inlining them back into it.
func TestScaffoldedEntryFileStaysShort(t *testing.T) {
	for _, h := range paths.Harnesses() {
		raw, err := Render(h, entryFile(t, h))
		if err != nil {
			t.Fatalf("%s: %v", h.ID, err)
		}
		if n := strings.Count(raw, "\n"); n > 60 {
			t.Errorf("%s: scaffolded %s is %d lines; keep it under 60 and link to the rules instead",
				h.ID, h.EntryFile, n)
		}
	}
}

// Every rule the scaffolded entry file points at has to exist under this
// harness's own rules directory, or the entry file sends the agent to a file that
// isn't there.
func TestEntryFileLinksResolve(t *testing.T) {
	for _, h := range paths.Harnesses() {
		raw, err := Render(h, entryFile(t, h))
		if err != nil {
			t.Fatalf("%s: %v", h.ID, err)
		}
		for _, f := range Workspace(h) {
			if !strings.HasPrefix(f.Rel, h.Dir+"/"+h.RulesSeg+"/") {
				continue
			}
			if !strings.Contains(raw, "("+f.Rel+")") {
				t.Errorf("%s: %s does not link to %s", h.ID, h.EntryFile, f.Rel)
			}
		}
	}
}

func entryFile(t *testing.T, h paths.Harness) File {
	t.Helper()
	for _, f := range Workspace(h) {
		if f.Rel == h.EntryFile {
			return f
		}
	}
	t.Fatalf("%s: no entry file in the workspace set", h.ID)
	return File{}
}

// The review agents are read by each harness's own loader, and every one of them
// refuses a definition whose header it cannot parse. Assert the dialect per
// harness, at the source.
func TestReviewAgentsCarryTheHeaderTheirHarnessParses(t *testing.T) {
	for _, h := range paths.Harnesses() {
		for _, name := range ReviewAgents {
			f := findFile(t, h, h.Dir+"/"+h.AgentsSeg+"/"+name+h.AgentExt())
			out, err := Render(h, f)
			if err != nil {
				t.Fatalf("%s %s: %v", h.ID, name, err)
			}
			switch h.AgentFormat {
			case paths.FormatTOML:
				for _, want := range []string{
					"name = \"" + name + "\"\n",
					"description = \"",
					"model_reasoning_effort = \"high\"\n",
					"developer_instructions = '''\n",
				} {
					if !strings.Contains(out, want) {
						t.Errorf("%s %s: missing %q", h.ID, name, want)
					}
				}
				if !strings.HasSuffix(out, "'''\n") {
					t.Errorf("%s %s: developer_instructions is not closed", h.ID, name)
				}
			default:
				if !strings.HasPrefix(out, "---\n") {
					t.Errorf("%s %s: does not open with frontmatter", h.ID, name)
				}
				if !strings.Contains(out, "\ndescription: ") {
					t.Errorf("%s %s: no description", h.ID, name)
				}
			}
		}
	}
}

// The reasoning budget is pinned wherever the harness expresses one, because
// review is chains-of-inference work and that is what effort buys. The model tier
// is pinned only where the harness has a stable alias for one: a hardcoded
// provider-prefixed model would name something the user may not have configured.
func TestReviewAgentsPinTheirEffort(t *testing.T) {
	want := map[string][]string{
		paths.Claude.ID:   {"\nmodel: sonnet\n", "\neffort: high\n"},
		paths.Codex.ID:    {"model_reasoning_effort = \"high\"\n"},
		paths.OpenCode.ID: {"\nmode: subagent\n", "\n  edit: deny\n"},
	}
	for _, h := range paths.Harnesses() {
		for _, name := range ReviewAgents {
			f := findFile(t, h, h.Dir+"/"+h.AgentsSeg+"/"+name+h.AgentExt())
			out, err := Render(h, f)
			if err != nil {
				t.Fatalf("%s %s: %v", h.ID, name, err)
			}
			for _, w := range want[h.ID] {
				if !strings.Contains(out, w) {
					t.Errorf("%s %s: missing %q", h.ID, name, strings.TrimSpace(w))
				}
			}
		}
	}
}

// The Codex header is TOML, so the reviewer's prose has to survive being embedded
// in it. A literal string ends at the first ”' and escapes nothing, so the two
// things that could corrupt the file are a ”' in the body and a quote or
// backslash in the description.
func TestAgentProseIsSafeToEmbedInTOML(t *testing.T) {
	for _, name := range ReviewAgents {
		raw, err := Content("agents/" + name + ".md")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		m, err := splitMeta(name, raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(m.body, "'''") {
			t.Errorf("%s: the body contains ''' and would end the TOML literal string early", name)
		}
		if strings.ContainsAny(m.description, "\"\\") {
			t.Errorf("%s: the description contains a quote or backslash", name)
		}
		// A YAML scalar carrying ": " is a mapping, not a string, in the two
		// harnesses that read this description as frontmatter.
		if strings.Contains(m.description, ": ") {
			t.Errorf("%s: the description contains \": \" and would parse as a YAML mapping", name)
		}
	}
}

// Every skill in KnowledgeSkills ships a SKILL.md in every harness, and its slash
// command wherever the harness has a command surface. The two are derived from one
// list so they cannot drift, and this is the assertion that keeps the derivation
// honest — including for Codex, where the answer is deliberately no commands at
// all.
func TestEverySkillShipsWithItsCommand(t *testing.T) {
	for _, h := range paths.Harnesses() {
		skills, commands := map[string]bool{}, map[string]bool{}
		for _, f := range Workspace(h) {
			switch {
			case strings.HasPrefix(f.Rel, h.Dir+"/"+h.SkillsSeg+"/"):
				name := strings.TrimSuffix(strings.TrimPrefix(f.Rel, h.Dir+"/"+h.SkillsSeg+"/"), "/SKILL.md")
				skills[name] = true
			case h.CommandsSeg != "" && strings.HasPrefix(f.Rel, h.Dir+"/"+h.CommandsSeg+"/"):
				name := strings.TrimSuffix(strings.TrimPrefix(f.Rel, h.Dir+"/"+h.CommandsSeg+"/"), ".md")
				commands[name] = true
			}
		}
		if len(skills) != len(KnowledgeSkills) {
			t.Fatalf("%s: %d skills, want %d", h.ID, len(skills), len(KnowledgeSkills))
		}
		wantCommands := len(KnowledgeSkills)
		if h.CommandsSeg == "" {
			wantCommands = 0
		}
		if len(commands) != wantCommands {
			t.Fatalf("%s: %d commands, want %d", h.ID, len(commands), wantCommands)
		}
		for _, name := range KnowledgeSkills {
			if !skills[name] {
				t.Errorf("%s: %s is in KnowledgeSkills and ships no SKILL.md", h.ID, name)
			}
			if wantCommands > 0 && !commands[commandPrefix+name] {
				t.Errorf("%s: %s ships no %s%s command", h.ID, name, commandPrefix, name)
			}
		}
	}
}

// The Agent Skills contract is what `scc skill validate` enforces on everyone else,
// and the two fields it can break loading on are checked here at the source. The
// full conformance run happens against a scaffolded workspace in internal/cli — a
// tool that ships non-conforming skills has no standing to check anyone else's.
func TestSkillsCarryTheirFrontmatter(t *testing.T) {
	for _, h := range paths.Harnesses() {
		for _, f := range Workspace(h) {
			if !strings.HasSuffix(f.Rel, "/SKILL.md") {
				continue
			}
			raw, err := Render(h, f)
			if err != nil {
				t.Fatalf("%s %s: %v", h.ID, f.Rel, err)
			}
			if !strings.HasPrefix(raw, "---\n") {
				t.Errorf("%s: %s does not open with frontmatter", h.ID, f.Rel)
				continue
			}
			// The name must equal the parent directory or the skill does not load at
			// all, in any tool that reads this format.
			dir := path2(f.Rel)
			if !strings.Contains(raw, "\nname: "+dir+"\n") {
				t.Errorf("%s: %s frontmatter name does not match the directory %q", h.ID, f.Rel, dir)
			}
			if !strings.Contains(raw, "\ndescription: ") {
				t.Errorf("%s: %s has no description", h.ID, f.Rel)
			}
		}
	}
}

// path2 returns the directory name a SKILL.md sits in.
func path2(rel string) string {
	parts := strings.Split(strings.TrimSuffix(rel, "/SKILL.md"), "/")
	return parts[len(parts)-1]
}

// A slash command with no description is one that cannot be found in the picker.
// Claude Code additionally documents argument-hint; opencode's schema does not
// define it, so it must not be written there.
func TestCommandsCarryTheirDescription(t *testing.T) {
	for _, h := range paths.Harnesses() {
		if h.CommandsSeg == "" {
			continue
		}
		for _, f := range Workspace(h) {
			if f.Kind != Command {
				continue
			}
			raw, err := Render(h, f)
			if err != nil {
				t.Fatalf("%s %s: %v", h.ID, f.Rel, err)
			}
			if !strings.HasPrefix(raw, "---\n") || !strings.Contains(raw, "\ndescription: ") {
				t.Errorf("%s: %s needs frontmatter carrying a description", h.ID, f.Rel)
			}
			if h.ID != paths.Claude.ID && strings.Contains(raw, "argument-hint:") {
				t.Errorf("%s: %s carries argument-hint, which this harness does not define", h.ID, f.Rel)
			}
		}
	}
}

// The seeds are the knowledge base's four fixed documents, at the paths every
// validator already looks for them. A seed written anywhere else would be a file
// nothing reads, next to the finding saying the real one is missing.
func TestSeedsLandWhereTheValidatorsLook(t *testing.T) {
	want := []string{
		"docs/" + paths.GlossarySeg,
		"docs/" + paths.StackSeg,
		"docs/" + paths.WikiSeg + "/" + paths.WikiLog,
		"docs/" + paths.WikiSeg + "/" + paths.WikiIndex,
	}
	var got []string
	for _, s := range Seeds() {
		got = append(got, s.Rel)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Seeds() destinations = %v, want %v", got, want)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("Seeds() is not sorted by destination: %q before %q", got[i-1], got[i])
		}
	}
}

// A seed is untracked by construction, so nothing else may claim it: a destination
// that is also a managed file would be recorded in one harness's manifest and
// rewritten by that harness's update, which is exactly what seeding is not.
func TestSeedsAreNotManagedFiles(t *testing.T) {
	for _, h := range paths.Harnesses() {
		managed := map[string]bool{}
		for _, f := range Workspace(h) {
			managed[f.Rel] = true
		}
		for _, s := range Seeds() {
			if managed[s.Rel] {
				t.Errorf("%s: %s is both a seed and a managed file", h.ID, s.Rel)
			}
		}
	}
}

// docs/ is one tree per repo, not one per harness, and a seed is written verbatim
// rather than rendered. Both facts have the same consequence: a seed may not name a
// harness or carry a template action, or the knowledge base would read as belonging
// to whichever tool happened to run init first.
func TestSeedsAreHarnessNeutralAndDataFree(t *testing.T) {
	for _, s := range Seeds() {
		raw, err := Content(s.Name)
		if err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}
		if strings.Contains(raw, "{{") {
			t.Errorf("%s: carries a template action, and seeds are written verbatim", s.Name)
		}
		for _, h := range paths.Harnesses() {
			if strings.Contains(raw, h.Dir+"/") {
				t.Errorf("%s: names %s/, and docs/ belongs to no harness", s.Name, h.Dir)
			}
		}
	}
}

// Directories init creates on its own must be inside the workspace and
// slash-separated, for the same reason destinations must be.
func TestDirsAreRelativeAndSlashed(t *testing.T) {
	for _, h := range paths.Harnesses() {
		for _, d := range Dirs(h) {
			if strings.HasPrefix(d, "/") || strings.Contains(d, "..") || strings.Contains(d, `\`) {
				t.Errorf("%s: directory %q is not a slash-separated path inside the workspace", h.ID, d)
			}
		}
	}
}

// Every file the set writes has a directory init created, or the write is left
// depending on AtomicWrite's MkdirAll to cover for a layout the package was
// supposed to declare.
func TestDirsCoverEveryDestination(t *testing.T) {
	for _, h := range paths.Harnesses() {
		dirs := map[string]bool{}
		for _, d := range Dirs(h) {
			dirs[d] = true
		}
		for _, f := range Workspace(h) {
			parent := f.Rel[:strings.LastIndex(f.Rel, "/")+1]
			if parent == "" { // the entry file, at the root
				continue
			}
			parent = strings.TrimSuffix(parent, "/")
			// A skill lives one level deeper than the directory init creates.
			if strings.HasSuffix(f.Rel, "/SKILL.md") {
				parent = parent[:strings.LastIndex(parent, "/")]
			}
			if !dirs[parent] {
				t.Errorf("%s: %s lands in %q, which Dirs() does not create", h.ID, f.Rel, parent)
			}
		}
		for _, s := range Seeds() {
			parent := strings.TrimSuffix(s.Rel[:strings.LastIndex(s.Rel, "/")+1], "/")
			if !dirs[parent] {
				t.Errorf("%s: seed %s lands in %q, which Dirs() does not create", h.ID, s.Rel, parent)
			}
		}
	}
}

func findFile(t *testing.T, h paths.Harness, rel string) File {
	t.Helper()
	for _, f := range Workspace(h) {
		if f.Rel == rel {
			return f
		}
	}
	t.Fatalf("%s: no file at %s", h.ID, rel)
	return File{}
}

func TestArtifactRendersItsData(t *testing.T) {
	data := ArtifactData{Name: "user-auth", Title: "User auth", Autonomy: "auto", CI: "wait"}
	for _, name := range []string{"requirements.md", "design.md", "tasks.md", "plan.md"} {
		out, err := Artifact(name, data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(out, "{{") || strings.Contains(out, "<no value>") {
			t.Errorf("%s: unrendered action or missing key:\n%s", name, out)
		}
		if !strings.Contains(out, "User auth") {
			t.Errorf("%s: title not interpolated", name)
		}
		if !strings.HasSuffix(out, "\n") || strings.HasSuffix(out, "\n\n") {
			t.Errorf("%s: does not end in exactly one newline", name)
		}
	}
}

// The kickoff answers are recorded on the artifact so nobody is asked twice and the
// run stays reproducible from the file.
func TestArtifactRecordsTheKickoffAnswers(t *testing.T) {
	data := ArtifactData{Name: "x", Title: "X", Autonomy: "gated", CI: "no-wait"}
	for _, name := range []string{"requirements.md", "plan.md"} {
		out, err := Artifact(name, data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.HasPrefix(out, "---\n") {
			t.Errorf("%s does not open with frontmatter:\n%s", name, out)
		}
		if !strings.Contains(out, "autonomy: gated") || !strings.Contains(out, "ci: no-wait") {
			t.Errorf("%s did not record the kickoff answers:\n%s", name, out)
		}
	}
}

func TestArtifactRejectsUnknownName(t *testing.T) {
	if _, err := Artifact("nope.md", ArtifactData{}); err == nil {
		t.Error("Artifact of a missing template returned no error")
	}
}

func TestTitle(t *testing.T) {
	cases := map[string]string{
		"user-auth":       "User auth",
		"a":               "A",
		"oauth2-login-v2": "Oauth2 login v2",
		"":                "",
	}
	for in, want := range cases {
		if got := Title(in); got != want {
			t.Errorf("Title(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitMetaRejectsAMalformedHeader(t *testing.T) {
	cases := map[string]string{
		"no header":      "# just prose\n",
		"unterminated":   "---\nname: x\ndescription: y\n",
		"not key-value":  "---\nname: x\ndescription: y\nnonsense\n---\n\nbody\n",
		"no description": "---\nname: x\n---\n\nbody\n",
	}
	for label, raw := range cases {
		if _, err := splitMeta(label, raw); err == nil {
			t.Errorf("%s: splitMeta accepted it", label)
		}
	}
}
