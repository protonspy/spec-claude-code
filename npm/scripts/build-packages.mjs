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
//   scc/                       launcher package (shim + optionalDependencies)
//   scc-<platform>-<arch>/     one per target, carrying the native binary
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

// --- launcher package ------------------------------------------------------
const rootSrc = join(npmDir, BIN);
const rootOut = join(outDir, BIN);
mkdirSync(join(rootOut, "bin"), { recursive: true });
cpSync(join(rootSrc, "bin", `${BIN}.js`), join(rootOut, "bin", `${BIN}.js`));
cpSync(join(rootSrc, "README.md"), join(rootOut, "README.md"));

// The checked-in optionalDependencies are a placeholder; the real list is
// generated from TARGETS above so a new platform can never be half-wired.
const rootPkg = JSON.parse(readFileSync(join(rootSrc, "package.json"), "utf8"));
rootPkg.version = version;
rootPkg.optionalDependencies = optionalDependencies;
writeFileSync(join(rootOut, "package.json"), JSON.stringify(rootPkg, null, 2) + "\n");
console.log(`built ${rootPkg.name}@${version}`);
