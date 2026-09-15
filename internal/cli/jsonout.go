package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/protonspy/spec-claude-code/internal/render"
)

// jsonFlagHelp is the single wording every command shows for --json, so the flag
// reads identically across the whole surface.
const jsonFlagHelp = "emit machine-readable JSON on stdout"

// addJSON binds --json on fs. Every command that produces output binds it
// through this helper rather than declaring its own, so the flag name and help
// text can't drift apart between commands.
func addJSON(fs *flag.FlagSet) *bool {
	return fs.Bool("json", false, jsonFlagHelp)
}

// emitJSON writes v as JSON to stdout and returns the exit code.
//
// stdout carries nothing but the JSON document: diagnostics go to stderr (see
// package render), so a caller can pipe stdout straight into jq while still
// seeing warnings on the terminal. A marshal failure is the tool's own bug, so
// it reports on stderr and exits 1 rather than emitting half a document.
//
// Indented for a person, compact for everything else — the same test render
// already makes about color, and for the same reason. The indentation is there to
// be read, and the overwhelming reader of a `--json` document here is an agent
// that pays for every byte of it: measured on a six-task plan, `map tasks --json`
// is 2198 bytes indented and about a third of that is leading whitespace, against
// 599 bytes for the human listing of the same tasks. A terminal still gets the
// readable form, so nothing anybody looks at changes.
func emitJSON(v any) int {
	var b []byte
	var err error
	if isTerminal(os.Stdout) {
		b, err = json.MarshalIndent(v, "", "  ")
	} else {
		b, err = json.Marshal(v)
	}
	if err != nil {
		render.Err(fmt.Sprintf("could not encode JSON output: %v", err))
		return ExitError
	}
	fmt.Println(string(b))
	return ExitOK
}
