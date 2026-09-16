package render

import (
	"io"
	"os"
	"strings"
	"testing"
)

// capture swaps the real streams for pipes and returns what each one received.
//
// The package writes to os.Stdout and os.Stderr by name rather than to an
// io.Writer, which is deliberate — the CLI has one terminal and nothing to inject
// — so a test that wants the bytes has to take the file descriptors.
func capture(t *testing.T, f func()) (stdout, stderr string) {
	t.Helper()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	realOut, realErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW

	// One channel each, drained concurrently: a single channel would leave the two
	// streams told apart by whichever write flushed first, which is not a fact.
	outC, errC := make(chan string, 1), make(chan string, 1)
	read := func(r *os.File, c chan<- string) {
		b, _ := io.ReadAll(r)
		c <- string(b)
	}
	go read(outR, outC)
	go read(errR, errC)

	f()
	os.Stdout, os.Stderr = realOut, realErr
	outW.Close()
	errW.Close()
	return <-outC, <-errC
}

// The split is the contract: stdout stays a clean stream a caller can pipe into
// jq, so every diagnostic goes to stderr. A status line on the wrong stream
// corrupts the JSON document of every --json command at once.
func TestStatusLinesSplitAcrossStreams(t *testing.T) {
	for _, tc := range []struct {
		name   string
		call   func()
		glyph  string
		stderr bool
	}{
		{"Info", func() { Info("hello") }, "•", false},
		{"OK", func() { OK("hello") }, "✓", false},
		{"Ask", func() { Ask("hello") }, "?", false},
		{"Warn", func() { Warn("hello") }, "!", true},
		{"Err", func() { Err("hello") }, "✗", true},
		{"Detail", func() { Detail("hello") }, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr := capture(t, tc.call)
			got, quiet := stdout, stderr
			if tc.stderr {
				got, quiet = stderr, stdout
			}
			if !strings.Contains(got, "hello") {
				t.Errorf("message missing from the stream it belongs on: %q", got)
			}
			if strings.Contains(quiet, "hello") {
				t.Errorf("message also reached the other stream: %q", quiet)
			}
			if tc.glyph != "" && !strings.Contains(got, tc.glyph) {
				t.Errorf("%q carries no %s glyph", got, tc.glyph)
			}
		})
	}
}

// Ask leaves the cursor where the answer is typed, which is the one thing that
// distinguishes it from Info.
func TestAskDoesNotEndTheLine(t *testing.T) {
	stdout, _ := capture(t, func() { Ask("Install it now?") })
	if strings.HasSuffix(stdout, "\n") {
		t.Errorf("Ask ended the line, so the answer appears below the question: %q", stdout)
	}
}

func TestColorIsOffWhenNobodyIsLooking(t *testing.T) {
	// A pipe is not a character device, so the package-level decision already made
	// at init is the one under test here.
	if useColor {
		t.Skip("this run has a TTY on stdout")
	}
	if got := Cyan("x"); got != "x" {
		t.Errorf("Cyan wrapped %q with escapes on a non-TTY", got)
	}
}

func TestColorWrapsEachGlyphWhenItIsOn(t *testing.T) {
	restore := useColor
	useColor = true
	defer func() { useColor = restore }()

	for name, got := range map[string]string{
		"Cyan":   Cyan("x"),
		"Green":  Green("x"),
		"Yellow": Yellow("x"),
		"Red":    Red("x"),
		"Bold":   Bold("x"),
	} {
		if !strings.HasPrefix(got, "\033[") || !strings.HasSuffix(got, "\033[0m") {
			t.Errorf("%s produced %q, which is not an escape-wrapped string", name, got)
		}
		if !strings.Contains(got, "x") {
			t.Errorf("%s lost the text: %q", name, got)
		}
	}
}

// isTTY answers false for anything that is not a character device, and for a file
// it cannot stat at all — a closed descriptor is the case that would otherwise
// panic on the nil FileInfo.
func TestIsTTY(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "render-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if isTTY(f) {
		t.Error("a regular file reported as a terminal")
	}
	f.Close()
	if isTTY(f) {
		t.Error("a closed file reported as a terminal")
	}
}
