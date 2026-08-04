# Code search — ask the graph before you read the files

This workspace keeps a symbol graph of its own code, in `.codegraph/`, rebuilt
whenever `scc launch` starts an agent. It exists so a structural question costs one
call instead of a grep and six reads.

Reach for it **first**, when the question is about structure:

| The question | Ask |
|---|---|
| Where does this behavior live, and what calls what? | `codegraph_explore`, or `scc graph explore "<question>"` |
| What breaks if I change this symbol? | `scc graph impact <symbol>` |
| Who calls this / what does it call? | `scc graph query <name>`, `callers`, `callees` |

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

That matters more here than it would elsewhere, because this project deliberately
keeps the *why* out of the code. A question the graph answers well — "where is this
implemented" — is a different question from the one the knowledge base answers —
"why is it like this, and what was ruled out". Asking the graph the second kind gets
you a confident answer about the wrong thing. See [knowledge-base.md](knowledge-base.md)
for where that half lives.

## When it is not there

A missing or stale graph is never a reason to stop. `scc launch` builds it on a best
effort and starts the agent either way, so a session may legitimately have none —
CodeGraph is not installed, the index failed, or someone passed `--no-graph`.

Fall back to ordinary reading and say nothing about it. If a graph query returns
something that contradicts the file in front of you, the file wins and the index is
stale: `scc graph sync`.
