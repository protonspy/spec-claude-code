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
| `init` | Scaffolds the workspace and records what it wrote, then wires in the git hooks and RTK's usage block. Idempotent; never overwrites your edits. |
| `update` | Compares every managed file against this build, shows the plan, and applies it once you agree. |
| `spec new\|list\|show\|delete\|validate` | The three-artifact vehicle for work whose *what* and *how* need settling first. |
| `plan new\|list\|delete\|validate` | One file, for everything else: a checklist, a decomposition into specs, or both. |
| `skill validate` | Conformance to the published [Agent Skills](https://agentskills.io/specification) spec. |
| `validate` | Every applicable validator, one exit code, one JSON document. `--pr` also reads the pull request open on this branch; `--tests` also runs this project's suite. |
| `test\|test set\|test show` | The suite as a number: run the command this project recorded, read `{"total": N, "coverage": P}` back out of it, and hold the coverage against a floor. |
| `hooks install\|check\|remove` | Hooks that run the validators without anybody remembering to — git's (`scc validate` before a commit, the message checked as it is written, `scc validate --pr --tests` before a push) and the harness's own (findings and undelivered work handed back to the agent at the end of a turn). `init` writes both; anything scc did not write is left alone. |
| `rtk` | Wires in [RTK](https://github.com/rtk-ai/rtk) after the fact: installs it if missing, then splices its usage block into the entry file. |
| `launch` | Starts the harness with the workspace's symbol graph and RTK block current — and, with `--jail`, inside a sandbox. |

### Two kinds of hook, and only one of them refuses

A rule holds until the session that does not re-read it. A validator turns the rule
into a check, and a check holds until the session that does not run it. `scc init`
wires both places a check can run without anybody remembering it:

| | Where | What it does |
|---|---|---|
| **git** | `.git/hooks` | `pre-commit` runs `scc validate`, `commit-msg` reads the message being written, `pre-push` runs `scc validate --pr --tests`. These **refuse** — a commit is a decision with a natural place to stand in front of. |
| **harness** | `.claude/settings.json` | `SessionStart` says once that no test command is recorded; `Stop` hands `scc validate` findings and undelivered work back to the agent at the end of a turn — the moment they are cheapest to fix. |

The harness hooks **report and never refuse**: a hook that can stop a turn can also
loop one. They **never push and never open a PR** either — the Stop hook says the
branch has commits that never left the machine, and the agent pushes in its next
turn, in the open, where you can still stop it. Publishing on an event nobody is
watching is not a thing scc does on your behalf.

They cost nothing a turn cannot afford: no network, no test suite, and silence when
there is nothing to say. Only a harness with a hook surface gets them — Claude Code
today; Codex and opencode have no mechanism that would read one. scc splices into
`settings.json` beside whatever else is there, never rewrites a file it cannot
parse, and `scc hooks remove` takes back exactly its own entries.

### The tests, as a number

"Full suite + lint" was the one step in the delivery sequence nothing could check. A
rule can ask for a green suite and for tests that mean something; under
`autonomy: auto` nobody reads the answer. So the project records one command, that
command prints one object, and scc does arithmetic:

```bash
npx @protonspy/scc test set "make test-report"   # once, in the repo's own idiom
npx @protonspy/scc test                          # {"total": 412, "coverage": 88.4}
npx @protonspy/scc validate --tests              # the gate; exit 2 under the floor
```

The command has to print `{"total": N, "coverage": P}` somewhere in its output and
keep the suite's exit status. Anything can produce it — `go test` piped through a
script, `jest --coverage --json`, `pytest --cov` — because scc never learns how your
project tests itself. That is what keeps the check deterministic: it is a comparison,
not a judgment. `Makefile`'s own `test-report` target is the worked example.

`total` is there because coverage alone can be true of a suite that does not exist:
zero tests at 100% is a finding, not a pass. **The floor is 86%** unless the workspace
records another (`--min`), and the command lives in `<harness>/scc-manifest.json` — no
second config file.

It runs where it is worth its cost. A bare `scc validate` never touches your suite; the
`pre-push` hook does, which is where a branch becomes a pull request. Writing the
command is the agent's job — `/scc-init` derives it from the project's language, and
both "no command" and "the command printed no report" are findings that say what to
record rather than only what is wrong.

### RTK, from `init` onward

[RTK](https://github.com/rtk-ai/rtk) is a CLI proxy that filters command output down
to what is worth spending context on. The block in `CLAUDE.md`/`AGENTS.md` is what
makes it work at all — an agent that never read it never types the prefix — so
`scc init` writes it, and `scc rtk` is the same step for a workspace that already
exists:

```bash
npx @protonspy/scc init           # scaffolds, then wires RTK in
npx @protonspy/scc init --rtk     # …and builds it with cargo without asking first
npx @protonspy/scc init --no-rtk  # scaffold and nothing else
npx @protonspy/scc rtk            # wire it into a workspace that already exists
npx @protonspy/scc rtk --check    # CI: exit 2 when the block is missing
```

The install is the part that is still a decision — cargo, a Rust toolchain, minutes
of build — so a bare `init` asks before running it and takes silence for no.
Nothing is written when the binary is absent: guidance naming a command the machine
cannot run is worse than no guidance. `scc launch` does the same thing at the top of
a session, for the workspaces wired before this or scaffolded with `--no-rtk`.

The block sits between RTK's own `<!-- rtk-instructions -->` markers, which is what
makes `rtk init` and `scc rtk` converge on one copy instead of two. `scc rtk`
replaces what is there with the block this scc ships — same guidance, roughly a
fifth of the bytes, in a file preloaded into every request — and says so when the
one it replaced claimed a newer version; `--keep` leaves it alone. `init` and
`launch` always keep, because replacing somebody's block is a trade-off to make
deliberately rather than as a side effect. Everything outside the markers is
untouched either way, and `--no-install` writes the block without touching cargo.

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
superseding, glossary vocabulary drift, dependencies missing from `docs/stack.md`,
codewiki citations that no longer resolve, and an assistant's signature in the commits
this branch added. On request: the pull request's own title and body (`--pr`), and the
test report against its coverage floor (`--tests`).

What deliberately is **not** checked: your source code. scc never parses it, so it
cannot tell you the code honors what the artifact says — that stays the orchestrator's
accountability, and a checker that was confidently incomplete would be worse than none.
The coverage floor is the closest it comes, and it is deliberately a number your own
suite produced rather than a judgment scc made about your code.

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
