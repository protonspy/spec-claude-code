#!/usr/bin/env node
// Launcher for the scc CLI distributed via npm.
//
// The actual Go binary ships in a per-platform optional dependency
// (@protonspy/scc-<platform>-<arch>). npm installs only the package whose
// "os"/"cpu" match the host, so this shim just resolves that package's binary
// and execs it — forwarding argv, stdio, signals, and the exit code. No
// postinstall, no network at install time.
import { spawn } from "node:child_process";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);

const PLATFORM = { darwin: "darwin", linux: "linux", win32: "win32" }[process.platform];
const ARCH = { x64: "x64", arm64: "arm64" }[process.arch];

if (!PLATFORM || !ARCH) {
  console.error(
    `scc: unsupported platform ${process.platform}/${process.arch}. ` +
      `Prebuilt binaries are available for linux, macOS, and Windows on x64/arm64.\n` +
      `Build from source instead: go install github.com/protonspy/spec-claude-code/cmd/scc@latest`
  );
  process.exit(1);
}

const pkg = `@protonspy/scc-${PLATFORM}-${ARCH}`;
const binName = process.platform === "win32" ? "scc.exe" : "scc";

let binPath;
try {
  binPath = require.resolve(`${pkg}/bin/${binName}`);
} catch {
  console.error(
    `scc: could not find the native binary for ${PLATFORM}-${ARCH}.\n` +
      `The optional dependency "${pkg}" was not installed.\n` +
      `Reinstall without --no-optional / --ignore-optional, or report an issue at\n` +
      `https://github.com/protonspy/spec-claude-code/issues`
  );
  process.exit(1);
}

// When invoked via `npx @protonspy/scc` (which is `npm exec` under the hood),
// echo that exact spelling in the binary's help/usage output. A global install
// runs this same launcher as the bare `scc` command — there npm is not in the
// picture (npm_command is unset), so the binary keeps its default name. An
// explicit SCC_PROG always wins.
//
// The name is read from this package's own package.json rather than written in,
// because one shim is published under more than one name and a hardcoded
// spelling would be wrong for whoever installed under the other — telling them
// to re-run a command under a package they never installed.
const env = { ...process.env };
if (!env.SCC_PROG) {
  const argv1 = process.argv[1] || "";
  const viaNpx =
    process.env.npm_command === "exec" ||
    argv1.includes("/_npx/") ||
    argv1.includes("\\_npx\\");
  if (viaNpx) {
    let self = "@protonspy/scc";
    try {
      self = require("../package.json").name || self;
    } catch {
      /* published without its manifest — fall back to the documented name */
    }
    env.SCC_PROG = `npx ${self}`;
  }
}

const child = spawn(binPath, process.argv.slice(2), { stdio: "inherit", env });

// Forward terminating signals so long-running commands shut down cleanly.
for (const sig of ["SIGINT", "SIGTERM", "SIGHUP"]) {
  process.on(sig, () => {
    try {
      child.kill(sig);
    } catch {
      /* child already gone */
    }
  });
}

child.on("error", (err) => {
  console.error(`scc: failed to launch binary: ${err.message}`);
  process.exit(1);
});

child.on("exit", (code, signal) => {
  if (signal) {
    // Re-raise the signal so the parent's exit status reflects it. Remove our own
    // handler for that signal first, otherwise the forwarding listener installed
    // above would intercept the re-raise and the wrapper would exit 0 instead of
    // dying from the signal.
    process.removeAllListeners(signal);
    process.kill(process.pid, signal);
  } else {
    // Preserve the binary's exit code verbatim — 2 means "validation findings"
    // and callers branch on it.
    process.exit(code ?? 0);
  }
});
