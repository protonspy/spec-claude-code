# The harness profile

`scc` supports three agent tools — Claude Code, Codex, and opencode — from **one**
set of prose templates. `paths.Harness` is the type that makes that possible: a
profile per tool saying where it keeps things and what it does with them.

## One prose source, a synthesized header

A template is written once, data-free except for the profile. `assets.Render(h, file)`
expands the paths and synthesizes whatever header that tool's loader parses: YAML
frontmatter for Claude Code and opencode, a TOML agent role file for Codex. A
`(version, harness)` pair therefore renders byte-identically on every machine, and
the manifest records both — which is what would let a future three-way merge
reconstruct the old side.

## The profile carries capabilities, not identities

**If a change needs a `switch h.ID` outside the two render functions, the profile is
missing a field.** Adding a harness is a new `paths.Harness` value plus its entry in
`paths.Harnesses()`; `init`'s flags, the picker, `update`'s targets, the skills
validator's search path and the workspace walk all derive from that list.

Four fields are worth knowing by name, because each encodes a fact about the tool
that `scc` is not free to assume:

- **`PreloadsRules`** — whether the tool loads `rules/*.md` at launch. Claude Code
  does; Codex and opencode do not, where `rules/` is just `scc`'s own directory.
  The entry file branches on it, because telling an agent that already holds the
  rules to "read the rule when the concern is live" makes it put the same bytes in
  twice — and teaches it that the one document it is meant to trust is wrong about
  its own environment. See [[context-budget]].
- **`SkillsAreCommands`** — whether registering a skill already gives the user a
  `/name`. On Claude Code it does, so shipping `commands/scc-wiki.md` beside
  `skills/scc-wiki/` was a second picker entry and a second preloaded `description`
  for one thing. The `scc-` prefix moved onto the skill itself, which is also what
  keeps `/scc-init` from colliding with the `/init` Claude Code already ships.
- **`SettingsSeg`** — whether the tool has an in-session hook surface at all. Empty
  for Codex and opencode, which are not missing a file `scc` could write: they have
  no mechanism that would read one. They therefore contribute nothing rather than a
  finding.
- **`SubagentsInheritRules`** — whether a subagent gets the rules one level down.
  Claude Code hands a non-fork subagent the whole hierarchy, which is why there is
  deliberately **no** `SubagentStart` hook: re-injecting would re-send ~26KB the
  agent already holds, once per subagent. `TestNoHarnessNeedsASubagentStage` fails
  the build if a harness ever has a hook surface and does not deliver the rules.

## The marker is a file, never a directory

`workspace.Find` walks up looking for `<harness>/scc-manifest.json` — a regular
file, for each harness in turn. Accepting the *directory* would be catastrophic in
two ways: every harness has a global twin in the user's home (`~/.claude`,
`~/.codex`, `~/.config/opencode`) that exists on any machine running that tool, so
any command run outside a workspace would resolve its root to `$HOME` and start
reading and writing the user's global configuration; and those directories exist in
every repository that merely *uses* the tool, where `scc` was never initialized.

## Related

[[managed-files]] · [[context-budget]] · [[scoped-rules]]
