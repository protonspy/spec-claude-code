# Notes — the code is not where a note goes

A comment says what a thing **is** and how to use it, on the declaration it belongs
to. Everything else you were about to write in the code — the gotcha, the why-not,
the "careful, this looks wrong and is not", the afternoon you lost — is a note, and
every note lives in one file: `docs/notes.md`.

Not a style preference. A comment reaches one reader, the one already looking at
that line. Nobody asking *what do we know about this area* ever finds it, no command
lists it, and it dies unread with the code it sat beside.

**No `TODO`, `FIXME`, `HACK`, `NOTE` or `XXX` in code, and no commented-out code.**
Something to do is a task in a plan; something to know is a note here.

## Writing one

Never by hand — `add` allocates the id and gets the format right:

```
scc notes add "wrap writes MCP config to the agent's own file, so it outlives the session" \
  --tag gotcha --path internal/cli/launch.go
```

`--tag` is required, and it is what the log is queried by: run `scc notes tags`
first and reuse one rather than coining a fourth name for the same concern.
`--path` is what keeps a note attached to code without living in it — repo-relative,
as many as apply, and one that later disappears is reported rather than left to rot.

**One line, always**, which is what makes a match a whole note. If it needs a second
line it is not a note:

| It is… | so it goes in |
|---|---|
| something learned, worth explaining | `docs/wiki/pages/` — [knowledge-base.md](knowledge-base.md) |
| a decision that is hard to reverse | `docs/adr/` |
| how a whole area of code works | `docs/codewiki/` |
| something that has to be done | a plan's `## Tasks` — [artifacts.md](artifacts.md) |

## Reading it

Never end to end. Ask it:

| The question | Ask |
|---|---|
| What do we know about this file? | `scc notes find --path <path>` |
| …about this concern? | `scc notes tags` · `scc notes find --tag <tag>` |
| …containing this word? | `scc notes find <term>…` |
| What was noted lately? | `scc notes find --since <YYYY-MM-DD>` |
| What is `n-0042`? | `scc notes show n-0042` |

The line format is greppable on purpose — `grep ' #gotcha ' docs/notes.md` is the
same answer — and an id is citable: from a spec, from a commit, and from the one
comment that may still mention a note, `see n-0042`.
