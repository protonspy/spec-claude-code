#!/usr/bin/env node
// Assemble the npm publish tree from the release artifacts.
//
// Usage: node npm/scripts/build-packages.mjs <tag> [artifactsDir]
//
//   <tag>          the release tag, e.g. "v1.2.3" (the leading "v" is stripped
//                  for the npm version; the raw tag is used to locate the
//                  artifact tarballs produced by the release workflow).
//   [artifactsDir] directory holding scc_<tag>_<goos>_<goarch>.{tar.gz,zip}
//                  (default: "artifacts").
//
// Output: npm/dist/
//   scc-<platform>-<arch>/     one per target, carrying the native binary
//   launchers/<name>/          the shim, published under each launcher name
//
// Publish order is the layout: every scc-*/ package must reach the registry
// before any launchers/ package that optionally depends on it.
//
// The Go binaries are reused as-is from the release artifacts (they already
// carry the version baked in via -ldflags), so the npm binary is byte-identical
// to the GitHub release binary.
import { execFileSync } from "node:child_process";
import {
  chmodSync,
  cpSync,
  existsSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const SCOPE = "@protonspy";
const BIN = "scc";

// Go target -> npm platform/arch + os/cpu constraints.
// Must match the Makefile PLATFORMS and the release.yml build matrix. The
// launcher's optionalDependencies are generated from this list, so adding a
// target here emits its platform package and wires it up automatically.
const TARGETS = [
  { goos: "linux", goarch: "amd64", node: "linux-x64", os: "linux", cpu: "x64" },
  { goos: "linux", goarch: "arm64", node: "linux-arm64", os: "linux", cpu: "arm64" },
  { goos: "darwin", goarch: "amd64", node: "darwin-x64", os: "darwin", cpu: "x64" },
  { goos: "darwin", goarch: "arm64", node: "darwin-arm64", os: "darwin", cpu: "arm64" },
  { goos: "windows", goarch: "amd64", node: "win32-x64", os: "win32", cpu: "x64" },
  { goos: "windows", goarch: "arm64", node: "win32-arm64", os: "win32", cpu: "arm64" },
];

const scriptDir = dirname(fileURLToPath(import.meta.url));
const npmDir = resolve(scriptDir, "..");
const repoRoot = resolve(npmDir, "..");

const tag = process.argv[2];
if (!tag) {
  console.error("error: missing <tag> argument (e.g. v1.2.3)");
  process.exit(1);
}
const version = tag.replace(/^v/, "");
const artifactsDir = resolve(process.argv[3] ?? join(repoRoot, "artifacts"));
const outDir = join(npmDir, "dist");

// Preflight: every platform artifact must exist before we touch npm/dist, so a
// version mismatch (e.g. `npm-build VERSION=v0.1.2` against v0.1.1 tarballs)
// fails with a clear message instead of a cryptic tar/unzip error — and never
// wipes a good npm/dist or leaves a half-built one that breaks `npm-publish`.
const missingArtifacts = TARGETS.map((t) => {
  const ext = t.goos === "windows" ? "zip" : "tar.gz";
  return join(artifactsDir, `${BIN}_${tag}_${t.goos}_${t.goarch}.${ext}`);
}).filter((f) => !existsSync(f));
if (missingArtifacts.length > 0) {
  console.error(
    `error: missing release artifacts for ${tag} in ${artifactsDir}:\n` +
      missingArtifacts.map((f) => "  " + f).join("\n") +
      `\nbuild them first:  make dist VERSION=${tag}`
  );
  process.exit(1);
}

rmSync(outDir, { recursive: true, force: true });
mkdirSync(outDir, { recursive: true });

const repository = {
  type: "git",
  url: "git+https://github.com/protonspy/spec-claude-code.git",
};

// --- per-platform packages -------------------------------------------------
const optionalDependencies = {};
for (const t of TARGETS) {
  const pkgName = `${SCOPE}/${BIN}-${t.node}`;
  const binName = t.goos === "windows" ? `${BIN}.exe` : BIN;
  const pkgDir = join(outDir, `${BIN}-${t.node}`);
  const binDir = join(pkgDir, "bin");
  mkdirSync(binDir, { recursive: true });

  const base = `${BIN}_${tag}_${t.goos}_${t.goarch}`;
  if (t.goos === "windows") {
    execFileSync("unzip", ["-o", join(artifactsDir, `${base}.zip`), "-d", binDir], {
      stdio: "inherit",
    });
  } else {
    execFileSync("tar", ["-xzf", join(artifactsDir, `${base}.tar.gz`), "-C", binDir], {
      stdio: "inherit",
    });
    chmodSync(join(binDir, binName), 0o755);
  }

  writeFileSync(
    join(pkgDir, "package.json"),
    JSON.stringify(
      {
        name: pkgName,
        version,
        description: `scc native binary for ${t.node}`,
        repository,
        license: "Apache-2.0",
        os: [t.os],
        cpu: [t.cpu],
        files: [`bin/${binName}`],
      },
      null,
      2
    ) + "\n"
  );
  optionalDependencies[pkgName] = version;
  console.log(`built ${pkgName}@${version}`);
}

// --- launcher packages -----------------------------------------------------
// Two names, one package. `scc-cli` is the documented install line; the scoped
// `@protonspy/scc` stays published so anybody who already installed it keeps
// receiving versions. Both carry the same shim, the same optionalDependencies,
// and the same `bin` — so both put the same `scc` command on PATH. The package
// name is what npm resolves and the bin name is what you type, and those were
// never required to match: the bare `scc` on npm has been taken since 2013.
//
// They are emitted under launchers/ rather than beside the platform packages,
// and that is load-bearing rather than tidy: the publish order is expressed as
// "everything in dist/scc-*/ first, then everything in dist/launchers/", and a
// second launcher sitting at the top level would be swept into the platform
// glob and published ahead of the binaries it optionally depends on. Anyone
// installing during that window gets a launcher that cannot resolve a binary.
const LAUNCHERS = [
  { dir: "scc-cli", name: "scc-cli" },
  { dir: BIN, name: `${SCOPE}/${BIN}` },
];

const launcherSrc = join(npmDir, BIN);
// The checked-in package.json is a template: its name and optionalDependencies
// are both placeholders, generated here so that neither a new platform nor a new
// launcher name can ever be half-wired.
const launcherPkg = JSON.parse(readFileSync(join(launcherSrc, "package.json"), "utf8"));
for (const launcher of LAUNCHERS) {
  const out = join(outDir, "launchers", launcher.dir);
  mkdirSync(join(out, "bin"), { recursive: true });
  cpSync(join(launcherSrc, "bin", `${BIN}.js`), join(out, "bin", `${BIN}.js`));
  cpSync(join(launcherSrc, "README.md"), join(out, "README.md"));

  writeFileSync(
    join(out, "package.json"),
    JSON.stringify(
      { ...launcherPkg, name: launcher.name, version, optionalDependencies },
      null,
      2
    ) + "\n"
  );
  console.log(`built ${launcher.name}@${version}`);
}
