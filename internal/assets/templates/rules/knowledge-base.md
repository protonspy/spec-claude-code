# The knowledge base — `docs/`

A spec answers *what this feature does now*. `docs/` answers *why* — the durable
reasoning, the decisions, the material read from outside. Keeping them separate is
what lets a spec stay anchored to one feature.

```
docs/
  wiki/          index.md and changelog.md, with pages/ — one page per concept
  raw/           sources dropped in to be processed — a file here is unfinished work
  adr/           numbered decision records
  codewiki/      narrated code, citing exact line ranges
  glossary.md    one canonical term per concept
  stack.md       adopted technology
  notes.md       one line per observation — [notes.md](notes.md)
```

`scc validate` checks all of it structurally: broken wikilinks, orphan pages,
index/changelog desync, unprocessed `raw/`, ADR numbering, citation resolution,
synonyms used where a canonical term belongs, dependencies missing from `stack.md`,
and a note no query would return.

## wiki/

One page per concept in `wiki/pages/`, linked with `[[wikilinks]]` — the slug is the
filename, never the path — and reachable from `index.md`, which stays a level up with
`changelog.md`. An orphan is a page nobody will find again. Log what you change.

**`raw/` is a drop box, not storage.** Outside material goes there to be read,
distilled into a wiki page, and removed. A file still sitting there is a finding: it
was collected and never processed.

## adr/

One record per decision that is **hard to reverse**, numbered contiguously from
`0001`, cited from anywhere as `adr:0007-use-sqlite-for-the-cache`. Frontmatter
`status:` is `proposed | accepted | rejected | superseded`; the body is `## Context`,
`## Decision`, `## Consequences`.

**A superseded record is marked, never edited** — an ADR records what was believed at
the time, and rewriting one destroys the thing it exists to preserve. Add the new
record; in the old one set `status: superseded` and `superseded-by: 0012-…`.

Not every design decision is an ADR. If it is cheap to change, it belongs in
`design.md`.

## codewiki/

Prose explaining code, one page per area, every section citing the exact lines it is
about:

```markdown
## How the dispatcher routes

[internal/cli/cli.go:48-64]()
```

A citation that no longer resolves is a finding, so this is the one part of `docs/`
that goes stale loudly rather than quietly. **Every section cites something** — one
that cites nothing has drifted free of the code it describes. Write it where reading
the code does not tell you why it is like that; do not narrate what a reader can see.

## glossary.md

One canonical term per concept and the synonyms to avoid, one entry per line:

```markdown
- **order total** — the amount charged, in minor units. Avoid: grand total, sum
- **workspace** — a directory holding {{.Manifest}}. Avoid: project root
```

Domain vocabulary drifts by default — three names for one thing appear within a week
of two people working in parallel. Use the canonical term in code, in requirements,
in the wiki. An avoided synonym used as a whole word in `docs/` is a finding.

## stack.md

Every adopted technology, with one line on why. **Technology not listed here is an
open decision, never something adopted silently** — and because a dependency file is
structured data, this is checkable: a direct dependency declared there and absent here
is a finding. Adding a dependency is two acts: add it, and say why.
