package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/assets"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/rtk"
)

// runRTK wires RTK into this workspace: the binary on PATH, and its usage block in
// the entry file every harness here loads.
//
// Two halves on purpose, because they fail independently. The block is scc's kind
// of work — a marker-delimited splice into a file the user owns, idempotent, safe
// to re-run. The install is not: it shells out to cargo, needs a network and a Rust
// toolchain, and takes minutes. --no-install is the seam for anyone who wants only
// the half scc is actually good at.
func runRTK(args []string) int {
	fs := flag.NewFlagSet("rtk", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	check := fs.Bool("check", false, "report only, write nothing; exit 2 when the block is missing")
	noInstall := fs.Bool("no-install", false, "never run cargo; only write the block")
	keep := fs.Bool("keep", false, "leave an existing block alone (default: replace it with the one this scc ships)")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !noPositionals(rest, "rtk") {
		return ExitError
	}

	target, ok := resolveRoot(*root)
	if !ok {
		return ExitError
	}
	if !requireWorkspace(target) {
		return ExitError
	}

	report, code := applyRTK(target, rtkOptions{check: *check, noInstall: *noInstall, keep: *keep, quiet: *jsonOut})
	if *jsonOut {
		if c := emitJSON(report); c != ExitOK {
			return c
		}
		return code
	}
	return code
}

// rtkOptions are the knobs both entry points share — the `rtk` command and
// `init --rtk`.
type rtkOptions struct {
	check     bool
	noInstall bool
	// keep leaves a block that is already there. Off by default: scc's block says
	// what RTK's says in roughly a fifth of the bytes, and the entry file is
	// preloaded into every request of the session.
	keep bool
	// quiet suppresses the human status lines because the caller is emitting JSON
	// on stdout, which nothing else may touch.
	quiet bool
}

// rtkReport is the frozen JSON shape of a run, and the same values the human lines
// are printed from, so the two cannot describe different outcomes.
type rtkReport struct {
	Installed bool        `json:"installed"`
	Path      string      `json:"path,omitempty"`
	Version   string      `json:"version,omitempty"`
	Install   string      `json:"install"` // present | installed | skipped | failed
	Files     []blockFile `json:"files"`
	Changed   int         `json:"changed"`
	Error     string      `json:"error,omitempty"`
}

// The values rtkReport.Install takes. Not rtk.Action: these describe the binary,
// which is either already there, newly built, deliberately left alone, or a failed
// build — a different question from what happened to the block.
const (
	installPresent   = "present"
	installInstalled = "installed"
	installSkipped   = "skipped"
	installFailed    = "failed"
)

// applyRTK does the work and returns the report plus the exit code, so `init --rtk`
// gets the same behavior without re-deriving any of it.
func applyRTK(root string, opts rtkOptions) (*rtkReport, int) {
	report := &rtkReport{Files: []blockFile{}}
	code := ExitOK

	if err := ensureBinary(report, opts); err != nil {
		report.Error = err.Error()
		render.Err(err.Error())
		code = ExitError
	}

	block, err := assets.RTKBlock()
	if err != nil {
		report.Error = err.Error()
		render.Err(err.Error())
		return report, ExitError
	}

	findings := false
	for _, entry := range entryFiles(root) {
		file, err := spliceEntry(root, entry, block, opts)
		if err != nil {
			report.Error = err.Error()
			render.Err(fmt.Sprintf("%s: %v", entry, err))
			code = ExitError
			continue
		}
		report.Files = append(report.Files, file)
		switch action := file.Action; action {
		case string(rtk.Present):
			// The block in the file is already scc's, or --keep asked for the one
			// that was there. Either way there is nothing to do.
			if !opts.quiet {
				render.Info(strings.TrimSpace(fmt.Sprintf("%s — RTK block already there %s", entry, file.Block)))
			}
		case blockMissing:
			// An initialized workspace whose entry file is gone: --check calls it a
			// finding, and a run that was asked to write the block reports that it
			// could not, rather than exiting 0 having done nothing.
			findings = true
			if !opts.check {
				code = ExitError
			}
			render.Warn(fmt.Sprintf("%s does not exist; run `%s init` first", entry, prog()))
		default:
			findings = true
			if opts.check {
				render.Warn(fmt.Sprintf("%s — RTK block would be %s", entry, action))
				continue
			}
			report.Changed++
			if !opts.quiet {
				render.OK(fmt.Sprintf("%s — RTK block %s%s", entry, action, sizeNote(file)))
			}
			// A replaced block that claimed a different version is the one case
			// where preferring scc's costs something, so it is said out loud
			// rather than left to be inferred from the file.
			if file.Was != "" {
				render.Warn(fmt.Sprintf("%s — the block it replaced claimed %s; scc ships %s", entry, file.Was, file.Block))
				render.Detail(fmt.Sprintf("  keep theirs with: %s rtk --keep", prog()))
			}
		}
	}

	// --check answers one question — is this workspace's entry file wired for RTK —
	// and answers it with the findings code, so CI branches on it the way it branches
	// on `scc validate`. An existing block is an answer of yes whatever version it
	// claims, so only a missing one is a finding. The binary is reported but
	// deliberately not counted either: a CI checkout has no reason to have RTK
	// installed, and failing there would train people to ignore the check.
	if opts.check && findings && code == ExitOK {
		return report, ExitFindings
	}
	return report, code
}

// sizeNote is the whole argument for replacing a block, in the unit that makes it:
// bytes the entry file no longer spends in every request of the session. Silent
// when nothing was replaced, and when the replacement was not actually smaller.
func sizeNote(file blockFile) string {
	if file.WasBytes <= file.Bytes {
		return ""
	}
	return fmt.Sprintf(" (%d → %d bytes)", file.WasBytes, file.Bytes)
}

// ensureBinary finds RTK, or builds it when it is missing and the user did not opt
// out. Returns an error only when an install was attempted and failed — everything
// else is a state, not a failure.
func ensureBinary(report *rtkReport, opts rtkOptions) error {
	if p, ok := rtk.Path(); ok {
		report.Installed, report.Path, report.Version, report.Install = true, p, rtk.Version(p), installPresent
		if !opts.quiet {
			render.Info(strings.TrimSpace("rtk found: " + p + " " + report.Version))
		}
		return nil
	}
	if opts.check || opts.noInstall {
		report.Install = installSkipped
		render.Warn(fmt.Sprintf("rtk is not on PATH; install it with: %s", rtk.InstallCmd()))
		return nil
	}

	render.Info(fmt.Sprintf("rtk is not on PATH; running %s — this takes a few minutes", rtk.InstallCmd()))
	// cargo's own output goes to stderr in both streams when the caller is emitting
	// JSON: stdout carries the document and nothing else.
	out := os.Stdout
	if opts.quiet {
		out = os.Stderr
	}
	if err := rtk.Install(out, os.Stderr); err != nil {
		report.Install = installFailed
		return err
	}
	p, ok := rtk.Path()
	if !ok {
		report.Install = installFailed
		return fmt.Errorf("cargo reported success but %s is still not on PATH; check that cargo's bin directory is in PATH", rtk.Bin)
	}
	report.Installed, report.Path, report.Version, report.Install = true, p, rtk.Version(p), installInstalled
	if !opts.quiet {
		render.OK(strings.TrimSpace("rtk installed: " + p + " " + report.Version))
	}
	return nil
}

// spliceEntry keeps RTK's block current in one entry file, and says so when another
// tool has written the same guidance behind its own markers.
//
// The foreign check is the half that is RTK's alone. Headroom's context-tool setup
// appends RTK instructions to this same file behind `<!-- headroom:rtk-instructions -->`;
// neither marker is a substring of the other, so both tools' idempotency checks pass
// and the file ends up telling the agent the same thing twice in every request.
// Detected, named, and left exactly where it is — that block belongs to Headroom.
func spliceEntry(root, entry, block string, opts rtkOptions) (blockFile, error) {
	raw, err := os.ReadFile(filepath.Join(root, entry))
	if err != nil && !os.IsNotExist(err) {
		return blockFile{}, err
	}
	file, err := spliceEntryBlock(root, entry, rtk.Markers, block, opts.keep, opts.check)
	if err != nil {
		return blockFile{}, err
	}
	if foreign, ok := rtk.ForeignBlock(string(raw)); ok {
		file.Foreign = foreign.Tool
		render.Warn(fmt.Sprintf("%s also carries %s's RTK block; the agent will read these instructions twice", entry, foreign.Tool))
		render.Detail("  remove that one with: " + foreign.Fix)
	}
	return file, nil
}
