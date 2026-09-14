package codegraph

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The scope: which trees of this workspace get a graph.
//
// **CodeGraph indexes one root at a time, and there is no way to filter one
// graph down to a set of directories.** Measured against 1.5.0: `init` takes a
// single optional `[path]` and refuses two ("too many arguments"), there is no
// include or exclude flag on any command, and the only exclusion it honors is
// `.gitignore` — which is git's file about git, not an index scope. So a
// workspace that wants `backend/src` and `frontend/src` indexed and nothing else
// gets **one graph per directory**, not one graph filtered to two.
//
// That is a real cost and it is the reason this file exists rather than a flag
// somewhere: every `scc graph` command then fans out, every answer comes back in
// as many sections as there are roots, and `.codegraph/` lands inside each scoped
// directory rather than at the workspace root. Worth it for a monorepo whose
// vendored trees and build output would otherwise be indexed on every sync;
// not worth it for a repository that is one tree, which is why the empty scope —
// the default — stays exactly what it was: one graph, at the root.
//
// scc composes paths here and nothing else. What CodeGraph does inside one of
// them is still CodeGraph's business.

// Root is one tree that gets its own graph.
type Root struct {
	// Rel is how the root is named in a report and in the manifest:
	// slash-separated, relative to the workspace, or "." for the whole thing.
	Rel string `json:"path"`
	// Dir is the absolute directory handed to CodeGraph, as a working directory.
	// Every command discovers the project from its cwd, which is what lets the
	// argument vectors stay unaware that scoping exists at all.
	Dir string `json:"-"`
}

// Indexed reports whether this root has a graph.
func (r Root) Indexed() bool { return Indexed(r.Dir) }

// Scoped reports whether a resolved set is a real scope or the whole workspace.
func Scoped(roots []Root) bool { return len(roots) != 1 || roots[0].Rel != "." }

// Roots resolves a recorded scope into the trees to index.
//
// An empty scope is the whole workspace — one root, ".", which is what every
// workspace had before scoping existed and what most should keep.
//
// Unmatched patterns come back separately rather than as an error. A scope is a
// committed file and branches differ: a pattern naming a directory this checkout
// does not have is worth saying once, and is not a reason to refuse to index the
// directories that are there.
func Roots(workspace string, scope []string) (roots []Root, missing []string) {
	if len(scope) == 0 {
		return []Root{{Rel: ".", Dir: workspace}}, nil
	}
	seen := map[string]bool{}
	for _, raw := range scope {
		pattern := normalize(raw)
		if pattern == "" {
			continue
		}
		matches, err := expand(workspace, pattern)
		if err != nil || len(matches) == 0 {
			missing = append(missing, raw)
			continue
		}
		for _, rel := range matches {
			if seen[rel] {
				continue
			}
			seen[rel] = true
			roots = append(roots, Root{Rel: rel, Dir: filepath.Join(workspace, filepath.FromSlash(rel))})
		}
	}
	if len(roots) == 0 {
		// Every pattern missed. The whole workspace is the wrong answer here —
		// it is the opposite of what the scope asked for, and indexing everything
		// because a path was misspelled is the kind of helpfulness that costs an
		// hour on a monorepo. The caller reports `missing` and does nothing.
		return nil, missing
	}
	return roots, missing
}

// normalize turns what somebody wrote into what CodeGraph can be pointed at.
//
// A trailing `/*` or `/**` is stripped, because `backend/src/*` is how people
// write "everything under backend/src" and handing that to filepath.Glob would
// mean one graph per child of that directory — which is not what anybody means
// and would be an expensive way to find out.
// A backslash is a separator here on every platform, which is deliberately not
// what filepath.ToSlash does. ToSlash is the *host's* conversion — a no-op on
// Linux — so a scope recorded as `backend\src` on Windows would name a directory
// there and nothing anywhere else. The manifest is committed and crosses
// machines, and internal/manifest already fixes the separator for recorded paths
// for exactly this reason; a scope is a recorded path too.
func normalize(raw string) string {
	p := strings.TrimSpace(strings.ReplaceAll(raw, `\`, "/"))
	for {
		switch {
		case strings.HasSuffix(p, "/**"):
			p = strings.TrimSuffix(p, "/**")
		case strings.HasSuffix(p, "/*"):
			p = strings.TrimSuffix(p, "/*")
		case strings.HasSuffix(p, "/"):
			p = strings.TrimSuffix(p, "/")
		default:
			if p == "." || p == "" {
				return ""
			}
			return p
		}
	}
}

// expand resolves one pattern to the directories it names, relative to the
// workspace and slash-separated.
//
// A pattern with an interior glob — `packages/*/src`, the monorepo shape — is
// expanded, because every match is a directory and each one is a tree worth its
// own graph. Only directories survive: a glob that catches files would otherwise
// hand CodeGraph a path it cannot index and turn a typo into an error per file.
//
// Anything that escapes the workspace is dropped. A manifest arrives with a
// clone, and `..` in a recorded path is the same hostile input the rule about
// SafeName exists for — here it would mean indexing, and writing a .codegraph
// into, a directory outside the repository.
func expand(workspace, pattern string) ([]string, error) {
	if !localPath(pattern) {
		return nil, fmt.Errorf("%q escapes the workspace", pattern)
	}
	if !strings.ContainsAny(pattern, "*?[") {
		if !isDir(filepath.Join(workspace, filepath.FromSlash(pattern))) {
			return nil, nil
		}
		return []string{pattern}, nil
	}
	hits, err := filepath.Glob(filepath.Join(workspace, filepath.FromSlash(pattern)))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, hit := range hits {
		if !isDir(hit) {
			continue
		}
		rel, err := filepath.Rel(workspace, hit)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if !localPath(rel) {
			continue
		}
		out = append(out, rel)
	}
	// Glob's order is the filesystem's; sorting is what makes two machines report
	// the same scope in the same order.
	sort.Strings(out)
	return out, nil
}

// localPath reports whether a recorded path stays inside the workspace.
func localPath(rel string) bool {
	if rel == "" || strings.Contains(rel, `\`) {
		return false
	}
	return filepath.IsLocal(filepath.FromSlash(rel))
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
