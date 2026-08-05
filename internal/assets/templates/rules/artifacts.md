# Plans and specs — address them, do not read them

A plan is a header and a checklist, and `scc` answers every question about it without
loading the file. Reading one end to end is the most wasteful thing this workspace can
ask of you: once it is in context you carry it for the rest of the session. **Never
open a plan** — `brief` is the header, `tasks` is the checklist, no command returns
both, so no question about a plan has the file as its answer.

| The question | Ask |
|---|---|
| What is here, and how far along? | `scc map` · `scc map <artifact>` |
| What is this work, and when is it done? | `scc map brief <plan>` — once, per session |
| What do I work on now? | `scc map tasks <plan> --next` · `--ready` · `--blocked` |
| Show me exactly that piece | `scc map show <artifact> <address>` |
| What else mentions this requirement? | `scc map trace specs/<feature>/R1.2` |

`<artifact>` is a path, a plan name, or a feature name. **An address is a name, never
a line number**, so it survives an edit above it:

```
1.2          a task            #risks     a section, by anchor slug
R1.2         a requirement     risks:2    the 2nd paragraph of that section
specs/foo/   a spec reference  L120-160   an explicit range, the escape hatch
```

Read a file directly only when the question is about *this exact text* — prose you are
about to rewrite, which is a spec's design and never a plan. **A plan's shape is
closed**: the title, one to three sentences, then `## Why`, `## Paths`, `## References`,
`## Out of scope`, `## Tasks`, `## Done when`, and any other heading is a finding.
`## References` names the specs this decomposes into and carries no checkbox — that
spec's state lives in that spec.

## Writing

**Tick boxes and amend tasks with `scc patch`, not with an editor.**

```
scc patch check <artifact> 1.1 1.2          · patch fm <artifact> pr=per-plan
scc patch task  <artifact> 1.2 --text "…" --method TDD --depends 1.1 --priority 2
scc patch add   <artifact> --group 1 --text "…" --reason "…"
scc patch rm    <artifact> 1.4 --reason "…"
```

Each resolves its address with the parser that read the file, so a miss is an error
rather than a write to the wrong place. It then re-runs the validators and **rolls the
change back if it introduced a finding** — exit `2`, file untouched. `--dry-run` shows
the lines first; deleting more than a screenful stops and asks for `--force`. That is
why you need not read a plan to change one line of it: do not defeat it by reading "to
be safe", since the printed before/after is the confirmation.

After `scc plan approve` the work is settled: `add` needs `--group` and `--reason` and
is given its number, `rm` strikes the task out where it stands so the number is never
reused, and rewriting a task or the prose is refused — a task that turned out wrong is
struck out and replaced. An edit made outside `scc` shows up as drift. A requirement id
is scoped to its own spec, so cite it as `specs/<feature>/R2.5` when that is not obvious.
