# Stack

Every adopted technology, with one line on why it earned its place. **Technology
that is not listed here is an open decision, never something adopted silently** —
and because dependency manifests are structured data, that rule is checkable: a
direct dependency in `go.mod` or `package.json` that is absent from this file is
reported.

So adding a dependency is a two-step act: add it, and say here why.

```markdown
## Runtime

- **the-http-router** — routing with the standard library's handler contract, no
  framework underneath it.

## Development

- **the-test-runner** — what CI runs; the suite is the gate for a feature.
```

Group however the project actually thinks about it. What matters is that the name
appears and the reason is one line a reader can disagree with.

<!-- Entries go below this line. Delete the guidance above once they read for
themselves. -->
