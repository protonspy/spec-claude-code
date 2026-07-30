---
autonomy: {{.Autonomy}}
ci: {{.CI}}
---

# {{.Title}}

<!-- A plan is one file. It is either a decomposition into specs, a bare checklist
     of work, or both at once.

     One source of truth per item: an item either carries a checkbox (it is a task,
     and the box is its state) or references a spec (the state lives in that spec and
     is read from there). Never both — two records of one fact disagree, and the copy
     is the one that goes stale.

     Delete this comment. -->

## Why

<!-- One paragraph. What this plan is for, and what "done" means for the whole of it. -->

## Decomposition

<!-- The leaves that are big enough to be specs. A leaf is an ordinary spec: not
     nested under this plan, built by exactly the same rules. No checkboxes here —
     their state is derived from the spec. Delete the heading if this plan has none. -->

- `specs/<feature>/` — <what it covers>

## Tasks

<!-- The items this plan just does itself. Same grammar as a spec's tasks.md, minus
     the requirement citations: a plan has no requirements to cite. Delete the
     heading if every item is a spec. -->

- [ ] 1.1 (Unit) <description>
- [ ] 1.2 (TDD) <description>

## Notes

<!-- Order, dependencies between leaves, anything a reader needs to not merge these
     in the wrong sequence. -->
