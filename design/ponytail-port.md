---
autonomy: gated
ci: wait
lang: en
status: draft
---

# Porting ponytail into scc

[ponytail](https://github.com/DietrichGebert/ponytail) is two things: a ~120-line prompt
that makes an agent reach for the smallest thing that works, and ~760 lines of hooks that
keep it in front of the model. The prompt ported in two pull requests. This plans the rest —
the hook surface, and the measurement that made anyone believe the prompt to begin with.

## Why

A rule holds until the session that does not re-read it. scc has already made that argument
twice — it is why `scc hooks` exists at all, and why the tenth validator turned a line in
`delivery.md` into a check. ponytail makes it a third time from the other end: it assumes
drift, re-asserts on every prompt, and re-asserts again inside every subagent. scc asserts
once, at session start, on one harness, and never looks again. It can do better than
re-assert, and that is the one place this port improves on its source: ponytail's drift is
over-building, which has no shape and cannot be detected, while scc's drift is methodology —
code written with no box ticked, a `TODO` left in source, a commit on `main` — and the
artifacts are the evidence, already on disk, in a range `internal/git` already computes.
Whether any of it is happening is not known here, which is the second half of the problem:
fifteen rules, ~870 lines preloaded into every request, and
`TestRulesStayShortEnoughToBePreloaded` is the only thing that has ever measured them — and
it measures cost. ponytail rests on a headless agent editing a pinned
public repo, four arms including the critic's own seven-word prompt, and a contamination bug
it found in its own numbers and published. This plan is done when scc can say which of its
rules would be missed if deleted, and has been allowed to act on the answer.

It lives in `design/` for the reason `design/plan.md` does: this repo is not an scc
workspace, so there is no `plans/` to put it in and no seal — `status:` here is prose until
scc can scaffold itself.

## Paths

- `benchmarks/` — new; the harness, the arms, the scorer
- `internal/hooks/harness.go` — the event set and its contract
- `internal/cli/hooks_agent.go` — `stopContext`, where a drift line would be written
- `internal/cli/hooks.go`
- `internal/git` — the `base..HEAD` range the drift stage reads
- `internal/paths/harness.go`
- `internal/assets/assets_test.go` — where the carve-out canary goes
- `internal/assets/templates/rules/ladder.md`
- `internal/assets/templates/rules/notes.md`

## References

- https://github.com/DietrichGebert/ponytail — the source, v4.10.0
- `hooks/claude-codex-hooks.json` there — `SessionStart`, `SubagentStart`,
  `UserPromptSubmit`; scc registers the first and `Stop`, and nothing else
- `benchmarks/results/2026-06-18-agentic.md` there — the method being ported, limitations
  section included, which is the part most worth imitating
- PR #41 — the graph-subcommand fix and the command canary
- PR #42 — `rules/ladder.md`
- `docs/rule-refinements` — the five rule fixes the same audit turned up, already committed
- `design/orchestration.md` §6 — the rule set and what each rule is for

## Out of scope

- The `lite | full | ultra` dial. `caveman.md` already settled this for scc: three
  descriptions of one register are three things to keep true, and nobody turns the dial.
  ponytail pays for it in `filterSkillBodyForMode`, a regex over its own Markdown shape that
  has already eaten a rule by accident.
- A Node runtime and a per-host state file. Every scc hook is three lines calling back into
  `scc hooks run <stage>`, so the decision lives in Go, where it is tested and where
  upgrading the binary changes the gate. A second language on the hook path would be a
  second place for it to rot.
- The twenty-host adapter tree and its rule copies. scc renders per harness from one source,
  which is the generated form of what `check-rule-copies.js` verifies by hand.
- ponytail's ultra register — "ship the one-liner and challenge the rest of the requirement".
  Under spec-driven development the requirement is the contract and the task cites it;
  questioning it belongs to `prior-art.md` and `specs.md`, at a moment that has passed.
- The `ponytail:` code comment. `notes.md` had already argued the other way, and `ladder.md`
  ships the note instead.

## Tasks

- [x] 1.1 (Unit) Route the graph's relationship questions to a subcommand that exists
- [x] 1.2 (Unit) Fail the build when scaffolded guidance names a command that does not exist
- [x] 1.3 (Unit) Scaffold `rules/ladder.md` — six rungs, the safety carve-outs, and the
  boundary that keeps the requirement off the ladder
- [ ] 2.1 (Unit) Seed `ceiling` in the note-tag vocabulary so `ladder.md` and `notes.md`
  agree — `ladder.md` tells the agent to type `--tag ceiling` while `notes.md` says to reuse
  an existing tag, and `scc notes tags` in a fresh workspace answers with nothing
  _Priority 1_
- [ ] 2.2 (Unit) Pin the ladder's carve-outs with an invariant canary — ponytail's
  `INVARIANTS` list, ported: assert each of the four survives verbatim, so a reword can drop
  one only out loud
  _Priority 1_
- [ ] 2.3 (Unit) Restore the edge-case rung — "two stdlib options the same size, take the one
  correct on edge cases" was cut from `ladder.md` for the line budget, not on merit
  _Priority 3_
- [ ] 2.4 (Unit) Settle whether `delivery.md` means to run the suite twice. Step 1 is `scc
  check` and step 2 is `scc validate --checks`, which adds the same four gates — so the
  sequence as written builds, formats, lints and tests twice on the way to one PR. Either
  step 1 goes or step 2 drops `--checks`, and which one is a question about intent rather
  than about wording. Found by the audit this port prompted; it has no other home
  _Priority 2_
- [ ] 3.1 (Unit) Establish whether a subagent inherits the preloaded rules, per harness, and
  record the answer where the next session can find it. Everything below turns on it, and it
  is a property of the harness rather than of scc — the question `PreloadsRules` already
  answers for the main session, asked one level down
  _Priority 1_
- [ ] 3.2 (Unit) Register `SubagentStart` as a third stage of `scc hooks run`, if 3.1 says
  the methodology is absent there. Emit the entry file's routing table and not the rules:
  ponytail injects its whole skill because it has one, and scc has fifteen — a subagent
  handed ~26KB is the cost `TestRulesStayShortEnoughToBePreloaded` exists to prevent
  _Depends 3.1_
  _Priority 1_
- [ ] 3.3 (Unit) Keep the new stage inside the contract the other two already hold: report,
  never refuse, never publish. Exit 2 would block the turn, and a hook that can block a turn
  can loop one
  _Depends 3.2_
- [ ] 3.4 (Unit) Report methodology drift at `Stop`: source changed on this branch with no
  box ticked anywhere, a `TODO`/`FIXME`/`HACK` in the diff — which `notes.md` forbids and no
  validator reads, because none of them reads source — and commits sitting on `main`, which
  `delivery.md` says work does not happen on. Each is a line only when it is true
  _Depends 3.3_
  _Priority 2_
- [ ] 3.5 (Unit) Keep it stateless: the baseline is `base..HEAD`, never a session snapshot.
  A session baseline needs a file to remember it, and this workspace has one file per
  harness on purpose — the branch is the better unit anyway, since `delivery.md` already
  makes one branch one unit of work
  _Depends 3.4_
  _Priority 2_
- [ ] 3.6 (Unit) Prove the drift stage stays silent on a clean branch, in a test rather than
  by inspection. A stage that speaks every turn is one the reader learns to skip, which
  costs more than the drift it reports
  _Depends 3.5_
  _Priority 2_
- [ ] 3.7 (Unit) Decide `UserPromptSubmit` on evidence rather than on appetite. It is
  ponytail's anti-drift stage, and scc's rules are already routed by moment, so the honest
  version fires only when a prompt names an artifact — and a stage that guesses wrong is
  noise in every turn of every session. Answer it with the harness below
  _Depends 4.4_
  _Priority 3_
- [ ] 4.1 (Unit) Pin a public target repo and a ticket set, at a fixed commit, in
  `benchmarks/`. Tickets rather than one-shot prompts: scc's claims are about what an agent
  leaves behind across a session, and a single completion cannot exhibit a ticked box
  _Priority 2_
- [ ] 4.2 (TDD) Isolate the arms, and test the isolation before trusting a number out of it.
  This is where ponytail's own benchmark was wrong for a whole draft — its plugin hook fired
  on the control arm, so the baseline was secretly running the skill. Here the leak is the
  operator's global configuration and any harness directory already on the machine
  _Depends 4.1_
  _Priority 2_
- [ ] 4.3 (Unit) Score a run on what scc actually claims: `scc validate` findings, the
  delivery gate's result, whether boxes were ticked in the file and requirements cited — plus
  diff LOC, tokens, cost and wall time. LOC alone would measure ponytail's thesis rather than
  this one
  _Depends 4.2_
  _Priority 2_
- [ ] 4.4 (Unit) Make a whole run reproducible from one command, and say in the README what
  it costs to run
  _Depends 4.3_
- [ ] 5.1 (Unit) Run the arms — no rules, the full set, the set minus `ladder.md`, and the
  subagent stage on and off — at n≥4, recording per-task tables and not only aggregates
  _Depends 4.4_
- [ ] 5.2 (Unit) Write it up in `design/`, limitations first: the model, the sample size,
  what the scorer cannot see, and anything found that contradicts the design
  _Depends 5.1_
- [ ] 5.3 (Unit) Act on the result — a rule or a hook stage the measurement cannot
  distinguish from its absence either earns a reason to stay or comes out
  _Depends 5.2_

## Done when

- `scc notes tags` in a fresh workspace answers `ceiling`, and the canary fails when a
  carve-out is reworded away
- Whether a subagent inherits the rules is written down per harness, and the hook stage
  exists only where the answer was no
- `scc hooks check` lists every registered stage, no stage can block or end a turn, and a
  clean branch draws no drift line at all
- One command reproduces a full arm sweep from a clean checkout
- `design/` holds a writeup with a limitations section and per-task tables, not only means
- At least one rule or hook stage has been kept, cut, or rewritten *because of* the
  measurement, and the commit that did it says so
- `make check` green, and every command this plan's artifacts name is one the dispatcher
  dispatches
