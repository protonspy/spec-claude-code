package validate

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/textutil"
)

// This file is the answer to one question: what does scc consider a *declared
// dependency*? It sits beside stack.go rather than inside it because the rule and the
// readers age differently — the rule is fixed, and an ecosystem is added here roughly
// whenever somebody uses one scc cannot see.
//
// Every reader in here obeys the same three rules, and they are what keep the stack
// check from becoming a source of wrong findings:
//
//  1. **An absent manifest is not an error.** Most projects have exactly one of these
//     files, so every reader returns nil when its file is not there.
//  2. **Direct dependencies only.** A transitive dependency is not a decision anybody
//     made, and reporting the closure would bury the one line that matters.
//  3. **What cannot be read confidently is not read at all.** A malformed file, an
//     unrecognized shape, an unresolved variable — each produces no dependency rather
//     than a guessed one. Silence is the correct output of a check that cannot be
//     sure, and it is also why a project in an ecosystem with no reader here is
//     simply not checked instead of being reported as undocumented.
//
// Rule 3 is why Gemfile, mix.exs, Package.swift and build.gradle are absent: those
// manifests are executable code, and reading them honestly would mean evaluating
// them.

// dependencies collects direct dependencies, mapping each to the manifest that
// declared it. A project may legitimately have several — a Python service with a
// package.json for its front end is one repo with two decisions to record.
func dependencies(root string) (map[string]string, error) {
	deps := map[string]string{}
	for _, read := range []func(string, map[string]string) error{
		goModDependencies,
		packageJSONDependencies,
		requirementsDependencies,
		pyprojectDependencies,
		cargoDependencies,
		composerDependencies,
		pomDependencies,
	} {
		if err := read(root, deps); err != nil {
			return nil, err
		}
	}
	return deps, nil
}

// manifest reads one dependency file, reporting whether it is there at all. The
// content is LF-normalized, so a checkout with CRLF endings parses identically.
func manifest(root, name string) (string, bool, error) {
	b, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return textutil.NormalizeNewlines(string(b)), true, nil
}

// goModDependencies reads the require blocks of go.mod by hand.
//
// By hand rather than with golang.org/x/mod: scc is stdlib-only, the grammar in play
// here is two forms of one directive, and a dependency added to read a dependency file
// would be its own punchline.
func goModDependencies(root string, deps map[string]string) error {
	text, ok, err := manifest(root, "go.mod")
	if err != nil || !ok {
		return err
	}
	inBlock := false
	for _, raw := range strings.Split(text, "\n") {
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
	text, ok, err := manifest(root, "package.json")
	if err != nil || !ok {
		return err
	}
	var doc struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		// A manifest scc cannot parse produces no findings. It is not scc's place to
		// report on the syntax of somebody else's build file.
		return nil
	}
	for _, group := range []map[string]string{doc.Dependencies, doc.DevDependencies} {
		for name := range group {
			deps[name] = "package.json"
		}
	}
	return nil
}

// composerDependencies reads require and require-dev from composer.json, on the same
// terms as package.json.
//
// Platform requirements are dropped: "php", "hhvm", and the ext-/lib- prefixes are
// statements about the interpreter the code runs on, not packages anybody adopted, and
// a finding demanding a stack.md entry for "ext-json" is noise.
func composerDependencies(root string, deps map[string]string) error {
	text, ok, err := manifest(root, "composer.json")
	if err != nil || !ok {
		return err
	}
	var doc struct {
		Require    map[string]string `json:"require"`
		RequireDev map[string]string `json:"require-dev"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		return nil
	}
	for _, group := range []map[string]string{doc.Require, doc.RequireDev} {
		for name := range group {
			if isPlatformRequirement(name) {
				continue
			}
			deps[name] = "composer.json"
		}
	}
	return nil
}

func isPlatformRequirement(name string) bool {
	switch strings.ToLower(name) {
	case "php", "php-64bit", "php-ipv6", "hhvm", "composer", "composer-plugin-api", "composer-runtime-api":
		return true
	}
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "ext-") || strings.HasPrefix(lower, "lib-")
}

// requirementsDependencies reads requirements.txt, one requirement per line.
//
// Only the file with that exact name, at the root. requirements-dev.txt,
// requirements/base.txt, and a dozen other layouts all exist; globbing for them would
// be scc guessing which files in somebody's repo are dependency manifests, and a wrong
// guess produces findings about a file that was never a manifest.
func requirementsDependencies(root string, deps map[string]string) error {
	text, ok, err := manifest(root, "requirements.txt")
	if err != nil || !ok {
		return err
	}
	// A trailing backslash continues the requirement on the next line; joining first
	// means a continuation is never mistaken for a requirement of its own.
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\\\n", " "), "\n") {
		line := strings.TrimSpace(stripHashComment(raw))
		// An option line: -r, -e, -c, --index-url, --hash. Not a requirement, and -r
		// deliberately does not recurse — that file is a manifest scc was not pointed at.
		if line == "" || strings.HasPrefix(line, "-") {
			continue
		}
		if name := requirementName(line); name != "" {
			deps[name] = "requirements.txt"
		}
	}
	return nil
}

// requirementName pulls the project name off a PEP 508 requirement, or returns "" for
// anything that is not one — a bare URL, a VCS reference, a line scc does not
// recognize. Everything after the name is a version specifier, an extras list, a
// marker, or a direct URL, and all four start with a character that cannot appear in a
// name.
func requirementName(spec string) string {
	if i := strings.IndexAny(spec, ";@[("); i >= 0 {
		spec = spec[:i]
	}
	if i := strings.IndexAny(spec, "=<>!~ \t"); i >= 0 {
		spec = spec[:i]
	}
	spec = strings.TrimSpace(spec)
	if !pep508NameRe.MatchString(spec) {
		return ""
	}
	return spec
}

// pep508NameRe is the name grammar PyPI actually enforces: letters, digits, and the
// three separators, starting with an alphanumeric.
var pep508NameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// pyprojectDependencies reads the four places a Python project declares a direct
// dependency: PEP 621's [project] dependencies and optional-dependencies, PEP 735's
// dependency-groups, and Poetry's own tables.
//
// Poetry's "python" key is skipped — it constrains the interpreter, not the project's
// technology choices, and it is the one key in that table that is never a package.
func pyprojectDependencies(root string, deps map[string]string) error {
	text, ok, err := manifest(root, "pyproject.toml")
	if err != nil || !ok {
		return err
	}
	tables := tomlTables(strings.Split(text, "\n"))
	add := func(specs []string) {
		for _, spec := range specs {
			if name := requirementName(spec); name != "" {
				deps[name] = "pyproject.toml"
			}
		}
	}
	add(tomlArray(tables["project"], "dependencies"))
	for _, group := range []string{"project.optional-dependencies", "dependency-groups"} {
		for _, key := range tomlKeys(tables[group]) {
			add(tomlArray(tables[group], key))
		}
	}
	for table, lines := range tables {
		if !isPoetryDependencyTable(table) {
			continue
		}
		for _, key := range tomlKeys(lines) {
			if key == "python" || !pep508NameRe.MatchString(key) {
				continue
			}
			deps[key] = "pyproject.toml"
		}
	}
	return nil
}

// isPoetryDependencyTable matches [tool.poetry.dependencies] and the per-group
// [tool.poetry.group.<name>.dependencies].
func isPoetryDependencyTable(table string) bool {
	return table == "tool.poetry.dependencies" ||
		(strings.HasPrefix(table, "tool.poetry.group.") && strings.HasSuffix(table, ".dependencies"))
}

// cargoDependencies reads Cargo's dependency tables: the three kinds, their
// per-target variants, a workspace's shared table, and the [dependencies.<crate>]
// long form.
func cargoDependencies(root string, deps map[string]string) error {
	text, ok, err := manifest(root, "Cargo.toml")
	if err != nil || !ok {
		return err
	}
	for table, lines := range tomlTables(strings.Split(text, "\n")) {
		kind, crate := cargoTable(table)
		if !kind {
			continue
		}
		// [dependencies.serde] names the crate in the header; a plain table names one
		// per line.
		if crate != "" {
			if isCrateName(crate) {
				deps[crate] = "Cargo.toml"
			}
			continue
		}
		for _, key := range tomlKeys(lines) {
			if isCrateName(key) {
				deps[key] = "Cargo.toml"
			}
		}
	}
	return nil
}

// cargoTable reports whether a table header declares dependencies, and the single
// crate it names when it is the [dependencies.<crate>] long form.
func cargoTable(table string) (bool, string) {
	// [target.'cfg(unix)'.dependencies] and [workspace.dependencies] both end in a
	// dependency table; what precedes it only says when it applies.
	for _, kind := range []string{"dependencies", "dev-dependencies", "build-dependencies"} {
		switch {
		case table == kind, strings.HasSuffix(table, "."+kind):
			return true, ""
		}
		if i := strings.Index(table, kind+"."); i >= 0 && (i == 0 || table[i-1] == '.') {
			crate := table[i+len(kind)+1:]
			// A crate's own sub-table, e.g. [dependencies.serde.features], names the
			// crate first.
			if j := strings.Index(crate, "."); j >= 0 {
				crate = crate[:j]
			}
			return true, crate
		}
	}
	return false, ""
}

var crateNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func isCrateName(s string) bool { return crateNameRe.MatchString(s) }

// pomDependencies reads Maven's direct dependencies with encoding/xml.
//
// The path is deliberately project/dependencies/dependency and nothing else, which
// excludes two things on purpose: <dependencyManagement>, which pins versions for
// modules rather than adopting anything, and <profiles>, whose dependencies apply only
// when a profile the build was not necessarily run with is active. An artifactId that
// still carries an unresolved ${property} is skipped for the same reason a malformed
// manifest is — scc does not resolve Maven properties, so it does not know the name.
func pomDependencies(root string, deps map[string]string) error {
	text, ok, err := manifest(root, "pom.xml")
	if err != nil || !ok {
		return err
	}
	var pom struct {
		Dependencies []struct {
			ArtifactID string `xml:"artifactId"`
		} `xml:"dependencies>dependency"`
	}
	if err := xml.Unmarshal([]byte(text), &pom); err != nil {
		return nil
	}
	for _, d := range pom.Dependencies {
		name := strings.TrimSpace(d.ArtifactID)
		if name == "" || strings.Contains(name, "${") {
			continue
		}
		deps[name] = "pom.xml"
	}
	return nil
}

// tomlTables groups a TOML file's lines by the table they are under, keyed by the
// header without its brackets. Lines before the first header belong to "".
//
// **This is not a TOML parser and must not become one.** It recognizes exactly two
// shapes — a table header alone on a line, and `key = value` under it — which is how
// every dependency table in the wild is written. Everything else passes through as a
// line nothing asks a question about, so an exotic file yields no dependencies rather
// than wrong ones. Array-of-table headers ([[x]]) are treated as their table, which is
// right for the only thing that matters here: they never hold dependencies.
func tomlTables(lines []string) map[string][]string {
	tables := map[string][]string{}
	current := ""
	for _, raw := range lines {
		line := strings.TrimSpace(stripHashComment(raw))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.Trim(line, "[]")
			// A quoted key in a header — [target.'cfg(unix)'.dependencies] — keeps its
			// quotes, which is fine: nothing here matches on that segment.
			continue
		}
		tables[current] = append(tables[current], line)
	}
	return tables
}

// tomlKeys returns the left-hand side of every `key = value` line in a table, with
// quotes stripped.
func tomlKeys(lines []string) []string {
	var keys []string
	for _, line := range lines {
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.Trim(strings.TrimSpace(key), `"'`)
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

// tomlArray returns the quoted strings of `key = [...]`, spanning lines until the
// bracket closes. An array that never closes yields nothing — the file is not one this
// reader understands, and half of a dependency list is worse than none.
func tomlArray(lines []string, key string) []string {
	for i, line := range lines {
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.Trim(strings.TrimSpace(name), `"'`) != key {
			continue
		}
		value = strings.TrimSpace(value)
		if !strings.HasPrefix(value, "[") {
			return nil
		}
		var out []string
		for j := i; j < len(lines); j++ {
			text := lines[j]
			if j == i {
				text = value
			}
			out = append(out, tomlStringRe.FindAllString(text, -1)...)
			if strings.Contains(text, "]") {
				return unquote(out)
			}
		}
		return nil
	}
	return nil
}

// tomlStringRe matches a basic or literal TOML string. Dependency specifiers contain
// neither escapes nor newlines, which is what makes this safe to match with a regexp
// instead of a scanner.
var tomlStringRe = regexp.MustCompile(`"[^"]*"|'[^']*'`)

func unquote(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, strings.Trim(s, `"'`))
	}
	return out
}

// stripHashComment drops a # comment. A # inside a quoted string would be cut too,
// which is why this is only ever applied to dependency declarations: a version
// specifier, a crate name, and a PEP 508 requirement all cannot contain one.
func stripHashComment(line string) string {
	if i := strings.Index(line, "#"); i >= 0 {
		return line[:i]
	}
	return line
}
