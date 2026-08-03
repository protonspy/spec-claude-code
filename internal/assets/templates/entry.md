# {{.Entry}}

Spec-driven development, scaffolded and checked by `scc`. Keep this file short —
the methodology lives in `{{.Rules}}/`, read when the concern is live. Never inline it here.

## Rules — `{{.Rules}}/<name>.md`

Read at these moments, without being asked:

- autonomy — at kickoff, before writing anything
- routing — work arrives and needs a vehicle: a spec, or a plan
- methodology — starting a task: which cycle, what to run first
- verification — code is written and you think it is done
- delivery — last task done: branch, review, PR

Read by name when you're in that territory: tasks, specs, project (build/test/lint
commands), knowledge-base (something learned, or a decision made).

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

`scc validate` — or `npx @protonspy/scc validate` if not installed (`@<version>` pins for CI).
`scc update` brings a newer scc's rules and agents in: it shows the plan, then asks.

Exit `0` ok · `1` could not run · `2` ran and found something. A finding is an answer, not a crash.

`scc` checks artifact *shape* only; it never reads source, so whether the code honors
the artifact is on you.
