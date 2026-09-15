---
description: Narrate an area of the codebase into docs/codewiki/, every section citing the exact lines it explains
argument-hint: [area or path to narrate | repair]
---

Use the `codewiki` skill.

Area: $ARGUMENTS

If no area was named, run `scc validate` and repair the `codewiki.*` findings it
reports — a broken citation means the code moved and the prose describing it is now
suspect, so re-read before re-numbering. If there are no findings and no area was
named, ask which area is hard to enter cold rather than picking one at random.
