# Delivery — branch, worktree, PR

Work does not happen on `main` and does not end with a green test run. It ends with a
pull request. Each unit of work gets its own branch in its own worktree:

```
git worktree add ../<repo>-<slug> -b <type>/<slug>
```

The user may run several sessions at once, one per feature, merging them as they land.
The worktree is what makes that possible: each session gets its own directory and none
touches the checkout the user is in. A shared tree with `git switch` cannot — two
sessions would fight over one working directory.

## Implementation is sequential — you write the code

**There is no implementation subagent and no parallel task dispatch.** Designed the
other way first, and rejected:

- Delegating implementation puts the cheaper model on the hardest work while the
  orchestrator keeps the part needing the least capability. That is backwards.
- Every fresh agent re-pays for discovery. Within one spec your accumulated context is
  the asset: you use the right parser in 1.2 because you wrote 1.1.
- **File-disjointness is not independence, and a clean merge hides the difference.**
  Two tasks touching no common file both need a `Money` type that does not exist yet.
  Each invents its own, with different semantics, and the merge is clean. Sequential
  execution cannot produce this: the later task sees the earlier task's code.

Feature-level parallelism has none of that and is supported — a *human* picks the split
and each session has full context. Worktrees isolate files, not the world: suites
fighting over a fixed port or one test database must be namespaced or serialized, and
two features green separately can still break together, which only CI on `main` sees.

## The delivery sequence

Once the last task is done:

1. **Full suite + lint** on the integrated branch. Per-task scoped runs cannot see
   breakage between tasks.
2. **`scc validate`** — the artifacts have to be in shape too, and exit `2` is not done.
3. **`code-review` and `security-review`** subagents on the diff, dispatched together.
   Each returns a verdict, what it checked, and findings by severity — you fix from
   that report, you do not re-review. `blocked` or any `blocker`/`critical` means the PR
   does not open yet; `major`/`high` is fixed before merge; `minor`/`low` is your call,
   and "not doing this, and why" in the PR body is a legitimate answer. A gate reported
   `not-run` is not a pass. The PR should arrive already reviewed — a human's attention
   spent on what a subagent would have caught is waste. One round of fix-and-re-run is
   worth it; a third means the finding needs a person.
4. **Commit and push.** Conventional Commits, written from the diff and the spec.
5. **Open the PR.** Body: what changed, which spec or plan, how it was verified.

Then CI, using the `ci:` answer from kickoff ([autonomy.md](autonomy.md)) — do not ask
now. **`wait`** means watch the checks until they settle, fixing and pushing while they
are red: the work is not finished while CI is failing. **`no-wait`** means opening the
PR is the finish line.

## Degrading

**No remote, or no `gh`** — commit on the branch and stop there, saying so. A branch
the user can push themselves is a real deliverable; silently skipping the PR is not.
**Worktrees accumulate** — remove one once its branch is merged, keep it if it holds
unmerged work, and say which you did.
