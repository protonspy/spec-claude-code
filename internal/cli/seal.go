package cli

import (
	"flag"
	"fmt"

	"github.com/protonspy/spec-claude-code/internal/artifact"
	"github.com/protonspy/spec-claude-code/internal/render"
)

// Checking the seal, on the way in and on the way out.
//
// An approved plan's content is fixed: only the checklist's state moves, and only
// through `scc patch`. So every command that reads or writes one hashes it first and
// says so when the hash disagrees. The cost is a sha256 over a few kilobytes of a
// file that was going to be read anyway, which is why it can be on by default.
//
// Checking *before* applying an edit is the part that matters. A harness that edited
// the file by hand and then ran `scc patch check` would otherwise have its edit
// resealed on top, and the evidence would be gone in the same command that should
// have reported it.

func addNoVerify(fs *flag.FlagSet) *bool {
	return fs.Bool("no-verify", false, "skip the seal check on an approved plan (for diagnosis)")
}

// loadVerified is loadMany plus the seal check — the read path for every command
// that reports on a plan's content or its checklist. It returns the exit code, so a
// drifted plan stops the command instead of being answered from.
func loadVerified(root string, args []string, skip bool) ([]*artifact.Artifact, int) {
	arts, ok := loadMany(root, args)
	if !ok {
		return nil, ExitError
	}
	if code := sealGuard(arts, skip); code != ExitOK {
		return nil, code
	}
	return arts, ExitOK
}

// sealGuard reports every artifact whose content no longer matches its seal, and
// returns the exit code the caller should use.
//
// The message names both hashes and both ways out, because the reader is an agent
// that cannot see the file: an error it cannot act on is the same as no error.
func sealGuard(arts []*artifact.Artifact, skip bool) int {
	if skip {
		return ExitOK
	}
	code := ExitOK
	for _, a := range arts {
		recorded, actual, drifted := a.Drift()
		if !drifted {
			continue
		}
		code = ExitFindings
		render.Err(fmt.Sprintf("%s — drift: the file changed outside scc", a.Path))
		render.Detail(fmt.Sprintf("  seal:    %s  (status: %s)", short(recorded), artifact.StatusApproved))
		render.Detail(fmt.Sprintf("  actual:  %s", short(actual)))
		render.Detail("  an approved plan's content only changes through `" + prog() + " patch`.")
		render.Detail(fmt.Sprintf("  → revert it with git, or `%s plan reseal %s --force` if the edit was meant",
			prog(), a.Name))
	}
	return code
}

func short(sum string) string {
	if len(sum) > 12 {
		return sum[:12] + "…"
	}
	if sum == "" {
		return "(none recorded)"
	}
	return sum
}
