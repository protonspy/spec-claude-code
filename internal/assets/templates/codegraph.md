<!-- scc:codegraph-instructions v1 -->
## CodeGraph
Ask the symbol graph before reading files. "Who calls this", "what breaks if I change it",
"where does this concept live" are one command here and a dozen reads otherwise.

- `scc graph explore "<question>"` — the relevant symbols' source plus the call paths between them. Start here.
- `scc graph query <name> [--kind function|class] [--limit N]` — find a symbol by name.
- `scc graph status` — what the graph holds. `--check` exits 2 when there is none.
- `scc graph sync` — re-index after you have written code you then need to search.
- `scc graph build [--force]` — first index, or a full rebuild when the graph has gone wrong.

`scc launch` indexes before the session starts, so the graph is current at turn one.
It goes stale as you edit: sync before searching for something you just wrote.
The graph is CodeGraph's — never edit `.codegraph/`, and never commit it.
<!-- /scc:codegraph-instructions -->
