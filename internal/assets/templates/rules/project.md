# This project

**This file is yours.** `scc` ships it as a stub, records that it did, and never
touches it again — an upgrade will leave whatever you write here alone. It is also
the reason `scc` needs no configuration file: the commands below are Markdown, which
the orchestrator already reads, and `scc` itself runs neither of them.

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
