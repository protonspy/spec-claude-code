// Package render groups terminal output helpers used by the CLI surface.
//
// Status lines split across streams on purpose: Info/OK/Ask go to stdout, and
// Warn/Err to stderr. That split is what keeps stdout a clean, parseable stream
// when a command runs with --json (see internal/cli/jsonout.go) — diagnostics
// must never land in the JSON a caller is piping into jq.
package render

import (
	"fmt"
	"os"
)

var useColor = isTTY(os.Stdout) && os.Getenv("NO_COLOR") == ""

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func code(c, text string) string {
	if !useColor {
		return text
	}
	return "\033[" + c + "m" + text + "\033[0m"
}

func Cyan(s string) string   { return code("36", s) }
func Green(s string) string  { return code("32", s) }
func Yellow(s string) string { return code("33", s) }
func Red(s string) string    { return code("31", s) }
func Bold(s string) string   { return code("1", s) }

// Info prints a neutral status line to stdout.
func Info(msg string) { fmt.Println(Cyan("•") + " " + msg) }

// Ask prints an interactive prompt to stdout without a trailing newline so the
// user's answer appears on the same line.
func Ask(msg string) { fmt.Print(Cyan("?") + " " + msg) }

// OK prints a success status line to stdout.
func OK(msg string) { fmt.Println(Green("✓") + " " + msg) }

// Warn prints a warning status line to stderr.
func Warn(msg string) { fmt.Fprintln(os.Stderr, Yellow("!")+" "+msg) }

// Err prints an error status line to stderr.
func Err(msg string) { fmt.Fprintln(os.Stderr, Red("✗")+" "+msg) }

// Detail prints an unglyphed continuation line to stderr, for the body of a
// report whose heading was already printed by Warn or Err. It carries no status
// of its own, so it does not compete with the ✓/✗/! lines for attention.
func Detail(msg string) { fmt.Fprintln(os.Stderr, msg) }
