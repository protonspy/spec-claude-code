package cli

import (
	"encoding/json"
	"flag"
	"fmt"

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

// emitJSON writes v as indented JSON to stdout and returns the exit code.
//
// stdout carries nothing but the JSON document: diagnostics go to stderr (see
// package render), so a caller can pipe stdout straight into jq while still
// seeing warnings on the terminal. A marshal failure is the tool's own bug, so
// it reports on stderr and exits 1 rather than emitting half a document.
func emitJSON(v any) int {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		render.Err(fmt.Sprintf("could not encode JSON output: %v", err))
		return ExitError
	}
	fmt.Println(string(b))
	return ExitOK
}
