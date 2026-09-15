---
name: glossary
description: Own docs/glossary.md — one canonical term per concept, and the synonyms nobody should use for it. Use it when a domain term is coined, contested, fuzzy, or renamed; before naming a spec, a plan, a wiki page, or an identifier that will outlive the session; and when `scc validate` reports glossary.* findings — a duplicate entry, a term listed as both canonical and avoided, or an avoided synonym used somewhere in docs/.
---

You own the project's vocabulary. Domain language drifts by default: two people
working in parallel produce three names for one thing inside a week, and every one
of them ends up in code, requirements, and the wiki, where nobody can tell whether
two documents are describing the same concept or two different ones.

The entry format is in `.claude/rules/knowledge-base.md`. This
skill is when to reach for it and how to decide.

## The entry

One entry per line, in `docs/glossary.md`:

```markdown
- **order total** — the amount charged, in minor units. Avoid: grand total, sum
- **workspace** — a directory holding .claude/scc-manifest.json. Avoid: project root
```

- The term is bold. The definition follows an em dash.
- `Avoid:` lists the synonyms, comma-separated. It is optional — a term with no
  contested spellings does not need one.
- Every listed synonym becomes a finding wherever it appears as a whole word in
  `docs/`. That is the enforcement, so list a synonym only when you mean it.

## Adding a term

1. **Check it is not already there** under another name. Read the file before adding
   to it — a glossary with two entries for one concept is worse than none, because
   now the canonical source is itself ambiguous.
2. **Define what it is, not what it is called.** "The amount charged, in minor
   units" settles arguments. "The total of the order" settles nothing.
3. **List the synonyms you are banning**, and only those. Do not ban a word the
   project uses correctly in an unrelated sense — the check is whole-word across all
   of `docs/`, so banning a common word turns the glossary into noise the first time
   it fires on an innocent sentence.
4. **Then use it.** Rename the occurrences in `docs/` that the validator will now
   flag, and prefer the canonical term in new requirements, wiki pages, and
   identifiers.
5. `scc validate` before you call it done.

## When to reach for this

- **A term is coined.** Something in the domain just got a name for the first time.
- **A term is contested.** Two documents or two people use different words and you
  cannot tell whether they mean the same thing. They probably do; that is the point.
- **A term is renamed.** Move the old name into the new entry's `Avoid:` list — this
  is the case the enforcement is most useful for, because it finds every place the
  rename was not applied.
- **Before naming anything durable** — a spec directory, a plan, a wiki slug, a
  type. The name will be read far more often than it was chosen.

## Findings

| Finding | What it means | The fix |
|---|---|---|
| `glossary.duplicate-term` | Two entries define the same term. | Merge them. One canonical term means one entry. |
| `glossary.synonym-is-canonical` | A term is defined as canonical and also listed as a synonym to avoid. | The glossary is contradicting itself. Decide which one the project uses. |
| `glossary.avoided-synonym` | A banned synonym is used in `docs/`. | Rewrite it to the canonical term — that is the entire point of listing it. If the sentence is genuinely about something else, the ban was too broad: narrow the `Avoid:` list. |

**Do not define what is obvious to anyone in the domain.** A glossary that opens
with "user — a person who uses the system" is one nobody reads to the entry that
mattered. Define what is specific to this project, ambiguous in general use, or
already being said three ways.
