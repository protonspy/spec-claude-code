# Hooks — the check that stops needing to be remembered

A rule holds until the session that does not re-read it. A validator turns the rule
into a check, and a check holds until the session that does not run it. **A hook is
the same check with the remembering taken out** — something else runs it, on every
commit or at the end of every turn, whoever typed the command.

`scc` installs two layers, and they degrade independently: a directory that is not a
git repository can still have harness hooks, and a repository whose harness has no
hook surface still gets its git hooks.

## The git layer — three stages, each where its question can first be answered

| Stage | Runs |
|---|---|
| `pre-commit` | `scc validate` |
| `commit-msg` | the message being written |
| `pre-push` | `scc validate --pr --checks` |

**`commit-msg` is the stage that answers the complaint.** The validator can report
an assistant's signature that is already in a commit; taking it out then means
rewriting history. At `commit-msg` the message is still a file, so a non-zero exit
leaves it in `.git/COMMIT_EDITMSG` for the next attempt. What it reads is what git
will keep — comment lines and everything below the `--verbose` scissors are stripped
first, or the check would fire on git's own template and on the diff, every commit.

The scripts are three lines and call back into `scc hooks run <stage>`, so **every
decision lives in Go**, where it is tested and where upgrading the binary changes
the gate without rewriting a file in somebody's `.git`. They **fail closed** when
`scc` is not on PATH — a hook that passes silently when its checker is missing
reports success it did not verify — and they name the way past themselves
(`SCC_SKIP_HOOKS=1`), because a gate with no documented escape is a gate people
delete the first time it is wrong.

Where the hooks go is **asked of git** (`core.hooksPath`, then
`rev-parse --git-path hooks`) rather than assumed to be `.git/hooks`: writing a hook
where git will not read it is the kind of failure that looks like success for weeks.
A hook `scc` did not write is reported as foreign and left alone — see
[[managed-files]].

## The harness layer — the end of a turn

A git hook fires when work enters history. The commit is the *second* moment a
finding exists; the first is the end of the turn that wrote it, while the agent
still has the file in mind. Claude Code will run a command then and read what it
prints into the next turn, so three events are spliced into `settings.json`:
`SessionStart`, `Stop`, and `UserPromptSubmit`.

### Stop may only carry what the next turn can resolve

This is the rule the design was missing, and getting it wrong shipped a loop. The
harness treats output on `Stop` as guidance the conversation *continues* for, under
the same loop protections as an outright block — so returning `0` is not a passive
report there. One release asserted the opposite, never read `stop_hook_active`, and
put a branch-scoped finding on a stage that re-fires until the consecutive-block
cap: measured in a real session, the agent answered `.` to itself nine times.

Two rules now hold it: **`stop_hook_active` is honoured unconditionally**, and
**Stop may only carry what the next turn can resolve.** A finding clears when fixed;
unpushed work clears when pushed; a fact about the branch clears for nobody, which
is why drift moved to `SessionStart`.

That second rule had two more violations of the same shape. The stage ran every
validator, `attribution` included — whose subject is commits already made, so the
fix is a rebase and the line re-appears at the end of every turn for the life of the
branch; it is out of this run entirely. And it reported findings anywhere in the
workspace, so a stale file the task never touched asked the agent, every turn, to go
and fix something it had no business in; findings are now intersected with what this
branch changed, and `scc validate` is where the whole workspace still answers.

### Silence is the common case

Three drift signals, **each a line only when it is true** — source changed with no
box ticked, a forbidden marker left in code, commits sitting on the base branch.
`TestTheDriftStageIsSilentOnACleanBranch` holds that, because a stage that speaks
every turn is one the reader learns to skip, and then the turn it had something to
say is the turn nobody read it.

This is where the design differs from the tool it was ported from. That one assumes
an agent drifts and re-asserts its instruction on every prompt, which works because
its instruction is a *disposition* — reach for the smallest thing that works — and a
disposition has no shape, so re-asserting is the only move available. `scc`'s rules
are not dispositions: each leaves evidence on disk when it is broken. So the answer
is not to re-assert a rule at somebody who has read it — it is to say what actually
happened.

### `UserPromptSubmit` is bounded by what it may speak about

It fires on every prompt of every session — the worst cost profile in the set — so
what makes it defensible is that it almost never speaks: only when the prompt names
an artifact this workspace actually has, and then only to say how to read that
artifact through `scc map` instead of opening it (see [[artifact-addressing]]).

Three details are load-bearing: it runs **no subprocess**, because it sits between
the keystroke and the agent while the other two run when nobody is waiting; a name
shorter than four characters is matched only by its path, since a spec called `ui`
would otherwise fire on "build the ui"; and `-`/`.` are *inside* a name rather than
boundaries around it, so `auth` does not match the spec `auth-flow`. This is also
the one stage where a non-zero exit discards the user's prompt before the agent sees
it, which a test pins.

### They never publish

The end of a turn is a plausible moment to `git push`, and doing it would send work
on an event nobody is watching. So the Stop stage *says* the branch has commits that
never left the machine, and the agent pushes in its next turn, in the open, where a
person can still stop it. The question is answered from refs already in the
repository — no fetch, no `gh` — because this runs at the end of every turn.

## No `SubagentStart`

The absence is gated rather than assumed: Claude Code hands a non-fork subagent the
whole rule hierarchy already, so the stage would re-send ~26KB the agent holds, once
per subagent. See [[harness-profile]] and [[context-budget]].

## Related

[[validation-contract]] · [[delivery-gate]] · [[managed-files]] · [[context-budget]]
