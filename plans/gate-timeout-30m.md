---
autonomy: auto
ci: no-wait
---

# Gate timeout 30m

Raise the timeout that bounds one gate run from 10 to 30 minutes, and let a run move
it with `--timeout <minutes>`.

## Why

A test suite that legitimately takes longer than ten minutes is killed and reported as
`check.timed-out`, failing the push for a reason that is not a defect. The bound still
exists so a hanging command cannot hang the push; the default moves to 30 minutes, and
a run that needs another bound asks for it on the command line. Done when
`gate.DefaultTimeout` is 30 minutes, `scc check` and `scc validate` accept
`--timeout`, and the suite and lint pass.

## Paths

- `internal/gate/gate.go`
- `internal/validate/checks.go`
- `internal/validate/all.go`
- `internal/cli/check.go`
- `internal/cli/validate_all.go`

## Out of scope

- A per-gate timeout: one bound applies to build, format, lint and test alike.
- Recording the timeout in the manifest: it is a flag on the run, never a committed value.

## Tasks

- [x] 1.1 (Unit) Set the default gate timeout to 30 minutes
- [x] 1.2 (Unit) Carry a per-run timeout on `gate.Config` and report it on a timed-out `gate.Result`
  _Depends 1.1_
- [x] 1.3 (Unit) Add `--timeout <minutes>` to `scc check` and `scc validate`, where it implies `--checks`
  _Depends 1.2_

## Done when

- `go test ./...` and `golangci-lint run` exit 0.
- `scc validate --checks --pr` exits 0.
