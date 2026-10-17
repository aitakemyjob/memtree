import { useCallback, useEffect, useState } from 'react';
import { Archive, ArchiveRestore, ChevronRight, GitMerge, Pin, PinOff, RefreshCw, Trash2, Undo2, X, Zap } from 'lucide-react';
import { api } from '../lib/api';
import { memoryActivation } from '../lib/activation';
import { canSave, draftFromRecord, emptyDraft, saveFromDraft, updateFromDraft } from '../lib/draft';
import { ALL_PROJECTS } from '../lib/filters';
import { childrenOf, subtreeLinkCount } from '../lib/latticeFlow';
import { fixed, hoursLabel, importanceName, plural, projectName, relTime, truncate } from '../lib/format';
import { useStore } from '../lib/store';
import type { MemoryDraft } from '../lib/types';
import { ActivationCurve } from './ActivationCurve';
import { ScoreChips } from './MemoryCard';
import { MemoryForm } from './MemoryForm';
import { SupersedePicker } from './SupersedePicker';
import { CopyButton, ErrorBox, Meter, Modal, Spinner, useAsync } from './ui';

/** Right pane: edits a memory, creates one, or shows a lattice category with its linked memories. */
export function Inspector() {
  const inspect = useStore((s) => s.inspect);
  const close = useStore((s) => s.closeInspector);
  const title = !inspect ? 'Inspector' : inspect.kind === 'memory' ? 'Memory' : inspect.kind === 'node' ? 'Category' : 'New memory';
  return (
    <aside className="inspector" aria-label="Inspector">
      <div className="panel-head">
        <span className="serif" style={{ textTransform: 'none', letterSpacing: 0, fontSize: 15, color: 'var(--text)' }}>{title}</span>
        <span className="actions">
          <button className="btn ghost icon sm" onClick={close} aria-label="Close inspector"><X size={14} /></button>
        </span>
      </div>
      {!inspect && (
        <div className="empty" style={{ padding: 24 }}>
          <div>
            <h2 style={{ fontSize: 16 }}>Nothing selected</h2>
            <div>Pick a memory in the list, a category in the lattice, or choose New memory.</div>
          </div>
        </div>
      )}
      {inspect?.kind === 'memory' && <MemoryInspector key={inspect.id} id={inspect.id} />}
      {inspect?.kind === 'node' && <NodeInspector key={inspect.id} id={inspect.id} />}
      {inspect?.kind === 'new' && <NewMemory key="new" seed={inspect.seed} />}
    </aside>
  );
}

/** Whether two drafts are identical. */
const sameDraft = (a: MemoryDraft, b: MemoryDraft) => JSON.stringify(a) === JSON.stringify(b);

function MemoryInspector({ id }: { id: string }) {
  const s = useStore();
  const ex = useAsync(() => api.explain(id), [id, s.versions.memory, s.versions.lattice]);
  const record = ex.data?.record;
  const lineage = useAsync(() => (record?.tier === 'semantic' ? api.lineage(id) : Promise.resolve([])), [id, record?.tier, s.versions.memory]);
  const [orig, setOrig] = useState<MemoryDraft>();
  const [draft, setDraft] = useState<MemoryDraft>();
  const [stale, setStale] = useState(false);
  const [busy, setBusy] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [supersede, setSupersede] = useState(false);
  const [nowMs] = useState(() => Date.now());

  // Adopt a freshly loaded record unless the user has unsaved edits, in
  // which case keep their draft and offer a reload (an agent may have
  // changed the memory underneath them).
  useEffect(() => {
    if (!record) return;
    const next = draftFromRecord(record);
    if (!orig || !draft || sameDraft(orig, draft)) {
      setOrig(next);
      setDraft(next);
      setStale(false);
    } else if (!sameDraft(orig, next)) {
      setStale(true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [record]);

  const refresh = useCallback(() => s.bump({ memory: true, lattice: true }), [s]);
  const act = async (label: string, fn: () => Promise<unknown>, okMsg?: string) => {
    setBusy(true);
    try {
      await fn();
      if (okMsg) s.toast({ kind: 'ok', title: okMsg });
      refresh();
    } catch (e) {
      s.fail(`Could not ${label}`, e);
    } finally {
      setBusy(false);
    }
  };

  if (ex.error && !record) return <ErrorBox error={ex.error} />;
  if (!record || !orig || !draft) return <div style={{ position: 'relative', flex: 1 }}><Spinner label="Loading memory…" /></div>;

  const update = updateFromDraft(id, orig, draft);
  const archived = !!record.archivedAt;
  const info = s.recallInfo[id];
  const ceiling = s.app?.settings.tiers;
  const activation = ex.data?.activation ?? memoryActivation(record, nowMs);

  return (
    <div className="inspector-body">
      <div className="insp-scroll">
        <div className="row" style={{ marginBottom: 8, flexWrap: 'wrap' }}>
          <span className="mono faint" title="Memory id">{record.id}</span>
          <CopyButton text={record.id} label="Copy id" className="" />
          <span className="grow" />
          <span className="faint">{record.harness} · {relTime(record.createdAt)}</span>
        </div>

        <div className="chips" style={{ marginBottom: 10 }}>
          <span className="chip">{record.tier}</span>
          <span className="chip">{importanceName(record.importance)}</span>
          <span className="chip project" title={record.project || 'Global'}>{projectName(record.project)}</span>
          {record.pinned && <span className="badge accent"><Pin size={10} /> pinned</span>}
          {archived && <span className="badge"><Archive size={10} /> archived</span>}
          {ex.data?.compressible && <span className="badge warn" title="Old and cool enough to be distilled">distillable</span>}
          {record.sessionId && <span className="chip mono" title="Session id">{truncate(record.sessionId, 18)}</span>}
        </div>

        {record.supersededBy && (
          <div className="notice warn">
            <GitMerge size={14} />
            <span className="grow">Superseded by <button className="link" onClick={() => s.inspectMemory(record.supersededBy!)}>{record.supersededBy}</button>. It no longer appears in recall.</span>
            <button className="btn sm" disabled={busy} onClick={() => act('undo supersede', () => api.unsupersede(id), 'Restored to recall')}><Undo2 size={12} /> Undo</button>
          </div>
        )}

        <div className="action-bar" role="toolbar" aria-label="Memory actions">
          <button className="btn sm" disabled={busy} onClick={() => act(record.pinned ? 'unpin' : 'pin', () => api.pin(id, !record.pinned))} title="Pinned memories never fade or compress">
            {record.pinned ? <PinOff size={13} /> : <Pin size={13} />} {record.pinned ? 'Unpin' : 'Pin'}
          </button>
          <button className="btn sm" disabled={busy} onClick={() => act('reinforce', () => api.touch([id]), 'Reinforced: activation refreshed')} title="Counts as a fresh access, so the memory stays warm">
            <Zap size={13} /> Reinforce
          </button>
          <button className="btn sm" disabled={busy} onClick={() => act(archived ? 'restore' : 'archive', () => (archived ? api.restore(id) : api.archive(id)), archived ? 'Restored' : 'Archived')}>
            {archived ? <ArchiveRestore size={13} /> : <Archive size={13} />} {archived ? 'Restore' : 'Archive'}
          </button>
          <button className="btn sm" disabled={busy || !!record.supersededBy} onClick={() => setSupersede(true)}>
            <GitMerge size={13} /> Supersede by…
          </button>
          <button className="btn sm danger" disabled={busy} onClick={() => setConfirmDelete(true)}>
            <Trash2 size={13} /> Delete
          </button>
        </div>

        {stale && (
          <div className="notice">
            <RefreshCw size={14} />
            <span className="grow">This memory changed elsewhere while you were editing.</span>
            <button className="btn sm" onClick={() => { const d = draftFromRecord(record); setOrig(d); setDraft(d); setStale(false); }}>Discard my edits</button>
          </div>
        )}

        <MemoryForm draft={draft} onChange={setDraft} />

        <h3 className="insp-h">Why this score</h3>
        <div className="why">
          {info ? (
            <>
              <div className="muted" style={{ marginBottom: 6 }}>From your last search ({info.source}{info.via ? ` via ${info.via}` : ''}):</div>
              <ScoreChips score={info.score} />
            </>
          ) : (
            <div className="muted">Search for something to see this memory's relevance. Its standing weight is:</div>
          )}
          <div className="formula mono" style={{ marginTop: 8 }}>
            importance {fixed(record.importance, 1)} × activation {fixed(activation)} = <b>{fixed(record.importance * activation)}</b>
          </div>
          <div className="faint" style={{ marginTop: 6 }}>
            final score = relevance × importance × activation × project boost (1.25 for the current project). Activation is ACT-R base-level decay over the access log ({record.accessLog.length} access{record.accessLog.length === 1 ? '' : 'es'}); age {hoursLabel(ex.data?.ageHours ?? 0)}.
          </div>
        </div>

        <h3 className="insp-h">Activation decay</h3>
        {ceiling && <ActivationCurve record={record} tiers={ceiling} nowMs={nowMs} />}

        {record.tier === 'semantic' && (
          <>
            <h3 className="insp-h">Distilled from</h3>
            {lineage.loading && !lineage.data && <div className="faint">Loading…</div>}
            {(lineage.data ?? []).length === 0 && !lineage.loading && <div className="faint">No source memories recorded.</div>}
            {(lineage.data ?? []).map((r) => (
              <button key={r.id} className="pick-row" onClick={() => s.inspectMemory(r.id)}>
                <span className="grow ellipsis">{truncate(r.summary.split('\n')[0], 90)}</span>
                <span className="faint">{relTime(r.createdAt)}</span>
                <ChevronRight size={12} />
              </button>
            ))}
          </>
        )}
      </div>

      <div className="insp-foot">
        <span className="faint grow">{update ? `${plural(Object.keys(update).filter((k) => k !== 'id' && k !== 'setPaths').length, 'change')} not saved` : 'No unsaved changes'}</span>
        <button className="btn sm" disabled={!update || busy} onClick={() => setDraft(orig)}>Revert</button>
        <button
          className="btn primary sm"
          disabled={!update || busy || !canSave(draft)}
          onClick={() => update && act('save changes', () => api.updateMemory(update), 'Saved')}
        >
          Save changes
        </button>
      </div>

      {supersede && <SupersedePicker id={id} onClose={() => setSupersede(false)} onPick={(by) => act('supersede', () => api.supersede(id, by), 'Marked as superseded')} />}
      {confirmDelete && (
        <Modal
          title="Delete this memory?"
          onClose={() => setConfirmDelete(false)}
          footer={
            <>
              <button className="btn" onClick={() => setConfirmDelete(false)}>Cancel</button>
              {!archived && (
                <button className="btn" onClick={() => { setConfirmDelete(false); void act('archive', () => api.archive(id), 'Archived'); }}>
                  Archive instead
                </button>
              )}
              <button
                className="btn danger"
                onClick={() => {
                  setConfirmDelete(false);
                  void act('delete', async () => {
                    await api.remove(id);
                    s.closeInspector();
                  }, 'Deleted');
                }}
              >
                Delete forever
              </button>
            </>
          }
        >
          <div>Deleting removes the memory and its category links for good.</div>
          <div className="muted">
            <b>Archive</b> is usually what you want: it hides the memory from recall but keeps it, and you can restore it at any time.
          </div>
        </Modal>
      )}
    </div>
  );
}

function NewMemory({ seed }: { seed?: Partial<MemoryDraft> }) {
  const s = useStore();
  const project = s.filters.project;
  const [draft, setDraft] = useState<MemoryDraft>(() => emptyDraft({ project: project === ALL_PROJECTS ? '' : project, ...seed }));
  const [busy, setBusy] = useState(false);
  const save = async () => {
    setBusy(true);
    try {
      const saved = await api.saveMemory(saveFromDraft(draft));
      s.toast({ kind: 'ok', title: 'Memory saved' });
      s.bump({ memory: true, lattice: true });
      s.inspectMemory(saved.id);
    } catch (e) {
      s.fail('Could not save the memory', e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="inspector-body">
      <div className="insp-scroll">
        <div className="muted" style={{ marginBottom: 12 }}>
          Saved memories are tagged as human-confirmed. Without categories a memory goes to the Unclassified queue and gets a provisional placement from its keywords.
        </div>
        <MemoryForm draft={draft} onChange={setDraft} isNew />
      </div>
      <div className="insp-foot">
        <span className="grow" />
        <button className="btn sm" onClick={() => s.closeInspector()}>Cancel</button>
        <button className="btn primary sm" disabled={!canSave(draft) || busy} onClick={save}>Save memory</button>
      </div>
    </div>
  );
}

function NodeInspector({ id }: { id: string }) {
  const s = useStore();
  const node = s.nodes.find((n) => n.id === id);
  const links = useAsync(() => api.links(id), [id, s.versions.lattice, s.versions.memory]);
  const kids = childrenOf(s.nodes, id);
  const crumbs = id.split('/');
  const sorted = [...(links.data ?? [])].sort((a, b) => b.weight - a.weight);
  return (
    <div className="inspector-body">
      <div className="insp-scroll">
        <div className="crumbs" style={{ marginBottom: 8, flexWrap: 'wrap' }}>
          {crumbs.map((c, i) => {
            const path = crumbs.slice(0, i + 1).join('/');
            return (
              <span key={path} className="row" style={{ gap: 4 }}>
                {i > 0 && <ChevronRight size={11} />}
                <button className="link mono" onClick={() => s.inspectNode(path)}>{c}</button>
              </span>
            );
          })}
        </div>
        {node ? (
          <dl className="kv" style={{ marginBottom: 14 }}>
            <dt>linked here</dt><dd>{node.linkCount}</dd>
            <dt>in subtree</dt><dd>{subtreeLinkCount(s.nodes, id)}</dd>
            <dt>activation</dt><dd><Meter value={(node.activation - 0.15) / 0.85} width={80} label="Node activation" /> {fixed(node.activation)}</dd>
            <dt>accessed</dt><dd>{node.accessCount}x, last {relTime(node.lastAccessed)}</dd>
            <dt>created</dt><dd>{relTime(node.createdAt)}</dd>
          </dl>
        ) : (
          <div className="faint">This category no longer exists.</div>
        )}
        {kids.length > 0 && (
          <>
            <h3 className="insp-h">Children</h3>
            <div className="chips" style={{ marginBottom: 12 }}>
              {kids.map((k) => <button key={k.id} className="chip mono clickable" onClick={() => s.inspectNode(k.id)}>{k.label} <span className="faint">{k.linkCount}</span></button>)}
            </div>
          </>
        )}
        <h3 className="insp-h">Linked memories</h3>
        {links.loading && !links.data && <div className="faint">Loading…</div>}
        <ErrorBox error={links.error} />
        {!links.loading && sorted.length === 0 && <div className="faint">Nothing is filed directly under this category.</div>}
        {sorted.map((l) => (
          <button key={`${l.itemType}:${l.itemId}`} className="link-row" onClick={() => s.inspectMemory(l.itemId)} title="Open memory">
            <div className="row">
              <Meter value={l.weight} color={l.source === 'human' ? 'var(--ok)' : l.source === 'agent' ? 'var(--accent-text)' : 'var(--text-3)'} width={44} label="Link weight" />
              <span className="mono faint">{fixed(l.weight)}</span>
              <span className={`badge ${l.source === 'human' ? 'ok' : l.source === 'agent' ? 'accent' : ''}`} title={l.source === 'auto' ? 'Placed by keyword matching; confirm it by tagging' : `Placed by ${l.source}`}>{l.source}</span>
              <span className="grow" />
              <span className="faint">{relTime(l.createdAt)}</span>
            </div>
            <div className="link-summary">{truncate(l.summary || l.itemId, 160)}</div>
          </button>
        ))}
      </div>
    </div>
  );
}
