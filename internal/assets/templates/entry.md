# {{.Entry}}

This repo practises spec-driven development, scaffolded and checked by `scc`.

**This file is deliberately short, and keeping it short is a requirement.** Every
session pays for it in context, and model accuracy degrades as context grows —
non-uniformly, well before any documented limit. The methodology lives in
`{{.Rules}}/`, one file per concern, read when the concern is live. Do not
inline those rules here.

## The rules

| Read | When |
|---|---|
| [routing.md]({{.Rules}}/routing.md) | Work arrives and needs a vehicle — a spec, or a plan. |
| [autonomy.md]({{.Rules}}/autonomy.md) | At kickoff, before writing anything. |
| [methodology.md]({{.Rules}}/methodology.md) | Starting any task: which cycle, and what to run first. |
| [tasks.md]({{.Rules}}/tasks.md) | Writing or reading a task list. |
| [verification.md]({{.Rules}}/verification.md) | A task's code is written and you think it is done. |
| [delivery.md]({{.Rules}}/delivery.md) | The last task is done: branch, review, PR. |
| [specs.md]({{.Rules}}/specs.md) | Writing or changing anything under `specs/`. |
| [knowledge-base.md]({{.Rules}}/knowledge-base.md) | Something learned is worth keeping, or a decision was made. |
| [project.md]({{.Rules}}/project.md) | You need this project's build, test, or lint commands. |

## The layout

```
specs/<feature>/   requirements.md · design.md · tasks.md
plans/<name>.md    one file: structure, plus a checklist and/or spec references
docs/              the knowledge base — wiki, adr, codewiki, glossary, stack

{{.Rules}}/ — the methodology above
{{.Skills}}/ — how to author each part of docs/, and how to run a plan to the end
{{.Agents}}/ — code-review and security-review, run before the PR
{{- if .HasCommands}}
{{.Commands}}/ — the same skills on demand, as /scc-plan-run, /scc-wiki, /scc-adr, …
{{- end}}
```

## Checking your work

```
scc validate        # every applicable validator; exit 2 means findings
scc update          # take a newer scc's rules and agents; shows the plan, then asks
```

If `scc` is not installed on this machine, run it with no install — same binary,
same exit codes:

```
npx @protonspy/scc validate       # always the latest
npx @protonspy/scc@<version> ...  # pin a version (CI)
```

Exit codes are the contract: `0` ok, `1` the command could not run, `2` it ran and
found something. A finding is an answer, not a crash.

`scc` checks the *shape* of these artifacts — that a decision was made and written
down. It never reads your source, so it cannot tell you the code honors what the
artifact says. That part is on you.
