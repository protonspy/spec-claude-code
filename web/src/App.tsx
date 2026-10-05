import { useEffect, useState } from "react";
import { fetchTree, type Tree } from "./api";
import { parseRoute, type Route } from "./links";
import { Home } from "./Home";
import { PageView } from "./PageView";
import { Sidebar } from "./Sidebar";
import { SourceView } from "./SourceView";

function useRoute(): Route {
  const [route, setRoute] = useState(() => parseRoute(location.hash));
  useEffect(() => {
    const on = () => setRoute(parseRoute(location.hash));
    addEventListener("hashchange", on);
    return () => removeEventListener("hashchange", on);
  }, []);
  return route;
}

export function App() {
  const route = useRoute();
  const [tree, setTree] = useState<Tree | null>(null);
  const [error, setError] = useState<string | null>(null);

  // The tree is fetched again on every navigation: the server reads the disk on
  // each request, so a page written since the last click shows up on the next.
  useEffect(() => {
    fetchTree()
      .then((t) => {
        setTree(t);
        setError(null);
        document.title = `${t.workspace} · scc view`;
      })
      .catch((e: Error) => setError(e.message));
  }, [route]);

  const current = route.kind === "home" ? "" : route.path;
  return (
    <div className="shell">
      <Sidebar tree={tree} current={current} />
      <main className="main">
        {error && <p className="error">Could not read the workspace: {error}</p>}
        {route.kind === "home" && <Home tree={tree} />}
        {route.kind === "page" && <PageView key={route.path} path={route.path} heading={route.heading} />}
        {route.kind === "source" && (
          <SourceView key={route.path} path={route.path} from={route.from} to={route.to} />
        )}
      </main>
    </div>
  );
}
