package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func runValidator(t *testing.T, fn func(string) (*finding.Set, error), root string) []string {
	t.Helper()
	set, err := fn(root)
	if err != nil {
		t.Fatalf("validator: %v", err)
	}
	return rules(set)
}

// Every knowledge-base validator is silent when its subject does not exist. That is what
// lets `scc validate` run all eight unconditionally instead of asking the user which
// apply.
func TestKnowledgeValidatorsAreSilentOnAnEmptyWorkspace(t *testing.T) {
	root := t.TempDir()
	for name, fn := range map[string]func(string) (*finding.Set, error){
		"wiki":     Wiki,
		"adr":      ADR,
		"glossary": Glossary,
		"stack":    Stack,
		"codewiki": Codewiki,
	} {
		if got := runValidator(t, fn, root); len(got) != 0 {
			t.Errorf("%s produced findings on an empty workspace: %v", name, got)
		}
	}
}

func TestWikiGraphChecks(t *testing.T) {
	root := t.TempDir()
	wiki := paths.Wiki(root)
	write(t, filepath.Join(wiki, paths.WikiIndex), "# Index\n\n- [[order-total]]\n- [[nowhere]]\n")
	write(t, filepath.Join(wiki, "order-total.md"), "# Order total\n\nThe amount charged.\n")
	write(t, filepath.Join(wiki, "unlinked.md"), "# Unlinked\n\nNobody links here.\n")
	write(t, filepath.Join(wiki, paths.WikiLog), "# Changelog\n\n- added [[order-total]]\n- added [[deleted-page]]\n")

	got := runValidator(t, Wiki, root)
	for _, want := range []string{"wiki.broken-link", "wiki.orphan-page", "wiki.changelog-desync"} {
		if !contains(got, want) {
			t.Errorf("rules = %v, want %s", got, want)
		}
	}
	// The index and the changelog are never orphans: the index is the entry point, and
	// the log is a log rather than a page.
	set, _ := Wiki(root)
	for _, f := range set.Sorted() {
		if f.Rule == "wiki.orphan-page" && !strings.Contains(f.File, "unlinked") {
			t.Errorf("orphan reported against %s, want only the unlinked page", f.File)
		}
	}
}

// Reachability is transitive: a page linked from a page linked from the index is not an
// orphan.
func TestWikiReachabilityIsTransitive(t *testing.T) {
	root := t.TempDir()
	wiki := paths.Wiki(root)
	write(t, filepath.Join(wiki, paths.WikiIndex), "# Index\n\n- [[one]]\n")
	write(t, filepath.Join(wiki, "one.md"), "# One\n\nsee [[two]]\n")
	write(t, filepath.Join(wiki, "two.md"), "# Two\n\nsee [[three|the third]]\n")
	write(t, filepath.Join(wiki, "three.md"), "# Three\n\nend of the chain\n")
	write(t, filepath.Join(wiki, paths.WikiLog), "# Changelog\n")

	if got := runValidator(t, Wiki, root); len(got) != 0 {
		t.Errorf("a fully reachable wiki produced findings: %v", got)
	}
}

func TestWikiMissingIndexAndChangelog(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(paths.Wiki(root), "lonely.md"), "# Lonely\n")
	got := runValidator(t, Wiki, root)
	for _, want := range []string{"wiki.missing-index", "wiki.missing-changelog"} {
		if !contains(got, want) {
			t.Errorf("rules = %v, want %s", got, want)
		}
	}
}

// raw/ is a drop box, not storage: a file still sitting there was collected and never
// processed. A dotfile is how an empty directory gets committed, which is the opposite.
func TestRawIsADropBox(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(paths.Raw(root), "paper.pdf.md"), "raw notes\n")
	write(t, filepath.Join(paths.Raw(root), ".gitkeep"), "")
	got := runValidator(t, Wiki, root)
	if !contains(got, "wiki.unprocessed-source") {
		t.Errorf("rules = %v, want wiki.unprocessed-source", got)
	}
	if len(got) != 1 {
		t.Errorf("rules = %v, want exactly one — the dotfile is not unprocessed work", got)
	}
}

func TestADRChecks(t *testing.T) {
	root := t.TempDir()
	adr := paths.ADR(root)
	write(t, filepath.Join(adr, "0001-use-sqlite.md"), "---\nstatus: superseded\nsuperseded-by: 0002-use-redis\n---\n\n# 0001 · SQLite\n")
	write(t, filepath.Join(adr, "0002-use-redis.md"), "---\nstatus: accepted\n---\n\n# 0002 · Redis\n\nreplaces adr:0001-use-sqlite\n")
	if got := runValidator(t, ADR, root); len(got) != 0 {
		t.Errorf("a conforming ADR set produced findings: %v", got)
	}
}

// A superseded record is marked, never edited — and the mark has to be complete in both
// directions or it records nothing.
func TestADRSupersedingMustBeComplete(t *testing.T) {
	root := t.TempDir()
	adr := paths.ADR(root)
	write(t, filepath.Join(adr, "0001-a.md"), "---\nstatus: superseded\n---\n\n# 0001\n")
	write(t, filepath.Join(adr, "0002-b.md"), "---\nstatus: accepted\nsuperseded-by: 0009-ghost\n---\n\n# 0002\n")
	got := runValidator(t, ADR, root)
	for _, want := range []string{"adr.superseded-without-successor", "adr.successor-without-status", "adr.unknown-successor"} {
		if !contains(got, want) {
			t.Errorf("rules = %v, want %s", got, want)
		}
	}
}

// Unlike requirement numbering, an ADR gap means a record went missing: ADRs are never
// removed, so there is no legitimate hole.
func TestADRNumberingAndNames(t *testing.T) {
	root := t.TempDir()
	adr := paths.ADR(root)
	write(t, filepath.Join(adr, "0001-first.md"), "---\nstatus: accepted\n---\n\n# 0001\n")
	write(t, filepath.Join(adr, "0003-third.md"), "---\nstatus: accepted\n---\n\n# 0003\n")
	write(t, filepath.Join(adr, "notes.md"), "---\nstatus: accepted\n---\n\n# not an ADR name\n")
	got := runValidator(t, ADR, root)
	for _, want := range []string{"adr.numbering-gap", "adr.malformed-filename"} {
		if !contains(got, want) {
			t.Errorf("rules = %v, want %s", got, want)
		}
	}
	// One gap finding, not one per record after the gap.
	if n := strings.Count(strings.Join(got, " "), "adr.numbering-gap"); n != 1 {
		t.Errorf("numbering-gap reported %d times, want 1", n)
	}
}

// A citation to a record that does not exist is broken whether or not docs/adr/ has any
// records at all, so it is reported either way.
func TestADRCitationsAreCheckedWithNoRecords(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(paths.Wiki(root), "page.md"), "# Page\n\ndecided in adr:0003-nowhere\n")
	if got := runValidator(t, ADR, root); !contains(got, "adr.unknown-citation") {
		t.Errorf("rules = %v, want adr.unknown-citation", got)
	}
}

func TestADRStatusAndCitations(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(paths.ADR(root), "0001-first.md"), "# 0001\n\nno frontmatter status\n")
	write(t, filepath.Join(paths.Wiki(root), "page.md"), "# Page\n\ndecided in adr:0007-does-not-exist\n")
	got := runValidator(t, ADR, root)
	for _, want := range []string{"adr.missing-status", "adr.unknown-citation"} {
		if !contains(got, want) {
			t.Errorf("rules = %v, want %s", got, want)
		}
	}
}

func TestGlossaryChecks(t *testing.T) {
	root := t.TempDir()
	write(t, paths.Glossary(root), `# Glossary

- **order total** — the amount charged, in minor units. Avoid: grand total, sum
- **workspace** — a directory holding the manifest. Avoid: project root
`)
	write(t, filepath.Join(paths.Wiki(root), "page.md"), `# Page

The grand total is computed here.

The summary assumes a consumer of the order total.
`)
	got := runValidator(t, Glossary, root)
	if !contains(got, "glossary.avoided-synonym") {
		t.Errorf("rules = %v, want glossary.avoided-synonym for 'grand total'", got)
	}
	// Whole-word matching only. "sum" inside "summary", "assumes", and "consumer" must
	// not fire — substring matching here is a false-positive generator, and one wrong
	// finding costs the user's trust in all eight validators.
	if n := len(got); n != 1 {
		set, _ := Glossary(root)
		t.Errorf("rules = %v, want exactly one; findings: %+v", got, set.Sorted())
	}
}

func TestGlossaryDuplicateAndContradiction(t *testing.T) {
	root := t.TempDir()
	write(t, paths.Glossary(root), `# Glossary

- **order total** — the amount charged. Avoid: sum
- **order total** — defined twice
- **sum** — also canonical, which contradicts the entry above
`)
	got := runValidator(t, Glossary, root)
	for _, want := range []string{"glossary.duplicate-term", "glossary.synonym-is-canonical"} {
		if !contains(got, want) {
			t.Errorf("rules = %v, want %s", got, want)
		}
	}
}

func TestStackChecks(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), `module example.com/app

go 1.25.0

require (
	github.com/go-chi/chi/v5 v5.0.0
	github.com/undocumented/thing v1.0.0
	golang.org/x/sync v0.7.0 // indirect
)
`)
	write(t, paths.Stack(root), "# Stack\n\n- chi — the router, because net/http's mux could not do it\n")
	got := runValidator(t, Stack, root)
	if !contains(got, "stack.undocumented-dependency") {
		t.Errorf("rules = %v, want stack.undocumented-dependency", got)
	}
	// chi is documented by its base name, and the indirect dependency is not a decision
	// anybody made — so exactly one finding.
	if len(got) != 1 {
		set, _ := Stack(root)
		t.Errorf("rules = %v, want exactly one; findings: %+v", got, set.Sorted())
	}
}

func TestStackReadsPackageJSON(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "package.json"), `{
  "dependencies": {"react": "^18.0.0"},
  "devDependencies": {"vitest": "^1.0.0"}
}`)
	write(t, paths.Stack(root), "# Stack\n\n- react — the UI layer\n")
	got := runValidator(t, Stack, root)
	if len(got) != 1 || !contains(got, "stack.undocumented-dependency") {
		t.Errorf("rules = %v, want one undocumented dependency (vitest)", got)
	}
}

// A manifest scc cannot parse produces no findings. It is not scc's place to report on
// the syntax of somebody else's build file, and a guess here is a finding that is wrong.
func TestStackIsSilentOnAnUnparseableManifest(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "package.json"), "{ not json")
	if got := runValidator(t, Stack, root); len(got) != 0 {
		t.Errorf("rules = %v, want silence on an unparseable manifest", got)
	}
}

func TestStackMissingFile(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module example.com/app\n\nrequire github.com/a/b v1.0.0\n")
	got := runValidator(t, Stack, root)
	if len(got) != 1 || !contains(got, "stack.missing") {
		t.Errorf("rules = %v, want exactly stack.missing", got)
	}
}

func TestCodewikiCitations(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "main.go"), "one\ntwo\nthree\nfour\nfive\n")
	write(t, filepath.Join(paths.Codewiki(root), "app.md"), `# App

## How it starts

[main.go:1-3]()

Prose about the start.

## How it ends

[main.go:4-99]()

## What it cites nowhere

Prose with no citation.

## Where the file went

[gone.go:1-2]()
`)
	got := runValidator(t, Codewiki, root)
	for _, want := range []string{"codewiki.citation-out-of-range", "codewiki.section-cites-nothing", "codewiki.citation-unresolved"} {
		if !contains(got, want) {
			t.Errorf("rules = %v, want %s", got, want)
		}
	}
	// The resolving citation produced nothing, and the page title (level 1) is not a
	// section that owes a citation.
	if len(got) != 3 {
		set, _ := Codewiki(root)
		t.Errorf("rules = %v, want exactly three; findings: %+v", got, set.Sorted())
	}
}

func TestCodewikiHeadingsAndRanges(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "main.go"), "one\ntwo\n")
	write(t, filepath.Join(paths.Codewiki(root), "app.md"), `# App

## Notes

[main.go:2-1]()

## Notes

[main.go:1]()
`)
	got := runValidator(t, Codewiki, root)
	for _, want := range []string{"codewiki.duplicate-heading", "codewiki.citation-invalid"} {
		if !contains(got, want) {
			t.Errorf("rules = %v, want %s", got, want)
		}
	}
}

// An ordinary link is not a citation, and a citation-shaped example in a fence is not
// either.
func TestCodewikiIgnoresWhatIsNotACitation(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "main.go"), "one\n")
	write(t, filepath.Join(paths.Codewiki(root), "app.md"), "# App\n\n## Section\n\n"+
		"[main.go:1]() and [a real link](https://example.com) and [another](../wiki/index.md)\n\n"+
		"```\n[does-not-exist.go:1-5]()\n```\n")
	if got := runValidator(t, Codewiki, root); len(got) != 0 {
		t.Errorf("rules = %v, want silence", got)
	}
}

func TestEverythingMergesAndCounts(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(paths.Wiki(root), "lonely.md"), "# Lonely\n")
	writeSpec(t, root, "billing", goodRequirements, goodDesign, "# tasks\n\n- [ ] 1.1 No methodology — R1.1\n")

	set, results, err := Everything(root)
	if err != nil {
		t.Fatalf("Everything: %v", err)
	}
	if len(results) != len(All()) {
		t.Fatalf("results = %+v, want one per validator", results)
	}
	byName := map[string]int{}
	total := 0
	for _, r := range results {
		byName[r.Name] = r.Findings
		total += r.Findings
	}
	if byName["wiki"] == 0 || byName["spec"] == 0 {
		t.Errorf("per-validator counts wrong: %+v", results)
	}
	if total != set.Len() {
		t.Errorf("per-validator counts sum to %d, merged set has %d", total, set.Len())
	}
	if set.ExitCode() != finding.ExitFindings {
		t.Errorf("exit = %d, want %d", set.ExitCode(), finding.ExitFindings)
	}
	// Results are named and ordered, so two runs report identically.
	for i := 1; i < len(results); i++ {
		if results[i-1].Name >= results[i].Name {
			t.Fatalf("results not sorted by name: %+v", results)
		}
	}
}

// A citation is a path inside the checkout. A traversing target must be refused before
// it reaches os.ReadFile, or a Markdown page decides what scc opens on the machine.
func TestCodewikiCitationsCannotEscapeTheWorkspace(t *testing.T) {
	root := t.TempDir()
	// A real file one level above the workspace, which the traversal below would reach.
	write(t, filepath.Join(root, "..", "outside-the-workspace.txt"), "secret\nsecret\n")
	defer os.Remove(filepath.Join(root, "..", "outside-the-workspace.txt"))

	for _, target := range []string{
		"../outside-the-workspace.txt",
		"docs/../../outside-the-workspace.txt",
	} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, "..", "outside-the-workspace.txt"), "secret\nsecret\n")
			defer os.Remove(filepath.Join(dir, "..", "outside-the-workspace.txt"))
			write(t, filepath.Join(paths.Codewiki(dir), "app.md"),
				"# App\n\n## Section\n\n["+target+":1-2]()\n")

			got := runValidator(t, Codewiki, dir)
			if !contains(got, "codewiki.citation-invalid") {
				t.Errorf("rules = %v, want codewiki.citation-invalid for %q", got, target)
			}
			// The give-away that the read happened anyway: a resolved citation is
			// range-checked, so out-of-range means scc opened the outside file.
			if contains(got, "codewiki.citation-out-of-range") {
				t.Errorf("rules = %v: %q was read despite escaping the workspace", got, target)
			}
		})
	}
}

// An absolute citation escapes just as surely, and filepath.Join would silently graft
// it onto root on Windows while honoring it on Unix.
func TestCodewikiCitationsCannotBeAbsolute(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(paths.Codewiki(root), "app.md"),
		"# App\n\n## Section\n\n[/etc/passwd:1-2]()\n")

	got := runValidator(t, Codewiki, root)
	if !contains(got, "codewiki.citation-invalid") {
		t.Errorf("rules = %v, want codewiki.citation-invalid", got)
	}
}

// baseName drops a Go major-version suffix so the last real path segment is what gets
// compared against stack.md. Every major from 2 up is a valid suffix, and the two-digit
// ones are the easy range to lose: a leading-digit character class that starts at 2
// silently rejects v10 through v19.
func TestBaseNameDropsEveryMajorVersionSuffix(t *testing.T) {
	for _, tc := range []struct{ module, want string }{
		{"github.com/go-chi/chi/v5", "chi"},
		{"github.com/go-chi/chi/v2", "chi"},
		{"github.com/go-chi/chi/v9", "chi"},
		{"github.com/go-chi/chi/v10", "chi"},
		{"github.com/go-chi/chi/v11", "chi"},
		{"github.com/go-chi/chi/v19", "chi"},
		{"github.com/go-chi/chi/v20", "chi"},
		{"github.com/go-chi/chi/v100", "chi"},
		// v1 is never written as a path suffix, and v0 is not a major version. Neither
		// is a segment that merely starts with v.
		{"github.com/go-chi/chi/v1", "v1"},
		{"github.com/go-chi/chi/v0", "v0"},
		{"github.com/spf13/viper", "viper"},
		{"gopkg.in/yaml.v3", "yaml.v3"},
	} {
		if got := baseName(tc.module); got != tc.want {
			t.Errorf("baseName(%q) = %q, want %q", tc.module, got, tc.want)
		}
	}
}
