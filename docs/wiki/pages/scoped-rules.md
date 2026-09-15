# Scoped rules

A rule under `<harness>/rules/` is normally preloaded: on Claude Code every file
there is loaded at launch with the same priority as the entry file, and therefore
carried in every request of the session. A **scoped** rule carries a `paths:` header
instead, so the harness loads it only when the agent touches the tree it governs.

Scoping is the only lever there is on what the methodology costs — see
[[context-budget]] — which is exactly why it is not a matter of taste.

## The bar

**A rule may be scoped only where a validator reports what it prevents.**

Scoping trades *in context before the decision* for *in context when the file is
open*. That trade is survivable precisely when something else catches the result: a
malformed EARS line, a task with no methodology annotation, a broken wikilink are
all `scc validate` exit `2` before the commit. Where nothing mechanical reports the
failure, the rule has to be in context before the agent decides — which means
unscoped.

`TestOnlyRulesAValidatorBacksAreScoped` holds the bar in the test suite, so the
argument cannot be lost to a later edit.

## The three that qualify

| Rule | Scope | Backed by |
|---|---|---|
| `specs.md` | `specs/**` | the EARS and spec validators |
| `tasks.md` | `specs/**`, `plans/**` | the task-grammar findings |
| `knowledge-base.md` | `docs/**` | the wiki, glossary, ADR, codewiki and stack validators |

## The refusals, and why they are clearer than the borderline cases

The rules that look most temptingly scopable are the firmest noes:

- **`artifacts.md`** would scope to `plans/**`, and its central instruction is
  *never open the plan*. A scoped copy would load only at the moment the thing it
  prevents has already happened.
- **`prior-art.md`** and **`routing.md`** fire *before any file exists* — one says
  read the record before writing an artifact, the other picks which artifact to
  write. There is no path to attach them to.
- **`caveman.md`**, **`ladder.md`** and **`delivery.md`** govern how the agent
  works rather than what it edits, and nothing mechanical reports a failure to
  follow them.

## Related

[[context-budget]] · [[validation-contract]] · [[harness-profile]]
