# Methodology — per task, Unit by default

Every task is built one of two ways, and the annotation on its line says which. The
choice is per task, not per project: a feature routinely has both.

## Before either cycle — find the tests that already cover this

**Identify the existing tests that exercise the paths this task will change, and run
them, before writing anything.** Name them, in one line, before you start.

This precedes both cycles, because it is about not breaking what works rather than
proving what is new. It is here because of a measured result: giving an agent a
test-first *procedure* while leaving it ignorant of which tests cover the code being
modified **increased regressions above doing nothing at all** — roughly 9.9% against
~6.1%, where identifying the covering tests first brought it to ~1.8%. The failure it
prevents is specific: a task adds a case to a function whose existing tests nobody
looked at, the new test passes, and the old behavior silently changed.

## Unit — the default

Write the code, then a unit test for **each function** in it. **There is no RED/GREEN
here** — no failing-test step to observe, because the code already exists. Two
conditions make this legitimate rather than a shortcut, and both are load-bearing:

- **Immediately, per function — never at the end.** Finish a function, test that
  function, move on. The evidence supports test-last only in this iterative form.
- **The test comes from the requirement, not from the code.** This is where an agent
  fails differently from a human: a human writing tests late writes too few, an agent
  writes tests asserting *what the code does* — green tests that faithfully encode the
  bug. Read the requirement the task cites and assert that.

The default because most code is plumbing: the shape is not in doubt, and tests
written straight after are just as binding. Those two conditions are the entire risk
of writing code first, and `scc` can check neither — you are accountable for them.

## TDD — RED/GREEN required

Write the failing test first, **watch it fail**, then make it pass, then refactor.
Skipping RED is not TDD: a test that has never failed has not been shown to test
anything. The commit that adds the failing test *is* the record that you watched it
fail — do not also write it down. RED/GREEN belongs to TDD and only to TDD: Unit is
not a lazier TDD, and TDD is not a stricter Unit.

**Mandatory when the cost of being wrong is high:** money, in any calculation,
rounding, split, or conversion involving currency · complex algorithms, whose
correctness is not obvious by reading them · hypothesis validation, code written to
prove something holds · anything else where the complexity is real and the chance of
being wrong is high.

The trigger is risk, not size. A three-line rounding helper that touches money is TDD;
a two-hundred-line CRUD handler is Unit. A `(TDD)` task is also one to surface before
it lands, even in an automatic run — see [autonomy.md](autonomy.md).

Neither cycle ends at a passing test. Run the scoped tests **and** the lint before you
call the task done: [verification.md](verification.md).
