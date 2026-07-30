// Package mdscan reads the Markdown artifacts scc governs. Exactly one package
// knows how to parse them, so eight validators cannot disagree about what a task
// line or a heading is — and so the fence-awareness that prevents most false
// positives is written once.
//
// Nothing here parses source code. That boundary is deliberate and permanent: a
// per-language parser would make scc own every ecosystem it touches, and a checker
// that is confidently incomplete is worse than one that knows it is judgment.
package mdscan

import (
	"fmt"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/textutil"
)

// Frontmatter is the delimited block at the very top of an artifact — the one part
// of a Markdown file that is already machine-readable, which is why the kickoff
// answers (autonomy, ci) are recorded there.
type Frontmatter struct {
	// Present is true when the file opened with a --- delimited block.
	Present bool
	// Lines is how many lines the block occupies, delimiters included, so a caller
	// can report line numbers in the body without counting them again.
	Lines int
	// Values holds scalar keys in the order-insensitive form every consumer wants.
	Values map[string]string
	// Lists holds inline lists ([a, b, c]).
	Lists map[string][]string
	// Maps holds one level of nested scalars. Exactly one field scc reads needs
	// this — the Agent Skills spec's optional `metadata` mapping — and supporting
	// it here is cheaper than reporting a finding on a skill that is perfectly
	// valid. Nothing deeper is accepted.
	Maps map[string]map[string]string
}

// Get returns a scalar value and whether the key was present.
func (f Frontmatter) Get(key string) (string, bool) {
	v, ok := f.Values[key]
	return v, ok
}

const delimiter = "---"

// ParseFrontmatter reads the leading frontmatter block, if any.
//
// It is deliberately not a YAML parser: flat `key: value` pairs, optionally quoted
// strings, simple inline lists, and one level of nested scalars. Anything else —
// deeper nesting, block sequences, multi-line scalars, anchors — is an error rather
// than a guess. That is the right trade for this tool. A partial YAML parser that
// silently mis-reads a nested key produces a finding about a file that was fine,
// and one false positive costs more than a miss; carrying a real YAML dependency to
// read a handful of keys costs a supply-chain surface on six platforms.
//
// One level of nesting is in rather than out because the Agent Skills spec — an
// external contract scc conforms to rather than defines — has an optional
// `metadata` mapping. Refusing it would mean reporting a finding on a skill that is
// valid by the standard the validator exists to enforce.
//
// A file with no leading delimiter is not an error: frontmatter is optional
// everywhere scc reads it, and a missing block means "not recorded", never "wrong".
func ParseFrontmatter(content string) (Frontmatter, error) {
	fm := Frontmatter{
		Values: map[string]string{},
		Lists:  map[string][]string{},
		Maps:   map[string]map[string]string{},
	}
	text := textutil.NormalizeNewlines(content)
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], " \t") != delimiter {
		return fm, nil
	}
	fm.Present = true

	// pending is a key that arrived with no value: either an empty scalar or the
	// head of a nested mapping, and which one it is only becomes clear on the next
	// line. Its resolution is deferred rather than guessed.
	pending, pendingIndent := "", 0

	for i := 1; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], " \t")
		if line == delimiter {
			resolvePending(&fm, pending)
			fm.Lines = i + 1
			return fm, nil
		}
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}

		if indented := line != strings.TrimLeft(line, " \t"); indented {
			if pending == "" {
				return fm, fmt.Errorf("frontmatter line %d is indented under a key that already has a value", i+1)
			}
			width := len(line) - len(strings.TrimLeft(line, " \t"))
			if pendingIndent == 0 {
				pendingIndent = width
			} else if width != pendingIndent {
				return fm, fmt.Errorf("frontmatter line %d changes indentation inside %q; scc reads one level of nesting only", i+1, pending)
			}
			key, value, found := strings.Cut(strings.TrimSpace(line), ":")
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			if !found || key == "" || value == "" {
				return fm, fmt.Errorf("frontmatter line %d is not a `key: value` pair inside %q; scc reads one level of nesting only", i+1, pending)
			}
			if fm.Maps[pending] == nil {
				fm.Maps[pending] = map[string]string{}
			}
			if _, dup := fm.Maps[pending][key]; dup {
				return fm, fmt.Errorf("frontmatter line %d repeats the key %q inside %q", i+1, key, pending)
			}
			fm.Maps[pending][key] = unquote(value)
			continue
		}

		resolvePending(&fm, pending)
		pending, pendingIndent = "", 0

		key, value, found := strings.Cut(line, ":")
		if !found {
			return fm, fmt.Errorf("frontmatter line %d is not `key: value`: %q", i+1, line)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return fm, fmt.Errorf("frontmatter line %d has an empty key", i+1)
		}
		if fm.has(key) {
			return fm, fmt.Errorf("frontmatter line %d repeats the key %q", i+1, key)
		}
		value = strings.TrimSpace(value)
		if value == "" {
			pending = key
			continue
		}
		if items, isList, err := parseInlineList(value); err != nil {
			return fm, fmt.Errorf("frontmatter line %d: %w", i+1, err)
		} else if isList {
			fm.Lists[key] = items
			continue
		}
		fm.Values[key] = unquote(value)
	}
	return fm, fmt.Errorf("frontmatter opened at line 1 and was never closed by a %q line", delimiter)
}

// resolvePending records a valueless key as an empty scalar when no nested block
// followed it. `autonomy:` with nothing after it is a key the user left blank, which
// is different from a key that heads a mapping.
func resolvePending(fm *Frontmatter, pending string) {
	if pending == "" {
		return
	}
	if _, isMap := fm.Maps[pending]; isMap {
		return
	}
	fm.Values[pending] = ""
}

func (f Frontmatter) has(key string) bool {
	if _, ok := f.Values[key]; ok {
		return true
	}
	if _, ok := f.Lists[key]; ok {
		return true
	}
	_, ok := f.Maps[key]
	return ok
}

// parseInlineList reads `[a, b, c]`. A bare `[` with no closing bracket is a block
// list or a typo, and either way scc will not guess at it.
func parseInlineList(value string) (items []string, isList bool, err error) {
	if !strings.HasPrefix(value, "[") {
		return nil, false, nil
	}
	if !strings.HasSuffix(value, "]") {
		return nil, true, fmt.Errorf("unterminated inline list %q", value)
	}
	inner := strings.TrimSpace(value[1 : len(value)-1])
	if inner == "" {
		return []string{}, true, nil
	}
	for _, part := range strings.Split(inner, ",") {
		item := unquote(strings.TrimSpace(part))
		if item == "" {
			return nil, true, fmt.Errorf("empty item in list %q", value)
		}
		items = append(items, item)
	}
	return items, true, nil
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
