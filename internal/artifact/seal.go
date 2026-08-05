package artifact

import (
	"strings"

	"github.com/protonspy/spec-claude-code/internal/manifest"
	"github.com/protonspy/spec-claude-code/internal/mdscan"
)

// The seal: tamper-evidence for an approved plan.
//
// What it is and is not, said here so nobody builds a guarantee on it later. It does
// not prevent an edit — `scc plan reseal --force` is one command away and sha256 is
// public. What it does is make an edit made outside scc *visible*: an approved plan
// whose content no longer hashes to its recorded checksum says so, by name, at the
// next command that touches it. The value is evidence and discipline, not security.
//
// It is opt-in by construction. A plan with no `status:` is not sealed and nothing is
// ever checked, which is what makes every plan written before this existed keep
// working untouched.

// The frontmatter keys the seal lives in.
const (
	KeyStatus   = "status"
	KeyChecksum = "checksum"
)

// StatusDraft and StatusApproved are the two phases of a plan's life. Draft is
// authorship, where everything is editable; approved is execution, where the content
// is fixed and only the checklist's state moves.
const (
	StatusDraft    = "draft"
	StatusApproved = "approved"
)

// Seal is the checksum recorded for content: sha256 over the file with its own
// `checksum:` line removed — otherwise the hash would have to describe itself — and
// with line endings normalized, so a checkout with CRLF seals identically to one
// with LF and Windows does not report permanent drift.
func Seal(content string) string {
	return manifest.Hash(withoutChecksum(content))
}

// Approved reports whether this artifact has been sealed for execution.
func (a *Artifact) Approved() bool { return a.Frontmatter[KeyStatus] == StatusApproved }

// Drift reports whether an approved artifact's content no longer matches its seal.
// A file that is not approved never drifts: there is nothing recorded to differ from.
func (a *Artifact) Drift() (recorded, actual string, drifted bool) {
	if !a.Approved() {
		return "", "", false
	}
	recorded = a.Frontmatter[KeyChecksum]
	actual = Seal(strings.Join(a.Lines, "\n"))
	return recorded, actual, recorded != "" && recorded != actual
}

// Approve stamps content as approved and seals it.
//
// The order is fixed and load-bearing: the status is written first, then the old
// checksum is dropped, then the hash is taken, then the checksum is written last.
// Taking the hash before the status was set would seal a file that no longer exists.
func Approve(content string) string { return reseal(content, StatusApproved) }

// Reseal recomputes the seal over content as it now stands, leaving the status
// alone. It is the answer to a legitimate edit made outside the cycle — a merge
// conflict resolved by hand — and it is deliberately a separate, forced command,
// because the same call made automatically would erase the evidence it exists to keep.
func Reseal(content string) string { return reseal(content, "") }

func reseal(content, status string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	lines, n := ensureFrontmatter(lines)
	if status != "" {
		lines, n = setKey(lines, n, KeyStatus, status)
	}
	lines, n = dropKey(lines, n, KeyChecksum)
	sum := manifest.Hash(strings.Join(lines, "\n"))
	lines, _ = setKey(lines, n, KeyChecksum, sum)
	return strings.Join(lines, "\n")
}

// withoutChecksum is the canonical form the hash covers.
func withoutChecksum(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	fm, err := mdscan.ParseFrontmatter(strings.Join(lines, "\n"))
	if err != nil || !fm.Present {
		return strings.Join(lines, "\n")
	}
	out, _ := dropKey(lines, fm.Lines, KeyChecksum)
	return strings.Join(out, "\n")
}

// EnsureFrontmatter returns the lines with a leading `---` block guaranteed, and how
// many lines that block occupies. Exported for migration, which has to write a plan's
// status into a file that may never have had a frontmatter block at all.
func EnsureFrontmatter(lines []string) ([]string, int) { return ensureFrontmatter(lines) }

// SetFrontmatterKey writes one key inside a frontmatter block whose extent the caller
// already knows, and returns the block's new length.
func SetFrontmatterKey(lines []string, fmLines int, key, value string) ([]string, int) {
	return setKey(lines, fmLines, key, value)
}

// ensureFrontmatter returns the lines with a leading `---` block guaranteed, and how
// many lines that block occupies.
func ensureFrontmatter(lines []string) ([]string, int) {
	fm, err := mdscan.ParseFrontmatter(strings.Join(lines, "\n"))
	if err == nil && fm.Present {
		return lines, fm.Lines
	}
	return append([]string{"---", "---", ""}, lines...), 2
}

// setKey writes one key inside the frontmatter block, replacing it in place if it is
// there and appending it just above the closing fence if it is not.
func setKey(lines []string, fmLines int, key, value string) ([]string, int) {
	for n := 2; n < fmLines; n++ {
		k, _, ok := strings.Cut(lines[n-1], ":")
		if ok && strings.TrimSpace(k) == key {
			lines[n-1] = key + ": " + value
			return lines, fmLines
		}
	}
	out := append([]string{}, lines[:fmLines-1]...)
	out = append(out, key+": "+value)
	return append(out, lines[fmLines-1:]...), fmLines + 1
}

func dropKey(lines []string, fmLines int, key string) ([]string, int) {
	for n := 2; n < fmLines; n++ {
		k, _, ok := strings.Cut(lines[n-1], ":")
		if ok && strings.TrimSpace(k) == key {
			out := append([]string{}, lines[:n-1]...)
			return append(out, lines[n:]...), fmLines - 1
		}
	}
	return lines, fmLines
}
