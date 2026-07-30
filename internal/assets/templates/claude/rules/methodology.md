# Methodology — per task, Unit by default

Every task is built one of two ways, and the annotation on the task line says
which. The choice is per task, not per project: a feature routinely has both.

## Before either cycle — find the tests that already cover this

**Identify the existing tests that exercise the paths this task will change, and
run them, before writing anything.** Name them, in one line, before you start.

This is not part of Unit or TDD. It precedes both, because it is about not breaking
what works rather than about proving what is new.

It is here because of a measured result. Giving an agent a test-first *procedure*
while leaving it ignorant of which tests actually cover the code being modified
**increased regressions above doing nothing at all** — roughly 9.9% against ~6.1%.
Identifying the covering tests first brought it to ~1.8%. Procedure without context
was worse than no procedure.

The failure it prevents is specific: a task adds a case to a function whose
existing tests nobody looked at, the new test passes, and the old behavior silently
changed.

## Unit — the default

Write the code, then write a unit test for **each function** in it. That is the
whole cycle. **There is no RED/GREEN here** — there is no failing-test step to
observe, because the code already exists.

Two conditions make this legitimate rather than a shortcut, and both are
load-bearing:

- **Immediately, per function — never at the end.** Finish a function, test that
  function, move on. The evidence supports test-last only in this iterative form;
  nothing supports saving the tests for the end of the feature.
- **The test comes from the requirement, not from the code.** This is where an
  agent fails differently from a human: a human writing tests late writes too few,
  an agent writes tests that assert *what the code does* — green tests that
  faithfully encode the bug. Read the requirement the task cites and assert that.
  Reading your own implementation to decide what to assert is the failure mode, not
  the method.

The default because most code is plumbing: the shape is not in doubt, and tests
written straight after are just as binding. Those two conditions are the entire risk
of writing code first, and you are accountable for them — `scc` cannot check either.

## TDD — RED/GREEN required

Write the failing test first, **watch it fail**, then make it pass, then refactor.
Skipping RED is not TDD: a test that has never failed has not been shown to test
anything. Say, in the task's notes, that you observed the red.

RED/GREEN belongs to TDD and only to TDD. Unit is not a lazier TDD and TDD is not a
stricter Unit — they are two different cycles.

**Mandatory when the cost of being wrong is high:**

- **money** — any calculation, rounding, split, or conversion involving currency
- **complex algorithms** — anything whose correctness is not obvious by reading it
- **hypothesis / thesis validation** — code written to prove something holds
- **anything else** where the complexity is real and the chance of being wrong is
  high

The trigger is risk, not size. A three-line rounding helper that touches money is
TDD; a two-hundred-line CRUD handler is Unit.

A task annotated `(TDD)` is also a task to surface before it lands, even in an
automatic run — see [autonomy.md](autonomy.md).

## After the code

Neither cycle ends at a passing test. Run the scoped tests **and** the lint before
you call the task done: [verification.md](verification.md).
