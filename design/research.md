# Research — where scc's design sits in the field

State of the art as of July 2026, gathered to check `orchestration.md` against what the
community and the literature actually converged on. Sources at the bottom.

Read this as a checklist against our design, not as a survey. Three groups: standards we
should conform to, findings that **challenge** a decision we made, and findings that
confirm one.

---

## 1 · Standards that actually exist

### Agent Skills / `SKILL.md` — a real open standard, and we can validate against it

Anthropic published the Agent Skills spec on 2025-12-18; by March 2026 **32 tools** from
competing vendors read the same `SKILL.md` from the same directory layout (Gemini CLI,
JetBrains Junie, AWS Kiro, Block Goose, Cursor, Codex CLI…). The authoritative spec lives
at `agentskills.io/specification`, outside Anthropic.

Hard constraints, all mechanically checkable:

| Field | Rule |
|---|---|
| `name` | required · 1–64 chars · lowercase `a-z0-9` and `-` only · no leading/trailing hyphen · **no consecutive hyphens** · **must match the parent directory name** |
| `description` | required · 1–1024 chars · should say *what* and *when* |
| `license`, `compatibility` (≤500), `metadata`, `allowed-tools` | optional |

Layout is `skill-name/SKILL.md` plus optional `scripts/`, `references/`, `assets/`.
Progressive disclosure has three tiers: metadata (~100 tokens, loaded at startup for
*every* skill), the body (loaded on activation, **<5000 tokens recommended, keep under 500
lines**), and resources (loaded only when needed). References should stay **one level deep**.

**Consequences for scc.** This is the ideal shape of an scc check: a published spec, all of
it artifact-level, none of it requiring us to parse source (§5). A `skill validate` command
has an external contract to hold skills to rather than an opinion. Note also that
`skills-ref validate` already exists as a reference validator — worth reading before
writing ours, and worth deciding whether we duplicate it or defer to it.

### EARS — our design doc states it wrong

EARS (Alistair Mavin et al., Rolls-Royce, IEEE RE 2009) has a ruleset: **zero or many
preconditions · zero or one trigger · one system name · one or many responses**, with the
clauses always in that order. It produces **five** patterns plus combinations:

| Pattern | Shape |
|---|---|
| Ubiquitous | `The <system> shall <response>` |
| State-driven | `While <precondition>, the <system> shall <response>` |
| Event-driven | `When <trigger>, the <system> shall <response>` |
| Optional feature | `Where <feature>, the <system> shall <response>` |
| Unwanted behavior | `If <trigger>, then the <system> shall <response>` |
| Complex | more than one keyword, e.g. `While … When …, the <system> shall …` |

**`orchestration.md` §10 currently describes EARS as `WHEN <trigger> THE SYSTEM SHALL
<response>`** — that is only the event-driven pattern. A validator built on that sentence
would reject four valid patterns, and would push authors to fake a trigger for
requirements that are simply always true. **Fix §10 before anything validates
requirements.**

### AGENTS.md — the cross-tool standard Claude Code doesn't read

`AGENTS.md` became the de facto standard for giving coding agents project context across
2025–2026. **Claude Code does not read it natively** — only `CLAUDE.md`. The usual bridge
is to keep the portable content in `AGENTS.md` and import it from `CLAUDE.md`.

**Open decision for scc:** scaffold `AGENTS.md` + a `CLAUDE.md` that imports it (portable
across the 30+ tools, one more file), or `CLAUDE.md` only (simpler, Claude-Code-only —
which is what the product name commits to anyway). Not decided.

---

## 2 · Findings that challenge our design

### 2.1 TDD instructions without test context made things *worse* — arXiv 2603.17973

TDAD (Test-Driven Agentic Development) measured code regression in agents. The headline is
not that TDD helps; it is the paradox: **adding generic TDD procedural instructions,
disconnected from which tests actually exercise the code being changed, increased
regressions above the no-intervention baseline.** What worked was **graph-based impact
analysis** — a dependency graph identifying the tests that genuinely cover the modified
paths, so the agent gets only relevant test context.

Reported effect of the working version: regressions cut roughly 70% (≈6.1% → ≈1.8%); the
instructions-only variant rose to ≈9.9%, worse than doing nothing.

This lands directly on two of our decisions:

- **§3/§6 put "TDD for money, complex algorithms, hypotheses, high risk" in
  `.claude/rules/` as a procedural rule.** That is exactly the intervention shape the paper
  says backfires on its own.
- **We trimmed the graph to Markdown artifacts only** (no source extractors), and the
  paper's working mechanism *is* a code-dependency graph.

So there is a real tension between "scc never parses code" (§5) and the one intervention
with measured evidence behind it. Options, none free: accept the risk and keep the rule
textual; have the *orchestrator* do impact analysis ad hoc (it can read the code — scc
still doesn't); or let a project-configured hook supply the relevant-test set. **This
deserves an explicit decision rather than silence.**

Caveat worth keeping: the fetched PDF text did not expose the tables, so the exact
percentages above come from secondary reporting of the paper. The qualitative finding —
instructions-only was worse than baseline — is stated in the paper itself.

### 2.2 "Plan" means the opposite thing in the dominant tool

In GitHub Spec Kit, `/speckit.plan` produces `plan.md` **plus** `data-model.md`,
`contracts/`, `research.md`, `quickstart.md` — i.e. spec-kit's *plan* is our *design
phase*, sitting **inside** one feature. Our plan is the **decomposition above** specs.

Anyone arriving from spec-kit (the most widely integrated toolkit — 29+ agent
integrations) will read `plans/checkout-revamp.md` and expect architecture. This is a
naming cost, not a design flaw, but it is worth paying attention to before the vocabulary
sets.

### 2.3 We removed `[P]`; the field standardized on it

Spec Kit marks parallel-safe tasks `[P]` in `tasks.md` and documents safe parallelization
groups. csdd had the same marker. We removed it in §8 along with dispatched parallelism.

Our reasoning stands on its own (file-disjointness ≠ semantic independence; the merge
doesn't catch duplicate invention), and Anthropic's own guidance agrees — see 3.1 — but we
should know we are diverging from the field's default on this, deliberately.

### 2.4 Spec Kit mandates TDD for everything; we default to Unit

Spec Kit's constitution, Article III: "All implementation MUST follow strict Test-Driven
Development." Our §3 makes Unit the default and TDD risk-triggered. Given 2.1, the
gap between these two positions is where most of our correctness risk lives.

### 2.5 The community has a name for our unanswered question — and we're in the weakest tier

Maturity taxonomy in circulation:

| Tier | Meaning |
|---|---|
| **Spec-First** | specs precede code, then are discarded |
| **Spec-Anchored** | specs persist, evolve with the code, and code validates against them |
| **Spec-as-Source** | only specs are edited; code is generated (Tessl) |

"The Spec Growth Engine" (arXiv 2606.27045) argues for **spec-anchored** with explicit
**drift detection** — spec drift being the divergence of documentation from
implementation. Our open question "what happens to a spec after its feature merges" is
precisely the spec-first/spec-anchored choice, and our current design leans spec-first,
the tier the literature treats as weakest.

### 2.6 OpenSpec's delta format solves a brownfield problem we don't address

OpenSpec is brownfield-first: changes are written as **deltas** against existing specs,
with `ADDED` / `MODIFIED` / `REMOVED` markers, so you specify the change rather than the
whole system. Two properties we'd want:

- **Deltas scoped to individual requirements let two in-flight changes edit the same
  `spec.md` without conflicting** — directly relevant to §9's multiple parallel sessions.
- Specs **grow one change at a time**, filling in around the work actually done, instead
  of demanding an up-front full-system spec.

scc currently assumes a spec describes a feature from scratch. Most real use is brownfield.

### 2.7 Context rot argues *for* delegation — the one place §8 is uncomfortable

Chroma's study across 18 frontier models (GPT-4.1, Claude 4 family, Gemini 2.5, Qwen3):
accuracy degrades **non-uniformly** as input grows, **30–50% before the documented limit**,
across varied task types and *sooner* on complex tasks. Above ~50% context fill, attention
favors recency over position.

§8 rejected the implementation subagent partly because "the orchestrator's accumulated
context is the asset." Context rot says that asset decays, and decays faster on the hard
tasks. This does not overturn §8 — our objection was about *partial* context producing
duplicate inventions, and about model tiering — but it does mean a long single-session
orchestrator has a ceiling. It is an argument for bounding a spec's size, and for treating
compaction as expected rather than exceptional.

---

## 3 · Findings that confirm our design

### 3.1 Anthropic's own guidance matches §8

"Subagents work best for **read-heavy research and exploration, not parallel coding**."
That is §8's conclusion and §7's split (review agents read; nobody delegates authorship),
arriving from the vendor rather than from our reasoning. Cost data in the same direction:
multi-agent workflows run ~4–7× the tokens of a single-agent session, agent teams ~15×.

### 3.2 Worktree-per-session is the community pattern, caveats included

The published playbooks say what §9 says: one worktree per parallel session, each running
its own tests, diffs reviewed and merged in dependency order. They also independently hit
our §9 resource-collision caveat — recommending `.env.local` per worktree (gitignored) and
explicit port/database isolation, because a committed shared `.env` is read by every
worktree. Our "worktrees isolate files, not the world outside them" is the same lesson.

### 3.3 Agent context files do improve efficiency — but the popular number is folklore

The measured study (arXiv 2601.20404): median wall-clock **98.57s → 70.34s (−28.6%)**,
median output tokens **2,925 → 2,440 (−16.6%)**; input tokens essentially flat.

**Important correction.** Blog posts widely cite this work for "best results came from
files with just three sections" and "aim for under 150 lines." **The paper contains no such
finding.** It measures operational efficiency only, explicitly states that output-quality
evaluation is out of scope (a sanity check on 50 sampled tasks confirmed non-empty
changes), and reports nothing about optimal size or structure. Do not cite it for a length
prescription.

The lean-`CLAUDE.md` instinct is still well founded — just cite **context rot** (2.7) for
it, which does measure degradation with length, rather than a study that didn't look.

### 3.4 The problem statement is the field's problem statement

The framing that motivated this project is the field's: the 2026 bottleneck is not
generation speed but **drift** — plausible code that confidently solves the wrong problem
because nothing grounded the work. Every major tool shipped an SDD flavor (Spec Kit, Kiro,
Cursor, OpenSpec, BMAD, Tessl, Antigravity), and skills-based installation is now a
first-class mode for Claude Code and Codex CLI rather than slash commands only.

---

## 4 · What was changed as a result

All seven items are resolved in `orchestration.md`. Recorded here so the reasoning stays
attached to the evidence:

| # | Finding | Resolution |
|---|---|---|
| 1.2 | EARS is five patterns, not one | **§10 rewritten** — all five plus complex; the validator must accept all five. Was a latent bug: the old text would have rejected valid requirements and pushed authors to fake triggers. |
| 2.1 | TDD procedure without test context measured *worse than nothing* | **New §3 step, before both cycles**: identify and run the tests that already cover the paths being changed. The orchestrator does the impact analysis (it reads code); scc still doesn't. |
| 2.5 | Spec-first is the weakest tier | **New §11 — spec-anchored.** Work touching a covered area updates that spec in the same PR. Explicitly *without* mechanical drift detection, which would need code parsing (§5); that downgrade is deliberate and stated. |
| 2.6 | Brownfield needs deltas | **§10 gained a delta section** — `ADDED`/`MODIFIED`/`REMOVED` per requirement. Decisive reason: deltas scoped per requirement let §9's concurrent sessions edit one spec without colliding. |
| 1.1 | Agent Skills is a published external standard | **§5 gained a validator catalogue** with `skill` conforming to the spec. Implemented in Go, not delegated to `skills-ref` — scc is one binary and shelling to Node for a few regexes would break that. |
| 1.3 | AGENTS.md is the cross-tool standard Claude Code can't read | **Declined, in §6.** Shipping it means shipping an import too — two files and an indirection for portability to tools this product isn't for. |
| 2.2 | "plan" means the opposite in spec-kit | **Kept `plans/`**, with a vocabulary warning in §1 so it isn't "fixed" later by someone who only knows spec-kit. |

Two further changes the research prompted that weren't on the original list:

- **§6 now cites context rot** for the lean `CLAUDE.md`, replacing the `AGENTS.md`
  efficiency study — because that study measured tokens and wall-clock, not quality, and the
  popular length prescription isn't in it (3.3). The instinct was right; the citation was
  wrong.
- **`stack` became a real gate** rather than a principle: dependency manifests (`go.mod`,
  `package.json`) are structured data, so diffing them against `docs/stack.md` catches a
  silently adopted dependency without reading any source.

Nothing here overturned the two-vehicle model, sequential implementation, or review-only
agents — and 3.1 independently confirmed the last two. Every gap was in the *artifact
contracts*, which is where scc's whole value is.

Still open after this pass: **context rot vs. the long-lived orchestrator context** (2.7).
It doesn't overturn §8, but it means a spec's size has a ceiling and compaction is expected
rather than exceptional. No decision recorded yet.

---

---

## 5 · Second pass — the gaps we'd left

The first pass checked the orchestration decisions. This one covers what it skipped: our own
default methodology, the autonomy call, the product's core premise, the knowledge base, and
workspace mechanics.

### 5.1 Tests-after is defensible — but only *iterative*, and agents fail it differently

The classical empirical record on TDD vs test-last is far more mixed than its reputation.
Controlled experiments with professionals: TDD's benefits over **iterative** test-last are
"small and thus in practice relatively unimportant, although effects are positive." TDD
programmers passed ~18% more black-box tests but took ~16% longer; the branch-coverage
advantage has a small effect size; in at least one experiment the test-last team was *more*
productive and wrote *more* tests. Participants found test-last easier and preferred it.

**This supports our Unit default — with a condition that must be written into the rule.**
The literature's test-last comparator is *iterative*: tests written per unit, immediately
after it. Nothing supports "tests at the end of the feature." Our §3 Unit says "a unit test
for each function," which is the right shape; it must say *immediately*, or it drifts into
the variant nobody has evidence for.

**And the human record does not transfer whole.** Those studies measured humans, whose
failure mode with test-after is skipping tests. An agent's failure mode is different and
worse: it writes tests that assert **what the code does** rather than what it should do —
green tests that encode the bug. That is the specific risk our default carries, and it needs
its own mitigation: the test must be derived from the **requirement**, not read off the
implementation.

### 5.2 The premise holds — and over-specification is a real failure mode

Structured specifications do help LLMs, measurably: SSDE (arXiv 2605.02455) found "adding
any type of structured specifications significantly improves output quality compared to the
baseline where only the natural language specification is provided"; SpecSyn reports a 21%
accuracy gain on contract generation. The product's premise is sound.

But the relationship is not monotonic. "When Prompt Under-Specification Improves Code
Correctness" (arXiv 2604.24712) found over-specification can **constrain the model's
reasoning or introduce conflicting requirements**, degrading correctness — there is a sweet
spot rather than a "more detail is better" gradient.

That is §10's "omit, don't fill" rule arriving from outside, and it extends further than we
applied it: we scoped it to `design.md`, and it applies to **requirements** too. EARS
structure yes; exhaustive enumeration no. A requirements document that specifies more than
the feature decides is not merely long — it can produce worse code.

### 5.3 Autonomy: §2 is too binary

The practice literature converges on three patterns: **pre-execution approval** (halt before
each consequential action), **post-execution review** (act, then surface for inspection), and
**escalation triggers** (run autonomously, halt on specific risk signals — irreversible
operations, sensitive data, low confidence). "Governed AI-Assisted Engineering" (arXiv
2606.22484) proposes graduated tiers — automated / standard review / expert review /
management approval / compliance check — selected by risk, scope, domain sensitivity, and
code patterns. The recurring principle: **autonomy is earned through demonstrated
performance, not assumed at deployment**, and approval mode is useful as trust calibration
early on.

Our §2 asks once at kickoff and then runs to completion. That is *one* of the three patterns,
and it has no escalation path: once "automatic" is chosen, a task touching money is treated
like a task renaming a variable.

**The fix is already in the design.** §3's TDD trigger list — money, complex algorithms,
hypothesis validation, high-risk complexity — is a risk classifier we already require the
orchestrator to apply per task. The same list can gate. A task annotated `(TDD)` because it
touches money is exactly the task worth surfacing even in autonomous mode. One classifier,
two consumers.

### 5.4 ADR: adopt an existing format, keep our own trigger

There are established formats and we should not invent one. **Nygard** (2011): title, status,
context, decision, consequences. **MADR** (Markdown ADR) is the most widely used, extending
Nygard with considered options, pros and cons per option, and links to related decisions.
Templates are curated at `adr.github.io`.

One detail from the Nygard format is worth preserving deliberately: **negative consequences
go in the same section as positive ones.** That is what stops an ADR from becoming a sales
pitch for the decision that was already made — a real risk when the author is an agent that
just made it.

csdd's "triple gate" (hard to reverse · surprising without context · a real trade-off) is not
a competing format — it is a **trigger** deciding *when* an ADR is warranted, which the
standard templates don't address. Keep the gate, adopt MADR's shape.

### 5.5 Glossary: the machine-checkable half is exactly the half machines are good at

"AI coding agents amplify whatever vocabulary you give them" — precise vocabulary becomes
correct class names, lifecycle states, and boundaries. And the limit is documented: LLMs are
**competent at DDD's mechanical layers** (lexical consistency, structural alignment,
pattern matching) and **unreliable on the parts requiring real domain knowledge**.

That is a clean division of labour and it maps onto our design exactly: the human decides
which term is canonical (domain knowledge); the orchestrator uses it consistently (lexical
consistency, where it is strong); `scc` lints the avoided synonyms (mechanical). Keeping the
glossary is well founded, and the lint is the part that carries the value.

### 5.6 `update` should use a three-way merge, not backup files

Copier solved the problem csdd solved with numbered `.old` backups, and solved it better. On
update it: checks out the **old** template version, renders it with the saved answers to
reconstruct "old generated", renders the **new** version the same way, diffs those two, and
applies that diff to the working tree as a **three-way merge**. Local edits survive unless
they collide on the same lines; collisions surface as **standard git conflict markers**.
Files meant to be generated once (local config) are excluded from the merge entirely.

This is better than a `.old` file for one reason: a backup makes the user re-merge by hand,
while conflict markers put the resolution where they already know how to do it — and it is
the same mechanism §9 already relies on for branches.

**It has a concrete consequence for our manifest.** A three-way merge needs the *old rendered
text*, not just its hash. scc embeds its templates, so it can re-render the old version — but
only if it knows **which version generated each file**. So `.claude/scc-manifest.json` must
record a template/scc version per entry, not only a content hash. Hashes answer "did the user
edit this?"; versions answer "what did it look like before?" We need both.

### 5.7 Task granularity: the rule is "independently verifiable"

Decomposition measurably helps: subtask success rates run substantially higher than
whole-task success rates, and agents that execute individual steps correctly still fail
end-to-end workflows. Runtime-structured decomposition cut **retry cost by 73.2%** by
retrying only the failed subtask instead of the whole plan. The design principle the
literature keeps landing on: subtasks should be **independently verifiable**.

We have no granularity guidance at all in `tasks.md`. "Independently verifiable" is the rule
to adopt, and it is not arbitrary — it is exactly what makes §7's per-task loop (scoped tests
+ lint) possible, and what makes a failure cost one task instead of a feature.

### 5.8 Lint fatigue is the biggest threat to a validator catalogue

This is the finding most likely to decide whether scc gets used. **Non-actionable warnings
run 35–91% of total warnings** in studied static-analysis deployments. False positives are
the single most common reason developers suppress warnings (34.4% of suppressions). Alert
fatigue means valid findings get ignored along with the noise, and the two main barriers to
adopting such tools are **false positives and cumbersome configuration**.

We just wrote a catalogue of eight validators (§5). Three rules follow, and they are
non-negotiable if the catalogue is to be worth having:

1. **A false positive costs more than a miss.** One wrong finding teaches the user to
   disbelieve all eight. This is the same trade §5 already made about not parsing code — now
   with numbers behind it.
2. **Zero configuration to get value.** Configuration is the *other* documented adoption
   barrier; a validator that must be tuned before it is useful will not be.
3. **Findings must be few and each must be fixable.** A finding the user cannot act on is
   noise wearing a useful shape.

There is an encouraging corollary: **structural checks are the class with the lowest false
positive rate.** A `SKILL.md` whose `name` doesn't match its directory is not a heuristic
judgment — it is either true or false. Our §5 boundary turns out to protect trust in the
tool, not just its scope.

### 5.9 One claim we cannot support: codewiki as an "interchange format"

csdd's README frames its repo-derived document as "an interchange shape, not a private one —
a document you compile locally and one produced by an external repo-wiki generator are the
same artifact." I could not find a published standard for this. Repo-wiki generators exist
(DeepWiki and its open-source variants produce cited documentation), but no common citation
format is documented that an external generator and scc would both target.

So if we carry that idea into `knowledge-base.md`, it has to be stated honestly: **scc
defines this format**; conformance by external tools is a hope, not an existing fact. The
`codewiki lint` checks remain valuable either way — they verify citations against a checkout,
which is true regardless of who wrote the document.

---

## 6 · Second-pass changes

| # | Finding | Resolution |
|---|---|---|
| 5.1 | Test-last is only supported *iteratively*; agents fail it by encoding the bug | **§3 Unit tightened**: immediately per function, and the test derives from the requirement, never read off the implementation |
| 5.2 | Over-specification can reduce correctness | **§10** — "omit, don't fill" extended from design to requirements |
| 5.3 | Autonomy needs escalation, not a single up-front switch | **§2 gained escalation triggers**, reusing §3's risk list — one classifier, two consumers |
| 5.4 | Established ADR formats exist | Recorded for `knowledge-base.md`: adopt MADR's shape, keep csdd's triple gate as the *trigger*, preserve Nygard's same-section consequences |
| 5.5 | LLMs are strong on lexical consistency, weak on domain knowledge | Confirms the glossary and locates its value in the lint |
| 5.6 | Three-way merge beats backup files | **New §12** — and the manifest must record a version per file, not just a hash |
| 5.7 | Independently-verifiable subtasks; 73.2% cheaper retries | **§4 gained a granularity rule** |
| 5.8 | 35–91% of warnings are non-actionable; false positives drive suppression | **§5 gained validator discipline**: a false positive costs more than a miss, zero config, few and fixable findings |
| 5.9 | No interchange standard for repo-derived docs exists | Flagged for `knowledge-base.md` — we define the format and must say so |

## Sources

Standards and primary specs:
- [Agent Skills specification](https://agentskills.io/specification)
- [Agent Skills open standard — adoption across 32 tools](https://www.paperclipped.de/en/blog/agent-skills-open-standard-interoperability/)
- [EARS — official guide, Alistair Mavin](https://alistairmavin.com/ears/)
- [Easy Approach to Requirements Syntax (Wikipedia)](https://en.wikipedia.org/wiki/Easy_Approach_to_Requirements_Syntax)
- [Adopting the EARS notation — Jama Software](https://www.jamasoftware.com/requirements-management-guide/writing-requirements/adopting-the-ears-notation-to-improve-requirements-engineering/)
- [github/spec-kit](https://github.com/github/spec-kit) · [spec-driven.md](https://github.com/github/spec-kit/blob/main/spec-driven.md)
- [OpenSpec concepts](https://github.com/Fission-AI/OpenSpec/blob/main/docs/concepts.md) · [OpenSpec comparison](https://openspec.pro/comparison/)

Papers:
- [TDAD: Test-Driven Agentic Development — regression via graph-based impact analysis](https://arxiv.org/pdf/2603.17973)
- [The Spec Growth Engine: Spec-Anchored, Code-Coupled, Drift-Enforced Architecture](https://arxiv.org/pdf/2606.27045)
- [From Prompt to Process: a Process Taxonomy of Frameworks Supporting AI Software Development Agents](https://arxiv.org/pdf/2606.04967)
- [SkillJuror: Measuring How Agent Skill Organization Changes Runtime Behavior](https://arxiv.org/pdf/2606.11543)
- [AGENTS.md efficiency measurement](https://arxiv.org/html/2601.20404v2)
- [Context Rot: How Increasing Input Tokens Impacts LLM Performance — Chroma](https://www.trychroma.com/research/context-rot)

Vendor guidance and practice:
- [How and when to use subagents in Claude Code — Anthropic](https://claude.com/blog/subagents-in-claude-code)
- [Steering Claude Code: CLAUDE.md, skills, hooks, subagents — Anthropic](https://claude.com/blog/steering-claude-code-skills-hooks-rules-subagents-and-more)
- [Claude Code best practices](https://code.claude.com/docs/en/best-practices)
- [Claude Code agents in 2026 — what parallel sessions cost](https://www.cloudzero.com/blog/claude-code-agents/)
- [Git worktrees + Claude Code playbook](https://www.developersdigest.tech/blog/git-worktrees-claude-code-parallel-agents-guide)
- [Parallel agentic development with git worktrees](https://www.mindstudio.ai/blog/parallel-agentic-development-git-worktrees)
- [spec-compare — 6 SDD tools, incl. worktree analysis](https://github.com/cameronsjo/spec-compare)
- [Spec-Driven Development in 2026 — tooling survey](https://dev.to/krlz/spec-driven-development-in-2026-what-it-is-the-tooling-and-how-teams-actually-use-it-2fk2)
- [Codex AGENTS.md vs Claude Code CLAUDE.md](https://www.mindstudio.ai/blog/codex-agents-md-vs-claude-code-claude-md-comparison)
