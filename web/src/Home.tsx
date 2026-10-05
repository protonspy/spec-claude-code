import type { Item, Tree } from "./api";
import { pageHref } from "./links";
import { Progress } from "./Progress";

export function Home({ tree }: { tree: Tree | null }) {
  if (!tree) return <p className="muted">Loading…</p>;
  const group = (id: string) => tree.groups.find((g) => g.id === id)?.items ?? [];
  const plans = group("plans");
  const tasks = group("specs").filter((it) => it.kind === "tasks");
  const index = group("wiki").find((it) => it.path === "docs/wiki/index.md");

  return (
    <article className="home">
      <header className="page-head">
        <h1>{tree.workspace}</h1>
        <p className="muted">The workspace's record: what was decided, what is being built, and how far along it is.</p>
      </header>

      <div className="cards">
        {tree.groups.map((g) => (
          <a key={g.id} className="card" href={g.items[0] ? pageHref(first(g.id, g.items)) : undefined}>
            <span className="card-n">{g.id === "specs" ? new Set(g.items.map((i) => i.path.split("/")[1])).size : g.items.length}</span>
            <span className="card-t">{g.title}</span>
          </a>
        ))}
      </div>

      {(plans.length > 0 || tasks.length > 0) && (
        <section>
          <h2>Progress</h2>
          <ul className="progress-list">
            {[...plans, ...tasks].map((it) => (
              <li key={it.path}>
                <a href={pageHref(it.path)}>{it.kind === "tasks" ? it.path.split("/")[1] : it.title}</a>
                <span className="muted small">{it.kind === "plan" ? "plan" : "spec"}</span>
                <Progress done={it.done} total={it.total} />
              </li>
            ))}
          </ul>
        </section>
      )}

      {index && (
        <p>
          Start with the <a href={pageHref(index.path)}>wiki index</a>.
        </p>
      )}
    </article>
  );
}

function first(id: string, items: Item[]): string {
  if (id === "wiki") return items.find((i) => i.path === "docs/wiki/index.md")?.path ?? items[0].path;
  return items[0].path;
}
