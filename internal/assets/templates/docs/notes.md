# Notes

Every small durable observation about this project, one per line. The gotcha, the
why-not, the "careful, this looks wrong and is not" — the things that used to be a
comment beside the code, where only the reader already looking at that line ever
found them.

**Notes do not live in the code.** A comment says what a thing is and how to use
it; anything else is a note, and it belongs here with the path it is about
attached to it.

One note is one line, index fields first:

```markdown
- n-0000 2026-02-09 #gotcha @internal/cli/launch.go — wrap writes MCP config to the agent's own file, so it outlives the session
```

`n-0000` is the id; real ones start at `n-0001` and are never reused, so a note can
be cited as `n-0042` from anywhere — including the one place code may still mention
one. Then the date. Then `#tags`, at least one, which is what the log is queried by.
Then `@paths`, repo-relative, which is what keeps a note attached to the code without
living inside it. Then an em dash, and the note itself.

The one line is the contract: a match is a whole note, never a fragment of one, so
`grep ' #gotcha ' docs/notes.md` and `scc notes find --tag gotcha` answer the same
question. Write with `scc notes add "…" --tag gotcha --path <path>`, which
allocates the id and gets the format right; read with `scc notes find`,
`scc notes tags`, `scc notes show n-0001`.

**If it needs a second line, it is not a note.** Something learned and worth
explaining is a `wiki/` page. A decision that is hard to reverse is an `adr/`
record. Something that has to be done is a task in a plan. A note is the thing
none of those three would take.

<!-- Delete this guidance once the log below reads for itself: until then a grep
over this file answers with the example above as well as with the notes. -->

## Log

<!-- Notes go below, oldest first. `scc notes add` appends here. -->
