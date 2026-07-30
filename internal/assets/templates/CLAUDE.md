# CLAUDE.md

This repo practises spec-driven development, scaffolded and checked by `scc`.

**This file is deliberately short, and keeping it short is a requirement.** Every
session pays for it in context, and model accuracy degrades as context grows —
non-uniformly, well before any documented limit. The methodology lives in
`.claude/rules/`, one file per concern, read when the concern is live. Do not
inline those rules here.

## The rules

| Read | When |
|---|---|
| [routing.md](.claude/rules/routing.md) | Work arrives and needs a vehicle — a spec, or a plan. |
| [autonomy.md](.claude/rules/autonomy.md) | At kickoff, before writing anything. |
| [methodology.md](.claude/rules/methodology.md) | Starting any task: which cycle, and what to run first. |
| [tasks.md](.claude/rules/tasks.md) | Writing or reading a task list. |
| [verification.md](.claude/rules/verification.md) | A task's code is written and you think it is done. |
| [delivery.md](.claude/rules/delivery.md) | The last task is done: branch, review, PR. |
| [specs.md](.claude/rules/specs.md) | Writing or changing anything under `specs/`. |
| [knowledge-base.md](.claude/rules/knowledge-base.md) | Something learned is worth keeping, or a decision was made. |
| [project.md](.claude/rules/project.md) | You need this project's build, test, or lint commands. |

## The layout

```
specs/<feature>/   requirements.md · design.md · tasks.md
plans/<name>.md    one file: structure, plus a checklist and/or spec references
docs/              the knowledge base — wiki, adr, codewiki, glossary, stack
.claude/rules/     the methodology above
.claude/skills/    how to author each part of docs/ — invoked when it applies
.claude/commands/  the same skills on demand, as /scc-wiki, /scc-adr, /scc-prd, …
```

## Checking your work

```
scc validate        # every applicable validator; exit 2 means findings
```

Exit codes are the contract: `0` ok, `1` the command could not run, `2` it ran and
found something. A finding is an answer, not a crash.

`scc` checks the *shape* of these artifacts — that a decision was made and written
down. It never reads your source, so it cannot tell you the code honors what the
artifact says. That part is on you.
