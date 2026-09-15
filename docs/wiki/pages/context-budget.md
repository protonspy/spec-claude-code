# The context budget

The single idea most of `scc`'s design decisions come out of: **what an agent reads
is not free, and what it reads at session start is paid again in every request of
that session.** A file loaded once sits in the conversation for the rest of the run,
so a byte added to a preloaded file is a byte multiplied by the number of turns.

That turns several apparently unrelated choices into one choice.

## The three tiers

Everything the methodology ships falls into one of three, and the tier decides how
much it may say.

| Tier | Paid | Consequence |
|---|---|---|
| The entry file and unscoped rules | every request, whole | capped by test: a rule at 55 lines, the entry file at 60 |
| A skill's `description` | every request, description only | capped at 340 characters each, 2400 for the set |
| A skill's body, a `--help` string, a wiki page | once, by the run that needed it | where detail belongs |

The corollary is a writing rule rather than a style preference: when something does
not fit in a rule, the answer is to move the detail down a tier — into a `--help`
string, read only when consulted — and leave the question-to-command table in the
rule. It is the same reasoning that sends a skill's detail out of its description
and into its body.

## Where the lever actually is

Measured on a freshly scaffolded Claude Code workspace, the entry file plus fifteen
rules came to 47KB in every request. Three of those rules now carry a `paths:`
header, so the harness loads them only when the agent touches the tree they govern —
about 10.4KB, a quarter of the standing cost, moved without changing a word of the
methodology. See [[scoped-rules]] for why only those three qualify.

## Why the artifacts became queryable

A plan measured at 56KB, and its workspace's 31 specs brought the corpus to roughly
90k tokens. An agent answering *what is the next open task?* by opening the plan
carries the whole plan for the rest of the session — so `scc map` exists to answer
the question without the file, and `scc patch` exists to change one line of it
without reading it first. See [[artifact-addressing]].

This is also why no `scc map` command returns both a plan's header and its
checklist: `brief` is paid once per session, `tasks --next` is paid per task, and a
command returning both would quietly restore the cost the split removed.

## Why the agent talks in fragments

The `caveman` rule sets the register the agent answers in, and its justification is
this same arithmetic: prose about the work is context every later request carries,
so narration is the part of a long run that can be cut without losing a fact. What
it must never compress is anything another reader parses — artifacts, commands,
quoted output, commit bodies, and questions put to the user. A denser EARS line is a
finding, not a saving.

## Why blocks are condensed rather than inherited

`scc` splices RTK's usage block into the entry file and replaces a larger one that
says the same thing: measured on rtk 0.42.4, the upstream block is 139 lines against
`scc`'s 18, both stamped the same version. The difference is paid continuously
rather than once, which is what makes replacing it worth the rudeness. See
[[integration-boundary]].

## Related

[[artifact-addressing]] · [[scoped-rules]] · [[integration-boundary]] · [[validation-contract]]
