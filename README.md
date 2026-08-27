# scc

Spec-driven development for Claude Code, Codex, and opencode — a single Go binary
that turns the SDD workflow into a mechanically validated contract for humans and
AI agents.

> **Status: pre-1.0.** Scaffolding, artifact creation, validation, and updating all
> work. `scc update` currently replaces what is safe to replace and *keeps* anything
> you edited, naming it; the three-way merge that would resolve those in place is
> still ahead.

## Use it

No install — run it straight from npm inside the repo you want to govern:

```bash
npx @protonspy/scc init                     # asks which harness, then scaffolds the rules, agents, and layout
npx @protonspy/scc init --codex             # or name it: --claude (default), --codex, --opencode
npx @protonspy/scc spec new user-auth       # specs/user-auth/: requirements.md, design.md, tasks.md
npx @protonspy/scc plan new checkout-revamp # plans/checkout-revamp.md
npx @protonspy/scc validate                 # every check; exit 2 means it found something
npx @protonspy/scc update                   # show what a newer scc would change, then confirm
```

Installed globally (`npm i -g @protonspy/scc`) the same commands are just `scc init`,
`scc spec new user-auth`, and so on.

| Command | What it does |
|---|---|
| `init` | Scaffolds the workspace and records what it wrote. Idempotent; never overwrites your edits. |
| `update` | Compares every managed file against this build, shows the plan, and applies it once you agree. |
| `spec new\|list\|show\|delete\|validate` | The three-artifact vehicle for work whose *what* and *how* need settling first. |
| `plan new\|list\|delete\|validate` | One file, for everything else: a checklist, a decomposition into specs, or both. |
| `skill validate` | Conformance to the published [Agent Skills](https://agentskills.io/specification) spec. |
| `validate` | Every applicable validator, one exit code, one JSON document. |
| `rtk` | Wires in [RTK](https://github.com/rtk-ai/rtk): installs it if missing, then splices its usage block into the entry file. |
| `launch` | Starts the harness with the workspace's symbol graph and RTK block current — and, with `--jail`, inside a sandbox. |

### RTK, optionally

[RTK](https://github.com/rtk-ai/rtk) is a CLI proxy that filters command output down
to what is worth spending context on. `scc rtk` — or `scc init --rtk` in one step —
installs it with cargo when it is not on PATH, and puts its usage block into
`CLAUDE.md`/`AGENTS.md` so the agent knows to prefix commands with it:

```bash
npx @protonspy/scc init --rtk    # scaffold, then wire RTK in
npx @protonspy/scc rtk           # wire it into a workspace that already exists
npx @protonspy/scc rtk --check   # CI: exit 2 when the block is missing
```

The block sits between RTK's own `<!-- rtk-instructions -->` markers, and scc inserts
one only where there is none: a block already in the file is left exactly as it is,
whatever version it claims, because RTK writes that block and `rtk init` is what
refreshes it. `--force` replaces it with the copy this scc ships. Everything outside
the markers is untouched either way.

Opt-in on purpose: it tells the agent to prefix every command with a binary the
machine may not have. `--no-install` writes the block and never touches cargo.

### A sandbox, optionally

An agent needs filesystem access to do its job, and the same access lets it run
`rm -rf`, read `~/.aws`, or ship a key somewhere — by accident, on a poisoned
instruction in a file it read, or through a dependency it installed. `scc launch
--jail` starts it inside [ai-jail](https://github.com/akitaonrails/ai-jail), which
sandboxes with bubblewrap on Linux and `sandbox-exec` on macOS:

```bash
npx @protonspy/scc launch claude --jail             # the agent, contained
npx @protonspy/scc launch claude --jail --jail-arg --lockdown
```

**It refuses rather than degrading.** Every other integration here starts the agent
anyway when its binary is missing, because every other one is an enhancement. A
sandbox is the property you asked for by name: if ai-jail is not installed, or the
platform has no backend (Windows — use WSL2), nothing starts and it says why. An
agent that started unjailed would hand you the confidence of containment without the
containment.

scc passes exactly the two flags that let an agent run at all — a network to reach
its model and the credential state to authenticate — and reads even those off
`ai-jail --help` rather than hardcoding them. Everything else is policy and belongs
in ai-jail's own `~/.ai-jail` / `./.ai-jail`, which scc never writes.

The idea, and the tool, are [Fábio Akita's](https://akitaonrails.com/2026/01/10/ai-agents-garantindo-a-protecao-do-seu-sistema/).

### Three harnesses, one methodology

The same rules, review agents, and skills — the knowledge base's authors, plus
`plan-run`, which drives a whole plan group by group — are scaffolded into whichever
tool you work in. Only the paths and the frontmatter dialect change.

| | Claude Code | Codex | opencode |
|---|---|---|---|
| entry file | `CLAUDE.md` | `AGENTS.md` | `AGENTS.md` |
| rules | `.claude/rules/` | `.codex/rules/` | `.opencode/rules/` |
| review agents | `.claude/agents/*.md` | `.codex/agents/*.toml` | `.opencode/agent/*.md` |
| skills | `.claude/skills/` | `.codex/skills/` | `.opencode/skills/` |
| slash commands | `.claude/commands/` | — (skills are the surface) | `.opencode/command/` |

`specs/`, `plans/`, and `docs/` are identical everywhere: they are the product, not
the tool. Running `init` twice with different flags gives one repo two managed trees,
and `update` keeps both current.

## What it checks

What gets checked: EARS grammar across all five patterns, requirement numbering,
one methodology annotation per task, traceability in both directions, plan
one-source-of-truth, skill conformance, wiki link/orphan graph, ADR numbering and
superseding, glossary vocabulary drift, dependencies missing from `docs/stack.md`, and
codewiki citations that no longer resolve.

What deliberately is **not** checked: your source code. scc never parses it, so it
cannot tell you the code honors what the artifact says — that stays the orchestrator's
accountability, and a checker that was confidently incomplete would be worse than none.

## Install

Published on npm as [`@protonspy/scc`](https://www.npmjs.com/package/@protonspy/scc) —
the launcher pulls the right prebuilt binary for your platform as an optional
dependency, so there is no toolchain to set up.

```bash
npx @protonspy/scc help          # no install; pins nothing, always the latest
npx @protonspy/scc@0.0.1 help    # pin a version (CI)
npm i -g @protonspy/scc          # then: scc help
```

The package is `@protonspy/scc`; the command it installs is `scc`. Without `-g` it
lands in `node_modules/.bin`, which npm scripts see and your shell does not — reach it
there as `npx scc`.

Or from source (Go 1.25+):

```bash
go install github.com/protonspy/spec-claude-code/cmd/scc@latest
```

Prebuilt binaries for Linux, macOS, and Windows on x64/arm64 are attached to each
[release](https://github.com/protonspy/spec-claude-code/releases).

## Design

One surface: a headless CLI. Every capability is reachable through flags, with
`--json` output and a stable exit-code contract, so an agent or a CI job drives it
exactly as well as a human does.

| Exit code | Meaning |
|---|---|
| `0` | ok |
| `1` | usage or runtime error |
| `2` | the command ran and reported validation findings |

The artifacts it governs are plain Markdown and JSON in your repo — the harness's own
directory, `specs/`, `docs/`. No server, no database, no conversion layer: the files
*are* the API, and they are the ones your harness already reads. `scc` adds no
directory of its own and no config file — it owns a single manifest inside the
harness's directory, which doubles as the workspace marker.

## Development

```bash
make check    # the CI gate: gofmt + go vet + go test -race
make build    # -> ./scc
make help     # every target
```

See [CLAUDE.md](CLAUDE.md) for architecture and conventions.

## License

Apache-2.0. See [LICENSE](LICENSE).
