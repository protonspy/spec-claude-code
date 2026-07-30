package validate

import (
	"regexp"
	"sort"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// glossaryEntryRe matches one entry:
//
//   - **order total** — the amount charged, in minor units. Avoid: grand total, sum
var (
	glossaryEntryRe = regexp.MustCompile(`^\s*[-*+]\s+\*\*([^*]+)\*\*\s*(.*)$`)
	glossaryAvoidRe = regexp.MustCompile(`(?i)\bavoid:\s*(.*)$`)
)

// Glossary validates docs/glossary.md and the vocabulary it governs.
//
// Domain vocabulary drifts by default — three names for one thing appear within a week
// of two people working in parallel — so the glossary picks one canonical term and lists
// the others as avoided. This validator holds the knowledge base to that.
//
// **Whole-word matching only.** Substring matching here is a false-positive generator:
// an avoided synonym like "sum" appears inside "summary", "assume", and "consumer", and
// a validator that reported those would be muted within a day and take the real
// findings with it.
func Glossary(root string) (*finding.Set, error) {
	set := &finding.Set{}
	path := paths.Glossary(root)
	if !isFile(path) {
		return set, nil
	}
	file := rel(root, path)
	doc, err := read(root, path)
	if err != nil {
		if doc == nil {
			return nil, err
		}
		set.Addf(file, 1, "glossary.frontmatter-unreadable", "%v", err)
		return set, nil
	}

	canonical := map[string]int{}  // lowercased term -> line
	avoided := map[string]string{} // lowercased synonym -> the canonical term to use

	for i, line := range doc.Body {
		m := glossaryEntryRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		term := strings.TrimSpace(m[1])
		key := strings.ToLower(term)
		if prior, dup := canonical[key]; dup {
			set.Addf(file, i+1, "glossary.duplicate-term",
				"%q is already defined on line %d; one canonical term means one entry", term, prior)
			continue
		}
		canonical[key] = i + 1

		if a := glossaryAvoidRe.FindStringSubmatch(m[2]); a != nil {
			for _, syn := range strings.Split(a[1], ",") {
				syn = strings.Trim(strings.TrimSpace(syn), ".")
				if syn == "" {
					continue
				}
				avoided[strings.ToLower(syn)] = term
			}
		}
	}

	// A term cannot be both the word to use and a word to avoid. Whichever the author
	// meant, the glossary currently says both.
	for syn, term := range avoided {
		if line, isCanonical := canonical[syn]; isCanonical {
			set.Addf(file, line, "glossary.synonym-is-canonical",
				"%q is defined as canonical and also listed as a synonym to avoid under %q", syn, term)
		}
	}

	if len(avoided) == 0 {
		return set, nil
	}
	if err := checkAvoidedTerms(set, root, avoided); err != nil {
		return nil, err
	}
	return set, nil
}

// checkAvoidedTerms scans the knowledge base for avoided synonyms.
//
// The knowledge base only: docs/wiki/, docs/adr/, and docs/codewiki/. Not source, which
// scc never reads, and not specs/ — a requirement quoting an external contract verbatim
// is not a vocabulary slip, and reporting it would be the validator second-guessing a
// quotation it cannot see the source of.
func checkAvoidedTerms(set *finding.Set, root string, avoided map[string]string) error {
	synonyms := make([]string, 0, len(avoided))
	for syn := range avoided {
		synonyms = append(synonyms, syn)
	}
	sort.Strings(synonyms)

	matchers := map[string]*regexp.Regexp{}
	for _, syn := range synonyms {
		// \b around a quoted phrase: whole words, and phrases matched as phrases.
		matchers[syn] = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(syn) + `\b`)
	}

	for _, dir := range []string{paths.Wiki(root), paths.ADR(root), paths.Codewiki(root)} {
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
				for _, syn := range synonyms {
					if matchers[syn].MatchString(line) {
						set.Addf(rel(root, path), i+1, "glossary.avoided-synonym",
							"%q is an avoided synonym; the glossary's canonical term is %q", syn, avoided[syn])
					}
				}
			}
		}
	}
	return nil
}
