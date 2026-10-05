// Everything the viewer decides about where a link goes lives here, so the
// rendering components never parse a path themselves.

/** A place in the viewer, addressed by the URL hash. */
export type Route =
  | { kind: "home" }
  | { kind: "page"; path: string; heading?: string }
  | { kind: "source"; path: string; from?: number; to?: number };

/**
 * slug renders heading text as an anchor exactly as `mdscan.Slug` does in Go:
 * lowercased, spaces to hyphens, everything that is not a letter, digit, hyphen
 * or underscore dropped. The table of contents comes from the server's sections
 * and the headings are rendered here, so the two must agree to the byte.
 */
export function slug(text: string): string {
  let out = "";
  for (const ch of text.toLowerCase()) {
    if (ch === " ") out += "-";
    else if (/[\p{L}\p{N}_-]/u.test(ch)) out += ch;
  }
  return out;
}

const wikilinkRe = /\[\[([^[\]]+)\]\]/g;
const adrRe = /\badr:(\d{4}-[a-z0-9][a-z0-9-]*)\b/g;
const fenceRe = /^[ \t]{0,3}(`{3,}|~{3,})/;
const citationRe = /^(.+?):(\d+)(?:-(\d+))?$/;

/** pageHref is the hash for a page, with an optional heading to scroll to. */
export function pageHref(path: string, heading?: string): string {
  return "#/page/" + encodeURI(path) + (heading ? "?h=" + encodeURIComponent(heading) : "");
}

/** sourceHref is the hash for a source file, with an optional cited range. */
export function sourceHref(path: string, from?: number, to?: number): string {
  let h = "#/source/" + encodeURI(path);
  if (from !== undefined) h += ":" + from + (to !== undefined && to !== from ? "-" + to : "");
  return h;
}

/**
 * linkify turns the two link forms Markdown does not know — `[[slug]]` wikilinks
 * and `adr:<name>` citations — into ordinary Markdown links to viewer routes.
 * Fenced blocks and inline code are left alone, the same masking the validators
 * apply, so an example in a code block is not turned into a link — except a code
 * span that is exactly one ADR citation.
 */
export function linkify(markdown: string): string {
  let fence: string | null = null;
  return markdown
    .split("\n")
    .map((line) => {
      const m = fenceRe.exec(line);
      if (fence !== null) {
        if (m && m[1][0] === fence[0] && m[1].length >= fence.length) fence = null;
        return line;
      }
      if (m) {
        fence = m[1];
        return line;
      }
      return line
        .split(/(`+[^`]*`+)/)
        .map((part, i) => (i % 2 === 1 ? linkifyCode(part) : linkifyProse(part)))
        .join("");
    })
    .join("\n");
}

// A code span holding nothing but an ADR citation is how this workspace usually
// writes one, so that span becomes a link; any other code span stays as it is.
function linkifyCode(span: string): string {
  const m = /^`+\s*adr:(\d{4}-[a-z0-9][a-z0-9-]*)\s*`+$/.exec(span);
  return m ? `[${span}](${pageHref(`docs/adr/${m[1]}.md`)})` : span;
}

function linkifyProse(text: string): string {
  return text
    .replace(wikilinkRe, (_, inner: string) => {
      const [target, label] = inner.split("|", 2).map((s) => s.trim());
      return `[${label || target}](${pageHref(`docs/wiki/pages/${target}.md`)})`;
    })
    .replace(adrRe, (whole, name: string) => `[${whole}](${pageHref(`docs/adr/${name}.md`)})`);
}

/** What a rendered link does: navigate inside the viewer, leave it, or nothing. */
export type Target = { internal: string } | { external: string } | null;

/**
 * resolveLink decides where a link in the page at `from` goes. A codewiki
 * citation is a link with an empty destination whose text is
 * `<path>:<from>[-<to>]`; a relative link is resolved against the page's own
 * directory, and one that climbs out of the workspace goes nowhere.
 */
export function resolveLink(href: string, text: string, from: string): Target {
  if (href === "") {
    const m = citationRe.exec(text.trim());
    if (!m) return null;
    const path = normalize(m[1]);
    if (path === null) return null;
    return { internal: sourceHref(path, Number(m[2]), m[3] ? Number(m[3]) : undefined) };
  }
  if (href.startsWith("#/")) return { internal: href };
  if (href.startsWith("#")) return { internal: pageHref(from, href.slice(1)) };
  if (/^[a-z][a-z0-9+.-]*:/i.test(href) || href.startsWith("//")) {
    return /^(https?|mailto):/i.test(href) ? { external: href } : null;
  }
  const [rawPath, anchor] = splitOnce(href, "#");
  const base = from.includes("/") ? from.slice(0, from.lastIndexOf("/") + 1) : "";
  const path = normalize(rawPath.startsWith("/") ? rawPath.slice(1) : base + decodeSafe(rawPath));
  if (path === null) return null;
  if (path.endsWith(".md")) return { internal: pageHref(path, anchor || undefined) };
  return { internal: sourceHref(path) };
}

/** parseRoute reads the URL hash back into a route. */
export function parseRoute(hash: string): Route {
  const h = hash.replace(/^#/, "");
  if (h.startsWith("/page/")) {
    const [path, query] = splitOnce(h.slice("/page/".length), "?");
    const heading = new URLSearchParams(query).get("h") ?? undefined;
    return { kind: "page", path: decodeSafe(path), heading };
  }
  if (h.startsWith("/source/")) {
    const rest = decodeSafe(h.slice("/source/".length));
    const m = /^(.*?):(\d+)(?:-(\d+))?$/.exec(rest);
    if (!m) return { kind: "source", path: rest };
    const from = Number(m[2]);
    return { kind: "source", path: m[1], from, to: m[3] ? Number(m[3]) : from };
  }
  return { kind: "home" };
}

/** normalize resolves `.` and `..` segments, and refuses a path that escapes. */
export function normalize(path: string): string | null {
  const out: string[] = [];
  for (const seg of path.split("/")) {
    if (seg === "" || seg === ".") continue;
    if (seg === "..") {
      if (out.length === 0) return null;
      out.pop();
    } else out.push(seg);
  }
  return out.length ? out.join("/") : null;
}

function splitOnce(s: string, sep: string): [string, string] {
  const i = s.indexOf(sep);
  return i < 0 ? [s, ""] : [s.slice(0, i), s.slice(i + 1)];
}

function decodeSafe(s: string): string {
  try {
    return decodeURI(s);
  } catch {
    return s;
  }
}
