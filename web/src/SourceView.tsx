import { useEffect, useRef, useState } from "react";
import { fetchSource, type Source } from "./api";

export function SourceView({ path, from, to }: { path: string; from?: number; to?: number }) {
  const [src, setSrc] = useState<Source | null>(null);
  const [error, setError] = useState<string | null>(null);
  const first = useRef<HTMLTableRowElement>(null);

  useEffect(() => {
    fetchSource(path).then(setSrc, (e: Error) => setError(e.message));
  }, [path]);

  useEffect(() => {
    if (src && first.current) first.current.scrollIntoView({ block: "center" });
    else if (src) window.scrollTo(0, 0);
  }, [src, from]);

  if (error) return <p className="error">{path}: {error}</p>;
  if (!src) return <p className="muted">Loading…</p>;

  const lo = from ?? 0;
  const hi = to ?? from ?? 0;
  const outOfRange = from !== undefined && from > src.lines.length;
  return (
    <article className="source">
      <header className="page-head">
        <div className="crumb">
          <code>{src.path}</code>
          {from !== undefined && <span className="kind">{from === hi ? `line ${from}` : `lines ${from}–${hi}`}</span>}
        </div>
        <button className="back" onClick={() => history.back()}>
          ← Back
        </button>
      </header>
      {outOfRange && (
        <p className="error">
          The citation points past the end of the file ({src.lines.length} lines) — the codewiki page has drifted.
        </p>
      )}
      <div className="code">
        <table>
          <tbody>
            {src.lines.map((line, i) => {
              const n = i + 1;
              const hit = n >= lo && n <= hi;
              return (
                <tr key={n} id={`L${n}`} className={hit ? "hit" : undefined} ref={n === lo ? first : undefined}>
                  <td className="ln">{n}</td>
                  <td className="tx">{line || " "}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </article>
  );
}
