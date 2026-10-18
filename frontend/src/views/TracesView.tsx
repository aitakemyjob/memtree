import { useEffect, useMemo, useState } from 'react';
import { Footprints } from 'lucide-react';
import { calendarSince } from '../lib/calendar';
import { ALL_PROJECTS } from '../lib/filters';
import { plural, projectName, relTime, truncate } from '../lib/format';
import { useMemoriesById } from '../lib/memories';
import { useStore } from '../lib/store';
import { originLabel, usesByMemory, wasUsed } from '../lib/traceExplain';
import { useDaily, useTraceEvent, useTraceEvents } from '../lib/traces';
import type { TraceEvent, TraceFilter } from '../lib/types';
import { MatchList, QueryHighlight } from '../components/QueryHighlight';
import { RetrievalCalendar } from '../components/RetrievalCalendar';
import { TraceHitRow } from '../components/TraceHitRow';
import { Empty, ErrorBox, Seg, Spinner, Switch } from '../components/ui';

type OriginFilter = 'agents' | 'mcp' | 'hook' | 'ui' | 'all';
const ORIGINS: Record<OriginFilter, string[] | undefined> = { agents: ['mcp', 'hook'], mcp: ['mcp'], hook: ['hook'], ui: ['ui', 'cli'], all: undefined };

/**
 * Retrieval traces: every time memory was searched or injected, newest
 * first, and for the selected one the query with the tokens that fired
 * categories, each returned memory with why it ranked where it did, and
 * whether the agent then used it.
 */
export function TracesView() {
  const s = useStore();
  const tab = s.tabs.find((t) => t.id === s.active);
  const requested = tab?.params.event as string | undefined;
  const [origin, setOrigin] = useState<OriginFilter>(requested ? 'all' : 'agents');
  const [quiet, setQuiet] = useState(false);
  const [sel, setSel] = useState<string | undefined>(requested);
  useEffect(() => {
    if (requested) setSel(requested);
  }, [requested]);
  const [nowMs] = useState(() => Date.now());

  const project = s.filters.project === ALL_PROJECTS ? undefined : s.filters.project;
  const filter: TraceFilter = { project, origins: ORIGINS[origin], quietOnly: quiet || undefined, limit: 200 };
  const events = useTraceEvents(filter);
  const daily = useDaily({ project, origins: ORIGINS[origin], since: calendarSince(nowMs) });
  const list = events.data ?? [];
  const current = sel ?? list[0]?.id;
  const pick = (id: string) => {
    setSel(id);
    if (tab) s.updateTab(tab.id, { event: id });
  };

  return (
    <div className="view">
      <div className="toolbar">
        <span className="title serif">Traces</span>
        <span className="faint">{plural(list.length, 'retrieval')}{project ? ` in ${projectName(project)}` : ''}</span>
        <span className="grow" />
        {s.app && !s.app.settings.traceRetrievals && <span className="badge warn" title="Enable in Settings > Memory tuning">tracing is off</span>}
        <Seg<OriginFilter>
          label="Asked by"
          value={origin}
          onChange={setOrigin}
          options={[
            { value: 'agents', label: 'Agents', title: 'MCP tools and hooks' },
            { value: 'mcp', label: 'MCP' },
            { value: 'hook', label: 'Hooks' },
            { value: 'ui', label: 'UI / CLI' },
            { value: 'all', label: 'All' },
          ]}
        />
        <Switch on={quiet} onChange={setQuiet} label="Quiet only" />
        <span className="faint">quiet only</span>
      </div>
      <div className="trace-cal-bar">
        <RetrievalCalendar counts={daily.data ?? []} endMs={nowMs} />
      </div>
      <ErrorBox error={events.error} />
      {events.loading && !events.data && <Spinner />}
      {!events.loading && list.length === 0 ? (
        <Empty icon={<Footprints size={28} />} title="No retrievals recorded">
          Each recall, hook injection and probe is recorded here with why every memory matched. Ask an agent something that touches earlier work, or search in Memories.
        </Empty>
      ) : (
        <div className="split">
          <div className="side" style={{ width: 340 }}>
            {list.map((e) => (
              <EventItem key={e.id} e={e} on={e.id === current} nowMs={nowMs} onPick={() => pick(e.id)} />
            ))}
          </div>
          <div className="main">{current && <EventDetail key={current} id={current} fallback={list.find((e) => e.id === current)} />}</div>
        </div>
      )}
    </div>
  );
}

/** One retrieval in the list. */
function EventItem({ e, on, nowMs, onPick }: { e: TraceEvent; on: boolean; nowMs: number; onPick: () => void }) {
  return (
    <button className={`list-item cap-item ${on ? 'on' : ''}`} style={{ width: '100%', border: 0, background: 'transparent', textAlign: 'left' }} onClick={onPick} aria-pressed={on}>
      <div className="grow" style={{ minWidth: 0 }}>
        <div className="title ellipsis">{e.query ? truncate(e.query, 70) : 'session start'}</div>
        <div className="sub row">
          <span className="ellipsis">{originLabel(e)}</span>
          <span className="grow" />
          {e.hits.length ? plural(e.hits.length, 'hit') : <span className="badge">quiet</span>}
          {wasUsed(e) && <span className="badge ok">used</span>}
          <span>{relTime(e.at, nowMs)}</span>
        </div>
      </div>
    </button>
  );
}

/** The selected retrieval: context, fired categories and ranked hits. */
function EventDetail({ id, fallback }: { id: string; fallback?: TraceEvent }) {
  const s = useStore();
  const ev = useTraceEvent(id);
  const e = ev.data ?? fallback;
  const ids = useMemo(() => e?.hits.map((h) => h.memoryId) ?? [], [e]);
  const mems = useMemoriesById(ids);
  if (!e) return ev.loading ? <Spinner /> : <ErrorBox error={ev.error ?? 'This retrieval is no longer recorded.'} />;
  const uses = usesByMemory(e.uses);
  return (
    <div className="scroll grow trace-detail">
      <div className="cap-head">
        <div className="row" style={{ flexWrap: 'wrap' }}>
          <b className="serif" style={{ fontSize: 15 }}>{originLabel(e)}</b>
          <span className="chip">{e.origin}</span>
          {e.reinforced && <span className="badge accent" title="The hits were reinforced as relied on">reinforced</span>}
          <span className="faint" title={e.at}>{relTime(e.at)} · {projectName(e.project)}</span>
        </div>
        <div className="trace-query"><QueryHighlight query={e.query} matches={e.matches} /></div>
        <MatchList matches={e.matches} />
        <div className="row faint" style={{ marginTop: 6, flexWrap: 'wrap' }}>
          <span className="mono" title="Trace id">{e.id}</span>
          {e.sessionId && (
            e.origin === 'hook'
              ? <button className="link" onClick={() => s.openView('captures')}>session {truncate(e.sessionId, 16)}</button>
              : <span className="mono" title="MCP session">session {truncate(e.sessionId, 16)}</span>
          )}
        </div>
      </div>
      <div style={{ padding: '8px 16px' }}>
        {e.hits.length === 0 && <div className="faint">Nothing qualified, so nothing was returned or injected.</div>}
        {e.hits.map((h) => (
          <TraceHitRow key={h.memoryId} hit={h} uses={uses.get(h.memoryId) ?? []} memory={mems.data?.get(h.memoryId)} onOpen={() => s.inspectMemory(h.memoryId)} />
        ))}
      </div>
    </div>
  );
}
