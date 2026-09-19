---
name: gauntlet-loop
description: Turns a goal for scc's own methodology or harness into one short, paste-ready "gauntlet loop" prompt - a prompt that makes an agent set a control arm, split the work into judgeable pieces, run a builder and a separate harsh critic on each, compare blind on the same ticket, and loop until the change beats its own absence or comes out. Triggers on "/gauntlet-loop", "gauntlet loop", "gauntlet this", "loop until it beats X".
---

# Gauntlet Loop — scc edition

The user gives a goal about scc: the methodology (the algorithm the agent runs) or the
harness that delivers it (rules, skills, CLI, hooks). You give back ONE short prompt they
can paste into a fresh agent session.

You are not doing the work. You are writing the prompt that makes another agent grind on
it until the change can be told apart from its own absence.

## Flow

1. **Read the goal.** Name the surface it touches — algorithm, rules, skills, CLI, hooks.
2. **Set the arm.** If the user named a control, use it. If not, offer **2 or 3 candidate
   arms**, one line each, with the number each would move, and stop. Wait for their pick.
3. **State the cost** in the same breath: how many agent sessions, roughly what that
   spends.
4. **Write the prompt.** One block, paste-ready, no preamble, no headings inside it.
5. **Offer to run it.** One flat line under it: "I can run this here." Not a question.

If they say run it, you become the lead agent and follow the prompt you just wrote.

## The bar here is an arm, not an artifact

A landing-page gauntlet screenshots Nike. scc has nothing to screenshot. What it has is the
question `design/ponytail-port.md` closed without answering: **nothing measures whether the
rules earn the ~47KB they cost** (measured: `wc -c internal/assets/templates/rules/*.md
internal/assets/templates/entry.md` = 47695, and every unscoped byte of it is in every
request of every session).

So the comparison is two harness configurations running **the same ticket**, and the critic
is blind to which is which. That is the whole port. A critic reading a rule and saying it
looks good is the vague-bar failure wearing a different hat.

An arm has to pass three tests:

- **Isolated.** The control must not secretly be running the thing. ponytail's own benchmark
  was wrong for a whole draft because its plugin hook fired on the control arm. Here the leak
  is the operator's global config — `~/.claude/rules/`, `~/.claude/settings.json`, a harness
  directory already on the machine. Scaffold into a throwaway root and test the isolation
  before trusting a number out of it (`design/ponytail-port.md` 4.2).
- **Ticketed, not prompted.** scc's claims are about what an agent leaves behind across a
  session — a ticked box, a cited requirement, a clean `scc validate`. One completion cannot
  exhibit those. Pin a target repo at a fixed commit and a ticket set.
- **Scored on what scc claims.** `scc validate` findings, the delivery gate's result, whether
  boxes were ticked in the file and requirements cited — *plus* diff LOC, tokens, cost and
  wall time. LOC alone measures ponytail's thesis, not this one (4.3).

## The five surfaces

| Surface | Control arm | The number already instrumented | The cheat to guard |
|---|---|---|---|
| **Algorithm** — routing, the ladder, spec→plan→task, `Ready`/`Next`/`Cycles` | the same ticket with that step of the methodology removed | findings caught, boxes ticked, requirements cited | a step that only ever fires where a validator already reports the failure |
| **Prompts** — `internal/assets/templates/rules/*.md`, `entry.md` | that rule absent from the scaffolded set | 55 lines/rule, 60 lines/entry (`TestRulesStayShortEnoughToBePreloaded`, `TestScaffoldedEntryFileStaysShort`); bytes preloaded | shortening the words while destroying the instruction |
| **Skills** — `assets.Skills()`, the eight `scc-*` | the skill not scaffolded | 340 chars/description, 2400 total (`TestSkillDescriptionsStayShortEnoughToPreload`) | moving cost from the body (paid once) into the description (paid always) |
| **CLI** — `internal/cli`, `map`/`patch`/`notes`/`check` | answering the same question with `Read` on the whole file | tokens per answer; `map tasks --json` is 1558 bytes against a 56KB plan reread | a flag that reports success on an edit it misfiled |
| **Hooks** — git stages, `SessionStart`/`Stop`/`UserPromptSubmit` | the stage registered but inert | silence on a clean branch, turns-to-resolution, loop safety | a Stop stage carrying something the next turn cannot resolve — that shipped once and cost nine turns |

**Scoping is the other lever, and it is mechanical.** A rule may be scoped with a `paths:`
header only where a validator reports what it prevents (`TestOnlyRulesAValidatorBacksAreScoped`).
If the gauntlet's answer is "scope it", the critic checks that gate before accepting the
saving.

## Two numbers, always

Every scc gauntlet moves a **standing cost** and an **effect**, and it is only a win if it
moves the first down without moving the second.

- Standing cost: bytes, lines, characters — already capped by tests, so a regression fails
  `make check` rather than needing a judge.
- Effect: did the thing the surface exists to cause actually happen on the ticket.

An optimizer told to cut tokens finds the first half in one round and the second never. Say
both in the prompt, or the loop exits on round one having deleted the methodology.

## Invariants the builder may not trade away

The critic checks these before it judges anything else. A change that wins by breaking one
has lost:

- Exit codes are the contract: `0` ok, `1` usage/runtime, `2` findings. `--help` is `0`.
- `go.mod` stays stdlib-only. Six platforms, every dep a supply-chain surface.
- `TestFreshArtifactsPassTheirOwnValidators` — a validator firing on scc's own output is the
  worst bug in the product.
- Never author what the user owns; edited files survive an update.
- LF everywhere, deterministic renders, Windows a first-class target.
- No hook stage may block or end a turn; `stop_hook_active` is honoured unconditionally.
- No attribution in a commit message or a PR body.
- `make check` green: `gofmt -l` + `go vet` + `go test -race`.

## Prompt template

Adapt the wording every time. Fill the brackets, keep it short, keep the last lines.

```
[CHANGE] in scc, and find out whether it earns its place.

The control arm is [ARM]. Scaffold both arms into throwaway roots from the same commit,
prove the control is not secretly running the thing — the operator's global config is the
leak — and run both over the same pinned ticket set.

Break this into the smallest pieces that can be changed and judged on their own. For each
piece, fan out a builder and a separate critic with fresh context. The critic runs both
arms, reads the two transcripts and the two diffs blind with the arm labels stripped, says
which session produced better work, and names the single biggest remaining gap. Then it
goes back to the builder.

Score both halves: [STANDING COST] and whether the ticket ended with [EFFECT]. A change
that cuts the cost and loses the effect has failed. Check the invariants first — exit
codes and the boundaries in `.claude/rules/project.md`, stdlib-only in
`docs/adr/0001-stdlib-only-dependencies.md`, a green `scc check`, and no validator firing
on scc's own output (`docs/wiki/pages/validation-contract.md`) — a builder that wins by breaking one has lost.

The critic should be a harsh critic. Praise is not useful. If the arm carrying the change
does not win blind, it keeps going.

/loop on each piece until the critic picks the change blind, or until it is clear the
change cannot be told apart from its absence — in which case say so and take it out.

Budget: [N] sessions, stop at [CEILING].

Keep a live progress page updating as the work evolves so I can watch it.

Fan out subagents and ultracode.
```

Rules for what you fill in:

- The arm is a concrete configuration, reproducible from a command. Not "without the rules".
- **Always include the budget line.** This is the one place the generic gauntlet's "no
  default cap" does not hold: the measurement half of `design/ponytail-port.md` was skipped
  because 48 sessions cost an estimated $14–$120, and that is written into the repo. A
  gauntlet that hides its price gets skipped for the same reason.
- "Or take it out" is not optional. The exit that was never reached (`ponytail-port.md` 5.3)
  is the one where a rule that cannot be distinguished from its absence comes out. A loop
  that can only conclude "keep it" is not a gauntlet.
- Everything else stays out. No file layout, no round count, no implementation. The agent
  decides those better than a spec written before the work started.

## Length and voice

Short. 150 to 220 words — longer than a generic gauntlet, because the arm and the two
numbers have to be concrete. If it needs a heading to stay readable, it is too long.

Plain sentences. No bullet lists inside the prompt.

## Two filled examples

**A rule.** User: "ladder.md is 3KB in every request and I have never seen it change
anything."

Arms offered: A) the full set minus `ladder.md` B) `ladder.md` scoped behind a `paths:`
header C) `ladder.md` rewritten to 20 lines. User picks A.

```
Find out whether rules/ladder.md earns the 3014 bytes it costs in every request of every
session.

The control arm is the full scaffolded rule set with ladder.md removed. Scaffold both arms
into throwaway roots from the same commit, prove the control is not secretly running it —
the operator's ~/.claude/rules is the leak — and run both over the same pinned ticket set
against a real repo at a fixed commit.

Break this into the smallest pieces that can be judged on their own, one ticket class per
piece. For each, fan out a builder and a separate critic with fresh context. The critic
reads the two transcripts and the two diffs blind with the arm labels stripped, says which
session reached for the smaller thing that worked, and names the single biggest remaining
gap.

Score both halves: bytes preloaded, and whether the ticket ended with scc validate clean,
boxes ticked and requirements cited. Check the invariants in `.claude/rules/project.md` first.

The critic should be a harsh critic. Praise is not useful.

/loop until the critic picks the arm carrying ladder.md blind, or until it is clear it
cannot be told apart from its absence — in which case say so and take it out.

Budget: 16 sessions, stop at $60.

Keep a live progress page updating as the work evolves so I can watch it.

Fan out subagents and ultracode.
```

**A CLI surface.** User: "make map --next cheap enough that nobody ever rereads the plan."

Arms offered: A) the same questions answered with `Read` on the whole plan B) today's
`map brief` plus `--next` C) a single command returning both. User picks A.

```
Cut what it costs an agent to work a plan through scc map, and prove the saving against
the thing it replaced.

The control arm is an agent answering the same questions by reading plans/*.md with Read.
Run both arms over the same pinned plan and the same question set — what is next, what is
blocked, what does task 1.7 depend on, what changed — in throwaway roots from the same
commit.

Break this into the smallest pieces that can be judged on their own, one subcommand per
piece. For each, fan out a builder and a separate critic with fresh context. The critic
runs both arms, compares the answers blind with the arm labels stripped, says which one a
session could act on without opening the file, and names the single biggest remaining gap.

Score both halves: tokens per answered question, and whether the answer was right — a
cheaper command that sends the agent back to the file has cost more, not less. Keep the
0/1/2 exit contract and make check green; --json stays compact and the human listing stays
readable.

The critic should be a harsh critic. Praise is not useful.

/loop on each subcommand until the critic picks ours blind. Do not stop before that.

Budget: 12 sessions, stop at $40.

Keep a live progress page updating as the work evolves so I can watch it.

Fan out subagents and ultracode.
```

## What breaks an scc gauntlet loop

- **A leaking control.** The baseline secretly runs the thing and every arm ties. ponytail
  shipped a whole draft this way. Test the isolation before trusting a number out of it.
- **Judging the rule instead of the run.** A critic that reads a rule and says it reads well
  has measured prose, not behavior. It must read two transcripts.
- **One number.** Cost alone deletes the methodology; effect alone grows it without limit.
- **The builder judging its own work.** Separate agent, fresh context, arm labels stripped.
- **A soft critic.** Binary job: which session produced better work, A or B. Scores out of 10
  drift upward every round.
- **An exit that can only say "keep it."** If the loop cannot conclude "take it out", it is
  an argument with extra steps — which is exactly the standing this port set out to stop
  accepting.
- **A named exit after N rounds.** The exit is winning blind, being told apart from absence,
  or the budget ceiling. Never a round count.

## Portability

`/loop` and `ultracode` are Claude Code features. For any other agent, swap the last two
lines for: "Keep looping until the critic picks the arm carrying the change. Run the
builders and critics as parallel subagents." The structure carries over unchanged.
