package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// The hash is taken over LF-normalized text, so the same template checked out
// with CRLF on Windows hashes identically to the LF copy on Linux. Without this,
// every managed file reads as edited on one of the two platforms.
func TestHashIgnoresLineEndingsAndBOM(t *testing.T) {
	lf := "line one\nline two\n"
	crlf := "line one\r\nline two\r\n"
	// Built from the code point so the byte sequence never appears literally in
	// this source file — gofmt rejects a BOM outside position zero.
	bom := string(rune(0xFEFF)) + lf
	if Hash(lf) != Hash(crlf) {
		t.Errorf("CRLF hashes differently from LF: %s vs %s", Hash(crlf), Hash(lf))
	}
	if Hash(lf) != Hash(bom) {
		t.Errorf("BOM changes the hash: %s vs %s", Hash(bom), Hash(lf))
	}
}

func TestHashIsStableAndDistinguishing(t *testing.T) {
	if Hash("a") != Hash("a") {
		t.Error("Hash is not deterministic")
	}
	if Hash("a") == Hash("b") {
		t.Error("different content hashed identically")
	}
	if len(Hash("a")) != 64 {
		t.Errorf("hash length = %d, want 64 hex chars", len(Hash("a")))
	}
}

// Byte-identical output on every machine is what keeps an upgrade's diff limited
// to what actually changed. Insertion order must not survive into the file.
func TestBytesAreDeterministicAndSorted(t *testing.T) {
	a := New("v1.0.0", paths.Claude)
	a.Set("specs/z.md", Hash("z"), "v1")
	a.Set(".claude/rules/routing.md", Hash("r"), "v1")

	b := New("v1.0.0", paths.Claude)
	b.Set(".claude/rules/routing.md", Hash("r"), "v1")
	b.Set("specs/z.md", Hash("z"), "v1")

	ab, err := a.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	bb, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if string(ab) != string(bb) {
		t.Errorf("insertion order leaked into the output:\n%s\n---\n%s", ab, bb)
	}
	if i, j := strings.Index(string(ab), "routing.md"), strings.Index(string(ab), "specs/z.md"); i > j {
		t.Errorf("entries not sorted by path:\n%s", ab)
	}
	if !strings.HasSuffix(string(ab), "}\n") {
		t.Errorf("output does not end in exactly one LF-terminated brace: %q", string(ab))
	}
	if strings.Contains(string(ab), "\r") {
		t.Error("output contains CR")
	}
}

// A manifest written on Windows is read on Linux — a committed workspace is a
// shared workspace — so the recorded separator cannot be the host's.
func TestPathsAreStoredSlashSeparated(t *testing.T) {
	m := New("v1.0.0", paths.Claude)
	m.Set(filepath.Join(".claude", "rules", "routing.md"), Hash("r"), "v1")
	b, err := m.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if !strings.Contains(string(b), `".claude/rules/routing.md"`) {
		t.Errorf("path not stored slash-separated:\n%s", b)
	}
	if strings.Contains(string(b), `\\`) {
		t.Errorf("path stored with escaped backslashes:\n%s", b)
	}
}

func TestEmptyManifestSerializesFilesAsArray(t *testing.T) {
	b, err := New("v1.0.0", paths.Claude).Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if !strings.Contains(string(b), `"files": []`) {
		t.Errorf("empty manifest did not serialize files as []:\n%s", b)
	}
}

// The manifest is the only state scc has, so a field this build does not
// understand is state an older binary must not delete on the next write.
func TestUnknownFieldsSurviveARoundTrip(t *testing.T) {
	in := `{
  "scc": "v9.9.9",
  "future": {"nested": true},
  "files": [
    {"path": "a.md", "hash": "deadbeef", "version": "v9", "provenance": "generated"}
  ]
}`
	var m Manifest
	if err := json.Unmarshal([]byte(in), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	out, err := m.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	for _, want := range []string{`"future"`, `"nested"`, `"provenance"`, `"generated"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("unknown field %s dropped:\n%s", want, out)
		}
	}
}

func TestGetSetRemove(t *testing.T) {
	m := New("v1.0.0", paths.Claude)
	if _, ok := m.Get("a.md"); ok {
		t.Error("Get on an empty manifest reported a hit")
	}
	m.Set("a.md", "h1", "v1")
	m.Set("a.md", "h2", "v2") // replaces, never duplicates
	if len(m.Files) != 1 {
		t.Fatalf("Set duplicated an entry: %+v", m.Files)
	}
	e, ok := m.Get("a.md")
	if !ok || e.Hash != "h2" || e.Version != "v2" {
		t.Errorf("Get = (%+v, %v), want the replaced entry", e, ok)
	}
	if !m.Remove("a.md") || len(m.Files) != 0 {
		t.Errorf("Remove left %d entries", len(m.Files))
	}
	if m.Remove("a.md") {
		t.Error("Remove of an absent path reported a removal")
	}
}

// Status is the check that stands between an upgrade and the user's authored
// work, so all three answers are load-bearing.
func TestStatusDistinguishesPristineEditedMissing(t *testing.T) {
	root := t.TempDir()
	pristine := "the shipped template\n"
	rel := ".claude/rules/routing.md"
	m := New("v1.0.0", paths.Claude)
	m.Set(rel, Hash(pristine), "v1.0.0")
	e, _ := m.Get(rel)

	if got, err := m.Status(root, e); err != nil || got != Missing {
		t.Errorf("Status before the write = (%v, %v), want missing", got, err)
	}

	target := filepath.Join(root, filepath.FromSlash(rel))
	if err := workspace.AtomicWrite(target, []byte(pristine), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	if got, err := m.Status(root, e); err != nil || got != Pristine {
		t.Errorf("Status of the shipped content = (%v, %v), want pristine", got, err)
	}

	// A CRLF checkout of an untouched file is still untouched.
	if err := workspace.AtomicWrite(target, []byte("the shipped template\r\n"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	if got, err := m.Status(root, e); err != nil || got != Pristine {
		t.Errorf("Status of a CRLF checkout = (%v, %v), want pristine", got, err)
	}

	if err := workspace.AtomicWrite(target, []byte(pristine+"my own note\n"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	if got, err := m.Status(root, e); err != nil || got != Edited {
		t.Errorf("Status of an edited file = (%v, %v), want edited", got, err)
	}
}

// A missing manifest is "not a workspace", which is a different answer from "an
// empty workspace" — and Find/IsWorkspace depend on the distinction.
func TestLoadOnADirectoryWithoutAManifest(t *testing.T) {
	m, found, err := Load(t.TempDir(), paths.Claude)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if found {
		t.Error("found = true for a directory with no manifest")
	}
	if len(m.Files) != 0 {
		t.Errorf("entries = %d, want 0", len(m.Files))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	m := New("v1.2.3", paths.Claude)
	m.Set(".claude/rules/routing.md", Hash("r"), "v1.2.3")
	m.Set(".claude/CLAUDE.md", Hash("c"), "v1.0.0")
	if err := Save(root, paths.Claude, m); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !workspace.IsWorkspace(root) {
		t.Error("Save did not create the workspace marker")
	}
	got, found, err := Load(root, paths.Claude)
	if err != nil || !found {
		t.Fatalf("Load = (%v, %v), want found", found, err)
	}
	if got.SCC != "v1.2.3" || len(got.Files) != 2 {
		t.Fatalf("round trip lost data: %+v", got)
	}
	e, ok := got.Get(".claude/rules/routing.md")
	if !ok || e.Hash != Hash("r") || e.Version != "v1.2.3" {
		t.Errorf("entry = (%+v, %v), want the saved entry", e, ok)
	}
}

// The harness is load-bearing state rather than a diagnostic like the scc
// version: an upgrade re-renders the recorded template version to reconstruct the
// merge base, and a template rendered for the wrong harness produces a base the
// file never had — which is how a merge silently clobbers.
func TestHarnessRoundTripsAndIsPerHarness(t *testing.T) {
	root := t.TempDir()
	for _, h := range paths.Harnesses() {
		m := New("v1.2.3", h)
		m.Set(h.Dir+"/rules/routing.md", Hash("r"), "5")
		if err := Save(root, h, m); err != nil {
			t.Fatalf("%s: Save: %v", h.ID, err)
		}
	}
	for _, h := range paths.Harnesses() {
		got, found, err := Load(root, h)
		if err != nil || !found {
			t.Fatalf("%s: Load = (%v, %v), want found", h.ID, found, err)
		}
		if got.Harness != h.ID {
			t.Errorf("%s: harness = %q, want %q", h.ID, got.Harness, h.ID)
		}
		if _, ok := got.Get(h.Dir + "/rules/routing.md"); !ok {
			t.Errorf("%s: manifest does not carry its own harness's entry", h.ID)
		}
	}
}

// Save writes what Bytes reports, so a caller can compare against the file on
// disk and skip an identical write — which is what makes a second `scc init` a
// no-op rather than a rewrite.
func TestSaveWritesExactlyBytes(t *testing.T) {
	root := t.TempDir()
	m := New("v1.0.0", paths.Claude)
	m.Set("a.md", Hash("a"), "v1.0.0")
	want, err := m.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if err := Save(root, paths.Claude, m); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := os.ReadFile(paths.Claude.Manifest(root))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("file = %q, Bytes = %q", got, want)
	}
}

func TestLoadRejectsInvalidJSON(t *testing.T) {
	root := t.TempDir()
	if err := workspace.AtomicWrite(paths.Claude.Manifest(root), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	if _, found, err := Load(root, paths.Claude); err == nil {
		t.Errorf("Load = (found %v, nil error), want an error naming the file", found)
	}
}
