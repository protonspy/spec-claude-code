// The three routes `scc view` answers; the shapes mirror internal/view.

export interface Item {
  path: string;
  title: string;
  kind: string;
  done: number;
  total: number;
}

export interface Group {
  id: "wiki" | "adr" | "codewiki" | "docs" | "plans" | "specs";
  title: string;
  items: Item[];
}

export interface Tree {
  workspace: string;
  groups: Group[];
}

export interface Section {
  slug: string;
  level: number;
  title: string;
  line: number;
}

export interface Task {
  number: string;
  checked: boolean;
  methodology?: string;
  text: string;
  requirements?: string[];
  status?: string;
  blocked: boolean;
}

export interface Page {
  path: string;
  kind: string;
  title: string;
  frontmatter: Record<string, string>;
  sections: Section[];
  tasks: Task[];
  done: number;
  total: number;
  body: string;
}

export interface Source {
  path: string;
  lines: string[];
}

async function get<T>(url: string): Promise<T> {
  const res = await fetch(url, { cache: "no-store" });
  if (!res.ok) throw new Error(res.status === 404 ? "Not found, or not readable from the viewer." : `HTTP ${res.status}`);
  return (await res.json()) as T;
}

export const fetchTree = () => get<Tree>("/api/tree");
export const fetchPage = (path: string) => get<Page>("/api/page?path=" + encodeURIComponent(path));
export const fetchSource = (path: string) => get<Source>("/api/source?path=" + encodeURIComponent(path));
