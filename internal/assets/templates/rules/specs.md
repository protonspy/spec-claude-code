# Specs — the three artifacts

`specs/<feature>/` holds exactly three files. `scc spec new <feature>` creates them
from the templates; this is what goes in them.

## requirements.md — EARS, all five patterns

Requirements are written in EARS and numbered `R<group>.<item>`, so the later phases
can cite them. This is not ceremony: prose has nothing to validate, an EARS clause
has named parts, and a missing part is a finding.

The ruleset: **zero or many preconditions · zero or one trigger · one system name ·
one or many responses**, always in that order.

| Pattern | Shape |
|---|---|
| Ubiquitous | `The <system> shall <response>` |
| State-driven | `While <precondition>, the <system> shall <response>` |
| Event-driven | `When <trigger>, the <system> shall <response>` |
| Optional feature | `Where <feature>, the <system> shall <response>` |
| Unwanted behavior | `If <trigger>, then the <system> shall <response>` |
| Complex | more than one keyword, in clause order |

**All five are valid.** Do not force everything into `When …`: inventing a trigger
for a requirement that is simply always true makes it worse. `If … then …` —
unwanted behavior — is the pattern most often missing and the one that most often
matters.

**Omit, don't fill.** Structured requirements measurably improve generated code, but
the curve is not monotonic: over-specification constrains reasoning and introduces
requirements that conflict with each other, and correctness drops. Specify what the
feature actually decides. A requirements document longer than the decision it records
can produce worse code than a shorter one.

## design.md — scaled by complexity

**The design must fit the decision being made.** The common failure is inventing
architecture for a change that had no architectural question in it — a component
diagram for a two-function addition, a data-model section for something that touches
no data.

That is worse than verbose. **Invented architecture constrains:** the next session
reads it as a decision somebody made, and honors it. Filler becomes binding.

So the sections are conditional. **Omit, don't fill:**

| When the change… | design.md carries |
|---|---|
| decides nothing structural | what changes, where, and why — a few paragraphs. No components, no diagram. |
| moves a boundary, a data shape, or an external contract | those sections only, for the parts that actually change |
| has real alternatives with trade-offs | the alternatives and why one won — plus an ADR if the decision is hard to reverse |

A heading filled with "N/A", or with prose written to satisfy the heading, is worse
than an absent heading: it reads as a decision and nobody can tell it apart from
one. Delete the heading instead.

`scc` checks that the design exists and traces to its requirements. It never checks
that a particular section is present — a required heading is a request for filler.

## tasks.md — the grammar

See [tasks.md](tasks.md). Exactly one `(Unit)` or `(TDD)` per task, and every task
cites the requirements it satisfies. Traceability runs both ways: every requirement
reaches at least one task, every task cites a requirement that exists.

## Changing a spec that already exists — deltas, not rewrites

Write the change as a **delta** against the spec, marking each affected requirement:

```
- **R2.3** (MODIFIED) When the cart is empty, the checkout shall …
- **R2.7** (ADDED) If the coupon has expired, then the checkout shall …
- **R1.4** (REMOVED)
```

Three reasons:

1. **You specify the change, not the system.** Adopting this practice on an existing
   codebase must not require writing the spec for everything that already works.
   Specs grow one change at a time.
2. **Deltas scoped to individual requirements make concurrent edits safe.** Two
   sessions can change one spec as long as they touch different requirements.
   Whole-file rewrites collide on contact.
3. **A reviewer reads intent instead of reconstructing it** from a diff.

The delta is how a change is proposed and reviewed. Once it lands, fold it into the
spec: the spec stays the current statement of the feature, never an append-only log.

## The spec is anchored, not disposable

A spec does not stop being true when its feature merges. **Work that touches an area
a spec covers updates that spec as part of the delivery** — as a delta, in the same
branch, in the same PR as the code.

Under autonomy the file is the only record of intent, and a record that stops being
maintained stops being a record. A stale requirement read as current is worse than an
absent one, because it is believed.

Nothing detects this drift mechanically — verifying that a spec matches the code
means understanding the code, which `scc` deliberately does not do. Keeping the spec
current is your obligation, and the reviewer's to check.

Anchored means *maintained while the code exists*, not *never removed*. A feature
genuinely deleted takes its spec with it.
