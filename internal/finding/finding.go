// Package finding is the one finding type and the one output shape shared by
// every scc validator.
//
// It exists so eight validators cannot drift apart. A user who has learned to
// read one validator's output has learned to read all of them, a CI job that
// filters on one rule slug filters on any of them, and the JSON shape is frozen
// in a single place instead of re-invented per command.
//
// The discipline the package is built for: a finding the user cannot act on is
// noise wearing a useful shape, and one false positive teaches them to disbelieve
// all eight validators at once. So a finding always names a file, usually names a
// line, and always carries a stable rule slug — everything a reader needs to go
// fix it or to suppress it deliberately.
package finding

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/render"
)

// Exit codes, duplicated from the CLI's contract because the dependency runs the
// other way (cli imports finding, never the reverse). A test in package cli
// asserts these still equal cli.ExitOK / cli.ExitFindings, so the duplication
// cannot drift silently.
//
// A finding is a legitimate answer to a lint question, not a failure of the tool.
// That is why it has its own code: CI and agents branch on "ran and found
// problems" separately from "could not run".
const (
	ExitOK       = 0
	ExitFindings = 2
)

// Finding is one structural problem in one artifact.
//
// File is slash-separated and relative to the workspace root, so output is
// identical on every platform and a rule slug plus a path is a stable identity
// for a finding across machines.
//
// Rule is a stable slug — "ears.missing-shall", "skill.name-mismatch" — so a CI
// job can filter without matching prose. Renaming one is a breaking change to
// that contract; rewording Message is not.
type Finding struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"` // 1-based; omitted when the finding is about the file as a whole
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Set accumulates findings and hands them back in a stable order.
type Set struct {
	findings []Finding
}

// Document is the frozen JSON shape every validator emits under --json. Count is
// redundant with len(findings) and included anyway: it is what a shell one-liner
// reads, and it survives a caller that never looks at the array.
type Document struct {
	Findings []Finding `json:"findings"`
	Count    int       `json:"count"`
}

// Add records a finding.
func (s *Set) Add(f Finding) { s.findings = append(s.findings, f) }

// Addf records a finding with a formatted message — the form nearly every
// validator uses.
func (s *Set) Addf(file string, line int, rule, format string, args ...any) {
	s.Add(Finding{File: Rel(file), Line: line, Rule: rule, Message: fmt.Sprintf(format, args...)})
}

// Extend merges another set's findings into this one, which is how `scc validate`
// collects eight validators into one exit code and one document.
func (s *Set) Extend(other *Set) {
	if other == nil {
		return
	}
	s.findings = append(s.findings, other.findings...)
}

// Len is the number of findings.
func (s *Set) Len() int { return len(s.findings) }

// Empty reports whether the run found nothing.
func (s *Set) Empty() bool { return len(s.findings) == 0 }

// Sorted returns the findings ordered by (file, line, rule, message), which is
// the order both output paths use. Sorting on the way out rather than on insert
// lets a validator emit findings in whatever order it walks the artifact.
func (s *Set) Sorted() []Finding {
	out := make([]Finding, len(s.findings))
	copy(out, s.findings)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.File != b.File:
			return a.File < b.File
		case a.Line != b.Line:
			return a.Line < b.Line
		case a.Rule != b.Rule:
			return a.Rule < b.Rule
		default:
			return a.Message < b.Message
		}
	})
	return out
}

// Document returns the frozen JSON shape. Findings is always an array, never
// null, so a caller can index it without a nil check.
func (s *Set) Document() Document {
	f := s.Sorted()
	if f == nil {
		f = []Finding{}
	}
	return Document{Findings: f, Count: len(f)}
}

// ExitCode is the process exit code this set implies: 0 when nothing was found,
// 2 when something was.
func (s *Set) ExitCode() int {
	if s.Empty() {
		return ExitOK
	}
	return ExitFindings
}

// Report prints the findings for a human, grouped by file and led with a count.
//
// Grouping is not cosmetic. The documented way a catalogue of validators destroys
// its own value is volume — 35–91% of warnings go unread in studied static-analysis
// deployments — and a flat list of forty lines is the shape that gets skipped.
// A count first, then one block per file, keeps "few findings, each fixable" true
// of the output even when the input is not.
//
// Findings themselves go to stderr, following package render's split: they are
// diagnostics, and a caller running with --json must be able to pipe stdout into
// jq while still reading them.
func (s *Set) Report(subject string) {
	if s.Empty() {
		render.OK(subject + ": no findings")
		return
	}
	sorted := s.Sorted()
	render.Err(fmt.Sprintf("%s: %s", subject, plural(len(sorted))))
	var file string
	for _, f := range sorted {
		if f.File != file {
			file = f.File
			render.Warn(render.Bold(file))
		}
		render.Detail("  " + location(f) + "  " + f.Message + "  " + render.Cyan("["+f.Rule+"]"))
	}
}

func location(f Finding) string {
	if f.Line <= 0 {
		return "-"
	}
	return strconv.Itoa(f.Line)
}

func plural(n int) string {
	if n == 1 {
		return "1 finding"
	}
	return strconv.Itoa(n) + " findings"
}

// Rel normalizes a path for a finding: slash-separated, so the same artifact
// reports the same location on Windows and on Linux.
func Rel(p string) string {
	return strings.ReplaceAll(filepath.ToSlash(p), `\`, "/")
}
