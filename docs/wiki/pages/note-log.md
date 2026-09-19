# The note log

A comment says what a thing **is** and how to use it. Everything else that used to be
written beside code — the gotcha, the why-not, the *careful, this looks wrong and is
not* — reaches exactly one reader: the one already looking at that line. Nobody asking
*what do we know about this area* ever finds it, no command lists it, and it dies with
the file.

`docs/notes.md` is where that goes instead, and `scc notes` is how it is queried.

## One note is one line

```
- n-0042 2026-02-09 #gotcha @internal/cli/launch.go — wrap writes MCP config to the agent's own file
```

Index fields first, and the single line is the entire design. **A match is a whole
note**, so `grep ' #gotcha ' docs/notes.md` and `scc notes find --tag gotcha` answer
the same question without either one reasoning about where a record ends. That is what
lets the file centralizing every note never be a file anybody reads.

It also closes the door the v1 plan format left open, where `## Notes` grew to half
the file because nothing forbade it: here there is **nowhere for prose to go**. A
thought needing a second line is a wiki page, an ADR, or a task — all three of which
already exist, and the rule says which is which. See [[spec-or-plan]].

## The CLI is the writer, never a gatekeeper

`scc notes add` allocates the id and gets the format right, because a format nobody
can be made to type is one that decays. `find | show | tags | paths | rm | validate`
is the rest, and what it adds over grep is the questions a substring cannot answer:
which tags exist before somebody coins a fourth name for one concern, what this
project already knows about a path, what is new since a date.

`--tag` is required and has no default. A default would be one tag on everything,
which is the drift the index exists to prevent.

## A number is spent, never reused

`scc notes rm` takes the text out and leaves an HTML-comment tombstone where the note
stood, so a citation to `n-0042` can dangle but can never come to mean a *different*
note. `mdscan` blanks comments, so the tombstone is invisible to a rendered read, to a
grep for a tag, and to every parser except the one allocating the next id.

## `@path` is the stale check

A note about code that no longer exists is read as current, which is worse than the
comment it replaced — that one at least died with the file. So a path is checked, and
the ninth validator reports the failure this file cannot tolerate quietly: a
hand-written line that missed the grammar, which no query will ever return.

At *write* time an unresolvable path is a warning and never a block, because a note
about a file this branch has not created yet is the note most worth having. Otherwise
`notes add` writes under the same verify-and-roll-back contract as `scc patch` — see
[[artifact-addressing]].

## Related

[[artifact-addressing]] · [[validation-contract]] · [[spec-or-plan]]
