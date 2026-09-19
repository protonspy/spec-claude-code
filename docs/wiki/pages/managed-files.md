# Managed files and ownership

`scc` writes into directories other people own — the workspace's entry file, the
harness's settings, `.git/hooks`. The rule that governs all of it is one line:
**never author what the user owns.** Everything below is how that is made
mechanical rather than aspirational.

## The manifest is what makes ownership knowable

`<harness>/scc-manifest.json` records `{path, hash, version}` for every file `scc`
laid down, so `manifest.Status` can answer `pristine | edited | missing` for each.
That is the whole basis for upgrading: `scc update` hashes every managed file
against this build and against the manifest, prints the plan grouped by outcome,
asks, and replaces only what is safe to replace. An edited file is **kept and
named**; `--force` is a separate decision.

The manifest doubles as the workspace marker — see [[harness-profile]] — and it is
`scc`'s only file per harness. It gained its first *input* with the delivery gate's
four commands; the bar for a further key is that something inside the binary reads
it, and that bar is deliberately high. A `scc.json` read by nothing would be dead
weight, and a key nothing reads is the same dead weight with a shorter path.

That input has a failure mode worth naming, because it is silent. `scc init` and
`scc update` both build the next manifest **from scratch**, so without
`Manifest.CarryOver` a re-run of `init` deleted the gate's four commands — and had
already been deleting the unknown fields `UnmarshalJSON` goes to the trouble of
preserving. `Manifest.commands` is the one table read, carry-over and write all
share, so a fifth key cannot be preserved on read and dropped on a re-scaffold.

## Four categories, four lifecycles

| Kind | Rendered from | Tracked | Updated |
|---|---|---|---|
| Workspace template — rules, agents, skills | profile only, data-free | yes | by `scc update` |
| Artifact template — a new spec or plan | takes data | no | never; the user owns the result |
| Seed — the five `docs/` anchors | data-free | **no** | never |
| Foreign file — entry file, `settings.json`, `.git/hooks` | not rendered | no | spliced, never authored |

A seed is the odd one out on purpose: data-free like a workspace file, untracked
like an artifact. `glossary.md`, `stack.md`, `notes.md`, `wiki/index.md` and
`wiki/changelog.md` are written once holding the format their validators check, and
then they are yours.

## Splicing: one block inside somebody else's file

`internal/mdblock` is the whole mechanism — find the marker pair, compare, replace
or append, preserve the file's line endings, leave everything outside the markers
untouched. **Which markers is the integration's decision; what happens after that
choice lives in one place.**

That choice cuts both ways, deliberately:

- **RTK's own markers**, because `rtk init` writes them too, and sharing them is
  what makes the two tools converge on one copy of a block they both write. A
  namespaced `scc:rtk-instructions` would leave the file carrying both.
- **`scc`'s own markers for CodeGraph**, because CodeGraph writes nothing into the
  entry file and the block is `scc`'s account of `scc graph`. Namespacing leaves a
  future CodeGraph release free to add its own.

**A block somebody else wrote for the same job is reported, never touched.**
Headroom namespaces its copy of the RTK guidance as `headroom:rtk-instructions`,
which is not a substring of RTK's own pair — so a workspace wired by both ends up
carrying the instructions twice. `rtk.ForeignBlock` detects exactly that, names
Headroom in the report, and gives `headroom unwrap <agent>` as the fix. It is
reported rather than removed because the block belongs to Headroom: `scc` removing
it would be authoring somebody else's file to undo somebody else's registration.

The same reasoning governs `settings.json`, which is spliced rather than authored:
`scc`'s entries are identified by their command, replaced in place, and every other
entry, event and unknown key survives. **A file `scc` cannot parse is one `scc` will
not rewrite** — reformatting somebody's broken JSON into valid JSON of `scc`'s
choosing would destroy what they were in the middle of fixing.

A `.git` hook `scc` did not write is reported as foreign and left alone, with
`--force` as the separate decision — and even then only into a script whose shebang
says POSIX `sh`, because appending `sh` to a Python hook is not a forced decision but
a broken repository.

## Determinism

Rendered output is normalized to LF (`textutil.NormalizeNewlines`) so scaffolded
files hash identically regardless of the build machine's checkout, and anything
serialized is sorted first. `.gitattributes` forces LF across the repository for the
same reason: CRLF would break gofmt, the manifest hashes, and the shell hooks.

## Related

[[harness-profile]] · [[integration-boundary]] · [[validation-contract]]
