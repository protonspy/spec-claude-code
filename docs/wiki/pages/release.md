# Releasing

`release.yml` is `workflow_dispatch` only, and it runs in one order: validate the
version, run the full CI gate, cross-compile the six targets, publish the npm
packages, then tag and cut the GitHub Release. Everything below is a property that
had to be designed rather than a step that had to be listed.

## A version is immutable

Re-dispatching an already-released version from a *different* commit is refused.
Publishing is idempotent — see below — so without that refusal the run would go
green having shipped nothing, which is the worst available outcome: a release that
reports success and changes nobody's install.

## Publishing is idempotent

Already-published packages are skipped rather than failing the job, so a run that
died after `npm-publish` is resumed by re-dispatching **the same commit**. The two
properties are one design: immutability is what makes re-dispatch safe, and
idempotency is what makes it useful.

## Adding a platform touches three files that must agree

`TARGETS` in `npm/scripts/build-packages.mjs`, `PLATFORMS` in the `Makefile`, and
the `build` matrix in `release.yml`. Nothing derives one from another, so a platform
added to two of the three ships a launcher whose `optionalDependencies` name a
package that was never built.

## The launcher ships in two tiers

One source, published under every name in `LAUNCHERS`, and the tiers are
**directories** — `npm/dist/launchers/` and `npm/dist/launchers-optional/` — so
publish order *and* failure policy are visible in the layout rather than in a script.
Both go after `dist/scc-*/`: a launcher reaching the registry ahead of the binaries
in its `optionalDependencies` is a broken install for everybody in that window.

- **Required** — a name that is already ours. `@protonspy/scc` is the documented
  install, and a failure there fails the release.
- **Optional** — a name being tried for the first time. Attempted last, allowed to
  fail with a warning.

**The optional tier exists because npm's typosquatting similarity check runs only on
a real publish.** `npm view` returning 404 means unregistered, not publishable, and
`npm publish --dry-run` never reaches the check at all. v0.9.0 died on `403 — Package
name too similar to existing package cp-cli` for a name both of those had called
free, and because that name published first, `set -e` took the working launcher down
with it.

The load-bearing corollary is a documentation rule: **only a required name may appear
in documentation**, the embedded entry template included. A name is promoted into the
docs in the release *after* the one that proved it publishes.

## Actions are pinned by commit SHA

Along with the `govulncheck` and `golangci-lint` versions. A release pipeline that
resolves a tag at run time is a supply-chain surface on every dispatch, which is the
same reasoning `go.mod` is held to: see `adr:0001-stdlib-only-dependencies`.

## Related

[[managed-files]] · [[validation-contract]] · [[integration-boundary]]
