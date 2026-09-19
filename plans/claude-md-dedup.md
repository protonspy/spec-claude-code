---
autonomy: auto
ci: wait
---

# CLAUDE.md back to an entry file

`CLAUDE.md` is 96.8KB and preloaded into every request. Almost all of it is a third
copy of what `.claude/rules/` and `docs/` already own, and one part of it contradicts
`rules/project.md` outright. This moves each concern to the one place that owns it and
leaves the entry file as the template `scc` itself ships.

## Why

The binary's own entry template says the methodology lives in `.claude/rules/` and is
never inlined, and `TestScaffoldedEntryFileStaysShort` holds every workspace `scc`
scaffolds to 60 lines. This workspace breaks the contract its own product enforces:
96.8KB standing cost, `## Commands` telling a session to run `make check` while
`rules/project.md` says `make` is absent on Windows, and `## CodeGraph` restating a
rule that is preloaded three files away. Done means `CLAUDE.md` is the rendered entry
template plus the two spliced usage blocks, nothing load-bearing was lost on the way
out, and `scc validate` is clean.

## Paths

- `CLAUDE.md`
- `docs/wiki/pages/` · `docs/wiki/index.md` · `docs/wiki/changelog.md`
- `docs/codewiki/` · `docs/adr/`

## Out of scope

- `.claude/rules/*` — the rule set is the product's, and it already owns what it owns.
- `design/` — the historical design documents stay where they are.
- `rules/project.md` — it is at its 55-line cap and already carries the real commands.

## Tasks

- [x] 1.1 (Unit) Fold the gate's detail into `docs/wiki/pages/delivery-gate.md` — `min_coverage` and its 86% default, `skipped` as a word rather than an absent key, the four-gate order and why it stops at the first failure, and `total` guarding a suite that does not exist
- [x] 1.2 (Unit) Fold `ScopedRules` into `docs/wiki/pages/scoped-rules.md` — the measured 47KB, which three rules carry a `paths:` header, and the refusals (`artifacts.md`, `prior-art.md`, `caveman.md`)
- [x] 1.3 (Unit) Fold the sandbox and the Windows backend into `docs/wiki/pages/integration-boundary.md` — `jailToolchain` and why `Compose` orders directories first, the container as the weaker boundary, and the RTK-versus-CodeGraph marker decision
- [x] 1.4 (Unit) Fold `CarryOver` and the manifest's `check` object into `docs/wiki/pages/managed-files.md` — the manifest as the first input rather than only a record
- [x] 1.5 (Unit) Fold the reading surface into `docs/wiki/pages/artifact-addressing.md` — the closed plan shape, `--next` printing whole while listings clip, and BM25 over regions instead of a search engine
- [x] 2.1 (Unit) Write `docs/wiki/pages/release.md` — the immutable version, idempotent publishing, the three files a platform touches, and the two launcher tiers
  _Depends 1.1_
- [x] 2.2 (Unit) Write `docs/codewiki/packages.md` — the package map from `## Architecture`, every section citing the lines it explains
  _Depends 2.1_
- [x] 2.3 (Unit) Write the ADRs for the decisions that are hard to reverse and cite no page — stdlib-only, one manifest and no config file, the exit-code contract, sharing RTK's markers, the sandbox on by default, and what was narrowed out of `csdd`
  _Depends 2.2_
- [x] 3.1 (Unit) Replace `CLAUDE.md` with the rendered entry template plus the RTK and CodeGraph blocks
  _Depends 2.3_
- [x] 3.2 (Unit) Relink `docs/wiki/index.md` and log `docs/wiki/changelog.md`
  _Depends 3.1_

## Done when

- `CLAUDE.md` is under 6KB and carries no heading the entry template does not.
- `scc validate` exits 0 — no orphan page, no broken wikilink, no unresolved codewiki citation, no ADR numbering gap.
- `scc check` is green.
- Every topic listed in the tasks above resolves to exactly one file under `docs/`.
