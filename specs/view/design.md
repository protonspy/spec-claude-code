# View — design

## What changes

Serves R1.1, R1.2, R1.3, R1.4, R1.5, R2.1, R2.2, R2.3, R2.4, R2.5, R2.6, R3.1, R3.2, R3.3, R3.4, R3.5, R3.6.

A new command, `scc view [--root] [--port N]`, and a new package, `internal/view`,
holding the HTTP server and the built frontend it embeds. `internal/cli/view.go` is
the thin handler every other resource has: flags, the workspace guard, then
`view.Serve`.

The server is `net/http` from the standard library; the module stays stdlib-only
(`adr:0001-stdlib-only-dependencies`). Reading goes through the parsers the rest of
scc already uses — `artifact.Load` for a page's frontmatter, sections and tasks,
`artifact.Within` for the containment check — so the viewer cannot disagree with
`scc map` about what a file says.

The frontend is React, built by Vite from `web/` into `internal/view/dist/`, which is
committed and embedded with `go:embed`. A `go build` therefore needs no node; only a
change to `web/` does. Reversing the "no embedded web dashboard" line of
`adr:0002-narrowed-from-csdd` is recorded in `adr:0003-local-read-only-viewer`.

## Boundaries and contracts

Bound to `127.0.0.1` only. Three JSON routes, GET only, everything else is the
embedded single-page app (unknown paths fall back to `index.html`):

| Route | Answers |
|---|---|
| `GET /api/tree` | `{workspace, groups: [{id, title, items: [{path, title, kind, done, total}]}]}` — group ids `wiki`, `adr`, `codewiki`, `docs`, `plans`, `specs` |
| `GET /api/page?path=<rel>` | `{path, kind, title, frontmatter, sections, tasks, done, total, body}` — `body` is the Markdown after the frontmatter block |
| `GET /api/source?path=<rel>` | `{path, lines}` — the frontend highlights the cited range |

Every request passes one middleware first: `Host` must be `localhost`, `127.0.0.1`
or `[::1]` with any port (DNS rebinding, R3.4), the method must be GET or HEAD
(R3.5), and every response carries
`Content-Security-Policy: default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:`
plus `X-Content-Type-Options: nosniff` (R3.6).

Both routes share one check: the path must be workspace-relative, slash-separated
and already clean, carry no `.`-prefixed segment (`.git`, `.env`, the harness
directory — the same rule the tree walk skips by), name a regular file, and pass
`artifact.Within`. A page must also end in `.md` and start with `docs/`, `plans/` or
`specs/`; a source file must be at most 1 MiB with no NUL byte in its first 8 KiB.
A missing asset under the app is a 404; only an extensionless path falls back to
`index.html`. A refusal is 404 either way, so the answer does not say whether the file
exists.

Frontend routing is hash-based — `#/page/<path>`, `#/source/<path>:<from>-<to>` — so
the server needs no route table beyond the three above. Link rewriting lives in one
module, `web/src/links.ts`: `[[slug]]` to `docs/wiki/pages/<slug>.md`, `adr:<name>`
to `docs/adr/<name>.md`, a relative `*.md` link resolved against the page's
directory, and an empty-target link whose text is `<path>:<from>[-<to>]` to the
source view. Heading anchors use the same slug rule as `mdscan.Slug`, so the table
of contents built from the server's `sections` lands on the rendered headings.

Markdown is rendered with `react-markdown` and `remark-gfm`, which escape raw HTML
by default (R3.6) — no `rehype-raw`, no `dangerouslySetInnerHTML`.

## Alternatives considered

- **Vendored React without a build step**, or **React from a CDN** — the first gives
  up JSX and TypeScript, the second needs network and loads a third-party script
  into a page that can read the workspace. The Vite build with a committed `dist/`
  keeps `go build` node-free while keeping the frontend normal React.
- **Rendering Markdown to HTML in Go** — needs a Markdown-to-HTML renderer the
  stdlib does not have; `mdscan` is a scanner, not a renderer, and writing one is
  far more code than the page it serves.

## Risks

- `internal/view/dist/` can drift from `web/`. CI rebuilds it and fails on a diff.
- Serving source widens what the viewer exposes beyond `docs/`. The dot-segment rule
  and loopback-only binding are the boundary; a repository file without a dot in
  its path is readable by anybody who can already reach the user's loopback.
