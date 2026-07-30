// Package validate holds every scc validator: one file per validator, one shared
// finding type, one shared Markdown scanner.
//
// One package rather than eight because the helpers below — resolve a path, read
// and normalize a file, report a finding at a line — must be identical across all
// of them. That is what keeps the message wording, the finding shape, and the
// path spelling from drifting between validators the user experiences as one tool.
//
// Three rules bind every check in here, and they are not style preferences:
//
//  1. **A false positive costs more than a miss.** In studied static-analysis
//     deployments 35–91% of warnings are non-actionable, and false positives are
//     the single most common reason developers suppress warnings. One wrong finding
//     teaches the user to disbelieve all eight validators. When a check cannot be
//     certain, it stays silent.
//  2. **Zero configuration.** Configuration is the other documented adoption
//     barrier. A validator that must be tuned first will not be.
//  3. **Few findings, each fixable.** A finding the user cannot act on is noise
//     wearing a useful shape.
//
// There is a reason to expect this to work: structural checks have the lowest
// false-positive rate of any class. A SKILL.md whose name does not match its
// directory is not a judgment call — it is true or false. Nothing here reads source
// code, which is the boundary that keeps it that way.
package validate

import (
	"os"
	"path/filepath"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/mdscan"
)

// read returns a Markdown file parsed by mdscan, with the finding-facing path
// (slash-separated, relative to root) already attached.
func read(root, path string) (*mdscan.Document, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return mdscan.Parse(rel(root, path), string(b))
}

// rel is how every finding spells a path: relative to the workspace root and
// slash-separated, so a finding's identity does not depend on the platform that
// produced it.
func rel(root, path string) string {
	r, err := filepath.Rel(root, path)
	if err != nil {
		return finding.Rel(path)
	}
	return finding.Rel(r)
}

// exists reports whether path is present, of any kind.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// isDir reports whether path is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// isFile reports whether path is a regular file. A directory carrying a file's name
// is not that file — the same distinction the workspace marker rests on.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
