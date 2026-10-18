import { Archive, GitMerge, Pin } from 'lucide-react';
import { memoryActivation } from '../lib/activation';
import { fixed, importanceName, kindColor, projectName, relTime, truncate } from '../lib/format';
import { factorsFromScore, factorTerms, type FactorTerm } from '../lib/traceExplain';
import type { MemoryRecord, Recalled } from '../lib/types';
import { Meter } from './ui';

/** Labelled score terms joined by ×, ending in their product. */
export function FactorChips({ terms, final }: { terms: FactorTerm[]; final: number }) {
  return (
    <div className="score-chips" aria-label="Score decomposition">
      {terms.map((t, i) => (
        <span key={t.label} className="row" style={{ gap: 4 }}>
          {i > 0 && <span className="op">×</span>}
          <span className="chip" title={t.title}>{t.label} {t.value}</span>
        </span>
      ))}
      <span className="op">=</span>
      <span className="chip strong">{fixed(final)}</span>
    </div>
  );
}

/** The factors of a recall score and their product. */
export function ScoreChips({ score }: { score: Recalled['score'] }) {
  return <FactorChips terms={factorTerms(factorsFromScore(score))} final={score.final} />;
}

/**
 * One memory in a list: kind/tier/importance chips, status badges, age,
 * summary, lattice paths and an activation bar. When it came from a search
 * it also shows the score decomposition, source and the category that
 * surfaced it.
 */
export function MemoryCard({ record, selected, onOpen, recall, nowMs }: { record: MemoryRecord; selected: boolean; onOpen: () => void; recall?: Recalled; nowMs: number }) {
  const act = memoryActivation(record, nowMs);
  const paths = record.latticePaths ?? [];
  return (
    <div
      className={`mem-card ${selected ? 'on' : ''} ${record.archivedAt || record.supersededBy ? 'faded' : ''}`}
      role="button"
      tabIndex={0}
      aria-pressed={selected}
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onOpen();
        }
      }}
    >
      <div className="row mem-meta">
        <span className="chip" style={{ color: kindColor(record.kind) }}>{record.kind}</span>
        <span className="chip">{record.tier}</span>
        <span className={`chip imp imp-${importanceName(record.importance)}`} title={`Importance: ${importanceName(record.importance)}`}>importance {record.importance}</span>
        {record.pinned && <span className="badge accent"><Pin size={10} /> pinned</span>}
        {record.supersededBy && <span className="badge warn"><GitMerge size={10} /> superseded</span>}
        {record.archivedAt && <span className="badge"><Archive size={10} /> archived</span>}
        <span className="grow" />
        <span className="faint" title={record.createdAt}>{relTime(record.createdAt, nowMs)}</span>
      </div>
      <div className="mem-summary">{truncate(record.summary, 360)}</div>
      <div className="row mem-foot">
        <span className="chip project" title={record.project || 'Global'}>{projectName(record.project)}</span>
        {paths.slice(0, 4).map((p) => <span key={p} className="chip mono">{p}</span>)}
        {paths.length > 4 && <span className="faint">+{paths.length - 4}</span>}
        <span className="grow" />
        <span className="act" title={`Activation ${act.toFixed(2)}`}>
          <Meter value={(act - 0.15) / 0.85} color="var(--accent-text)" label="Activation" width={48} />
          <span className="faint mono">{act.toFixed(2)}</span>
        </span>
      </div>
      {recall && (
        <div className="recall-info">
          <ScoreChips score={recall.score} />
          <div className="row" style={{ marginTop: 4 }}>
            <span className={`badge ${recall.source === 'both' ? 'ok' : recall.source === 'lattice' ? 'violet' : ''}`}>{recall.source}</span>
            {recall.via && <span className="faint">via <span className="mono">{recall.via}</span></span>}
          </div>
        </div>
      )}
    </div>
  );
}
