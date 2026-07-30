// Package ears parses a requirement written in EARS — the Easy Approach to
// Requirements Syntax (Mavin et al., Rolls-Royce, IEEE RE 2009).
//
// EARS is why scc can check a requirement at all. Prose has nothing to validate; an
// EARS clause has named parts, and a missing part is a finding rather than an
// opinion.
//
// The ruleset: zero or many preconditions · zero or one trigger · one system name ·
// one or many responses, and the clauses always appear in that order. That yields
// five patterns plus their combinations, and **all five are valid**:
//
//	The <system> shall <response>                            ubiquitous
//	While <precondition>, the <system> shall <response>       state-driven
//	Where <feature>, the <system> shall <response>            optional feature
//	When <trigger>, the <system> shall <response>             event-driven
//	If <trigger>, then the <system> shall <response>          unwanted behavior
//
// A parser built on the event-driven pattern alone — the shape most people picture
// when they hear EARS — would reject four legitimate patterns and, worse, push
// authors to invent a trigger for a requirement that is simply always true. Unwanted
// behavior (`If … then …`) is the pattern most often missing from generated
// requirements and the one that most often matters.
//
// # How it parses, and why in that order
//
// The response is found first, then the system, and only then the leading clauses.
// The obvious order — read clauses left to right, splitting on commas — breaks on a
// clause that contains a comma ("When the cart is empty, or has one item, the
// checkout shall …"), which is ordinary English and not a defect. Anchoring on
// `shall` and on the last `the` before it makes the clause region unambiguous, so an
// internal comma costs at most the exact clause text and never a spurious finding.
package ears

import (
	"fmt"
	"strings"
)

// Pattern names the EARS shape a requirement turned out to be.
type Pattern string

const (
	Ubiquitous       Pattern = "ubiquitous"
	StateDriven      Pattern = "state-driven"
	OptionalFeature  Pattern = "optional-feature"
	EventDriven      Pattern = "event-driven"
	UnwantedBehavior Pattern = "unwanted-behavior"
	Complex          Pattern = "complex"
)

// Requirement is one parsed EARS clause.
type Requirement struct {
	Pattern Pattern

	// Preconditions are the `While` and `Where` clauses, in the order written.
	Preconditions []Precondition

	// Trigger is the `When` or `If` clause, empty when there is none.
	Trigger string
	// TriggerKeyword is "When" or "If" — which one decides whether this describes an
	// event or unwanted behavior, and they are not interchangeable.
	TriggerKeyword string

	// System is the name between `the` and `shall`.
	System string

	// Response is everything after `shall`.
	//
	// EARS allows many responses, and this is deliberately one string rather than a
	// list: splitting on "and" would invent structure the author did not write, and
	// an invented clause boundary is a finding waiting to be wrong. What is
	// checkable is that there is a response at all.
	Response string
}

// Precondition is one leading clause and the keyword that introduced it.
type Precondition struct {
	Keyword string // "While" or "Where"
	Text    string
}

// The keywords, matched case-insensitively. EARS capitalizes them, but a requirement
// written in lowercase is a style slip and not a different requirement — reporting it
// as unparseable would be a false positive.
const (
	kwWhile = "While"
	kwWhere = "Where"
	kwWhen  = "When"
	kwIf    = "If"
	kwThen  = "then"
	kwThe   = "the"
	kwShall = "shall"
)

var leadingKeywords = []string{kwWhile, kwWhere, kwWhen, kwIf}

// Parse reads one requirement.
//
// The error names what is missing rather than guessing at intent: "no `shall`" is
// actionable, and "this is not EARS" is not. Every message is written to be a finding
// a reader can act on without knowing this package exists.
func Parse(text string) (Requirement, error) {
	norm := strings.Join(strings.Fields(text), " ")
	if norm == "" {
		return Requirement{}, fmt.Errorf("the requirement is empty")
	}

	head, response, err := splitShall(norm)
	if err != nil {
		return Requirement{}, err
	}
	if response == "" {
		return Requirement{}, fmt.Errorf("nothing follows `shall`: a requirement needs at least one response")
	}

	// The system is what follows the last `the` before `shall`. Searching from the
	// right is what keeps a precondition that contains the word "the" from being
	// mistaken for the system clause.
	at := lastWordIndex(head+" ", kwThe)
	if at < 0 {
		return Requirement{}, fmt.Errorf("expected `the <system> shall <response>`: no `the` before `shall` in %q", clip(head))
	}
	system := strings.TrimSpace(head[min(at+len(kwThe)+1, len(head)):])
	if system == "" {
		return Requirement{}, fmt.Errorf("no system name between `the` and `shall`")
	}

	req := Requirement{System: system, Response: response}
	if err := parseClauses(&req, strings.TrimSpace(head[:at])); err != nil {
		return req, err
	}
	req.Pattern = classify(req)
	return req, nil
}

// parseClauses reads the leading clause region — everything before `the <system>`.
//
// Splitting on commas and then reattaching a part that does not open with a keyword
// is what makes an internal comma harmless: "the cart is empty, or has one item"
// stays one trigger instead of becoming a second clause nobody wrote.
func parseClauses(req *Requirement, region string) error {
	if region == "" {
		return nil
	}
	var current *string       // the clause text being accumulated
	var currentKeyword string // the keyword that opened it
	thenSeen := false

	parts := strings.Split(region, ",")
	// A trailing empty part means the region ended with the comma that closes the
	// last clause. `If <trigger>, then` closes it too: the comma is before the
	// `then`, which belongs to the pattern rather than to the clause.
	last := strings.TrimSpace(parts[len(parts)-1])
	closed := last == ""
	if !closed && strings.EqualFold(last, kwThen) {
		thenSeen, closed = true, true
		parts = parts[:len(parts)-1]
	}

	for _, raw := range parts {
		part := strings.TrimSpace(raw)
		if part == "" {
			continue
		}
		if strings.EqualFold(part, kwThen) {
			if !strings.EqualFold(currentKeyword, kwIf) {
				return fmt.Errorf("`then` belongs to the unwanted-behavior pattern (`If <trigger>, then …`) and there is no `If` here")
			}
			thenSeen = true
			continue
		}
		kw, rest, ok := openingKeyword(part)
		if !ok {
			if current == nil {
				return fmt.Errorf("%q is not an EARS keyword: a requirement opens with While, Where, When, If, or `the <system> shall`",
					firstWord(part))
			}
			// A comma inside a clause. Put it back rather than inventing a boundary.
			*current += ", " + part
			continue
		}
		switch kw {
		case kwWhile, kwWhere:
			if req.Trigger != "" {
				return fmt.Errorf("`%s` appears after the trigger; EARS clause order is preconditions, then the trigger, then the system response", kw)
			}
			req.Preconditions = append(req.Preconditions, Precondition{Keyword: kw, Text: rest})
			current = &req.Preconditions[len(req.Preconditions)-1].Text
		case kwWhen, kwIf:
			if req.Trigger != "" {
				return fmt.Errorf("a requirement takes at most one trigger, and this one has two (`%s` and `%s`); split it in two", req.TriggerKeyword, kw)
			}
			req.Trigger, req.TriggerKeyword = rest, kw
			current = &req.Trigger
		}
		currentKeyword = kw
	}

	if strings.EqualFold(req.TriggerKeyword, kwIf) && !thenSeen {
		return fmt.Errorf("the unwanted-behavior pattern is `If <trigger>, then the <system> shall <response>` — the `then` is missing")
	}
	if !closed {
		return fmt.Errorf("the `%s` clause is not closed by a comma before `the %s shall`", currentKeyword, req.System)
	}
	return nil
}

// classify names the pattern. A combination is `complex`, which is a legitimate EARS
// pattern and not a fallback for "did not fit".
func classify(req Requirement) Pattern {
	switch {
	case len(req.Preconditions) > 1, len(req.Preconditions) > 0 && req.Trigger != "":
		return Complex
	case len(req.Preconditions) == 1:
		if req.Preconditions[0].Keyword == kwWhere {
			return OptionalFeature
		}
		return StateDriven
	case req.TriggerKeyword == kwIf:
		return UnwantedBehavior
	case req.Trigger != "":
		return EventDriven
	default:
		return Ubiquitous
	}
}

// splitShall divides a requirement at `shall`, returning what precedes it and the
// response. A `shall` at the very start of the clause still splits, so the error the
// caller reports is the missing system name rather than a missing keyword.
func splitShall(s string) (head, response string, err error) {
	lower := strings.ToLower(s)
	if i := strings.Index(lower, " "+kwShall+" "); i >= 0 {
		return s[:i], strings.TrimSpace(s[i+len(kwShall)+2:]), nil
	}
	if strings.HasPrefix(lower, kwShall+" ") {
		return "", strings.TrimSpace(s[len(kwShall):]), nil
	}
	// `shall` with nothing after it: the response is what is missing, and saying so
	// is more useful than reporting the keyword absent.
	if strings.HasSuffix(lower, " "+kwShall) || lower == kwShall {
		return strings.TrimSuffix(s, s[len(s)-len(kwShall):]), "", nil
	}
	return "", "", fmt.Errorf("no `shall` clause: an EARS requirement says what the system shall do")
}

// openingKeyword reports the EARS keyword a clause opens with.
func openingKeyword(part string) (keyword, rest string, ok bool) {
	for _, kw := range leadingKeywords {
		if len(part) > len(kw) && strings.EqualFold(part[:len(kw)], kw) && part[len(kw)] == ' ' {
			return kw, strings.TrimSpace(part[len(kw)+1:]), true
		}
	}
	return "", "", false
}

// lastWordIndex returns the index of the last case-insensitive occurrence of word
// that sits on a word boundary, or -1. The boundary check is what keeps "breathe "
// from matching "the ".
func lastWordIndex(s, word string) int {
	lower, target := strings.ToLower(s), strings.ToLower(word)+" "
	for i := strings.LastIndex(lower, target); i >= 0; i = strings.LastIndex(lower[:i], target) {
		if i == 0 || lower[i-1] == ' ' || lower[i-1] == ',' {
			return i
		}
	}
	return -1
}

func firstWord(s string) string {
	if i := strings.IndexByte(s, ' '); i > 0 {
		return s[:i]
	}
	return s
}

// clip shortens text for an error message. A finding that quotes an entire paragraph
// back at the user is a finding they will not read.
func clip(s string) string {
	const max = 40
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
