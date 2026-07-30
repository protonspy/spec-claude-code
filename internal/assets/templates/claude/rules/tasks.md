# Task grammar

One grammar governs every task line, whether it sits in a spec's `tasks.md` or in a
plan's checklist. The methodology is a property of the task, not of the vehicle that
carried it.

```
- [ ] 1.1 (Unit) Parse the manifest file — R1.2, R1.4
- [ ] 1.2 (TDD) Calculate the pro-rata split across accounts — R2.1
- [ ] 1.3 (Unit) Render the summary table — R3.1
```

- `- [ ]` / `- [x]` — the checkbox is the state.
- `1.1` — a unique number, `<group>.<item>`.
- `(Unit)` or `(TDD)` — **required, exactly one.** A task with no methodology is a
  task where nobody decided, which is the failure this whole practice exists to
  prevent. `scc` exits `2` on a task that is missing it.
- The description, in the imperative.
- `— R1.2, R1.4` — the requirements this task satisfies, after an em dash. Required
  in a spec's `tasks.md`; that citation is what makes traceability checkable.

There is no parallel-dispatch marker. Implementation is sequential — see
[delivery.md](delivery.md) for why, and for the parallelism that *is* supported.

## Requirement IDs

Requirements are numbered `R<group>.<item>` in `requirements.md` and cited by that
ID. It greps cleanly, it never collides with a task's own number, and a reader who
has never seen this document can follow it.

## How big is a task

**A task is the right size when it can be verified on its own.** Not "one file", not
"an hour" — verifiable alone, which is what makes the per-task loop in
[verification.md](verification.md) possible at all.

Granularity is not tidiness; it decides what a failure costs. Agents complete
individual steps far more reliably than whole workflows, and structuring work so a
failure can be retried at the subtask level cut retry cost by ~73% against retrying
a whole plan.

- **Too coarse** and a red result tells you only that a feature is broken.
- **Too fine** and the checklist becomes bookkeeping about work smaller than the act
  of recording it.

## Two checklists, one truth

Claude Code's own todo list is where you track the task you are on right now. The
file — `specs/<feature>/tasks.md`, or the checklist in `plans/<name>.md` — is the
durable record: it survives the session, it gets reviewed, it gets committed, and it
is what `scc` validates.

**Checking an item off in the session means checking the `- [ ]` box in the file
too.** A session that ends with its todo list complete and the file untouched has
lost everything except the code: the next session does not know which tasks were
done, and neither does the reviewer.
