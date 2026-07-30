# npm packaging

How `scc` reaches npm. Nothing here is built from source at install time.

## Layout

| Path | What it is |
|---|---|
| `scc/` | The published launcher package (`@protonspy/scc`): a Node shim, no binary. |
| `scripts/build-packages.mjs` | Assembles `npm/dist/` from the release artifacts. |
| `dist/` | Generated, git-ignored. One directory per package to publish. |

## How the install works

The launcher declares one `optionalDependencies` entry per platform
(`@protonspy/scc-linux-x64`, `…-darwin-arm64`, …), each carrying a prebuilt Go
binary and constrained by npm's `os`/`cpu` fields. npm installs only the package
matching the host, and `bin/scc.js` resolves that package's binary and execs it.

No postinstall script and no network access at install time — the binary is
already there or the install failed.

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
