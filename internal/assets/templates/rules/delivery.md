# Delivery — branch, PR

Work does not happen on `main` and does not end with a green test run. It ends with a
pull request. Each unit of work gets its own branch in the checkout you are in —
`git switch -c <type>/<slug>`, from a green `main` — and the checkout goes back to
`main`, clean, once the work lands. Nothing here needs a second directory: how a user
runs several sessions against one repo at once is theirs to set up.

## Implementation is sequential — you write the code

**There is no implementation subagent and no parallel task dispatch.** It puts the
cheaper model on the hardest work, and every fresh agent re-pays for discovery — within
one spec your context is the asset: you use the right parser in 1.2 because you wrote
1.1. Above all, **file-disjointness is not independence, and a clean merge hides it**:
two tasks sharing no file both need a `Money` type that does not exist, each invents one
with different semantics, and the merge is clean. Sequential execution cannot do that.

Feature-level parallelism has none of that and is supported — a *human* picks the split
and each session has full context. Separate sessions isolate files, not the world: a fixed
port or one test database must be namespaced, and two green features can break together.

## The delivery sequence

Once the last task is done:

1. **`scc test` + lint** on the integrated branch — the whole suite, and its coverage
   against this workspace's floor. Per-task runs cannot see breakage between tasks.
2. **`scc validate --tests --pr`** — artifacts, record and coverage in one gate; `2` is not done.
3. **`code-review` and `security-review`** subagents on the diff, dispatched together.
   Each returns a verdict, what it checked, and findings by severity — you fix from that
   report, you do not re-review. `blocked` or any `blocker`/`critical` means the PR does
   not open yet; `major`/`high` is fixed before merge; `minor`/`low` is your call, and
   "not doing this, and why" in the PR body is a legitimate answer. A gate reported
   `not-run` is not a pass. The PR should arrive already reviewed — a human's attention
   spent on what a subagent would catch is waste; one round of fix-and-re-run, not three.
4. **Commit and push.** Conventional Commits, written from the diff and the spec.
5. **Open the PR.** Body: what changed, which spec or plan, how it was verified.

**The work is the user's, and the record says so.** No `Co-Authored-By` for an assistant,
no session link, no "generated with" footer or badge, no naming of a model, vendor or
harness — not in a commit message, not in a PR title or body. The `commit-msg` hook
rejects it as you write it, `SCC_SKIP_HOOKS=1` is not the fix, `--pr` reads the PR body.

**The tests are a number, not a claim.** `scc test` runs what this workspace recorded
and reads back `{"total": N, "coverage": P}`; under the floor, or zero tests, is a
finding. `pre-push` runs it, so a branch that cannot clear it does not become a PR.

**A branch leaves no trace in the artifacts, so record it.** `scc spec track <feature>
--here` when you branch, `--pr <n>` when the PR opens; `scc spec sync` reads git and the
forge back into every spec, so *unfinished* is a state the workspace reports.

Then CI, using the `ci:` answer from kickoff ([autonomy.md](autonomy.md)) — do not ask
now. **`wait`** means watch the checks until they settle, fixing and pushing while they
are red: the work is not finished while CI is failing. **`no-wait`** means opening the
PR is the finish line.

## Degrading

**No remote, or no `gh`** — commit on the branch and stop there, saying so. A branch
the user can push themselves is a real deliverable; silently skipping the PR is not.
**A checkout left dirty or off `main`** is what this shape can leave behind — say what
is still uncommitted rather than starting the next unit of work on top of it.
