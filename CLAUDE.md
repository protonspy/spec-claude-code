# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`scc` is a single Go binary (`github.com/protonspy/spec-claude-code`) that enforces Spec-Driven Development inside an agent-driven repo — Claude Code, Codex, or opencode. One surface: a headless CLI, so an AI agent or a CI job drives it exactly as well as a human does.

The artifacts it governs (the harness's own directory, `specs/`, `plans/`, `docs/`) are plain Markdown/JSON. There is no server and no database — the files *are* the API. scc has no directory of its own: its state lives in one file under the harness's directory, alongside what that tool already keeps there.

This file covers working *on* scc. The product's own rules and methodology are not documented here: they live in `design/` while being designed, and ship as templates the binary scaffolds into `<harness>/rules/`.

Note: this repo is not itself an scc workspace (no harness directory, `specs/`, `plans/`, or `docs/` are committed) — those trees only exist in workspaces the binary scaffolds, and in test temp dirs.

**Status: v0.4.0-shaped.** Everything through `design/plan.md` phase 10 is built and green: scaffolding (`init`), artifact creation (`spec`, `plan`), and all eight validators behind `scc validate`. `init` also scaffolds the seven skills named in `design/orchestration.md` §6 — the six knowledge-base authors (one per `docs/` artifact a validator checks, plus `prd`) and the `plan-run` workflow skill — each with a `scc-`-prefixed slash command derived from the same list (`assets.Skills()`, which is `KnowledgeSkills` + `WorkflowSkills`), wherever the harness has a command surface.

These landed after phase 10, and all are documented in `design/orchestration.md` §6 and §12:

- **Three harnesses, one template set.** `scc init --claude|--codex|--opencode` (Claude Code is the default, and a terminal with no flag gets a picker). One prose source: paths come from a `paths.Harness` profile and the header each loader parses is synthesized at render time — YAML frontmatter for Claude Code and opencode, a TOML agent role file for Codex.

  The profile also carries `PreloadsRules`, because where the rules go is scc's choice but what the harness then does with them is not. Claude Code loads `.claude/rules/*.md` at launch with the same priority as `CLAUDE.md`; Codex and opencode load nothing from `rules/`, which is scc's own directory there. The entry file branches on it: told to "read the rule when the concern is live", an agent that already has all nine in context re-reads them, putting the same ~26KB in twice — and learns that the one document it is meant to trust is wrong about its own environment. Confirm per harness with `/context`.
- **`scc update` (phase 11), as replace-or-keep rather than the planned three-way merge.** It hashes every managed file against this build and against the manifest, prints the plan grouped by outcome, asks, and then replaces what is safe to replace. An edited file is kept and named; `--force` is the separate decision. `internal/merge` is still unbuilt.
- **`scc rtk`, and `scc init --rtk`.** Wires in [RTK](https://github.com/rtk-ai/rtk), the CLI proxy that filters command output: `cargo install` when the binary is missing, plus a splice of RTK's marker-delimited usage block into the entry file. **scc's block wins by default**, replacing whatever sits between the markers; `--keep` is the separate decision. The reason is size, not authorship: `rtk init` (measured on 0.42.4) writes 139 lines / 5140 bytes, scc ships 18 lines / ~900, both stamped `v2`, both saying the same thing — and the entry file is preloaded into every request of the session, so the difference is paid continuously rather than once. Between two blocks of the same version the condensed one is simply better, and leaving the larger one because it got there first is a standing cost. Where that costs something is a block claiming a *newer* version, which is a real downgrade: the run names the version it displaced and points at `--keep`. Opt-in in both places, since the block tells the agent to prefix every command with a binary the machine may not have. `--check` reports without writing and exits 2 when the block is missing.

  **scc uses RTK's own markers rather than namespacing its own**, and that is load-bearing: `rtk init` writes `<!-- rtk-instructions v2 -->` … `<!-- /rtk-instructions -->` into the project entry file (verified against rtk 0.42.4), so addressing the block by that pair is what makes `rtk init` and `scc rtk` converge on one copy. A namespaced `scc:rtk-instructions` would make each tool blind to the other's block and leave the file carrying both — which is the bug, not the fix. Headroom is the counter-example: it *does* namespace (`headroom:rtk-instructions`), which is why a workspace wired by both can end up with two blocks. scc detects that one and names it in the report (`rtkFile.Foreign`), with `headroom unwrap <agent>` as the fix — reported, never touched, because that block belongs to Headroom.
- **`scc launch <harness>`.** Starts the harness this workspace was scaffolded for, from the workspace root, behind [Headroom](https://github.com/headroomlabs-ai/headroom)'s compression proxy (`headroom wrap <slug>`), with the workspace's symbol graph brought up to date first. Headroom is the *default* here, which is the deliberate opposite of how RTK is wired: RTK edits a file the user owns and changes how every later command is typed, while Headroom wraps one process for the length of one session. So it degrades instead of failing — missing binary, declined install, unattended run, or a harness Headroom does not wrap all end in the agent starting bare with a warning saying why. `--no-headroom` forces that path, and a missing binary prompts for `uv tool install` (`--yes`/`--no-install` are the unattended answers). This is the one command that does not obey the 0/1/2 exit-code contract; see the convention below.

  Two things about `wrap` are load-bearing and were originally documented wrong here:

  **It does write to disk.** `headroom wrap` registers MCP servers into the agent's own config (`~/.claude.json` and the Codex/opencode equivalents), and those registrations outlive the session that made them — which is why Headroom ships `unwrap` at all. So `scc launch` defaults to **`--headroom-mcp retrieve`**: Headroom's own retrieve tool stays, because the proxy's compression markers are unactionable without it, and the code-memory server it would otherwise install does not, because code intelligence in an scc workspace is CodeGraph's job. `all` keeps Headroom's defaults; `none` drops the retrieve tool too.

  **It also wants the entry file, and scc says no.** `headroom wrap`'s context-tool setup appends RTK guidance to `$PWD/CLAUDE.md` or `$PWD/AGENTS.md` — the same file `scc rtk` splices — behind its own marker pair, `<!-- headroom:rtk-instructions -->`. Neither marker is a substring of the other, so both tools' idempotency checks pass and both append: an entry file carrying the same RTK instructions twice, in every request of the session. So `scc launch` passes `--no-context-tool` by default. Headroom already gates that injection behind `HEADROOM_RTK`, which makes this belt-and-braces — but only until an environment exports that variable for its own reasons, and `wrap claude` resolves `setup_context_tool = (context_tool or _rtk_opt_in()) and not no_rtk`, so the flag wins over both the env var and `--context-tool`. `--headroom-context-tool` hands it back.

  **The opt-out flags are discovered, not hardcoded.** `internal/headroom` reads `headroom wrap <agent> --help` and picks the spelling that build advertises. This is not defensiveness for its own sake: Headroom renamed this exact control once already (`--no-serena` → `--code-memory none`, and `--no-tokensave` in between), and the harness profiles disagree today — `wrap opencode` still takes `--no-serena` while `wrap claude` and `wrap codex` take `--code-memory`. A flag name compiled into scc turns that kind of release into a launch that dies on `no such option`, which is strictly worse than one unwanted MCP server. A build advertising no opt-out is reported, not overridden.

  **`--` reaches `wrap`, not only the agent.** `headroom wrap` parses every flag it recognizes out of the tail and forwards only the rest, so a pass-through argument that collides with one of Headroom's — `--verbose`, which both Claude Code and `wrap` define — is silently eaten. `WrapArgs` therefore takes scc's options and the pass-through as separate parameters and puts scc's first, so a colliding argument the user typed lands last and wins. To force something past `wrap` to the agent, use a second terminator: `scc launch claude -- -- -p`.

- **`scc graph`, and the launch-time index.** A wrap over [CodeGraph](https://github.com/colbymchenry/codegraph) — `build | sync | status | query | explore` — plus the same index run automatically by `scc launch`: `codegraph init` when `.codegraph/` is absent, `sync` when it is there. Launch is the one moment where indexing is free, and it degrades exactly the way Headroom does, for the same reason: a graph is an enhancement, so a missing binary or a failed index still starts the agent. `--no-graph` opts out and a plan-only run (`--json`/`--dry-run`) reports without indexing.

  What scc adds over typing `codegraph` directly is the two things it already knows: the workspace root, so `scc graph build` from `specs/` indexes the repo rather than a subtree, and whether the binary is there at all. The graph itself is *not* an scc artifact — not in the manifest, never touched by `scc update`, and `.codegraph/` stays CodeGraph's directory on CodeGraph's schedule. Unlike the launch path, a missing binary in `scc graph` is a hard error: the whole command is the binary.

  npm is the only installer scc will run. CodeGraph's headline install pipes a remote script into a shell (`curl … | sh`, `irm … | iex`), which is a fine thing for a person to type and not a thing scc executes on their behalf — `InstallHint` names it and leaves the decision where it belongs.
- **The plan as a contract (`design/plan-format-v2.md`).** A plan stopped being a document and became a header plus a checklist. Six sections and no others — `Why`, `Paths`, `References`, `Out of scope`, `Tasks`, `Done when` — because a closed set is the *only* thing that ever capped a plan's size: the 56KB plan measured below got there through `## Notes`, which nothing forbade, growing to half the file. There is no line limit anywhere in the contract except on the description, since a limit that fires on a legitimate plan is worse than the growth it prevents; what there is instead is nowhere for prose to go.

  **`## Decomposition` became `## References`, and that cost almost nothing** — `parseLeaves` recognizes a leaf by the `specs/<feature>/` citation *anywhere in the file*, not by the heading above it, so `Leaf`, the `specs/foo/` address and `map trace` all kept working untouched. What closes the door on leaves appearing elsewhere is `plan.unknown-section` itself.

  **A task gains four flags and no more** — `_Depends_`, `_Priority_`, `_Status removed_`, `_Reason_` — and the vocabulary is closed because an italic one-liner is a shape prose also uses: a parser that absorbed any of them would silently eat a sentence and hand the task a region that is not the task. An unknown one is `task.unknown-flag`, reported and left where it sits — since the box is the state, a flag that could restate it is the `item-has-two-records` defect arriving by another door, and there is no `_Blocked_` because that is derived from `_Depends_`. `(Unit)`/`(TDD)` is reused as the test strategy rather than a new `_Test_` flag — zero migration, and the concept already had a name. `_Status_` never takes `open` or `completed`.

  Three consequences had to land together or the result is worse than before: `Task.End` covers the flags (so `map show` returns them and `patch rm` removes them), `Detail` excludes them (so the searcher does not index `_Priority 2_` as prose), and **`renderTask` re-emits them plus the continuation** — without that, `patch task --method TDD` was a data-loss command that deleted a sixty-line description and every dependency the task declared.

  **The reading surface is what gives "never read the plan" its authority.** `map brief` is the header, `map tasks` is the checklist, and no command returns both — so a session pays `brief` once and `--next` per task instead of ~14k tokens per reread. Forbidding the read without offering the equivalent query produces an agent that disobeys the rule, correctly — so the surface shipped in the phase before the rule did. `--next` is now determined (eligible → priority ascending, absent last → number compared *numerically*, which is also the fix for `1.10` sorting before `1.9`), and `--ready`/`--blocked`/`--deps` share that one implementation, because two notions of eligibility would be two answers to "what do I work on".
- **`scc plan approve|reseal|migrate`, and the seal.** `approve` validates, then writes `status: approved` and a `checksum:` over the file minus its own checksum line, LF-normalized. It is **tamper-evidence, not prevention** — `reseal --force` is one command away and sha256 is public — and it is recorded that way here so nobody builds a guarantee on it later. The check runs before an edit is applied, which is the whole value: a harness that edited by hand and then ran `patch check` would otherwise have its edit resealed by the command that should have reported it. A plan with no `status:` is never checked, which is what makes every pre-existing plan keep working.

  After approval the work is fixed and only discovery moves: `add` allocates the number (high-water mark including removed tasks, so nothing is stored anywhere) and demands `--reason`; `rm` strikes the task out where it stands rather than deleting it; rewriting a task or the prose is refused. What discovery can never touch is guaranteed structurally rather than by instruction — `Why`, `Out of scope`, `Done when` and the title are reachable only through `append`/`prepend`/`replace`, and those are exactly the three refused.

  `migrate` moves a v1 plan across: it renames `Decomposition`, moves every other heading to `plans/archive/<name>-notes.md` (safe because the plan scanner uses `ReadDir` and skips directories), creates the missing required sections **empty and lets the findings appear** — a placeholder that satisfied the validator would be a plan that lies — and writes `status: draft`, never `approved`.
- **`scc map` and `scc patch`.** The artifacts are structured documents that happen to be Markdown, and the cost of treating them as prose is paid on every request rather than once: measured on a real workspace, one plan is 56KB and its 31 specs bring the corpus to ~90k tokens, so an agent answering "what is the next open task?" by reading the file carries the whole plan for the rest of the session. `map` turns a file into addressable pieces — `index | outline | brief | tasks | show | blocks | find | trace` — and `patch` changes one of them — `check | uncheck | task | add | rm | append | prepend | replace | fm`.

  **The addresses are the design.** A task is `1.2`, a requirement `R1.2`, a section `#notes`, a leaf `specs/<feature>/`, a paragraph `notes:7`; `L120-160` is the escape hatch and the only form that is a line number. That is what lets `patch` write into a file nobody read: a line number stops being true the moment anything above it moves, so an editor addressing by line has to read first — which is the cost the package exists to remove. The guard that reading-first was providing is replaced by three that are stronger for a structured file: an address that does not resolve is an error and never an insert at a guess, the file is re-validated afterwards and **rolled back if the edit introduced a finding** (exit 2), and the displaced and written lines are printed back — elided past a few lines, because a confirmation that echoed 400 lines would put the file in context by the back door.

  Two things measurement decided rather than taste. **`blocks` exists because section addressing bottoms out**: that plan's `## Notes` is 411 lines — half the file — with no headings inside it, but every paragraph opens with a bolded thesis, so the leads alone are an index a twentieth of the size. And **a requirement id is scoped to its spec**: `R2.5` is defined in nine of those 31 specs, so `map trace R2.5` unscoped answers with the list of specs and stops rather than concatenating nine traces.

  **`internal/artifact` owns the grammars, and `internal/validate` consumes them.** The task grammar used to live in the validator; a reader that disagreed with the validator about what a task is would be worse than no reader. The parser now states facts about a line (`Methodologies`, `Loose`, `HasCitation`) and turning a fact into a finding stays in `validate` — which is also what lets `map` read a malformed artifact instead of refusing exactly the file a user most needs to inspect.

  **No search engine.** The obvious reach for `find` is an inverted index; at 352KB and 94 artifacts a linear pass ranks the whole workspace in 55ms, and Tantivy or its kin would cost a CGO surface or a second binary against a stdlib-only `go.mod` and a six-platform cross-compile. What precision needed here was not a better index but a better *unit*: BM25 over addressable regions rather than lines, so a hit comes back as something `show` accepts. The seam is `artifact.Search` — it takes artifacts and returns hits, and nothing outside that file knows how it found them.

  **`map find` is now undocumented rather than removed.** Its stated reason was the whole corpus (94 artifacts, 352KB), not the plan — and with the plan small, searching *inside* one stopped making sense, while searching `design.md` and the knowledge base is still the only alternative to reading a file. So `runMapFind` and `search.go` stay and the line comes out of `rules/artifacts.md`, `entry.md` and `mapUsage()`: deleting the code saves nothing, and deleting the line saves tokens in every request of every session.
- **`caveman.md`, the register the agent answers in.** The output budget belongs to the code: prose about the work is written once and then carried in every later request of the session, so narration is the part of a long run that can be cut without losing a fact. It ships as a *rule* rather than a skill because it is on by default, and a default the model has to decide to load is not one — the cost is what every rule costs, since the harnesses that read `rules/` preload it. One level (ultra) rather than a dial, because three descriptions of the register are three things to keep true instead of one, and nobody turns the dial.

  What it must never compress is the line that keeps it honest: artifacts under `specs/`, `plans/`, `docs/`, anything a validator parses or a shell runs, quoted output, commit and PR bodies, and questions asked of the user. A denser EARS line is a finding, not a saving.

  **The language is the third kickoff answer.** `autonomy.md` asks it with the other two and it lands in the artifact's frontmatter as `lang: en|wenyan`, graded by `checkKickoffAs` on exactly the terms `autonomy` and `ci` are — checked when present, absent meaning the run predates the question. There is no `--lang` flag on `spec new` or `plan new`: it is the one answer that can arrive after the file exists, so `scc patch fm <artifact> lang=wenyan` is the whole path to it, and a value neither the rule nor the validator knows is rolled back like any other bad edit. `TestTheRuleOffersEveryKickoffAnswerThisAccepts` is what stops the rule and the validator from naming different values.
- **The four seeded `docs/` anchors** (`assets.Seeds`). `init` writes `glossary.md`, `stack.md`, `wiki/index.md`, and `wiki/changelog.md` — the knowledge base's only fixed-name documents, each holding the format its validator checks. A seed is written once and tracked nowhere: not in the manifest, not by `scc update`.

`scc` is a redesign of `csdd` (`github.com/protonspy/csdd`), narrowed to spec-driven development and deliberately leaner. When reaching for something from there, port the *decision*, not the file. Already decided against: a TUI, an embedded web dashboard, an MCP server, a devcontainer.

## Commands

```bash
make build          # -> ./scc  (set VERSION=vX.Y.Z to stamp the version)
make check          # the CI gate: gofmt -l + go vet + go test -race
make test           # go test -race -coverprofile=coverage.out ./...
make fmt            # fails if anything is not gofmt-clean
make help           # every target with its one-line description
```

A single test / package:

```bash
go test ./internal/workspace/ -run TestSafeName -v
go test ./internal/cli/ -run 'TestVersion.*' -race
```

Lint locally the way CI does (golangci-lint v2 schema, conservative set in `.golangci.yml`):

```bash
golangci-lint run          # v2.12.2 in CI
govulncheck ./...          # CI fails the build on reachable vulns
```

Release is manual and CI-driven: `make release VERSION=v0.1.0` dispatches `release.yml`. The `dist`/`npm-build`/`npm-publish` targets exist as a bootstrap/fallback path only.

## Architecture

```
cmd/scc/main.go         os.Exit(cli.Run(os.Args[1:]))
        |
   internal/cli         the whole command surface
        |
   scaffold · validate                 write / check
        |          \
   assets · manifest   ears · mdscan · artifact   templates, hashes, grammars
        |
   paths · workspace · render · textutil · finding
        |
   plain files on disk: <harness>/ · specs/ · plans/ · docs/ · CLAUDE.md|AGENTS.md
```

Three packages sit off to the side of that tree — `rtk`, `headroom`, `codegraph` — reached only from `internal/cli`. They are the third-party integrations, and they are the only code that starts another process.

`internal/cli/cli.go` is the whole dispatcher: `Run(args)` switches on `args[0]` and hands off to `run<Resource>` in a file named for that resource. Each handler owns its own `flag.FlagSet`. Adding a subcommand means adding a case there plus one file — nothing is registered dynamically, so the command set is readable in one place.

| Package | Role |
|---|---|
| `internal/paths` | Every directory/file name in the on-disk layout, in one place, plus the `Harness` profile (`Claude`, `Codex`, `OpenCode`) that says where each tool keeps things — and, in `PreloadsRules`, what it does with them. Never hardcode `".claude"` or `"specs"` elsewhere — the harness-relative paths are methods on `Harness`. |
| `internal/workspace` | Resolves the root by walking up for *any* harness's `scc-manifest.json` marker; `Harnesses(root)` says which trees exist. Owns `KebabCheck`, `SafeName`, `AtomicWrite`. Knows nothing about specs or wikis. |
| `internal/render` | CLI terminal output (`✓ ✗ ! •`, `NO_COLOR`/TTY aware), split across stdout/stderr. |
| `internal/textutil` | Line-ending and BOM normalization, in exactly one place. |
| `internal/finding` | One finding type and one frozen JSON shape (`{findings, count}`) for every validator, plus the grouped human report. |
| `internal/manifest` | `<harness>/scc-manifest.json`: `{path, hash, version}` per managed file plus the harness, deterministic serialization, `Status → pristine\|edited\|missing`. Unknown fields are preserved. Every call takes the `paths.Harness` whose manifest it means. |
| `internal/assets` | The embedded template set — rules, review agents, skills, slash commands, artifact templates. **Workspace templates are data-free except for the harness profile** (a `(version, harness)` pair still renders byte-identically everywhere, and the manifest records both, so the future three-way merge can still reconstruct the old side); **artifact templates take data** (`spec new` renders them and the user owns the result); **seeds are the `docs/` anchors** — data-free like a workspace file, untracked like an artifact. `Render(h, file)` is the only way to get a workspace file's bytes: it expands paths and synthesizes the per-harness header for agents and commands. `Version` is the template-set version and must be bumped whenever a workspace template changes. |
| `internal/scaffold` | Applies the template set to a root (`Apply`) and brings an existing one current (`PlanUpdate`/`ApplyUpdate`). Idempotent, never overwrites without being told to, manifest written last. |
| `internal/mdscan` | The only Markdown parser: fence- and HTML-comment-aware headings, checkboxes, links, wikilinks, slugs, plus a small frontmatter reader. `Body` is the comment/fence-stripped text every validator applies its grammar to. |
| `internal/artifact` | The navigable model of one artifact, layered on `mdscan`: sections (two ends — the subtree, and the body before the first child), tasks with their continuation *and their flags*, requirements, spec-reference leaves, paragraph blocks. Owns **every grammar** (task, requirement, spec reference, flag), `Find` for address resolution, `Editor` for line splices resolved against the original and applied bottom-up, `Search`, the schedule (`Ready`/`BlockedTasks`/`Next`/`Cycles`, one implementation shared by `--next`, `--ready` and `--blocked`), and the seal. Knows nothing about findings or exit codes. |
| `internal/ears` | EARS requirement parsing, all five patterns plus complex. |
| `internal/validate` | The eight validators, one file each, sharing `mdscan` and `finding`. The exception is `stack_manifests.go`: the seven dependency-file readers age on their own schedule, so they sit beside the rule rather than inside it. |
| `internal/rtk` | RTK's marker pair and the idempotent splice of its block into the entry file, plus finding or `cargo install`ing the binary. |
| `internal/headroom` | Headroom's agent-slug table, the `wrap` argument vector, the MCP opt-out discovered from `wrap <agent> --help`, and finding or installing the binary (uv, then pip — never npm, which ships the SDK and no CLI). The slugs live here rather than on `paths.Harness` because they are Headroom's vocabulary, not scc's layout. |
| `internal/codegraph` | CodeGraph's argument vectors (`init`/`sync`/`index`/`status`/`query`/`explore`), the `.codegraph/` presence test, and finding or `npm install -g`ing the binary. Composes command lines and reads nothing inside the graph — the database is CodeGraph's schema on CodeGraph's schedule. |
| `internal/cli` | The dispatcher and every command handler. |

`internal/rtk`, `internal/headroom`, and `internal/codegraph` are the only packages that shell out to another program. Keep that boundary there rather than in a command handler: a third party's binary name, install command, and argument vocabulary all age on that third party's schedule, and one package per integration is what keeps a version bump from touching the dispatcher. Headroom's renamed MCP flag is the worked example — the fix stayed inside `internal/headroom`, and nothing else in the tree knows the flag exists.

`go.mod` is stdlib-only. Keep it that way unless a dependency earns its place — the binary is distributed to six platforms and every dep is a supply-chain surface.

## Conventions

**Exit codes are the contract.** `0` ok · `1` usage/runtime error · `2` validation findings. Every lint/validate command returns `2` on findings so CI and agents can branch on it. A finding is a legitimate answer to a lint question, not a failure of the tool — don't collapse `2` into `1`.

`scc launch` is the single exception, and it has to be: it returns whatever the agent it started returned. A launcher that flattened the exit status of what it launched into its own vocabulary would be unusable in the scripts people actually write. scc's own failures — no workspace, unknown harness, binary not on PATH — still happen before anything starts and still report `1`.

**A validator that fires on scc's own output is the worst bug in the product.** The templates carry their instructions in HTML comments and fenced examples, which is exactly what `mdscan` excludes — and `TestFreshArtifactsPassTheirOwnValidators` in `internal/cli` is the gate. Treat it as required reading before changing a template or a validator: one wrong finding teaches the user to disbelieve all eight.

**Machine-readable output.** Bind the `--json` flag via `addJSON` and emit through `emitJSON` (`internal/cli/jsonout.go`) so the flag name, help text, and stdout/stderr split stay identical across commands — stdout is a clean JSON stream, diagnostics go to stderr.

**Hostile input.** Any positional name that becomes a path segment must pass `workspace.SafeName` *before* `filepath.Join` — without it `scc spec delete .. --force` resolves to the workspace root. Resource names are kebab-case (`workspace.KebabCheck`).

**Writes are atomic.** Use `workspace.AtomicWrite` for anything a concurrent reader might see.

**The rules are a standing budget, not a place to explain things.** Every file under `<harness>/rules/` is preloaded into every request on a harness that reads them, so a line added there is paid continuously rather than once. `TestRulesStayShortEnoughToBePreloaded` caps a rule at 55 lines (three predate the budget and are capped where they stand; they may shrink, never grow), and `TestScaffoldedEntryFileStaysShort` caps the entry file at 60. When a feature does not fit, the answer is to move the detail into a `--help` string — read only when consulted — and keep the question→command table in the rule. The plan-format change was measured in and out on this basis: it landed at +2 lines and +709 bytes across `entry.md` + `artifacts.md` + `tasks.md`, against ~14k tokens saved per plan reread.

**A command that edits an artifact verifies it afterwards.** `scc patch` snapshots the file, writes, re-runs the validator that owns it, and restores the snapshot if the edit introduced a finding the file did not already have. Two details are load-bearing: the comparison is on `rule + message` and deliberately **not on line number**, because an insertion moves every finding below it and comparing on line would blame this edit for the whole tail of a pre-existing problem; and an artifact scc has no validator for is written and *reported as unverified* rather than silently claimed clean.

**The marker is the file `<harness>/scc-manifest.json`, never the harness directory.** Two reasons, and both are load-bearing: every harness has a global twin in the user's home (`~/.claude`, `~/.codex`, `~/.config/opencode`) that exists on any machine running that tool, so an upward walk accepting the *directory* would resolve the root to `$HOME` for any command run outside a workspace — every command would then read and write the user's global configuration. And those directories exist in every repo that merely *uses* the tool, where scc was never initialized. `workspace.Find` therefore stats a regular file, for each harness in turn.

**scc has exactly one file per harness and no config file.** The manifest is it — content hashes, doubling as the marker. scc runs no tests and no linters, so it has nothing to configure; a project's test and lint commands are a rule under `<harness>/rules/`, which is Markdown the orchestrator already reads. Resist adding `scc.json`: a JSON schema to version, read by nothing inside the binary, is dead weight.

**Adding a harness touches one place.** A new `paths.Harness` value plus its entry in `paths.Harnesses()` is the whole change — `init`'s flags, the picker, `update`'s targets, the skills validator's search path, and the workspace walk all derive from that list. If a change needs a `switch h.ID` outside `assets.renderAgent`/`renderCommand`, the profile is missing a field.

**Never author what the user owns.** Upgrades preserve user-edited files rather than overwriting them; the manifest of content hashes is what makes a pristine file distinguishable from an edited one.

**Determinism.** Normalize rendered output to LF (`textutil.NormalizeNewlines`) so scaffolded files hash identically regardless of the build machine's checkout. Sort before serializing.

**Windows is a first-class target** and CI runs the suite on Linux/macOS/Windows. `.gitattributes` forces LF everywhere — CRLF would break gofmt, manifest hashes, and shell hooks. Watch for path separators, file-locking/delete-pending behavior, and the fact that the race detector doesn't run on the Windows job.

## Testing

Tests live beside the code and lean on a few package-local helpers rather than a framework:

- `capture(t, f)` in `internal/cli/cli_test.go` swaps `os.Stdout`/`os.Stderr` for pipes — the only safe way to assert on CLI output, since `render` writes to the real files. `run(t, args...)` wraps it around `cli.Run`. It drains both pipes concurrently, so a command that outruns the pipe buffer fails the test instead of deadlocking.
- `cli.Run` returns an exit code and never calls `os.Exit`, so the whole surface is drivable in-process.
- Compare resolved paths with `os.SameFile`, not string equality: `t.TempDir()` can sit under a symlink (`/var` → `/private/var` on macOS) and Windows reports 8.3 short names.
- `_test.go` files are exempt from `errcheck`/`staticcheck` in `.golangci.yml` — deliberate, for patterns like `Hash("a") != Hash("a")` asserting determinism.

## Releasing

`release.yml` is `workflow_dispatch` only: validate the version → run the full CI gate → cross-compile 6 targets → publish the npm packages → tag + GitHub Release. Details that are load-bearing:

- **A version is immutable.** Re-dispatching an already-released version from a *different* commit is refused, because publishing is idempotent and the run would otherwise go green having shipped nothing.
- **Publishing is idempotent.** Already-published packages are skipped, so a run that died after `npm-publish` can be resumed by re-dispatching the same commit.
- **Adding a platform touches three places** that must agree: `TARGETS` in `npm/scripts/build-packages.mjs`, `PLATFORMS` in the `Makefile`, and the `build` matrix in `release.yml`.
- **The launcher is published under every name in `LAUNCHERS`**, from one source, in two tiers. A `required` name is already ours — `@protonspy/scc` is the documented install, and a failure there fails the release. A non-required name is one being tried for the first time: attempted last, allowed to fail with a warning. Both tiers are directories (`npm/dist/launchers/`, `npm/dist/launchers-optional/`) so publish order *and* failure policy are visible in the layout, after `dist/scc-*/` — a launcher reaching the registry ahead of the binaries in its `optionalDependencies` is a broken install for anyone in that window.

  The optional tier exists because **npm's typosquatting similarity check runs only on a real publish**: `npm view` returning 404 means unregistered, not publishable, and `npm publish --dry-run` never reaches the check. v0.9.0 died on `403 — Package name too similar to existing package cp-cli` for a name both of those had called free, and because that name published first, `set -e` took the working launcher with it. Corollary, and it is the load-bearing half: **only a required name may appear in documentation**, the embedded `entry.md` included. Promote a name into the docs in the release *after* the one that proved it publishes.
- Actions are pinned by commit SHA. Keep them pinned.

## Commits

Conventional Commits, scoped by package or surface, with a descriptive subject written as a claim about behavior — e.g. `feat(cli): return exit 2 when spec validation reports findings`. Changes land through PRs on `main`.
