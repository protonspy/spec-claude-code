# scc

Spec-driven development for Claude Code — a single Go binary that turns the SDD
workflow into a mechanically validated contract for humans and AI agents.

> **Status: pre-1.0.** Scaffolding, artifact creation, and validation all work.
> `scc update` — bringing an existing workspace onto improved templates through a
> three-way merge — is not built yet; until it is, re-running `scc init` is the
> upgrade path. It is additive and never overwrites a file you edited, so it is safe,
> but it also will not deliver improved templates to files that already exist.

## Use it

```bash
scc init                        # scaffold .claude/rules/, the review agents, the layout
scc spec new user-auth          # specs/user-auth/: requirements.md, design.md, tasks.md
scc plan new checkout-revamp    # plans/checkout-revamp.md
scc validate                    # every check; exit 2 means it found something
```

| Command | What it does |
|---|---|
| `init` | Scaffolds the workspace and records what it wrote. Idempotent; never overwrites your edits. |
| `spec new\|list\|show\|delete\|validate` | The three-artifact vehicle for work whose *what* and *how* need settling first. |
| `plan new\|list\|delete\|validate` | One file, for everything else: a checklist, a decomposition into specs, or both. |
| `skill validate` | Conformance to the published [Agent Skills](https://agentskills.io/specification) spec. |
| `validate` | Every applicable validator, one exit code, one JSON document. |

What gets checked: EARS grammar across all five patterns, requirement numbering,
one methodology annotation per task, traceability in both directions, plan
one-source-of-truth, skill conformance, wiki link/orphan graph, ADR numbering and
superseding, glossary vocabulary drift, dependencies missing from `docs/stack.md`, and
codewiki citations that no longer resolve.

What deliberately is **not** checked: your source code. scc never parses it, so it
cannot tell you the code honors what the artifact says — that stays the orchestrator's
accountability, and a checker that was confidently incomplete would be worse than none.

## Install

```bash
npx @protonspy/scc help          # no install
npm i -g @protonspy/scc          # then: scc help
```

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

The artifacts it governs are plain Markdown and JSON in your repo — `.claude/`,
`specs/`, `docs/`. No server, no database, no conversion layer: the files *are* the
API, and they are the ones Claude Code already reads. `scc` adds no directory of its
own and no config file — it owns a single manifest inside `.claude/`.

## Development

```bash
make check    # the CI gate: gofmt + go vet + go test -race
make build    # -> ./scc
make help     # every target
```

See [CLAUDE.md](CLAUDE.md) for architecture and conventions.

## License

Apache-2.0. See [LICENSE](LICENSE).
