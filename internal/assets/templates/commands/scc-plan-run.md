---
description: Run a plan under plans/ to completion group by group — worktree, implement, review, PR, CI, merge, then the next group
argument-hint: [the plan, plus any standing instruction for every group]
---

Use the `plan-run` skill.

Plan, and how to run it: $ARGUMENTS

Read the plan and name the groups back, numbered and in order, before writing any
code. The order is the one thing the user can correct cheaply now and expensively
after three merges.

Then ask how this loop should run — automatic or gated, a worktree per group or the
current checkout, and what happens once each PR is open. Three questions, one
exchange, after they can see the groups they are agreeing to. **These are the
developer's calls.** Anything the plan's frontmatter already records is a proposed
answer to confirm, not a decision already made.

Anything said above about *how* to implement is a standing instruction: it applies
to every group, and you carry it into each one explicitly rather than trusting it to
survive from the first group to the last.
