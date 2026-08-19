# Orchestration model

How work enters an scc workspace and how it gets built. This is the design that
replaces `csdd`'s fixed, human-gated pipeline; it is not implemented yet.

`research.md` beside this file records where these decisions sit relative to the field and
the literature — including the measured results behind §3's impact-analysis step, §5's
skill conformance, §10's delta format, and §11. Read it before revisiting a decision here;
several of them are evidence-backed rather than preferences.

## 0 · What scc ships

Same philosophy as csdd: **scc is a template plus a CLI.** The binary scaffolds a
workspace — rules, skills, agents, spec templates — into the harness's own
configuration directory plus `specs/`, `plans/`, and `docs/`, all of it plain Markdown compiled
into the binary via `//go:embed`, and then validates what lives there. There is no
conversion layer and no runtime: the files the harness already reads *are* the API,
and the CLI is how they are created, checked, and upgraded.

scc adds **no directory of its own** and **no config file**. It owns exactly one file,
`<harness>/scc-manifest.json`: content hashes and the template version per managed
file, so an upgrade can tell a pristine file from an edited one — and, by existing at
all, the workspace marker. It has nothing to configure because it runs nothing; the
project's test and lint commands (§7) are a rule under the harness's rules directory,
Markdown the orchestrator already reads.

The marker is that file rather than the directory itself because every harness has a
global twin in the user's home — `~/.claude`, `~/.codex`, `~/.config/opencode` — so a
walk accepting the directory would resolve any out-of-workspace command's root to
`$HOME`, and because those directories also exist in repos that merely use the tool,
where scc was never initialized.

That is why everything below describes both a rule and its artifact. A rule the
binary cannot scaffold is documentation; a rule it cannot check is a suggestion.

## 1 · Routing — which vehicle

The premise: **the orchestrator routes.** csdd made every change walk the same
path — requirements → design → tasks, each phase blocked on human approval. That
is the right ceremony for a large feature and pure overhead for a small one. Here
the orchestrator picks the vehicle to match the work, and the tool's job is to
make that choice explicit, recorded, and mechanically checkable.

```
                     does this need requirements
                       and a design settled
                          before any code?
                               │
              ┌──────── yes ───┴─── no ────────┐
              │                                │
            Spec                             Plan
      specs/<feature>/                  plans/<name>.md
   requirements → design → tasks      structure + checklist
              │                                │
              │                     ┌───────────┴───────────┐
              │                     │                       │
              │              a direct checklist    a decomposition that
              │              of work to do         references Specs
              │                     │                       │
              └──────────────► every task carries a methodology (§3) ◄┘
```

**Two vehicles, not three.** Everything that isn't worth a spec is a **plan** — a
document with a structure and a checklist. That covers both ends of what used to be
three: an initiative too large for one spec, decomposed into specs; and a change small
enough that the *what* was never in doubt, recorded as a checklist and nothing more. A
plan can be either, or both at once — an initiative whose decomposition includes a
handful of items it just does itself.

| Vehicle | When | Path | Contents |
|---|---|---|---|
| **Spec** | The *what* and the *how* need settling before code. | `specs/<feature>/` | `requirements.md` · `design.md` · `tasks.md` |
| **Plan** | Everything else. | `plans/<name>.md` | structure, plus a checklist of tasks and/or references to specs |

The collapse is what makes the small case affordable. The earlier design had small
changes write nothing at all, because a `specs/` tree cluttered with three-line features
makes the specs that matter harder to find. A plan is **one file in a different
directory**, so that cost never appears — and the record survives the session, which is
what a small change was previously losing.

**A plan is not a feature, so it does not live in `specs/`.** Each directory name means
exactly one thing: `specs/` holds features, `plans/` holds plans, `docs/` is the
knowledge base. csdd put plans in `docs/plans/`; a plan is work, not knowledge, so it
sits beside `specs/` rather than inside `docs/`.

**A plan is one file, not a directory,** because a plan holds no state beyond its own
checklist. Where an item *is* a task, its checkbox is the state. Where an item
references a spec, the state is **derived** from that spec, never copied — two records
of the same fact can disagree, and the copy is the one that goes stale. Every item has
exactly one source of truth.

**A plan's leaves are ordinary specs.** `plans/checkout-revamp.md` references
`specs/cart-totals/`, `specs/payment-intents/`. They are not nested under the plan: a
leaf is a normal spec that happens to have been created by one, built by exactly the
same rules whether a plan produced it or a human asked directly.

> **Vocabulary warning.** In GitHub Spec Kit — the most widely integrated toolkit — "plan"
> means the *opposite*: `/speckit.plan` produces the architecture, data model, and API
> contracts *inside* one feature, i.e. what we call design. Ours is the decomposition
> *above* specs. The collision is a known, accepted cost: our plan covers both a
> decomposition and a bare checklist, and no better single word covers both. Someone
> arriving from spec-kit will need this paragraph.

### Why both vehicles write a file

Neither vehicle is session-only, and **autonomy is the reason.** csdd's human approved
each phase, so they *saw* the requirements go by. Here the orchestrator writes and
proceeds. The artifact is the only channel through which anyone — the reviewer, the next
session, the orchestrator itself after a compaction — ever learns what was decided. A
session's context dies; the file is what makes the work resumable and the reasoning
reviewable.

It is also the only thing `scc` can act on: §5 validates artifacts, §2 records the gate
answer on one, §4's methodology annotations are checked in a file. No artifact, no tool
— just rules and two reviewers.

What the two-vehicle split buys is that the *weight* of the record matches the weight of
the work. A one-line change gets one checklist item in a plan, not three ceremonial
files in `specs/`.

### Before either vehicle — read the record

Routing is not the first act. **Before the first artifact of a piece of work exists —
before `scc spec new`, before `scc plan new`, before a line of code — the orchestrator
reads what this workspace already settled.** `docs/` is not reference material for when
someone is stuck: it is the constraint set. An ADR binds the design about to be written,
`stack.md` says what may be built on, `glossary.md` says what to call it, and a spec
already covering the area turns new work into a delta (§10) rather than a second
statement of the same feature.

The failure it prevents is silent, which is why it is a rule rather than a habit. A spec
written without the pass re-decides a decision somebody already made, names a concept a
second way, or adds a dependency for something the stack already carries — and none of
it reads as wrong on review, because it reads as *new work* rather than as a
contradiction. Under autonomy nobody sees the phase where the contradiction was
available to notice.

**It is its own rule (`prior-art.md`), not a paragraph in `knowledge-base.md`.** The two
halves fire at opposite moments: that one is triggered by having learned something, this
one by being about to write. A read-side instruction filed under the write-side rule is
read after the spec exists, which is after it was any use.

**The pass has to say what no index says.** `scc map` covers `plans/` and `specs/`, and
the symbol graph covers code — so `docs/` is the one corpus reached by opening a file.
That is affordable only because the seeded anchors (§6) are built for it: `glossary.md`
and `stack.md` are lists, `wiki/index.md` and the ADR filenames are tables of contents.
The rule says to open a page when its title bears on the work, never to survey the base.

What the pass finds is then **stated and cited**: one or two lines up front naming the
ADRs that bind and the spec that already covers the area — the material of the first
checkpoint under `gated`, and the only trace the pass happened at all under `auto` —
then carried into the artifact as an `adr:` citation, a delta, or a `[[wikilink]]`,
where it outlives the session. "Nothing here governs this" is a result and is said out
loud; an empty `docs/` is a young workspace, not a finding, so nothing is invented to
fill it.

## 2 · Autonomy — the gate is a question, not a flag

The spec phases are **autonomous by default**: the orchestrator writes
requirements, design, and tasks, then starts implementing — it does not stop to
be approved at each phase the way csdd did.

But autonomy is the user's call, so **the orchestrator asks once, at kickoff**:
run automatically, or gate the phases for review? The answer is recorded on the
artifact, which matters for two reasons — the run is reproducible from the file,
and the orchestrator never asks twice for the same piece of work.

Asking beats a flag here: the person who needs to make the call is in the
conversation, not on a command line.

**Where it is recorded:** frontmatter on the spec's `requirements.md`, or on the plan.
Frontmatter because it is the one part of a Markdown artifact that is already
machine-readable, so `scc` can check the value without reading prose.

```yaml
---
autonomy: auto      # or: gated
ci: wait            # or: no-wait  (§9)
---
```

`scc spec new <feature> --autonomy=gated --ci=no-wait` writes both. They are validated
only when present: a missing key means the run predates the convention, not that the
user did something wrong.

### Automatic still escalates

"Automatic" is not "never stop." The practice literature describes three oversight shapes —
approval before each action, review after, and **escalation triggers**: run autonomously,
but halt on specific risk signals. Asking once and then never stopping is only the middle
option, and it treats a task that moves money like a task that renames a variable.

So autonomous mode keeps one exception: **the risk that mandates TDD also warrants a
checkpoint.** §3 already requires the orchestrator to classify every task against that list
— money, complex algorithms, hypothesis validation, high-risk complexity. A task annotated
`(TDD)` for one of those reasons is precisely the task worth surfacing before it lands, even
in a run the user asked to be automatic.

One classifier, two consumers: it picks the methodology, and it picks what deserves a human
glance. Adding a second, separate risk taxonomy for gating would guarantee the two drift
apart.

The related principle from the same literature — **autonomy is earned through demonstrated
performance, not assumed** — is why the question exists at all rather than defaulting to
automatic silently.

## 3 · Methodology — per task, Unit by default

Every task is built one of two ways. The orchestrator decides per task, not per
project — a feature routinely has both.

### Before either cycle — find the tests that already cover this

**Identify the existing tests that exercise the paths this task will change, and run
them, before writing anything.** This is not part of Unit or TDD; it precedes both,
because it is about not breaking what works rather than about proving what's new.

This step exists because of a measured result, not a preference. TDAD (arXiv 2603.17973)
found that giving an agent generic test-first *procedure* while leaving it ignorant of
which tests actually cover the code being modified **increased regressions above the
no-intervention baseline** — roughly 9.9% against ~6.1% for doing nothing. What worked was
impact analysis: identifying the tests that genuinely exercise the modified paths, which
brought regressions to ~1.8%. Procedure without context was worse than no procedure.

The orchestrator does this analysis, and it can: it reads the code. `scc` does not (§5),
so this lands as a rule rather than a command — which is exactly the split the rest of the
design already draws.

The failure it prevents is specific: a task that adds a case to a function whose existing
tests nobody looked at, whose new test passes, and whose old behavior silently changed.

### Unit (default)

Write the code, then write a unit test for **each function** in it. That is the whole
cycle — **no RED/GREEN.** There is no failing-test step to observe, because the code
already exists; the deliverable is unit tests covering every function.

Two conditions make this the default rather than a shortcut, and both are load-bearing.

**Immediately, per function — not at the end.** The empirical record supports test-last
only in its *iterative* form: controlled experiments with professionals put TDD's advantage
over iterative test-last at "small and in practice relatively unimportant", with one study
finding the test-last group more productive and writing *more* tests. Nothing supports
writing the tests at the end of the feature. Finish a function, test that function, move on.

**The test comes from the requirement, not from the code.** This is where an agent fails
differently from a human. A human writing tests late tends to write too few; an agent tends
to write tests that assert **what the code does** rather than what it should do — green
tests that faithfully encode the bug. So when writing a Unit test, the reference is the
requirement the task cites (§10), and reading the implementation to decide what to assert is
the failure mode, not the method.

The default because most code is plumbing: the shape of the thing is not in doubt, and tests
written straight after it are cheaper and just as binding — as long as none are missing and
none merely mirror the code. Those two clauses are the whole risk of code-first, and the
rules the orchestrator is accountable for (§5).

### TDD (RED/GREEN required)

Write the failing test first, **watch it fail (RED)**, then make it pass (GREEN),
then refactor. Skipping RED is not TDD — a test that has never failed has not been
shown to test anything.

RED/GREEN belongs to TDD and only to TDD. It is not a stricter Unit, and Unit is
not a lazier TDD: they are two different cycles, and the annotation on the task
says which one is in effect.

Mandatory when the cost of being wrong is high:

- **money** — any calculation, rounding, split, or conversion involving currency
- **complex algorithms** — anything whose correctness is not obvious by reading it
- **hypothesis / thesis validation** — code written to prove something holds
- **anything else** where the complexity is real and the chance of getting it
  wrong is high

The trigger is risk, not size. A three-line rounding helper that touches money is
TDD; a two-hundred-line CRUD handler is Unit.

## 4 · Task grammar

The same grammar governs every task line, whether it sits in a spec's `tasks.md` or in a
plan's checklist — the methodology is a property of the task, not of the vehicle that
carried it.

The annotation is **required**: a task with no methodology is a task where nobody
decided, which is exactly the failure this design exists to prevent.

```
- [ ] 1.1 (Unit) Parse the manifest file — R1.2, R1.4
- [ ] 1.2 (TDD) Calculate the pro-rata split across accounts — R2.1
- [ ] 1.3 (Unit) Render the summary table — R3.1
```

`(Unit)` / `(TDD)` — required, exactly one per task. That is the whole grammar:
there is no parallel-dispatch marker, because implementation is sequential (§8).

**Requirements are cited after an em dash**, by the `R<group>.<item>` IDs
`requirements.md` numbers them with (§10). Required in a spec's `tasks.md`, since that
citation is the only thing that makes the traceability check in §5 possible; omitted in
a plan, which has no requirements to cite. The syntax greps cleanly, never collides with
the task's own number, and survives a reader who has never seen this document.

`scc` validates the grammar and exits `2` when a task is missing its methodology, so no
work — spec or plan — reaches implementation with the decision unmade.

### How big is a task — independently verifiable

**A task is the right size when it can be verified on its own.** Not "one file", not "an
hour": verifiable alone, which is what makes §7's per-task loop (scoped tests + lint)
possible at all.

The measured reason to care: subtask success rates run substantially higher than whole-task
success rates — agents execute individual steps correctly and still fail end-to-end
workflows — and structuring work so a failure can be retried at the subtask level cut retry
cost by ~73% against retrying a whole plan. Granularity is not tidiness; it decides what a
failure costs.

Too coarse and a red result tells you a feature is broken. Too fine and the checklist becomes
bookkeeping about work smaller than the act of recording it.

## 5 · What scc checks — and what it deliberately doesn't

**scc checks structure.** That is the product: every artifact it governs has a shape, and
a shape is checkable. Findings exit `2`.

| Validator | What it holds to a shape |
|---|---|
| **spec** | EARS grammar across all five patterns (§10) · requirements numbered and unique · one methodology per task (§4) · traceability: every requirement reaches a task, every task cites a requirement, nothing orphaned · delta markers well-formed |
| — | *Numbering is unique but deliberately **not** contiguous. A requirement removed through a delta (§10) leaves a hole, so a gap is the mechanism working rather than a defect, and reporting it would be a false positive on the practice this design asks people to use. An ADR gap is a finding; a requirement gap is not.* |
| **plan** | checklist grammar · every reference resolves to a real `specs/` entry · no item both carries a checkbox and references a spec (§1's one-source-of-truth rule) |
| **skill** | the published Agent Skills spec: `name` 1–64 chars, lowercase alphanumeric and hyphens, no leading/trailing or consecutive hyphen, **matching the parent directory** · `description` 1–1024 chars · body within the recommended budget · references one level deep |
| **wiki** | broken wikilinks · orphan pages · index/log desync · sources dropped in `docs/raw/` and never processed |
| **codewiki** | at `docs/codewiki/`, one page per area: every `[path:start-end]()` citation resolves against the checkout · slugs unique and derived from their headings · no section citing nothing |
| **adr** | numbering contiguous · superseded records marked rather than edited · `adr:<slug>` citations resolve |
| **glossary** | each concept has one canonical term · an avoided synonym used as a whole token is a finding |
| **stack** | every direct dependency the project declares appears in `docs/stack.md` — a dependency file is structured data, not source, so this is checkable without reading code. Seven readers today: `go.mod`, `package.json`, `requirements.txt`, `pyproject.toml`, `Cargo.toml`, `composer.json`, `pom.xml` |

Two of these are worth calling out because they are stronger than they look.

**`skill` has an external contract.** The Agent Skills spec is published and maintained
outside Anthropic (`agentskills.io`), and 32 tools across competing vendors read the same
format. So this validator is not scc's opinion about skills — it is conformance to a
standard, which is the most defensible kind of check we can ship. Implemented in Go rather
than delegating to the reference `skills-ref` validator, because scc is a single binary and
shelling out to Node would break that for a handful of regexes.

The `Structure`-tree check an earlier draft listed for `codewiki` is **dropped**: the
design never settled what that tree looks like on disk, and a check written against a
guess is exactly the false positive that would cost the user's trust in the other three.
The three above are unambiguous.

**`stack` turns a rule into a gate.** "Technology not listed in `docs/stack.md` is an open
decision, never adopted silently" reads like an unenforceable principle, but a project's
dependency file is a structured file. Diffing it against `stack.md` catches the silent
dependency, and does it in any ecosystem that declares its dependencies as data.

**An ecosystem with no reader is not checked, never failed.** No declared dependencies
means Stack returns before it asks whether `stack.md` exists, so a Ruby, Elixir or
Gradle project passes `scc validate` untouched rather than failing on a file scc never
understood. That is the same rule as "a manifest scc cannot parse produces no findings",
and it is why those three are absent: their manifests are executable code, and reading
one honestly would mean evaluating it.

### The discipline eight validators require

A catalogue this size can destroy its own value. In studied static-analysis deployments
**35–91% of warnings are non-actionable**, false positives are the single most common reason
developers suppress warnings (34.4% of suppressions), and the two documented barriers to
adopting such tools are false positives and cumbersome configuration. Alert fatigue does not
discriminate: valid findings get ignored alongside the noise.

Three rules, non-negotiable:

1. **A false positive costs more than a miss.** One wrong finding teaches the user to
   disbelieve all eight. When a check cannot be certain, it stays silent — the same trade §5
   already made about not parsing code, now with numbers behind it.
2. **Zero configuration to be useful.** Configuration is the other documented adoption
   barrier. A validator that must be tuned first won't be.
3. **Few findings, each fixable.** A finding the user cannot act on is noise wearing a useful
   shape.

There is a reason to expect this to work: **structural checks have the lowest false-positive
rate of any class.** A `SKILL.md` whose `name` doesn't match its directory is not a judgment
call — it is true or false. The boundary in §5 protects trust in the tool, not only its scope.

### What it deliberately doesn't check

It does **not** parse your source to verify that each function has a test, and it does not
observe RED before GREEN. Those are **rules the orchestrator applies**, not gates the
binary enforces — the orchestrator decides which tests to run.

This is the deliberate break from csdd, whose instinct was to make everything
deterministic. Two reasons it doesn't extend to code:

- **A per-language parser makes scc own every ecosystem it touches.** Go is free
  (`go/ast` is stdlib), and every other language is a parser to write, a dependency
  to carry, and a grammar to chase. A tool that only works on Go workspaces is not
  the tool being built here.
- **A wrong answer is worse than no answer.** A checker that misses a missing test
  reports "clean" and the rule quietly stops existing. Judgment that knows it is
  judgment beats a validator that is confidently incomplete.

So the enforcement boundary is: **scc guarantees the decision was made and
recorded; the orchestrator is accountable for honoring it.** The annotation is what
makes that accountability legible — a reviewer reading the task list can see which tasks
claimed TDD and check them.

## 6 · Where the rules live

`<harness>/rules/` is the home of the rules, as in csdd. The scaffolded entry file
stays small and points at them.

This is a token-budget decision, not organization: every session pays for the entry
file in context, so the routing table and the two cycles do not belong inline there.
The rules are read when the rule is relevant.

The evidence for keeping `CLAUDE.md` short is **context rot**, not the frequently-cited
`AGENTS.md` efficiency study. Chroma's measurement across 18 frontier models found accuracy
degrading non-uniformly as input grows — 30–50% before the documented context limit, and
sooner on more complex tasks. Length has a cost in correctness, not just in tokens. (The
`AGENTS.md` study measured only wall-clock and output tokens and explicitly put quality out
of scope; the "three sections, under 150 lines" prescription attributed to it is not in it.
Don't cite it for a length rule.)

### Three harnesses, one template set

`scc init` scaffolds for Claude Code (the default), Codex, or opencode — one per run,
`--claude` / `--codex` / `--opencode`, and it asks when a person is at the terminal
and passed no flag. What differs between them is entirely *where files go and what
dialect the loader parses*, never what the methodology says:

| | Claude Code | Codex | opencode |
|---|---|---|---|
| entry file | `CLAUDE.md` | `AGENTS.md` | `AGENTS.md` |
| rules | `.claude/rules/` | `.codex/rules/` | `.opencode/rules/` |
| review agents | `.claude/agents/*.md` | `.codex/agents/*.toml` | `.opencode/agent/*.md` |
| skills | `.claude/skills/<n>/SKILL.md` | `.codex/skills/<n>/SKILL.md` | `.opencode/skills/<n>/SKILL.md` |
| slash commands | `.claude/commands/scc-*.md` | — | `.opencode/command/scc-*.md` |
| manifest | `.claude/scc-manifest.json` | `.codex/scc-manifest.json` | `.opencode/scc-manifest.json` |

**One prose source, addressed three ways.** The rules, the skills, and the reviewers
are written once; the paths in them come from a harness profile and the header each
loader parses is synthesized at render time. A copy of the template set per harness
would drift, and the drift would be silent — the same argument that keeps the
methodology out of skills two paragraphs up.

This is the one thing templates may interpolate, and it does not cost the upgrade
path: a `(template version, harness)` pair still renders byte-identically everywhere,
and the manifest records both, so re-rendering the recorded version to reconstruct a
merge base stays exact.

**Codex ships no slash commands, deliberately.** Its custom prompts are user-global
and now deprecated in favor of skills, so there is nothing project-scoped to write
and scc does not write into anyone's home directory. The skills are the whole
surface there, which is what Codex itself recommends.

**Codex and opencode share `AGENTS.md`, and the first one initialized wins it.** A
repo scaffolded for both has one entry file, written by whichever ran first and never
overwritten afterwards, pointing at that harness's rules directory. The second
harness's session follows those links and gets the right methodology: eight of the
nine rules are byte-identical between the two, and the ninth differs only in a
glossary *example* naming a manifest path. So the second tree is a duplicate nothing
links to — a wart, not a broken workspace, and the design is choosing it over the
alternatives: inventing a neutral rules directory neither tool knows about, or
rewriting a file the user owns.

`scc update` keeps both trees current regardless, because it works from each
harness's own manifest rather than from the entry file.

### Which skills ship — the authors of `docs/`, plus the runners

Eight, in two lists, and each list has its own rule. The first is mechanical: **a
skill ships for each `docs/` artifact a validator checks, and for nothing else.**

| Skill | Authors | Checked by |
|---|---|---|
| `wiki` | `docs/wiki/` — ingest from `docs/raw/`, query, repair | `wiki` |
| `codewiki` | `docs/codewiki/` — narrated code, cited by line range | `codewiki` |
| `glossary` | `docs/glossary.md` — canonical terms and banned synonyms | `glossary` |
| `stack` | `docs/stack.md` — adopted technology, one line of why | `stack` |
| `adr` | `docs/adr/` — hard-to-reverse decisions, superseded not edited | `adr` |
| `prd` | `plans/<name>.md` — an initiative decomposed into specs | `plan` |

Five of the six pair with a validator of the same name. `prd` is the exception and
earns its place for the opposite reason: it is the **entry point** for work too large
for one spec, the one place §1's routing question gets asked out loud, and without it
the only on-ramp scc offers is `scc plan new` handing the user an empty template.

**The methodology is deliberately not a skill.** The cycles, verification, and
delivery are rules under `.claude/rules/` (§3, §7, §9) — read when the concern is
live. csdd shipped `tdd-cycle`, `unit-cycle`, and `verify-change` as skills; here
that would be a second copy of a rule, and the copy is the one that goes stale.

**The exceptions, and the line they draw — `plan-run` and `init`.** The second list
holds skills that *run* the methodology rather than describe it, and it holds two.

| Skill | Runs | Owned by no rule |
|---|---|---|
| `plan-run` | a whole `plans/<name>.md`, group by group | the loop **across** units of work |
| `init` | a repository's whole knowledge base, from the code already there | the order **across** the six authors, and the bar on reconstructed knowledge |

The rule above is what makes it admissible rather than a leak. §9's delivery sequence
ends at one merged pull request, because that is where one unit of work ends — a spec,
or a plan's leaf. A plan is many of those in sequence, and running one to the end means
picking the next group, branching it from the merge the previous group produced,
holding CI green because every later group inherits that base, and recovering the
position from `main` when a session dies mid-plan. None of that is in any rule, and
none of it belongs in one: a rule is read when a concern is live, and this is a
procedure a person starts on purpose and lets run.

So the line is not "methodology may now be a skill." It is: **a skill may hold a
procedure no rule holds; it may never restate one that a rule does.** `plan-run`
therefore cites `delivery.md` for the single-group mechanics instead of copying
them — if it ever grows its own account of how to open a PR, it has crossed back over.

**It has its own kickoff, and that is not a violation of §4 — it is §4 applied.** The
rule there is *ask once, before doing anything, and record the answer*; the mistake it
forbids is asking repeatedly, not asking at all. A plan's recorded `autonomy` and `ci`
were given when the plan was **written**. Starting a loop that will open and merge pull
requests for hours is a different and larger thing to agree to, and it raises a
question authoring never had — one pull request per group, or one at the end.

So `plan-run` asks three questions, once, **after reporting the groups**, because the
answers are only meaningful to someone who can see what they are agreeing to. Anything
already in the frontmatter is shown as a proposed answer to confirm, never as a
decision already taken. Four keys go back into the plan, and a resumed session reads
them instead of asking again:

| Key | Values | Decides |
|---|---|---|
| `autonomy` | `auto` · `gated` | straight through, or stop at each group boundary |
| `pr` | `per-group` · `per-plan` | one pull request per group, or one at the end |
| `ci` | `wait` · `no-wait` | whether the checks have to settle before the merge |
| `merge` | `auto` · `manual` | whether the loop merges, or stops at the PR for the developer |

`pr` and `merge` are new, plan-only, and validated by `plan` on exactly the terms
`autonomy` and `ci` are — **only when present**, since a plan no loop has run over is
not a plan with a defect. They are validated at all because the skill writes them and
every later session reads them, so `merge: whenever` would silently decide how the next
ten groups get built.

There was a fifth key, `worktree: per-group | in-place`, and it is gone. The worktree
existed for one case — several sessions at once on one repo — which is the user's own
setup, and charging every single-session run a directory to create, switch to and
remember to remove (and a stale checkout whenever the run died before the last step)
bought nothing back. §9 is a branch in the checkout you are already in.

The tempting alternative was for the loop to **override** `ci: no-wait`, on the
argument that the next group branches from the merge and an unverified base poisons
everything after it. Rejected: the argument is sound and the developer is still the one
who gets to weigh it. Stating the cost at the moment of the question is how a tool
respects a decision it disagrees with; overriding is how it stops being trusted.
`no-wait` therefore stays available inside a loop, and the skill says out loud what it
buys and what it costs before accepting it.

**`init` clears the same bar, one layer up.** Every knowledge skill is triggered by a
concern going live — something was learned, a decision was made, a dependency was
added. None of them fires when the base is *empty*, and no rule can order them, because
a rule is read at the moment its own concern arrives and this is the moment before any
of them has one. Bootstrapping an existing repository needs three things no rule holds:
a survey that precedes every artifact, an order across the six (checkable first,
interpretive last, `scc validate` between stages), and a bar that only applies here.

That bar is the reason it is a skill rather than a `--bootstrap` flag on the CLI.
**Everything written on this run is reconstructed rather than remembered**, so nothing
goes in that cannot be pointed at — a manifest, a CI file, a commit, a migration — and
what nobody can justify is *reported by name* instead of filled in with something
plausible. A gap is visible and an invention is believed, and the knowledge base's whole
value is that it can be trusted without checking. The ADRs are last and strictest for
exactly that reason, and each says in its own `## Context` that it was written after the
fact and from what.

It shares its name with `scc init` deliberately: the CLI command lays the four anchors
down empty, which is all a binary can do, and the skill is what fills them. So a person
who has just scaffolded a repository and asks "now what" has one word to type, and the
skill's first act is to check that the workspace exists at all — the harness, when it
does not, is the user's call and not the agent's.

**What it deliberately does not write is `specs/`.** Restating a working system as
requirements is the failure mode of every documentation pass; §11 already says a spec
meets existing code as a **delta**, so the first spec in a bootstrapped repository is
written by the next change rather than by this run.

**One slash command per skill, namespaced `scc-`.** A skill is model-invoked through
its description; a command is the human saying *now*. Both are cheap, they are derived
from one list so they cannot drift, and the prefix is not decoration — slash commands
share a flat namespace across every source Claude Code loads them from, and a bare
`/adr` collides on contact.

The skills carry the *procedure and the judgment*; the format they produce stays in
`.claude/rules/knowledge-base.md`, stated once. A skill that restated the format would
be the same duplication the methodology rules are being kept out of skills to avoid.

**How a skill names a rule — the path from the root, and never a link.** Every
template writes a rule's path the same way, `<harness>/rules/<rule>.md`, expanded at
render time so it is correct in each of the three trees. A `SKILL.md` is no exception,
but it must write that path as **inline code rather than as a Markdown link**, and the
two constraints that force it point in opposite directions:

- An agent reads files from the workspace root. `<harness>/rules/delivery.md` is the
  path it can act on directly; a relative walk like `../../rules/delivery.md` is one
  the agent has to resolve against a directory it was never told it was in.
- `skill` resolves a Markdown link **against the skill's own directory** and holds it
  to the Agent Skills spec's one-level-deep rule. The same path written as a link
  would resolve to `<harness>/skills/<n>/<harness>/rules/…`, and scc would report two
  findings against its own scaffold.

Inline code satisfies both: correct to read, and not a link, so the check that would
misjudge it never sees it. This was got wrong once in the other direction — skills
linking relatively, which conformed but handed the agent a path it could not use.

Guarded from both sides, because neither guard alone is enough.
`TestFreshArtifactsPassTheirOwnValidators` catches the link form, since a validator
firing on scc's own output is the one defect that teaches users to disbelieve the
other seven. `TestSkillsNameRulesByTheirHarnessPath` catches what no validator can
see — a `../..` sitting inside a code span is silently useless to whoever reads it.

### The four seeded anchors — `docs/` is not an empty tree

`init` writes `docs/glossary.md`, `docs/stack.md`, `docs/wiki/index.md`, and
`docs/wiki/changelog.md`, each one a heading and the format its validator checks.
These are the knowledge base's only documents with a *fixed name*; a wiki page, an
ADR, and a codewiki page are named after what they are about, so those directories
are correctly scaffolded empty.

The reason is the same one that makes the skills ship: a workspace that arrives with
eight validators and an empty `docs/` demands conformance to documents nobody was
handed. Seeding also improves the first finding a real project sees — a repo with
dependencies used to get one `stack.missing`, and now gets one line per undocumented
dependency, pointing at a file that already explains what an entry looks like.

**A seed is written once and tracked nowhere** — not in the manifest, not by
`scc update`. It is a third category alongside managed files and artifact templates,
and it is closer to the artifacts: the moment the file exists it holds project
knowledge, which scc has no improved version of to deliver. Untracked also settles
the two-harness case without a rule of its own, because `docs/` is one tree per repo
rather than one per harness — the second `init` finds the files already there and
leaves them alone, exactly as it does for any existing file.

## 7 · Agents — review only

Two subagents ship with the workspace, and both of them read rather than write:

| Agent | Role |
|---|---|
| **code-review** | Runs five gates over the diff: the ticked boxes are true, the code matches the tasks, the feature's tests, the lint, and the best practices no linter has an opinion about. |
| **security-review** | Looks for what the change makes possible, attack-class agnostic: surface, trust boundaries, reachability, deliberate attack. |

Splitting review by lens is deliberate: a single reviewer asked for "everything"
reliably under-weights security, because the correctness findings are easier to
produce and crowd it out.

Review is where a subagent genuinely earns its cost, and for the opposite reason
implementation doesn't (§8): **a cold context is the feature.** The author of a
change is its worst reader — they see what they meant. And a reviewer reads instead
of writes, so running one on a cheaper model costs far less than delegating
authorship would.

**Both pin `model: sonnet` and `effort: high`.** The mid tier is the right trade for
work that reads and judges rather than authors, and the reasoning budget is where the
quality actually comes from here: tracing a value from an argument to a shell, or a
ticked box to the code behind it, is chains-of-inference work, not knowledge work. A
cheap model at low effort produces the review's *shape* without its content, which is
worse than no review because it reads like one.

**A checklist for correctness, a method for security.** They differ because the
questions differ. Correctness has a finite, knowable set of things the orchestrator
already claimed to have done — so the reviewer re-does them as named gates and
re-runs the commands itself, since "the author said it was green" is exactly the
evidence a cold reader exists to distrust. Security has no such set: a checklist of
attack names bounds the review at the names, and the interesting weakness is the one
nobody named. So that agent gets passes over the code — surface, boundaries,
reachability, attack — with the known classes demoted to prompts that jog the passes
and explicitly not the definition of done.

**Both report in a fixed shape, and the shape is the handoff.** A verdict, a table of
what was actually checked (including what could not be run, as `not-run` and never as
a pass), findings with severities that map to actions, and a place for the
unconfirmed suspicion so it is not smuggled in as a finding. The orchestrator branches
on that report in §9 without re-reading the diff — which is the whole point of paying
for a second context.

### The orchestrator closes the loop on every task

A task is not done when the code is written. Whoever built it — which is always the
orchestrator — finishes by verifying it, in this order:

1. **Build the task** — Unit or TDD, per the annotation on the task line.
2. **Run the tests in its scope** — the tests covering what was just built, not the
   whole suite.
3. **Run the lint** — the project's linter, as the best-practices layer that finds
   the programming defects tests don't: unused code, unchecked errors, shadowed
   variables, unsafe conversions.
4. **Move on, or fix** — a red task is not finished.

**Scope, not suite,** because per-task feedback needs to be fast and attributable.
Running everything after each task buys little: the failure that a full run catches
and a scoped run misses is breakage *between* tasks, and that is worth looking for
once the work is integrated, not N times along the way. The full suite runs at the
end of the spec (or of each of a plan's leaves).

**Tests and lint are both required, because they answer different questions.** The
tests say the code does what the task asked. The lint says the code is written the
way this project writes code. A task that passes its tests with an unchecked error
in it is not finished, and neither check substitutes for the other.

The linter is whatever the project already uses — scc does not own it, for the same
reason it does not parse source (§5).

### The file is the record; the todo list is the session

Claude Code already tracks work in progress with its own checklist, and the
orchestrator drives each task through it — one item in progress at a time, marked off
as it goes. That is the execution surface, and scc does not replace it.

The two must not drift apart:

- **`specs/<feature>/tasks.md`**, or the checklist in **`plans/<name>.md`**, is the
  durable record. It survives the session, it is what gets reviewed and committed, and it
  is what scc validates.
- **The session checklist** is working state. It is where the orchestrator tracks the
  task it is on right now.

So checking an item off in the session means checking the `- [ ]` box in the file too. A
session that ends with the checklist complete and the file untouched has lost everything
except the code — the next session has no idea which tasks were done, and neither does
the reviewer.

This is the same reason scc has no daemon and no database (§0): Claude Code's own
mechanisms are the interface, and the Markdown is the memory.

## 8 · Why implementation is sequential

**The orchestrator writes the code. There is no implementation subagent, and no
parallel dispatch.** This was designed the other way first — a task-level
implementation agent, worktree per task, four layers of isolation to keep concurrent
runs from poisoning each other — and then rejected. Recording why, so it doesn't get
re-proposed:

- **The only upside was wall-clock.** Everything else about delegating implementation
  was a cost.
- **It puts the weakest participant on the hardest work.** Subagents typically run a
  cheaper model. Delegating implementation hands off *writing the code* — the part
  that most needs capability — while the orchestrator keeps routing, which needs the
  least. That is backwards.
- **Every agent pays for discovery again.** It starts with no context and rediscovers
  the codebase. Within one spec, the orchestrator's accumulated context is the
  *asset*, not clutter: it uses the right parser in task 1.2 precisely because it
  wrote task 1.1.
- **File-disjointness is not independence, and the merge does not catch the
  difference.** Two tasks touching no common file both need a `Money` type that
  doesn't exist yet. Each agent creates its own, in its own file, with different
  semantics. The merge is *clean*. You end up with two `Money` types and no signal
  that anything went wrong. Sequential execution cannot produce this, because the
  later task sees the earlier task's code.

The last one is decisive. The isolation machinery was sound against tree
interference, edit conflicts, resource collisions, and cross-task breakage — and
blind to the failure that partial context actually causes.

Sequential execution has a second benefit worth naming: nothing needs isolating, so
separate checkouts, per-agent resource namespacing, merge-conflict resolution, and
result attribution all stop being problems rather than being solved.

None of this rules out parallelism as such — only *dispatched, task-level*
parallelism. Feature-level parallelism is real and supported: the user runs several
Claude Code sessions, one per feature, and merges them into `main`. See §9, which lays
out why that version survives the objections above.

## 9 · Delivery — branch, PR

Work does not happen on `main` and does not end with a green test run. It ends with a
pull request.

### Branch in the checkout you are in

Each unit of work — a spec, or a plan's leaf — gets its own branch, cut from a green
`main` in the checkout the session is already sitting in, and the checkout goes back to
`main` and clean once the work has landed. That last part is the whole discipline: the
next unit of work starts where this one did.

**Nothing here needs a second directory, and the procedure used to make one.** It was
there for one case — **the user running several Claude Code sessions at once**, one per
feature, each needing a directory of its own, since a shared tree with `git switch`
would have two sessions fighting over one working directory. That case is real and
still supported, but arranging it is the user's own setup, made once for the runs they
actually parallelize. Making it a step of the ordinary procedure charged every
single-session run for it: a directory to create, one to switch into, one to remember
to remove, and a stale checkout left behind whenever a run died before the last step.
What survives is the line that setup was really carrying — leave the checkout clean, on
`main`.

So parallelism is back, and it is worth being precise about why this version is fine
when §8's was not. **The user drives this one; the orchestrator drove that one.** Four
differences, and they all matter:

| | §8 — rejected | §9 — this |
|---|---|---|
| Unit of work | one task | one feature |
| Who splits it | the orchestrator, guessing from file overlap | the human, who knows the domain |
| Who does the work | a subagent, typically a cheaper model | a full session, same capability as any other |
| Discovery cost | re-paid per task | paid once per feature, amortized across it |

The `Money` failure that killed §8 is the clearest case. Two *tasks* in one spec both
needing a type that doesn't exist yet is routine — they are neighbors by construction.
Two *features* a human deliberately separated are much less likely to collide, and
within each session the accumulated context prevents it outright. The risk drops from
"expected" to "possible", and the human picked the split, so it is their call to make.

### What running several sessions still costs

Separate sessions isolate files. They do not isolate the world outside them, so two of
these concerns from §8 survive at feature granularity and should be said plainly:

- **Shared external resources.** Two sessions running the suite at the same time will
  fight over a fixed port, one test database, or a shared temp path. Either the suite
  namespaces those per session, or the test runs have to be serialized. Nothing about
  a separate directory fixes this.
- **Cross-feature breakage.** Two features green on their own branches can be broken
  together. Only CI on `main` after the merge sees that — which is a good reason to
  care about the merge order and about `main` staying green.

Both are the user's to accept: they chose to run two sessions. Naming them is the
point, so the cost is visible before it is paid.

### The delivery sequence

Once the last task is done:

1. **Full suite + lint** on the integrated branch — the per-task scoped runs (§7)
   cannot see breakage between tasks.
2. **`code-review` and `security-review`** on the diff (§7), dispatched together, and
   fix from their reports by severity: `blocker`/`critical` stops the PR,
   `major`/`high` is fixed before merge, `minor`/`low` is a judgment call stated in
   the PR body. The PR should arrive already reviewed; a PR is for the human, and
   spending their attention on findings a subagent would have caught is the waste
   this ordering avoids. One fix-and-re-review round; a second means the finding
   wants a person.
3. **Commit and push** — Conventional Commits, generated from the diff and the spec.
4. **Open the PR.**

### Waiting for CI is a question, not a policy

CI is the one part of this the orchestrator cannot make faster, and its runtime varies
from thirty seconds to thirty minutes. So the orchestrator **asks**:

- **Wait** — it watches the PR's checks until they settle. Red means fix, push, and
  keep watching; the work is not finished while CI is failing.
- **Don't wait** — opening the PR is the finish line. CI runs; the human picks it up.

Recorded as `ci: wait | no-wait` in the same frontmatter block as §2's answer.

Ask this at kickoff, together with §2's autonomy question, not when the PR is open.
By then the work is done and the user may well be gone — which is exactly the
situation "don't wait" exists for, and exactly when a blocking question is most
expensive. Both answers get recorded on the artifact for the same reason: the run
stays reproducible from the file, and nobody gets asked twice.

### Degrading

- **No remote, or no `gh`** — commit on the branch and stop there, saying so. A branch
  the user can push themselves is a real deliverable; silently skipping the PR is not.
- **A checkout left dirty or off `main`** is what this shape can leave behind. Say what
  is still uncommitted rather than starting the next unit of work on top of it.

## 10 · The three spec artifacts

§1 routes work into a spec at `specs/<feature>/`; this is what those three files
actually are.

### requirements.md — EARS, all five patterns

Requirements use EARS (Alistair Mavin et al., Rolls-Royce, IEEE RE 2009), numbered
`R<group>.<item>` and written as list items so the later phases can cite them:

```markdown
- **R1.1** The billing engine shall compute the order total
- **R1.2** When a coupon is applied, the billing engine shall recompute the total
```

Only lines in that position are parsed. Prose elsewhere in `requirements.md` is not a
requirement and gets no findings — the alternative is a validator that fires on the
document's own introduction. This is not ceremony — it is the only reason `scc` can check a
requirement at all. Prose has nothing to validate; an EARS clause has named parts, and a
missing one is a finding.

The ruleset: **zero or many preconditions · zero or one trigger · one system name · one or
many responses**, and the clauses always appear in that order. That yields five patterns
plus combinations:

| Pattern | Shape |
|---|---|
| Ubiquitous | `The <system> shall <response>` |
| State-driven | `While <precondition>, the <system> shall <response>` |
| Event-driven | `When <trigger>, the <system> shall <response>` |
| Optional feature | `Where <feature>, the <system> shall <response>` |
| Unwanted behavior | `If <trigger>, then the <system> shall <response>` |
| Complex | more than one keyword — `While <precondition>, when <trigger>, the <system> shall <response>` |

**All five are valid and the validator must accept all five.** An earlier draft of this
document described EARS as event-driven only (`WHEN … THE SYSTEM SHALL …`). A validator
built on that would reject four legitimate patterns, and — worse — would push authors to
invent a trigger for a requirement that is simply always true. Unwanted behavior (`If …
then …`) is the pattern most often missing from generated requirements, and the one that
most often matters.

### Changing an existing spec — deltas, not rewrites

A change to a spec that already exists is written as a **delta** against it, with each
affected requirement marked `ADDED`, `MODIFIED`, or `REMOVED` (the shape OpenSpec settled
on for brownfield work) — **as a marker on the requirement line itself**, in
`requirements.md`, not in a separate file:

```markdown
- **R2.3** (MODIFIED) When the cart is empty, the checkout shall skip the summary
- **R2.7** (ADDED) If the coupon has expired, then the checkout shall reject it
- **R1.4** (REMOVED)
```

Inline rather than a `deltas/` directory, because the second reason below is about two
sessions touching *different requirements in one file* — which a marker per requirement
gives directly, and a separate file per change does not. A `REMOVED` requirement is a
statement that it is gone: nothing is left to hold to EARS, and no task has to reach it.

Three reasons, and the second is the one that decides it:

1. **You specify the change, not the system.** Adopting scc on an existing codebase must
   not require writing the spec for everything that already works. Specs grow one change
   at a time, filling in around the work actually done.
2. **Deltas scoped to individual requirements make concurrent edits safe.** Two sessions
   (§9) can change the same spec without conflicting, as long as they touch different
   requirements. Whole-file rewrites collide on contact.
3. **A reviewer reads intent instead of reconstructing it.** The delta says what changed
   and why; a raw diff makes them infer it.

The delta is how a change is *proposed and reviewed*. Once it lands, it is folded into the
spec — the spec stays the current statement of the feature, never an append-only log of
deltas.

**Requirements are subject to "omit, don't fill" too.** Structured requirements measurably
improve generated code — adding structured specification to a natural-language baseline
significantly improves output quality — but the curve is not monotonic: over-specification
can constrain the model's reasoning or introduce requirements that conflict with each other,
and correctness drops. EARS structure, yes; exhaustive enumeration of everything the feature
might touch, no. A requirements document that specifies more than the feature actually
decides is not merely long — it can produce worse code than a shorter one.

### design.md — scaled by complexity

**The design must fit the decision being made.** The common failure is a design that
invents architecture for a change that had no architectural question in it: a
component diagram for a two-function addition, a data-model section for something that
touches no data.

That is worse than verbose. **Invented architecture constrains.** The next person — or
the next session — reads it as a decision somebody made, and honors it. Filler becomes
binding.

So the sections are conditional, and the rule is **omit, don't fill**:

| When the change… | design.md carries |
|---|---|
| decides nothing structural | what changes, where, and why — a few paragraphs. No components, no diagram. |
| moves a boundary, a data shape, or an external contract | those sections only, for the parts that actually change. |
| has real alternatives with trade-offs | the alternatives and why one won — and an ADR if the decision is hard to reverse. |

A heading filled with "N/A", or with prose written to satisfy the heading, is worse
than an absent heading: it reads as a decision, and no one can tell it apart from one.

**This constrains scc more than it constrains the orchestrator.** A required heading is
a request for filler — a template shipping an `## Architecture` section *causes* the
behavior being complained about, and a validator demanding that section makes it
mandatory. So the design template marks those sections clearly optional, and validation
checks that the design exists and traces to its requirements. It never checks that a
particular section is present.

### tasks.md — the grammar in §4

Exactly one methodology per task, `(Unit)` or `(TDD)`, validated. Tasks cite the
requirements they satisfy, which is what makes §5's traceability check possible.

## 11 · The spec is anchored, not disposable

A spec does not stop being true when its feature merges. **Work that touches an area a
spec covers updates that spec as part of the delivery** — as a delta (§10), in the same
branch, in the same PR as the code.

The field has a name for this choice. Three tiers are in circulation: **spec-first** (specs
precede code, then are discarded), **spec-anchored** (specs persist and evolve with the
code), and **spec-as-source** (only specs are edited; code is generated). scc is
**spec-anchored**, and the reason is the same one that made the artifact mandatory in the
first place (§1): under autonomy, the file is the only record of intent. A record that
stops being maintained stops being a record — and a stale requirement read as current is
worse than an absent one, because it is believed.

What that does *not* mean:

- **No mechanical drift detection.** Verifying that a spec still matches the code requires
  understanding the code, which §5 rules out. The literature's version of spec-anchored
  includes automated drift checks; ours doesn't, and that is a deliberate downgrade rather
  than an oversight. Keeping the spec current is the orchestrator's obligation, checked by
  the reviewer, not proven by the binary.
- **Not permanence for its own sake.** A feature genuinely deleted takes its spec with it.
  Anchored means *maintained while the code exists*, not *never removed*.

The knowledge base stays separate and keeps its own job. `docs/wiki/` and `docs/adr/`
answer *why* — the durable reasoning, the decisions, the material read from outside. The
spec answers *what this feature does now*. Neither replaces the other, and the spec being
anchored is what keeps them from having to.

## 12 · Upgrading a workspace — three-way merge, not backups

A new scc version ships improved templates. Bringing a workspace up to it must never lose
what the user changed, and **must not make them re-merge by hand**.

The mechanism, borrowed from Copier rather than from csdd: re-render the templates **as the
old version produced them**, re-render the **new** version, diff those two, and apply that
diff to the working tree as a **three-way merge**. Local edits survive unless they collide on
the same lines, and a collision surfaces as **standard git conflict markers**.

Why that beats csdd's numbered `.old` backups: a backup file preserves the user's work and
then hands them the merge as homework. Conflict markers put the resolution in the tool they
already use for exactly this, and it is the same mechanism §9 relies on for branches — one
conflict idiom in the whole product instead of two.

**This constrains the manifest.** A three-way merge needs the old rendered *text*, not just
its hash. scc embeds its templates, so it can re-render any past version — but only if it
knows which version produced each file, and for which harness. So `<harness>/scc-manifest.json`
records **a version per entry alongside the content hash**, and the harness once for the
file: the hash answers "did the user edit this?", the version and harness answer "what did
it look like before?" All three are required, and a manifest carrying only hashes cannot
support this.

Files that exist to be edited once and then owned by the user are **excluded from the merge**
rather than merged badly.

### What shipped first: replace-or-keep, with the plan shown

The merge above is the destination. What `scc update` does today is the honest subset
of it, and it is deliberately not a merge:

- Every managed file is hashed against what this build renders and against what the
  manifest says scc last wrote. That splits it four ways — **current**, **create**
  (new in this version, or deleted), **update** (still exactly what scc wrote, and
  the template moved), **conflict** (the user changed it) — plus **delete** for a
  file this version no longer ships and **owned** for the two files that exist to be
  edited.
- **The plan is printed and confirmed before anything is written.** Not a nicety: an
  update is the one command that can destroy work, and a summary the user agrees to
  is what makes the difference between a tool and a surprise. `--yes` is that
  agreement given up front and is required when stdin is not a terminal, `--dry-run`
  reports and stops.
- **A conflict is kept, not merged and not clobbered**, and is reported by name with
  what to do about it. `--force` is a separate, explicit decision. The entry it keeps
  in the manifest stays at the version scc last wrote — which is precisely the base
  revision the three-way merge will need when it lands.

The merge is what upgrades this into: same plan, same confirmation, but a conflict
gets resolved instead of deferred.

## Open questions

- **Does a Plan's decomposition get validated for coverage** — i.e. can scc tell
  that the specs and tasks a plan produced actually cover the plan? This is an
  artifact-level question, so unlike the code checks above it is fair game.
- **What records the routing decision?** The choice of Spec vs Plan is a judgment call;
  whether it carries a written justification (auditable, verbose) or stays implicit in
  which artifact exists (cheap, unaccountable) is undecided.
- **What happens to a spec after its feature merges?** It stops describing the code and
  starts describing what was once intended — and a stale requirement read as current is
  worse than an absent one. csdd keeps `specs/` forever. The alternative is that the
  spec is scaffolding: durable knowledge migrates into `docs/wiki/` and the decisions
  into `docs/adr/` (both of which scc keeps), after which the spec can be archived or
  deleted without losing anything. That would make the knowledge base the permanent
  record and the spec a working document — but it needs a rule for *when* the migration
  is due, or it never happens.
