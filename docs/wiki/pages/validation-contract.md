# The validation contract

`scc` is a lint tool for a methodology. Ten validators read files already on disk;
two more run only when asked. What makes the whole thing usable by an agent or a CI
job is a contract narrow enough to branch on.

## Exit codes

```
0  ok
1  usage or runtime error — the tool could not run
2  validation findings
```

**A finding is a legitimate answer to a lint question, not a failure of the tool.**
Collapsing `2` into `1` destroys the distinction a caller needs most: *the check ran
and found something* against *the check did not run*.

`--help` is `0`, and it took 52 call sites to be wrong about that at once — every
handler routed `flag.ErrHelp` to the error exit, so `scc map tasks --help` reported
the same code as a command that could not run. `scc launch` is the single deliberate
exception to the whole contract: it returns whatever the agent it started returned,
because a launcher that flattened its child's exit status would be unusable in the
scripts people actually write.

A related trap sits one layer down. Go's `flag` package stops at the first non-flag
argument, so a single pass reads everything after a positional as more positionals —
measured, `scc notes add --tag gotcha "text" --path internal/x` wrote the flag into
the note as prose, recorded no path, and exited `0`. `parseFlags` alternates between
peeling positionals and parsing flags until nothing is left. **Misfiling an argument
while reporting success is worse than rejecting it.**

## A validator that fires on `scc`'s own output is the worst bug in the product

One wrong finding teaches the user to disbelieve all ten. The templates therefore
carry their instructions in HTML comments and fenced examples, which is exactly what
`mdscan` excludes, and `TestFreshArtifactsPassTheirOwnValidators` is the gate on it.
Treat that test as required reading before changing a template or a validator.

## Two validators whose subject is not a file

Eight validators read Markdown. Two do not, and both exist because a rule that has
to be *remembered* survives only until the session that skips it.

- **`attribution`** reads the commits this branch added — `base..HEAD` — for an
  assistant's signature. **A signature is a shape, never a word**: a trailer, a
  "generated with" line, a vendor link, a Markdown badge, or a bare name on a line
  of its own. The vocabulary (`claude`, `anthropic`, `codex`, and so on) never
  decides on its own that a line *is* a signature — only whether a signature names
  an assistant — so a commit bumping a vendor's SDK stays an ordinary commit. It
  **never returns an error**: a project with no git, an unborn HEAD, or a shallow CI
  clone has nothing on this branch to check, which is a clean pass rather than a
  broken `scc validate` in every workspace that is not a repository.
- **`tests`** runs the project's own recorded commands. See [[delivery-gate]].

## Cost decides what runs by default

The ten file validators cost milliseconds. The two that do not are opt-in for
reasons of cost rather than doctrine, and the reasoning is identical in both cases:
`scc validate` sits on the pre-commit path, where **a gate that costs a second per
commit is a gate somebody turns off, taking the other ten with it**.

- `--pr` enables the pull-request check, the one check that goes to the network.
- `--checks` enables the delivery gate, which runs a compiler, a linter and a suite.

`pre-push` enables both, because that is where a branch becomes a pull request and
where the methodology says the claim is actually made. See [[hooks]].

## Findings have one shape

`internal/finding` owns one type and one frozen JSON document for every validator,
plus the grouped human report. Machine output is compact and human output is
indented: measured on a six-task plan, `map tasks --json` was 2198 bytes indented
against 599 for the human listing of the same tasks, about a third of it leading
whitespace. The corollary is a testing rule — **a test may not assert on the
spelling of a document**; it unmarshals it, because a substring check against
formatted JSON is an assertion about the reader rather than about the command. Four
such tests broke on that change while the commands were correct.

## Hostile input

Any positional name that becomes a path segment passes `workspace.SafeName` *before*
`filepath.Join` — without it, `scc spec delete .. --force` resolves to the workspace
root. Resource names are kebab-case. The same boundary applies to artifact
addresses, for a reason worth stating: an address reaches `scc` from a file the
agent read as readily as from the user. See [[artifact-addressing]].

## Related

[[delivery-gate]] · [[artifact-addressing]] · [[scoped-rules]] · [[hooks]]
