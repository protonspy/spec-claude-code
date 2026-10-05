import type { Group, Item, Tree } from "./api";
import { pageHref } from "./links";

const specFile: Record<string, string> = {
  requirements: "Requirements",
  design: "Design",
  tasks: "Tasks",
};

export function Sidebar({ tree, current }: { tree: Tree | null; current: string }) {
  return (
    <nav className="sidebar" aria-label="Workspace">
      <a className="brand" href="#/">
        <span className="brand-mark">scc</span>
        <span className="brand-name">{tree?.workspace ?? "…"}</span>
      </a>
      {tree?.groups.map((g) =>
        g.items.length === 0 ? null : (
          <section key={g.id} className="group">
            <h2>
              {g.title} <span className="count">{g.id === "specs" ? specCount(g) : g.items.length}</span>
            </h2>
            {g.id === "specs" ? <Specs items={g.items} current={current} /> : <Items items={g.items} current={current} />}
          </section>
        ),
      )}
    </nav>
  );
}

function Items({ items, current }: { items: Item[]; current: string }) {
  return (
    <ul>
      {items.map((it) => (
        <li key={it.path}>
          <Link item={it} current={current} label={label(it)} />
        </li>
      ))}
    </ul>
  );
}

// A spec is three files under one feature name, so the sidebar shows the feature
// once with its files beneath it rather than three near-identical titles.
function Specs({ items, current }: { items: Item[]; current: string }) {
  const byFeature = new Map<string, Item[]>();
  for (const it of items) {
    const feature = it.path.split("/")[1];
    byFeature.set(feature, [...(byFeature.get(feature) ?? []), it]);
  }
  return (
    <ul>
      {[...byFeature].map(([feature, files]) => (
        <li key={feature} className="feature">
          <span className="feature-name">{feature}</span>
          <ul>
            {files.map((it) => (
              <li key={it.path}>
                <Link item={it} current={current} label={specFile[it.kind] ?? it.title} />
              </li>
            ))}
          </ul>
        </li>
      ))}
    </ul>
  );
}

function Link({ item, current, label }: { item: Item; current: string; label: string }) {
  const active = item.path === current;
  return (
    <a href={pageHref(item.path)} className={active ? "active" : undefined} aria-current={active ? "page" : undefined}>
      <span className="label">{label}</span>
      {item.total > 0 && (
        <span className={item.done === item.total ? "tally done" : "tally"}>
          {item.done}/{item.total}
        </span>
      )}
    </a>
  );
}

// An ADR's filename carries its number, which the title does not.
function label(it: Item): string {
  const m = /\/(\d{4})-[^/]+\.md$/.exec(it.path);
  return m && it.path.startsWith("docs/adr/") ? `${m[1]} · ${it.title}` : it.title;
}

function specCount(g: Group): number {
  return new Set(g.items.map((it) => it.path.split("/")[1])).size;
}
