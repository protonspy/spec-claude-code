import { Children, isValidElement, useEffect, useMemo, useState, type ReactNode } from "react";
import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { fetchPage, type Page } from "./api";
import { linkify, pageHref, resolveLink, slug } from "./links";
import { Progress } from "./Progress";

export function PageView({ path, heading }: { path: string; heading?: string }) {
  const [page, setPage] = useState<Page | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchPage(path).then(setPage, (e: Error) => setError(e.message));
  }, [path]);

  useEffect(() => {
    if (!page) return;
    if (heading) document.getElementById(heading)?.scrollIntoView();
    else window.scrollTo(0, 0);
  }, [page, heading]);

  const components = useMemo(() => markdownComponents(path), [path]);
  const body = useMemo(() => (page ? linkify(page.body) : ""), [page]);

  if (error) return <p className="error">{path}: {error}</p>;
  if (!page) return <p className="muted">Loading…</p>;

  const toc = page.sections.filter((s) => s.level >= 2 && s.level <= 3);
  const fm = Object.entries(page.frontmatter);
  return (
    <div className="page">
      <article className="doc">
        <header className="page-head">
          <div className="crumb">
            <code>{page.path}</code>
            <span className="kind">{page.kind}</span>
          </div>
          {fm.length > 0 && (
            <dl className="frontmatter">
              {fm.map(([k, v]) => (
                <div key={k}>
                  <dt>{k}</dt>
                  <dd>{v}</dd>
                </div>
              ))}
            </dl>
          )}
          {page.total > 0 && <Progress done={page.done} total={page.total} />}
        </header>
        <div className="markdown">
          <Markdown remarkPlugins={[remarkGfm]} components={components}>
            {body}
          </Markdown>
        </div>
      </article>
      {toc.length > 1 && (
        <aside className="toc" aria-label="On this page">
          <h2>On this page</h2>
          <ul>
            {toc.map((s) => (
              <li key={s.slug + s.line} className={`l${s.level}`}>
                <a href={pageHref(path, s.slug)}>{s.title}</a>
              </li>
            ))}
          </ul>
        </aside>
      )}
    </div>
  );
}

function markdownComponents(from: string): Components {
  const heading =
    (Tag: "h1" | "h2" | "h3" | "h4" | "h5" | "h6") =>
    ({ children }: { children?: ReactNode }) => <Tag id={slug(textOf(children))}>{children}</Tag>;
  return {
    h1: heading("h1"),
    h2: heading("h2"),
    h3: heading("h3"),
    h4: heading("h4"),
    h5: heading("h5"),
    h6: heading("h6"),
    a: ({ href, children }) => {
      const target = resolveLink(href ?? "", textOf(children), from);
      if (target === null) return <span className="dead-link">{children}</span>;
      if ("external" in target) {
        return (
          <a href={target.external} target="_blank" rel="noreferrer noopener">
            {children}
          </a>
        );
      }
      return (
        <a href={target.internal} className={target.internal.startsWith("#/source/") ? "cite" : undefined}>
          {children}
        </a>
      );
    },
  };
}

// textOf flattens rendered children back to the text they came from, which is
// what a heading's slug and a citation's path are computed from.
function textOf(node: ReactNode): string {
  let out = "";
  Children.forEach(node, (child) => {
    if (typeof child === "string" || typeof child === "number") out += child;
    else if (isValidElement<{ children?: ReactNode }>(child)) out += textOf(child.props.children);
  });
  return out;
}
