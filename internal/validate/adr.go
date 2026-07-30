package validate

import (
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

var (
	// adrNameRe is the filename shape: a four-digit number, a hyphen, a kebab slug.
	adrNameRe = regexp.MustCompile(`^(\d{4})-([a-z0-9][a-z0-9-]*)\.md$`)
	// adrCitationRe matches `adr:0007-use-sqlite-for-the-cache` anywhere in prose.
	adrCitationRe = regexp.MustCompile(`\badr:(\d{4}-[a-z0-9][a-z0-9-]*)\b`)
)

// adrRecord is one record on disk.
type adrRecord struct {
	number int
	file   string // the finding-facing path
	path   string // the path on disk
}

// adrStatuses are the four a record can be in.
var adrStatuses = map[string]bool{"proposed": true, "accepted": true, "rejected": true, "superseded": true}

// ADR validates docs/adr/.
//
// The check that carries the weight is the superseding one. **A superseded record is
// marked, never edited** — the point of an ADR is that it records what was believed at
// the time, so rewriting one destroys the only thing it exists to preserve. Structurally
// that means: a record marked superseded names its successor, and the successor exists.
func ADR(root string) (*finding.Set, error) {
	set := &finding.Set{}
	dir := paths.ADR(root)
	files, err := markdownFiles(dir)
	if err != nil {
		return nil, err
	}
	// Citations are resolved even when there are no records at all: `adr:0003-x` in a
	// wiki page sends the reader nowhere whether or not docs/adr/ exists, and that is a
	// broken reference either way.
	if len(files) == 0 {
		return set, checkADRCitations(set, root, nil)
	}

	slugs := map[string]bool{}  // "0007-use-sqlite" -> exists
	numbers := map[int]string{} // 7 -> filename
	var records []adrRecord

	for _, path := range files {
		name := filepath.Base(path)
		m := adrNameRe.FindStringSubmatch(name)
		if m == nil {
			set.Addf(rel(root, path), 0, "adr.malformed-filename",
				"an ADR is named `NNNN-kebab-slug.md`, so records sort and cite predictably")
			continue
		}
		n, _ := strconv.Atoi(m[1])
		slug := strings.TrimSuffix(name, ".md")
		if prior, dup := numbers[n]; dup {
			set.Addf(rel(root, path), 0, "adr.duplicate-number",
				"%04d is already used by %s; a citation would be ambiguous", n, prior)
			continue
		}
		numbers[n] = name
		slugs[slug] = true
		records = append(records, adrRecord{number: n, file: rel(root, path), path: path})
	}

	sort.Slice(records, func(i, j int) bool { return records[i].number < records[j].number })
	checkADRContiguity(set, records)

	for _, r := range records {
		if err := checkADRRecord(set, root, r.path, r.file, slugs); err != nil {
			return nil, err
		}
	}
	if err := checkADRCitations(set, root, slugs); err != nil {
		return nil, err
	}
	return set, nil
}

// checkADRContiguity reports gaps. Unlike requirement numbering — where a delta
// legitimately leaves a hole — an ADR is never removed, so a missing number means a
// record was deleted or never committed, and either way the history has a hole in it.
func checkADRContiguity(set *finding.Set, records []adrRecord) {
	for i, r := range records {
		want := i + 1
		if r.number == want {
			continue
		}
		set.Addf(r.file, 0, "adr.numbering-gap",
			"numbered %04d where %04d was expected; ADR numbers run contiguously from 0001 because a gap means a record went missing",
			r.number, want)
		return // one finding, not one per record after the gap
	}
}

func checkADRRecord(set *finding.Set, root, path, file string, slugs map[string]bool) error {
	doc, err := read(root, path)
	if err != nil {
		if doc == nil {
			return err
		}
		set.Addf(file, 1, "adr.frontmatter-unreadable", "%v", err)
		return nil
	}
	status, hasStatus := doc.Frontmatter.Get("status")
	successor, hasSuccessor := doc.Frontmatter.Get("superseded-by")

	switch {
	case !hasStatus:
		set.Addf(file, 1, "adr.missing-status",
			"an ADR records its `status`: %s", allowed(adrStatuses))
	case !adrStatuses[status]:
		set.Addf(file, 1, "adr.status-invalid",
			"`status: %s` is not one of %s", status, allowed(adrStatuses))
	}

	// The two halves have to agree, in both directions: a record cannot be superseded
	// by nothing, and it cannot name a successor while claiming to be current.
	if status == "superseded" && (!hasSuccessor || successor == "") {
		set.Addf(file, 1, "adr.superseded-without-successor",
			"marked superseded and names no successor; add `superseded-by: NNNN-slug`")
	}
	if hasSuccessor && successor != "" {
		if status != "superseded" {
			set.Addf(file, 1, "adr.successor-without-status",
				"names a successor but its status is %q; set `status: superseded`", status)
		}
		if !slugs[successor] {
			set.Addf(file, 1, "adr.unknown-successor", "`superseded-by: %s` resolves to no record", successor)
		}
	}
	return nil
}

// checkADRCitations resolves `adr:<slug>` across the knowledge base. A citation to a
// record that does not exist is a reader sent to nothing.
func checkADRCitations(set *finding.Set, root string, slugs map[string]bool) error {
	dirs := []string{paths.ADR(root), paths.Wiki(root), paths.Codewiki(root)}
	for _, dir := range dirs {
		files, err := markdownFiles(dir)
		if err != nil {
			return err
		}
		for _, path := range files {
			doc, err := read(root, path)
			if err != nil || doc == nil {
				continue
			}
			for i, line := range doc.Body {
				for _, m := range adrCitationRe.FindAllStringSubmatch(line, -1) {
					if !slugs[m[1]] {
						set.Addf(rel(root, path), i+1, "adr.unknown-citation",
							"adr:%s resolves to no record under %s/", m[1], paths.ADRSeg)
					}
				}
			}
		}
	}
	return nil
}
