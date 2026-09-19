# The package map

`scc` is one binary and the dependency direction only ever points one way:

```
cmd/scc/main.go         os.Exit(cli.Run(os.Args[1:]))
        |
   internal/cli         the whole command surface
        |
   scaffold · validate · hooks         write / check / gate
        |          \
   assets · manifest   ears · mdscan · artifact · notes   templates, hashes, grammars
        |
   paths · workspace · render · textutil · finding
        |
   plain files on disk: <harness>/ · specs/ · plans/ · docs/ · CLAUDE.md|AGENTS.md
```

This page is about why the seams fall where they do. What each package *does* is in
its own doc comments; what the concepts mean is in the wiki.

## The dispatcher is a switch, on purpose

[internal/cli/cli.go:48-60]()

`Run(args)` switches on `args[0]` and hands off to `run<Resource>` in a file named
for that resource; each handler owns its own `flag.FlagSet`. Nothing is registered
dynamically, so the entire command set is readable in one place, and adding a
subcommand is a case plus a file.

`Run` returns an exit code and never calls `os.Exit` — which is what makes the whole
surface drivable in-process from the tests.

## Two flag-parsing bugs the handlers must not each re-discover

[internal/cli/flags.go:30-95]()

Go's `flag` stops at the first non-flag argument, so a single pass reads everything
after a positional as more positionals. Measured, `scc notes add --tag gotcha "text"
--path internal/x` wrote the flag into the note as prose, recorded no path, and
exited `0`. `parseFlags` alternates between peeling positionals and parsing flags
until nothing is left, and honours a bare `--` for a value spelled like a flag.
Misfiling an argument while reporting success is worse than rejecting it.

The second is the exit code for `--help`, which 52 call sites got wrong at once by
routing `flag.ErrHelp` to `ExitError`. `parseFlags` returns the error and `exitFor`
maps it; `helpWord` turns a lone `help` positional on a leaf command into the same
thing.

## The layout has one vocabulary

[internal/paths/paths.go:44-124]()

Every directory and file name in the on-disk layout lives in `internal/paths`, with
the harness-relative paths as methods on `Harness`. The profile carries capabilities
rather than identities — `PreloadsRules`, `SubagentsInheritRules`, `SettingsSeg` — so
a change needing a `switch h.ID` outside the asset renderers means the profile is
missing a field.

[internal/workspace/workspace.go:94-128]()

`Find` walks up for a *regular file*, `<harness>/scc-manifest.json`, for each harness
in turn. Accepting the directory would resolve the root to `$HOME` for any command
run outside a workspace, because every harness has a global twin there.

## Templates are data-free, artifacts are not

[internal/assets/assets.go:773-800]()

`Render(h, file)` is the only way to get a workspace file's bytes: it expands paths
and synthesizes the per-harness header. A `(version, harness)` pair renders
byte-identically everywhere, and the manifest records both — which is what would let
a future three-way merge reconstruct the old side. Artifact templates take data,
because the user owns the result.

[internal/scaffold/scaffold.go:92-110]()

`Apply` is idempotent, never overwrites without being told to, and writes the
manifest last.

## The grammars live with the reader

[internal/artifact/artifact.go:460-540]()

`Resolve` accepts a path relative to the working directory, and `Within` is the
boundary in front of it — an address reaches `scc` from a file the agent read as
readily as from the user, so the check belongs here rather than in a handler.

[internal/artifact/schedule.go:85-100]()

One schedule implementation is shared by `--next`, `--ready` and `--blocked`. Two
notions of eligibility would be two answers to *what do I work on*.

[internal/ears/ears.go:102-120]() · [internal/mdscan/mdscan.go:1-20]()

`mdscan` is the only Markdown parser, and its `Body` — comments and fences stripped —
is what every validator applies its grammar to. That is also what lets the templates
carry their instructions in HTML comments without tripping the validators they ship
with.

## The record, and the one thing in it that is an input

[internal/manifest/manifest.go:67-90]()

`{path, hash, version}` per managed file, deterministically serialized, plus the
`check` object.

[internal/manifest/manifest.go:423-445]()

`CarryOver` is the reason a re-run of `init` no longer deletes the project's own
commands: `init` and `update` both build the next manifest from scratch.

## The checkers

[internal/validate/all.go:129-142]()

Ten validators, one file each. [internal/validate/checks.go:27-40]() and
[internal/validate/attribution.go:74-90]() are the two that run only when asked —
one goes to the network, the other runs a compiler and a test suite.

[internal/gate/gate.go:72-80]() · [internal/gate/gate.go:323-340]()

`internal/gate` knows the four kinds and their order and nothing about any language.
It is the only process-starter whose binary the *user* chose.

## The integrations, one package each

[internal/mdblock/mdblock.go:93-130]()

The splice is one implementation — find, compare, replace, append, preserve line
endings. *Which* markers is the integration's decision; everything after that choice
lives here.

[internal/jail/toolchain.go:125-215]()

`Needs` works out what the sandbox's private home would take away, and `Compose` puts
every directory before the files inside it — mounts apply in order, so a directory
bind arriving after a file bind underneath it hides the file.

[internal/codegraph/scope.go:47-70]()

A recorded scope is N graphs, not one graph filtered, and the fan-out is `cmd.Dir`
per root: the builders never learn that scoping exists.

[internal/git/git.go:380-395]() · [internal/git/git.go:553-570]() ·
[internal/git/git.go:631-650]()

`git` is read-only in every call, which is what makes running it over every spec in a
workspace safe by construction. `Changed` is the only one that reads content, and it
compares against the *working* tree rather than `HEAD`, because at the end of a turn
the agent has usually written code and not yet committed it.

## Hooks are events, not scripts

[internal/hooks/harness.go:42-60]()

The scripts and settings entries are one line and call back into
`scc hooks run <stage>`, so every decision is Go — tested, and upgraded with the
binary rather than by rewriting a file in somebody's `.git`.

## Testing conventions

[internal/cli/cli_test.go:1-40]()

`capture(t, f)` swaps `os.Stdout`/`os.Stderr` for pipes and drains both concurrently,
so a command that outruns the pipe buffer fails the test instead of deadlocking — the
only safe way to assert on output, since `render` writes to the real files. Compare
resolved paths with `os.SameFile` rather than string equality: `t.TempDir()` can sit
under a symlink and Windows reports 8.3 short names. A test may not assert on the
*spelling* of a `--json` document; `decode` unmarshals it.
