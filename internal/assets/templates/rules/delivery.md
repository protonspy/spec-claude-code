# Delivery — branch, worktree, PR

Work does not happen on `main` and does not end with a green test run. It ends with
a pull request.

## One branch per unit of work, in its own worktree

Each unit of work — a spec, or a plan's leaf — gets its own branch, developed in its
own **git worktree**:

```
git worktree add ../<repo>-<slug> -b <type>/<slug>
```

The reason is that the user may run several agent sessions at once, one per
feature, and merge them into `main` as they land. A worktree is what makes that
possible: each session gets its own directory, none disturbs the others, and none
touches the checkout the user is sitting in. A shared tree with `git switch` cannot
do this — two sessions would fight over one working directory.

## Implementation is sequential — you write the code

**There is no implementation subagent and no parallel task dispatch.** This was
designed the other way first and rejected:

- Delegating implementation puts the cheaper model on the hardest work while the
  orchestrator keeps the part that needs the least capability. That is backwards.
- Every fresh agent re-pays for discovery. Within one spec your accumulated context
  is the asset: you use the right parser in task 1.2 because you wrote task 1.1.
- **File-disjointness is not independence, and a clean merge hides the difference.**
  Two tasks touching no common file both need a `Money` type that does not exist
  yet. Each creates its own, with different semantics. The merge is clean and
  nothing signals that anything went wrong. Sequential execution cannot produce
  this, because the later task sees the earlier task's code.

Feature-level parallelism does not have that problem and is supported: the *human*
picks the split, each session has a full context, and two features a person
deliberately separated are unlikely to collide.

## What running several sessions still costs

Worktrees isolate files, not the world outside them. Say these out loud rather than
discovering them:

- **Shared external resources.** Two suites running at once fight over a fixed port,
  one test database, a shared temp path. Either the suite namespaces them per
  worktree, or the runs are serialized.
- **Cross-feature breakage.** Two features green on their own branches can be broken
  together. Only CI on `main` after the merge sees that.

## The delivery sequence

Once the last task is done:

1. **Full suite + lint** on the integrated branch. The per-task scoped runs cannot
   see breakage between tasks.
2. **`scc validate`** — the artifacts have to be in shape too, and exit `2` is not
   done.
3. **`code-review` and `security-review`** subagents on the diff, dispatched
   together. Each returns a verdict, a table of what it actually checked, and
   findings by severity — you fix from that report, you do not re-review. `blocked`
   or any `blocker`/`critical` finding means the PR does not open yet; `major`/`high`
   is fixed before merge; `minor`/`low` is your call, and saying "not doing this, and
   why" in the PR body is a legitimate answer. A gate reported `not-run` is not a
   pass: run it yourself or say in the PR that it was not run.

   The PR should arrive already reviewed: a PR is for the human, and spending their
   attention on findings a subagent would have caught is pure waste. Fixing then
   re-running the two agents is worth one round; a third round means the finding
   needs a person, not another review.
4. **Commit and push.** Conventional Commits, written from the diff and the spec.
5. **Open the PR.** Body: what changed, which spec or plan, how it was verified.

## Then, CI — the answer you already have

Use the `ci:` answer recorded at kickoff ([autonomy.md](autonomy.md)); do not ask now.

- **`wait`** — watch the PR's checks until they settle. Red means fix, push, keep
  watching. The work is not finished while CI is failing.
- **`no-wait`** — opening the PR is the finish line.

## Degrading

- **No remote, or no `gh`** — commit on the branch and stop there, saying so. A
  branch the user can push themselves is a real deliverable; silently skipping the
  PR is not.
- **Worktrees accumulate.** Remove a worktree once its branch is merged; keep it if
  it still holds unmerged work, and say which you did.
