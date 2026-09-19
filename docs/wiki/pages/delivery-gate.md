# The delivery gate

`.claude/rules/delivery.md` asks for a green build, a clean linter, and tests that mean
something. A reviewer can look; under `autonomy: auto` nobody does — so the step
that certifies the work was the one step certified by nobody. `scc check` turns that
claim into a command.

## One command per gate, four gates

The project records one command each for **build, format, lint, test**, in
`<harness>/scc-manifest.json`. `scc` runs them in that order and judges them by
their exit status.

**`scc` still does not know how to build or test anything, and the one-command
contract is how it stays that way.** A Go project's build gate is `go build ./...`
and a Java one's is `mvn -q compile`; `internal/gate` never learns the difference,
which is what keeps the check deterministic — it is a comparison, not a judgment.

The pipeline **stops at the first failure**, because the four presuppose each other:
a lint report and a test failure over a broken build are derived noise, and the
minutes spent producing them are minutes nobody gets back. Build first for that
reason, then cheapest to dearest — a format check is seconds, a linter tens of
seconds, a suite minutes.

## `skipped` is a word, not an absent key

A language with no formatter and no linter is a real answer. A gate left unrecorded
is a decision nobody has made. Collapsing the two would either nag every project
that has no linter or go quiet about every project that forgot one — so a project
declines a gate once, deliberately, with `scc check skip <gate>`, and the validator
is silent about it forever. `scc check clear` puts it back to undecided, which is a
different act with a different consequence.

`scc init` scaffolds the `check` object with all four keys **empty**, so the first
person to open the manifest sees the four questions this project has to answer
rather than a file that says nothing about them.

Empty costs nothing in meaning — `""` and absent both read as *nobody has decided*,
and the state that needed telling apart is the word `skipped`. The floor is
deliberately left out: it has a working default, and writing it into the base would
pin every workspace to today's number while looking configured.

The four keys are **grouped** into one object rather than sitting flat beside the
file hashes, and that was measured rather than chosen: this file sorts its keys, and
flat, `build` landed on line 2 with its three siblings on line 170 — on the far side
of every content hash in the workspace, which is precisely not the glance the shape
exists for. The flat keys an earlier version wrote are still read and migrated on the
next write.

Within a gate the findings stay distinct — `check.not-configured`, `check.failed`,
`check.timed-out`, `tests.no-report`, `tests.none`, `tests.below-floor` — because they
are different things to go and do. **`scc` cannot derive the commands**: they depend
on a toolchain it never sees, so `check.not-configured` names both ways out, the
command that records a gate and the one that declines it.

## The test gate asks for one thing more

It prints a JSON object on stdout:

```
{"total": 412, "coverage": 88.4}
```

**`total` is there because coverage alone is a claim that can be true of a suite
that does not exist.** Zero tests at 100% is the shape of a repository whose tests
were deleted, and a floor that passed it would certify the one project with nothing
to certify. The coverage floor defaults to 86%.

Three properties of a wrapped test command each have a failure mode that looks like
success: the **exit status has to survive** the wrapper (a script ending in `echo`
returns the echo's `0` and reports a broken build as green), the report goes to
**stdout** with the runner's noise on stderr, and **`total` is a real count** rather
than a constant.

The pipeline belongs in the repository — a `make` target, an npm script, a small
program — not inline in the manifest, because a pipeline recorded inline is one
nobody can run by hand, fix, or review in a diff.

## Why it is off unless asked for

The other ten validators read files already on disk and cost milliseconds; this one
runs a compiler, a linter and a test suite. `scc validate` sits on the pre-commit
path. So `scc validate --checks` and **pre-push** run it — see [[hooks]] — which is
where a branch becomes a pull request and where the claim is actually made. The
explicit flag treats a missing command as a finding; the push hook cannot, or every
workspace that never wired anything up would fail every push over configuration
nobody chose.

## The commands are arbitrary code

A hostile manifest is a hostile command, and that is stated rather than glossed.
Three things bound it: nothing runs them on a bare `scc validate`, each is printed
before it runs, and the hook that runs them is one the user installed in their own
checkout. A project you would not build from a fresh clone is not one to record
commands in.

## Windows builds its own command line

`internal/gate` sets `SysProcAttr.CmdLine` rather than passing an argv to `cmd /c`,
because `os/exec` quotes arguments for a C runtime `cmd.exe` does not use —
measured, a recorded `-run "TestX"` arrives as `-run \"TestX\"`, runs a test nobody
named, and reports success. The practical consequence for whoever records the
commands: one string has to work under both `sh -c` and `cmd /c`, so shell operators
and POSIX-only syntax are out.

## Related

[[validation-contract]] · [[hooks]] · [[integration-boundary]]
