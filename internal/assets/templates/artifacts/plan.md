---
autonomy: {{.Autonomy}}
ci: {{.CI}}
---

# {{.Title}}

<one to three sentences: what this work is>

<!-- A plan is a short header and a checklist, and the sections below are all the
     sections there are. That is the only thing holding a plan's size down: every
     session that runs this file carries it, so there is deliberately nowhere for
     prose to grow. `scc validate` reports any other heading.

     What was decided and why is an ADR under docs/adr/, cited from the item it
     governs. What changed and what shipped is git. A constraint on one item goes on
     that item's own line, where whoever works it will see it.

     Delete the sections you do not need — Paths, References and Out of scope are
     optional — and delete this comment. -->

## Why

<!-- One paragraph. Why this exists, and what "done" means for the whole of it. -->

## Paths

<!-- The files and directories this work touches. Optional, and a hint rather than a
     contract: it is what stops the first session hunting for where the code lives. -->

- `path/to/the/thing`

## References

<!-- The specs this decomposes into, plus the ADRs and documents that decide it. A
     reference carries no checkbox — a spec's state lives in that spec and is read
     from there, and two records of one fact disagree. -->

- `specs/<feature>/` — <what it covers>

## Out of scope

<!-- What someone could reasonably think this covers and it does not. -->

- <the thing this is not>

## Tasks

<!-- Same grammar as a spec's tasks.md, minus the requirement citations: a plan has
     no requirements to cite. The number is `<group>.<item>`, it is never reused, and
     `(Unit)`/`(TDD)` is how this gets built.

     Under a task, at most one of each, and there are no others:

       _Depends 1.1, 1.2_   every one of them has to be ticked before this can start
       _Priority 2_         a whole number, 1 or greater; lower is more urgent
       _Status removed_     struck out by discovery; the line stays, the number stays
       _Reason …_           required with _Status removed_

     There is no `_Blocked_` — that is derived from _Depends_ — and nothing restates
     the box. Order is `scc map tasks <plan> --next`, not the order they are written. -->

- [ ] 1.1 (Unit) <description, in the imperative>
- [ ] 1.2 (TDD) <description>
  _Depends 1.1_

## Done when

<!-- Verifiable criteria for the whole plan, not per task. If a line cannot be
     checked by running something or reading something, it is not one of these. -->

- <the check that says this is finished>
