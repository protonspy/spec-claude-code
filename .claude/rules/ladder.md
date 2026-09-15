# The ladder — how much code a task gets

[methodology.md](methodology.md) says how a task is built and tested, and which tests
already cover it. Name those first. Then, before the code, stop at the first rung:

1. **Already here?** A helper, a type, a pattern this repo has — reuse it. Ask
   `scc graph explore "<what you were about to build>"` before you write: the thing
   three files over is what the graph exists to find ([code-search.md](code-search.md)).
2. **The standard library does it?** Use it.
3. **A native platform feature covers it?** `<input type="date">` over a picker
   library, CSS over JS, a database constraint over application code.
4. **A dependency this project already carries?** `docs/stack.md` is that list, and an
   answer rather than a guess. A *new* one is an ADR and a `stack.md` entry
   ([knowledge-base.md](knowledge-base.md)), never something reached for mid-task.
5. **One line?** One line.
6. **Only then:** the least code that satisfies the requirement the task cites.

Two rungs hold, take the higher one. Two options the same size on one rung, take the
one that is right on the edge cases: short is the tiebreak, never the target.

**The ladder shortens the solution, never the reading.** It runs after you understand
what the change touches: a small diff in the wrong place is not lazy, it is a second
bug wearing efficiency as a disguise. A report names a symptom, so ask `scc graph
explore "what calls <name>"` and fix the shared function once — one guard where the
callers route through is a smaller diff than one per caller, and patching only the
path the report named leaves every sibling caller broken.

## What the ladder never touches

The line is what the code has to survive, not taste. Cutting one of these is not
laziness, it is damage:

- **Validation at a trust boundary** — from a user, a file, a request, a service.
- **Error handling that prevents data loss**, on the failure path of anything writing.
- **Security and accessibility.** The shortest path join lets `../../` out of the tree.
- **Anything the requirement asks for**, and the check the task owes ([verification.md](verification.md)).

## The requirement is not a rung

The spec decides *whether* a thing exists; this rule decides only how much code it
takes. Quietly building less than `R1.2` says is the one failure this rule can cause,
and it ships as a small clean diff, which is why nobody catches it. A requirement that
looks over-built is a spec delta ([specs.md](specs.md)) or a checkpoint
([autonomy.md](autonomy.md)) — raised in writing, never settled by building less.

## Mark a corner you cut

A simplification with a known ceiling — a global lock, an O(n²) scan, a naive
heuristic — is fine, and it is a note rather than a comment ([notes.md](notes.md)):

```
scc notes add "global lock; per-account locks if throughput matters" --tag ceiling --path internal/x.go
```

Name the ceiling and what lifts it. One nobody wrote down is indistinguishable from a bug.
