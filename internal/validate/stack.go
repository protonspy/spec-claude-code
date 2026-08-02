package validate

import (
	"regexp"
	"sort"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// Stack turns a rule into a gate.
//
// "Technology not listed in docs/stack.md is an open decision, never adopted silently"
// reads like an unenforceable principle — but a project's dependency file is structured
// data, not source, so diffing it against stack.md catches the silent dependency without
// reading a line of code. That is the whole trick, and it works in any ecosystem that
// declares its dependencies as data. The readers live in stack_manifests.go.
//
// Three deliberate limits:
//
//   - **Direct dependencies only.** An indirect dependency is not a decision anybody
//     made; reporting the transitive closure would bury the one line that matters.
//   - **An unparsed manifest produces no findings.** A file scc cannot read
//     confidently, or a shape it does not recognize, yields no dependency rather than a
//     guessed one. Silence is the correct behavior when a check cannot be sure.
//   - **An ecosystem with no reader is not checked at all.** No declared dependencies
//     means no expectation of a stack.md and no finding of any kind, so a project scc
//     cannot read the manifest of passes rather than failing on a file it never
//     understood.
func Stack(root string) (*finding.Set, error) {
	set := &finding.Set{}
	deps, err := dependencies(root)
	if err != nil {
		return nil, err
	}
	if len(deps) == 0 {
		return set, nil
	}

	path := paths.Stack(root)
	file := rel(root, path)
	if !isFile(path) {
		set.Addf(file, 0, "stack.missing",
			"this project declares %d dependencies and has no %s; adopted technology gets one line saying why it earned its place",
			len(deps), paths.StackSeg)
		return set, nil
	}
	doc, err := read(root, path)
	if err != nil {
		if doc == nil {
			return nil, err
		}
		set.Addf(file, 1, "stack.frontmatter-unreadable", "%v", err)
		return set, nil
	}
	listed := strings.ToLower(strings.Join(doc.Body, "\n"))

	names := make([]string, 0, len(deps))
	for name := range deps {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if documented(listed, name) {
			continue
		}
		set.Addf(file, 0, "stack.undocumented-dependency",
			"%s is declared in %s and absent from %s: an adopted dependency is a decision, and this one is unrecorded",
			name, deps[name], paths.StackSeg)
	}
	return set, nil
}

// documented reports whether stack.md names this dependency in any spelling somebody
// would plausibly have written it down as.
//
// The full path or its last segment: writing "chi" for github.com/go-chi/chi documents
// the decision, and insisting on the module path would be the validator enforcing a
// spelling nobody agreed to. The same goes for the separator — PyPI treats
// "Flask-SQLAlchemy" and "flask_sqlalchemy" as one project, and Maven artifacts are
// written both ways — so each candidate is also tried with the separators swapped.
//
// Every extra spelling can only remove a finding, never add one. That asymmetry is the
// reason this is allowed to be generous: the cost of a miss here is a dependency
// nobody was reminded to document, and the cost of a false positive is the user
// learning to disbelieve the validator.
func documented(listed, name string) bool {
	for _, candidate := range []string{name, baseName(name)} {
		for _, spelling := range []string{
			candidate,
			strings.ReplaceAll(candidate, "_", "-"),
			strings.ReplaceAll(candidate, "-", "_"),
		} {
			if spelling != "" && strings.Contains(listed, strings.ToLower(spelling)) {
				return true
			}
		}
	}
	return false
}

// baseName is the name a human would write a dependency down as: the last path segment,
// with a Go major-version suffix dropped.
//
// The suffix matters. github.com/go-chi/chi/v5 ends in "v5", and a validator that looked
// for "v5" in stack.md would report a documented dependency as undocumented — a false
// positive on the single most common shape of Go module path.
func baseName(module string) string {
	module = strings.TrimSuffix(module, "/")
	if i := strings.LastIndex(module, "/"); i >= 0 {
		if majorVersionRe.MatchString(module[i+1:]) {
			module = module[:i]
			if i = strings.LastIndex(module, "/"); i < 0 {
				return module
			}
		}
		return module[i+1:]
	}
	return module
}

// majorVersionRe matches a Go module's major-version path suffix: v2, v5, v11.
//
// Every integer from 2 up, which is two alternatives rather than one character class:
// a bare 2-9, or a multi-digit number whose leading digit may be 1 (v10, v11). A single
// `v[2-9]\d*` looks equivalent and is not — it rejects the whole v10-v19 range.
var majorVersionRe = regexp.MustCompile(`^v([2-9]|[1-9]\d+)$`)
