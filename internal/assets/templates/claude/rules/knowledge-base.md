# The knowledge base — `docs/`

A spec answers *what this feature does now*. `docs/` answers *why* — the durable
reasoning, the decisions, the material read from outside. Neither replaces the other,
and keeping them separate is what lets a spec stay anchored to one feature.

```
docs/
  wiki/          index.md, changelog.md, and one page per concept
  raw/           sources dropped in to be processed — a file here is unfinished work
  adr/           numbered decision records
  codewiki/      narrated code, citing exact line ranges
  glossary.md    one canonical term per concept
  stack.md       adopted technology
```

`scc validate` checks all of it structurally: broken wikilinks, orphan pages,
index/changelog desync, unprocessed `raw/`, ADR numbering, citation resolution,
synonyms used where a canonical term belongs, and dependencies missing from
`stack.md`.

## wiki/

One page per concept, linked with `[[wikilinks]]`. Every page is reachable from
`index.md` — an unreachable page is an orphan, and an orphan is a page nobody will
ever find again. Record what changed in `changelog.md` when you change the wiki.

**`raw/` is a drop box, not storage.** Material collected from outside goes there to
be read, distilled into a wiki page, and then removed. A file still sitting in
`raw/` is a finding: it was collected and never processed.

## adr/

One record per decision that is **hard to reverse**. Numbered contiguously from
`0001`, one file per record, cited from anywhere as
`adr:0007-use-sqlite-for-the-cache`:

```markdown
---
status: accepted        # proposed | accepted | rejected | superseded
---

# 0007 · Use SQLite for the cache

## Context
## Decision
## Consequences
```

**A superseded record is marked, never edited.** The point of an ADR is that it
records what was believed at the time; rewriting one destroys exactly the thing it
exists to preserve. Add the new record, and in the old one set:

```yaml
status: superseded
superseded-by: 0012-move-the-cache-to-redis
```

Not every design decision is an ADR. If it is cheap to change, `design.md` is where
it belongs.

## codewiki/

Prose that explains code, one page per area, with every section citing the exact
lines it is about:

```markdown
## How the dispatcher routes

[internal/cli/cli.go:48-64]()

One switch, no registration...
```

A citation that no longer resolves is a finding, so this is the one part of `docs/`
that goes stale loudly rather than quietly. **Every section cites something** — a
section that cites nothing is prose that has drifted free of the code it describes.

Write it for the parts where reading the code does not tell you why it is like that.
Do not narrate what a reader can see.

## glossary.md

One canonical term per concept, and the synonyms to avoid. One entry per line:

```markdown
- **order total** — the amount charged, in minor units. Avoid: grand total, sum
- **workspace** — a directory holding .claude/scc-manifest.json. Avoid: project root
```

Domain vocabulary drifts by default — three names for one thing appear within a week
of two people working in parallel. Pick one, list the others after `Avoid:`, and use
the canonical term everywhere: in code, in requirements, in the wiki. An avoided
synonym used as a whole word in `docs/` is a finding.

## stack.md

Every adopted technology, with one line on why. **Technology not listed here is an
open decision, never something adopted silently** — and because dependency manifests
are structured data, this is checkable: a dependency in `go.mod` or `package.json`
that is absent from `stack.md` is a finding.

So adding a dependency is a two-step act: add it, and say here why it earned its
place.
