---
autonomy: auto
ci: no-wait
---

# Attribution stale base

The commit range scc checks is measured against the local base branch only, so a
local `main` behind `origin/main` pulls already-merged commits into "this branch".

## Why

`git.Commits` resolves the base with `ref()`, which prefers `refs/heads/<base>`. A
branch cut from a fresh `origin/main` while local `main` is twenty commits behind
reports every one of those twenty as its own work — the attribution validator then
blocks `git push` on a commit somebody else merged weeks ago. Done means a commit
reachable from either copy of the base is never in the range.

## Paths

- `internal/git/git.go`
- `internal/git/repo_test.go`
- `internal/validate/attribution_test.go`

## Out of scope

- `Look`'s ahead/behind counts, which answer a different question about a named branch.

## Tasks

- [x] 1.1 (Unit) Exclude commits reachable from the local or the remote base in `Commits`, and diff `Changed` against the fresher of the two
- [x] 1.2 (Unit) Cover a stale local base in the git and attribution tests
  _Depends 1.1_

## Done when

- `go test ./internal/git/ ./internal/validate/ ./internal/cli/` passes, including a branch cut from `origin/main` while local `main` is behind.
- `scc validate` reports no finding for a commit that is already on `origin/main`.
