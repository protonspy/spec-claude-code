---
status: accepted
---

# A local, read-only viewer, built with React

## Context

`adr:0002-narrowed-from-csdd` decided against an embedded web dashboard, among
other surfaces built for a person rather than for an agent, and said that decision
is reopened by a new record rather than by a pull request. This is that record.

The addressing commands — `scc map`, `scc notes find`, `scc graph` — exist so an
agent never reads a whole file. A person reading the record end to end is the one
reader they were never built for: the wiki, the ADRs, the codewiki next to the
lines it cites, every plan and spec with its progress. The user asked for that as
`scc view`, a page served by the binary.

Two questions follow. Whether a web surface may exist at all, given 0002; and how a
React frontend ships inside a binary whose module is stdlib-only
(`adr:0001-stdlib-only-dependencies`).

## Decision

**`scc view` exists, and is held to three limits** that keep it inside what 0002
was protecting:

- **Read-only.** GET and HEAD only; nothing on disk changes. Every write stays a
  command with an exit code, so the agent-facing surface is still the only one that
  does anything.
- **Loopback-only.** The listener binds `127.0.0.1`, and a `Host` header that does
  not name loopback is refused, which closes DNS rebinding. There is no
  authentication because there is no remote access to authenticate.
- **The same parsers.** Every route reads through `internal/artifact`, so the page
  shows what `scc map` would say and is not a second reading of the files.

**The frontend is React, built by Vite from `web/` into `internal/view/dist/`,
which is committed and embedded with `go:embed`.** The Go module stays
stdlib-only: `web/` is build machinery outside the module, as `npm/` already is,
and a `go build` needs no node. CI rebuilds `dist/` and fails on a diff.

Rejected: React vendored without a build step (no JSX, no TypeScript, hand-managed
updates), and React loaded from a CDN (needs network, and puts a third-party script
in a page that can read the workspace). Rendering Markdown in Go was rejected
because the stdlib has no renderer and `mdscan` is a scanner.

## Consequences

- The binary grows by the bundle — about 400 KB, 120 KB gzipped.
- A change to `web/` needs node ≥ 22.12 and a committed rebuild; a change to Go
  alone does not. The CI job is what makes a forgotten rebuild loud.
- The frontend's packages are npm dependencies with their own supply chain. They
  run in the developer's build and in the browser, never in the Go binary's
  process, and `package-lock.json` pins them; `docs/stack.md` lists each.
- Markdown from the repository is untrusted input to the page: raw HTML is shown as
  text, and a Content-Security-Policy keeps scripts and connections on the
  viewer's own origin, so a hostile page in a cloned repository cannot read
  through the source route and send it anywhere.
- The source route can read any text file in the workspace up to 1 MiB whose real
  path has no dot-named segment, not only the files a codewiki page cites. The dot
  rule is applied after symlinks are followed and Windows short names expanded, so
  neither a committed link to `.env` nor `GIT~1` reaches past it.
- There is no authentication: any account on the machine can reach the loopback
  port. Accepted, because the readable set is the checkout and the record, not
  credentials. If a shared host ever makes that unacceptable, the fix is a per-run
  token in the URL, not a wider bind.
