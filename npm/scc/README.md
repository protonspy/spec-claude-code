# scc

Spec-driven development for coding agents — a single Go binary that turns the SDD
workflow into a mechanically validated contract for humans and AI agents. Works
with Claude Code, Codex, and opencode.

```bash
npm i -g @protonspy/scc      # then, anywhere: scc help
npx @protonspy/scc help      # no install
```

The command is `scc` either way. Installing without `-g` puts it in
`node_modules/.bin`, which is on PATH for npm scripts but not for your shell —
there, reach it as `npx scc`.

This package is a thin launcher. The native binary ships in a per-platform
optional dependency (`@protonspy/scc-linux-x64`, `…-darwin-arm64`, …); npm
installs only the one matching your machine. There is no postinstall step and no
download at install time.

Prebuilt for Linux, macOS, and Windows on x64 and arm64. On any other platform,
build from source instead:

```bash
go install github.com/protonspy/spec-claude-code/cmd/scc@latest
```

Source, issues, and documentation: <https://github.com/protonspy/spec-claude-code>

Licensed under Apache-2.0.
