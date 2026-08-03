package scaffold

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/assets"
	"github.com/protonspy/spec-claude-code/internal/manifest"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

func planFor(t *testing.T, root string, h paths.Harness) *UpdatePlan {
	t.Helper()
	plan, err := PlanUpdate(root, h)
	if err != nil {
		t.Fatalf("PlanUpdate: %v", err)
	}
	return plan
}

func action(t *testing.T, plan *UpdatePlan, rel string) UpdateAction {
	t.Helper()
	for _, it := range plan.Items {
		if it.Path == rel {
			return it.Action
		}
	}
	t.Fatalf("no plan item for %s", rel)
	return ""
}

// write puts content at rel and, when recorded is true, tells the manifest that
// is what scc last wrote — the difference between a stale file scc may replace
// and one the user edited.
func writeManaged(t *testing.T, root string, h paths.Harness, rel, content string, recorded bool) {
	t.Helper()
	if err := workspace.AtomicWrite(filepath.Join(root, filepath.FromSlash(rel)), []byte(content), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	if !recorded {
		return
	}
	m, _, err := manifest.Load(root, h)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	m.Set(rel, manifest.Hash(content), "1")
	if err := manifest.Save(root, h, m); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// A workspace scaffolded by this build is already current, in every harness. If
// this fails, `scc update` would offer to rewrite files it just wrote.
func TestPlanOnAFreshWorkspaceIsAllCurrent(t *testing.T) {
	for _, h := range paths.Harnesses() {
		root := t.TempDir()
		applyTo(t, root, h, false)
		plan := planFor(t, root, h)
		if got := plan.Pending(); len(got) != 0 {
			t.Errorf("%s: fresh workspace has pending items: %+v", h.ID, got)
		}
		if plan.Writes(true) {
			t.Errorf("%s: fresh workspace would be written to", h.ID)
		}
	}
}

func TestPlanUpdateNeedsAWorkspace(t *testing.T) {
	if _, err := PlanUpdate(t.TempDir(), paths.Claude); err == nil {
		t.Error("PlanUpdate on a bare directory returned no error")
	}
}

// The four states an update has to tell apart, and the reason the plan is worth
// showing before anything is written.
func TestPlanNamesEveryState(t *testing.T) {
	root := t.TempDir()
	applyTo(t, root, paths.Claude, false)

	deleted := ".claude/rules/specs.md"
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(deleted))); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	// Stale but pristine: the content scc recorded, which is not what it renders now.
	stale := ".claude/rules/routing.md"
	writeManaged(t, root, paths.Claude, stale, "# an older template\n", true)
	// Edited: differs from both the render and the record.
	edited := ".claude/rules/tasks.md"
	writeManaged(t, root, paths.Claude, edited, "# mine\n", false)
	// Owned, and changed by the user — the file whose whole purpose is to be edited.
	owned := paths.Claude.EntryFile
	writeManaged(t, root, paths.Claude, owned, "# my own entry file\n", false)
	// Managed once, not shipped now, still exactly as scc left it.
	gone := ".claude/rules/retired.md"
	writeManaged(t, root, paths.Claude, gone, "# retired\n", true)
	// Same, but the user changed it, so it is theirs to keep.
	goneEdited := ".claude/rules/retired-edited.md"
	writeManaged(t, root, paths.Claude, goneEdited, "# retired\n", true)
	writeManaged(t, root, paths.Claude, goneEdited, "# and then edited\n", false)

	plan := planFor(t, root, paths.Claude)
	want := map[string]UpdateAction{
		deleted:    UpCreate,
		stale:      UpUpdate,
		edited:     UpConflict,
		owned:      UpOwned,
		gone:       UpDelete,
		goneEdited: UpOrphan,
	}
	for rel, w := range want {
		if got := action(t, plan, rel); got != w {
			t.Errorf("%s: action = %q, want %q", rel, got, w)
		}
	}
	if !plan.Writes(false) {
		t.Error("a plan with a create and an update reports no writes")
	}
}

// Applying it: everything scc may touch is brought current, the manifest ends up
// describing what is actually on disk, and a second run has nothing to do.
func TestApplyUpdateBringsTheTreeCurrent(t *testing.T) {
	root := t.TempDir()
	applyTo(t, root, paths.OpenCode, false)
	rel := ".opencode/rules/routing.md"
	writeManaged(t, root, paths.OpenCode, rel, "# an older template\n", true)
	missing := ".opencode/skills/wiki/SKILL.md"
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(missing))); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	plan := planFor(t, root, paths.OpenCode)
	res, err := ApplyUpdate(root, paths.OpenCode, plan, UpdateOptions{SCCVersion: "v0.0.0-test"})
	if err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	if len(res.Applied) != 2 {
		t.Errorf("applied = %+v, want the stale file and the missing one", res.Applied)
	}
	for _, r := range []string{rel, missing} {
		want := renderRelFor(t, paths.OpenCode, r)
		if got := read(t, root, r); got != want {
			t.Errorf("%s was not brought current", r)
		}
	}
	m, _, err := manifest.Load(root, paths.OpenCode)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, e := range m.Files {
		if e.Version != assets.Version {
			t.Errorf("%s recorded version %q, want %q", e.Path, e.Version, assets.Version)
		}
	}
	if got := planFor(t, root, paths.OpenCode); len(got.Pending()) != 0 {
		t.Errorf("a second update still has work: %+v", got.Pending())
	}
}

// The product's first rule: never author over what the user owns. An edited file
// survives an update, and keeps the version it was recorded at — that recorded
// version is the base a later merge would need, and claiming today's would
// describe content scc never wrote.
func TestApplyUpdateKeepsEditedFilesUnlessForced(t *testing.T) {
	root := t.TempDir()
	applyTo(t, root, paths.Claude, false)
	edited := ".claude/rules/verification.md"
	mine := "# mine, and I meant it\n"
	writeManaged(t, root, paths.Claude, edited, mine, false)
	entry := paths.Claude.EntryFile
	myEntry := "# my own entry file\n"
	writeManaged(t, root, paths.Claude, entry, myEntry, false)

	res, err := ApplyUpdate(root, paths.Claude, planFor(t, root, paths.Claude), UpdateOptions{SCCVersion: "v0.0.0-test"})
	if err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	if got := read(t, root, edited); got != mine {
		t.Error("an update overwrote an edited file without --force")
	}
	if got := read(t, root, entry); got != myEntry {
		t.Error("an update overwrote a user-owned file")
	}
	if len(res.Kept) != 2 {
		t.Errorf("kept = %+v, want the edited file and the owned one", res.Kept)
	}
	m, _, err := manifest.Load(root, paths.Claude)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if e, ok := m.Get(edited); !ok || e.Hash == manifest.Hash(mine) {
		t.Errorf("the kept file's entry was rewritten to its edited content: %+v", e)
	}

	// --force is the separate, explicit decision — the only one that destroys work.
	forced, err := ApplyUpdate(root, paths.Claude, planFor(t, root, paths.Claude), UpdateOptions{
		SCCVersion: "v0.0.0-test", Force: true,
	})
	if err != nil {
		t.Fatalf("ApplyUpdate --force: %v", err)
	}
	if got := read(t, root, edited); got != renderRelFor(t, paths.Claude, edited) {
		t.Error("--force did not take the new template")
	}
	if got := read(t, root, entry); got != myEntry {
		t.Error("--force overwrote a user-owned file, which it must never do")
	}
	if len(forced.Applied) != 1 {
		t.Errorf("applied = %+v, want only the forced file", forced.Applied)
	}
}

// A file this version no longer ships is removed when it is still pristine, and
// kept — but untracked — when the user changed it.
func TestApplyUpdateDropsUnmanagedFiles(t *testing.T) {
	root := t.TempDir()
	applyTo(t, root, paths.Claude, false)
	gone := ".claude/rules/retired.md"
	writeManaged(t, root, paths.Claude, gone, "# retired\n", true)
	kept := ".claude/rules/retired-edited.md"
	writeManaged(t, root, paths.Claude, kept, "# retired\n", true)
	writeManaged(t, root, paths.Claude, kept, "# and then edited\n", false)

	if _, err := ApplyUpdate(root, paths.Claude, planFor(t, root, paths.Claude), UpdateOptions{SCCVersion: "v0.0.0-test"}); err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(gone))); !os.IsNotExist(err) {
		t.Error("a pristine file scc no longer ships was left behind")
	}
	if got := read(t, root, kept); got != "# and then edited\n" {
		t.Error("an edited file scc no longer ships was deleted")
	}
	m, _, err := manifest.Load(root, paths.Claude)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, rel := range []string{gone, kept} {
		if _, ok := m.Get(rel); ok {
			t.Errorf("%s is still in the manifest", rel)
		}
	}
}

// A skill that did not exist when the workspace was scaffolded reaches it through
// `scc update`, in every harness that has the surface for it.
//
// This is the path a new skill actually ships on, and it is the one with something
// to get wrong: the destination is a directory that does not exist yet, so a create
// that only wrote files would fail on precisely the workspaces the update is for.
// Nothing here is specific to plan-run — it is read out of assets.WorkflowSkills, so
// the next workflow skill is covered the day it is added.
func TestApplyUpdateAddsASkillTheWorkspacePredates(t *testing.T) {
	for _, h := range paths.Harnesses() {
		root := t.TempDir()
		applyTo(t, root, h, false)

		// Roll the workspace back to before the skill existed: the files gone, and
		// no manifest entry claiming scc ever wrote them.
		var rels []string
		for _, skill := range assets.WorkflowSkills {
			rels = append(rels, path.Join(h.Dir, h.SkillsSeg, skill, "SKILL.md"))
			if h.CommandsSeg != "" {
				rels = append(rels, path.Join(h.Dir, h.CommandsSeg, "scc-"+skill+".md"))
			}
		}
		m, _, err := manifest.Load(root, h)
		if err != nil {
			t.Fatalf("%s: Load: %v", h.ID, err)
		}
		for _, rel := range rels {
			if err := os.RemoveAll(filepath.Dir(filepath.Join(root, filepath.FromSlash(rel)))); err != nil {
				t.Fatalf("%s: %v", h.ID, err)
			}
			m.Remove(rel)
		}
		if err := manifest.Save(root, h, m); err != nil {
			t.Fatalf("%s: Save: %v", h.ID, err)
		}

		plan := planFor(t, root, h)
		for _, rel := range rels {
			if got := action(t, plan, rel); got != UpCreate {
				t.Errorf("%s: %s planned as %q, want %q", h.ID, rel, got, UpCreate)
			}
		}
		if _, err := ApplyUpdate(root, h, plan, UpdateOptions{SCCVersion: "v0.0.0-test"}); err != nil {
			t.Fatalf("%s: ApplyUpdate: %v", h.ID, err)
		}

		after, _, err := manifest.Load(root, h)
		if err != nil {
			t.Fatalf("%s: Load: %v", h.ID, err)
		}
		for _, rel := range rels {
			f := findWorkspaceFile(t, h, rel)
			want, err := assets.Render(h, f)
			if err != nil {
				t.Fatalf("%s: Render %s: %v", h.ID, rel, err)
			}
			if got := read(t, root, rel); got != want {
				t.Errorf("%s: %s is not what this version renders", h.ID, rel)
			}
			e, ok := after.Get(rel)
			if !ok {
				t.Errorf("%s: %s was written and not recorded", h.ID, rel)
				continue
			}
			if e.Version != assets.Version {
				t.Errorf("%s: %s recorded at version %q, want %q", h.ID, rel, e.Version, assets.Version)
			}
		}
		if plan := planFor(t, root, h); plan.Writes(true) {
			t.Errorf("%s: a second update would write again", h.ID)
		}
	}
}

func findWorkspaceFile(t *testing.T, h paths.Harness, rel string) assets.File {
	t.Helper()
	for _, f := range assets.Workspace(h) {
		if f.Rel == rel {
			return f
		}
	}
	t.Fatalf("%s ships no file at %s", h.ID, rel)
	return assets.File{}
}
