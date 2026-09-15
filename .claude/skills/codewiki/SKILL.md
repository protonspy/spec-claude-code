---
name: codewiki
description: Narrate an area of this codebase into docs/codewiki/, where every section cites the exact lines it explains. Use it when a subsystem is hard to enter cold and the code does not say why it is shaped that way, when someone asks for onboarding notes or an architecture walkthrough, and when `scc validate` reports codewiki.* findings — a citation that no longer resolves, one that runs past the end of its file, or a section that cites nothing at all.
---

You write prose that explains code, and every section of it points at the lines it
is about. The citation is what makes this different from a wiki page: it goes stale
loudly. When the code moves, `scc validate` fails instead of the page quietly
becoming a lie.

The format is in `.claude/rules/knowledge-base.md`. This skill is
the procedure and the judgment.

## The citation form

A link whose text is `path:start-end` and whose target is empty:

```markdown
## How the dispatcher routes

[internal/cli/cli.go:48-64]()

One switch, no registration. Adding a subcommand means...
```

- The path is relative to the workspace root, always — never to the page.
- A single line is `path:42`. A range is `path:42-58`.
- The target stays empty. `scc validate` resolves the path itself, confirms the file
  exists, and confirms the range is inside it.

## Writing a page

1. **Read the area first — all of it.** You are claiming to explain this code. Every
   sentence you write is checkable against the lines you cited, and a reader who
   catches one wrong claim stops believing the page.
2. **Pick the sections from the reader's questions**, not from the file layout. "How
   the dispatcher routes" is a section. "cli.go" is a table of contents.
3. **Cite before you narrate.** Put the citation directly under the heading, then
   explain. It anchors the section and it is how you notice you are about to write a
   paragraph that is not about any particular code.
4. **Every `##` section cites something.** This is checked. A section with no
   citation is prose that has drifted free of what it describes — either find the
   lines it is about, or the section belongs in `docs/wiki/` instead.
5. **Headings must be unique on the page.** Two identical headings share one anchor,
   and a link to it lands on whichever came first.
6. `scc validate` before you call it done.

## What to narrate, and what to leave alone

Write about the parts where **reading the code does not tell you why it is like
that**:

- A constraint the shape exists to satisfy, which is invisible from any one file.
- An invariant several files cooperate to hold.
- A deliberate choice that looks wrong until you know what it prevents.
- The entry point — where execution actually begins, which is often the single
  hardest thing to find in an unfamiliar codebase.

Do not narrate what a reader can see. "This function takes a name and returns a
path" is the signature restated at greater length, and a page full of that trains
people to skip the page.

Prefer the range that makes the point. Citing a whole 400-line file says "the answer
is in here somewhere", which the reader already knew.

## Repairing drift

| Finding | What it means | The fix |
|---|---|---|
| `codewiki.citation-unresolved` | The file is gone or was renamed. | Find where the code went. If it was deleted, delete the section — a section about code that no longer exists is not repairable. |
| `codewiki.citation-out-of-range` | The file is shorter than the citation claims. | The code moved. **Re-read it before re-numbering**: the lines may now be something else entirely, and shifting the range to make the finding go away without reading is how a page becomes confidently wrong. |
| `codewiki.citation-invalid` | Malformed, or the path escapes the workspace. | A citation names a path inside the checkout. Fix the path. |
| `codewiki.section-cites-nothing` | A section explains no specific code. | Cite the lines, or move the section to `docs/wiki/`. |
| `codewiki.duplicate-heading` | Two sections share one anchor. | Rename one. |

**A finding here is usually a real problem, not a bookkeeping chore.** The citation
broke because the code changed, which means the prose describing it is now suspect
in a way no validator can measure. Re-read, then rewrite what is no longer true.
