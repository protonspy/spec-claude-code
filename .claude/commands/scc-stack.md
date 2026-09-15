---
description: Record an adopted technology in docs/stack.md, or account for a dependency that nobody decided on
argument-hint: [technology being adopted or dropped]
---

Use the `stack` skill.

Technology: $ARGUMENTS

If nothing was named, run `scc validate` and work through the
`stack.undocumented-dependency` findings: for each one, either record the decision or
establish that nobody can justify the dependency and remove it. Both are correct
outcomes; listing a name with no reason is not.

Adopting technology is a decision with a long tail. If it is not obviously the right
call, stop and ask rather than committing on someone else's behalf.
