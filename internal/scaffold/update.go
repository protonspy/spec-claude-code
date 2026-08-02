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

// UpdateAction is what an update would do to one managed file.
//
// The set is deliberately larger than "write / don't write": the difference
// between a file scc may replace and one the user has authored into is the whole
// product decision here, and a plan that collapsed them would be a plan the user
// cannot review before agreeing to it.
type UpdateAction string

const (
	// UpCurrent means the file on disk already is what this version renders.
	UpCurrent UpdateAction = "current"
	// UpCreate means the template set ships it and the workspace does not have
	// it — either it is new in this version, or somebody deleted it.
	UpCreate UpdateAction = "create"
	// UpUpdate means the file is still exactly what scc last wrote, and scc now
	// writes something different. Replacing it loses nothing.
	UpUpdate UpdateAction = "update"
	// UpConflict means the file differs from what scc recorded — the user edited
	// it, or it arrived from somewhere scc has no record of — and the template
	// also moved. Only --force overwrites it.
	UpConflict UpdateAction = "conflict"
	// UpOwned means the file is one the user owns from their first edit (the
	// entry file, the project rule). A newer version exists and scc says so, but
	// it never merges into somebody's own prose.
	UpOwned UpdateAction = "owned"
	// UpDelete means the manifest carries it, this version no longer ships it,
	// and it is still pristine — so removing it destroys nothing.
	UpDelete UpdateAction = "delete"
	// UpOrphan means the same, except the file was edited. The entry is dropped
	// and the file stays, now the user's.
	UpOrphan UpdateAction = "orphan"
)

// UpdateItem is one file's place in the plan.
type UpdateItem struct {
	Path   string       `json:"path"`
	Action UpdateAction `json:"action"`
	// From is the template version the manifest recorded, when it had one.
	From string `json:"from,omitempty"`
	// To is the template version that would apply.
	To string `json:"to,omitempty"`
}

// UpdatePlan is what an update would do to one harness's tree, computed without
// writing anything. Every command that changes a workspace reports before it
// acts; this is the type that makes "report, then ask, then act" possible rather
// than aspirational.
type UpdatePlan struct {
	Root    string       `json:"root"`
	Harness string       `json:"harness"`
	Items   []UpdateItem `json:"items"`
}

// PlanUpdate compares the workspace against what this build renders for h.
//
// It reads and hashes; it writes nothing, so it is safe to run for a report and
// then throw away.
func PlanUpdate(root string, h paths.Harness) (*UpdatePlan, error) {
	prior, found, err := manifest.Load(root, h)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s is not an scc workspace for %s: run `init --%s` first", root, h.Label, h.ID)
	}
	plan := &UpdatePlan{Root: root, Harness: h.ID}

	managed := map[string]bool{}
	for _, f := range assets.Workspace(h) {
		managed[f.Rel] = true
		want, err := assets.Render(h, f)
		if err != nil {
			return nil, err
		}
		item, err := planOne(root, f, want, prior)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Rel, err)
		}
		plan.Items = append(plan.Items, item)
	}

	// Entries this version no longer ships. The manifest is exactly the set of
	// files scc manages, so a stale entry would send the next update looking for a
	// template that does not exist.
	for _, e := range prior.Files {
		if managed[e.Path] {
			continue
		}
		action := UpDelete
		got, err := manifest.HashFile(filepath.Join(root, filepath.FromSlash(e.Path)))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			// Already gone: nothing to remove but the entry.
		case err != nil:
			return nil, fmt.Errorf("%s: %w", e.Path, err)
		case got != e.Hash:
			action = UpOrphan
		}
		plan.Items = append(plan.Items, UpdateItem{Path: e.Path, Action: action, From: e.Version})
	}
	return plan, nil
}

func planOne(root string, f assets.File, want string, prior *manifest.Manifest) (UpdateItem, error) {
	item := UpdateItem{Path: f.Rel, To: assets.Version}
	if e, ok := prior.Get(f.Rel); ok {
		item.From = e.Version
	}

	got, err := manifest.HashFile(filepath.Join(root, filepath.FromSlash(f.Rel)))
	if errors.Is(err, fs.ErrNotExist) {
		item.Action = UpCreate
		return item, nil
	}
	if err != nil {
		return item, err
	}
	if got == manifest.Hash(want) {
		item.Action = UpCurrent
		return item, nil
	}
	// From here the file differs from what this version renders. Who last wrote it
	// decides what may be done about it.
	if f.Owned {
		item.Action = UpOwned
		return item, nil
	}
	if e, ok := prior.Get(f.Rel); ok && got == e.Hash {
		item.Action = UpUpdate
		return item, nil
	}
	item.Action = UpConflict
	return item, nil
}

// Count returns how many items carry an action.
func (p *UpdatePlan) Count(a UpdateAction) int {
	n := 0
	for _, it := range p.Items {
		if it.Action == a {
			n++
		}
	}
	return n
}

// Pending lists the items that are not already current, in plan order — what a
// user is actually being asked to agree to.
func (p *UpdatePlan) Pending() []UpdateItem {
	var out []UpdateItem
	for _, it := range p.Items {
		if it.Action != UpCurrent {
			out = append(out, it)
		}
	}
	return out
}

// Writes reports whether applying the plan would change anything — on disk or in
// the manifest — given whether the caller is forcing edited files. A plan whose
// only items are conflicts and owned files is a plan with nothing to do until the
// user decides something.
//
// An orphan counts: nothing is written to the file, but it stops being managed,
// and a stale entry left in the manifest would send the next update looking for a
// template that no longer exists.
func (p *UpdatePlan) Writes(force bool) bool {
	for _, it := range p.Items {
		switch it.Action {
		case UpCreate, UpUpdate, UpDelete, UpOrphan:
			return true
		case UpConflict:
			if force {
				return true
			}
		}
	}
	return false
}

// UpdateOptions are the knobs of applying a plan.
type UpdateOptions struct {
	// SCCVersion is stamped into the manifest as a diagnostic.
	SCCVersion string
	// Force also overwrites the files reported as conflicts — the ones scc can
	// see somebody edited. It is a separate decision from agreeing to the plan,
	// and the CLI asks for it separately, because it is the only part of an
	// update that destroys work.
	Force bool
}

// UpdateResult is what applying the plan actually did.
type UpdateResult struct {
	Root            string       `json:"root"`
	Harness         string       `json:"harness"`
	Applied         []UpdateItem `json:"applied"`
	Kept            []UpdateItem `json:"kept"`
	ManifestWritten bool         `json:"manifestWritten"`
}

// ApplyUpdate carries out a plan and rewrites the manifest to match what is now
// on disk.
//
// The manifest is written last and only when it changed, for the same reasons
// Apply does it: a crash midway leaves a workspace whose marker still describes a
// state that really existed, and an update that changed nothing does not touch
// the file's mtime.
func ApplyUpdate(root string, h paths.Harness, plan *UpdatePlan, opts UpdateOptions) (*UpdateResult, error) {
	prior, found, err := manifest.Load(root, h)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s is not an scc workspace for %s", root, h.Label)
	}
	res := &UpdateResult{Root: root, Harness: h.ID}
	next := manifest.New(opts.SCCVersion, h)

	rendered := map[string]string{}
	for _, f := range assets.Workspace(h) {
		want, err := assets.Render(h, f)
		if err != nil {
			return nil, err
		}
		rendered[f.Rel] = want
	}

	for _, it := range plan.Items {
		dest := filepath.Join(root, filepath.FromSlash(it.Path))
		want, shipped := rendered[it.Path]
		switch it.Action {
		case UpCreate, UpUpdate:
			if err := workspace.AtomicWrite(dest, []byte(want), 0o644); err != nil {
				return nil, fmt.Errorf("%s: %w", it.Path, err)
			}
			res.Applied = append(res.Applied, it)
			next.Set(it.Path, manifest.Hash(want), assets.Version)
		case UpConflict:
			if !opts.Force {
				// Left exactly as the user has it, and still tracked at the version
				// scc last wrote — which is the base revision a later merge needs.
				res.Kept = append(res.Kept, it)
				carry(next, prior, it.Path)
				continue
			}
			if err := workspace.AtomicWrite(dest, []byte(want), 0o644); err != nil {
				return nil, fmt.Errorf("%s: %w", it.Path, err)
			}
			res.Applied = append(res.Applied, it)
			next.Set(it.Path, manifest.Hash(want), assets.Version)
		case UpOwned:
			res.Kept = append(res.Kept, it)
			carry(next, prior, it.Path)
		case UpDelete:
			if err := os.Remove(dest); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("%s: %w", it.Path, err)
			}
			res.Applied = append(res.Applied, it)
		case UpOrphan:
			// The file stays and stops being managed: it is the user's now.
			res.Applied = append(res.Applied, it)
		case UpCurrent:
			if shipped {
				next.Set(it.Path, manifest.Hash(want), assets.Version)
			}
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

// carry keeps a file's existing entry — hash and version both — so a file scc did
// not rewrite is still recorded as what it actually is. Claiming today's version
// for content scc never wrote would destroy the merge base an upgrade needs.
func carry(next, prior *manifest.Manifest, rel string) {
	if e, ok := prior.Get(rel); ok {
		next.Set(rel, e.Hash, e.Version)
	}
}
