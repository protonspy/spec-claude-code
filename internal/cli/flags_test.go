package cli

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Help is what the user asked for, so it is exit 0.
//
// Every command here binds its own FlagSet and every one of them routed
// flag.ErrHelp to ExitError, which put `scc map tasks --help` and `scc validate
// --help` on the same exit code as a command that could not run. The 0/1/2
// contract is what an agent branches on, so help reading as a failure is help an
// agent stops asking for.
func TestHelpIsNotAFailure(t *testing.T) {
	root := mapWorkspace(t)
	for _, args := range [][]string{
		{"map", "tasks", "--help"},
		{"map", "show", "--help"},
		{"validate", "--help"},
		{"patch", "check", "--help"},
		{"notes", "find", "--help"},
		{"notes", "add", "--help"},
		{"spec", "new", "--help"},
		{"plan", "new", "--help"},
		{"check", "set", "--help"},
		{"graph", "query", "--help"},
		{"hooks", "install", "--help"},
		{"init", "--help"},
		{"update", "--help"},
		{"launch", "--help"},
		{"rtk", "--help"},
		{"spec", "track", "--help"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, stderr, code := run(t, append(args, "--root", root)...)
			if code != ExitOK {
				t.Errorf("%v: exit %d, want %d", args, code, ExitOK)
			}
			if !strings.Contains(stderr, "-") {
				t.Errorf("%v: printed no usage:\n%s", args, stderr)
			}
		})
	}
}

// A genuine parse failure is still exit 1. Without this the fix above would turn
// every unknown flag into a silent success.
func TestAnUnknownFlagIsStillAnError(t *testing.T) {
	root := mapWorkspace(t)
	if _, _, code := run(t, "map", "tasks", "sample", "--nonesuch", "--root", root); code != ExitError {
		t.Errorf("exit %d, want %d for an unknown flag", code, ExitError)
	}
}

// A flag written after a positional is a flag, not prose.
//
// Go's flag package stops at the first non-flag argument, so the old single-pass
// parse read everything after the artifact name as more positionals. Measured
// before the fix: `notes add --tag gotcha "text" --path internal/x` wrote "--path
// internal/x" into the note body, recorded no path, and exited 0 — a misfiled
// record that reports success, which is worse than a rejected one.
func TestAFlagAfterAPositionalIsStillAFlag(t *testing.T) {
	root := mapWorkspace(t)
	if _, stderr, code := run(t, "notes", "add", "--tag", "gotcha", "a measured note",
		"--path", "internal/cli/flags.go", "--root", root); code != ExitOK {
		t.Fatalf("notes add: exit %d\n%s", code, stderr)
	}
	b, err := os.ReadFile(filepath.Join(root, "docs", "notes.md"))
	if err != nil {
		t.Fatal(err)
	}
	// The note that was just written, not the seeded guidance above it — which
	// quotes `--path` in its own worked example.
	var line string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, "a measured note") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("the note was not appended:\n%s", b)
	}
	if !strings.Contains(line, "@internal/cli/flags.go") {
		t.Errorf("the --path was not recorded as a path:\n%s", line)
	}
	if strings.Contains(line, "--path") {
		t.Errorf("the --path flag was written into the note as prose:\n%s", line)
	}
}

// parseFlags is driven directly for the shapes a command surface cannot reach:
// the terminator, a bare dash, and flags on both sides of a positional.
func TestParseFlagsFindsFlagsWhereverTheySit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		want  []string
		tag   string
		on    bool
		fails bool
	}{
		{name: "leading positional", args: []string{"x", "--tag", "a"}, want: []string{"x"}, tag: "a"},
		{name: "trailing positional", args: []string{"--tag", "a", "x"}, want: []string{"x"}, tag: "a"},
		{name: "flag after positional", args: []string{"--tag", "a", "x", "--on"}, want: []string{"x"}, tag: "a", on: true},
		{name: "positional between flags", args: []string{"--on", "x", "--tag", "a", "y"}, want: []string{"x", "y"}, tag: "a", on: true},
		{name: "terminator keeps a flag verbatim", args: []string{"x", "--", "--tag"}, want: []string{"x", "--tag"}},
		{name: "a bare dash is a positional", args: []string{"-", "--tag", "a"}, want: []string{"-"}, tag: "a"},
		{name: "unknown flag fails", args: []string{"x", "--nonesuch"}, fails: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("t", flag.ContinueOnError)
			fs.SetOutput(&strings.Builder{})
			tag := fs.String("tag", "", "")
			on := fs.Bool("on", false, "")
			got, err := parseFlags(fs, tc.args)
			if tc.fails {
				if err == nil {
					t.Fatalf("parseFlags(%v) = %v, want an error", tc.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFlags(%v): %v", tc.args, err)
			}
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("positionals = %v, want %v", got, tc.want)
			}
			if *tag != tc.tag {
				t.Errorf("--tag = %q, want %q", *tag, tc.tag)
			}
			if *on != tc.on {
				t.Errorf("--on = %v, want %v", *on, tc.on)
			}
		})
	}
}
