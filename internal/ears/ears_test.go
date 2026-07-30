package ears

import (
	"strings"
	"testing"
)

// All five patterns are valid, and this is the test that says so. A parser built on
// the event-driven pattern alone would reject four legitimate shapes and push authors
// to invent triggers for requirements that are simply always true.
func TestAllFivePatterns(t *testing.T) {
	cases := []struct {
		text    string
		want    Pattern
		system  string
		trigger string
	}{
		{
			text:   "The manifest writer shall record a content hash per managed file",
			want:   Ubiquitous,
			system: "manifest writer",
		},
		{
			text:   "While a merge is in progress, the CLI shall refuse to write the manifest",
			want:   StateDriven,
			system: "CLI",
		},
		{
			text:   "Where the workspace has a codewiki, the validator shall resolve every citation",
			want:   OptionalFeature,
			system: "validator",
		},
		{
			text:    "When a task is missing its methodology, the validator shall exit 2",
			want:    EventDriven,
			system:  "validator",
			trigger: "a task is missing its methodology",
		},
		{
			text:    "If the manifest is unreadable, then the CLI shall report the path and exit 1",
			want:    UnwantedBehavior,
			system:  "CLI",
			trigger: "the manifest is unreadable",
		},
	}
	for _, c := range cases {
		req, err := Parse(c.text)
		if err != nil {
			t.Errorf("%q: %v", c.text, err)
			continue
		}
		if req.Pattern != c.want {
			t.Errorf("%q: pattern = %q, want %q", c.text, req.Pattern, c.want)
		}
		if req.System != c.system {
			t.Errorf("%q: system = %q, want %q", c.text, req.System, c.system)
		}
		if req.Trigger != c.trigger {
			t.Errorf("%q: trigger = %q, want %q", c.text, req.Trigger, c.trigger)
		}
		if req.Response == "" {
			t.Errorf("%q: no response captured", c.text)
		}
	}
}

// More than one keyword is the complex pattern — a legitimate EARS shape, not a
// fallback for "did not fit".
func TestComplexPattern(t *testing.T) {
	req, err := Parse("While the session is gated, when a phase completes, the orchestrator shall stop and present it")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if req.Pattern != Complex {
		t.Errorf("pattern = %q, want %q", req.Pattern, Complex)
	}
	if len(req.Preconditions) != 1 || req.Preconditions[0].Keyword != "While" {
		t.Errorf("preconditions = %+v", req.Preconditions)
	}
	if req.Trigger != "a phase completes" {
		t.Errorf("trigger = %q", req.Trigger)
	}
	if req.System != "orchestrator" {
		t.Errorf("system = %q", req.System)
	}
}

// Zero or many preconditions: two of them is still valid, and still complex.
func TestManyPreconditions(t *testing.T) {
	req, err := Parse("While the branch is clean, where a remote exists, the CLI shall open a pull request")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(req.Preconditions) != 2 {
		t.Fatalf("preconditions = %+v, want 2", req.Preconditions)
	}
	if req.Pattern != Complex {
		t.Errorf("pattern = %q, want %q", req.Pattern, Complex)
	}
}

// The response is captured whole. EARS allows many responses, and splitting on "and"
// would invent a clause boundary the author did not write.
func TestResponseIsNotSplit(t *testing.T) {
	req, err := Parse("The CLI shall write the file and record its hash")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if req.Response != "write the file and record its hash" {
		t.Errorf("response = %q, want the whole clause", req.Response)
	}
}

// A style slip in capitalization is not a different requirement, and reporting it as
// unparseable would be a false positive.
func TestKeywordsAreCaseInsensitive(t *testing.T) {
	for _, text := range []string{
		"WHEN a task lands, THE validator SHALL run",
		"when a task lands, the validator shall run",
		"When a task lands, The Validator shall run",
	} {
		if _, err := Parse(text); err != nil {
			t.Errorf("%q: %v", text, err)
		}
	}
}

// Extra whitespace, including a line wrapped by an editor, is not a defect.
func TestWhitespaceIsNormalized(t *testing.T) {
	req, err := Parse("  When   a task lands,\n  the validator  shall   run  ")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if req.System != "validator" || req.Response != "run" {
		t.Errorf("req = %+v", req)
	}
}

// Each error names what is missing. "This is not EARS" is not actionable; "no shall
// clause" is.
func TestErrorsNameWhatIsMissing(t *testing.T) {
	cases := map[string]string{
		"The system does nothing":                                   "shall",
		"The shall run":                                             "system name",
		"The validator shall":                                       "follows `shall`",
		"When a task lands the validator shall run":                 "not closed by a comma",
		"If the manifest is unreadable, the CLI shall exit 1":       "then",
		"When a task lands, when another lands, the CLI shall exit": "at most one trigger",
		"When a task lands, while gated, the CLI shall exit":        "clause order",
		"Once a task lands, the CLI shall exit":                     "not an EARS keyword",
		"":                                                          "empty",
	}
	for text, want := range cases {
		_, err := Parse(text)
		if err == nil {
			t.Errorf("%q parsed without error, want one mentioning %q", text, want)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error = %q, want it to mention %q", text, err, want)
		}
	}
}

// A word that merely starts with a keyword is not that keyword. Without the trailing
// space, a requirement about iframe rendering would parse as an `If`.
func TestKeywordsRequireAWordBoundary(t *testing.T) {
	req, err := Parse("The iframe renderer shall sandbox every frame")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if req.Pattern != Ubiquitous {
		t.Errorf("pattern = %q, want %q", req.Pattern, Ubiquitous)
	}
	if req.System != "iframe renderer" {
		t.Errorf("system = %q", req.System)
	}
}

// A clause carrying an internal comma still parses as a well-formed requirement. The
// clause text may be split in a place the author did not intend, which is why nothing
// reports on clause text — only on whether the requirement is well formed.
func TestInternalCommaDoesNotBreakWellFormedness(t *testing.T) {
	req, err := Parse("When the cart is empty, or has one item, the checkout shall skip the summary")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if req.System != "checkout" || req.Response == "" {
		t.Errorf("req = %+v, want the system and response intact", req)
	}
}
