import { useState } from 'react';
import { ChevronRight } from 'lucide-react';
import { calendarSince } from '../lib/calendar';
import { plural, relTime, truncate } from '../lib/format';
import { useStore } from '../lib/store';
import { hitReason, originLabel, usesByMemory } from '../lib/traceExplain';
import { useDaily, useMemoryTrace } from '../lib/traces';
import { RetrievalCalendar } from './RetrievalCalendar';
import { UseBadges } from './TraceHitRow';
import { ErrorBox } from './ui';

const WEEKS = 13;

/**
 * A memory's retrieval history for the inspector: a per-day grid and each
 * time it was returned, by whom, for what query, why, and whether it was
 * then used. A row opens the full retrieval in the Traces view.
 */
export function MemoryTracePanel({ memoryId }: { memoryId: string }) {
  const s = useStore();
  const [nowMs] = useState(() => Date.now());
  const events = useMemoryTrace(memoryId, 30);
  const daily = useDaily({ memoryId, since: calendarSince(nowMs, WEEKS) });
  const list = events.data ?? [];
  if (s.app && !s.app.settings.traceRetrievals && !list.length) {
    return <div className="faint">Retrieval tracing is off (Settings › Memory tuning).</div>;
  }
  return (
    <div className="mem-trace">
      <RetrievalCalendar counts={daily.data ?? []} endMs={nowMs} weeks={WEEKS} label="This memory's retrievals per day" />
      <ErrorBox error={events.error} />
      {!events.loading && !list.length && <div className="faint">Not retrieved yet (within the retention window).</div>}
      {list.map((e) => {
        const hit = e.hits.find((h) => h.memoryId === memoryId);
        const uses = usesByMemory(e.uses).get(memoryId) ?? [];
        return (
          <button key={e.id} className="pick-row trace-row" onClick={() => s.openView('traces', { event: e.id })}>
            <div className="grow" style={{ minWidth: 0 }}>
              <div className="row">
                <span className="ellipsis">{originLabel(e)}</span>
                {hit && <span className="faint">#{hit.rank} of {e.hits.length}</span>}
                <UseBadges uses={uses} />
                <span className="grow" />
                <span className="faint" title={e.at}>{relTime(e.at, nowMs)}</span>
              </div>
              <div className="faint ellipsis">{e.query ? `“${truncate(e.query, 90)}”` : 'session start'}{hit ? ` · ${hitReason(hit)}` : ''}</div>
            </div>
            <ChevronRight size={12} />
          </button>
        );
      })}
      {list.length >= 30 && <div className="faint">Showing the latest {plural(30, 'retrieval')}.</div>}
    </div>
  );
}
