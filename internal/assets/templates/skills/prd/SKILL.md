---
name: prd
description: Turn an initiative that spans more than one feature into a plan under plans/ — a decomposition where each leaf is either a task you will do here or a reference to a spec that will be built separately. Use it when someone arrives with a PRD, a roadmap item, an epic, or a rough idea too large for one spec, and the first job is to find out what it actually decomposes into. For a single feature whose shape is already clear, skip this and run `scc spec new`.
---

You take an initiative that is too big for one spec and turn it into a plan: one
file under `plans/`, holding the decomposition and nothing else.

**A note on the word.** "PRD" is what you arrive with — a product document, a
roadmap line, a paragraph in a chat. It is not an artifact this project keeps. The
artifact is a **plan**, and the vocabulary in
`{{.Rules}}/routing.md` is what the rest of the workspace uses. Do not
create a `docs/prd/` directory.

## First, check this is a plan at all

`{{.Rules}}/routing.md` asks one question, and it is worth asking
before you start: does this need requirements and a design settled before any code?

- **Too large for one spec** — decomposes into several. That is a plan. Continue.
- **One feature, requirements not yet settled** — that is a spec. Run
  `scc spec new <feature>` and write `requirements.md`. Do not wrap one feature in a
  plan for ceremony.
- **Small enough that the *what* was never in doubt** — that is a plan too, but a
  bare checklist. Skip the interrogation below and write the list.

## Then find out what it really is

An initiative arrives underspecified. Your job before writing anything is to make it
specific enough to decompose, and the fastest way there is **a small batch of
concrete multiple-choice questions**, asked once — not a long interview, and not
guessing:

1. **Who is this for, and what can they not do today?** An initiative that cannot
   name the user is not ready to be decomposed.
2. **What is in and what is explicitly out?** The out-list is the more valuable
   half; it is the one that prevents the third spec from growing a fourth.
3. **What does "done" mean for the whole of it?** Not per feature — the condition
   under which the initiative is finished and the plan can be closed.
4. **What is already decided and not up for discussion?** A deadline, a vendor, an
   existing system that has to keep working.

Ask them together, offer concrete options rather than open prompts, and stop asking
once you can name the leaves. **If a question's answer would not change the
decomposition, do not ask it.**

## Decompose

```
scc plan new <name>
```

Then fill it in. The rules the file must hold to:

- **A leaf is either a spec reference or a task with a checkbox — never both.** Where
  an item is a task, the box is its state. Where it references a spec, the state
  lives in that spec and is read from there. Two records of one fact disagree, and
  the copy is the one that goes stale. `scc validate` reports this as
  `plan.item-has-two-records`.
- **A leaf that is a spec is an ordinary spec.** `specs/<feature>/` is not nested
  under the plan and is built by exactly the same rules as one somebody asked for
  directly.
- **Each spec-sized leaf is one coherent feature** — something a person could
  describe in a sentence and verify on its own. If a leaf needs three sentences and
  an "and", it is two leaves.
- **Say what order they go in, and what depends on what**, under `## Notes`. This is
  the part only you know right now, and the part the next session most needs.
- **A plan is work, not knowledge.** It lives in `plans/`, never in `docs/`.

Referenced specs must exist, or `scc validate` reports `plan.unknown-spec`. Create
them with `scc spec new` as you name them — an empty spec directory is a real
placeholder; a reference to nothing is a broken plan.

## Before you hand it over

- `scc validate` — exit 0, or fix what it names.
- **Read the leaf list back as a whole and ask what is missing.** Migration,
  backfill, the switch-over, the thing that has to keep working while this ships,
  and how it gets turned off if it goes wrong. Decompositions fail at the seams, not
  in the middle of a feature.
- **Check the vocabulary.** Every term the plan coins is a term three specs will
  inherit. If any of them is contested or new, use the `glossary` skill now, while
  it costs one edit.
- **A choice made here that is hard to reverse is an ADR**, not a line in the plan's
  notes. Use the `adr` skill.
