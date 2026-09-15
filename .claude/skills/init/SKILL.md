---
name: init
description: Bootstrap this project's knowledge base from the code that already exists — survey the repository, then write docs/stack.md, docs/glossary.md, the wiki, the codewiki, the ADRs for decisions already taken, and the project rule's real build, test, and lint commands, and the delivery gate's four. Use it on a workspace whose docs/ is still the four seeded anchors, when someone asks to document an existing codebase, when a repository has just been scaffolded and nothing under docs/ is filled in, or when someone runs /scc-init. Not for one page or one new decision — the wiki, glossary, stack, codewiki, and adr skills each own their own artifact, and this run is what calls them.
---

You fill an empty knowledge base from a repository that already exists.

The formats are not yours. Each artifact has an owner — the `stack`, `glossary`,
`wiki`, `codewiki`, and `adr` skills — and the rule they all enforce is
`.claude/rules/knowledge-base.md`. **Call them; do not restate them.** What this skill
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

`scc validate` fails outside one. If `.claude/scc-manifest.json` is absent the repository was
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
   `.claude/rules/code-search.md`. A survey is exactly the shape of question the graph
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
2. **`.claude/rules/project.md`** — the build, test, lint, and format commands. **Run each
   one before you write it.** Take them from CI first, since a workflow file is the one
   place somebody maintains them, then verify locally. A guessed command that exits `0`
   looks exactly like a passing suite, which is the whole reason that rule exists.

   **Then record the same four as the delivery gate — `scc check set <gate>
   "<command>"`.** The rule above is prose somebody reads; these are run, and their
   answers decide whether a branch becomes a pull request. There are four gates and
   they run in this order, stopping at the first failure:

   | Gate | What it has to do | Typical source |
   |---|---|---|
   | `build` | the project compiles | `go build ./...`, `tsc --noEmit`, `cargo build`, `mvn -q compile` |
   | `format` | the formatter has nothing to say — the *check*, not the rewrite | `gofmt -l .`, `prettier --check`, `black --check`, `cargo fmt --check` |
   | `lint` | the best-practices layer | `golangci-lint run`, `eslint .`, `ruff check`, `clippy -- -D warnings` |
   | `test` | the suite passes, and covers enough | see below |

   **A language that genuinely has no such step gets `scc check skip <gate>`, once.**
   That is a decision and it is recorded like one — the validator then stays quiet
   about it forever. Leaving a gate unrecorded is *not* the same thing: that is a
   decision nobody has made, and it is reported on every run until somebody makes it.
   Skip deliberately and rarely: most languages have a formatter, and "we have no
   linter" is usually "nobody has picked one yet", which is a finding worth keeping.

   **The format gate is the check, never the rewrite.** `gofmt -w .` exits `0` having
   silently changed the tree; the gate has to be the one that *fails* on unformatted
   code, or it certifies every run.

   The test gate asks for one thing more — a JSON object anywhere in its output:

   ```
   {"total": 412, "coverage": 88.4}
   ```

   Nothing in scc knows how your project produces those, which is why writing this is
   *your* job on this run. Build it from what the survey already told you:

   | Stack | Where the two numbers come from |
   |---|---|
   | Go | `go test -json -coverprofile=…` for the count, `go tool cover -func` for the total line |
   | Node / TS | `jest --coverage --json`, `vitest run --coverage --reporter=json` — both already emit totals |
   | Python | `pytest --cov --cov-report=json -q`, and the summary line for the count |
   | Rust | `cargo test` for the count, `cargo llvm-cov --summary-only` for coverage |
   | Anything else | whatever CI already runs, plus three lines of shell that print the object |

   **Put a pipeline in the repository, not in the manifest.** Record a target — `make
   test-report`, `npm run test:report`, `./scripts/test-report.sh` — and set *that* as
   the command. A pipeline recorded inline is one nobody can run by hand, fix, or review
   in a diff. A single command that already does the job goes in as it is.

   Three things to get right, because each has a failure mode that looks like success:
   a wrapped command's **exit status has to survive** the wrapper (a script ending in
   `echo` returns the echo's `0` and reports a broken build as green); the test report
   goes to **stdout** and the runner's own noise to stderr; and **`total` is a real
   count**, not a constant — zero tests with high coverage is a finding, and so is a
   number nobody updates.

   Then run `scc check` and read what it says. A gate that does not do what it claims
   is exactly as useless as no gate, and this is the one moment where fixing it costs
   nothing.
3. **`.devcontainer/Dockerfile`** — fit the container to this project. `scc init`
   seeds it with the agent's own toolchain (Claude Code, `scc`, `codegraph`, `rtk`)
   and a placeholder for yours; what it cannot know is the language, which you have
   just surveyed. Fill the marked block with what **`scc check` actually runs** —
   the compiler, the package manager, the linter and the formatter you recorded in
   step 2 — and nothing that only a human would want.

   | This project | Add |
   |---|---|
   | Go | `golang` at the version in `go.mod`, plus `golangci-lint` if the lint gate uses it |
   | Node / TS | the Node version in `.nvmrc` or `engines`, and the package manager the lockfile implies |
   | Python | `python` plus `uv`/`poetry`/`pip` — whichever the lockfile is for |
   | Rust | `rustup` with the toolchain in `rust-toolchain.toml` |

   **The bar is the gate, not the language.** A container that cannot run
   `scc check` is a container the delivery gate cannot be verified in, which is the
   whole reason this file exists. So the test is mechanical: build it, open a shell
   in it, and run `scc check`. If a gate fails for a missing tool, that tool belongs
   in the Dockerfile.

   This matters most on **Windows**, where it is not a convenience: ai-jail's
   sandbox has no Windows backend, so the container is the only isolation
   `scc launch` can give. On Linux and macOS the same file is useful and optional —
   the kernel sandbox is what runs there.

   Leave the credentials alone. The seeded `devcontainer.json` forwards a token and
   mounts `.gitconfig` read-only on purpose; **do not add a mount for `~/.ssh` or a
   cloud credential file**, which is the one change that would hand a compromised
   dependency the keys the container exists to keep away from it.
4. **`docs/glossary.md`** — the `glossary` skill, conservatively. Every synonym listed
   after `Avoid:` becomes a finding wherever it appears under `docs/`, so list one only
   where you actually saw two names used for one thing. Take terms from the domain this
   project is about, never from its framework.
5. **`docs/wiki/`** — the `wiki` skill. One page per concept a newcomer has to hold to
   read the code, never one per directory: a wiki that mirrors the file tree *is* the
   file tree, and it goes stale faster. Link every page from `index.md` and log the run
   in `changelog.md`.
6. **`docs/codewiki/`** — the `codewiki` skill, only where reading the code does not
   tell you why it is shaped that way. Every section cites the lines it explains, and a
   citation is a promise to keep the page current, so cite the part that is stable.
7. **`docs/adr/`** — the `adr` skill, last, and the one to be strictest about. A
   decision qualifies only when both hold: undoing it would be expensive, **and** there
   is evidence in the repository that it was taken. Number from `0001` in the order the
   history says they happened.

   **Say that the record is reconstructed.** An ADR is what was believed at the time,
   and these were not written at the time — so open `## Context` with one line naming
   what it was reconstructed from, and when. `status: accepted` where the code shows
   the decision in force; never `proposed` for something already built.

8. **`docs/notes.md`** — the `TODO`, `FIXME`, `HACK` and aside comments already in the
   code, moved into the log one line each with the file they sit on:
   `scc notes add "…" --tag <t> --path <file>`. Leave the comment where it is; this run
   does not touch code, and it goes when that file is next edited. What does not make
   the move is anything that is really work — that is a task in a plan, not a note.

## What this does not do

- **It does not write specs.** Documenting a system that already works as `specs/`
  means restating the whole product as requirements. `.claude/rules/specs.md` is explicit
  that a spec meets existing code as a **delta**, so the first spec here is written by
  the next change and not by this run.
- **It does not rewrite `CLAUDE.md`.** That file is the user's.
- **It does not touch code.** A defect you notice is worth reporting at the end; it is
  not this run's to fix, and a documentation branch that also changes behavior is one
  nobody can review.

## Finishing

`scc validate --checks` exits `0` — the artifacts in shape and every gate actually
doing what it claims — then deliver it as ordinary work: one branch, one pull
request, `.claude/rules/delivery.md`.

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
