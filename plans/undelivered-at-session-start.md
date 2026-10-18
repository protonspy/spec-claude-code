---
autonomy: auto
ci: wait
---

# Undelivered at session start

Move the "commits never pushed" line from the Stop hook to SessionStart, and word it
as a pointer to the delivery sequence rather than an order to push.

## Why

On Stop, context continues the conversation, so the line read as an instruction and
re-fired at the end of every turn spent waiting — on the review agents, on a question
to the user, on a branch nobody wanted pushed yet — and it asked for a push before the
reviews delivery.md puts first. Done when Stop no longer reports unpushed work,
SessionStart does, and the suite and lint pass.

## Paths

- `internal/cli/hooks_agent.go`
- `internal/hooks/harness.go`
- `docs/wiki/pages/hooks.md`
- `README.md`

## Tasks

- [x] 1.1 (Unit) Report unpushed work at SessionStart instead of Stop, pointing at delivery.md's reviews-then-push sequence
- [x] 1.2 (Unit) Give the Stop flag regression test a finding to say now that unpushed work is gone from Stop
  _Depends 1.1_
- [x] 1.3 (Unit) Update the hooks wiki page, the README and the stage descriptions
  _Depends 1.1_

## Done when

- `go test ./...` and `golangci-lint run` exit 0.
- `scc validate --checks --pr` exits 0.
