package validate

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// Wiki validates docs/wiki/ and the drop box beside it.
//
// Every check is a graph fact over Markdown — no code is read, and none of them is a
// judgment call:
//
//   - a [[wikilink]] resolves to a page
//   - every page is reachable from index.md, because an orphan is a page nobody will
//     find again
//   - the changelog exists and names only pages that exist
//   - no two files claim one slug, since [[order-total]] must mean one thing
//   - the pages are under wiki/pages/
//   - docs/raw/ is empty, because a file still sitting there was collected and never
//     processed
//
// What this cannot check is whether a page's name names anything — `into-an-engine.md`
// resolves, links, and is reachable. That one is on the wiki skill.
func Wiki(root string) (*finding.Set, error) {
	set := &finding.Set{}
	if err := checkRaw(set, root); err != nil {
		return nil, err
	}

	dir := paths.Wiki(root)
	pages, err := wikiPages(dir)
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return set, nil
	}

	// A page's name is its slug: [[order-total]] resolves to order-total.md. Pages
	// arrive canonical-first, so a slug claimed in both layouts reports the loose copy
	// as the duplicate rather than the one already in the right place.
	slugs := map[string]bool{}
	claimed := map[string]string{}
	for _, pg := range pages {
		if first, dup := claimed[pg.slug]; dup {
			set.Addf(rel(root, pg.path), 0, "wiki.duplicate-page",
				"[[%s]] already resolves to %s; one slug cannot name two files",
				pg.slug, rel(root, first))
			continue
		}
		claimed[pg.slug] = pg.path
		slugs[pg.slug] = true
		if pg.legacy {
			set.Addf(rel(root, pg.path), 0, "wiki.legacy-page",
				"pages live in %s/%s/ now; move it there so the wiki's fixed documents stay distinguishable from its content",
				paths.WikiSeg, paths.WikiPagesSeg)
		}
	}

	indexPath := filepath.Join(dir, paths.WikiIndex)
	if !isFile(indexPath) {
		set.Addf(rel(root, indexPath), 0, "wiki.missing-index",
			"the wiki has %d pages and no %s; without an entry point every page is an orphan",
			len(pages), paths.WikiIndex)
	}

	// links[from] = the pages it points at, collected once and used for both the
	// broken-link check and the reachability walk. The index is a node in that graph
	// without being a page, so it is keyed by something no filename can produce.
	const indexNode = "\x00index"
	links := map[string][]string{}
	collect := func(node, p string) error {
		doc, err := read(root, p)
		if err != nil {
			if doc == nil {
				return err
			}
			set.Addf(rel(root, p), 1, "wiki.frontmatter-unreadable", "%v", err)
			return nil
		}
		for _, link := range doc.Wikilinks {
			target := strings.TrimSuffix(link.Target, ".md")
			if !slugs[target] {
				set.Addf(rel(root, p), link.Line, "wiki.broken-link",
					"[[%s]] resolves to no page under %s/%s/", link.Target, paths.WikiSeg, paths.WikiPagesSeg)
				continue
			}
			links[node] = append(links[node], target)
		}
		return nil
	}
	if isFile(indexPath) {
		if err := collect(indexNode, indexPath); err != nil {
			return nil, err
		}
	}
	for _, pg := range pages {
		if err := collect(pg.slug, pg.path); err != nil {
			return nil, err
		}
	}

	// Reachability from the index. The changelog is a log rather than a page, so it is
	// never walked and is never an orphan.
	reachable := map[string]bool{}
	walk := []string{indexNode}
	for len(walk) > 0 {
		cur := walk[len(walk)-1]
		walk = walk[:len(walk)-1]
		for _, next := range links[cur] {
			if !reachable[next] {
				reachable[next] = true
				walk = append(walk, next)
			}
		}
	}
	orphans := make([]string, 0)
	for _, pg := range pages {
		if !reachable[pg.slug] {
			orphans = append(orphans, pg.path)
		}
	}
	sort.Strings(orphans)
	for _, p := range orphans {
		set.Addf(rel(root, p), 0, "wiki.orphan-page",
			"not reachable from %s; link it from the index or from a page that is", paths.WikiIndex)
	}

	checkChangelog(set, root, dir, slugs)
	return set, nil
}

// page is one wiki page: where it sits, what [[slug]] resolves to it, and whether it
// is still in the layout that predates wiki/pages/.
type page struct {
	path   string
	slug   string
	legacy bool
}

// wikiPages lists the pages, the canonical ones first.
//
// A .md file directly in wiki/ that is not one of the two fixed documents is still
// read as a page, and only then reported. Refusing to see it would turn every existing
// wiki into a wall of wiki.broken-link the moment scc was upgraded — pages that are
// really there, reported as missing. A finding that names the layout is an answer the
// author can act on; a finding that says a page does not exist when it does is the
// kind that teaches people to stop believing the validator.
func wikiPages(dir string) ([]page, error) {
	canonical, err := markdownFiles(filepath.Join(dir, paths.WikiPagesSeg))
	if err != nil {
		return nil, err
	}
	out := make([]page, 0, len(canonical))
	for _, p := range canonical {
		out = append(out, page{path: p, slug: pageSlug(p)})
	}
	loose, err := markdownFiles(dir)
	if err != nil {
		return nil, err
	}
	for _, p := range loose {
		if base := filepath.Base(p); base == paths.WikiIndex || base == paths.WikiLog {
			continue
		}
		out = append(out, page{path: p, slug: pageSlug(p), legacy: true})
	}
	return out, nil
}

func pageSlug(p string) string { return strings.TrimSuffix(filepath.Base(p), ".md") }

// checkChangelog is the index/log desync check, in the form that is actually
// structural: the log exists, and every page it names still exists. Comparing the log
// against what changed would mean reading history, which scc does not do.
func checkChangelog(set *finding.Set, root, dir string, slugs map[string]bool) {
	path := filepath.Join(dir, paths.WikiLog)
	if !isFile(path) {
		set.Addf(rel(root, path), 0, "wiki.missing-changelog",
			"the wiki has pages and no %s; record what changed when you change it", paths.WikiLog)
		return
	}
	doc, err := read(root, path)
	if err != nil || doc == nil {
		return
	}
	for _, link := range doc.Wikilinks {
		target := strings.TrimSuffix(link.Target, ".md")
		if !slugs[target] {
			set.Addf(rel(root, path), link.Line, "wiki.changelog-desync",
				"the log names [[%s]], which no longer exists", link.Target)
		}
	}
}

// checkRaw reports sources that were collected and never processed. raw/ is a drop box,
// not storage: material goes there to be read, distilled into a page, and removed.
func checkRaw(set *finding.Set, root string) error {
	dir := paths.Raw(root)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		// A dotfile is how a directory gets committed while empty, which is the
		// opposite of unprocessed work.
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		set.Addf(rel(root, filepath.Join(dir, name)), 0, "wiki.unprocessed-source",
			"still in %s/: distill it into a wiki page and remove it", paths.RawSeg)
	}
	return nil
}

// markdownFiles lists the .md files directly in dir, sorted. Absent is not an error:
// every knowledge-base validator is silent when its subject does not exist.
func markdownFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}
