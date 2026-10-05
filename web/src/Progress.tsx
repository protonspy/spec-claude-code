export function Progress({ done, total }: { done: number; total: number }) {
  if (total === 0) return <span className="muted small">no tasks</span>;
  const pct = Math.round((done / total) * 100);
  return (
    <span className="progress" title={`${done} of ${total} tasks done`}>
      <span className="bar" role="progressbar" aria-valuemin={0} aria-valuemax={total} aria-valuenow={done}>
        <span className="fill" style={{ width: `${pct}%` }} />
      </span>
      <span className="small">
        {done}/{total}
      </span>
    </span>
  );
}
