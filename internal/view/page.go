package view

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/artifact"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// Page is one Markdown file, answered by GET /api/page. Body is the Markdown
// after the frontmatter block, which is shown separately as Frontmatter.
type Page struct {
	Path        string             `json:"path"`
	Kind        string             `json:"kind"`
	Title       string             `json:"title"`
	Frontmatter map[string]string  `json:"frontmatter"`
	Sections    []artifact.Section `json:"sections"`
	Tasks       []artifact.Task    `json:"tasks"`
	Done        int                `json:"done"`
	Total       int                `json:"total"`
	Body        string             `json:"body"`
}

// Source is one file a codewiki page cites, answered by GET /api/source.
type Source struct {
	Path  string   `json:"path"`
	Lines []string `json:"lines"`
}

// maxFile caps what either route reads, and what the tree parses; a page or a
// cited file larger than this is not narrated code, and a hostile repository's
// multi-gigabyte .md should not be read into memory to find that out.
const maxFile = 1 << 20

// LoadPage resolves rel to a Markdown file under docs/, plans/ or specs/ and
// parses it. false covers every refusal alike, so a caller cannot tell a file that
// is not there from one it may not read.
func LoadPage(root, rel string) (Page, bool) {
	abs, ok := resolve(root, rel)
	if !ok || !strings.HasSuffix(rel, ".md") {
		return Page{}, false
	}
	switch strings.SplitN(rel, "/", 2)[0] {
	case paths.DocsSeg, paths.PlansSeg, paths.SpecsSeg:
	default:
		return Page{}, false
	}
	a, err := artifact.Load(root, abs)
	if err != nil {
		return Page{}, false
	}
	done, total := a.Done()
	body := a.Lines
	if n := a.Doc().Frontmatter.Lines; n > 0 && n <= len(body) {
		body = body[n:]
	}
	fm := a.Frontmatter
	if fm == nil {
		fm = map[string]string{}
	}
	return Page{
		Path: a.Path, Kind: string(a.Kind), Title: a.Title, Frontmatter: fm,
		Sections: orEmpty(a.Sections), Tasks: orEmpty(a.Tasks),
		Done: done, Total: total, Body: strings.Join(body, "\n"),
	}, true
}

// LoadSource resolves rel to a text file anywhere in the workspace and returns its
// lines.
func LoadSource(root, rel string) (Source, bool) {
	abs, ok := resolve(root, rel)
	if !ok {
		return Source{}, false
	}
	data, err := os.ReadFile(abs)
	if err != nil || len(data) > maxFile {
		return Source{}, false
	}
	if bytes.IndexByte(data[:min(len(data), 8192)], 0) >= 0 {
		return Source{}, false
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	return Source{Path: rel, Lines: strings.Split(strings.TrimSuffix(text, "\n"), "\n")}, true
}

// resolve is the containment check both routes share: rel must already be a
// clean, relative, slash-separated path naming a regular file that stays inside
// the workspace once symlinks are followed. No segment may be dot-named — .git,
// .env, the harness directory, and `..` with them — which is also what the tree
// skips, so no route serves a file the sidebar would never list.
//
// The dot rule runs twice: on the request, and on the real path after symlinks
// are followed. The second is the one that holds — a committed symlink to
// ../.env, or a Windows short name like GIT~1, both pass the first check, and
// EvalSymlinks expands either back to the dot-named path it stands for.
func resolve(root, rel string) (string, bool) {
	if rel == "" || strings.ContainsAny(rel, "\\:\x00") || path.IsAbs(rel) || path.Clean(rel) != rel || dotted(rel) {
		return "", false
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if !artifact.Within(root, abs) {
		return "", false
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", false
	}
	realAbs, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", false
	}
	realRel, err := filepath.Rel(realRoot, realAbs)
	if err != nil || dotted(filepath.ToSlash(realRel)) {
		return "", false
	}
	st, err := os.Stat(realAbs)
	if err != nil || !st.Mode().IsRegular() || st.Size() > maxFile {
		return "", false
	}
	return abs, true
}

// dotted reports whether any segment of a slash-separated path is dot-named,
// `..` included.
func dotted(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
