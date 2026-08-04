# Plans and specs — address them, do not read them

A plan is a structured document that happens to be Markdown. Reading one end to end
to answer a question about its structure is the most wasteful thing this workspace can
ask of you: a plan decomposing into thirty specs is tens of kilobytes of prose wrapped
around a dozen checkboxes, and once it is in context you carry it all session. `scc
map` answers those questions without loading the file; `scc patch` changes them
without loading it either.

| The question | Ask |
|---|---|
| What is here, and how far along? | `scc map` |
| What is the shape of this one? | `scc map <artifact>` |
| What do I work on next? | `scc map tasks <artifact> --next` |
| What is left in group 4? | `scc map tasks <artifact> --open --group 4` |
| Where is the note about X? | `scc map find "<terms>"` |
| Show me exactly that piece | `scc map show <artifact> <address>` |
| What else mentions this requirement? | `scc map trace specs/<feature>/R1.2` |

`<artifact>` is a path, a plan name, or a feature name. **An address is a name, never
a line number**, so it survives an edit above it:

```
1.2          a task           #notes     a section, by anchor slug
R1.2         a requirement    notes:7    the 7th paragraph of that section
specs/foo/   a leaf           L120-160   an explicit range, the escape hatch
```

`find` returns addresses, which is what makes the pair work: search, then `show` only
the hit. A long `## Notes` with no headings inside it is still navigable — `scc map
blocks` indexes its paragraphs by their opening sentence. Read the file directly only
when the question is about *this exact text*: prose you are about to rewrite.

## Writing

**Tick boxes and amend tasks with `scc patch`, not with an editor.**

```
scc patch check  <artifact> 1.1 1.2
scc patch task   <artifact> 1.2 --text "…" --method TDD --req R1.1,R1.2
scc patch add    <artifact> --section tasks --number 1.3 --method Unit --text "…"
scc patch append <artifact> '#notes' --text -     reads stdin, for paragraphs
scc patch fm     <artifact> pr=per-plan
```

Each resolves its address with the parser that read the file, so a miss is an error
rather than a write to the wrong place. It then re-runs the validators and **rolls the
change back if it introduced a finding** — exit `2`, file untouched. `--dry-run` shows
the lines first; deleting more than a screenful stops and asks for `--force`.

That is why you need not read a plan to change one line of it. Do not defeat it by
reading "to be safe": the printed before/after is the confirmation.

A requirement id is scoped to its own spec — `R2.5` in one feature is not `R2.5` in
another — so cite it as `specs/<feature>/R2.5` when the spec is not obvious.
