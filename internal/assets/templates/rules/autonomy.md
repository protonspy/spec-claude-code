# Autonomy — ask once, at kickoff

The spec phases are **autonomous by default**: write requirements, design, and
tasks, then start implementing. Do not stop for approval at each phase.

But autonomy is the user's call, so **ask, once, before writing anything** — three
questions, together, in the same breath:

1. **Run automatically, or gate each phase for review?**
2. **When the PR is open, wait for CI, or finish there?** Asked now because by then
   the work is done and the user may be gone, which is when a block costs the most.
3. **Answer in English, or in 文言文?** Classical Chinese at maximum terseness. The
   advertised saving counts characters, not tokens. Governs speech, not artifacts.

**Nobody there to answer? Do not spend the turn asking.** A headless run, a CI job, an
unattended session: the question has no reader, and a turn that ends holding one has
delivered nothing — the one outcome kickoff was never meant to produce. Take `auto`,
`no-wait` and the user's own language, record them as assumed, say so in one line, and
build. A later session changes any of the three with `scc patch fm`.

Record the answers in the artifact's frontmatter — `requirements.md` for a spec, the
plan file itself for a plan — then never ask again for this piece of work:

```yaml
---
autonomy: auto      # or: gated
ci: wait            # or: no-wait
lang: en            # or: wenyan — omit to mirror the user
---
```

`scc spec new <feature> --autonomy=auto --ci=wait` writes the first two;
`scc patch fm <artifact> lang=wenyan` writes the third without opening the file.

Recording them is what makes the run reproducible from the file and what stops a
second session from re-asking. Ask in conversation rather than reading a flag,
because the person who has to make the call is in the conversation.

## `auto` is not "never stop"

Automatic keeps one exception. **The risk that mandates TDD also warrants a
checkpoint:** a task you annotated `(TDD)` because it touches money, a complex
algorithm, or a hypothesis being validated is precisely the task worth surfacing
before it lands. Surface it, briefly, even in an automatic run.

One classifier, two consumers — it picks the methodology (see
[methodology.md](methodology.md)) and it picks what deserves a human glance. There
is deliberately no second risk taxonomy for gating; two lists would drift apart.

## `gated`

Stop after `requirements.md`, after `design.md`, and after `tasks.md`. Present what
you wrote and wait. Do not start implementing until the phase you are on is
approved.
