package cli

import (
	"os"
	"path/filepath"

	"github.com/protonspy/spec-claude-code/internal/mdblock"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// blockFile is what happened to one marker-delimited block in one entry file.
//
// One shape for both integrations that write into that file, because it is the same
// question twice: which file, what happened to the block, and how many bytes it now
// costs in every request of the session. The JSON is `scc rtk`'s frozen shape, which
// is why the fields are named for a block rather than for RTK.
type blockFile struct {
	Path   string `json:"path"`
	Action string `json:"action"` // added | present | replaced | missing
	// Block is the version the opening marker claims, for a file that had one.
	Block string `json:"block,omitempty"`
	// Was is the version of the block that got replaced, when it differed from the
	// one scc ships. This is the honest half of preferring scc's block by default:
	// between two v2 blocks the smaller one simply wins, but a v3 replaced by a v2
	// is a downgrade and has to be visible rather than inferred.
	Was string `json:"was,omitempty"`
	// Bytes and WasBytes size the block now in the file against the one it
	// replaced — the whole argument for replacing it, stated in the unit that
	// matters.
	Bytes    int `json:"bytes,omitempty"`
	WasBytes int `json:"was_bytes,omitempty"`
	// Foreign names another tool's block found in the same file — Headroom writes
	// RTK guidance behind its own markers. Reported and never touched: scc does not
	// own that block, and the file would carry the same instructions twice.
	Foreign string `json:"foreign,omitempty"`
}

// blockMissing is the action for an entry file that is not on disk. scc writes these
// blocks into a document it does not own, so it declines to bring that document into
// existence: an entry file holding nothing but a usage block would be a workspace
// missing its methodology while looking configured.
const blockMissing = "missing"

// spliceEntryBlock keeps one block current in one entry file, and reports what that
// took. check reports without writing.
//
// Everything outside the markers is untouched, and the file is written atomically:
// the entry file is loaded on every launch, and a half-written one is a workspace
// that has lost its methodology.
func spliceEntryBlock(root, entry string, m mdblock.Markers, block string, keep, check bool) (blockFile, error) {
	path := filepath.Join(root, entry)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return blockFile{Path: entry, Action: blockMissing}, nil
	}
	if err != nil {
		return blockFile{}, err
	}
	doc := string(raw)
	next, action, err := m.Splice(doc, block, keep)
	if err != nil {
		return blockFile{}, err
	}
	// The version reported is the one that ends up in the file: what was already
	// there when the block was left alone, what scc wrote when it did the writing.
	found := m.Version(doc)
	if action != mdblock.Present {
		found = m.Version(block)
	}
	file := blockFile{Path: entry, Action: string(action), Block: found, Bytes: len(block)}
	if action == mdblock.Present {
		file.Bytes = len(m.Block(doc))
	}
	if action == mdblock.Replaced {
		was := m.Block(doc)
		file.WasBytes = len(was)
		// Only when it differs: naming the version on every replacement would bury
		// the one case that actually needs reading.
		if v := m.Version(was); v != found {
			file.Was = v
		}
	}
	if action == mdblock.Present || check {
		return file, nil
	}
	if err := workspace.AtomicWrite(path, []byte(next), 0o644); err != nil {
		return blockFile{}, err
	}
	return file, nil
}

// entryFiles lists the entry file of every harness this workspace was scaffolded
// for, deduplicated: Codex and opencode both read AGENTS.md, and splicing the same
// block into one file twice would append a second copy on the second pass.
func entryFiles(root string) []string {
	var out []string
	seen := map[string]bool{}
	for _, h := range workspace.Harnesses(root) {
		if seen[h.EntryFile] {
			continue
		}
		seen[h.EntryFile] = true
		out = append(out, h.EntryFile)
	}
	return out
}
