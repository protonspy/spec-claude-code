---
autonomy: auto
ci: wait
---

# Pre push skip checks

Give the pre-push hook a narrow way past the delivery gate's commands —
`SCC_SKIP_CHECKS` — so a machine that cannot run part of the suite can push without
`SCC_SKIP_HOOKS=1` also switching off the artifact, attribution and pull-request checks.

## Why

A test suite that times out locally for an environmental reason — a database the
machine does not have — leaves `SCC_SKIP_HOOKS=1` as the only way to push, and that
skips every validator, attribution included, to get past one gate CI runs anyway.
`SCC_SKIP_CHECKS=1` skips the four command gates for one push, `SCC_SKIP_CHECKS=test`
(or a comma list) skips the named ones, and the hook says which it skipped so the push
never reads as verified. Done when the pre-push hook honors it, rejects an unknown gate
name with exit 1, and the suite and lint pass.

## Paths

- `internal/hooks/hooks.go`
- `internal/cli/hooks.go`
- `internal/validate/checks.go`
- `internal/validate/all.go`
- `docs/wiki/pages/hooks.md`

## Out of scope

- `scc validate --checks` and `scc check`: asked for by name, they run what was asked.
- Recording a skip in the manifest: that is `scc check skip <gate>`, and already exists.

## Tasks

- [x] 1.1 (Unit) Add `validate.WithoutGates` so a run treats the named gates as declined
- [x] 1.2 (Unit) Read `SCC_SKIP_CHECKS` in the pre-push hook, report the gates it skipped, and reject an unknown gate name
  _Depends 1.1_
- [x] 1.3 (Unit) Document `SCC_SKIP_CHECKS` beside `SCC_SKIP_HOOKS` in the hooks wiki page and the hook's failure hint
  _Depends 1.2_

## Done when

- `go test ./...` and `golangci-lint run` exit 0.
- `scc validate --checks --pr` exits 0.
