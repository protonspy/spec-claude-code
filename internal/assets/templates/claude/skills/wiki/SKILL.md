---
name: wiki
description: Build and maintain docs/wiki/ — one page per concept, linked with wikilinks and reachable from index.md. Use it when a source lands in docs/raw/ and has to be distilled into a page, when a question is better answered from accumulated knowledge than from re-reading the code, and when `scc validate` reports wiki.* findings — a broken wikilink, an orphan page, a changelog naming a page that no longer exists, or a source still sitting unprocessed in docs/raw/.
---

You own `docs/wiki/`: the durable half of what this project knows. A spec says what
one feature does now; the wiki says what is true across features and outlasts them.

The format is not yours to invent — it is in
[knowledge-base.md](../../rules/knowledge-base.md), and `scc validate` enforces it.
This skill is the procedure.

## Ingest — something landed in `docs/raw/`

`raw/` is a drop box, not storage. A file still there is unfinished work, and
`scc validate` says so by name. Empty it:

1. **Read the source completely** before writing anything. A page distilled from a
   skim is worse than no page — it looks authoritative and is not.
2. **Decide the concept, not the document.** One page per concept. A source that
   covers three concepts becomes three pages, or one page and two edits to pages
   that already exist. Do not mirror the source's own structure.
3. **Check `docs/glossary.md` first.** If the source names something the project
   already has a canonical term for, use the canonical term. If it coins a term
   worth keeping, that is the `glossary` skill's job — do it, then come back.
4. **Write the page** at `docs/wiki/<slug>.md`. The filename is the slug, so
   `order-total.md` is what `[[order-total]]` resolves to.
5. **Link it in.** Add a `[[wikilink]]` from `index.md`, or from a page already
   reachable from it. A page nobody links is a page nobody finds again.
6. **Record it in `changelog.md`** — what changed, naming the pages with wikilinks.
7. **Delete the source from `docs/raw/`.** This is the step that gets skipped. The
   finding exists because of it.
8. `scc validate` before you call it done.

A source that turns out not to be worth a page still gets deleted. "Read and
rejected" is a complete outcome; leaving the file behind pretends the work is
pending.

## Query — answer from what is already known

Read `index.md` first, follow wikilinks from there, and answer from the pages. Cite
the page you answered from so the reader can check you.

If the wiki does not cover it, say so plainly and go read the code. Then consider
whether what you just had to reconstruct should have been a page — the second time
someone reconstructs the same thing is the signal, not the first.

## Maintain — clear what `scc validate` reports

| Finding | What it means | The fix |
|---|---|---|
| `wiki.missing-index` | Pages exist with no entry point, so every page is an orphan. | Write `index.md` and link the pages. |
| `wiki.broken-link` | A `[[wikilink]]` resolves to no page. | Fix the slug, or write the page it was reaching for. |
| `wiki.orphan-page` | The page exists and nothing reaches it. | Link it from `index.md` or from a reachable page. Or delete it — an unreachable page nobody has missed is a candidate. |
| `wiki.missing-changelog` | The wiki has pages and no log of how it got that way. | Write `changelog.md`. |
| `wiki.changelog-desync` | The log names a page that no longer exists. | The page was renamed or removed; the log is a record, so correct the entry rather than rewriting history around it. |
| `wiki.unprocessed-source` | A file is still in `docs/raw/`. | Ingest it, above. |

**`index.md` is a map, not a dump.** When it grows into a flat list of every page,
group it — the entry point earns its keep by making the shape of the knowledge
visible, and a hundred unsorted links do the opposite.

## What does not belong here

- **A decision that is hard to reverse.** That is an ADR — use the `adr` skill. The
  wiki can link to it; it does not restate it.
- **An explanation of specific code.** That is `docs/codewiki/`, where every claim
  cites the lines it is about — use the `codewiki` skill.
- **What one feature does now.** That is `specs/<feature>/requirements.md`.
- **Anything you have not verified.** The wiki's whole value is that it can be
  trusted without checking. One confidently wrong page costs more than the ten
  correct ones beside it earn.
