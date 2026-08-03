// Package rtk wires RTK — the CLI proxy that filters command output down to what
// is worth spending context on — into a workspace: finding or installing the
// binary, and keeping its usage block current inside the entry file.
//
// The block is spliced rather than written, because the entry file belongs to the
// user. RTK delimits its own instructions with an HTML comment pair carrying a
// version (`<!-- rtk-instructions v2 -->` … `<!-- /rtk-instructions -->`) and
// rewrites between them, so scc addressing the block by those same markers is what
// keeps `rtk init` and `scc rtk` converging on one copy instead of each appending
// its own. Everything outside the markers is untouched, always.
package rtk

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/textutil"
)

// Repo is where the binary is built from. RTK is a Rust program distributed as
// source, so cargo is the install path rather than a release download.
const Repo = "https://github.com/rtk-ai/rtk"

// Bin is the executable's name, as it appears on PATH and as it prefixes every
// command in the block.
const Bin = "rtk"

// The markers RTK itself writes. The opening one carries a version, so it is
// matched by prefix: a block stamped v1 or v9 is still the block, and replacing it
// with the one this build ships is exactly what an update means.
const (
	openPrefix = "<!-- rtk-instructions"
	closeTag   = "<!-- /rtk-instructions -->"
)

// Action is what splicing the block did to a document.
type Action string

const (
	// Added: the document carried no block, so one was appended.
	Added Action = "added"
	// Present: it already carried a block, which was left exactly as it was.
	//
	// This is the default outcome for an existing block whatever its version says,
	// and the reason is ownership: RTK writes that block, stamps its own version
	// into the opening marker, and `rtk init` is what refreshes it. An scc that
	// rewrote it on every run would silently downgrade a v3 block to whatever this
	// build happens to ship, and it would do it to a file the user owns.
	Present Action = "present"
	// Replaced: the caller passed force, so an existing block was overwritten with
	// the one this build ships.
	Replaced Action = "replaced"
)

// InstallCmd is the command Install runs, as a string, so the CLI can name it
// before running it and can print it verbatim when cargo is missing and the user
// has to run it themselves.
func InstallCmd() string { return "cargo install --git " + Repo }

// Splice returns doc with the block present exactly once, and what it had to do to
// get there.
//
// Insert only when the marker is absent. A document that already carries a block is
// returned untouched and reported Present, because that block is RTK's — see the
// Action constants. force is the explicit "replace it with what this scc ships",
// and it is the only path that ever overwrites one.
//
// It preserves the document's own line endings: an entry file checked out CRLF
// stays CRLF, because scc is a guest in this file and rewriting every line of
// someone else's document is not a change they asked for.
//
// A document carrying an opening marker with no closing one is malformed rather
// than blockless, and it is an error: appending a second block there would leave
// the file with two openings and one close, which no tool could then update.
func Splice(doc, block string, force bool) (string, Action, error) {
	block = strings.TrimRight(textutil.NormalizeNewlines(block), "\n")
	eol := "\n"
	if strings.Contains(doc, "\r\n") {
		eol = "\r\n"
		block = strings.ReplaceAll(block, "\n", "\r\n")
	}

	start := strings.Index(doc, openPrefix)
	if start < 0 {
		if strings.Contains(doc, closeTag) {
			return "", "", fmt.Errorf("found %s with no opening marker", closeTag)
		}
		trimmed := strings.TrimRight(doc, " \t\r\n")
		if trimmed == "" {
			return block + eol, Added, nil
		}
		return trimmed + eol + eol + block + eol, Added, nil
	}

	rest := doc[start:]
	end := strings.Index(rest, closeTag)
	if end < 0 {
		return "", "", fmt.Errorf("found %s with no closing %s", openPrefix+" …", closeTag)
	}
	end += len(closeTag)
	if !force || rest[:end] == block {
		return doc, Present, nil
	}
	return doc[:start] + block + doc[start+end:], Replaced, nil
}

// BlockVersion reports what the opening marker in doc claims — "v2" for
// `<!-- rtk-instructions v2 -->` — or "" when doc carries no block.
//
// Advisory: it is printed so a run that left a block alone says which one it left,
// and never compared. Version ordering is RTK's to define, not scc's to guess.
func BlockVersion(doc string) string {
	start := strings.Index(doc, openPrefix)
	if start < 0 {
		return ""
	}
	rest := doc[start+len(openPrefix):]
	end := strings.Index(rest, "-->")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

// Path reports where the rtk binary is, and whether it is on PATH at all.
func Path() (string, bool) {
	p, err := exec.LookPath(Bin)
	if err != nil {
		return "", false
	}
	return p, true
}

// Version reports what `rtk --version` says, or "" when the binary cannot answer.
// Advisory only: it is printed, never branched on, so a build of RTK that words
// its version differently costs nothing.
func Version(bin string) string {
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Install builds and installs RTK with cargo, streaming the build's own output to
// stdout and stderr — it takes minutes, and a silent command that long reads as a
// hang.
//
// Missing cargo is reported as itself rather than as a failed build: the user has
// to install a Rust toolchain, which is a different problem from a build that
// broke, and telling them "cargo install failed" would send them looking in the
// wrong place.
func Install(stdout, stderr io.Writer) error {
	cargo, err := exec.LookPath("cargo")
	if err != nil {
		return fmt.Errorf("cargo is not on PATH; install a Rust toolchain (https://rustup.rs), then run: %s", InstallCmd())
	}
	cmd := exec.Command(cargo, "install", "--git", Repo)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", InstallCmd(), err)
	}
	return nil
}
