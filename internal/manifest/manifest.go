// Package manifest reads and writes .claude/scc-manifest.json — scc's only file
// and, by existing at all, the workspace marker.
//
// It records one entry per scc-managed file: a content hash and the template
// version that produced it. The two answer different questions and an upgrade
// needs both. The hash answers "did the user edit this?", which is what keeps an
// upgrade from overwriting authored work. The version answers "what did this look
// like before?", which is the base revision of the three-way merge that brings a
// workspace onto new templates without handing the user a merge as homework.
//
// Everything here is deterministic on purpose. The same workspace on two machines
// must produce a byte-identical manifest — entries sorted by path, LF endings, a
// trailing newline, and slash-separated relative paths — or every upgrade opens
// with a spurious diff and the file stops being reviewable.
package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/textutil"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// Status is what a managed file on disk turned out to be.
type Status string

const (
	// Pristine means the file's content still hashes to what the recorded
	// template version rendered — scc may replace it outright.
	Pristine Status = "pristine"
	// Edited means the user changed it. An upgrade merges rather than writes.
	Edited Status = "edited"
	// Missing means the file is gone. An upgrade restores it; nothing is lost.
	Missing Status = "missing"
)

// Entry is one scc-managed file.
//
// Path is slash-separated and relative to the workspace root — never a
// filepath.Join result. A manifest written on Windows is read on Linux (a
// committed workspace is a shared workspace), and this is the one place in scc
// where the on-disk layout crosses machines, so the separator is fixed here
// rather than left to the host.
type Entry struct {
	Path    string // slash-separated, relative to the workspace root
	Hash    string // SHA-256 (lowercase hex) of the pristine render, LF-normalized
	Version string // the template-set version that rendered it

	// extra carries keys a newer scc wrote that this build does not understand,
	// so an older binary reading a newer manifest does not silently delete them.
	extra map[string]json.RawMessage
}

// Manifest is the file's whole content: a version stamp for diagnostics and the
// managed-file entries.
type Manifest struct {
	// SCC is the scc version that last wrote this file. It is a diagnostic — no
	// code branches on it — but it is the first thing worth knowing when a
	// workspace behaves like a different version than the user expects.
	SCC string

	// Files are the managed entries, kept sorted by Path by every operation that
	// mutates them.
	Files []Entry

	extra map[string]json.RawMessage
}

// Field names in the JSON document. Changing one is a format change: an older
// binary would read the new key as unknown and preserve it while treating the
// entry as absent, which is the failure this constant exists to make visible.
const (
	keySCC     = "scc"
	keyFiles   = "files"
	keyPath    = "path"
	keyHash    = "hash"
	keyVersion = "version"
)

// Hash is the content hash recorded for a managed file: SHA-256 over the
// LF-normalized text, lowercase hex.
//
// Normalizing first is what makes the hash portable. The same template checked
// out with CRLF on Windows must hash identically to the LF copy on Linux, or
// every managed file reads as edited on one of the two platforms.
func Hash(content string) string {
	sum := sha256.Sum256([]byte(textutil.NormalizeNewlines(content)))
	return hex.EncodeToString(sum[:])
}

// HashFile returns the hash of the file at path, or Missing-friendly errors:
// os.IsNotExist(err) is true when the file is absent.
func HashFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return Hash(string(b)), nil
}

// New returns an empty manifest stamped with the given scc version.
func New(sccVersion string) *Manifest {
	return &Manifest{SCC: sccVersion}
}

// Load reads the manifest at root. A missing file is not an error: it means the
// directory is not a workspace yet, and the caller gets an empty manifest plus
// found=false so it can tell "no workspace" from "empty workspace".
func Load(root string) (m *Manifest, found bool, err error) {
	b, err := os.ReadFile(paths.Manifest(root))
	if errors.Is(err, fs.ErrNotExist) {
		return &Manifest{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	m = &Manifest{}
	if err := json.Unmarshal([]byte(textutil.NormalizeNewlines(string(b))), m); err != nil {
		return nil, true, fmt.Errorf("%s is not valid scc manifest JSON: %w", paths.Manifest(root), err)
	}
	m.sortFiles()
	return m, true, nil
}

// Save writes the manifest under root atomically, so a concurrent reader — or a
// crash — never observes a half-written marker.
func Save(root string, m *Manifest) error {
	b, err := m.Bytes()
	if err != nil {
		return err
	}
	return workspace.AtomicWrite(paths.Manifest(root), b, 0o644)
}

// Bytes serializes the manifest exactly as Save would write it: sorted, indented
// with two spaces, LF, one trailing newline. Exposed separately so a caller can
// compare against what is already on disk and skip an identical write — which is
// what makes a second `scc init` a true no-op instead of a rewrite with the same
// content.
func (m *Manifest) Bytes() ([]byte, error) {
	m.sortFiles()
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	// MarshalIndent already emits LF; normalize anyway so the guarantee is stated
	// in one place rather than inherited from the encoder's current behavior.
	return []byte(textutil.NormalizeNewlines(string(b)) + "\n"), nil
}

// Get returns the entry for a slash-separated relative path.
func (m *Manifest) Get(rel string) (Entry, bool) {
	for _, e := range m.Files {
		if e.Path == rel {
			return e, true
		}
	}
	return Entry{}, false
}

// Set records or replaces the entry for rel, preserving any unknown fields a
// newer scc had written for that same entry.
func (m *Manifest) Set(rel, hash, version string) {
	rel = path.Clean(strings.ReplaceAll(rel, `\`, "/"))
	for i, e := range m.Files {
		if e.Path == rel {
			m.Files[i].Hash = hash
			m.Files[i].Version = version
			return
		}
	}
	m.Files = append(m.Files, Entry{Path: rel, Hash: hash, Version: version})
	m.sortFiles()
}

// Remove drops the entry for rel, reporting whether it was there.
func (m *Manifest) Remove(rel string) bool {
	for i, e := range m.Files {
		if e.Path == rel {
			m.Files = append(m.Files[:i], m.Files[i+1:]...)
			return true
		}
	}
	return false
}

// Status reports what the managed file recorded by e currently is on disk under
// root: pristine, edited by the user, or missing.
//
// A read error other than "not exist" is returned rather than guessed at. Calling
// an unreadable file either name would be wrong, and §5's rule applies to the
// upgrade path too: a wrong answer here overwrites authored work.
func (m *Manifest) Status(root string, e Entry) (Status, error) {
	got, err := HashFile(filepath.Join(root, filepath.FromSlash(e.Path)))
	if errors.Is(err, fs.ErrNotExist) {
		return Missing, nil
	}
	if err != nil {
		return "", err
	}
	if got == e.Hash {
		return Pristine, nil
	}
	return Edited, nil
}

func (m *Manifest) sortFiles() {
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
}

// MarshalJSON writes the manifest as an object built from a map, which
// encoding/json emits with sorted keys — so known and unknown fields alike land
// in one deterministic order without this code maintaining one.
func (m Manifest) MarshalJSON() ([]byte, error) {
	out := cloneRaw(m.extra)
	if err := putJSON(out, keySCC, m.SCC); err != nil {
		return nil, err
	}
	files := m.Files
	if files == nil {
		files = []Entry{} // an empty manifest serializes as [], never null
	}
	if err := putJSON(out, keyFiles, files); err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// UnmarshalJSON keeps every key it does not recognize. An older binary that
// dropped them would silently discard whatever a newer scc recorded — and the
// manifest is the only state scc has, so a dropped field is lost state.
func (m *Manifest) UnmarshalJSON(b []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if v, ok := raw[keySCC]; ok {
		if err := json.Unmarshal(v, &m.SCC); err != nil {
			return fmt.Errorf("field %q: %w", keySCC, err)
		}
		delete(raw, keySCC)
	}
	if v, ok := raw[keyFiles]; ok {
		if err := json.Unmarshal(v, &m.Files); err != nil {
			return fmt.Errorf("field %q: %w", keyFiles, err)
		}
		delete(raw, keyFiles)
	}
	m.extra = orNil(raw)
	return nil
}

// MarshalJSON mirrors Manifest.MarshalJSON: map-built, so key order is sorted and
// therefore identical on every machine.
func (e Entry) MarshalJSON() ([]byte, error) {
	out := cloneRaw(e.extra)
	for k, v := range map[string]string{keyPath: e.Path, keyHash: e.Hash, keyVersion: e.Version} {
		if err := putJSON(out, k, v); err != nil {
			return nil, err
		}
	}
	return json.Marshal(out)
}

// UnmarshalJSON preserves unknown per-entry fields, for the same reason
// Manifest.UnmarshalJSON does.
func (e *Entry) UnmarshalJSON(b []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	for key, dst := range map[string]*string{keyPath: &e.Path, keyHash: &e.Hash, keyVersion: &e.Version} {
		if v, ok := raw[key]; ok {
			if err := json.Unmarshal(v, dst); err != nil {
				return fmt.Errorf("field %q: %w", key, err)
			}
			delete(raw, key)
		}
	}
	e.extra = orNil(raw)
	return nil
}

func cloneRaw(in map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(in)+3)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func putJSON(dst map[string]json.RawMessage, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	dst[key] = b
	return nil
}

func orNil(raw map[string]json.RawMessage) map[string]json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return raw
}
