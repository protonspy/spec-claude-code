---
name: adr
description: Record a decision that is hard to reverse as a numbered ADR under docs/adr/, and supersede an old record rather than editing it. Use it when a choice is made that would be expensive to undo — storage, wire protocol, a module boundary, a vendor holding data, a framework the code will shape itself around — when someone asks why the project is built this way, and when `scc validate` reports adr.* findings such as a numbering gap, a superseded record with no successor, or a citation resolving to no record.
---

You write the record of a decision, so that the reasoning survives the people who
were in the room. The filename shape, the frontmatter, and the citation form are in
`.claude/rules/knowledge-base.md`. This skill is when to write one
and how to write one worth reading.

## Is this an ADR?

One question: **how expensive would it be to undo?**

Write one when the answer is "expensive" — a datastore, a wire format, an
authentication model, a boundary between services, a vendor that will end up holding
data, a framework the rest of the code will be shaped around. Also write one when a
previous decision is being **reversed**, which is the case people skip and the case a
reader most needs.

Do not write one for a decision that is cheap to change. That belongs in the spec's
`design.md`, where it is read by the people building the feature and forgotten
harmlessly once it stops being true. An `adr/` directory full of reversible choices
buries the four records that actually explain the system.

Rough test: if changing your mind next month means editing one package, it is not an
ADR. If it means a migration, it is.

## Writing one

```markdown
---
status: accepted        # proposed | accepted | rejected | superseded
---

# 0007 · Use SQLite for the cache

## Context
## Decision
## Consequences
```

Numbered contiguously from `0001`; the filename is `NNNN-kebab-slug.md`, and the
number is never reused — gaps are findings because a gap means a record went
missing. Take the next number after the highest one present.

**Context** is the section that does the work. Write what was true when the decision
was made — the constraint, the load, the deadline, the thing that was already built.
A reader a year from now is trying to learn whether the reasons still hold, and they
can only do that if the reasons are written down. "We chose SQLite because it is
simple" tells them nothing; "single writer, under 10GB, and operating a second
service was the cost we were avoiding" tells them exactly when to revisit.

**Decision** is one paragraph in the active voice. What was chosen, and what was
chosen against — name the alternatives that were real, and say what ruled each out.

**Consequences** includes the ones you do not like. A record listing only benefits
is marketing, and the next reader will discover the downside on their own and trust
nothing else in the file.

## Superseding

**A superseded record is marked, never edited.** The point of an ADR is that it
records what was believed at the time; rewriting it destroys the thing it exists to
preserve — and the new record loses the context that explains why the change was
needed.

Write the new record, then in the old one set only the frontmatter:

```yaml
status: superseded
superseded-by: 0012-move-the-cache-to-redis
```

Leave the prose exactly as it was. The new record's Context says what changed.

## Findings

| Finding | What it means | The fix |
|---|---|---|
| `adr.malformed-filename` | Not `NNNN-kebab-slug.md`. | Rename it. The number is how it is cited. |
| `adr.duplicate-number` | Two records share a number. | Renumber the later one; a citation cannot resolve to two records. |
| `adr.numbering-gap` | A number is missing from the sequence. | A record was deleted or never committed. Restore it, or renumber — do not leave the hole, because a gap reads as a lost decision. |
| `adr.missing-status` / `adr.status-invalid` | The frontmatter `status` is absent or not one of the four. | Set it. A record with no status does not say whether it is in force. |
| `adr.superseded-without-successor` | Marked superseded with no `superseded-by`. | Name the record that replaced it, or the reader is told this is obsolete and given nowhere to go. |
| `adr.unknown-successor` / `adr.unknown-citation` | A reference resolves to no record. | Fix the slug. |

Cite a record from anywhere in `docs/` as `adr:0007-use-sqlite-for-the-cache`.
