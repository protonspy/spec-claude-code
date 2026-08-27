---
name: plan-run
description: Drive a whole plan under plans/ to completion — brief the plan once, report the groups, take whatever the invocation already decided and ask only for the rest, then implement group by group, asking scc for the next task rather than opening the file, and deliver either one PR per group or one at the end, settling CI before calling the plan delivered. Resumes from the repository rather than from memory. Use it when someone asks to implement an entire plan, to keep going until the plan is finished, or runs /scc-plan-run. Not for a single spec or a one-off change, which delivery.md already carries end to end on its own.
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

A **group** is one family of task numbers sharing a major number — `1.1`, `1.2` → group
1 — and it is the smallest part of the plan that can merge on its own. Implementing it
means those tasks, in order.

A task that names a spec under `## References` is that spec: run the ordinary cycle for
it, and tick the task when the spec closes. The reference itself is never ticked —
its state lives in that spec and is read from there.

**You do not decide the order and you do not look for prose that overrides it.**
`scc map tasks <plan> --next` is the order: eligible first, then priority, then number.
A task that names a dependency waits for it; `--blocked` says what an impasse is on.

A plan with a flat, unnumbered checklist has exactly one group. Say so and run it
once, rather than inventing a decomposition the author did not write.

## Before the first group — brief, report, then ask what is still open

1. **Brief the plan.** `scc map brief <plan>` gives you the title, why it exists, the
   paths, the references and what "done" means — the header and nothing else. Then
   `scc map <plan>` for the group counts. **Never open the plan file**: `brief` reads
   the header, `tasks` reads the checklist, and there is nothing else in it. Ask
   nothing yet — the questions below are only answerable by someone who can see what
   they are agreeing to.
2. **Name the groups back, numbered, in order.** Order is the one thing a person can
   correct cheaply now and expensively after three merges.
3. **Take every answer the invocation already gave, and ask only for what is left.**
   A prompt like *"implement the whole plan, open one PR at the end, and if CI passes
   it is delivered"* has answered most of the table below in one sentence.
   Re-asking what somebody just typed is the friction that stops people using this
   skill at all. Restate what you took, so a wrong reading is cheap to correct, then
   ask for the remainder in a single exchange.

| Ask | Answers | Recorded as |
|---|---|---|
| Run every group straight through, or stop at each group boundary for review? | automatic · gated | `autonomy: auto` · `gated` |
| One PR at the end of the plan, or one per group? | at the end · per group | `pr: per-plan` · `per-group` |
| Once a PR is open — wait for CI and merge, merge without waiting, or stop and let me merge? | wait and merge · merge now · stop at the PR | `ci` + `merge`, below |

The plan's frontmatter may already carry `autonomy` and `ci` from when it was written.
**Show those as the proposed answers and confirm them; do not ask blind, and do not
treat them as decided.** They were given for authoring the plan — this is a loop that
will open and merge pull requests for hours, which is a larger thing to agree to.

Say what each answer costs, in one line each, because some of them have a consequence
that only shows up later:

- **wait and merge** (`ci: wait`, `merge: auto`) — the loop runs to the end unattended.
- **merge now** (`ci: no-wait`, `merge: auto`) — fastest, and every later group
  branches from a base CI never checked. Say this out loud before accepting it.
- **stop at the PR** (`merge: manual`) — the loop pauses after every group until the
  merge lands. It is no longer unattended, and that is a legitimate thing to want.
- **one PR at the end** (`pr: per-plan`) — the cheap shape, and measurably the fast
  one: the groups become sequential commits on a single branch, the review subagents
  run once over the whole diff instead of once per group, and CI settles once. What
  it costs is granularity — a large diff to review, a red pipeline that does not say
  which group broke it, and nothing landed on `main` if the run stops half way. Right
  for a plan whose groups are one coherent piece of work; wrong when the groups ship
  to users independently or when someone else has to review them as they land.

**Write every answer into the plan's frontmatter before starting**, then never ask
again for this plan:

```bash
scc patch fm <plan> autonomy=auto ci=wait pr=per-plan merge=auto
```

```yaml
---
autonomy: auto
ci: wait
pr: per-plan
merge: auto
---
```

`patch fm` writes each key by name — replacing one that is already there, adding one
that is not — and re-validates afterwards, so a typo'd value is refused rather than
recorded as an answer nobody gave. It is also the first write of the loop, and doing
it this way means the plan never has to be in context to be configured.

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
2. **Branch** in the checkout you are in, as `{{.Rules}}/delivery.md` has
   it. One directory for the whole run, which means the checkout goes back to `main`
   and clean when the group ends.
3. **Implement the group, one `--next` at a time.** `scc map tasks <plan> --next
   --group N --json` gives you the one task to do; do it, verify it, tick it, ask
   again. That loop — one call per task — is why the plan never has to be in context.
   A task naming a spec gets the spec cycle.
4. **Deliver**, following delivery.md's sequence in full — suite and lint, `scc
   validate`, both review subagents, commit, push, open the PR. Then record it on every
   spec the group delivered: `scc spec track <feature> --here --pr <n>`. A plan run
   opens more branches than anything else here, so it is the loop where an unrecorded
   one is most likely to be the branch nobody comes back to.
5. **Record the group's state in that same PR.** The checkboxes are ticked in the plan
   file, in the branch that does the work, so `main` and the plan agree the moment the
   merge lands — `scc patch check <plan> 1.1 1.2 …`, which addresses each task by
   number and re-validates the file.
6. **CI and merge, exactly as answered.** `ci: wait` means watch the checks until they
   settle and fix what is red before merging. `merge: auto` means you merge once that
   answer is satisfied; `merge: manual` means you open the PR, say where it is, and
   stop — the loop resumes when the developer's merge is on `main`.
7. **Back to a green main** — `git switch main && git pull --ff-only`, with nothing
   uncommitted left behind. The next group branches from there.
8. **Report the group in one line**, then start the next.

### `pr: per-plan` — one pull request at the end

Branch once from a green `main`, then for each group in order:

1. **Implement the group**, exactly as above — one `--next` at a time, each verified
   before the next.
2. **Run the suite, the lint, and `scc validate` before moving on.** These stay per
   group and are not deferred with the rest. They are cheap, and they are what makes a
   later failure attributable: a break caught at group 3 is group 3's, while the same
   break found after group 9 costs a bisect.
3. **Commit the group on its own**, with the group in the subject, and tick its
   checkboxes in the same commit — `scc patch check <plan> 2.1 2.2 …`, as above. The
   commits are the granularity this shape gives up in pull requests — do not squash
   the plan into one.
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
- `--next` reports nothing eligible while open tasks remain. `--blocked` names what
  each is waiting on; a cycle or a dependency on a struck-out task is a defect in the
  plan and exits `2`.
- The plan reports **drift** — it was edited outside `scc` after being approved. Say
  so and stop: something changed the work the developer signed off on, and `git diff`
  is the answer, not `plan reseal`.

**Work that turns up mid-loop is discovery, not an edit.** In an approved plan,
`scc patch add <plan> --group N --text "…" --reason "…"` allocates the next number
and records why it was not in the plan; `scc patch rm <plan> 2.3 --reason "…"` strikes
a task out where it stands. Neither touches the prose, and neither is a reason to open
the file.

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

Resuming is a question about state, not about prose, so read it as state: `scc map
<plan>` for the counts and `scc map tasks <plan> --next` for what to do. Re-reading a
whole plan to find one unticked box is the cost this loop would otherwise pay every
time a session dies. `brief` again only if you have lost what the plan is for.

Which branch you read that from depends on the shape:

- **`pr: per-group` — read `main`.** Switch to it, pull, and map the plan there; a
  half-finished group branch is stale by construction. A group whose boxes are ticked
  on `main` is done, as is a task naming a spec whose `tasks.md` is fully ticked
  there — `scc map
  trace specs/<feature>/` answers that in one call, without opening either file. An
  open PR means that group is mid-flight; under `merge: manual` that is the expected
  resting state. Finish it before starting another — two open groups is the fan-out
  this loop exists to avoid.
- **`pr: per-plan` — read the plan's branch.** Nothing reaches `main` until the end, so
  `main` will say no group is done and it will be wrong. Find the branch, read its log
  for the per-group commits, and map the plan **there**. If every group is committed
  but no PR is open, the run died between the last group and the review pass: run the
  subagents and push. If the PR is open, the run died waiting on CI.

A plan whose frontmatter carries no `pr` or `merge` is a plan no loop has
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
- **The plan grows while the loop runs.** Re-map it at each group boundary — `scc map
  <plan>` — because this is the read the loop performs most often and the one that
  most tempts you to reach for the file instead. The group list from step 2 is a
  report, not a contract, and a group appended after you started is still part of the
  plan.
- **The plan is still a draft.** `scc plan approve <plan>` before the first group: it
  validates, fixes the content, and seals it, so a later edit made outside `scc` is
  visible rather than silent. A plan that will not approve has findings — report them
  and stop, rather than running a plan whose own validator rejects it.
