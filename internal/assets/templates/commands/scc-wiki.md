---
description: Build or maintain docs/wiki/ — ingest a source from docs/raw/, answer from what is known, or clear wiki.* findings
argument-hint: [ingest <file> | query <question> | maintain]
---

Use the `wiki` skill.

Request: $ARGUMENTS

If nothing was asked for specifically, look at `docs/raw/` first — anything sitting
there is unprocessed work and is the default job. If `raw/` is empty, run
`scc validate` and clear whatever `wiki.*` findings it reports. If there are none,
say so rather than inventing pages.
