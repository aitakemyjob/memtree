import { useMemo, useState } from 'react';
import { api } from '../lib/api';
import { firstLine, projectName, relTime, truncate } from '../lib/format';
import { Modal, Spinner, useAsync } from './ui';

/** Pick the memory that replaces `id`; the old one is kept but stops appearing in recall. */
export function SupersedePicker({ id, onPick, onClose }: { id: string; onPick: (by: string) => void; onClose: () => void }) {
  const [q, setQ] = useState('');
  const all = useAsync(() => api.listMemories({ allProjects: true, limit: 500 }), []);
  const shown = useMemo(() => {
    const t = q.trim().toLowerCase();
    return (all.data ?? []).filter((r) => r.id !== id && (!t || r.summary.toLowerCase().includes(t) || r.id.includes(t) || r.keywords.some((k) => k.includes(t)))).slice(0, 60);
  }, [all.data, q, id]);
  return (
    <Modal title="Superseded by which memory?" onClose={onClose}>
      <div className="muted">The newer memory takes over. This one stays in the lineage but no longer shows up in recall. You can undo it later.</div>
      <input className="input" autoFocus aria-label="Search memories" placeholder="Search memories…" value={q} onChange={(e) => setQ(e.target.value)} />
      <div style={{ position: 'relative', minHeight: 60 }}>
        {all.loading && !all.data && <Spinner />}
        {shown.map((r) => (
          <button key={r.id} className="pick-row" onClick={() => { onPick(r.id); onClose(); }}>
            <span className="grow ellipsis">{truncate(firstLine(r.summary), 110)}</span>
            <span className="faint">{projectName(r.project)} · {relTime(r.createdAt)}</span>
          </button>
        ))}
        {!all.loading && shown.length === 0 && <div className="faint">No other memories match.</div>}
      </div>
    </Modal>
  );
}
