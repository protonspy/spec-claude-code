# Wiki

The entry point. Every page under `wiki/pages/` has to be reachable from here —
directly, or through a page that is.

Pages link to each other as `[[page-slug]]`, where the slug is the filename without
its extension and without its directory.

These pages are about `scc` **as a codebase** — the concepts a newcomer has to hold
before the code reads as deliberate. What one feature does now belongs in a spec;
`design/` holds the product's own design documents, which predate this wiki and are
the source most of it was reconstructed from.

## Start here

- [[context-budget]] — the one idea most of the design falls out of: what an agent
  reads at session start is paid again in every request.
- [[spec-or-plan]] — the two vehicles for work, how the choice is made, and why a
  plan is a contract rather than a document.

## The artifacts

- [[artifact-addressing]] — how `scc map` and `scc patch` read and change a piece of
  a file nobody opened, and why an address is never a line number.
- [[scoped-rules]] — the `paths:` header, the only lever on what the methodology
  costs, and the bar a rule has to clear to use it.

## The layout on disk

- [[harness-profile]] — three agent tools from one template set, and why a
  `switch h.ID` means the profile is missing a field.
- [[managed-files]] — the manifest, the four kinds of file `scc` writes, and the
  rule that it never authors what the user owns.

## Checking the work

- [[validation-contract]] — exit codes as a contract, the ten validators, and the
  two whose subject is not a file.
- [[delivery-gate]] — the project's own build, format, lint and test commands, as
  something `scc` can run and judge.
- [[hooks]] — the git and harness layers, and the rule a `Stop` hook has to obey.

## The outside world

- [[integration-boundary]] — six third-party binaries, one package each, and why
  everything degrades except the sandbox.
