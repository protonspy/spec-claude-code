---
description: Add, settle, or rename a canonical term in docs/glossary.md, and list the synonyms to avoid
argument-hint: [term, or the ambiguity to settle]
---

Use the `glossary` skill.

Term: $ARGUMENTS

Read `docs/glossary.md` before adding to it — an entry that duplicates an existing
concept under a different name makes the canonical source itself ambiguous. If
nothing was named, run `scc validate` and resolve the `glossary.*` findings.
