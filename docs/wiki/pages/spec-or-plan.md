# Spec or plan

Every piece of work picks one vehicle, and the choice is recorded in a file because
a session's context dies and the artifact is the only channel through which the
decision survives.

One question decides it:

> Does this need requirements and a design settled before any code?

**Yes → a spec.** No → **a plan.**

| Vehicle | Path | Contents |
|---|---|---|
| Spec | `specs/<feature>/` | `requirements.md` · `design.md` · `tasks.md` |
| Plan | `plans/<name>.md` | a header, plus a checklist and/or references to specs |

A plan covers both ends of *not a spec*: an initiative too large for one spec,
decomposed into the specs it references; or a change small enough that the *what*
was never in doubt, which is a checklist and nothing more.

> If you know GitHub Spec Kit, note that "plan" there means the opposite of this —
> the architecture *inside* one feature, which here is `design.md`.

## One source of truth per item

Where a plan item *is* a task, its checkbox is the state. Where it *references a
spec*, the state is derived from that spec and never copied — and an item must not
do both. Two records of one fact disagree, and the copy is the one that goes stale.

The same principle closes the task flag vocabulary: there is no `_Blocked_` flag,
because blocked is derived from `_Depends_`, and `_Status_` never takes `open` or
`completed`, because the checkbox already says that.

## The plan is a contract, not a document

A plan has six sections and no others — `Why`, `Paths`, `References`, `Out of
scope`, `Tasks`, `Done when` — and any other heading is a finding. The closed set is
the only thing that ever capped a plan's size: the 56KB plan that motivated this got
there through a `## Notes` section that nothing forbade, growing to half the file.

There is no line limit anywhere in the contract except on the description, because a
limit that fires on a legitimate plan is worse than the growth it prevents. What
there is instead is **nowhere for prose to go**. `docs/notes.md` is the same move
applied to code comments.

## Requirements are omitted, not filled

Structure improves generated code, but the curve is not monotonic: over-specification
introduces requirements that conflict, and correctness drops. The same holds for
`design.md`, where invented architecture actively constrains — the next session reads
it as a decision somebody made, and honors it. So a heading with nothing real to say
is deleted rather than filled, and `scc` requires no section in a design at all.

## A spec meets existing code as a delta

Changing an existing spec means amending individual requirements — `(ADDED)`,
`(MODIFIED)`, `(REMOVED)` — not rewriting the file. That is what makes adopting the
methodology on a working system possible without restating the whole product as
requirements, and it is why `/scc-init` writes no specs. It also makes concurrent
edits safe: two sessions can change one spec while touching different requirements.

## Related

[[artifact-addressing]] · [[context-budget]] · [[validation-contract]]
