import { describe, expect, it } from "vitest";
import { linkify, normalize, parseRoute, resolveLink, slug } from "./links";

describe("slug", () => {
  it("matches mdscan.Slug", () => {
    expect(slug("How the dispatcher routes")).toBe("how-the-dispatcher-routes");
    expect(slug("R1 · Serving")).toBe("r1--serving");
    expect(slug("Out of scope")).toBe("out-of-scope");
    expect(slug("Ação_1-b")).toBe("ação_1-b");
    expect(slug("`code` & (parens)")).toBe("code--parens");
  });
});

describe("linkify", () => {
  it("turns wikilinks into page links", () => {
    expect(linkify("see [[context-budget]] and [[note-log|notes]]")).toBe(
      "see [context-budget](#/page/docs/wiki/pages/context-budget.md) and [notes](#/page/docs/wiki/pages/note-log.md)",
    );
  });

  it("turns adr citations into page links", () => {
    expect(linkify("per adr:0001-stdlib-only-dependencies.")).toBe(
      "per [adr:0001-stdlib-only-dependencies](#/page/docs/adr/0001-stdlib-only-dependencies.md).",
    );
  });

  it("links a code span that is exactly an adr citation", () => {
    expect(linkify("see `adr:0001-a`.")).toBe("see [`adr:0001-a`](#/page/docs/adr/0001-a.md).");
    expect(linkify("`run adr:0001-a`")).toBe("`run adr:0001-a`");
  });

  it("leaves code spans and fenced blocks alone", () => {
    const md = "`[[x]]` and [[y]]\n```\n[[z]] adr:0001-a\n```\n~~~~\n[[w]]\n~~~~\n[[v]]";
    const out = linkify(md);
    expect(out).toContain("`[[x]]`");
    expect(out).toContain("[y](#/page/docs/wiki/pages/y.md)");
    expect(out).toContain("\n[[z]] adr:0001-a\n");
    expect(out).toContain("\n[[w]]\n");
    expect(out).toContain("[v](#/page/docs/wiki/pages/v.md)");
  });
});

describe("resolveLink", () => {
  const from = "docs/codewiki/packages.md";

  it("sends a codewiki citation to the source view with its range", () => {
    expect(resolveLink("", "internal/cli/cli.go:48-64", from)).toEqual({
      internal: "#/source/internal/cli/cli.go:48-64",
    });
    expect(resolveLink("", "internal/cli/cli.go:7", from)).toEqual({
      internal: "#/source/internal/cli/cli.go:7",
    });
  });

  it("goes nowhere for an empty link that is not a citation", () => {
    expect(resolveLink("", "just text", from)).toBeNull();
    expect(resolveLink("", "../../etc/passwd:1", from)).toBeNull();
  });

  it("resolves relative Markdown links against the page directory", () => {
    expect(resolveLink("../wiki/index.md", "", from)).toEqual({ internal: "#/page/docs/wiki/index.md" });
    expect(resolveLink("other.md#risks", "", from)).toEqual({
      internal: "#/page/docs/codewiki/other.md?h=risks",
    });
  });

  it("sends a relative link to a non-Markdown file to the source view", () => {
    expect(resolveLink("../../internal/x.go", "", from)).toEqual({ internal: "#/source/internal/x.go" });
  });

  it("refuses a link that climbs out of the workspace", () => {
    expect(resolveLink("../../../secret.md", "", from)).toBeNull();
  });

  it("keeps a same-page anchor on the page", () => {
    expect(resolveLink("#risks", "", from)).toEqual({ internal: "#/page/docs/codewiki/packages.md?h=risks" });
  });

  it("opens web links outside and drops other schemes", () => {
    expect(resolveLink("https://example.com", "", from)).toEqual({ external: "https://example.com" });
    expect(resolveLink("javascript:alert(1)", "", from)).toBeNull();
    expect(resolveLink("file:///etc/passwd", "", from)).toBeNull();
  });
});

describe("parseRoute", () => {
  it("reads pages, headings and sources back", () => {
    expect(parseRoute("")).toEqual({ kind: "home" });
    expect(parseRoute("#/page/docs/wiki/index.md?h=start-here")).toEqual({
      kind: "page",
      path: "docs/wiki/index.md",
      heading: "start-here",
    });
    expect(parseRoute("#/source/internal/cli/cli.go:48-64")).toEqual({
      kind: "source",
      path: "internal/cli/cli.go",
      from: 48,
      to: 64,
    });
    expect(parseRoute("#/source/go.mod")).toEqual({ kind: "source", path: "go.mod" });
  });
});

describe("normalize", () => {
  it("resolves dots and refuses escapes", () => {
    expect(normalize("a/./b/../c")).toBe("a/c");
    expect(normalize("../a")).toBeNull();
  });
});
