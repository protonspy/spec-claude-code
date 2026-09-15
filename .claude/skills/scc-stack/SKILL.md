---
name: scc-stack
description: Own docs/stack.md — every adopted technology with one line on why it earned its place. Use it before adding a dependency to any manifest, when a technology is swapped or dropped, and on any stack.* finding.
argument-hint: [technology being adopted or dropped]
---

You own `docs/stack.md`. The rule it enforces is in
`.claude/rules/knowledge-base.md`, and it is short: **technology
not listed here is an open decision, never something adopted silently.**

This is checkable because dependency manifests are structured data. `scc validate`
reads the direct dependencies out of every manifest it knows — `go.mod`,
`package.json`, `requirements.txt`, `pyproject.toml`, `Cargo.toml`,
`composer.json`, `pom.xml` — and reports any that `stack.md` does not mention.
Indirect dependencies are skipped: nobody decided those.

## Nothing asked for

`$ARGUMENTS` is the technology being adopted or dropped. Empty, run `scc validate`
and work through the `stack.undocumented-dependency` findings: for each, either
record the decision or establish that nobody can justify the dependency and remove
it. Both are correct outcomes; listing a name with no reason is not.

## Adding a dependency is two steps

Adding it to the manifest is the first. The second is here, and it happens in the
same change, not later:

1. **Say what problem it solves** — the one you actually have, not the category.
2. **Say what it was chosen over**, if anything real was considered. This is the
   line that pays off in a year, when someone asks why not the obvious alternative.
3. **Say what it costs.** Every dependency is a supply-chain surface, a version to
   keep current, and an API someone will have to learn. If you cannot name the cost
   you have not finished evaluating it.

```markdown
## Go

- **stdlib only** — the binary ships to six platforms and every dependency is a
  supply-chain surface. A dependency has to be worth that.
```

The validator matches on the module path or its last segment, so writing `chi` for
`github.com/go-chi/chi` is enough. It is looking for a decision, not a spelling.

## Before you add anything

Ask the three questions, in this order:

1. **Can the standard library do it?** For most of what a dependency is reached for
   first, it can, at the cost of code you then own. Sometimes that trade is right.
2. **Is this dependency load-bearing or convenient?** A convenience with a
   maintainer who stops answering is a migration you did not plan.
3. **Does the project already have something that does this?** Two libraries solving
   one problem is a cost paid by everyone who reads the code afterwards, forever.

**If the answer is not obviously yes, stop and ask.** Adopting technology is a
decision with a long tail, and it is one of the few things worth interrupting an
automatic run for. Bringing a dependency in unasked is not autonomy, it is a
commitment made on someone else's behalf.

## Removing one

Delete it from the manifest, delete the entry, and say in the commit what replaced
it. A stack entry for something no longer installed is worse than a missing one: it
describes a project that does not exist.

## When it is also an ADR

`stack.md` is the inventory — what is adopted, and why. An **ADR** is the record of
a decision that is hard to reverse. Reach for the `adr` skill as well when the
choice locks something in: a database, a wire protocol, a framework the code will
shape itself around, a vendor with data in it.

The two do not duplicate each other. The stack entry stays one line and names the
ADR; the ADR carries the context, the alternatives, and the consequences.

## Findings

| Finding | What it means | The fix |
|---|---|---|
| `stack.missing` | The project declares dependencies and has no `stack.md`. | Write it, one entry per direct dependency already installed. Expect to discover at least one nobody can justify. |
| `stack.undocumented-dependency` | Something is installed that `stack.md` does not mention. | Record the decision — or, if no one can state what it is for, remove the dependency. Both are correct outcomes. Silencing the finding by listing the name with no reason is not. |
