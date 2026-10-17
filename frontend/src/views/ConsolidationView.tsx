import { useState } from 'react';
import { ArrowRight, Combine, Layers } from 'lucide-react';
import { api } from '../lib/api';
import { memoryActivation } from '../lib/activation';
import { scopeArgs } from '../lib/filters';
import { ageShort, hoursLabel, plural, projectName, truncate } from '../lib/format';
import { useNow } from '../lib/hooks';
import { distilDraft, mergedKeywords } from '../lib/snippets';
import { useStore } from '../lib/store';
import type { Cluster, TierConfig } from '../lib/types';
import { Empty, ErrorBox, Field, Modal, PathEditor, Spinner, useAsync } from '../components/ui';

/**
 * Clusters of old, cooled, related memories that could be distilled into
 * one semantic memory, the thresholds that decide that, and a manual
 * "Distil" form. Agents do the same over MCP with consolidation_candidates
 * and memory_compress.
 */
export function ConsolidationView() {
  const s = useStore();
  const tiers = s.app?.settings.tiers;
  const nowMs = useNow();
  const scope = scopeArgs(s.filters.project);
  const data = useAsync(() => api.candidates(scope), [JSON.stringify(scope), s.versions.memory]);
  const [distil, setDistil] = useState<Cluster>();
  const clusters = data.data ?? [];

  return (
    <div className="view scroll">
      <div className="page">
        <h1>Consolidation</h1>
        <p className="muted lead">
          Memories fade. When a group of related memories has aged and cooled, they can be distilled into one lasting semantic memory; the originals are kept, marked superseded, and never deleted.
        </p>

        {tiers && <ThresholdDiagram tiers={tiers} />}

        <div className="card" style={{ marginBottom: 18 }}>
          <h3>Who does this</h3>
          <div className="muted">
            memtree runs no model, so it only proposes clusters. A connected agent calls <span className="mono">consolidation_candidates</span>, writes the gist, and calls <span className="mono">memory_compress</span> with it. You can do the same by hand below.
          </div>
        </div>

        <h2 className="sec">Candidates {data.data && <span className="faint">({clusters.length})</span>}</h2>
        <div style={{ position: 'relative', minHeight: 80 }}>
          {data.loading && !data.data && <Spinner />}
          <ErrorBox error={data.error} />
          {!data.loading && !data.error && clusters.length === 0 && tiers && (
            <Empty icon={<Layers size={28} />} title="Nothing qualifies yet">
              A memory becomes a candidate only when it is at least {hoursLabel(tiers.ageHoursToSemantic)} old (episodic) or {hoursLabel(tiers.ageHoursToCompact)} (semantic), its activation has fallen to {tiers.activationCeilingToCompress} or below, it is not pinned, and
              at least {tiers.minClusterSize} related memories (same project, overlapping keywords) qualify together. You have {s.app?.stats.episodic ?? 0} episodic memories; recently used or young ones stay out. Adjust the thresholds in Settings.
            </Empty>
          )}
          {clusters.map((c, i) => (
            <div key={i} className="card cluster">
              <div className="row" style={{ marginBottom: 8, flexWrap: 'wrap' }}>
                <b className="serif" style={{ fontSize: 15 }}>{plural(c.records.length, 'memory', 'memories')}</b>
                <span className="chip project">{projectName(c.project)}</span>
                <span className="grow" />
                <button className="btn primary sm" onClick={() => setDistil(c)}><Combine size={13} /> Distil…</button>
              </div>
              <div className="chips" style={{ marginBottom: 10 }}>
                <span className="faint">shared keywords</span>
                {(c.sharedKeywords ?? []).map((k) => <span key={k} className="chip">{k}</span>)}
                {(c.sharedKeywords ?? []).length === 0 && <span className="faint">none (linked by overlap)</span>}
              </div>
              {(c.records ?? []).map((r) => (
                <button key={r.id} className="pick-row" onClick={() => s.inspectMemory(r.id)}>
                  <span className="grow ellipsis">{truncate(r.summary.split('\n')[0], 120)}</span>
                  <span className="faint mono">act {memoryActivation(r, nowMs).toFixed(2)}</span>
                  <span className="faint">{ageShort(nowMs - Date.parse(r.createdAt))}</span>
                </button>
              ))}
            </div>
          ))}
        </div>
      </div>
      {distil && <DistilModal cluster={distil} onClose={() => setDistil(undefined)} />}
    </div>
  );
}

/** A small left-to-right picture of the tier rules, driven by the settings. */
function ThresholdDiagram({ tiers }: { tiers: TierConfig }) {
  const box = (title: string, sub: string, cls = '') => (
    <div className={`flow-box ${cls}`}>
      <b>{title}</b>
      <span className="faint">{sub}</span>
    </div>
  );
  const arrow = (label: string) => (
    <div className="flow-arrow">
      <span>{label}</span>
      <ArrowRight size={16} />
    </div>
  );
  return (
    <div className="card" style={{ marginBottom: 18 }} aria-label="Consolidation thresholds">
      <h3>How a memory ages</h3>
      <div className="flow-row">
        {box('Episodic', 'a specific event; decay 0.5')}
        {arrow(`older than ${hoursLabel(tiers.ageHoursToSemantic)} and activation ≤ ${tiers.activationCeilingToCompress}`)}
        {box('Candidate', `${tiers.minClusterSize}+ related memories`, 'hot')}
        {arrow('an agent (or you) distils')}
        {box('Semantic gist', 'decay 0.2; originals superseded')}
        {arrow(`after ${hoursLabel(tiers.ageHoursToCompact)}, same rules`)}
        {box('Candidate again', 'distil into a broader gist', 'hot')}
      </div>
      <div className="faint" style={{ marginTop: 10 }}>Pinned memories never compress. A distilled memory starts a fresh clock so it cannot be re-distilled immediately. Related means the same project and keyword overlap of 20% or more.</div>
    </div>
  );
}

function DistilModal({ cluster, onClose }: { cluster: Cluster; onClose: () => void }) {
  const s = useStore();
  const recs = cluster.records ?? [];
  const [gist, setGist] = useState(() => distilDraft(recs));
  const [kind, setKind] = useState('note');
  const [paths, setPaths] = useState<string[]>(() => [...new Set(recs.flatMap((r) => r.latticePaths ?? []))]);
  const [keywords] = useState(() => mergedKeywords(recs));
  const [busy, setBusy] = useState(false);
  const submit = async () => {
    setBusy(true);
    try {
      const saved = await api.compress(recs.map((r) => r.id), { summary: gist.trim(), kind, keywords, latticePaths: paths, provenance: 'HUMAN_CONFIRMED' });
      s.toast({ kind: 'ok', title: `Distilled ${recs.length} memories`, body: 'The originals are kept as superseded.' });
      s.bump({ memory: true, lattice: true });
      s.inspectMemory(saved.id);
      onClose();
    } catch (e) {
      s.fail('Could not distil', e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal
      wide
      title={`Distil ${plural(recs.length, 'memory', 'memories')}`}
      onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn primary" disabled={!gist.trim() || busy} onClick={submit}>Save gist and supersede originals</button>
        </>
      }
    >
      <div className="muted">Write one memory that keeps what matters. The gist is prefilled with the summaries; trim and rewrite it. It becomes a semantic memory with the highest importance and weakest provenance of the originals.</div>
      <Field label="Gist">
        <textarea className="input" aria-label="Gist" rows={9} value={gist} onChange={(e) => setGist(e.target.value)} />
      </Field>
      <div className="grid2">
        <Field label="Kind">
          <select className="input select" aria-label="Kind" value={kind} onChange={(e) => setKind(e.target.value)}>
            {['note', 'decision', 'lesson'].map((k) => <option key={k} value={k}>{k}</option>)}
          </select>
        </Field>
        <Field label="Keywords">
          <div className="chips">{keywords.map((k) => <span key={k} className="chip">{k}</span>)}</div>
        </Field>
      </div>
      <Field label="Lattice categories" hint="Prefilled with every category the originals were filed under.">
        <PathEditor paths={paths} onChange={setPaths} />
      </Field>
    </Modal>
  );
}
