package artifact

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/paths"
)

// TargetKind says what an address resolved to, so a caller can render it in the
// vocabulary the user already types.
type TargetKind string

const (
	TargetTask        TargetKind = "task"
	TargetRequirement TargetKind = "requirement"
	TargetSection     TargetKind = "section"
	TargetLeaf        TargetKind = "leaf"
	TargetBlock       TargetKind = "block"
	TargetRange       TargetKind = "range"
)

// Target is one resolved address: what it is, what it is called, and the lines it
// occupies.
type Target struct {
	Kind  TargetKind `json:"kind"`
	Ref   string     `json:"ref"`
	Label string     `json:"label"`
	Line  int        `json:"line"`
	End   int        `json:"end"`
}

// Lines is how many lines the target covers.
func (t Target) Lines() int { return t.End - t.Line + 1 }

var (
	taskRefRe  = regexp.MustCompile(`^\d+(?:\.\d+)*$`)
	reqRefRe   = regexp.MustCompile(`^(?i:R)\d+(?:\.\d+)+$`)
	rangeRefRe = regexp.MustCompile(`^(?i:L)?(\d+)(?:-(\d+))?$`)
	blockRefRe = regexp.MustCompile(`^([a-z0-9][a-z0-9-]*):(\d+)$`)
)

// Find resolves an address against the artifact.
//
// The forms, tried in this order:
//
//	1.2            a task, by its number
//	R1.2           a requirement, by its id
//	specs/foo/     a decomposition leaf, by the spec it names
//	#notes | Notes a section, by anchor slug or by the title as written
//	notes:7        the 7th paragraph of that section
//	L120-160       an explicit line range, the escape hatch
//
// None of them is a line number except the last, which is why an address survives an
// edit above it. Order matters where the forms could collide: a bare number is a
// task before it is a line, because a caller who means a line writes the L.
func (a *Artifact) Find(ref string) (Target, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Target{}, fmt.Errorf("no address given")
	}

	if taskRefRe.MatchString(ref) {
		if t, ok := a.Task(ref); ok {
			return Target{TargetTask, ref, t.Summary(72), t.Line, t.End}, nil
		}
		return Target{}, a.unknown("task", ref)
	}
	if reqRefRe.MatchString(ref) {
		if r, ok := a.Requirement(ref); ok {
			return Target{TargetRequirement, r.ID, clip(r.Text, 72), r.Line, r.End}, nil
		}
		return Target{}, a.unknown("requirement", ref)
	}
	if strings.HasPrefix(ref, paths.SpecsSeg+"/") {
		if l, ok := a.Leaf(ref); ok {
			return Target{TargetLeaf, l.Ref, clip(l.Text, 72), l.Line, l.End}, nil
		}
		return Target{}, a.unknown("leaf", ref)
	}
	if m := blockRefRe.FindStringSubmatch(ref); m != nil {
		n, _ := strconv.Atoi(m[2])
		for _, b := range a.Blocks() {
			if b.Section == m[1] && b.Index == n {
				return Target{TargetBlock, ref, clip(b.Lead, 72), b.Line, b.End}, nil
			}
		}
		return Target{}, a.unknown("block", ref)
	}
	if m := rangeRefRe.FindStringSubmatch(ref); m != nil && strings.ContainsAny(ref, "Ll-") {
		from, _ := strconv.Atoi(m[1])
		to := from
		if m[2] != "" {
			to, _ = strconv.Atoi(m[2])
		}
		if from < 1 || from > len(a.Lines) {
			return Target{}, fmt.Errorf("line %d is outside %s (%d lines)", from, a.Path, len(a.Lines))
		}
		if to > len(a.Lines) {
			to = len(a.Lines)
		}
		return Target{TargetRange, ref, "", from, to}, nil
	}
	if s, ok := a.Section(ref); ok {
		return Target{TargetSection, s.Slug, s.Title, s.Line, s.End}, nil
	}
	// A paragraph's own slug, which is what the notes index prints and therefore what
	// a caller is most likely to paste back.
	for _, b := range a.Blocks() {
		if b.Slug == ref {
			return Target{TargetBlock, fmt.Sprintf("%s:%d", b.Section, b.Index), clip(b.Lead, 72), b.Line, b.End}, nil
		}
	}
	return Target{}, a.unknown("address", ref)
}

// unknown says what was not found and what the artifact does have, because an
// address that misses is nearly always a near-miss and the caller is an agent that
// cannot see the file.
func (a *Artifact) unknown(kind, ref string) error {
	var have []string
	switch kind {
	case "task":
		for _, t := range a.Tasks {
			have = append(have, t.Number)
		}
	case "requirement":
		for _, r := range a.Requirements {
			have = append(have, r.ID)
		}
	case "leaf":
		for _, l := range a.Leaves {
			have = append(have, l.Ref)
		}
	default:
		for _, s := range a.Sections {
			have = append(have, "#"+s.Slug)
		}
	}
	if len(have) == 0 {
		return fmt.Errorf("no %s %q in %s, which has none", kind, ref, a.Path)
	}
	return fmt.Errorf("no %s %q in %s — it has %s", kind, ref, a.Path, listOf(have, 12))
}

func listOf(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf(", … (%d more)", len(items)-max)
}
