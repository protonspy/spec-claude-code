# npm packaging

How `scc` reaches npm. Nothing here is built from source at install time.

## Layout

| Path | What it is |
|---|---|
| `scc/` | The launcher source: a Node shim, no binary. Its `package.json` is a template — `name`, `version`, and `optionalDependencies` are all generated. |
| `scripts/build-packages.mjs` | Assembles `npm/dist/` from the release artifacts. |
| `dist/scc-<platform>/` | Generated. One per target, carrying the native binary. |
| `dist/launchers/<name>/` | Generated. The same shim, under a name that is already ours. |
| `dist/launchers-optional/<name>/` | Generated. The same shim, under a name being tried — allowed to fail. |

## How the install works

The launcher declares one `optionalDependencies` entry per platform
(`@protonspy/scc-linux-x64`, `…-darwin-arm64`, …), each carrying a prebuilt Go
binary and constrained by npm's `os`/`cpu` fields. npm installs only the package
matching the host, and `bin/scc.js` resolves that package's binary and execs it.

No postinstall script and no network access at install time — the binary is
already there or the install failed.

## Launcher names, required and optional

The launcher is published from one source under every name in `LAUNCHERS` in
`build-packages.mjs`. Each name is `required` or not, and that flag is the whole
difference:

| Tier | Meaning | On failure |
|---|---|---|
| `required: true` | The name is already ours. `@protonspy/scc` is the documented install. | The release fails. |
| `required: false` | A name being tried for the first time. | Warn and carry on. |

All of them put the same `scc` command on PATH: npm resolves the *package* name
and installs the *bin* name, and those never had to match. The bare `scc` on npm
has belonged to an unrelated project since 2013, which is why no name here is it.

**Why the optional tier exists.** npm applies a typosquatting similarity check
that runs *only on a real publish*. `npm view <name>` returning 404 says the name
is unregistered, not that it is publishable, and `npm publish --dry-run` never
reaches the check either. v0.9.0 died on `403 — Package name too similar to
existing package cp-cli` for a name both of those had called free; because that
name published first, `set -e` took the working launcher down with it and left
six orphaned platform packages on the registry.

So an unproven name is attempted last and allowed to fail, and **only a required
name may appear in documentation** — including the `entry.md` embedded in six
binaries. Pointing an install line at a package that might be refused is the same
bug somewhere more expensive. Promote a name to `required`, and into the docs, in
the release *after* the one that proved it publishes.

Installing two launchers globally is the one thing to avoid: they compete for the
same command name, and npm resolves that by letting the last one win.

The tiers are also directories, so publish order and failure policy are both
visible in the layout: `dist/scc-*/`, then `dist/launchers/`, then
`dist/launchers-optional/`. A launcher that reached the registry ahead of the
binaries in its `optionalDependencies` would be a broken install for anyone who
hit that window.

## Releasing

The release workflow does this automatically. Manually, from a clean tree:

```bash
make dist      VERSION=v0.1.0   # cross-compile the 6 targets into dist/
make npm-build VERSION=v0.1.0   # assemble npm/dist/ from those artifacts
make npm-dry-run                # validate every package
make npm-publish                # publish (skips already-published)
```

`build-packages.mjs` reuses the release artifacts as-is, so the binary published
to npm is byte-identical to the one attached to the GitHub Release.

Adding a platform means adding it to `TARGETS` in `build-packages.mjs`, the
`PLATFORMS` list in the `Makefile`, and the `build` matrix in
`.github/workflows/release.yml` — all three must agree.
