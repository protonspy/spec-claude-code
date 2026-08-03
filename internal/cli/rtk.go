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
	"github.com/protonspy/spec-claude-code/internal/workspace"
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
	force := fs.Bool("force", false, "replace an existing block with the one this scc ships (default: leave it alone)")
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

	report, code := applyRTK(target, rtkOptions{check: *check, noInstall: *noInstall, force: *force, quiet: *jsonOut})
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
	// force replaces a block that is already there. Off by default: the block
	// between RTK's markers is RTK's, and `rtk init` is what refreshes it.
	force bool
	// quiet suppresses the human status lines because the caller is emitting JSON
	// on stdout, which nothing else may touch.
	quiet bool
}

// rtkReport is the frozen JSON shape of a run, and the same values the human lines
// are printed from, so the two cannot describe different outcomes.
type rtkReport struct {
	Installed bool      `json:"installed"`
	Path      string    `json:"path,omitempty"`
	Version   string    `json:"version,omitempty"`
	Install   string    `json:"install"` // present | installed | skipped | failed
	Files     []rtkFile `json:"files"`
	Changed   int       `json:"changed"`
	Error     string    `json:"error,omitempty"`
}

type rtkFile struct {
	Path   string `json:"path"`
	Action string `json:"action"` // added | present | replaced | missing
	// Block is the version the opening marker claims, for a file that had one.
	// Reported, never compared: which version supersedes which is RTK's to say.
	Block string `json:"block,omitempty"`
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

// rtkMissing is the action for an entry file that is not on disk. scc writes the
// block into a document it does not own, so it declines to bring that document into
// existence: an entry file holding nothing but RTK's block would be a workspace
// missing its methodology while looking configured.
const rtkMissing = "missing"

// applyRTK does the work and returns the report plus the exit code, so `init --rtk`
// gets the same behavior without re-deriving any of it.
func applyRTK(root string, opts rtkOptions) (*rtkReport, int) {
	report := &rtkReport{Files: []rtkFile{}}
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
			// Already wired. Whose block it is and which version it claims is RTK's
			// business, so this reports and stops there.
			if !opts.quiet {
				render.Info(strings.TrimSpace(fmt.Sprintf("%s — RTK block already there %s", entry, file.Block)))
			}
		case rtkMissing:
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
				render.OK(fmt.Sprintf("%s — RTK block %s", entry, action))
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

// entryFiles lists the entry file of every harness this workspace was scaffolded
// for, deduplicated: Codex and opencode both read AGENTS.md, and splicing the same
// block into one file twice would append a second copy on the second pass.
func entryFiles(root string) []string {
	var out []string
	seen := map[string]bool{}
	for _, h := range workspace.Harnesses(root) {
		if seen[h.EntryFile] {
			continue
		}
		seen[h.EntryFile] = true
		out = append(out, h.EntryFile)
	}
	return out
}

func spliceEntry(root, entry, block string, opts rtkOptions) (rtkFile, error) {
	path := filepath.Join(root, entry)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return rtkFile{Path: entry, Action: rtkMissing}, nil
	}
	if err != nil {
		return rtkFile{}, err
	}
	next, action, err := rtk.Splice(string(raw), block, opts.force)
	if err != nil {
		return rtkFile{}, err
	}
	// The version reported is the one that ends up in the file: what was already
	// there when the block was left alone, what scc wrote when it did the writing.
	found := rtk.BlockVersion(string(raw))
	if action != rtk.Present {
		found = rtk.BlockVersion(block)
	}
	file := rtkFile{Path: entry, Action: string(action), Block: found}
	if action == rtk.Present || opts.check {
		return file, nil
	}
	if err := workspace.AtomicWrite(path, []byte(next), 0o644); err != nil {
		return rtkFile{}, err
	}
	return file, nil
}
