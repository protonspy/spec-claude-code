# View — tasks

## 1 · Server

- [x] 1.1 (Unit) Build the tree of docs, plans and specs into groups for `/api/tree` — R2.1, R2.3, R2.6
- [x] 1.2 (Unit) Serve `/api/page` through the page-path guard and `artifact.Load` — R2.2, R2.3, R2.6, R3.1, R3.2
- [x] 1.3 (Unit) Serve `/api/source` through the source-path guard — R2.5, R3.1, R3.3
- [x] 1.4 (Unit) Wrap every route in the host, method and header middleware, and serve the embedded app with an `index.html` fallback — R3.4, R3.5, R3.6
- [x] 1.5 (Unit) Add `scc view` with `--root` and `--port`, the workspace guard, loopback listen and interrupt shutdown — R1.1, R1.2, R1.3, R1.4, R1.5
  _Depends 1.1, 1.2, 1.3, 1.4_

## 2 · Frontend

- [x] 2.1 (Unit) Scaffold `web/` with Vite, React and TypeScript, and write the link-rewriting and slug module with its tests — R2.4, R2.5
- [x] 2.2 (Unit) Build the sidebar, page view with frontmatter, table of contents and task progress, and the source view, into `internal/view/dist/` — R2.1, R2.2, R2.3, R2.4, R2.5, R3.6
  _Depends 1.5, 2.1_

## 3 · Record

- [x] 3.1 (Unit) Write `adr:0003-local-read-only-viewer`, the `stack.md` entries, the CI job that rebuilds `dist/` and fails on a diff, and the help text — R1.1, R3.6
  _Depends 2.2_
