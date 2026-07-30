# Implementation plan — from `orchestration.md` to a shipped binary

`orchestration.md` says what scc *is*. This says in what order it gets built, what
each step delivers, and where the design still owes an answer before code can be
written.

It is a plan in the §1 sense — a decomposition, plus a checklist for the items it
just does itself — but it lives in `design/` rather than `plans/` because this repo
is not an scc workspace (no `.claude/`, `specs/`, or `docs/` are committed). When
scc can scaffold itself, this file migrates.

Section references (§n) are to `orchestration.md` throughout.

**Status: phases 1–10 are built and green; phase 11 (`scc update`) is not started.**
The command surface is `init`, `spec new|list|show|delete|validate`,
`plan new|list|delete|validate`, `skill validate`, `validate`, `version`, `help`. §9
records what the build settled that the plan left open, and where it deviated. Every
decision listed in §5 has been folded back into `orchestration.md`.

## 0 · Ground rules for every phase

- **`make check` stays green at every commit.** gofmt + vet + `go test -race`, on
  Linux/macOS/Windows.
- **One phase ≈ one to three PRs**, Conventional Commits scoped by package
  (`feat(manifest):`, `feat(cli):`). No phase lands half a package.
- **`go.mod` stays stdlib-only.** Three places below would be easier with a
  dependency — YAML frontmatter, diff3, TOML manifests — and each one is solved
  narrowly instead (§0's "every dep is a supply-chain surface").
- **Nothing reads source code.** The enforcement boundary in §5 is a design
  constraint on this plan, not a later concern: no phase adds a parser for a
  project's own language.
- **Every validator ships with its false-positive argument.** §5's discipline
  ("a false positive costs more than a miss") is checked in review by asking, per
  check, *what input makes this wrong?* If the answer isn't "none", the check
  stays silent instead.

## 1 · Where the code already is

Green and done: build/test/release infrastructure, and the four foundation
packages — `paths`, `workspace`, `render`, `textutil`. The command surface is
`version` and `help`. `cli.Run` returns an exit code and never calls `os.Exit`,
so the whole surface is drivable in-process from tests.

What that gives this plan for free: root resolution via the marker file, hostile-input
name checks (`SafeName`, `KebabCheck`), atomic writes, LF/BOM normalization, the
`0/1/2` exit contract, and the `--json` convention (`addJSON` + `emitJSON`).

Two gaps in the current state worth naming, both closed in Phase 2: `paths` has no
`.claude/rules/` segment and no `docs/` segments, and nothing embeds a template yet.

## 2 · The surface being built

| Command | Phase | What it does |
|---|---|---|
| `scc init` | 2 | Scaffold the workspace; write the manifest. Idempotent. |
| `scc spec new\|list\|show\|delete` | 4 | Create and manage `specs/<feature>/`. |
| `scc plan new\|list\|delete` | 4 | Create and manage `plans/<name>.md`. |
| `scc skill validate` | 6 | Agent Skills conformance. |
| `scc spec validate` | 7 | EARS, numbering, one methodology per task, traceability, deltas. |
| `scc plan validate` | 8 | Checklist grammar, references resolve, one source of truth. |
| `scc wiki\|adr\|glossary\|stack\|codewiki validate` | 9 | The knowledge base. |
| `scc validate` | 10 | Every validator, one exit code, one JSON document. |
| `scc update` | 11 | Three-way merge onto a new template version (§12). |

Resource-first, matching the dispatcher's existing shape: `Run` switches on
`args[0]`, hands off to `run<Resource>` in a file named for that resource, and
each handler owns its `flag.FlagSet`.

New packages, in dependency order:

| Package | Phase | Role |
|---|---|---|
| `internal/manifest` | 1 | Entries of `{path, hash, version}`; deterministic (de)serialization; pristine-vs-edited. |
| `internal/assets` | 1 | The embedded template set (`//go:embed`) and its rendering. |
| `internal/finding` | 1 | One finding type and one JSON shape for all eight validators. |
| `internal/scaffold` | 2 | Apply the template set to a root, recording what it wrote. |
| `internal/mdscan` | 5 | Fence-aware Markdown scanning: headings, checkboxes, links, frontmatter, slugs. |
| `internal/ears` | 7 | Parse a requirement into EARS clauses; all five patterns plus complex. |
| `internal/validate` | 6–10 | One file per validator, sharing `mdscan` and `finding`. |
| `internal/merge` | 11 | Line-level diff and diff3 with git conflict markers. |

`internal/validate` is one package with a file per validator rather than eight
packages, so the shared helpers (resolve a path, read-and-normalize, emit a
finding at a line) stay unexported and identical across all eight — which is what
keeps message wording and finding shape from drifting.

## 3 · Why this order

Three constraints fix the sequence almost completely.

**The manifest is upstream of everything that writes.** `init` cannot record what
it wrote without it, and `update` cannot tell a pristine file from an edited one.
So Phase 1.

**Scaffolding mechanism before scaffolded content.** The two fail differently and
review differently: `init` is code (idempotence, atomicity, Windows paths), the rules
and templates are prose that has to be read line by line. Splitting them keeps a
400-line Markdown PR from hiding a bug in the writer, and vice versa.

**Validators need their subject to exist.** `spec validate` is testable against
fixtures on day one, but the check that actually matters — that the *shipped*
templates and skills pass their own validators — needs the content from Phase 3.
Within the validators, order by increasing false-positive risk: `skill` is an
external, mechanical contract; `codewiki` resolves citations against a live
checkout and is the most likely to be wrong.

`update` goes last for a reason that isn't difficulty: **an upgrade is untestable
until two template versions exist.** Shipping it before there is a real diff
between v0.1.0 and v0.2.0 means shipping it tested only against synthetic
history.

## 4 · The phases

### Phase 1 — Foundations (3 PRs)

**1.1 `internal/manifest`.** `Entry{Path, Hash, Version}`, a manifest holding
entries sorted by path, `Load`/`Save` (through `workspace.AtomicWrite`),
`Hash(content)` as SHA-256 over `textutil.NormalizeNewlines`, and
`Status(root, entry) → Pristine | Edited | Missing`.

Load-bearing details, each of which is a test:

- **Paths are stored slash-separated and relative to the root.** A manifest
  written on Windows must be read on Linux — this is the one place where the
  layout crosses machines.
- **Serialization is deterministic.** Sorted keys, LF, trailing newline: the same
  workspace on two machines produces a byte-identical manifest, or every upgrade
  shows a spurious diff.
- **The hash is of the *pristine render*, not of the file on disk.** That is what
  makes "did the user edit this?" answerable, and Phase 11 depends on it being
  this and not the other thing.
- **Unknown fields are preserved, not dropped.** An older binary reading a newer
  manifest must not silently delete what it doesn't understand.

**1.2 `internal/assets`.** `//go:embed templates/**` plus `Render(name, data)`,
normalizing to LF. A golden test asserts every embedded file is LF-clean, non-empty,
and ends in a newline.

One decision here that Phase 11 leans on: **scaffolded templates are data-free.**
No project name, no date, no path interpolated into a rule or a spec template. Then
a template version renders byte-identically in every workspace, its hash identifies
its content globally, and an upgrade can reconstruct the old text from the manifest
alone. The files that genuinely want per-project content — `CLAUDE.md`, and the rule
holding the project's test and lint commands — are exactly the files §12 excludes
from the merge because the user owns them, so the two rules agree instead of
fighting.

**1.3 `internal/finding`.** `Finding{File, Line, Rule, Message}`, a set with `Add`
and a stable sort by `(file, line, rule)`, a frozen JSON shape
(`{"findings": [...], "count": n}`), human output through `render`, and
`ExitCode()` returning `ExitOK` or `ExitFindings`. `Rule` is a stable slug
(`ears.missing-shall`, `skill.name-mismatch`) so a CI job can filter without
matching prose.

### Phase 2 — `scc init`, the mechanism (2 PRs)

**2.1 Layout.** Add to `paths`, in one commit: `RulesSeg = "rules"` and the `docs/`
segments (`wiki`, `adr`, `raw`, `codewiki`, `glossary.md`, `stack.md`). This is the
commit CLAUDE.md is waiting for — "the domain segments land there as `design/`
settles".

**2.2 `internal/scaffold` + `cli/init.go`.** Walk the embedded set, write each file
that is missing, record every managed file in the manifest, report what it did.

- **Idempotent.** A second `init` writes nothing and exits `0`.
- **Never overwrites.** An existing file is skipped whether it is pristine or
  edited — §0's "never author what the user owns". `--force` overwrites, and names
  every edited file it is about to clobber.
- **The manifest is written last**, so a crashed `init` leaves no workspace marker
  and the retry is clean rather than half-adopted.
- Flags: `--root`, `--json`, `--force`.

Tests: init into `t.TempDir()`; every expected path exists; `workspace.Find` from a
nested subdirectory resolves back to it; second run is a no-op; an edited file
survives; `--json` reports per-file actions; compare resolved paths with
`os.SameFile`, never string equality.

### Phase 3 — The template set (2–3 PRs)

The product's actual content. `.claude/rules/`, one file per concern, because §6's
whole point is that a rule is read when it is relevant:

| File | Carries |
|---|---|
| `routing.md` | §1 — the two vehicles and the routing question. |
| `autonomy.md` | §2 — ask once at kickoff; automatic still escalates on `(TDD)` risk. |
| `methodology.md` | §3 — impact analysis first, then Unit or TDD; when TDD is mandatory. |
| `tasks.md` | §4 — the grammar, and "independently verifiable" as the size rule. |
| `verification.md` | §7 — build, scoped tests, lint, fix; scope not suite. |
| `delivery.md` | §9 — branch in a worktree, review before the PR, the CI question, degrading. |
| `specs.md` | §10–§11 — EARS, deltas, design scaled by complexity, spec-anchored. |
| `knowledge-base.md` | `docs/` — what belongs in the wiki, in an ADR, in the glossary, in the stack. |
| `project.md` | The project's own test and lint commands. Shipped as a stub, owned by the user from first edit. This file is why scc needs no config file. |

Plus `CLAUDE.md` (small, links out — §6's context-rot argument is the reason, and
it is worth stating inside the file so nobody "helpfully" inlines the rules), the
two review agents from §7 (`code-review`, `security-review` — split by lens
deliberately), the spec templates (`requirements.md`, `design.md` with its optional
sections marked *optional*, `tasks.md`), and the plan template.

Two things to get right here, both of which the design already argues:

- **The design template must not require sections.** §10: a required heading is a
  request for filler, and invented architecture constrains the next session. The
  template marks structural sections optional and says why.
- **Shipped skills are validated by our own validator.** Whatever skills ship get a
  test in Phase 6 asserting `scc skill validate` finds nothing in them. A tool that
  ships non-conforming skills has no standing to check anyone else's.

### Phase 4 — Creating specs and plans (2 PRs)

`scc spec new|list|show|delete`, `scc plan new|list|delete`.

- Every positional goes through `SafeName` **and** `KebabCheck` before any
  `filepath.Join` — `scc spec delete ..` must not resolve to the workspace root.
- Commands require a workspace (`workspace.IsWorkspace`) and otherwise exit `1`
  saying to run `init`.
- Creating an existing artifact fails with `workspace.ErrAlreadyExists` unless
  `--force`.
- `list` output is sorted; `--json` on all of them.

**Spec and plan files are not recorded in the manifest.** They are authored from
birth — created from a template and then owned — which is precisely §12's excluded
category. The manifest stays a record of *scc-managed* files, which is what keeps
`update` from ever touching a requirement.

`new` also takes `--autonomy=auto|gated` and `--ci=wait|no-wait`, written into the
artifact's frontmatter. See §5 below — this is a decision this plan makes, not one
the design states.

### Phase 5 — `internal/mdscan` (1 PR)

Every validator reads Markdown; exactly one package knows how. A line-based,
fence-aware scanner over `textutil.NormalizeNewlines`d input: headings with level
and line number, fenced-code ranges, list items and checkboxes, links and
wikilinks, GitHub-compatible slugs, and a deliberately small frontmatter parser
(flat `key: value`, quoted strings, simple inline lists — not YAML, and it rejects
what it doesn't understand rather than guessing).

This is where false positives are prevented for all eight validators at once, so the
tests are the point: a `- [ ]` inside a fenced block is not a task, a `[[link]]` in
a code span is not a wikilink, a `#` inside a fence is not a heading, CRLF input
behaves identically to LF, and a tab-indented fence still closes.

### Phase 6 — `scc skill validate` (1 PR)

The Agent Skills contract from §5: `name` 1–64 chars, lowercase alphanumeric and
hyphens, no leading, trailing, or consecutive hyphen, **matching the parent
directory**; `description` 1–1024 chars; body within the published budget;
references one level deep. Walk `.claude/skills/*/SKILL.md`.

Read the published spec for the exact budget number rather than reproducing one from
memory — the value of this validator is that it is somebody else's standard, and a
number we invented would forfeit that.

First validator because it is the most mechanical: a `name` that doesn't match its
directory is true or false, never a judgment call. It also freezes the finding shape
under real use before seven more validators depend on it.

### Phase 7 — `scc spec validate` (2 PRs)

**7.1 `internal/ears`.** Parse a requirement line into
`{preconditions[], trigger?, system, responses[]}` and accept **all five patterns
plus combinations** — ubiquitous, state-driven (`While`), event-driven (`When`),
optional feature (`Where`), unwanted behavior (`If … then`). §10 is explicit that a
validator built on the event-driven pattern alone would reject four legitimate
patterns and push authors to invent triggers for requirements that are simply always
true. Clause order is part of the grammar and is checked.

Only lines in the numbered-requirement position are parsed. Prose elsewhere in
`requirements.md` is not a requirement and gets no findings — the alternative is a
validator that fires on the document's own introduction.

**7.2 The spec validator.** Numbering unique and contiguous, exactly one `(Unit)` or
`(TDD)` per task (§4 — the annotation is required, and this is the check that keeps
work from reaching implementation with the decision unmade), and traceability in both
directions: every requirement reaches at least one task, every task cites at least
one requirement that exists, nothing orphaned.

Traceability needs a citation syntax the design doesn't fix. See §5 below.

Delta markers (`ADDED`/`MODIFIED`/`REMOVED`) are deferred within this phase until
the design says where a delta lives — also §5.

### Phase 8 — `scc plan validate` (1 PR)

Checklist grammar; every `specs/` reference resolves to a real directory; and §1's
one-source-of-truth rule — no item both carries a checkbox and references a spec,
because then the same fact has two records and the copy is the one that goes stale.

Coverage validation ("do the specs a plan produced actually cover the plan?") is an
open question in the design and stays out until it is answered.

### Phase 9 — The knowledge base (4 PRs)

| Validator | Checks | Note |
|---|---|---|
| `wiki` | Broken wikilinks, orphan pages, index/log desync, unprocessed `docs/raw/`. | All four are graph facts over Markdown — no code read. |
| `adr` | Numbering contiguous, superseded records marked rather than edited, `adr:<slug>` citations resolve. | Smallest of the four. |
| `glossary` | One canonical term per concept; an avoided synonym used as a whole token. | Whole-token matching only — substring matching here is a false-positive generator. |
| `stack` | Every dependency in the project's manifest appears in `docs/stack.md`. | `go.mod` (hand-parsed `require` blocks) and `package.json` (`encoding/json`) first. |
| `codewiki` | `[path:start-end]()` citations resolve against the checkout, `Structure` agrees with the section delimiters, slugs unique and derived from headings, no section citing nothing. | Heaviest and last. |

On `stack`: skip any ecosystem whose manifest can't be parsed confidently without a
dependency — an unparsed manifest produces no findings, which is the correct
behavior under §5's first rule. TOML ecosystems wait for a safe parse, not for a
naive one.

### Phase 10 — `scc validate` (1 PR)

Run every applicable validator, merge findings into one sorted set, exit `2` if any,
and emit one JSON document. Skip validators whose subject is absent — a workspace
with no `docs/codewiki/` is not a workspace with findings.

This is also where the aggregate output has to stay readable: §5's third rule ("few
findings, each fixable") is easiest to violate here, so the human output groups by
file and leads with counts.

### Phase 11 — `scc update` (2 PRs)

**11.1 `internal/merge`.** Line-level diff (Myers) plus diff3, emitting standard git
conflict markers on collision. In-tree rather than shelling out to `git merge-file`:
`update` must work in a directory that isn't a git repository and on a machine
without git, and the algorithm is small, deterministic, and fully unit-testable.
This is the largest single piece of new logic in the plan and it is pure — no
filesystem, no workspace — so it gets the heaviest test suite: identical inputs,
one-sided edits both ways, adjacent non-overlapping edits, true conflicts, empty
files, no-trailing-newline, and CRLF input.

**11.2 The command.** Per managed entry: re-render the template **as the recorded
version produced it**, re-render the current version, diff those two, and apply that
diff to the working tree as a three-way merge (§12, borrowed from Copier). Pristine
files are replaced outright. Edited files are merged, and collisions surface as
conflict markers — the same idiom §9 already uses for branches, so there is one
conflict mechanism in the product instead of two.

**Where the old text comes from.** §12 says the merge needs the old rendered *text*,
not just its hash. Because Phase 1.2 made templates data-free, the pristine content
of a managed file is fully determined by its template version — so `assets` carries a
content-addressed store of historical renderings, keyed by the very hash the manifest
already records. The manifest needs no new field, and the version it records stays
useful as the diagnostic that explains *why* a file looks the way it does. A template
version that changed nothing adds nothing to the store.

Files excluded from the merge (`CLAUDE.md`, `project.md`) are reported as
"yours — check the new template if you want the change", never merged badly.

## 5 · Decisions this plan makes, and gaps it can't close

Four of these are decisions taken here because the code needs them and the design is
silent; each should be folded back into `orchestration.md` when it lands. Two are
genuine gaps that block a specific check.

**Decided: the autonomy and CI answers live in frontmatter.** §2 and §9 both say the
answer is "recorded on the artifact" without saying where. Frontmatter on the spec's
`requirements.md` and on the plan (`autonomy: auto|gated`, `ci: wait|no-wait`), set by
`spec new`/`plan new` flags. Frontmatter because it is the one part of a Markdown
artifact that is already machine-readable, and flags because the orchestrator asked
the question in conversation and needs a way to write the answer down. Validated only
if present, never required — a missing key means the run predates the convention, not
that the user did something wrong.

**Decided: templates are data-free.** Argued in Phase 1.2; it is what makes Phase 11
possible without storing rendered text in the manifest.

**Decided: user artifacts are not in the manifest.** Argued in Phase 4.

**Decided: task-to-requirement citation syntax.** Traceability (§5) is unbuildable
without one. Proposal, added to §4's grammar and to both templates:

```
- **R1.2** When the manifest is missing, the CLI shall exit 1 with a message.
- [ ] 1.1 (Unit) Parse the manifest file — R1.2, R1.4
```

Requirement IDs are `R<major>.<minor>`; a task cites them after an em dash. It greps
cleanly, it never collides with the task's own number, and it survives a reader who
has never seen this document. Requires one line in §4 and one in §10.

**Gap — where does a delta live?** §10 says a change to an existing spec is written
as a delta with each requirement marked `ADDED`/`MODIFIED`/`REMOVED`, and that it is
folded into the spec once it lands. It does not say whether the delta is a separate
file (`specs/<feature>/deltas/<name>.md`) or inline markers on requirement lines
awaiting fold-in. The validator's "delta markers well-formed" check can't be written
until this is answered; the rest of Phase 7 doesn't depend on it. **One paragraph in
§10 unblocks it.**

**Gap — what is a codewiki, on disk?** §5 specifies the codewiki validator precisely
but the design never states the artifact's path or whether it is one file or a tree.
Phase 2 has to name a segment; Phase 9 has to walk it. Currently assuming
`docs/codewiki/`. **One line in the design confirms or corrects it.**

The design's own three open questions map onto phases as follows: **plan coverage
validation** gates a check inside Phase 8 and nothing else; **what records the
routing decision** would add a frontmatter key in Phase 4 if the answer is
"something written"; **what happens to a spec after its feature merges** affects the
wording of `specs.md` in Phase 3 and adds no code either way. None of them block the
start of any phase.

## 6 · Milestones

| Release | Phases | What the user has |
|---|---|---|
| **v0.1.0** | 1–4 | A template system: `init` scaffolds the rules and templates, `spec new`/`plan new` create work. No validation yet. Useful on its own — this is csdd's value without its ceremony. |
| **v0.2.0** | 5–8 | The enforcement half: skill, spec, and plan validation, exiting `2` on findings. This is the version a CI job can gate on. |
| **v0.3.0** | 9–10 | The knowledge base validated, and one `scc validate` over everything. |
| **v0.4.0** | 11 | `scc update` — three-way merge onto a new template version. |

Between v0.1.0 and v0.4.0 the upgrade story is "re-run `init`; it never overwrites
what you edited". That is honest and it works, because `init` is idempotent and
additive by construction — it just doesn't deliver improved *templates* to files that
already exist. Say so in the README rather than letting a user discover it.

## 7 · Risks, largest first

**Authoring the rules is the long pole, not the code.** Phase 3 is prose that has to
be right — it is the product's actual behavior, and every word of it is read by an
agent that will follow it literally. Budget review time accordingly, and resist
padding: §6's context-rot evidence means a longer rule set is measurably worse, not
just more expensive.

**diff3 is the one genuinely hard algorithm.** Mitigated by keeping `internal/merge`
pure and testing it exhaustively before `update` ever touches a workspace, and by
scheduling it after there is real version history to test against.

**Eight validators can destroy their own value.** §5 quantifies it: 35–91% of
warnings non-actionable in studied deployments, false positives the most common
reason warnings get suppressed. The per-check question in §0 is the mitigation, and
the ordering in §3 (most mechanical first) means the trust-building checks ship
before the risky ones.

**EARS parsing is the most likely place to be over-eager.** Restricting the parser to
lines in the numbered-requirement position is the main defense; the second is that an
unparseable line reports *that it could not be parsed as EARS*, which is actionable,
rather than guessing which clause is missing.

**Windows.** The race detector doesn't run on the Windows job, so anything
concurrent gets its coverage on Linux. Slash-separated manifest paths (Phase 1.1),
delete-pending behavior in `spec delete` tests, and `.gitattributes` keeping the tree
LF are the three recurring traps.

## 8 · Checklist

Grammar per §4, since this file is itself a plan.

```
Phase 1 — foundations
- [x] 1.1 (Unit) internal/manifest: entries, deterministic (de)serialization, slash paths
- [x] 1.2 (TDD) internal/manifest: Hash over normalized content, Status pristine/edited/missing
- [x] 1.3 (Unit) internal/assets: embed the template tree, Render, LF-clean golden test
- [x] 1.4 (Unit) internal/finding: finding type, stable sort, frozen JSON shape, ExitCode

Phase 2 — init
- [x] 2.1 (Unit) paths: rules segment and every docs/ segment, in one commit
- [x] 2.2 (Unit) internal/scaffold: write missing files, record the manifest, report actions
- [x] 2.3 (TDD) scaffold: idempotence and never-overwrite, including --force reporting
- [x] 2.4 (Unit) cli: scc init with --root/--json/--force

Phase 3 — the template set
- [x] 3.1 (Unit) .claude/rules/: the nine rule files
- [x] 3.2 (Unit) CLAUDE.md and the two review agents
- [ ] 3.3 (Unit) shipped skills — none ship; the design never says which would (§9)
- [x] 3.4 (Unit) spec templates (optional sections marked optional) and the plan template

Phase 4 — creating work
- [x] 4.1 (TDD) cli: spec new/list/show/delete, SafeName + KebabCheck on every positional
- [x] 4.2 (Unit) cli: plan new/list/delete
- [x] 4.3 (Unit) frontmatter: --autonomy and --ci recorded on the artifact

Phase 5 — markdown
- [x] 5.1 (TDD) internal/mdscan: fence-aware headings, checkboxes, links, wikilinks, slugs
- [x] 5.2 (Unit) internal/mdscan: the small frontmatter parser, rejecting what it can't read
- [x] 5.3 (Unit) internal/mdscan: Body — comment- and fence-stripped lines for every validator

Phase 6 — skill
- [x] 6.1 (TDD) validate: Agent Skills conformance against the published spec
- [x] 6.2 (Unit) cli: scc skill validate, exit 2 on findings
- [x] 6.3 (Unit) the shipped skills pass their own validator (vacuously true; see 3.3)

Phase 7 — spec
- [x] 7.1 (TDD) internal/ears: all five patterns plus complex, clause order enforced
- [x] 7.2 (TDD) validate: numbering, one methodology per task, traceability both ways
- [x] 7.3 (Unit) cli: scc spec validate [<feature>]
- [x] 7.4 (Unit) validate: delta markers — unblocked by §9's decision on where a delta lives

Phase 8 — plan
- [x] 8.1 (TDD) validate: checklist grammar, references resolve, one source of truth
- [x] 8.2 (Unit) cli: scc plan validate

Phase 9 — knowledge base
- [x] 9.1 (TDD) validate: wiki — broken links, orphans, changelog desync, unprocessed raw
- [x] 9.2 (Unit) validate: adr — contiguous numbering, superseded marked, citations resolve
- [x] 9.3 (TDD) validate: glossary — canonical terms, whole-token synonym detection
- [x] 9.4 (TDD) validate: stack — go.mod and package.json against docs/stack.md
- [x] 9.5 (TDD) validate: codewiki — citations resolve, slugs unique, no section citing nothing

Phase 10 — aggregate
- [x] 10.1 (Unit) cli: scc validate over every applicable validator, one JSON document

Phase 11 — update
- [ ] 11.1 (TDD) internal/merge: line diff
- [ ] 11.2 (TDD) internal/merge: diff3 with git conflict markers
- [ ] 11.3 (Unit) assets: content-addressed store of historical renderings
- [ ] 11.4 (TDD) cli: scc update — re-render old, re-render new, three-way merge
- [ ] 11.5 (Unit) update: excluded files reported rather than merged
```

## 9 · What the build settled, and where it deviated

Everything §5 listed as decided is now in `orchestration.md`. Both gaps are closed, and
four things came out differently from the plan. Each deviation is here rather than
silently in the code, because each is a decision somebody could reasonably reverse.

**Closed: where a delta lives.** Inline markers on the requirement line —
`- **R2.3** (MODIFIED) When …` — not a `specs/<feature>/deltas/` tree. The reason is
§10's own second argument: deltas exist partly so two sessions can change one spec as
long as they touch *different requirements*, and a marker per requirement gives that
directly while a file per change does not. `(REMOVED)` requirements are exempt from EARS
parsing and from traceability, because there is nothing left to hold to a grammar and no
task has to reach one. Phase 7.4 is unblocked and done.

**Closed: what a codewiki is on disk.** `docs/codewiki/`, one Markdown page per area,
every `##` section carrying at least one `[path:start-end]()` citation. The `Structure`
tree check §5 listed is **dropped**, not deferred: the design never said what that tree
looks like, and a check against a guess is precisely the false positive that costs trust
in the three checks that are unambiguous.

**Deviation: requirement numbering is not checked for contiguity.** The plan's 7.2 asked
for "unique and contiguous". Contiguity is wrong here — a requirement removed through a
delta legitimately leaves a hole, so the check would fire on the mechanism the design
asks people to use. Uniqueness is checked; gaps are not. ADR numbering *is* contiguous,
because an ADR is never removed, so there a gap means a record went missing.

**Deviation: `internal/mdscan` was split across phases 4 and 5.** The frontmatter parser
(5.2) landed early, because `spec show` reads the kickoff answers back and the
alternative was a second frontmatter parser in the CLI. It also grew one level of nested
scalars, which the plan did not anticipate: the Agent Skills spec's optional `metadata`
field is a mapping, and refusing it would mean reporting a finding on a skill that is
valid by the standard the validator exists to enforce. Nothing deeper is accepted.

**Deviation: `assets` has `Content` plus `Artifact`, not one `Render(name, data)`.** The
plan's own next paragraph explains why: workspace templates are data-free, so passing
data to them is meaningless, while artifact templates *do* take data because they are
authored from birth and never tracked in the manifest. Two functions make the difference
impossible to get wrong at a call site.

**Deviation: no skills ship, so 3.3 and 6.3 are hollow.** The design lists skills among
what the binary scaffolds but never says which. Nothing was invented to fill the gap.
`skill validate` is complete and tested against fixtures, and the test asserting that
shipped skills pass it will start meaning something the moment one ships. **One line in
the design saying which skills ship, if any, closes this.**

**Also settled along the way**, all of it now in the rules the binary scaffolds, because
a validator needs a written convention to check against: ADR frontmatter (`status`,
`superseded-by`), the ADR filename shape (`NNNN-kebab-slug.md`), the glossary entry
format (`- **term** — definition. Avoid: synonym`), the wiki's `index.md`/`changelog.md`
pair, and the codewiki citation form.

**Phase 11 is deliberately not started.** §3's reason now literally applies: only one
template version exists, so `scc update` could only be tested against synthetic history.
It becomes real work the moment a second template version ships. Until then the honest
upgrade story is the one §6 already states — re-run `init`, which never overwrites what
you edited.

One thing the build found that no phase predicted, worth recording because it is the
class of bug this whole design is about: the first end-to-end run of
`scc spec new` + `scc validate` **reported a finding on the file scc had just
generated** — the shipped `design.md` template cited its requirements only inside an
HTML comment, so the traceability check saw a design tracing to nothing. A tool whose
first act is to flag its own output teaches the user to ignore all eight validators.
There is now a test asserting that fresh artifacts pass every validator, and it should
be treated as the gate on any future template change.
