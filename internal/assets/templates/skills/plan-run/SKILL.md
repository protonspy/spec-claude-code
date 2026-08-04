---
name: plan-run
description: Drive a whole plan under plans/ to completion — read the plan, report the groups, take whatever the invocation already decided and ask only for the rest, then implement group by group and deliver either one PR per group or one at the end, settling CI before calling the plan delivered. Resumes from the repository rather than from memory. Use it when someone asks to implement an entire plan, to keep going until the plan is finished, or runs /scc-plan-run. Not for a single spec or a one-off change, which delivery.md already carries end to end on its own.
---

You run a plan to the end.

The mechanics of delivering *one* unit of work are not here — they are in
`{{.Rules}}/delivery.md`, and repeating them would give this project two
copies of one procedure. This skill owns only what a loop adds on top: choosing the
next group, knowing when to stop, and finding your place again after a session dies.

And it owns one thing that is nobody's default to assume — **how the loop runs is the
developer's decision, taken once, after they have seen the groups.**

## What "delivered" means

The plan is delivered when **CI is green on the pull request that carries it** — not
when you believe the work is done.

That distinction is the point of this whole skill. Your own assessment of a finished
group is a claim; a green pipeline on a pushed branch is a fact about the repository
that a person can check without you. Every stopping rule below is written against
that fact. Never report a plan as delivered on the strength of a passing local suite:
say the PR is open, say where, and say what CI is doing.

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

## Before the first group — read, report, then ask what is still open

1. **Read the plan and work out the groups.** Ask nothing yet. The questions below
   are only answerable by someone who can see what they are agreeing to.
2. **Name the groups back, numbered, in order.** Order is the one thing a person can
   correct cheaply now and expensively after three merges.
3. **Take every answer the invocation already gave, and ask only for what is left.**
   A prompt like *"implement the whole plan, open one PR at the end, and if CI passes
   it is delivered"* has answered three of the four questions in one sentence.
   Re-asking what somebody just typed is the friction that stops people using this
   skill at all. Restate what you took, so a wrong reading is cheap to correct, then
   ask for the remainder in a single exchange.

| Ask | Answers | Recorded as |
|---|---|---|
| Run every group straight through, or stop at each group boundary for review? | automatic · gated | `autonomy: auto` · `gated` |
| One PR at the end of the plan, or one per group? | at the end · per group | `pr: per-plan` · `per-group` |
| A git worktree per group, or a branch in the checkout you are already in? | worktree · in place | `worktree: per-group` · `in-place` |
| Once a PR is open — wait for CI and merge, merge without waiting, or stop and let me merge? | wait and merge · merge now · stop at the PR | `ci` + `merge`, below |

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
- **one PR at the end** (`pr: per-plan`) — the cheap shape, and measurably the fast
  one: the groups become sequential commits on a single branch, the review subagents
  run once over the whole diff instead of once per group, and CI settles once. What
  it costs is granularity — a large diff to review, a red pipeline that does not say
  which group broke it, and nothing landed on `main` if the run stops half way. Right
  for a plan whose groups are one coherent piece of work; wrong when the groups ship
  to users independently or when someone else has to review them as they land.

**Write every answer into the plan's frontmatter before starting**, then never ask
again for this plan:

```yaml
---
autonomy: auto
ci: wait
pr: per-plan
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

Both shapes implement the same groups in the same order. What differs is how often
you stop to deliver.

### `pr: per-group` — one merged pull request per group

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
7. **Back to a green main** — `git switch main && git pull --ff-only`.
8. **Remove the worktree** now that its branch has landed, if there was one.
9. **Report the group in one line**, then start the next.

### `pr: per-plan` — one pull request at the end

Branch once from a green `main`, then for each group in order:

1. **Implement the group**, exactly as above — a leaf gets the spec cycle, a task
   family its tasks in order.
2. **Run the suite, the lint, and `scc validate` before moving on.** These stay per
   group and are not deferred with the rest. They are cheap, and they are what makes a
   later failure attributable: a break caught at group 3 is group 3's, while the same
   break found after group 9 costs a bisect.
3. **Commit the group on its own**, with the group in the subject, and tick its
   checkboxes in the same commit. The commits are the granularity this shape gives up
   in pull requests — do not squash the plan into one.
4. **Report the group in one line**, then start the next. Do not push a PR yet.

Then, once — and only once every group is in:

5. **Run both review subagents over the whole branch diff.** This is the deferral that
   makes the shape cheap. Their findings are fixed on the same branch before anything
   is pushed.
6. **Push and open one PR** covering the plan, its body naming every group it carries.
7. **CI and merge, exactly as answered** — the same rules as step 6 above, applied
   once. Green CI here is the plan delivered.

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

Whenever you stop, say which groups are done, where the work is, and which are
untouched. A half-run plan the user cannot locate is worse than one that never
started — and under `pr: per-plan` that is the whole run, sitting unmerged on a
branch, so name the branch every time.

## Resuming

Derive your position from the repository, never from what you remember. A compaction,
a crash, or a fresh session must land in the same place. **The answers are in the
plan's frontmatter — read them and carry on, do not ask again.** They were the
developer's call once; asking a second time because your context died makes them pay
for your problem.

Where you read your position from depends on the shape:

- **`pr: per-group` — read `main`.** Pull it and re-read the plan there; the copy in
  an old worktree is stale by construction. A task group whose boxes are ticked on
  `main` is done, as is a leaf whose spec's `tasks.md` is fully ticked there. An open
  PR means that group is mid-flight — under `merge: manual` that is the expected
  resting state. Finish it before starting another; two open groups is the fan-out
  this loop exists to avoid.
- **`pr: per-plan` — read the plan's branch.** Nothing reaches `main` until the end,
  so `main` will say no group is done and it will be wrong. Find the branch, read its
  log for the per-group commits, and re-read the plan **there**. If every group is
  committed but no PR is open, the run died between the last group and the review
  pass: run the subagents and push. If the PR is open, the run died waiting on CI.

A leftover worktree whose branch is already merged is debris. Remove it.

A plan whose frontmatter carries no `pr`, `worktree`, or `merge` is a plan no loop has
run over. Ask what the invocation did not already answer.

## Degrading

- **No remote, or no `gh`.** delivery.md stops at the branch, and a loop cannot —
  under `pr: per-group` the next group needs the last one on `main`, and under
  `pr: per-plan` there is no pipeline to make "delivered" mean anything. Say so *when
  you ask the questions*, not after the first group is written: "there is no remote
  here, so this runs local-only — the work merges into your `main` with `git merge
  --no-ff`, no PR is opened and no CI runs, so the suite passing locally is all the
  evidence you will get." That turns a degraded run into something the developer
  chose. Never let it look like every group was reviewed, and never call such a plan
  delivered.
- **The plan grows while the loop runs.** Re-read it at each group boundary. The
  group list from step 2 is a report, not a contract, and a group appended after you
  started is still part of the plan.
