# Artifact addressing

`scc map` reads a piece of an artifact and `scc patch` changes one, both without the
caller ever opening the file. The whole design is in what they take as an argument.

## An address is a name, never a line number

```
1.2          a task              #risks      a section, by anchor slug
R1.2         a requirement       risks:2     the 2nd paragraph of that section
specs/foo/   a spec reference    L120-160    an explicit range — the escape hatch
```

A line number stops being true the moment anything above it moves, so an editor that
addresses by line has to read the file first — which is the cost this package exists
to remove (see [[context-budget]]). A name survives an edit above it, so it can be
resolved against a file nobody has read.

`L120-160` is the one form that is a line number, and it is documented as the escape
hatch rather than as an equal.

## What replaces reading first

Reading before editing was providing a guard, and three stronger ones replace it for
a structured file:

1. **An address that does not resolve is an error**, never an insert at a guess.
2. **The file is re-validated afterwards and rolled back** if the edit introduced a
   finding the file did not already have — exit `2`, file untouched. The comparison
   is on `rule + message` and deliberately *not* on line number, because an
   insertion moves every finding below it and comparing on line would blame this
   edit for the whole tail of a pre-existing problem.
3. **The displaced and written lines are printed back**, elided past a few lines —
   a confirmation that echoed 400 lines would put the file in context by the back
   door.

An artifact `scc` has no validator for is written and *reported as unverified*,
rather than silently claimed clean.

## Two things measurement decided

**`blocks` exists because section addressing bottoms out.** One measured plan's
`## Notes` ran to 411 lines — half the file — with no headings inside it, so there
was nothing to address. Every paragraph opened with a bolded thesis, so the leads
alone are an index a twentieth of the size.

**A requirement id is scoped to its spec.** `R2.5` was defined in nine of one
workspace's 31 specs, so `scc map trace R2.5` unscoped answers with the list of
specs and stops, rather than concatenating nine traces.

## The grammar lives with the reader, not the checker

`internal/artifact` owns every grammar — task, requirement, spec reference, flag —
and `internal/validate` consumes it. The task grammar used to live in the validator,
and a reader that disagreed with the validator about what a task *is* would be worse
than no reader at all. The parser now states facts about a line (`Methodologies`,
`Loose`, `HasCitation`); turning a fact into a finding stays in the validator. That
split is also what lets `map` read a malformed artifact instead of refusing exactly
the file a user most needs to inspect.

## An address is hostile input

`artifact.Resolve` accepts a path relative to the working directory — that is what
makes `scc map show tasks.md` work from inside a spec directory. Until
`artifact.Within` was put in front of it, it was also what let
`scc patch append ../outside.md L3 --text X` write outside the repository and exit
`0`. The boundary belongs in `internal/artifact` rather than in a command handler,
because an address reaches `scc` from a file the agent read as readily as from the
user, so every command that resolves one needs it. Symlinks are evaluated on both
sides before the comparison.

## Related

[[context-budget]] · [[spec-or-plan]] · [[validation-contract]]
