# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`scc` is a single Go binary (`github.com/protonspy/spec-claude-code`) that enforces Spec-Driven Development inside an agent-driven repo — Claude Code, Codex, or opencode. One surface: a headless CLI, so an AI agent or a CI job drives it exactly as well as a human does.

The artifacts it governs (the harness's own directory, `specs/`, `docs/`) are plain Markdown/JSON. There is no server and no database — the files *are* the API. scc has no directory of its own: its state lives in one file under the harness's directory, alongside what that tool already keeps there.

This file covers working *on* scc. The product's own rules and methodology are not documented here: they live in `design/` while being designed, and ship as templates the binary scaffolds into `<harness>/rules/`.

Note: this repo is not itself an scc workspace (no harness directory, `specs/`, or `docs/` are committed) — those trees only exist in workspaces the binary scaffolds, and in test temp dirs.

**Status: v0.4.0-shaped.** Everything through `design/plan.md` phase 10 is built and green: scaffolding (`init`), artifact creation (`spec`, `plan`), and all eight validators behind `scc validate`. `init` also scaffolds the six knowledge-base skills named in `design/orchestration.md` §6 — one per `docs/` artifact a validator checks, plus `prd` — each with a `scc-`-prefixed slash command derived from the same list (`assets.KnowledgeSkills`), wherever the harness has a command surface.

Two things landed after phase 10 and both are documented in `design/orchestration.md` §6 and §12:

- **Three harnesses, one template set.** `scc init --claude|--codex|--opencode` (Claude Code is the default, and a terminal with no flag gets a picker). One prose source: paths come from a `paths.Harness` profile and the header each loader parses is synthesized at render time — YAML frontmatter for Claude Code and opencode, a TOML agent role file for Codex.
- **`scc update` (phase 11), as replace-or-keep rather than the planned three-way merge.** It hashes every managed file against this build and against the manifest, prints the plan grouped by outcome, asks, and then replaces what is safe to replace. An edited file is kept and named; `--force` is the separate decision. `internal/merge` is still unbuilt.

`scc` is a redesign of `csdd` (`github.com/protonspy/csdd`), narrowed to spec-driven development and deliberately leaner. When reaching for something from there, port the *decision*, not the file. Already decided against: a TUI, an embedded web dashboard, an MCP server, a devcontainer.

## Commands

```bash
make build          # -> ./scc  (set VERSION=vX.Y.Z to stamp the version)
make check          # the CI gate: gofmt -l + go vet + go test -race
make test           # go test -race -coverprofile=coverage.out ./...
make fmt            # fails if anything is not gofmt-clean
make help           # every target with its one-line description
```

A single test / package:

```bash
go test ./internal/workspace/ -run TestSafeName -v
go test ./internal/cli/ -run 'TestVersion.*' -race
```

Lint locally the way CI does (golangci-lint v2 schema, conservative set in `.golangci.yml`):

```bash
golangci-lint run          # v2.12.2 in CI
govulncheck ./...          # CI fails the build on reachable vulns
```

Release is manual and CI-driven: `make release VERSION=v0.1.0` dispatches `release.yml`. The `dist`/`npm-build`/`npm-publish` targets exist as a bootstrap/fallback path only.

## Architecture

```
cmd/scc/main.go         os.Exit(cli.Run(os.Args[1:]))
        |
   internal/cli         the whole command surface
        |
   scaffold · validate                 write / check
        |          \
   assets · manifest   ears · mdscan   templates, hashes, grammars
        |
   paths · workspace · render · textutil · finding
        |
   plain files on disk: <harness>/ · specs/ · plans/ · docs/ · CLAUDE.md|AGENTS.md
```

`internal/cli/cli.go` is the whole dispatcher: `Run(args)` switches on `args[0]` and hands off to `run<Resource>` in a file named for that resource. Each handler owns its own `flag.FlagSet`. Adding a subcommand means adding a case there plus one file — nothing is registered dynamically, so the command set is readable in one place.

| Package | Role |
|---|---|
| `internal/paths` | Every directory/file name in the on-disk layout, in one place, plus the `Harness` profile (`Claude`, `Codex`, `OpenCode`) that says where each tool keeps things. Never hardcode `".claude"` or `"specs"` elsewhere — the harness-relative paths are methods on `Harness`. |
| `internal/workspace` | Resolves the root by walking up for *any* harness's `scc-manifest.json` marker; `Harnesses(root)` says which trees exist. Owns `KebabCheck`, `SafeName`, `AtomicWrite`. Knows nothing about specs or wikis. |
| `internal/render` | CLI terminal output (`✓ ✗ ! •`, `NO_COLOR`/TTY aware), split across stdout/stderr. |
| `internal/textutil` | Line-ending and BOM normalization, in exactly one place. |
| `internal/finding` | One finding type and one frozen JSON shape (`{findings, count}`) for every validator, plus the grouped human report. |
| `internal/manifest` | `<harness>/scc-manifest.json`: `{path, hash, version}` per managed file plus the harness, deterministic serialization, `Status → pristine\|edited\|missing`. Unknown fields are preserved. Every call takes the `paths.Harness` whose manifest it means. |
| `internal/assets` | The embedded template set — rules, review agents, skills, slash commands, artifact templates. **Workspace templates are data-free except for the harness profile** (a `(version, harness)` pair still renders byte-identically everywhere, and the manifest records both, so the future three-way merge can still reconstruct the old side); **artifact templates take data** (`spec new` renders them and the user owns the result). `Render(h, file)` is the only way to get a workspace file's bytes: it expands paths and synthesizes the per-harness header for agents and commands. `Version` is the template-set version and must be bumped whenever a workspace template changes. |
| `internal/scaffold` | Applies the template set to a root (`Apply`) and brings an existing one current (`PlanUpdate`/`ApplyUpdate`). Idempotent, never overwrites without being told to, manifest written last. |
| `internal/mdscan` | The only Markdown parser: fence- and HTML-comment-aware headings, checkboxes, links, wikilinks, slugs, plus a small frontmatter reader. `Body` is the comment/fence-stripped text every validator applies its grammar to. |
| `internal/ears` | EARS requirement parsing, all five patterns plus complex. |
| `internal/validate` | The eight validators, one file each, sharing `mdscan` and `finding`. |
| `internal/cli` | The dispatcher and every command handler. |

`go.mod` is stdlib-only. Keep it that way unless a dependency earns its place — the binary is distributed to six platforms and every dep is a supply-chain surface.

## Conventions

**Exit codes are the contract.** `0` ok · `1` usage/runtime error · `2` validation findings. Every lint/validate command returns `2` on findings so CI and agents can branch on it. A finding is a legitimate answer to a lint question, not a failure of the tool — don't collapse `2` into `1`.

**A validator that fires on scc's own output is the worst bug in the product.** The templates carry their instructions in HTML comments and fenced examples, which is exactly what `mdscan` excludes — and `TestFreshArtifactsPassTheirOwnValidators` in `internal/cli` is the gate. Treat it as required reading before changing a template or a validator: one wrong finding teaches the user to disbelieve all eight.

**Machine-readable output.** Bind the `--json` flag via `addJSON` and emit through `emitJSON` (`internal/cli/jsonout.go`) so the flag name, help text, and stdout/stderr split stay identical across commands — stdout is a clean JSON stream, diagnostics go to stderr.

**Hostile input.** Any positional name that becomes a path segment must pass `workspace.SafeName` *before* `filepath.Join` — without it `scc spec delete .. --force` resolves to the workspace root. Resource names are kebab-case (`workspace.KebabCheck`).

**Writes are atomic.** Use `workspace.AtomicWrite` for anything a concurrent reader might see.

**The marker is the file `<harness>/scc-manifest.json`, never the harness directory.** Two reasons, and both are load-bearing: every harness has a global twin in the user's home (`~/.claude`, `~/.codex`, `~/.config/opencode`) that exists on any machine running that tool, so an upward walk accepting the *directory* would resolve the root to `$HOME` for any command run outside a workspace — every command would then read and write the user's global configuration. And those directories exist in every repo that merely *uses* the tool, where scc was never initialized. `workspace.Find` therefore stats a regular file, for each harness in turn.

**scc has exactly one file per harness and no config file.** The manifest is it — content hashes, doubling as the marker. scc runs no tests and no linters, so it has nothing to configure; a project's test and lint commands are a rule under `<harness>/rules/`, which is Markdown the orchestrator already reads. Resist adding `scc.json`: a JSON schema to version, read by nothing inside the binary, is dead weight.

**Adding a harness touches one place.** A new `paths.Harness` value plus its entry in `paths.Harnesses()` is the whole change — `init`'s flags, the picker, `update`'s targets, the skills validator's search path, and the workspace walk all derive from that list. If a change needs a `switch h.ID` outside `assets.renderAgent`/`renderCommand`, the profile is missing a field.

**Never author what the user owns.** Upgrades preserve user-edited files rather than overwriting them; the manifest of content hashes is what makes a pristine file distinguishable from an edited one.

**Determinism.** Normalize rendered output to LF (`textutil.NormalizeNewlines`) so scaffolded files hash identically regardless of the build machine's checkout. Sort before serializing.

**Windows is a first-class target** and CI runs the suite on Linux/macOS/Windows. `.gitattributes` forces LF everywhere — CRLF would break gofmt, manifest hashes, and shell hooks. Watch for path separators, file-locking/delete-pending behavior, and the fact that the race detector doesn't run on the Windows job.

## Testing

Tests live beside the code and lean on a few package-local helpers rather than a framework:

- `capture(t, f)` in `internal/cli/cli_test.go` swaps `os.Stdout`/`os.Stderr` for pipes — the only safe way to assert on CLI output, since `render` writes to the real files. `run(t, args...)` wraps it around `cli.Run`. It drains both pipes concurrently, so a command that outruns the pipe buffer fails the test instead of deadlocking.
- `cli.Run` returns an exit code and never calls `os.Exit`, so the whole surface is drivable in-process.
- Compare resolved paths with `os.SameFile`, not string equality: `t.TempDir()` can sit under a symlink (`/var` → `/private/var` on macOS) and Windows reports 8.3 short names.
- `_test.go` files are exempt from `errcheck`/`staticcheck` in `.golangci.yml` — deliberate, for patterns like `Hash("a") != Hash("a")` asserting determinism.

## Releasing

`release.yml` is `workflow_dispatch` only: validate the version → run the full CI gate → cross-compile 6 targets → publish the npm packages → tag + GitHub Release. Details that are load-bearing:

- **A version is immutable.** Re-dispatching an already-released version from a *different* commit is refused, because publishing is idempotent and the run would otherwise go green having shipped nothing.
- **Publishing is idempotent.** Already-published packages are skipped, so a run that died after `npm-publish` can be resumed by re-dispatching the same commit.
- **Adding a platform touches three places** that must agree: `TARGETS` in `npm/scripts/build-packages.mjs`, `PLATFORMS` in the `Makefile`, and the `build` matrix in `release.yml`.
- Actions are pinned by commit SHA. Keep them pinned.

## Commits

Conventional Commits, scoped by package or surface, with a descriptive subject written as a claim about behavior — e.g. `feat(cli): return exit 2 when spec validation reports findings`. Changes land through PRs on `main`.
