---
autonomy: auto
ci: wait
status: approved
checksum: 111a95fa7926c8e393d2bd2180abe5b090a00635d77df7995bff2f8047a6abe0
---

# Plan review agent

Ship a third read-only subagent, `scc-plan-review`, that reads a draft plan against
`docs/`, the specs and plans already in the workspace, and the code, and reports where
its context, tasks or direction disagree with what was decided and built.

## Why

`scc validate` checks a plan's shape and nothing checks its substance: a plan can pass
every validator while it re-decides an ADR, adds a dependency `docs/stack.md` never
adopted, re-builds a helper that already exists, names a concept by a synonym, or misses
the migration seam. `scc plan approve` seals the plan and refuses a rewritten task after
it, so the cheapest moment to catch any of that is before the seal — and the author is
the plan's worst reader. Done when the agent ships to all three harnesses and the two
skills that approve a plan dispatch it first.

## Paths

- `internal/assets/templates/agents/scc-plan-review.md`
- `internal/assets/assets.go`
- `internal/assets/assets_test.go`
- `internal/assets/templates/skills/scc-prd/SKILL.md`
- `internal/assets/templates/skills/scc-plan-run/SKILL.md`
- `.claude/agents/`
- `.claude/skills/`

- `design/orchestration.md`

## References

- `docs/wiki/pages/spec-or-plan.md` — the plan contract the reviewer holds a plan to.

## Out of scope

- Reviewing a spec's requirements, design or tasks: plans only.
- Dispatch from a preloaded rule such as `routing.md` or `artifacts.md`: the two plan
  skills carry it, so no session pays for it unless it is approving a plan.
- The reviewer editing the plan: it reports, and the orchestrator applies what it
  accepts with `scc patch`.
- A per-agent model choice: every agent stays on the harness's `sonnet` alias.

## Tasks

- [x] 1.1 (Unit) Write the `scc-plan-review` agent template: read-only, holds a draft plan to `docs/adr/`, `docs/stack.md`, `docs/glossary.md`, the wiki, the note log, existing specs and plans, and the code, and returns a verdict, findings by severity, questions for the user, and proposed `scc patch` amendments
- [x] 1.2 (Unit) Register `scc-plan-review` in `ReviewAgents`, pin its effort in `TestReviewAgentsPinTheirEffort`, and bump `assets.Version` to 38
  _Depends 1.1_
- [x] 1.3 (Unit) Dispatch `scc-plan-review` from the `scc-prd` and `scc-plan-run` skills before `scc plan approve`, and fold its findings and questions back into the plan before sealing it
  _Depends 1.2_
- [x] 1.4 (Unit) Re-render this workspace's own agents and skills onto template version 38 with `scc update`
  _Depends 1.3_
- [x] 1.5 (Unit) Update `design/orchestration.md` §7 to three review agents: an
      `scc-plan-review` row, its high effort, and that the plan skills dispatch it
      rather than delivery.md
  _Depends 1.3_

## Done when

- `go test ./...` and `golangci-lint run` exit 0.
- `.claude/agents/scc-plan-review.md` exists in this workspace, rendered by `scc update`.
- `scc validate --checks --pr` exits 0.

- The `scc-prd` and `scc-plan-run` skill templates dispatch `scc-plan-review` before `scc plan approve`, and `design/orchestration.md` §7 names all three agents.
