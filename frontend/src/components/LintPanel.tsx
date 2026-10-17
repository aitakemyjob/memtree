import { useState } from 'react';
import { ArrowLeftRight, Sparkles } from 'lucide-react';
import { api } from '../lib/api';
import { truncate } from '../lib/format';
import { useStore } from '../lib/store';
import type { LintReport } from '../lib/types';
import { Confirm, ErrorBox, Modal, PathEditor, PathInput, Spinner } from './ui';

/** Findings the server omitted because each list is capped. */
export function hiddenFindings(r: LintReport): number {
  return Math.max(0, r.total - (r.nearDuplicates.length + r.outsideAxes.length + r.singleUse.length + r.weaklyFiled.length));
}

/** Which of two near-duplicate categories is the larger (ties keep A), the default merge target. */
export function defaultTarget(d: { a: string; b: string; linksA: number; linksB: number }): 'a' | 'b' {
  return d.linksB > d.linksA ? 'b' : 'a';
}

function Group({ title, hint, count, children }: { title: string; hint: string; count: number; children: React.ReactNode }) {
  if (count === 0) return null;
  return (
    <section className="lint-group">
      <h3 className="insp-h" style={{ marginTop: 6 }}>{title} <span className="badge warn">{count}</span></h3>
      <div className="faint" style={{ marginBottom: 6 }}>{hint}</div>
      {children}
    </section>
  );
}

/**
 * Lattice hygiene findings with per-row fixes. Every finding is advisory;
 * nothing changes until you press a button.
 */
export function LintPanel({ report, loading, error, onShow, onChanged }: {
  report?: LintReport; loading: boolean; error: unknown; onShow: (ids: string[]) => void; onChanged: () => void;
}) {
  const s = useStore();
  const axisRoots = s.app?.axisRoots ?? [];
  const [flipped, setFlipped] = useState<Record<string, boolean>>({});
  const [confirm, setConfirm] = useState<{ from: string; to: string; moves: number }>();
  const [mergeFrom, setMergeFrom] = useState<string>();
  const [openRow, setOpenRow] = useState<string>();
  const [paths, setPaths] = useState<Record<string, string[]>>({});

  const done = (msg: string) => {
    s.toast({ kind: 'ok', title: msg });
    s.bump({ lattice: true, memory: true });
    void s.refreshNodes();
    onChanged();
  };
  const merge = async (from: string, to: string) => {
    try {
      await api.merge(from, to);
      done(`Merged ${from} into ${to}`);
    } catch (e) {
      s.fail('Merge failed', e);
    }
  };
  const file = async (id: string, key: string) => {
    try {
      await api.tag(id, paths[key] ?? []);
      setOpenRow(undefined);
      done('Filed');
    } catch (e) {
      s.fail('Could not file the memory', e);
    }
  };
  const filer = (key: string, id: string, label: string) => (
    <div className="lint-file">
      <PathEditor paths={paths[key] ?? []} onChange={(p) => setPaths((x) => ({ ...x, [key]: p }))} />
      <button className="btn primary sm" disabled={(paths[key] ?? []).length === 0} onClick={() => file(id, key)}>{label}</button>
    </div>
  );

  if (loading && !report) return <div style={{ position: 'relative', minHeight: 60 }}><Spinner /></div>;
  if (error) return <ErrorBox error={error} />;
  if (!report) return null;
  const hidden = hiddenFindings(report);
  const empty = report.total === 0;

  return (
    <div className="lint-body">
      {empty && <div className="faint" style={{ padding: 14 }}>The lattice is tidy.</div>}

      <Group title="Near-duplicate categories" count={report.nearDuplicates.length} hint="Sibling categories that probably mean the same thing. Merging moves every memory link from one into the other.">
        {report.nearDuplicates.map((d) => {
          const key = `${d.a}|${d.b}`;
          const t = flipped[key] ? (defaultTarget(d) === 'a' ? 'b' : 'a') : defaultTarget(d);
          const to = t === 'a' ? d.a : d.b;
          const from = t === 'a' ? d.b : d.a;
          const moves = t === 'a' ? d.linksB : d.linksA;
          return (
            <div key={key} className="lint-row">
              <div className="grow" style={{ minWidth: 0 }}>
                <span className="mono">{d.a}</span> <span className="faint">({d.linksA})</span> ~ <span className="mono">{d.b}</span> <span className="faint">({d.linksB})</span>
                <div className="faint">{d.reason}</div>
              </div>
              <button className="btn sm" onClick={() => onShow([d.a, d.b])}>Show</button>
              <button className="btn sm" aria-label="Swap merge direction" title="Swap direction" onClick={() => setFlipped((x) => ({ ...x, [key]: !x[key] }))}><ArrowLeftRight size={12} /></button>
              <button className="btn primary sm" onClick={() => setConfirm({ from, to, moves })}>Merge into <span className="mono">{to.split('/').pop()}</span></button>
            </div>
          );
        })}
      </Group>

      <Group title="Roots outside the standard axes" count={report.outsideAxes.length} hint={`Top-level categories other than ${axisRoots.join(', ')}. Agents look under the standard axes first.`}>
        {report.outsideAxes.map((id) => (
          <div key={id} className="lint-row">
            <span className="mono grow">{id}</span>
            <button className="btn sm" onClick={() => onShow([id])}>Show</button>
            <button className="btn sm" onClick={() => setMergeFrom(id)}>Merge into…</button>
          </div>
        ))}
      </Group>

      <Group title="Single-use categories" count={report.singleUse.length} hint="A leaf holding exactly one memory. It is fine to keep if the topic will recur; otherwise move the memory somewhere broader.">
        {report.singleUse.map((u) => (
          <div key={`${u.category}|${u.memoryId}`} className="lint-row col-row">
            <div className="row" style={{ width: '100%' }}>
              <div className="grow" style={{ minWidth: 0 }}>
                <span className="mono">{u.category}</span> <span className="faint">→</span> {truncate(u.summary, 110)}
              </div>
              <button className="btn sm" onClick={() => onShow([u.category])}>Show</button>
              <button className="btn sm" onClick={() => s.inspectMemory(u.memoryId)}>Open memory</button>
              <button className="btn sm" onClick={() => setOpenRow(openRow === `s${u.memoryId}` ? undefined : `s${u.memoryId}`)}>Move to…</button>
            </div>
            {openRow === `s${u.memoryId}` && filer(`s${u.memoryId}`, u.memoryId, 'Move')}
          </div>
        ))}
      </Group>

      <Group title="Weakly filed memories" count={report.weaklyFiled.length} hint="No confirmed placement: only keyword-guessed categories, or none. File them so recall finds them by topic.">
        {report.weaklyFiled.map((w) => (
          <div key={w.memoryId} className="lint-row col-row">
            <div className="row" style={{ width: '100%' }}>
              <div className="grow" style={{ minWidth: 0 }}>
                {truncate(w.summary, 130)}
                <div className="chips" style={{ marginTop: 3 }}>
                  <span className="faint">guessed:</span>
                  {w.paths.length === 0 && <span className="faint">none</span>}
                  {w.paths.map((p) => <span key={p} className="chip mono">{p}</span>)}
                </div>
              </div>
              <button className="btn sm" onClick={() => s.inspectMemory(w.memoryId)}>Open</button>
            </div>
            {filer(`w${w.memoryId}`, w.memoryId, 'File')}
          </div>
        ))}
      </Group>

      {hidden > 0 && <div className="faint" style={{ padding: '8px 14px' }}>+{hidden} more not shown (each list is capped at 25).</div>}

      {confirm && (
        <Confirm title="Merge categories?" confirmLabel={`Merge into ${confirm.to}`} onClose={() => setConfirm(undefined)} onConfirm={() => merge(confirm.from, confirm.to)}>
          Moves {confirm.moves} memory {confirm.moves === 1 ? 'link' : 'links'} from <span className="mono">{confirm.from}</span> into <span className="mono">{confirm.to}</span>, then removes <span className="mono">{confirm.from}</span>. Memories themselves are not changed.
        </Confirm>
      )}
      {mergeFrom && (
        <Modal title={`Merge ${mergeFrom}`} onClose={() => setMergeFrom(undefined)}>
          <div className="muted">Moves every link from <span className="mono">{mergeFrom}</span> into the category you pick, then removes it.</div>
          <PathInput autoFocus allowNew={false} exclude={[mergeFrom]} buttonLabel="Merge into" placeholder="Target category" label="Merge target" onPick={async (to) => { const from = mergeFrom; setMergeFrom(undefined); await merge(from, to); }} />
        </Modal>
      )}
    </div>
  );
}

/** Toolbar-sized icon for the Lint button. */
export const LintIcon = Sparkles;
