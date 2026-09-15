# {{.Entry}}

Spec-driven development, scaffolded and checked by `scc`. The methodology lives in `{{.Rules}}/` — never inline it here.

## Rules — `{{.Rules}}/<name>.md`

{{if .RulesPreloaded -}}
{{.Label}} loads `{{.Rules}}/` at session start — nothing to open{{if .RulesScoped}}, bar `specs.md`,
`tasks.md` and `knowledge-base.md`, read when you touch their tree{{end}}. The failure is not a
rule you never read, it is one you had and misapplied — so the triggers say *when* each governs.
{{- else -}}
Nothing loads these for you. Open the file whose moment has arrived, and open it again
in a new session: a rule you read yesterday is not a rule you have read.
{{- end}}

`caveman.md` is always on: the register you answer in. The rest, by where you are:

- `autonomy.md` — at kickoff, before writing anything
- `prior-art.md` — then read what `docs/` already decides, before the first artifact
- `routing.md` — work arrives and needs a vehicle: a spec, or a plan
- `methodology.md` — starting a task: which cycle, what to run first
- `ladder.md` — about to write the code: how much of it the task gets
- `verification.md` — code is written and you think it is done
- `delivery.md` — last task done: branch, review, PR

Triggered by what you are about to touch:

- `project.md` — **before any build, test, lint, or format command.** This project's
  commands exist nowhere else; a guessed one that exits 0 looks like a passing suite.
- `code-search.md` — before going looking for code you have not read
- `artifacts.md` — before opening a plan or a spec
- `specs.md` — writing requirements, design, or tasks for a spec
- `tasks.md` — working through a spec's task list
- `knowledge-base.md` — something was learned, or a decision was made
- `notes.md` — **before typing a comment that is not a docstring**; `scc notes find --path <f>`

## Ask the index before you read the file

**Code** — `scc graph query|explore <symbol>`; read the source to change it, not to find it.

**Plans and specs** — a plan is a header and a checklist: `map brief <plan>` once, then
`map tasks <plan> --next` per task; **never open the plan**. Also `scc map` and `map
show <artifact> <address>`. An address is a name, never a line number: `1.2` `#risks`.

**Changing one** — `scc patch check <artifact> 1.2`, plus `task` `add` `append` `fm`. Not
an editor: it resolves the address, re-validates, and rolls back an edit that adds a
finding — so you need not read a plan to change one line of it.

## Layout

```
specs/<feature>/    requirements.md · design.md · tasks.md
plans/<name>.md     structure, plus a checklist and/or spec references
docs/               knowledge base — wiki, adr, codewiki, glossary, stack, notes
{{.RulesCol}}the methodology above
{{.SkillsCol}}authoring each part of docs/, and running a plan group by group
{{- if .SkillsAreCommands}}
                    on demand too: /scc-plan-run, /scc-wiki, /scc-adr, …
{{- else if .HasCommands}}
{{.CommandsCol}}the same skills on demand: /scc-plan-run, /scc-wiki, /scc-adr, …
{{- end}}
```

## Checking your work

`scc validate` — or `npx @protonspy/scc validate` if not installed (`@<version>` pins for CI).
`scc update` brings a newer scc's rules and agents in: it shows the plan, then asks.
Exit `0` ok · `1` could not run · `2` ran and found something. A finding is an answer, not a crash.
`scc` checks artifact *shape* only; it never reads source, so whether the code honors it is on you.
