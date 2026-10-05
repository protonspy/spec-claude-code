---
autonomy: auto
ci: wait
branch: feat/view
delivery: in-progress
---

# View — requirements

## Purpose

`scc view` serves the workspace's knowledge base, plans and specs as a local,
read-only web page: the wiki, the ADRs, the codewiki with the source lines it cites,
the glossary, the stack, the note log, every plan and every spec. It is for a person
reading the record end to end — the one reader the addressing commands are not built
for — and it changes nothing on disk.

## R1 · Serving

- **R1.1** When `scc view` runs in a workspace, the viewer shall serve the web interface on the loopback interface and print its URL.
- **R1.2** Where `--port` is given, the viewer shall listen on that port, and on a free port the system picks when it is `0`.
- **R1.3** If the port cannot be bound, then the viewer shall exit 1 with a message naming the address.
- **R1.4** If the root is not an scc workspace, then the viewer shall exit 1 without starting a server.
- **R1.5** When the process receives an interrupt, the viewer shall shut the server down and exit 0.

## R2 · Content

- **R2.1** The viewer shall list every Markdown file under `docs/`, every plan and every spec, grouped as wiki, ADR, codewiki, other docs, plans and specs.
- **R2.2** When a page is opened, the viewer shall render its Markdown body, its frontmatter and a table of contents built from its headings.
- **R2.3** When a plan or a spec's `tasks.md` is opened, the viewer shall show its task progress as done over total.
- **R2.4** The viewer shall render `[[slug]]` wikilinks, `adr:<name>` citations and relative links to Markdown files as links to those pages inside the viewer.
- **R2.5** When a codewiki citation `[<path>:<from>-<to>]()` is followed, the viewer shall show that source file with the cited lines highlighted.
- **R2.6** The viewer shall read the workspace from disk on every request.

## R3 · Safety

- **R3.1** If a requested path resolves outside the workspace, then the viewer shall answer 404 without reading the file.
- **R3.2** If a requested page is not a Markdown file under `docs/`, `plans/` or `specs/`, then the viewer shall answer 404.
- **R3.3** If a requested path has a segment starting with `.`, or a requested source file is larger than 1 MiB or is not text, then the viewer shall answer 404.
- **R3.4** If a request's `Host` header does not name a loopback host, then the viewer shall answer 403.
- **R3.5** If a request uses a method other than GET or HEAD, then the viewer shall answer 405.
- **R3.6** The viewer shall render raw HTML inside Markdown as text and send a Content-Security-Policy limiting scripts and connections to its own origin.

## Out of scope

Editing anything, search, the symbol graph, live reload pushed from the server,
access from another machine, and authentication — loopback-only is the access
control.
