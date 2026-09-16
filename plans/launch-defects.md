---
autonomy: auto
ci: wait
---

# Launch defects

A run of `scc launch claude` on Windows fell back to the host unsandboxed and warned
about a malformed block in this repo's own entry file. Two causes: the dev container's
`postCreateCommand` writes to a read-only mount, and `internal/mdblock` reads prose
about its markers as a block.

## Why

Both defects ship to every scaffolded workspace and both fail in the direction that
looks like success. The container one ends with an agent running outside the boundary
`scc launch` said it would provide. The mdblock one is worse: a document that merely
*documents* the markers is read as carrying the block, so `scc launch` skips the RTK
wiring it believes is already done, and `scc rtk` would splice over the prose between
a marker pair that only ever appeared in a sentence. Done is a launch that sandboxes
and an entry file that can describe the markers without being edited by them.

## Paths

- `internal/mdblock/mdblock.go`
- `internal/mdscan/mdscan.go`
- `.devcontainer/` and `internal/assets/templates/devcontainer/`

## Out of scope

- Editing `CLAUDE.md` to work around the marker bug — the file is the user's, and a
  document that cannot name the markers scc splices is the defect, not the workaround.

## Tasks

- [x] 1.1 (TDD) Add `mdscan.Mask`: fenced blocks and inline code spans blanked to
      spaces, every byte offset preserved, HTML comments left intact because the markers
      a caller looks for here are HTML comments.
- [x] 1.2 (TDD) Resolve the block through one span helper in `internal/mdblock`,
  measured over `MaskProse` and sliced out of the original, so `Splice`, `Block`,
  `Version` and `Remove` cannot disagree about where the block is.
  _Depends 1.1_
- [x] 1.3 (Unit) Move the dev container's `safe.directory` write out of
  `postCreateCommand` and into the image as a `--system` setting, in the seeded
  template and in this repo's copy.
- [x] 1.4 (Unit) Record what each defect cost, as notes against the files that carried
  them.
  _Depends 1.1, 1.2, 1.3, 1.5_
- [x] 1.5 (Unit) Mask the document in `rtk.ForeignBlock` too: a sentence naming
      Headroom's marker is not Headroom's block, and reporting it sends the reader to
      `headroom unwrap` for a block nobody wrote.
- [x] 2.1 (Unit) Raise coverage to the 86% floor the delivery gate requires, package by
      package from the least-covered, so `pre-push` passes on its own terms rather than
      on `SCC_SKIP_HOOKS`.

## Done when

- `scc launch --dry-run` reports no malformed-block warning on this repo's `CLAUDE.md`,
  and reports the RTK block as absent rather than present.
- `devcontainer up` completes its `postCreateCommand` on Windows, so `scc launch`
  reports the container rather than falling back to the host.
- `scc validate --checks` is clean, which it was not on `main`: the suite went from
  73.6% to 86.0%, the floor this workspace records.