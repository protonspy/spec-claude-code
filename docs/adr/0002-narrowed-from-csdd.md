---
status: accepted
---

# Narrowed from csdd: no TUI, no dashboard, no MCP server

## Context

Reconstructed. `scc` is a redesign of `csdd` (`github.com/protonspy/csdd`), and the
evidence for the narrowing is what is absent: no HTTP server, no terminal UI, and no
MCP transport anywhere under `internal/`, against a command surface that is entirely
`flag.FlagSet` and a `Run(args)` switch.

The question the narrowing answers is *who actually drives this*. The answer is an
agent or a CI job as often as a person, and a surface built for a person is one an
agent drives badly — a TUI cannot be scripted, a dashboard cannot be asserted on, and
an MCP server makes the tool reachable only from inside a harness that speaks it.

## Decision

One surface: a headless CLI, with exit codes as the contract and `--json` wherever a
machine reads the output. Specifically decided against, and to be reopened by a new
record rather than by a pull request: a TUI, an embedded web dashboard, and an MCP
server.

When something from `csdd` is wanted here, the *decision* is ported and not the file.

## Consequences

- Every feature has to be expressible as arguments in and a document out. That is a
  real constraint on design — `scc map` and `scc patch` exist in the shape they do
  because a plan had to be readable and editable without a screen.
- The agent-facing surface is the same surface a person uses, so it is exercised
  constantly instead of by a second code path nobody runs.
- **One item on that list was later reversed, and it is recorded here rather than
  quietly dropped.** A dev container was on the "decided against" list and is now
  built: `internal/devcontainer` drives the Dev Containers CLI, because ai-jail has
  no Windows backend and the alternative was shipping no sandbox at all on one of
  the three platforms. The reversal does not weaken the other three — it is the
  editor-free `devcontainer up` / `devcontainer exec` path, not the VS Code flow, so
  the surface is still arguments in and a process out.
