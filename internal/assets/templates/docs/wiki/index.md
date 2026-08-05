# Wiki

The entry point. Every page under `wiki/pages/` has to be reachable from here —
directly, or through a page that is — because a page nothing links to is a page
nobody will find again.

Pages link to each other as `[[page-slug]]`, where the slug is the filename without
its extension and without its directory. A link that resolves to no page is reported,
and so is a page this index cannot reach.

This file and `changelog.md` live here rather than in `pages/`: they are the wiki's
fixed documents, not pages, and neither is ever an orphan.

## Pages

<!-- One line per page, as `- [[page-slug]] — what it covers`. Group them under
their own headings once there are enough that a flat list stops helping. -->
