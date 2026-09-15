# Glossary

One canonical term per concept, and the synonyms nobody should use for it. Every
name listed after `Avoid:` is reported wherever it appears as a whole word under
`docs/`, so this file lists one only where two names were actually observed.

Reconstructed on 2026-09-15 from `CLAUDE.md`, `.claude/rules/`, `design/`, and the
`internal/paths`, `internal/artifact` and `internal/manifest` packages. The
vocabulary turned out to be near-uniform across all four, which is why there is
exactly one `Avoid:` entry below.

## The layout

- **workspace** — a directory holding `<harness>/scc-manifest.json`. That file is the
  marker, never the harness directory, because every harness has a global twin in
  the user's home. Avoid: project root
- **harness** — the agent tool a workspace is scaffolded for: Claude Code, Codex, or
  opencode. Modelled as a `paths.Harness` profile that names where that tool keeps
  things and what it does with them.
- **entry file** — the Markdown file a harness preloads at launch: `CLAUDE.md` for
  Claude Code, `AGENTS.md` for Codex and opencode. Owned by the user; scc only
  splices marker-delimited blocks into it.
- **artifact** — one file scc parses and can address: a spec's three documents, a
  plan, or a page under `docs/`. Not every file in a workspace is one.
- **seed** — a `docs/` anchor `scc init` writes once and then tracks nowhere: not in
  the manifest, never touched by `scc update`. Distinct from a template, which is
  tracked, and from an artifact, which the user creates.
- **manifest** — `<harness>/scc-manifest.json`: one `{path, hash, version}` per
  managed file, plus the delivery gate's four commands. scc's only file per harness,
  and the reason it needs no configuration file.

## The vehicles

- **spec** — `specs/<feature>/`, exactly three files: `requirements.md`,
  `design.md`, `tasks.md`. The vehicle for work whose *what* and *how* need settling
  before code.
- **plan** — `plans/<name>.md`, one file: a header plus a checklist. The vehicle for
  everything else. Note this is the opposite of GitHub Spec Kit's sense of the word,
  where "plan" means the architecture inside one feature — which here is `design.md`.
- **delta** — a change to an existing spec written as amendments to individual
  requirements, marked `(ADDED)`, `(MODIFIED)` or `(REMOVED)`, rather than a rewrite
  of the file. What makes adopting the methodology on existing code possible.
- **leaf** — a plan item that references a spec rather than being a task itself.
  Recognised by a `specs/<feature>/` citation anywhere in the file, and carrying no
  checkbox: its state is derived from that spec and never copied.

## The grammar

- **EARS** — Easy Approach to Requirements Syntax: the five requirement patterns
  (ubiquitous, state-driven, event-driven, optional-feature, unwanted-behavior) plus
  complex. A clause has named parts, so a missing part is mechanically detectable.
- **requirement id** — `R<group>.<item>`, and it is **scoped to its own spec**:
  `R2.5` is defined in many specs at once, so it is cited as
  `specs/<feature>/R2.5` wherever that is not obvious from context.
- **task** — one checklist line, `- [ ] <group>.<item> (Unit|TDD) <description>`,
  where the checkbox is the only record of its state. Right-sized when it can be
  verified on its own.
- **methodology annotation** — the required `(Unit)` or `(TDD)` on a task line,
  saying how that task is built and tested. Per task, never per project.
- **flag** — one of exactly four italic one-liners under a task: `_Depends_`,
  `_Priority_`, `_Status removed_`, `_Reason_`. The vocabulary is closed, so an
  unrecognised italic line is a finding rather than prose.
- **address** — how `scc map` and `scc patch` name a piece of an artifact without
  reading it: `1.2` a task, `R1.2` a requirement, `#notes` a section, `notes:7` a
  paragraph, `specs/foo/` a leaf. `L120-160` is the escape hatch and the only form
  that is a line number.
- **seal** — the `status: approved` plus `checksum:` pair `scc plan approve` writes
  over a plan. Tamper-**evidence**, not prevention: `reseal --force` is one command
  away and sha256 is public.

## Checking and delivery

- **validator** — one of the ten checks behind `scc validate`, each owning one rule
  and one family of findings. Two more run only when asked: the PR check under
  `--pr` and the delivery gate under `--checks`.
- **finding** — a legitimate answer to a lint question, reported as
  `{rule, message, path, line}` and carrying exit code `2`. Deliberately not an
  error: exit `1` means the tool could not run.
- **gate** — one of the four commands a project records for `scc check` — build,
  format, lint, test — run in that order and judged by exit status. `skipped` is a
  recorded decision; unrecorded is a decision nobody has made.
- **rule** — a Markdown file under `<harness>/rules/` stating one piece of the
  methodology. Unscoped rules are preloaded into every request on a harness that
  reads them, which is why they are a standing budget rather than documentation.
- **scoped rule** — a rule carrying a `paths:` header, so the harness loads it only
  when the agent touches the tree it governs. Permitted only where a validator
  reports what the rule prevents.
- **block** — a marker-delimited region scc keeps current inside a file somebody
  else owns, spliced by `internal/mdblock`. Whose markers is the integration's
  decision: RTK's own, so `rtk init` and `scc rtk` converge on one copy; scc's own
  for CodeGraph, which writes none of its own.
- **note** — one line in `docs/notes.md`, index fields first, replacing the gotcha
  comment that used to sit beside the code. One note is one line, which is what
  makes a grep match a whole note.
- **kickoff answers** — the three questions asked once before the first artifact —
  `autonomy`, `ci`, `lang` — recorded in that artifact's frontmatter so a second
  session never re-asks.
