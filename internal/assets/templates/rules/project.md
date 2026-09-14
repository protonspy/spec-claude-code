# This project

**This file is yours.** `scc` ships it as a stub, records that it did, and never
touches it again — an upgrade will leave whatever you write here alone. It is most of
why `scc` needs no configuration file: the commands below are Markdown, which the
orchestrator already reads, and `scc` runs none of them.

**The test report is the exception, because it is checked rather than read.** Record
it once with `scc test set "<command>"`; `scc test` and the `pre-push` hook run it and
hold its coverage against a floor. It has to print `{"total": N, "coverage": P}` on
stdout and keep the suite's exit status — `scc test help` has the rest. Writing that
command is the agent's job, from this project's own runner and coverage tool.

Fill it in. An empty answer here means every session re-derives your build commands
by guessing.

## Commands

```bash
# Build
<command>

# Test — the whole suite
<command>

# Test — one package or one file (used after every task; scope, not suite)
<command>

# Test — the report the gate reads; then: scc test set "<command>"
<command>

# Lint — the best-practices layer that finds what tests do not
<command>

# Format / format check
<command>
```

## Conventions

- **Branch names:** e.g. `feat/<slug>`, `fix/<slug>`
- **Commits:** e.g. Conventional Commits, scoped by package
- **Anything a new contributor gets wrong on their first try:** …

## Boundaries

Things that are *not* to be changed without asking, and why. Generated files,
vendored trees, public API surfaces, migration files that have already run.
