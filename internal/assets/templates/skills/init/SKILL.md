---
name: init
description: Bootstrap this project's knowledge base from the code that already exists — survey the repository, then write docs/stack.md, docs/glossary.md, the wiki, the codewiki, the ADRs for decisions already taken, and the project rule's real build, test, and lint commands. Use it on a workspace whose docs/ is still the four seeded anchors, when someone asks to document an existing codebase, when a repository has just been scaffolded and nothing under docs/ is filled in, or when someone runs /scc-init. Not for one page or one new decision — the wiki, glossary, stack, codewiki, and adr skills each own their own artifact, and this run is what calls them.
---

You fill an empty knowledge base from a repository that already exists.

The formats are not yours. Each artifact has an owner — the `stack`, `glossary`,
`wiki`, `codewiki`, and `adr` skills — and the rule they all enforce is
`{{.Rules}}/knowledge-base.md`. **Call them; do not restate them.** What this skill
owns is what no rule can: the *order* across artifacts, the survey that precedes all
of them, and the bar for what may be written at all.

## The bar — nothing without evidence

Everything here is **reconstructed, not remembered.** Nobody is telling you why this
system is the way it is; you are inferring it from what survived. So write only what
you can point at — a file, a dependency manifest, a CI workflow, a commit, a comment,
a migration.

Where the code cannot tell you why, **say the reasoning is unrecorded** and move on. A
plausible reason invented here is worse than a gap, because a gap is visible and an
invention is believed. The knowledge base's whole value is that it can be trusted
without checking, and this run is where that trust is either earned or spent.

## Before anything — is there a workspace

`scc validate` fails outside one. If `{{.Manifest}}` is absent the repository was
never scaffolded, and which harness it is scaffolded for is **the user's call, not
yours** — `scc init --claude`, `--codex`, or `--opencode`. Ask, then carry on.

## Ask twice, then start

One exchange, before the survey, because both answers change how much you read:

| Ask | Answers |
|---|---|
| How deep — the anchors, or everything? | **anchors** (`stack.md`, `glossary.md`, the project rule, and a wiki a newcomer can enter) · **full** (also `codewiki/` and the ADRs for decisions already taken) |
| The whole repository, or one subtree? | the root · one package, service, or app inside a monorepo |

Anchors is the right default for a first run. State what you took, so a wrong reading
costs a sentence rather than a session.

## The survey — read the map, not the territory

Cheapest sources first, and stop when another pass stops changing the map:

1. **What was written for a newcomer** — README, CI workflows, `Makefile` or the
   scripts block, the dependency manifests, the top-level directories.
2. **The structure** — `scc graph explore "<question>"`, per
   `{{.Rules}}/code-search.md`. A survey is exactly the shape of question the graph
   answers, and exactly the one that ruins a context window when answered by reading
   files.
3. **The history, for what was argued about** — `git log` over a directory that moved,
   a revert, a migration, a dependency swapped out. This is the cheapest evidence that
   a decision was expensive, which is what an ADR needs and what code alone never says.

**Report the map back before writing anything** — the areas, the concepts you would
give pages, the decisions you would record. That is the last moment a correction is
cheap.

## Then write, in this order

Cheapest and most checkable first, most interpretive last, with `scc validate` between
stages so no stage inherits the previous one's findings.

1. **`docs/stack.md`** — the `stack` skill. It has a finish line nothing else here
   has: `stack.undocumented-dependency` reaching zero, because the validator reads the
   direct dependencies straight out of the manifest. Where nobody can justify one, say
   what it is *used for* — the import sites are a fact you can point at — and mark the
   decision unrecorded. Do not invent a rationale, and do not delete a dependency to
   silence a finding.
2. **`{{.Rules}}/project.md`** — the build, test, lint, and format commands. **Run each
   one before you write it.** Take them from CI first, since a workflow file is the one
   place somebody maintains them, then verify locally. A guessed command that exits `0`
   looks exactly like a passing suite, which is the whole reason that rule exists.
3. **`docs/glossary.md`** — the `glossary` skill, conservatively. Every synonym listed
   after `Avoid:` becomes a finding wherever it appears under `docs/`, so list one only
   where you actually saw two names used for one thing. Take terms from the domain this
   project is about, never from its framework.
4. **`docs/wiki/`** — the `wiki` skill. One page per concept a newcomer has to hold to
   read the code, never one per directory: a wiki that mirrors the file tree *is* the
   file tree, and it goes stale faster. Link every page from `index.md` and log the run
   in `changelog.md`.
5. **`docs/codewiki/`** — the `codewiki` skill, only where reading the code does not
   tell you why it is shaped that way. Every section cites the lines it explains, and a
   citation is a promise to keep the page current, so cite the part that is stable.
6. **`docs/adr/`** — the `adr` skill, last, and the one to be strictest about. A
   decision qualifies only when both hold: undoing it would be expensive, **and** there
   is evidence in the repository that it was taken. Number from `0001` in the order the
   history says they happened.

   **Say that the record is reconstructed.** An ADR is what was believed at the time,
   and these were not written at the time — so open `## Context` with one line naming
   what it was reconstructed from, and when. `status: accepted` where the code shows
   the decision in force; never `proposed` for something already built.

## What this does not do

- **It does not write specs.** Documenting a system that already works as `specs/`
  means restating the whole product as requirements. `{{.Rules}}/specs.md` is explicit
  that a spec meets existing code as a **delta**, so the first spec here is written by
  the next change and not by this run.
- **It does not rewrite `{{.Entry}}`.** That file is the user's.
- **It does not touch code.** A defect you notice is worth reporting at the end; it is
  not this run's to fix, and a documentation branch that also changes behavior is one
  nobody can review.

## Finishing

`scc validate` exits `0`, then deliver it as ordinary work — one branch, one pull
request, `{{.Rules}}/delivery.md`.

Then report **what was left unknown**, by name: the dependency nobody could justify,
the area whose reasoning is unrecorded, the decision that looked expensive and had no
evidence behind it. That list is the most valuable thing this run produces, and it is
the part that disappears if you do not write it down.

## Re-running

It is additive and resumable, and it takes its position from the repository rather
than from memory. Read what `docs/` already holds, **never rewrite a page somebody
else wrote**, clear `scc validate` findings first and fill gaps second. A second run
over a documented workspace should report that there is nothing to add — not produce a
second account of the same system.

## Degrading

No graph, no CI file, no history worth reading — none of that stops the run. Say which
one is missing, fall back to what is there, and lower the claim rather than the
honesty: five pages somebody can check beat a whole knowledge base assembled out of
inference.
