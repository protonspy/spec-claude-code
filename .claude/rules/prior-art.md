# Prior art — read the record before you write one

Before the first artifact of a piece of work exists — before `scc spec new`, before
`scc plan new`, before a line of code — find out what this workspace already settled.
`docs/` is not reference material for when you get stuck: it is the constraint set. An
ADR binds the design you are about to write, `stack.md` says what you may build on,
`glossary.md` says what to call it.

The failure it prevents is silent. A spec written without this pass re-decides a
decision somebody already made, names a concept a second way, or adds a dependency for
something the stack already carries — and none of it reads as wrong on review, because
it reads as new work rather than as a contradiction.

## The passes, before writing

| Ask | Where | What binds you |
|---|---|---|
| Has this been decided? | `docs/adr/` — the filenames are the index | a record that constrains this design; `rejected` and `superseded` count |
| Has this been built? | `scc map` · `scc map trace specs/<feature>/` | a spec covering the area — then this is a delta, not a new spec |
| What is it called? | `docs/glossary.md` | the canonical term, and the synonyms that are findings |
| What may I build on? | `docs/stack.md` | what is adopted; anything absent is an open decision |
| How does it work today? | `docs/wiki/index.md` · `docs/codewiki/` | the concept pages, and the code already narrated |
| What bit somebody here already? | `scc notes find --path <path>` · `--tag` | a gotcha already paid for once ([notes.md](notes.md)) |

**`docs/` is in no index but one.** `scc map` covers `plans/` and `specs/`, the symbol
graph covers code ([code-search.md](code-search.md)), and `scc notes find` covers the
note log. The rest is deliberate reading, cheap only because the anchors are built for
it: `glossary.md` and `stack.md` are lists, `wiki/index.md` and the ADR filenames are
tables of contents. Open a page when its title bears on this work — never to survey
the base.

## Say what you found, then cite it

Report it in a line or two before you write anything: the ADRs that bind, the spec that
already covers the area, the terms you will use. Under `gated` that is the material of
the first checkpoint; under `auto` it is the only trace that the pass happened at all.
"Nothing here governs this" is a result, and worth saying.

Then carry it into the artifact, where it outlives the session:

- an ADR that governs is cited from `design.md` as `adr:0007-use-sqlite-for-the-cache`
- a spec covering the area is amended as a delta, never re-specified ([specs.md](specs.md))
- a wiki page explaining the ground is `[[linked]]`, not summarized a second time
- the glossary's term is the one the requirements use — a synonym is a finding

## What the pass turns up is usually work

A decision you had to reconstruct from the code was never written down; a concept you
found under three names is a glossary entry; a page describing what the system no
longer does is stale. Writing that is [knowledge-base.md](knowledge-base.md), and it
belongs in this delivery rather than a later one.

Do not invent a record to fill a gap. An empty `docs/` is a young workspace, not a
finding: write the ADR when the decision you are making is hard to reverse, and the
page when you learned something the next session would otherwise learn again.
