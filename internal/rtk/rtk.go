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
//
// Sharing RTK's markers rather than namespacing scc's own is the load-bearing
// choice here, and it is worth naming what it buys: `rtk init` writes this exact
// pair into the project's entry file, so addressing the block by these markers is
// what makes `rtk init` and `scc rtk` converge on one copy. A marker of scc's own
// — `scc:rtk-instructions`, say — would make each tool blind to the other's block
// and leave the file carrying both.
const (
	openPrefix = "<!-- rtk-instructions"
	closeTag   = "<!-- /rtk-instructions -->"
)

// Foreign is a marker some other tool writes for the same guidance.
//
// Headroom's context-tool setup appends RTK instructions to the same entry file
// behind its own namespaced pair. scc cannot address that block — it is
// Headroom's, written by Headroom, refreshed on Headroom's schedule — and it must
// not silently ignore it either, because a file carrying both blocks tells the
// agent the same thing twice in every request of the session. So: detected,
// named, and left exactly where it is.
type Foreign struct {
	// Tool is what to call it when telling somebody it is there.
	Tool string
	// Open is the marker that proves it.
	Open string
	// Fix is the command that removes it, run by the tool that owns it.
	Fix string
}

// foreigners are the blocks scc knows to look for. A short list on purpose: a
// marker that turns out not to be there costs one Contains call, and a marker
// nobody warns about costs a duplicated block in every session.
var foreigners = []Foreign{
	{
		Tool: "Headroom",
		Open: "<!-- headroom:rtk-instructions -->",
		Fix:  "headroom unwrap <agent>",
	},
}

// ForeignBlock reports the other tool's RTK block in doc, if there is one.
func ForeignBlock(doc string) (Foreign, bool) {
	for _, f := range foreigners {
		if strings.Contains(doc, f.Open) {
			return f, true
		}
	}
	return Foreign{}, false
}

// Action is what splicing the block did to a document.
type Action string

const (
	// Added: the document carried no block, so one was appended.
	Added Action = "added"
	// Present: the block in the document is already the one scc ships, byte for
	// byte, or the caller asked for an existing block to be kept.
	Present Action = "present"
	// Replaced: the document carried a different block, and scc's replaced it.
	//
	// This is the default, and the reason is size. `rtk init` and scc both stamp
	// v2 and give the agent the same instruction, but RTK's own block spends
	// roughly five times the bytes doing it — and the entry file is preloaded into
	// every request of the session, so the difference is paid continuously rather
	// than once. Between two blocks of the same version, the condensed one is
	// simply better, and leaving the larger one in place because it got there first
	// is not deference, it is a standing cost.
	//
	// What that does give up is version ordering: a future `rtk init` writing v3
	// would be overwritten by scc's v2. Splice therefore keeps the replaced block's
	// version visible so the caller can say so, and Keep is the standing answer for
	// anyone who has deliberately curated their own.
	Replaced Action = "replaced"
)

// InstallCmd is the command Install runs, as a string, so the CLI can name it
// before running it and can print it verbatim when cargo is missing and the user
// has to run it themselves.
func InstallCmd() string { return "cargo install --git " + Repo }

// Splice returns doc with the block present exactly once, and what it had to do to
// get there.
//
// The block scc ships wins by default — see Replaced for why. keep is the standing
// "leave whatever is already there", for a document whose block somebody curated
// on purpose.
//
// Only the region between the markers is ever rewritten. Everything outside them
// is untouched, always, and the document's own line endings are preserved: an
// entry file checked out CRLF stays CRLF, because scc is a guest in this file and
// rewriting every line of someone else's document is not a change they asked for.
//
// A document carrying an opening marker with no closing one is malformed rather
// than blockless, and it is an error: appending a second block there would leave
// the file with two openings and one close, which no tool could then update.
func Splice(doc, block string, keep bool) (string, Action, error) {
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
	if keep || rest[:end] == block {
		return doc, Present, nil
	}
	return doc[:start] + block + doc[start+end:], Replaced, nil
}

// Block returns the marker-delimited block in doc, markers included, or "" when
// doc carries none.
//
// It exists so a caller can measure what is already there against what it would
// have written. That comparison matters more than it sounds: `rtk init` and scc
// both stamp v2 and say the same thing, but RTK's own block spends roughly five
// times the bytes doing it, and the entry file is preloaded into every request of
// the session. Version ordering cannot separate those two — only size can.
func Block(doc string) string {
	start := strings.Index(doc, openPrefix)
	if start < 0 {
		return ""
	}
	rest := doc[start:]
	end := strings.Index(rest, closeTag)
	if end < 0 {
		return ""
	}
	return rest[:end+len(closeTag)]
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
