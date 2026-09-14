// Package manifest reads and writes <harness>/scc-manifest.json — scc's only
// file and, by existing at all, the workspace marker.
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

	// Harness is the harness this tree was scaffolded for: "claude", "codex",
	// "opencode". Unlike SCC it is load-bearing, because a template renders
	// differently per harness — an upgrade that re-renders the recorded version
	// to reconstruct the merge base has to render it for the same harness or the
	// base is wrong and the merge silently clobbers.
	Harness string

	// Build, Test, Lint and Format are this project's own commands — the delivery
	// gate's four. Empty means nobody has decided; the literal "skipped" means
	// somebody decided this project has none, which is a real answer for a language
	// with no formatter or no linter and has to be told apart from the first.
	//
	// Test is the one that also prints {"total": N, "coverage": P}.
	//
	// It is here rather than in a config file of its own because the manifest is
	// scc's only file and a second one would be a schema to version, read by
	// nothing else. It is the first key in it that is *input* rather than record,
	// which is worth knowing: everything else here is what scc wrote, and this is
	// what the project told scc.
	Build  string
	Test   string
	Lint   string
	Format string

	// MinCoverage is the coverage floor in percent. Zero means the default,
	// because every manifest written before this key existed has no value and
	// "absent" has to keep meaning the same thing as "unset".
	MinCoverage float64

	// Files are the managed entries, kept sorted by Path by every operation that
	// mutates them.
	Files []Entry

	extra map[string]json.RawMessage
}

// Field names in the JSON document. Changing one is a format change: an older
// binary would read the new key as unknown and preserve it while treating the
// entry as absent, which is the failure this constant exists to make visible.
const (
	keySCC         = "scc"
	keyHarness     = "harness"
	keyBuild       = "build"
	keyTest        = "test"
	keyLint        = "lint"
	keyFormat      = "format"
	keyMinCoverage = "min_coverage"
	keyFiles       = "files"
	keyPath        = "path"
	keyHash        = "hash"
	keyVersion     = "version"
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

// New returns an empty manifest stamped with the given scc version and harness.
func New(sccVersion string, h paths.Harness) *Manifest {
	return &Manifest{SCC: sccVersion, Harness: h.ID}
}

// Load reads the manifest for h at root. A missing file is not an error: it means
// the directory is not a workspace for that harness yet, and the caller gets an
// empty manifest plus found=false so it can tell "no workspace" from "empty
// workspace".
func Load(root string, h paths.Harness) (m *Manifest, found bool, err error) {
	b, err := os.ReadFile(h.Manifest(root))
	if errors.Is(err, fs.ErrNotExist) {
		return &Manifest{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	m = &Manifest{}
	if err := json.Unmarshal([]byte(textutil.NormalizeNewlines(string(b))), m); err != nil {
		return nil, true, fmt.Errorf("%s is not valid scc manifest JSON: %w", h.Manifest(root), err)
	}
	// Every recorded path is joined onto the root by somebody — and `scc update`
	// joins it and then calls os.Remove. A manifest is a committed file that
	// arrives with a clone, so an entry reading "../../.ssh/authorized_keys" is
	// exactly the hostile input this project's rule about SafeName exists for.
	// Refuse the whole file rather than skipping the bad entry: a manifest scc did
	// not write is not a manifest, and silently dropping part of it would leave a
	// workspace whose state nobody can account for.
	for _, e := range m.Files {
		if !isLocalPath(e.Path) {
			return nil, true, fmt.Errorf("%s records the path %q, which escapes the workspace — refusing to use it",
				h.Manifest(root), e.Path)
		}
	}
	m.sortFiles()
	return m, true, nil
}

// isLocalPath reports whether a recorded path stays inside the workspace when
// joined onto the root: relative, no "..", no root, and — on Windows — not a
// reserved device name.
func isLocalPath(rel string) bool {
	if rel == "" || strings.ContainsRune(rel, '\\') {
		return false
	}
	return filepath.IsLocal(filepath.FromSlash(rel))
}

// Save writes h's manifest under root atomically, so a concurrent reader — or a
// crash — never observes a half-written marker.
func Save(root string, h paths.Harness, m *Manifest) error {
	b, err := m.Bytes()
	if err != nil {
		return err
	}
	return workspace.AtomicWrite(h.Manifest(root), b, 0o644)
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
	if err := putJSON(out, keyHarness, m.Harness); err != nil {
		return nil, err
	}
	// Omitted when unset, so a workspace that never wired its commands up keeps
	// the manifest it already had. A key that appeared with an empty value would
	// rewrite every manifest in existence on the next `scc init`, for nothing —
	// and it would erase the distinction the empty value exists to carry, since
	// "nobody decided" and "decided there is none" are different states.
	for key, v := range m.commands() {
		if *v == "" {
			continue
		}
		if err := putJSON(out, key, *v); err != nil {
			return nil, err
		}
	}
	if m.MinCoverage > 0 {
		if err := putJSON(out, keyMinCoverage, m.MinCoverage); err != nil {
			return nil, err
		}
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
	if v, ok := raw[keyHarness]; ok {
		if err := json.Unmarshal(v, &m.Harness); err != nil {
			return fmt.Errorf("field %q: %w", keyHarness, err)
		}
		delete(raw, keyHarness)
	}
	for key, dst := range m.commands() {
		v, ok := raw[key]
		if !ok {
			continue
		}
		if err := json.Unmarshal(v, dst); err != nil {
			return fmt.Errorf("field %q: %w", key, err)
		}
		delete(raw, key)
	}
	if v, ok := raw[keyMinCoverage]; ok {
		if err := json.Unmarshal(v, &m.MinCoverage); err != nil {
			return fmt.Errorf("field %q: %w", keyMinCoverage, err)
		}
		delete(raw, keyMinCoverage)
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

// CarryOver moves everything that belongs to the project rather than to a
// scaffold run from the manifest that was on disk into the one about to replace
// it: the test command, its floor, and any key a newer scc wrote that this build
// does not know about.
//
// It exists because `scc init` and `scc update` both build the next manifest from
// scratch and then fill in the file entries — which is right for the entries and
// silently destructive for everything else. Before there was anything else in the
// file that mattered, the only casualty was the unknown-field preservation
// Unmarshal goes to the trouble of doing; now a re-run of `init` would throw away
// the delivery gate's whole configuration. One method, called by both, rather
// than a field-by-field copy in each.
func (m *Manifest) CarryOver(prior *Manifest) {
	if prior == nil {
		return
	}
	mine, theirs := m.commands(), prior.commands()
	for key, dst := range mine {
		*dst = *theirs[key]
	}
	m.MinCoverage = prior.MinCoverage
	m.extra = cloneRaw(prior.extra)
	if len(m.extra) == 0 {
		m.extra = nil
	}
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

// commands maps each command key to the field that holds it.
//
// One table rather than four repetitions in each of marshal, unmarshal and
// CarryOver. Adding a fifth gate then touches this function and the key
// constants, and nothing else — which is what stops the next one from being
// preserved on read and dropped on a re-scaffold.
func (m *Manifest) commands() map[string]*string {
	return map[string]*string{
		keyBuild:  &m.Build,
		keyTest:   &m.Test,
		keyLint:   &m.Lint,
		keyFormat: &m.Format,
	}
}
