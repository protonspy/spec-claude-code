---
description: Turn an initiative too large for one spec into a plan under plans/ — decomposed into specs and tasks
argument-hint: [the initiative, epic, or PRD]
---

Use the `prd` skill.

Initiative: $ARGUMENTS

Check the routing question before you start: work that is one feature with unsettled
requirements is a spec, not a plan — run `scc spec new <feature>` instead of wrapping
one feature in a plan for ceremony.

If the initiative is too vague to decompose, ask a small batch of concrete
multiple-choice questions once, then decompose. Do not guess at the scope, and do not
interview at length — stop asking the moment you can name the leaves.
