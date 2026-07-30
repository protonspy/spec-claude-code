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
// All four checks are graph facts over Markdown — no code is read, and none of them is
// a judgment call:
//
//   - a [[wikilink]] resolves to a page
//   - every page is reachable from index.md, because an orphan is a page nobody will
//     find again
//   - the changelog exists and names only pages that exist
//   - docs/raw/ is empty, because a file still sitting there was collected and never
//     processed
func Wiki(root string) (*finding.Set, error) {
	set := &finding.Set{}
	if err := checkRaw(set, root); err != nil {
		return nil, err
	}

	dir := paths.Wiki(root)
	pages, err := markdownFiles(dir)
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return set, nil
	}

	// A page's name is its slug: [[order-total]] resolves to order-total.md.
	slugs := map[string]bool{}
	for _, p := range pages {
		slugs[strings.TrimSuffix(filepath.Base(p), ".md")] = true
	}

	indexSlug := strings.TrimSuffix(paths.WikiIndex, ".md")
	if !slugs[indexSlug] {
		set.Addf(rel(root, filepath.Join(dir, paths.WikiIndex)), 0, "wiki.missing-index",
			"the wiki has %d pages and no %s; without an entry point every page is an orphan",
			len(pages), paths.WikiIndex)
	}

	// links[from] = the pages it points at, collected once and used for both the
	// broken-link check and the reachability walk.
	links := map[string][]string{}
	for _, p := range pages {
		slug := strings.TrimSuffix(filepath.Base(p), ".md")
		doc, err := read(root, p)
		if err != nil {
			if doc == nil {
				return nil, err
			}
			set.Addf(rel(root, p), 1, "wiki.frontmatter-unreadable", "%v", err)
			continue
		}
		for _, link := range doc.Wikilinks {
			target := strings.TrimSuffix(link.Target, ".md")
			if !slugs[target] {
				set.Addf(rel(root, p), link.Line, "wiki.broken-link",
					"[[%s]] resolves to no page under %s/", link.Target, paths.WikiSeg)
				continue
			}
			links[slug] = append(links[slug], target)
		}
	}

	// Reachability from the index. The changelog is a log rather than a page, so it is
	// not expected to be linked and is not an orphan when it is not.
	logSlug := strings.TrimSuffix(paths.WikiLog, ".md")
	reachable := map[string]bool{indexSlug: true, logSlug: true}
	walk := []string{indexSlug}
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
	for slug := range slugs {
		if !reachable[slug] {
			orphans = append(orphans, slug)
		}
	}
	sort.Strings(orphans)
	for _, slug := range orphans {
		set.Addf(rel(root, filepath.Join(dir, slug+".md")), 0, "wiki.orphan-page",
			"not reachable from %s; link it from the index or from a page that is", paths.WikiIndex)
	}

	checkChangelog(set, root, dir, slugs)
	return set, nil
}

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
