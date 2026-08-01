// Package scaffold applies the embedded template set to a directory and records
// what it wrote in the manifest. It is the mechanism behind `scc init`; the
// content it writes lives in internal/assets.
//
// Two properties are the whole point of the package, and both are tested rather
// than asserted:
//
//   - **It never authors over what the user owns.** An existing file is left alone,
//     whether it is pristine or edited. --force overwrites, and names every edited
//     file it clobbered — silently replacing authored work is the one outcome that
//     would make the tool untrustworthy.
//   - **It is idempotent.** A second run writes nothing at all, including the
//     manifest, so re-running init is free and safe rather than a diff.
package scaffold

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/protonspy/spec-claude-code/internal/assets"
	"github.com/protonspy/spec-claude-code/internal/manifest"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// Action is what happened to one file.
type Action string

const (
	// Created means the file was missing and scc wrote it.
	Created Action = "created"
	// Skipped means the file was already there and scc left it alone.
	Skipped Action = "skipped"
	// Replaced means --force overwrote a file that was still pristine.
	Replaced Action = "replaced"
	// Clobbered means --force overwrote a file the user had edited. It is a
	// separate action from Replaced because it is the only one that destroys work,
	// and it has to be reportable on its own.
	Clobbered Action = "clobbered"
	// Unmanaged means the manifest carried an entry for a file this version no
	// longer ships. The entry is dropped and the file, if any, becomes the user's.
	Unmanaged Action = "unmanaged"
)

// Change is one file's outcome. Path is slash-separated and relative to the root,
// matching what the manifest records.
type Change struct {
	Path   string `json:"path"`
	Action Action `json:"action"`
}

// Result is what a run did, in a shape both the human report and --json read from.
type Result struct {
	Root            string   `json:"root"`
	Harness         string   `json:"harness"`
	Changes         []Change `json:"changes"`
	Created         int      `json:"created"`
	Skipped         int      `json:"skipped"`
	Replaced        int      `json:"replaced"`
	Clobbered       []string `json:"clobbered"`
	ManifestWritten bool     `json:"manifestWritten"`
	AlreadyPresent  bool     `json:"alreadyPresent"`
}

// Options are the knobs of one run.
type Options struct {
	// SCCVersion is stamped into the manifest as a diagnostic — the first thing
	// worth knowing when a workspace behaves like a different version than the user
	// expects. No logic branches on it; only assets.Version decides behavior.
	SCCVersion string
	// Harness is which agent harness to scaffold for. The zero value is not a
	// harness, so a caller that forgets it gets a refusal rather than a tree
	// written into the wrong directory.
	Harness paths.Harness
	// Force overwrites existing files instead of leaving them alone.
	Force bool
}

// Apply writes the template set into root. With opts.Force, existing files are
// overwritten; without it, they are left exactly as they are.
//
// The manifest is written last, and only when its serialization differs from what
// is already on disk. Last, so a crash midway leaves no workspace marker and the
// retry is a clean adoption rather than a half-initialized tree. Only-when-changed,
// so a second run is a true no-op — the marker's mtime is not noise a reviewer
// should have to explain.
func Apply(root string, opts Options) (*Result, error) {
	h := opts.Harness
	if h.ID == "" {
		return nil, fmt.Errorf("no harness given: expected one of %s", paths.HarnessIDs())
	}
	prior, found, err := manifest.Load(root, h)
	if err != nil {
		return nil, err
	}
	res := &Result{Root: root, Harness: h.ID, AlreadyPresent: found}

	for _, dir := range assets.Dirs(h) {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			return nil, err
		}
	}

	next := manifest.New(opts.SCCVersion, h)
	for _, f := range assets.Workspace(h) {
		pristine, err := assets.Render(h, f)
		if err != nil {
			return nil, err
		}
		action, err := write(root, f, pristine, prior, opts.Force)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Rel, err)
		}
		res.record(f.Rel, action)

		// A file scc did not write keeps whatever version the manifest already
		// recorded for it: that version is the base revision an upgrade merges
		// from, and overwriting it with today's would claim the file looked like
		// something it never looked like. A file with no prior entry is adopted at
		// the current version, which makes today's template its merge base — the
		// only honest answer when scc has no record of where it came from.
		if e, ok := prior.Get(f.Rel); ok && action == Skipped {
			next.Set(f.Rel, e.Hash, e.Version)
			continue
		}
		next.Set(f.Rel, manifest.Hash(pristine), assets.Version)
	}

	// Entries for templates this version no longer ships are dropped: the manifest
	// is exactly the set of files scc manages, and a stale entry would send a later
	// upgrade looking for a template that does not exist.
	managed := map[string]bool{}
	for _, f := range assets.Workspace(h) {
		managed[f.Rel] = true
	}
	for _, e := range prior.Files {
		if !managed[e.Path] {
			res.record(e.Path, Unmanaged)
		}
	}

	same, err := manifestUnchanged(root, h, next)
	if err != nil {
		return nil, err
	}
	if !same {
		if err := manifest.Save(root, h, next); err != nil {
			return nil, err
		}
		res.ManifestWritten = true
	}
	return res, nil
}

// write puts one file in place and reports what it did to whatever was there.
func write(root string, f assets.File, pristine string, prior *manifest.Manifest, force bool) (Action, error) {
	dest := filepath.Join(root, filepath.FromSlash(f.Rel))
	onDisk, err := os.ReadFile(dest)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Created, workspace.AtomicWrite(dest, []byte(pristine), 0o644)
	case err != nil:
		return "", err
	}

	if !force {
		return Skipped, nil
	}
	// Writing identical content is not an overwrite; skipping it keeps --force from
	// touching files it has nothing to change.
	if manifest.Hash(string(onDisk)) == manifest.Hash(pristine) {
		return Skipped, nil
	}
	// Edited, as far as scc can tell: either it differs from the version the
	// manifest recorded, or scc has no record of it at all. Both mean somebody
	// else's content is about to be destroyed, so both are reported as Clobbered.
	action := Clobbered
	if e, ok := prior.Get(f.Rel); ok && manifest.Hash(string(onDisk)) == e.Hash {
		action = Replaced
	}
	return action, workspace.AtomicWrite(dest, []byte(pristine), 0o644)
}

func manifestUnchanged(root string, h paths.Harness, m *manifest.Manifest) (bool, error) {
	next, err := m.Bytes()
	if err != nil {
		return false, err
	}
	current, err := os.ReadFile(h.Manifest(root))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return string(current) == string(next), nil
}

func (r *Result) record(rel string, a Action) {
	r.Changes = append(r.Changes, Change{Path: rel, Action: a})
	switch a {
	case Created:
		r.Created++
	case Skipped:
		r.Skipped++
	case Replaced:
		r.Replaced++
	case Clobbered:
		r.Replaced++
		r.Clobbered = append(r.Clobbered, rel)
	}
}
