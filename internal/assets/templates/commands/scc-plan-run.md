---
description: Run a plan under plans/ to completion — implement group by group, then deliver as one PR per group or one at the end, and settle CI before calling it delivered
argument-hint: [the plan, plus how to run it and any standing instruction]
---

Use the `plan-run` skill.

Plan, and how to run it: $ARGUMENTS

Brief the plan — `scc map brief <plan>` — then `scc map <plan>` for the counts, and
name the groups back, numbered and in order, before writing any code. The order is the
one thing the user can correct cheaply now and expensively after three merges.

**Never open the plan file.** `brief` is its header and `tasks` is its checklist;
there is nothing else in it, and opening one as the first act of a run puts all of it
in context for every turn of a loop that lasts hours. Inside a group, ask `scc map
tasks <plan> --next` for the one task to do, and ask again once it is ticked.

Then take every answer the line above already gave and ask only for what is left.
"Implement the whole plan, one PR at the end, delivered when CI is green" has settled
most of it; re-asking what someone just typed is the friction that stops people using
this at all. Restate what you took so a wrong reading is cheap to correct, then put
the remaining questions in one exchange — automatic or gated, one PR at the end or one
per group, a worktree per group or the current checkout, and what happens once a PR is
open. **These are the developer's calls.** Anything the plan's frontmatter already
records is a proposed answer to confirm, not a decision already made.

The plan is delivered when CI is green on its pull request — never on the strength of
a passing local suite.

Anything said above about *how* to implement is a standing instruction: it applies
to every group, and you carry it into each one explicitly rather than trusting it to
survive from the first group to the last.
