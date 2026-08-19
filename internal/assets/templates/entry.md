# {{.Entry}}

Spec-driven development, scaffolded and checked by `scc`. The methodology lives in `{{.Rules}}/` — never inline it here.

## Rules — `{{.Rules}}/<name>.md`

{{if .RulesPreloaded -}}
{{.Label}} loads `{{.Rules}}/` at session start — nothing to open. The triggers say *when*
each governs: the failure is not a rule you never read, it is one you had and misapplied.
{{- else -}}
Nothing loads these for you. Open the file whose moment has arrived, and open it again
in a new session: a rule you read yesterday is not a rule you have read.
{{- end}}

`caveman.md` is always on: the register you answer in. The rest, by where you are:

- `autonomy.md` — at kickoff, before writing anything
- `prior-art.md` — then read what `docs/` already decides, before the first artifact
- `routing.md` — work arrives and needs a vehicle: a spec, or a plan
- `methodology.md` — starting a task: which cycle, what to run first
- `verification.md` — code is written and you think it is done
- `delivery.md` — last task done: branch, review, PR

Triggered by what you are about to touch:

- `project.md` — **before any build, test, lint, or format command.** This project's
  commands exist nowhere else, and a guessed test command that exits 0 looks exactly
  like a passing suite.
- `code-search.md` — before going looking for code you have not read
- `artifacts.md` — before opening a plan or a spec
- `specs.md` — writing requirements, design, or tasks for a spec
- `tasks.md` — working through a spec's task list
- `knowledge-base.md` — something was learned, or a decision was made

## Ask the index before you read the file

**Code** — `scc graph query|explore <symbol>`, or `codegraph_explore` where registered.
Read the source when you are about to change it, not to find it.

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
docs/               knowledge base — wiki, adr, codewiki, glossary, stack
{{.RulesCol}}the methodology above
{{.SkillsCol}}authoring each part of docs/, and running a plan group by group
{{- if .HasCommands}}
{{.CommandsCol}}the same skills on demand: /scc-plan-run, /scc-wiki, /scc-adr, …
{{- end}}
```

## Checking your work

`scc validate` — or `npx @protonspy/scc validate` if not installed (`@<version>` pins for CI).
`scc update` brings a newer scc's rules and agents in: it shows the plan, then asks.

Exit `0` ok · `1` could not run · `2` ran and found something. A finding is an answer, not a crash.
`scc` checks artifact *shape* only; it never reads source, so whether the code honors it is on you.
