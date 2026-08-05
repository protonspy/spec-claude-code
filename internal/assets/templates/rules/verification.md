# Verification — closing the loop on every task

A task is not done when the code is written. You built it, so you finish it, in this
order:

1. **Build the task** — Unit or TDD, per the annotation on the task line
   ([methodology.md](methodology.md)).
2. **Run the tests in its scope** — the tests covering what you just built, plus the
   ones you identified as already covering these paths. Not the whole suite.
3. **Run the lint** — the project's linter (see [project.md](project.md)).
4. **Fix, or move on.** A red task is not finished. Then tick the box in the file.

## Scope, not suite

Per-task feedback has to be fast and attributable. The failure a full run catches
and a scoped run misses is breakage *between* tasks, and that is worth looking for
once the work is integrated — not N times along the way.

The full suite runs at the end of the spec, or of each of a plan's groups. See
[delivery.md](delivery.md).

## Tests and lint both, because they answer different questions

- The **tests** say the code does what the task asked.
- The **lint** says the code is written the way this project writes code — unused
  code, unchecked errors, shadowed variables, unsafe conversions. These are
  programming defects tests do not find.

A task that passes its tests with an unchecked error in it is not finished. Neither
check substitutes for the other.

The linter is whatever this project already uses. `scc` does not own it and does not
run it, for the same reason it does not read your source.

## And the artifact

If the work changed what a spec says — and work that touches an area a spec covers
usually does — update that spec in the same branch, as a delta. See
[specs.md](specs.md).
