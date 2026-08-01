---
description: Record a hard-to-reverse decision as a numbered ADR under docs/adr/, or supersede one that stopped being true
argument-hint: [the decision, or the ADR being superseded]
---

Use the `adr` skill.

Decision: $ARGUMENTS

First ask whether this is an ADR at all: how expensive would it be to undo? A
decision that is cheap to change belongs in the spec's `design.md`, and an `adr/`
full of reversible choices buries the records that actually explain the system.

If an existing record is being replaced, write the new one and mark the old one
superseded — never edit its prose.
