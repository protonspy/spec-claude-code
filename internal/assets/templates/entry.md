# {{.Entry}}

Spec-driven development, scaffolded and checked by `scc`. Keep this file short —
the methodology lives in `{{.Rules}}/`. Never inline it here.

## Rules — `{{.Rules}}/<name>.md`

{{if .RulesPreloaded -}}
{{.Label}} loads `{{.Rules}}/` into your context at session start, so these are already
in front of you and there is nothing to open. What the triggers below tell you is *when*
each rule governs — the failure they prevent is not a rule you never read, it is a rule
you had all along and applied at the wrong moment, or not at all.
{{- else -}}
Nothing loads these for you. Open the file whose moment has arrived, and open it again
in a new session: a rule you read yesterday is not a rule you have read.
{{- end}}

Triggered by where you are in the work:

- `autonomy.md` — at kickoff, before writing anything
- `routing.md` — work arrives and needs a vehicle: a spec, or a plan
- `methodology.md` — starting a task: which cycle, what to run first
- `verification.md` — code is written and you think it is done
- `delivery.md` — last task done: branch, review, PR

Triggered by what you are about to touch:

- `project.md` — **before you run any build, test, lint, or format command.** This
  project's commands exist nowhere else: `scc` ships the file as a stub for the team
  to fill in, and runs none of them itself. A command that did not come from there is
  a guess, and a guessed test command that exits 0 looks exactly like a passing suite.
- `specs.md` — writing requirements, design, or tasks for a spec
- `tasks.md` — working through a spec's task list
- `knowledge-base.md` — something was learned, or a decision was made

## Layout

```
specs/<feature>/   requirements.md · design.md · tasks.md
plans/<name>.md    structure, plus a checklist and/or spec references
docs/              knowledge base — wiki, adr, codewiki, glossary, stack

{{.Rules}}/ — the methodology above
{{.Skills}}/ — authoring each part of docs/, and running a plan group by group
{{- if .HasCommands}}
{{.Commands}}/ — the same skills on demand: /scc-plan-run, /scc-wiki, /scc-adr, …
{{- end}}
```

## Checking your work

`scc validate` — or `npx scc-cli validate` if not installed (`@<version>` pins for CI).
`scc update` brings a newer scc's rules and agents in: it shows the plan, then asks.

Exit `0` ok · `1` could not run · `2` ran and found something. A finding is an answer, not a crash.

`scc` checks artifact *shape* only; it never reads source, so whether the code honors
the artifact is on you.
