# This project

`scc` is one Go binary that enforces spec-driven development inside an agent-driven
repo. `cmd/scc/main.go` is three lines; `internal/cli/cli.go` switches on `args[0]`
and hands off to one file per resource. `go.mod` is **stdlib-only** — see
`docs/stack.md` before reaching for a dependency.

## Commands

```bash
# Build
go build ./...

# Test — the whole suite
go test ./...

# Test — one package or one test (used after every task; scope, not suite)
go test ./internal/workspace/ -run TestSafeName -v
go test ./internal/cli/ -run 'TestVersion.*'

# Test — the report the gate reads (~1 min; runs the whole suite)
go run -C tools ./testreport

# Lint — the conservative set in .golangci.yml
golangci-lint run

# Format check — NOT `gofmt -w`, which exits 0 having rewritten the tree
golangci-lint fmt --diff
```

**`-race` needs cgo and fails on a bare Windows host** (`go test -race` → "requires
cgo"); CI runs it on Linux/macOS only. Add `-race` there. `make` is not required and
is absent on Windows; the `Makefile` wraps these. `tools/` is a nested module the
root's `./...` never sees — hence `go run -C tools`.

## Conventions

- **Branches:** `feat/<slug>`, `fix/<slug>`, `refactor/<slug>`, `docs/<slug>`; PRs
  into `main`.
- **Commits:** Conventional Commits, scoped by package or surface, subject written
  as a claim about behavior — `feat(cli): return exit 2 when spec validation reports findings`.
- **Exit codes are the contract:** `0` ok · `1` usage/runtime error · `2` findings.
  Never collapse `2` into `1`. `--help` is `0`.
- **First-try mistakes:** hardcoding `".claude"` or `"specs"` instead of a
  `paths.Harness` method; adding a `switch h.ID` where the profile needs a field;
  forgetting `workspace.SafeName` before `filepath.Join` on a positional; asserting
  on a `--json` document's *spelling* instead of unmarshalling it.

## Boundaries

Not to be changed without asking: `go.mod`'s stdlib-only rule · `internal/assets`
template bytes without bumping `assets.Version` · the six-platform target list,
which lives in three files that must agree (`Makefile`, `npm/scripts/build-packages.mjs`,
`release.yml`) · pinned action SHAs and the pinned `govulncheck`/`golangci-lint`
versions · `CLAUDE.md`, which is the user's.
