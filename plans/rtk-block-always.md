---
autonomy: auto
ci: no-wait
---

# Rtk block always

Make `scc init` and `scc update` always leave RTK's usage block in every entry file,
whether or not the `rtk` binary is on PATH; only `--no-rtk` opts out.

## Why

Today `init` writes the block only when the binary is present or gets built, and
`update` never touches it, so a workspace scaffolded on a machine without RTK — or
created before the block existed — has an entry file that never mentions the prefix.
The block passes through unchanged when no filter matches, and the binary stays a
separate question (`init` still asks before running cargo). Done when a bare `init`
and an `update` both leave the block in place without a binary, an existing block is
kept as it is, `--no-rtk` skips it on both, and the suite and lint pass.

## Paths

- `internal/cli/init.go`
- `internal/cli/update.go`
- `internal/cli/rtk.go`
- `internal/cli/rtk_test.go`
- `internal/cli/update_test.go`

## Out of scope

- Replacing an existing block on `update`: an existing one is kept; `scc rtk` replaces on demand.
- Installing the binary from `update`: it splices the block and builds nothing.

## Tasks

- [x] 1.1 (Unit) Write the block on `init` even when the binary is absent or its install is declined
- [x] 1.2 (Unit) Plan and apply a missing block on `update`, with `--no-rtk` to skip it
  _Depends 1.1_
- [x] 1.3 (Unit) Compare an owned entry file to its template without the RTK and
      CodeGraph blocks, so `update` reports a current workspace as current

## Done when

- `go test ./...` and `golangci-lint run` exit 0.
- `scc validate --checks --pr` exits 0.
