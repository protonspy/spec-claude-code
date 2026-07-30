package validate

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/mdscan"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// The Agent Skills contract, from the published specification at
// agentskills.io/specification. These numbers are quoted from it, not chosen here:
// the value of this validator is that it enforces somebody else's standard — one
// that tools across competing vendors read — and a limit scc invented would forfeit
// exactly that.
const (
	skillNameMax          = 64   // "Must be 1-64 characters"
	skillDescriptionMax   = 1024 // "Must be 1-1024 characters"
	skillCompatibilityMax = 500  // "Must be 1-500 characters if provided"
	skillBodyMaxLines     = 500  // "Keep your main SKILL.md under 500 lines"
)

// The spec counts characters, so scc counts runes. len() on a string is bytes, which
// agrees with the spec only for ASCII and reports a shorter budget than the standard
// grants for anything else — an accented description would be failed at 1024 bytes
// while still inside its 1024 characters.
func charLen(s string) int { return utf8.RuneCountInString(s) }

// skillNameRe is the name charset: lowercase alphanumerics and hyphens, no leading
// or trailing hyphen, no consecutive hyphens.
var skillNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// skillFile is the file the spec requires in every skill directory.
const skillFile = "SKILL.md"

// Skills validates every skill under .claude/skills/.
//
// Implemented in Go rather than by delegating to the reference `skills-ref`
// validator: scc is a single binary on six platforms, and shelling out to Node for a
// handful of regexes would break that.
//
// A workspace with no skills directory is not a workspace with findings — it is a
// workspace with no skills.
func Skills(root string) (*finding.Set, error) {
	set := &finding.Set{}
	dir := paths.Skills(root)
	if !isDir(dir) {
		return set, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if err := skill(set, root, filepath.Join(dir, name), name); err != nil {
			return nil, err
		}
	}
	return set, nil
}

func skill(set *finding.Set, root, dir, dirName string) error {
	manifestPath := filepath.Join(dir, skillFile)
	if !isFile(manifestPath) {
		set.Addf(rel(root, dir), 0, "skill.missing-skill-md",
			"a skill directory must contain %s", skillFile)
		return nil
	}
	file := rel(root, manifestPath)

	doc, err := read(root, manifestPath)
	if err != nil {
		if doc == nil {
			return err
		}
		// The frontmatter could not be read. Report that and stop: every other check
		// depends on fields this one could not extract, and guessing at them is how a
		// validator produces findings about a file that was fine.
		set.Addf(file, 1, "skill.frontmatter-unreadable", "%v", err)
		return nil
	}
	fm := doc.Frontmatter
	if !fm.Present {
		set.Addf(file, 1, "skill.missing-frontmatter",
			"%s must open with YAML frontmatter carrying `name` and `description`", skillFile)
		return nil
	}

	checkSkillName(set, file, fm, dirName)
	checkSkillDescription(set, file, fm)

	if v, ok := fm.Get("compatibility"); ok && charLen(v) > skillCompatibilityMax {
		set.Addf(file, 1, "skill.compatibility-too-long",
			"`compatibility` is %d characters; the spec allows %d", charLen(v), skillCompatibilityMax)
	}

	// The body budget is a SHOULD in the spec, and it is reported as one: the agent
	// loads the whole file once the skill activates, so length is a real cost, but
	// exceeding it is a recommendation missed rather than a broken contract.
	if body := len(doc.Lines) - fm.Lines; body > skillBodyMaxLines {
		set.Addf(file, fm.Lines+skillBodyMaxLines+1, "skill.body-too-long",
			"the body is %d lines; the spec recommends keeping %s under %d and moving detail into references/",
			body, skillFile, skillBodyMaxLines)
	}

	checkSkillReferences(set, dir, file, doc)
	return nil
}

func checkSkillName(set *finding.Set, file string, fm mdscan.Frontmatter, dirName string) {
	name, ok := fm.Get("name")
	if !ok || name == "" {
		set.Addf(file, 1, "skill.missing-name", "`name` is required and must be 1-%d characters", skillNameMax)
		return
	}
	if charLen(name) > skillNameMax {
		set.Addf(file, 1, "skill.name-too-long",
			"`name` is %d characters; the spec allows 1-%d", charLen(name), skillNameMax)
	}
	if !skillNameRe.MatchString(name) {
		set.Addf(file, 1, "skill.name-invalid",
			"`name` %q must be lowercase alphanumerics and hyphens, with no leading, trailing, or consecutive hyphen", name)
	}
	// The one that actually breaks loading: a mismatched name means the skill does
	// not load at all, in any tool that reads this format.
	if name != dirName {
		set.Addf(file, 1, "skill.name-mismatch",
			"`name` is %q but the directory is %q; they must match or the skill will not load", name, dirName)
	}
}

func checkSkillDescription(set *finding.Set, file string, fm mdscan.Frontmatter) {
	desc, ok := fm.Get("description")
	if !ok || strings.TrimSpace(desc) == "" {
		set.Addf(file, 1, "skill.missing-description",
			"`description` is required: say what the skill does and when to use it")
		return
	}
	if charLen(desc) > skillDescriptionMax {
		set.Addf(file, 1, "skill.description-too-long",
			"`description` is %d characters; the spec allows 1-%d", charLen(desc), skillDescriptionMax)
	}
}

// checkSkillReferences holds the two rules about bundled files: a reference resolves,
// and it is one level deep.
//
// Only relative links are considered. A URL, an anchor, and an absolute path are all
// things this validator cannot resolve, and a check that cannot resolve its subject
// says nothing.
func checkSkillReferences(set *finding.Set, dir, file string, doc *mdscan.Document) {
	for _, link := range doc.Links {
		target := link.Target
		if !isLocalReference(target) {
			continue
		}
		target = strings.SplitN(target, "#", 2)[0]
		if target == "" {
			continue
		}
		if strings.Count(strings.Trim(path.Clean(target), "/"), "/") > 1 {
			set.Addf(file, link.Line, "skill.reference-too-deep",
				"%q is more than one level from %s; the spec asks for shallow references", target, skillFile)
		}
		resolved := filepath.Join(dir, filepath.FromSlash(target))
		if !exists(resolved) {
			set.Addf(file, link.Line, "skill.broken-reference", "%q does not exist", target)
		}
	}
}

// isLocalReference reports whether a link target is a path inside the skill —
// the only kind this validator can check.
func isLocalReference(target string) bool {
	switch {
	case target == "":
		return false
	case strings.HasPrefix(target, "#"): // an anchor within this file
		return false
	case strings.HasPrefix(target, "/"): // absolute: outside the skill
		return false
	case strings.HasPrefix(target, ".."): // escapes the skill; not scc's to judge
		return false
	case strings.Contains(target, "://"), strings.HasPrefix(target, "mailto:"):
		return false
	}
	return true
}
