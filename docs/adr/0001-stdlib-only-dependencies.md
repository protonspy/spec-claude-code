---
status: accepted
---

# Stdlib-only dependencies

## Context

Reconstructed from the repository rather than from the discussion that produced it:
`go.mod` declares no `require` block, `internal/validate/stack_manifests.go`
hand-rolls readers for seven dependency-file formats including TOML, the `Makefile`
and `release.yml` cross-compile six targets, and CI runs `govulncheck` with a pinned
version. Those four facts only make sense together under this decision, so it is
recorded here rather than inferred again by the next person who wants to add a
library.

`scc` is distributed as a binary to six platforms and is run inside other people's
repositories, frequently in CI. Every direct dependency is a supply-chain surface
that ships with it, and every indirect one is a surface nobody chose.

## Decision

`go.mod` stays stdlib-only. A dependency is added only when it earns its place
against that cost, and adding one is two acts rather than one: the `require` line,
and an entry in `docs/stack.md` saying why — which the stack validator enforces by
reporting a declared dependency that `stack.md` does not list.

The rule is scoped to the module that ships. `tools/` is a nested module the root's
`./...` never sees, and the npm packaging under `npm/` is JavaScript build
machinery; neither ends up inside the distributed binary.

## Consequences

- Real work is hand-rolled that a library would have done: TOML and `go.mod`
  parsing for the stack validator, the Markdown scanner, the BM25 ranking behind
  `map find`. The second of those is load-bearing anyway — `mdscan` is the only
  parser, and a general-purpose one would not strip HTML comments and fences the way
  every validator depends on.
- `govulncheck` still runs and CI still fails on a reachable vulnerability, because
  the stdlib itself gets CVEs. Stdlib-only reduces the surface; it does not remove
  the need to watch it.
- A contributor who reaches for a library is not making a small call. This record,
  `docs/stack.md`, and the boundary line in `.claude/rules/project.md` are the three
  places that say so.
