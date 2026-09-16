---
autonomy: auto
ci: no-wait
lang: en
status: result
---

# Measuring the rules, first run

`design/ponytail-port.md` shipped its hook and rule work and skipped its measurement
half, recording why: 48 agent sessions at an estimated $14–$120, against an apparatus
that produces nothing until somebody spends that. Its §Why was left standing —
*nothing measures whether the rules earn the ~870 lines they cost* — with the
instruction to restart from there rather than from the task list.

This is that restart, at a twentieth of the planned size. It answers the question for
two tickets on one target with one model, finds one defect large enough to change the
product, and says plainly where its own numbers stop.

## Limitations, before anything else

- **One model, one target, n=3.** Sonnet 5 against `github.com/dustin/go-humanize`
  pinned at `4d1d908` — 2,241 lines, stdlib-only, suite green. Nothing here transfers
  to a large codebase or another model without being run again.
- **Every run is one headless turn.** `claude -p` gives the agent nobody to come back
  to. That is what makes §3's failure visible and it is also the *unattended* case
  specifically. Nothing here says what the same rules do with a person in the session.
- **The operator's global configuration was a constant, not an absence.**
  `~/.claude/rules/context7.md` and an RTK `PreToolUse` hook were present in both
  arms. Isolating the config directory needs a copy of the credentials file, which
  was refused, so the leak was ruled out by probe (§2) rather than by removal.
- **A difference inside the spread of three runs is not a difference**, and §6 is
  named rather than acted on for exactly that reason.
- **The hidden acceptance test is a narrow notion of effect.** It says the ticket
  works. It does not say the repository is better for the change.
- **gofmt carries no signal on this target.** The pin is already not gofmt-clean
  under Go 1.26, so the column was dropped rather than reported as damage the agent
  caused. That same fact is what produced §6.

## The apparatus

Two tickets, chosen so that the methodology's own routing sends them different ways:

- **T1** — `Ordinal` is wrong for negative numbers, with the expected strings given.
  A correctness fix in a file that already has covering tests.
- **T2** — add `Cents(int64) string`, comma-grouped, two fraction digits, no panic on
  `math.MinInt64`. New public surface, and money, which `methodology.md` mandates TDD
  for.

Each run is a fresh copy of the pin, scaffolded per arm, handed the ticket verbatim
through `claude -p` with one tool allowlist. Scoring reads a hidden acceptance test
the agent never sees, the repository's own `go test`/`go vet`, the artifacts the
methodology claims to leave behind, the git state, and the runner's own cost.

Arms: **none** (bare checkout, no scc), **full** (today's `scc init --claude`),
**fixed** (full, with `autonomy.md` edited), **rule+hook** (fixed, plus one line of
`SessionStart` context; run in both the JSON and plain-stdout output forms).

## 1. The standing cost, measured

The first request of a session, full scaffold against bare checkout:

| | tokens in request 1 |
|---|---|
| bare checkout | 34,752 |
| full scaffold | 49,364 – 49,546 |

**+14,612 to +14,794 tokens in every request of every session, or +42%.** The three
scoped rules (`specs.md`, `tasks.md`, `knowledge-base.md`, 10.3KB) are absent from
that figure, which is the first direct evidence that `ScopedRules` does what it was
built to do — about 3.2k tokens per request that the unscoped set would have cost.

## 2. Isolation

The same probe, down the same code path as a real run, in both arms: *what process
does this repository require before writing code?* The control answered `NONE`. The
treatment recited kickoff, routing, prior art, the artifact set, the methodology
annotation and the delivery sequence. No leak.

## 3. T1 — the methodology arm wins blind, 3 of 3

Both arms pass the acceptance test on every seed, so effect is settled and the
critics judged what is left. Three critics, one per seed, fresh context, arm labels
stripped, each reading only its own pair. **All three picked the scaffolded arm.**
Two gave the same reason unprompted: it left durable regression tests in
`ordinals_test.go` covering cases the ticket never named, while the control wrote a
throwaway test and deleted it — *"leaving the repository with zero test coverage for
negative ordinals after the change."* That is `methodology.md`'s Unit rule, which
says the test comes immediately and from the requirement.

It cost **+56% in dollars and +41% in input tokens** to get that.

Every critic, on both tickets, independently went after the same thing nobody asked
them about: the `math.MinInt` / `math.MinInt64` path. On T1 neither arm guarded it.

## 4. T2 — the methodology arm delivers nothing, 3 of 3

| arm | acceptance | lines added |
|---|---|---|
| none | 3/3 | 31, 35, 55 |
| full | **0/3** | 0, 0, 0 |

Three seeds, one failure, three times: the scaffolded arm ended its turn asking
`autonomy.md`'s three kickoff questions and wrote no code. The control shipped the
function every time.

The rule says *ask, once, before writing anything*, and has no branch for nobody
being there — which is precisely the unattended run `autonomy: auto` exists to
describe. scc's own Go code has known this shape since the first integration: every
installer degrades on *"nobody is here to answer the install prompt"*. The rules
never learned it.

Why T2 and not T1: a bug fix with the expected output given is work the agent starts;
new public surface routes through `routing.md` first, and that is where kickoff sits.

## 5. The fix, and the one that did not work

**Editing the rule changed nothing — 0 of 3 again.** `autonomy.md` gained a paragraph
saying that an unanswerable question should not be asked, paid for by cutting the
section it made redundant. The arm behaved exactly as before, and the reason is the
useful part: **the rule asks the agent to act on a condition it cannot observe.**
From inside the turn, an unattended session looks like any other.

The harness can observe it. A hook in a `claude -p` run has
`CLAUDE_CODE_SESSION_ATTENDED=0` in its environment, measured against 2.1.273. scc
already registers `SessionStart`, so the whole intervention is one sentence of
injected context and no new mechanism.

| arm | acceptance | plan written | branch cut | mean cost |
|---|---|---|---|---|
| full | 0/3 | 0 | 0 | $0.23 |
| fixed (rule only) | 0/3 | 0 | 0 | $0.18 |
| rule + hook | **6/6** | 5/6 | **6/6** | $2.22 |

Both output forms worked. The plain-stdout form is the one Claude Code documents;
the JSON `hookSpecificOutput` form is documented for `UserPromptSubmit` only and
works here regardless, which is what scc already emits.

`CLAUDE_CODE_SESSION_ATTENDED` is **not documented** — neither the variable nor its
values — so `unattendedLines` reads exactly one spelling and says nothing for
anything else, unset included. The default is the safe one in the only direction
that matters: a missed line leaves today's behaviour, while a line in front of an
attended session would talk somebody out of a question they were right to be asked.

The security review asked the right question about it — an environment variable has
no provenance, so a hostile `.envrc` or `devcontainer.json` in a clone could tell an
*attended* session that nobody is watching, and the one checkpoint a person gets is
`gated`. That was testable and was tested: exported as `1` into the shell that
launched `claude -p`, the hook still saw `0`. **The harness sets the value rather
than inheriting it.** One direction of a symmetric mechanism is not a proof of both,
so the injected text ends by saying the conversation wins — live user text is the one
input no file in the repository can forge.

The price is the honest headline. **$2.22 a run against the control's $0.20** — an
order of magnitude — because this is the first arm in which the methodology actually
ran: prior art, a plan, TDD, the gates, a branch and a commit.

**Judged blind on the Go code alone** — diffs only, no plan, no branch, no final
message — it is **2 of 3**, which is not a result. The one critic who picked the
control is §6.

**Regression on the small ticket.** Seven runs of the three patched variants against
T1: acceptance **7/7**, diffs of 8–17 lines against the control's 8–20, at $0.16–$0.78
with five of the seven under $0.26. The expensive pipeline fires on the ticket that
routes to it, not on every ticket — which is the failure mode a fix like this one
would most plausibly have introduced, and did not.

## 6. Found, not fixed: the gate reformats what the branch never touched

One blind critic picked the control on T2 for a reason worth recording: the
methodology arm had bundled lint-driven rewrites of `bigbytes.go`, `bytes.go` and
`si_test.go` into a money-formatting change. The agents recorded their format gate
correctly as the check rather than the rewrite (`gofmt -l .`, not `gofmt -w .`) — the
guidance held. What follows is that a tree-wide gate on a repository that was already
failing it turns any task into a whole-tree reformat.

It happened in **2 of 6** delivering runs and 0 of 3 control runs. That is under the
resolution of n=3, so nothing was changed on the strength of it. The shape of the fix
is visible and consistent with discipline this codebase already has in three places —
`attribution` reads `base..HEAD`, the Stop hook intersects findings with
`git.Changed`, `spec sync` refuses to re-litigate a settled record: **a gate failure
on files this branch did not touch is somebody else's, and belongs in the PR body
rather than in the diff.** Measuring it needs n≈10 per arm.

## What this cost

24 headless agent sessions, 8 subagent critics, about $17 of an authorised $180.

## What is still unanswered

- **Fourteen of the fifteen rules have still never been told apart from their own
  absence.** This run ablated the whole scaffold, not one rule at a time. `ladder.md`,
  `caveman.md`, `prior-art.md` and the rest stand exactly where `ponytail-port.md` 5.3
  left them: argued for, never measured.
- **The attended session is unmeasured**, and it is the one most people are in.
- **§6**, which needs a bigger n than this run could pay for.
