---
name: plan-run
description: Drive a whole plan under plans/ to completion group by group — read the plan, report the groups, ask the developer how the loop should run, then for each group branch from a green main, implement, open the PR, settle CI, merge, pull, and take the next. Resumes from what is on main rather than from memory. Use it when someone asks to implement an entire plan, to keep going until the plan is finished, or runs /scc-plan-run. Not for a single spec or a one-off change, which delivery.md already carries end to end on its own.
---

You run a plan to the end, one merged pull request per group.

The mechanics of delivering *one* unit of work are not here — they are in
`{{.Rules}}/delivery.md`, and repeating them would give this project two
copies of one procedure. This skill owns only what a loop adds on top: choosing the
next group, branching every group from the merge the last one produced, knowing when
to stop, and finding your place again after a session dies.

And it owns one thing that is nobody's default to assume — **how the loop runs is the
developer's decision, taken once, after they have seen the groups.**

## What a group is

A **group** is the smallest part of the plan that can merge on its own.

| In the plan | One group is | What implementing it means |
|---|---|---|
| `## Decomposition` | one leaf, `specs/<feature>/` | an ordinary spec — the whole cycle, by the ordinary rules |
| `## Tasks` | one family of task numbers sharing a major number (`1.1`, `1.2` → group 1) | those tasks, in order |

The order is the order they are written in, unless `## Notes` says otherwise. Notes
wins — that heading exists precisely to say what must not be merged out of sequence.

A plan with a flat, unnumbered checklist has exactly one group. Say so and run it
once, rather than inventing a decomposition the author did not write.

## Before the first group — read, report, then ask

1. **Read the plan and work out the groups.** Ask nothing yet. The questions below
   are only answerable by someone who can see what they are agreeing to.
2. **Name the groups back, numbered, in order.** Order is the one thing a person can
   correct cheaply now and expensively after three merges.
3. **Then ask, once, in one exchange — all three questions together.** How this loop
   runs is the developer's call, not yours, and not a default you inferred from a
   file.

| Ask | Answers | Recorded as |
|---|---|---|
| Run every group straight through, or stop at each group boundary for review? | automatic · gated | `autonomy: auto` · `gated` |
| A git worktree per group, or a branch in the checkout you are already in? | worktree · in place | `worktree: per-group` · `in-place` |
| Once a group's PR is open — wait for CI and merge, merge without waiting, or stop and let me merge? | wait and merge · merge now · stop at the PR | `ci` + `merge`, below |

The plan's frontmatter may already carry `autonomy` and `ci` from when it was written.
**Show those as the proposed answers and confirm them; do not ask blind, and do not
treat them as decided.** They were given for authoring the plan — this is a loop that
will open and merge pull requests for hours, which is a larger thing to agree to.

Say what each answer costs, in one line each, because two of them have a consequence
that only shows up later:

- **wait and merge** (`ci: wait`, `merge: auto`) — the loop runs to the end unattended.
- **merge now** (`ci: no-wait`, `merge: auto`) — fastest, and every later group
  branches from a base CI never checked. Say this out loud before accepting it.
- **stop at the PR** (`merge: manual`) — the loop pauses after every group until the
  merge lands. It is no longer unattended, and that is a legitimate thing to want.
- **in place** (`worktree: in-place`) — one directory, and this session cannot run
  alongside another on the same repo. Right when the project's setup is expensive to
  duplicate; wrong when the user is running several features at once.

**Write all four answers into the plan's frontmatter before starting**, then never ask
again for this plan:

```yaml
---
autonomy: auto
ci: wait
worktree: per-group
merge: auto
---
```

That is what makes a resumed session pick up where this one stopped instead of
interrogating the developer a second time. `scc validate` checks the values.

4. If the invocation carried a standing instruction, restate it and apply it to
   every group — carry it into each group explicitly, rather than trusting it to
   survive the context between the first group and the fifth.

## The loop

For each group, in order:

1. **Start green.** In the primary checkout, `git switch main && git pull --ff-only`.
   Every group branches from the previous group's merge, which is the whole reason
   this is a loop and not a fan-out.
2. **Branch**, in a worktree or in place, as `worktree:` was answered. The worktree
   mechanics are `{{.Rules}}/delivery.md`'s; `in-place` means the same
   branch without the worktree, and it means you must leave the checkout on `main`
   and clean when the group ends.
3. **Implement the group.** A leaf is a spec and gets the spec cycle. A task family
   is its tasks, sequential, each verified before the next.
4. **Deliver**, following delivery.md's sequence in full — suite and lint, `scc
   validate`, both review subagents, commit, push, open the PR.
5. **Record the group's state in that same PR.** A task group's checkboxes are ticked
   in the plan file, in the branch that does the work, so `main` and the plan agree
   the moment the merge lands. A leaf is never ticked — its state lives in the spec
   and is read from there.
6. **CI and merge, exactly as answered.** `ci: wait` means watch the checks until they
   settle and fix what is red before merging. `merge: auto` means you merge once that
   answer is satisfied; `merge: manual` means you open the PR, say where it is, and
   stop — the loop resumes when the developer's merge is on `main`.
7. **Back to a green main** — `git switch main && git pull --ff-only`. Every group
   branches from the previous group's merge, which is what makes this a loop.
8. **Remove the worktree** now that its branch has landed, if there was one.
9. **Report the group in one line**, then start the next.

## Where the loop stops

Stopping part-way is a legitimate outcome, and continuing past any of these is not:

- A review subagent returns `blocked`, or any `blocker`/`critical` finding survives a
  round of fixes.
- CI is red twice on the same cause. A second identical failure is a person's
  problem, not a third push.
- The branch conflicts with `main` in a way that is not mechanical — something landed
  that the plan did not account for.
- The group turns out to need a decision the plan never made. Bring the decision back;
  do not make it silently in the middle of a loop nobody is watching.

Under `autonomy: gated`, stop at every group boundary and wait, having reported what
merged.

Whenever you stop, say which groups merged, which one is open and where, and which
are untouched. A half-run plan the user cannot locate is worse than one that never
started.

## Resuming

Derive your position from `main`, never from what you remember. A compaction, a
crash, or a fresh session must land in the same place:

- Pull `main` and re-read the plan **there** — the copy in an old worktree is stale by
  construction.
- **The four answers are in that frontmatter. Read them and carry on — do not ask
  again.** They were the developer's call once; asking a second time because your
  context died makes them pay for your problem.
- A task group whose boxes are ticked on `main` is done.
- A leaf whose spec's `tasks.md` is fully ticked on `main` is done.
- An open PR for this plan means that group is mid-flight. Under `merge: manual` that
  is the expected resting state and the merge may simply not have happened yet.
  Finish it before starting another; two open groups is the fan-out this loop exists
  to avoid.
- A leftover worktree whose branch is already merged is debris. Remove it.

A plan whose frontmatter carries no `worktree` or `merge` is a plan no loop has run
over. Ask the three questions.

## Degrading

- **No remote, or no `gh`.** delivery.md stops at the branch, and a loop cannot —
  the next group needs the last one on `main`. Say so *when you ask the three
  questions*, not after the first group is written: "there is no remote here, so this
  runs local-only — each group merges into your `main` with `git merge --no-ff`, no PR
  is opened and no CI runs." That turns a degraded run into something the developer
  chose. Never let it look like every group was reviewed.
- **The plan grows while the loop runs.** Re-read it at each group boundary. The
  group list from step 2 is a report, not a contract, and a group appended after you
  started is still part of the plan.
