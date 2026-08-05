# Task grammar

One grammar governs every task line, in a spec's `tasks.md` or a plan's checklist.
The methodology is a property of the task, not of the vehicle that carried it.

```
- [ ] 1.1 (Unit) Parse the manifest file — R1.2, R1.4
- [ ] 1.2 (TDD) Calculate the pro-rata split — R2.1
  _Depends 1.1_
```

- `- [ ]` / `- [x]` — the checkbox is the state. Nothing else records it.
- `1.1` — `<group>.<item>`, unique, and never reused once it has been handed out.
- `(Unit)` or `(TDD)` — **required, exactly one.** A task with no methodology is a
  task where nobody decided, which is the failure this practice exists to prevent.
  `scc` exits `2` on a task missing it.
- The description, in the imperative, then `— R1.2, R1.4` — the requirements it
  satisfies. Required in a spec's `tasks.md`: that citation is traceability.

## Flags

Four, at most one of each, on their own lines under the task — and no others: an italic
line that is not one of these is a finding rather than prose. `_Depends 1.1, 1.2_` all
of them ticked before this can start · `_Priority 2_` a whole number 1 or greater, lower
is more urgent, absent is last · `_Status removed_` struck out, the line and the number
stay and the work does not · `_Reason …_` required with `_Status removed_`, and on a
task added after approval.

`_Status_` never restates the box — two records of one fact disagree. There is no
`_Blocked_` (derived from `_Depends_`) and no parallel-dispatch marker; implementation
is sequential, see [delivery.md](delivery.md) for why and for the parallelism that
*is* supported. **`scc map tasks <artifact> --next` is the order** — eligible first,
then priority, then number; `--blocked` names what an impasse waits on.

## How big is a task

**A task is the right size when it can be verified on its own.** Not "one file", not
"an hour" — verifiable alone, which is what makes the per-task loop in
[verification.md](verification.md) possible at all. Granularity decides what a failure
costs, not tidiness: agents complete individual steps far more reliably than whole
workflows, and retrying at the subtask level cut retry cost by ~73% against retrying a
whole plan. Too coarse and a red result says only that a feature is broken; too fine
and the checklist is bookkeeping.

## Two checklists, one truth

Your harness's todo list tracks the task you are on right now. The file — a spec's
`tasks.md`, or the checklist in `plans/<name>.md` — is the durable record: it survives
the session, it gets reviewed, it gets committed, and it is what `scc` validates.
**Checking an item off in the session means checking the `- [ ]` box in the file too.**
A session ending with its todo list complete and the file untouched has lost everything
except the code: neither the next session nor the reviewer knows which tasks were done.
Use `scc patch check <artifact> 1.2` rather than an editor — it addresses the task by
number, so the file is never read to change one box, and it re-validates afterwards.
See [artifacts.md](artifacts.md).
