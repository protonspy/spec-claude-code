# The integration boundary

`scc` drives six third-party binaries and vendors none of them: RTK, CodeGraph,
Headroom, ai-jail, the Dev Containers CLI, and git/`gh`. Each gets **one package**
under `internal/`, reached only from `internal/cli`, and that package owns the
binary's name, its install command, and its argument vocabulary.

The reason is release cadence: all three of those age on the third party's schedule,
and one package per integration is what keeps a version bump from touching the
dispatcher. `internal/gate` is the seventh process-starter and the odd one out — the
commands it runs are the *project's*, not a vendor's. See [[delivery-gate]].

## Flags are discovered, not compiled in

Headroom renamed one control twice — `--no-serena` became `--no-tokensave` became
`--code-memory none` — and its own profiles still disagree: `wrap opencode` takes
`--no-serena` while `wrap claude` and `wrap codex` take `--code-memory`. A flag name
compiled into `scc` turns that kind of release into a launch that dies on "no such
option", which is strictly worse than one unwanted MCP server.

So `internal/headroom` and `internal/jail` both read `<binary> --help` and pick the
spelling that build advertises. **A build advertising no opt-out is reported, never
substituted** — a sandbox opened by a guess is the failure the sandbox exists to
prevent.

## Everything degrades — except the sandbox, which refuses

This is the load-bearing asymmetry of `scc launch`.

Headroom, CodeGraph and RTK are **enhancements**: a missing binary, a declined
install, an unattended run, or a harness Headroom does not wrap all end in the agent
starting anyway, with one line saying why. A launcher that refused to run because a
compression proxy was missing would be putting its own preference above the thing
the user actually asked for.

A **sandbox is the property the user asked for by name.** So a missing binary, a
declined install, an unattended run, or an unsupported platform all end in *nothing
starting* — and the refusal happens before the graph is built or the entry file is
touched, so it leaves the workspace as it found it. Somebody who typed `--jail` and
watched an agent start believes they are contained, and a false belief about
containment is worse than a known absence of it.

**The default splits that.** The sandbox is on by default and **the install is the
opt-in**: ai-jail on PATH means every launch here is contained, absent means one
line naming what would have contained it. A default that refused would break
`scc launch` for everyone who has not installed it, and nobody *typed* a default.
`--no-sandbox` is the deliberate way out, and it contradicts `--jail` rather than
outranking it.

## What a bare launch may leave behind

One question settles what is on by default: **does this outlive the session?**

- **RTK's usage block** does, and it is what the agent needs — an agent that never
  read the block never types the prefix, so installing the binary and leaving the
  file alone buys nothing. On by default, and *bounded by the block*: the step runs
  when an entry file carries none and does nothing when they all do, so it fires
  once per workspace rather than once per session.
- **The symbol graph** does, and launch is the one moment indexing is free.
- **`headroom wrap`** does too — it registers MCP servers into the agent's own
  config, which is why Headroom ships `unwrap` at all. Compression for one session
  is not worth a config edit the user did not ask for and will not see, so it is
  opt-in behind `--headroom`, and even then `--headroom-mcp none` is the default.

Two further details are easy to get wrong. `headroom wrap` also wants to append its
own RTK guidance to the entry file, behind markers that are not a substring of
`scc`'s — so both tools' idempotency checks pass and the file ends up carrying the
same instructions twice. `scc launch` passes `--no-context-tool`. And `wrap` parses
every flag it recognizes out of the tail before forwarding the rest, so a
pass-through argument colliding with one of Headroom's (`--verbose` is defined by
both) is silently eaten; a second terminator forces it through:
`scc launch claude -- -- -p`.

## The sandbox takes the toolchain away, so it is mapped back

ai-jail's private home replaces `$HOME` with a fresh tmpfs and then prunes `PATH` to
what survived — so `rtk` in `~/.cargo/bin` and an npm-installed `scc` are simply
gone, while the entry file still tells the agent to prefix every command with one
and the rules still tell it to answer questions with the other. It discovers that
one failed command at a time and falls back to reading whole files, which is the
cost this methodology exists to remove (see [[context-budget]]).

So three binaries are mapped back read-only — `scc`, `rtk`, `codegraph` — and the
list is closed because `scc` can name exactly what its own guidance names. Two
details were measured rather than reasoned: mounts apply in order, so **every
directory goes before the files inside it** or a directory bind hides a file bind
underneath it; and `scc` is mapped as the real binary behind the npm shim, because
`os.Executable()` is the Go binary and mapping it at the name `PATH` knows takes
node out of the picture.

## Only npm, and only sometimes

CodeGraph's headline install pipes a remote script into a shell. That is a fine
thing for a person to type and not a thing `scc` executes on their behalf, so npm is
the only installer it will run for it and `InstallHint` names the rest. `cargo` and
`uv`/`pip` installs are prompted for, because a Rust build is a toolchain and
minutes; an unattended run declines rather than deciding silently.

## Related

[[managed-files]] · [[delivery-gate]] · [[context-budget]] · [[hooks]]
