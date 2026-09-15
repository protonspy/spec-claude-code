# Code search — ask the graph before you read the files

This workspace keeps a symbol graph of its own code, in `.codegraph/`, rebuilt
whenever `scc launch` starts an agent. It exists so a structural question costs one
call instead of a grep and six reads.

**Grep and the graph answer different questions, and each is bad at the other's.**
Sort by what you are actually asking:

| You are asking | Use | Because |
|---|---|---|
| Where is this exact text — a string, a flag, an error message, an import? | grep | The graph indexes symbols, not text. Grep is exact, instant, and never stale. |
| Which symbol is called `<name>`, and where? | `scc graph query <name>` | An exact-name lookup over symbols rather than text; `--kind function` narrows it, `--limit` caps it. |
| Who calls this? What breaks if I change it? | `scc graph explore "what calls <name>"` | Grep finds the name; only the graph knows which occurrences are calls, and explore answers with the call paths between them. |
| Where does this *concept* live? | `scc graph explore "<question>"` | The concept has no single spelling to grep for. |

The tell is whether your question names a **string** or a **relationship**. A string
is grep's, and reaching for the graph there is the long way round. A relationship is
the graph's, and grepping for it is how a session ends up reading fifteen files to
answer what one call would have.

Read files directly when the question is about *this exact text* — a line you are
editing, a diff you are reviewing, a file you have just written. The graph is a map;
it is not the territory, and it does not replace reading the code you are about to
change.

Two ways in, and they answer identically. Use the `codegraph_explore` tool where it
is registered. Use `scc graph explore` in a shell when it is not — from a subagent,
or from a harness with no MCP surface.

## What the graph does not know

**It indexes code, not this repository's knowledge.** `docs/` is Markdown and no part
of it is in the graph: not the glossary, not the wiki, not an ADR, not a `design.md`.
Plans and specs are not in it either, and they have their own index — see
[artifacts.md](artifacts.md), which is the same rule for the other corpus.

That matters here because this project keeps the *why* out of the code. "Where is
this implemented" is the graph's question; "why is it like this, and what was ruled
out" is the knowledge base's, and asking the graph the second kind gets a confident
answer about the wrong thing — see [knowledge-base.md](knowledge-base.md).

## When it is not there

A missing graph is never a reason to stop: `scc launch` builds it on a best effort and
starts the agent either way, so a session may legitimately have none. Fall back to
ordinary reading and say nothing about it. If a query contradicts the file in front of
you, the file wins and the index is stale.

**Staleness is the one failure worth a reflex**: a stale graph answers confidently
about code that changed, which is worse than no graph. scc syncs at the top of a
session and before a push, so you start and finish current — in between, run
`scc graph sync` after writing code you are then going to search.
