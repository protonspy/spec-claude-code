package assets

import (
	"strings"
	"testing"
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
	raw, err := Content("CLAUDE.md")
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if strings.Contains(raw, "\r") {
		t.Error("CLAUDE.md carries CR after Content()")
	}
}

func TestContentRejectsUnknownTemplate(t *testing.T) {
	if _, err := Content("nope.md"); err == nil {
		t.Error("Content of a missing template returned no error")
	}
}

// The workspace set and the embedded tree must agree in both directions: a File
// pointing at nothing breaks init, and an embedded file no File points at ships in
// the binary and reaches no workspace.
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
	for _, f := range Workspace() {
		if _, err := Content(f.Name); err != nil {
			t.Errorf("Workspace() references %q which is not embedded", f.Name)
		}
		referenced[f.Name] = true
	}
	for _, name := range []string{"requirements.md", "design.md", "tasks.md", "plan.md"} {
		referenced["artifacts/"+name] = true
	}
	for name := range inTree {
		if !referenced[name] {
			t.Errorf("embedded template %q is not in Workspace() and is not an artifact template", name)
		}
	}
}

// Destinations must be unique and slash-separated: they are written verbatim into
// the manifest, and two Files claiming one path means one silently wins.
func TestWorkspaceDestinationsAreUniqueAndSlashed(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range Workspace() {
		if seen[f.Rel] {
			t.Errorf("duplicate destination %q", f.Rel)
		}
		seen[f.Rel] = true
		if strings.Contains(f.Rel, `\`) {
			t.Errorf("destination %q is not slash-separated", f.Rel)
		}
		if strings.HasPrefix(f.Rel, "/") || strings.Contains(f.Rel, "..") {
			t.Errorf("destination %q escapes the workspace root", f.Rel)
		}
	}
}

// The set is sorted by destination so init's report and the manifest are in the
// same order on every run and on every platform.
func TestWorkspaceIsSortedByDestination(t *testing.T) {
	set := Workspace()
	for i := 1; i < len(set); i++ {
		if set[i-1].Rel >= set[i].Rel {
			t.Fatalf("not sorted: %q before %q", set[i-1].Rel, set[i].Rel)
		}
	}
}

// The upgrade path depends on this: a data-free template renders identically in
// every workspace, so its hash identifies its content globally. A template with a
// placeholder in it could not be reconstructed from the manifest alone.
func TestWorkspaceTemplatesAreDataFree(t *testing.T) {
	for _, f := range Workspace() {
		raw, err := Content(f.Name)
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		if i := strings.Index(raw, "{{"); i >= 0 {
			t.Errorf("%s contains a template action at byte %d — workspace templates must be data-free", f.Name, i)
		}
	}
}

// Exactly the two files whose purpose is to be edited are Owned. If a third
// appears, it is either a mistake or a decision that belongs in the design.
func TestOnlyTheUserOwnedFilesAreOwned(t *testing.T) {
	var owned []string
	for _, f := range Workspace() {
		if f.Owned {
			owned = append(owned, f.Rel)
		}
	}
	want := []string{".claude/rules/project.md", "CLAUDE.md"}
	if strings.Join(owned, ",") != strings.Join(want, ",") {
		t.Errorf("owned files = %v, want %v", owned, want)
	}
}

// CLAUDE.md is paid for in context by every session, and accuracy degrades as
// context grows. The rules exist so it can stay short; this is the guard that keeps
// someone from "helpfully" inlining them back into it.
func TestScaffoldedEntryFileStaysShort(t *testing.T) {
	raw, err := Content("CLAUDE.md")
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if n := strings.Count(raw, "\n"); n > 60 {
		t.Errorf("scaffolded CLAUDE.md is %d lines; keep it under 60 and link to .claude/rules/ instead", n)
	}
}

// Every rule the scaffolded CLAUDE.md points at has to exist, or the entry file
// sends the agent to a file that isn't there.
func TestEntryFileLinksResolve(t *testing.T) {
	raw, err := Content("CLAUDE.md")
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	dests := map[string]bool{}
	for _, f := range Workspace() {
		dests[f.Rel] = true
	}
	for _, f := range Workspace() {
		if !strings.HasPrefix(f.Rel, ".claude/rules/") {
			continue
		}
		if !strings.Contains(raw, "("+f.Rel+")") {
			t.Errorf("CLAUDE.md does not link to %s", f.Rel)
		}
	}
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

// The review agents are read by Claude Code's subagent loader, which needs the
// frontmatter contract: a name matching the file and a description saying when to
// use it.
func TestAgentsCarryTheirFrontmatter(t *testing.T) {
	for _, f := range Workspace() {
		if !strings.HasPrefix(f.Rel, ".claude/agents/") {
			continue
		}
		raw, err := Content(f.Name)
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		if !strings.HasPrefix(raw, "---\n") {
			t.Errorf("%s does not open with frontmatter", f.Rel)
		}
		base := strings.TrimSuffix(strings.TrimPrefix(f.Rel, ".claude/agents/"), ".md")
		if !strings.Contains(raw, "\nname: "+base+"\n") {
			t.Errorf("%s: frontmatter name does not match the filename", f.Rel)
		}
		if !strings.Contains(raw, "\ndescription: ") {
			t.Errorf("%s: no description in frontmatter", f.Rel)
		}
	}
}

// Directories init creates on its own must be inside the workspace and
// slash-separated, for the same reason destinations must be.
func TestDirsAreRelativeAndSlashed(t *testing.T) {
	for _, d := range Dirs() {
		if strings.HasPrefix(d, "/") || strings.Contains(d, "..") || strings.Contains(d, `\`) {
			t.Errorf("directory %q is not a slash-separated path inside the workspace", d)
		}
	}
}
