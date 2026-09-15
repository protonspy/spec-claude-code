# Routing — which vehicle

Work does not walk one fixed path. You pick the vehicle to match it, and the
choice has to be recorded, because the artifact is the only thing anyone else —
the reviewer, the next session, you after a compaction — will ever see.

Ask one question:

> Does this need requirements and a design settled before any code?

**Yes → a spec.** No → **a plan.**

| Vehicle | When | Path | Contents |
|---|---|---|---|
| **Spec** | The *what* and the *how* need settling before code. | `specs/<feature>/` | `requirements.md` · `design.md` · `tasks.md` |
| **Plan** | Everything else. | `plans/<name>.md` | structure, plus a checklist of tasks and/or references to specs |

```
scc spec new <feature>
scc plan new <name>
```

Both write a file. Neither is session-only, and that is not bookkeeping: you write
the requirements and then proceed without anyone approving them, so the file is the
only channel through which the decision survives. A session's context dies.

## A plan covers both ends of "not a spec"

- **An initiative too large for one spec** — decomposed into specs it references.
- **A change small enough that the *what* was never in doubt** — a checklist and
  nothing more.
- Or both at once: a decomposition with a handful of items it just does itself.

The weight of the record matches the weight of the work. A one-line change gets one
checklist item, not three ceremonial files under `specs/`.

## Rules that make a plan checkable

- **A plan is one file**, not a directory. It holds no state beyond its checklist.
- **One source of truth per item.** Where an item *is* a task, its checkbox is the
  state. Where an item *references a spec*, the state is derived from that spec and
  never copied — an item must not do both. Two records of one fact disagree, and
  the copy is the one that goes stale.
- **A plan's referenced specs are ordinary specs.** `plans/checkout-revamp.md` names
  `specs/cart-totals/`; that spec is not nested under the plan and is built by
  exactly the same rules as one a human asked for directly.
- **A plan is work, not knowledge**, so it lives in `plans/` — never in `docs/`,
  which is the knowledge base, and never in `specs/`, which holds features.

Whichever vehicle you picked, every task in it carries a methodology annotation.
See [tasks.md](tasks.md) and [methodology.md](methodology.md).

> **If you know GitHub Spec Kit:** "plan" there means the opposite of this — the
> architecture *inside* one feature, which here is `design.md`. A plan here is the
> decomposition *above* specs, or a bare checklist.
