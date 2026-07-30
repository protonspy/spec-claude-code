package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/textutil"
)

// Stack turns a rule into a gate.
//
// "Technology not listed in docs/stack.md is an open decision, never adopted silently"
// reads like an unenforceable principle — but a dependency manifest is structured data,
// not source, so diffing the manifest against stack.md catches the silent dependency
// without reading a line of code. That is the whole trick, and it works in any ecosystem
// that has a manifest.
//
// Two deliberate limits:
//
//   - **Direct dependencies only.** An indirect dependency is not a decision anybody
//     made; reporting the transitive closure would bury the one line that matters.
//   - **An unparsed manifest produces no findings.** Ecosystems whose manifests cannot
//     be read confidently without a dependency (TOML, for one) wait for a safe parse
//     rather than getting a naive one. Silence is the correct behavior when a check
//     cannot be sure.
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
		// The full path, or the last segment — someone writing "chi" for
		// github.com/go-chi/chi has documented the decision, and insisting on the
		// module path would be the validator enforcing a spelling nobody agreed to.
		if strings.Contains(listed, strings.ToLower(name)) || strings.Contains(listed, strings.ToLower(baseName(name))) {
			continue
		}
		set.Addf(file, 0, "stack.undocumented-dependency",
			"%s is declared in %s and absent from %s: an adopted dependency is a decision, and this one is unrecorded",
			name, deps[name], paths.StackSeg)
	}
	return set, nil
}

// dependencies collects direct dependencies, mapping each to the manifest that declared
// it. Ecosystems are added here one at a time, each with a parse that cannot be wrong.
func dependencies(root string) (map[string]string, error) {
	deps := map[string]string{}
	if err := goModDependencies(root, deps); err != nil {
		return nil, err
	}
	if err := packageJSONDependencies(root, deps); err != nil {
		return nil, err
	}
	return deps, nil
}

// goModDependencies reads the require blocks of go.mod by hand.
//
// By hand rather than with golang.org/x/mod: scc is stdlib-only, the grammar in play
// here is two forms of one directive, and a dependency added to read a dependency file
// would be its own punchline.
func goModDependencies(root string, deps map[string]string) error {
	path := filepath.Join(root, "go.mod")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	inBlock := false
	for _, raw := range strings.Split(textutil.NormalizeNewlines(string(b)), "\n") {
		line := strings.TrimSpace(raw)
		// An indirect dependency is not a decision anybody made.
		if line == "" || strings.HasPrefix(line, "//") || strings.Contains(line, "// indirect") {
			continue
		}
		switch {
		case line == "require (":
			inBlock = true
			continue
		case inBlock && line == ")":
			inBlock = false
			continue
		}
		fields := strings.Fields(line)
		switch {
		case inBlock && len(fields) >= 2:
			deps[fields[0]] = "go.mod"
		case !inBlock && len(fields) >= 3 && fields[0] == "require":
			deps[fields[1]] = "go.mod"
		}
	}
	return nil
}

// packageJSONDependencies reads dependencies and devDependencies with encoding/json.
// Both count: a test framework is as much an adopted decision as a web server, and it is
// as much of a supply-chain surface.
func packageJSONDependencies(root string, deps map[string]string) error {
	path := filepath.Join(root, "package.json")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var manifest struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		// A manifest scc cannot parse produces no findings. It is not scc's place to
		// report on the syntax of somebody else's build file.
		return nil
	}
	for _, group := range []map[string]string{manifest.Dependencies, manifest.DevDependencies} {
		for name := range group {
			deps[name] = "package.json"
		}
	}
	return nil
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
