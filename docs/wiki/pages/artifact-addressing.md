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

## The reading surface is what gives "never open the plan" its authority

Forbidding the read without offering the equivalent query produces an agent that
disobeys the rule, correctly — so the surface shipped in the phase before the rule
did. `map brief` is the header, `map tasks` is the checklist, and no command returns
both, so a session pays `brief` once and `--next` per task.

`--next` is **determined** rather than first-in-file: eligible tasks first, then
priority ascending with absent last, then the number compared *numerically* — which
is also the fix for `1.10` sorting before `1.9`. `--ready`, `--blocked` and `--deps`
share that one implementation, because two notions of eligibility would be two
answers to *what do I work on*.

**`--next` prints the task whole; the listings clip.** Every task in a real plan runs
past one line, and the line below the checkbox is usually where the decision sits, so
a `--next` that stopped at the line break sent the reader back to the file — the exact
cost this surface exists to remove. It therefore ignores `--width`. A list of sixty
one-line tasks is still a list, so the listings clip: `Task.Continuation()` is the raw
lines with the flags removed, and `Task.Detail` is the same text collapsed to one
line, right for a row in a table and wrong for anything meant to be read.

## No search engine

`map find` ranks the workspace with BM25 in the binary, and the obvious reach for it
was an inverted index. At 352KB and 94 artifacts a linear pass ranks the whole corpus
in 55ms, and Tantivy or its kin would cost a CGO surface or a second binary against a
stdlib-only `go.mod` and a six-platform cross-compile.

What precision needed was not a better index but a better **unit**: BM25 over
addressable regions rather than lines, so a hit comes back as something `show`
accepts. The seam is `artifact.Search` — it takes artifacts and returns hits, and
nothing outside that file knows how it found them.

The command is now **undocumented rather than removed**. Its stated reason was the
whole corpus, not the plan, and with the plan small, searching *inside* one stopped
making sense — while searching the knowledge base is still the only alternative to
opening a file. Deleting the code would save nothing; deleting the line from the
rules and the entry file saves tokens in every request of every session.

## The seal is tamper-evidence, not prevention

`scc plan approve` validates, then writes `status: approved` and a `checksum:` over
the file minus its own checksum line, LF-normalized. `reseal --force` is one command
away and sha256 is public, so this is recorded as evidence rather than as a guarantee,
in case somebody later builds one on it.

The check runs **before an edit is applied**, which is the whole value: a harness that
edited by hand and then ran `patch check` would otherwise have its edit resealed by
the command that should have reported it. A plan with no `status:` is never checked,
which is what lets every pre-existing plan keep working.

After approval the work is fixed and only discovery moves. `add` allocates the number
from a high-water mark that includes removed tasks — so nothing is stored anywhere —
and demands `--reason`; `rm` strikes the task out where it stands rather than deleting
it; rewriting a task or the prose is refused. What discovery can never touch is
guaranteed **structurally** rather than by instruction: `Why`, `Out of scope`, `Done
when` and the title are reachable only through `append`, `prepend` and `replace`, and
those are exactly the three that are refused.

`scc plan migrate` moves a v1 plan across: it renames `Decomposition` to `References`,
moves every other heading into `plans/archive/<name>-notes.md`, creates the missing
required sections **empty and lets the findings appear** — a placeholder that satisfied
the validator would be a plan that lies — and writes `status: draft`, never `approved`.

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
