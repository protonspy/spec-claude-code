# Stack

Every adopted technology, with one line on why it earned its place. **Technology
that is not listed here is an open decision, never something adopted silently.**

Reconstructed on 2026-09-15 from `go.mod`, `Makefile`, `.github/workflows/`,
`npm/`, `.devcontainer/`, and the integration packages under `internal/`. Where
nobody recorded a reason, this file says so rather than inventing one.

## The module

- **Go 1.25** — `go.mod` declares `go 1.25.0`; CI pins the toolchain ahead of it at
  `1.25.13`, deliberately, so `GOTOOLCHAIN=auto` still installs for older setups.
  Chosen for what the product is: one static binary, cross-compiled to six targets,
  that an agent or a CI job can run with nothing installed underneath it.
- **The standard library, and nothing else** — `go.mod` has no `require` block at
  all. The binary ships to six platforms and every dependency is a supply-chain
  surface, so a dependency has to be worth that. The cost is real and visible in the
  tree: `internal/mdscan` is a hand-written Markdown reader, `internal/manifest`
  hand-rolls TOML and `go.mod` parsing for seven dependency-file formats, and
  `internal/artifact` owns every grammar. That is code this project owns forever
  instead of a version it would have to keep current.

## Build and gate

- **GNU make** — `Makefile` is the documented developer entry point (`make build`,
  `make check`, `make test-report`, the release and npm targets). CI does not use
  it; it calls `go` directly, so the Makefile is convenience rather than
  load-bearing. It is not available on every developer machine — Windows hosts
  typically have no `make` — which is why the delivery gate runs `go` and
  `golangci-lint` directly instead.
- **gofmt** — the formatting authority, enforced in CI and by the `format` gate via
  `golangci-lint fmt --diff`, which exits non-zero on a diff where bare `gofmt -l`
  exits `0`. `.gitattributes` forces LF everywhere for the same reason: CRLF breaks
  gofmt and the manifest's content hashes.
- **golangci-lint v2** — CI pins `v2.12.2`. `.golangci.yml` enables a deliberately
  conservative set (`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`) so
  the gate stays high-signal rather than drowning in style nits. Cost: a v2 schema
  that is not the v1 one most examples online are written against.
- **govulncheck v1.7.0** — pinned, and fails the build on *reachable* vulnerable
  code paths. Pinned rather than `@latest` after v1.8.0 raised its `go` directive to
  1.26 and broke a job nothing else needed to change. It is the one scanner a
  stdlib-only module still needs, because the stdlib itself gets CVEs.
- **`tools/` — a nested, stdlib-only Go module** holding the delivery gate's test
  report. Nested rather than part of the main module so the root's `./...` never
  sees it: an untested helper package in the main module would land in the coverage
  profile at 0% and drag the floor down. Run as `go run -C tools ./testreport`,
  which behaves identically under `sh -c` and `cmd /c` — the two shells `scc check`
  uses.

## CI and release

- **GitHub Actions** — `ci.yml` (the gate: vet, build, race+coverage on Linux,
  macOS and Windows, plus gofmt, govulncheck and golangci-lint on Linux) and
  `release.yml` (manual `workflow_dispatch`: validate, gate, cross-compile six
  targets, publish npm, tag). Every action is pinned by commit SHA.
- **GitHub CLI (`gh`)** — dispatches the release workflow (`make release`), and
  `internal/git` reads a pull request's title and body through it for
  `scc validate --pr`. Read-only from scc's side; scc never writes with it.

## Distribution

- **npm** — the published install surface. One launcher package (`@protonspy/scc`)
  with six platform packages in `optionalDependencies`, so `npm i -g @protonspy/scc`
  pulls exactly one binary. Chosen because the audience already has node: every
  harness this tool targets is distributed that way.
- **Node.js ≥ 16** — `npm/scc/package.json` `engines`. The launcher is a small JS
  shim (`npm/scc/bin/scc.js`) that execs the real Go binary; `npm/scripts/build-packages.mjs`
  assembles the publish tree. Cost: a node dependency for people who only wanted a
  Go binary — which is why direct download from the GitHub Release stays supported.

## Integrated binaries — driven, never vendored

Each is a third-party CLI scc composes a command line for and starts. None is a Go
dependency; none is vendored. The shared reason is in `CLAUDE.md`: a third party's
binary name, install command and argument vocabulary all age on that third party's
schedule, so each lives in its own package and a version bump never touches the
dispatcher. The shared cost is that scc cannot guarantee any of them is present,
which is why every one of them degrades to starting the agent without it — except
the sandbox, which refuses.

- **RTK** (`internal/rtk`, `cargo install`) — a CLI proxy that filters command
  output before it reaches the agent's context. scc splices RTK's own usage block
  into the entry file so the agent actually uses it.
- **CodeGraph** (`internal/codegraph`, `npm install -g`) — the symbol graph behind
  `scc graph` and `.claude/rules/code-search.md`. npm is the only installer scc will run
  for it: CodeGraph's headline install pipes a remote script into a shell, which is
  a fine thing for a person to type and not a thing scc executes on their behalf.
- **Headroom** (`internal/headroom`, `uv` then `pip`) — a compression proxy for the
  agent's context. Opt-in behind `--headroom` because `headroom wrap` registers MCP
  servers into the agent's own config, and those outlive the session.
- **ai-jail** (`internal/jail`, `cargo install`) — the sandbox: bubblewrap on Linux,
  `sandbox-exec` on macOS. Integrated as a binary rather than reimplemented because
  a sandbox is security-critical kernel-interface work and a half-copy of one has
  the confidence of containment without the containment. No Windows backend.
- **Dev Containers CLI** (`internal/devcontainer`, `npm install -g`) — the Windows
  sandbox backend, `up` then `exec`, so the agent runs in the container without an
  editor in the loop. Weaker than ai-jail's boundary, and scc says so on the run
  that gets it.
- **Docker** — what the container backend runs on. Checked separately from the CLI
  because the two fail differently and a user can fix one at a time.
- **git** (`internal/git`) — read-only, always: does this branch still exist, has it
  landed, what has it added, what has not been pushed, what did it change. Nothing
  in that package installs or writes.

## The dev container image

- **`mcr.microsoft.com/devcontainers/javascript-node:22`** — `.devcontainer/Dockerfile`.
  Node-shaped whatever the project is written in, because three of the four agent
  tools it installs ship on npm and features are layered on after the Dockerfile has
  already run. The Go toolchain is installed on top of it.
