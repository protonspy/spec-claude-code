# Changelog

What changed in the wiki, and when. Newest first, one line per change, naming the
pages it touched — this is the log that says whether a page was revisited after the
thing it describes moved.

A page named here that no longer exists is reported, so a rename is recorded as a
change rather than left pointing at the old slug.

- 2026-09-19 — `CLAUDE.md` went back to being the entry file the binary ships, so
  what it carried moved to the one place that owns it: extended [[delivery-gate]]
  (the grouped `check` object, the distinct findings), [[integration-boundary]] (the
  Windows dev container, one graph per scoped tree), [[managed-files]] (`CarryOver`),
  [[artifact-addressing]] (the reading surface, BM25 over regions, the seal) and
  [[spec-or-plan]] (the spec's delivery record); added [[note-log]] and [[release]],
  and `docs/codewiki/packages.md` and the first two ADRs. Review follow-up: named
  Headroom's foreign RTK block in [[managed-files]], and corrected four codewiki
  citations to the lines that actually evidence them.
- 2026-09-15 — first pass, reconstructed from `CLAUDE.md`, `.claude/rules/`, `design/`
  and the tree itself: added [[context-budget]], [[scoped-rules]],
  [[artifact-addressing]], [[spec-or-plan]], [[harness-profile]], [[managed-files]],
  [[validation-contract]], [[delivery-gate]], [[hooks]] and
  [[integration-boundary]], and wrote `index.md` to group them.
