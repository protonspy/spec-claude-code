package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/protonspy/spec-claude-code/internal/codegraph"
	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/git"
)

// The Stop stage stopped re-indexing on every turn: syncGraph compares the graph's
// mtime against the files this branch touched — a stat per changed file rather
// than a walk — and skips when nothing is newer.
//
// It errs toward syncing, and the reason is asymmetric: an unnecessary index costs
// a subprocess, while a skipped one leaves the agent an index that answers
// confidently about code that changed.
func TestStaleErrsTowardSyncing(t *testing.T) {
	dir := t.TempDir()

	// No graph to stat at all: nothing to be stale about, and the answer is still
	// "sync" rather than a claim the index is current.
	if !stale(dir, []git.Change{{Path: "a.go"}}) {
		t.Error("a workspace with no graph reported a current index")
	}

	graph := filepath.Join(dir, codegraph.Dir)
	if err := os.MkdirAll(graph, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// No list of changed files: the question cannot be answered, so sync.
	if !stale(dir, nil) {
		t.Error("an unanswerable question reported a current index")
	}

	// A file older than the index: nothing to do.
	old := filepath.Join(dir, "old.go")
	if err := os.WriteFile(old, []byte("package x\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	if stale(dir, []git.Change{{Path: "old.go"}}) {
		t.Error("a file older than the index reported the index stale")
	}

	// A file the branch touched that no longer exists is skipped rather than
	// counted: a deleted file cannot be newer than anything.
	if stale(dir, []git.Change{{Path: "gone.go"}}) {
		t.Error("a file that is not there reported the index stale")
	}

	// And one written after the index is exactly what a sync is for.
	fresh := filepath.Join(dir, "fresh.go")
	if err := os.WriteFile(fresh, []byte("package x\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(fresh, future, future); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	if !stale(dir, []git.Change{{Path: "old.go"}, {Path: "fresh.go"}}) {
		t.Error("a file newer than the index did not report it stale")
	}
}

// The Stop stage reports findings, and it intersects them with what this branch
// touched: a migrated plan or a stale ADR the task never went near asked the
// agent, every turn, to go and fix a file it had no business in.
//
// Its answer when git cannot say is the whole set, unfiltered — the filter exists
// to stop it nagging about work nobody here did, and a workspace where the
// question cannot be asked is one where every finding is as likely as not to be
// this session's.
func TestTouchedHereFallsBackToTheWholeSet(t *testing.T) {
	set := &finding.Set{}
	set.Addf("plans/other.md", 3, "plan.unknown-section", "a section nobody agreed to")
	set.Addf("docs/adr/0001-x.md", 1, "adr.no-status", "no status")

	// Not a repository: the question cannot be asked, so nothing is filtered out.
	if got := touchedHere(t.TempDir(), set); got.Len() != set.Len() {
		t.Errorf("outside a repository the set was filtered to %d of %d", got.Len(), set.Len())
	}

	// An empty set stays empty rather than becoming nil, so the caller's Len() and
	// Sorted() keep working.
	empty := &finding.Set{}
	if got := touchedHere(t.TempDir(), empty); got == nil || !got.Empty() {
		t.Errorf("an empty set came back as %+v", got)
	}
	if got := touchedHere(t.TempDir(), nil); got != nil {
		t.Errorf("a nil set came back as %+v", got)
	}
}

// A finding that quotes a test log contributes one row to the report rather than
// twelve: this is context the agent pays for on its next turn.
func TestFirstLineClipsAMultiLineMessage(t *testing.T) {
	if got := firstLine("one line only"); got != "one line only" {
		t.Errorf("firstLine = %q", got)
	}
	got := firstLine("the gate failed\n--- FAIL: TestX\n    x_test.go:1: nope\n")
	if got != "the gate failed" {
		t.Errorf("firstLine = %q, want just the first line", got)
	}
	if firstLine("") != "" {
		t.Error("firstLine invented text for an empty message")
	}
}
