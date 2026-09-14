# This project

**This file is yours.** `scc` ships it as a stub, records that it did, and never
touches it again — an upgrade will leave whatever you write here alone. It is most of
why `scc` needs no configuration file: the commands below are Markdown, which the
orchestrator already reads, and `scc` runs none of them.

**The delivery gate is the exception, because it is run rather than read.** Record
each of build, format, lint and test once with `scc check set <gate> "<command>"`;
`scc check` and the `pre-push` hook run them. A project with no such step says so
once with `scc check skip <gate>` — that is a decision, and it is not the same as
leaving one unrecorded. The test gate also has to print `{"total": N, "coverage": P}`
on stdout and keep the suite's exit status; `scc check help` has the rest. Writing
these is the agent's job, from this project's own toolchain.

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

# Test — the report the gate reads; then: scc check set test "<command>"
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
