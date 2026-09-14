package cli

import (
	"fmt"
	"os"

	"github.com/protonspy/spec-claude-code/internal/devcontainer"
	"github.com/protonspy/spec-claude-code/internal/jail"
	"github.com/protonspy/spec-claude-code/internal/render"
)

// The sandbox, chosen by what this platform can actually do.
//
// Two backends and one policy. On Linux and macOS it is ai-jail, which is a
// kernel boundary — bubblewrap or sandbox-exec — and which hides `~/.ssh`,
// `~/.aws` and `~/.gnupg` outright. On Windows there is no such backend and is
// unlikely to be one, so it is a dev container: a Docker boundary, weaker, and
// the only isolation available to somebody who is not going to install WSL2.
//
// **The two are not equals, and nothing here pretends they are.** Anthropic's own
// dev container guidance warns that under `--dangerously-skip-permissions` a
// container "does not prevent a malicious project from exfiltrating anything
// accessible inside the container, including the Claude Code credentials stored
// in `~/.claude`" — which are mounted in precisely so a rebuild does not sign you
// out. So the container is reported as what it is, WSL2 is still named as the
// better Windows answer, and a user who wants the strong boundary is told where
// it lives.
//
// **Default-on, with the install as the opt-in.** A sandbox nobody turns on is a
// sandbox nobody has. But a default cannot refuse the way `--jail` does — nobody
// typed it — so an absent backend degrades to a line rather than to a stopped
// launch, and the line names what would have contained this.
type sandboxOptions struct {
	noInstall bool
	yes       bool
	quiet     bool
	// plan is a --json or --dry-run run: report the sandbox, start nothing, and
	// above all build no container image, which is minutes of work nobody asked a
	// plan-only run to do.
	plan  bool
	extra []string
}

// containerReport says what happened on the dev container side of a launch.
type containerReport struct {
	// Wrapping is whether the agent actually runs inside the container.
	Wrapping bool   `json:"wrapping"`
	Path     string `json:"path,omitempty"`
	Version  string `json:"version,omitempty"`
	// Config is the directory the configuration was found in, when it was.
	Config string `json:"config,omitempty"`
	// Install is what happened to the CLI: present | installed | skipped | failed.
	Install string `json:"install"`
	// Weaker is always true when this report exists, and is written down anyway:
	// a JSON consumer should be able to see, without knowing the platform, that
	// this run got the lesser of the two boundaries.
	Weaker bool `json:"weaker_than_jail"`
	// Reason names why the agent is not in a container, for the run where that is
	// a surprise.
	Reason string `json:"reason,omitempty"`
}

// resolveSandbox picks the boundary this platform can give, and returns whichever
// of the two reports applies. Both nil means the agent runs on the host, and the
// user has been told why.
func resolveSandbox(root string, opts sandboxOptions) (*jailReport, *containerReport) {
	if jail.Supported() {
		return defaultJail(opts), nil
	}
	return nil, resolveContainer(root, opts)
}

// defaultJail is the jail as a default rather than as a demand.
//
// The difference from resolveJail is the whole point: this one never refuses. A
// missing binary is a line, not a stopped launch, because nobody asked for a
// sandbox by name and breaking `scc launch` for everybody who has not installed
// ai-jail would be scc's preference overruling the thing the user actually asked
// for. Installing ai-jail is how somebody says yes to this.
func defaultJail(opts sandboxOptions) *jailReport {
	if _, ok := jail.Path(); !ok {
		if !opts.quiet {
			render.Warn(fmt.Sprintf("starting without a sandbox: %s is not on PATH", jail.Bin))
			render.Detail(fmt.Sprintf("  install it and every `%s launch` here is contained: %s", prog(), jail.InstallCmd()))
			render.Detail(fmt.Sprintf("  or demand it now with `%s launch --jail`, which installs it and refuses to start without it", prog()))
		}
		return nil
	}
	// Present means consent. From here it is the same path `--jail` takes, and a
	// failure in it is still a refusal — the binary is here, so anything that goes
	// wrong now is a real problem rather than an absence.
	return resolveJail(jailOptions{noInstall: opts.noInstall, yes: opts.yes, quiet: opts.quiet, extra: opts.extra})
}

// resolveContainer is the Windows path: a dev container, or a line saying why not.
func resolveContainer(root string, opts sandboxOptions) *containerReport {
	report := &containerReport{Install: installSkipped, Weaker: true}

	if !devcontainer.Present(root) {
		report.Reason = fmt.Sprintf("this workspace has no %s/%s", devcontainer.Dir, devcontainer.Config)
		warnHostRun(report, opts)
		return report
	}
	report.Config = devcontainer.Dir

	bin, ok := devcontainer.Path()
	if !ok {
		switch {
		case opts.noInstall || opts.plan:
			report.Reason = devcontainer.Bin + " is not on PATH"
		case !devcontainer.Available():
			report.Reason = fmt.Sprintf("npm is not on PATH, so %s cannot be installed", devcontainer.Bin)
		case opts.yes:
			// Asked for by flag; no question to put.
		case opts.quiet || !interactive():
			report.Reason = fmt.Sprintf("%s is not on PATH, and nobody is here to answer the install prompt", devcontainer.Bin)
		default:
			render.Warn(fmt.Sprintf("%s is not on PATH — it is what runs the agent inside this workspace's container", devcontainer.Bin))
			render.Detail("  " + devcontainer.Docs)
			if !confirmInstall(promptIn, fmt.Sprintf("Install it now with `%s`?", devcontainer.InstallCmd())) {
				report.Reason = "install declined"
			}
		}
		if report.Reason != "" {
			warnHostRun(report, opts)
			return report
		}
		out := os.Stdout
		if opts.quiet {
			out = os.Stderr
		}
		render.Info(fmt.Sprintf("installing %s: %s", devcontainer.Bin, devcontainer.InstallCmd()))
		if err := devcontainer.Install(out, os.Stderr); err != nil {
			report.Install, report.Reason = installFailed, err.Error()
			warnHostRun(report, opts)
			return report
		}
		if bin, ok = devcontainer.Path(); !ok {
			report.Install = installFailed
			report.Reason = fmt.Sprintf("npm reported success but %s is still not on PATH", devcontainer.Bin)
			warnHostRun(report, opts)
			return report
		}
		report.Install = installInstalled
	} else {
		report.Install = installPresent
	}
	report.Path, report.Version = bin, devcontainer.Version(bin)

	// Docker is checked apart from the CLI because the two fail differently and a
	// user can only fix one at a time. `devcontainer` on PATH with no daemon is the
	// common Windows morning, and "install the CLI" is the wrong thing to tell
	// somebody who needs to start Docker Desktop.
	if !devcontainer.DockerReady() {
		report.Reason = "no Docker daemon is running"
		warnHostRun(report, opts)
		return report
	}

	// A plan-only run reports the container and builds nothing: `devcontainer up`
	// is an image build, which is minutes of work and the one thing --dry-run must
	// not do.
	if opts.plan {
		report.Reason = "plan-only run"
		return report
	}

	// The image, before the agent. Its output goes to stderr in both streams when
	// the caller is emitting JSON, because stdout carries the document.
	out := os.Stdout
	if opts.quiet {
		out = os.Stderr
	}
	render.Info(fmt.Sprintf("%s up — building the container if this is the first run", devcontainer.Bin))
	code, err := devcontainer.Run(bin, root, devcontainer.UpArgs(root), out, os.Stderr)
	switch {
	case err != nil:
		report.Reason = err.Error()
	case code != 0:
		report.Reason = fmt.Sprintf("%s up exited %d", devcontainer.Bin, code)
	default:
		report.Wrapping = true
		if !opts.quiet {
			render.OK("the agent will run inside the container")
			// Said on the run that gets it, not buried in documentation: this is the
			// weaker of the two boundaries, and the stronger one is a WSL2 install away.
			render.Detail("  a container is a weaker boundary than ai-jail — for the kernel sandbox, run scc inside WSL2")
		}
		return report
	}
	warnHostRun(report, opts)
	return report
}

// warnHostRun says, once, that the agent is about to run on the host after all.
//
// A warning rather than a status line because the run is about to do less than the
// default promises, and silence there is how somebody spends a month believing
// their agent is contained.
// It is silent only under --json, where stdout carries the document — and
// deliberately **not** silent under --dry-run, which is where the other
// integrations here do stay quiet. A plan-only run is somebody asking what would
// happen, and "you would not be contained" is the most important answer that run
// has to give.
func warnHostRun(report *containerReport, opts sandboxOptions) {
	if opts.quiet {
		return
	}
	render.Warn("starting on the host, outside any sandbox: " + report.Reason)
	if report.Config == "" {
		render.Detail(fmt.Sprintf("  `%s init` seeds %s/, and `/scc-init` fits it to this project", prog(), devcontainer.Dir))
		return
	}
	render.Detail("  " + devcontainer.InstallHint())
	render.Detail("  or run scc inside WSL2, where ai-jail's kernel sandbox works")
}
