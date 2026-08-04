# npm packaging

How `scc` reaches npm. Nothing here is built from source at install time.

## Layout

| Path | What it is |
|---|---|
| `scc/` | The launcher source: a Node shim, no binary. Its `package.json` is a template — `name`, `version`, and `optionalDependencies` are all generated. |
| `scripts/build-packages.mjs` | Assembles `npm/dist/` from the release artifacts. |
| `dist/scc-<platform>/` | Generated. One per target, carrying the native binary. |
| `dist/launchers/<name>/` | Generated. The same shim, once per published launcher name. |

## How the install works

The launcher declares one `optionalDependencies` entry per platform
(`@protonspy/scc-linux-x64`, `…-darwin-arm64`, …), each carrying a prebuilt Go
binary and constrained by npm's `os`/`cpu` fields. npm installs only the package
matching the host, and `bin/scc.js` resolves that package's binary and execs it.

No postinstall script and no network access at install time — the binary is
already there or the install failed.

## Two launcher names

The launcher is published twice, from one source, listed in `LAUNCHERS` in
`build-packages.mjs`:

| Name | Role |
|---|---|
| `scc-cli` | The documented install. Unscoped, so `npm i -g scc-cli` is the whole line. |
| `@protonspy/scc` | Kept published so earlier installs keep receiving versions. |

Both put the same `scc` command on PATH: npm resolves the *package* name and
puts the *bin* name on PATH, and those never had to match. The bare `scc` on npm
has belonged to an unrelated project since 2013, which is why neither name is it.

Installing both globally is the one thing to avoid — they compete for the same
command name, and npm resolves that by letting the last one win.

The split is also why launchers live under `dist/launchers/` instead of beside
the platform packages: publishing walks `dist/scc-*/` first and `dist/launchers/*/`
second, and a launcher that reached the registry ahead of the binaries it depends
on would be a broken install for anyone who hit that window.

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
