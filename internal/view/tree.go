package view

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/artifact"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// Item is one page in the sidebar: enough to label it and show its progress.
type Item struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

// Group is one section of the sidebar. The ids are the contract the frontend
// switches on; the titles are what a person reads.
type Group struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Items []Item `json:"items"`
}

// Tree is the whole sidebar, answered by GET /api/tree.
type Tree struct {
	Workspace string  `json:"workspace"`
	Groups    []Group `json:"groups"`
}

// groupOrder is the order the sidebar shows, and every id it can carry. All six
// are always present, empty or not, so the frontend never has to ask whether one
// exists.
var groupOrder = []struct{ id, title string }{
	{"wiki", "Wiki"},
	{"adr", "ADRs"},
	{"codewiki", "Codewiki"},
	{"docs", "Docs"},
	{"plans", "Plans"},
	{"specs", "Specs"},
}

// BuildTree reads the workspace from disk: every Markdown file under docs/, then
// every plan and spec through artifact.Scan, so the viewer and `scc map` agree on
// what is a plan and what is a spec.
func BuildTree(root string) (Tree, error) {
	items := map[string][]Item{}
	docs, err := docPages(root)
	if err != nil {
		return Tree{}, err
	}
	for _, rel := range docs {
		a, err := artifact.Load(root, filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return Tree{}, err
		}
		g := docGroup(rel)
		items[g] = append(items[g], itemOf(a))
	}
	arts, err := artifact.Scan(root)
	if err != nil {
		return Tree{}, err
	}
	for _, a := range arts {
		g := "specs"
		if a.Kind == artifact.KindPlan {
			g = "plans"
		}
		items[g] = append(items[g], itemOf(a))
	}

	t := Tree{Workspace: filepath.Base(root)}
	for _, g := range groupOrder {
		list := items[g.id]
		if list == nil {
			list = []Item{}
		}
		t.Groups = append(t.Groups, Group{ID: g.id, Title: g.title, Items: list})
	}
	return t, nil
}

func itemOf(a *artifact.Artifact) Item {
	done, total := a.Done()
	return Item{Path: a.Path, Title: a.Title, Kind: string(a.Kind), Done: done, Total: total}
}

// docPages lists docs/**/*.md, slash-separated and workspace-relative. A directory
// whose name starts with a dot is skipped, the same rule the source route applies.
// A file over maxFile is left out, as the page route would refuse it.
// A directory's own files come before its subdirectories', and its index.md first
// among them, because that is the entry point every wiki page is reachable from.
func docPages(root string) ([]string, error) {
	base := paths.Docs(root)
	var out []string
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == base {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() {
			if p != base && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() && strings.HasSuffix(d.Name(), ".md") {
			if info, err := d.Info(); err != nil || info.Size() > maxFile {
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		di, dj := pathDir(out[i]), pathDir(out[j])
		if di != dj {
			return di < dj
		}
		if ii, ij := isIndex(out[i]), isIndex(out[j]); ii != ij {
			return ii
		}
		return out[i] < out[j]
	})
	return out, nil
}

func pathDir(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i]
	}
	return ""
}

func isIndex(rel string) bool { return strings.HasSuffix(rel, "/index.md") }

// docGroup sorts a docs/ page by the directory directly under docs/.
func docGroup(rel string) string {
	parts := strings.Split(rel, "/")
	if len(parts) > 2 {
		switch parts[1] {
		case paths.WikiSeg:
			return "wiki"
		case paths.ADRSeg:
			return "adr"
		case paths.CodewikiSeg:
			return "codewiki"
		}
	}
	return "docs"
}
