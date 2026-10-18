import { ChevronRight } from 'lucide-react';
import { truncate } from '../lib/format';
import { factorTerms, hitReason } from '../lib/traceExplain';
import type { MemoryRecord, TraceHit, TraceUse } from '../lib/types';
import { FactorChips } from './MemoryCard';

const ROUTE_BADGE: Record<string, string> = { both: 'ok', lattice: 'violet', anchor: 'accent' };

/** Badges for the evidence that a memory was used. */
export function UseBadges({ uses }: { uses: TraceUse[] }) {
  const kinds = [...new Set(uses.map((u) => u.evidence))];
  return (
    <>
      {kinds.map((k) => (
        <span key={k} className="badge ok" title={uses.filter((u) => u.evidence === k).map((u) => u.detail || k).join(', ')}>{k}</span>
      ))}
    </>
  );
}

/**
 * One memory a retrieval returned: rank, summary, how it was reached, the
 * score terms that multiply to its final score, and any use evidence.
 */
export function TraceHitRow({ hit, uses, memory, onOpen }: { hit: TraceHit; uses: TraceUse[]; memory?: MemoryRecord; onOpen: () => void }) {
  return (
    <div className="trace-hit">
      <button className="pick-row" onClick={onOpen} aria-label={`Open ${hit.memoryId}`}>
        <span className="mono faint">#{hit.rank}</span>
        <span className="grow ellipsis">{memory ? truncate(memory.summary.split('\n')[0], 110) : <span className="mono">{hit.memoryId}</span>}</span>
        <span className={`badge ${ROUTE_BADGE[hit.route] ?? ''}`}>{hit.route}</span>
        <UseBadges uses={uses} />
        <ChevronRight size={12} />
      </button>
      <div className="trace-why">
        <div className="muted">{hitReason(hit)}</div>
        {hit.route !== 'anchor' && <FactorChips terms={factorTerms(hit.factors)} final={hit.factors.final} />}
      </div>
    </div>
  );
}
