# Specs — the three artifacts

`specs/<feature>/` holds exactly three files. `scc spec new <feature>` creates them;
this is what goes in them.

## requirements.md — EARS, all five patterns

Numbered `R<group>.<item>` so later phases can cite them. Prose has nothing to
validate; an EARS clause has named parts, and a missing part is a finding.

The ruleset: **zero or many preconditions · zero or one trigger · one system name ·
one or many responses**, in that order.

| Pattern | Shape |
|---|---|
| Ubiquitous | `The <system> shall <response>` |
| State-driven | `While <precondition>, the <system> shall <response>` |
| Event-driven | `When <trigger>, the <system> shall <response>` |
| Optional feature | `Where <feature>, the <system> shall <response>` |
| Unwanted behavior | `If <trigger>, then the <system> shall <response>` |
| Complex | more than one keyword, in clause order |

**All five are valid.** Inventing a trigger for something simply always true makes it
worse. `If … then …` is the pattern most often missing and most often the one that
matters.

**Omit, don't fill.** Structure improves generated code, but the curve is not
monotonic: over-specification constrains reasoning and introduces requirements that
conflict, and correctness drops. A requirements document longer than the decision it
records can produce worse code than a shorter one.

## design.md — scaled by complexity

**The design must fit the decision being made.** Inventing architecture for a change
that had no architectural question is worse than verbose — **invented architecture
constrains**: the next session reads it as a decision somebody made, and honors it.

So the sections are conditional. Omit, don't fill:

| When the change… | design.md carries |
|---|---|
| decides nothing structural | what changes, where, and why. No components, no diagram. |
| moves a boundary, a data shape, or an external contract | those sections only, for the parts that change |
| has real alternatives with trade-offs | the alternatives and why one won — plus an ADR if it is hard to reverse |

A heading filled with "N/A", or with prose written to satisfy the heading, reads as a
decision nobody can tell apart from a real one. Delete the heading instead. `scc`
never checks that a section is present: a required heading is a request for filler.

## tasks.md

See [tasks.md](tasks.md). Traceability runs both ways: every requirement reaches a
task, every task cites a requirement that exists.

## Changing an existing spec — deltas, not rewrites

```
- **R2.3** (MODIFIED) When the cart is empty, the checkout shall …
- **R2.7** (ADDED) If the coupon has expired, then the checkout shall …
- **R1.4** (REMOVED)
```

You specify the change, not the system, so adopting this on an existing codebase
never means writing the spec for everything that already works. Deltas scoped to
individual requirements also make concurrent edits safe — two sessions can change one
spec while touching different requirements, where whole-file rewrites collide on
contact — and a reviewer reads intent instead of reconstructing it from a diff.

Once the delta lands, fold it in: the spec is the current statement of the feature,
never an append-only log.

## The spec is anchored, not disposable

**Work touching an area a spec covers updates that spec as part of the delivery** —
as a delta, same branch, same PR. Under autonomy the file is the only record of
intent, and a stale requirement read as current is worse than an absent one because
it is believed.

Nothing detects this drift mechanically: verifying a spec against code means
understanding the code, which `scc` deliberately does not do. It is your obligation
and the reviewer's to check. Anchored means *maintained while the code exists* — a
feature genuinely deleted takes its spec with it.
