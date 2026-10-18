import { useMemo, useState } from 'react';
import { AlertTriangle, Plug, Radio, Webhook } from 'lucide-react';
import { defaultActivityFilters, distinct, filterActivity, type ActivityFilters } from '../lib/filters';
import { durationLabel, findMemoryId, plural, projectName, relTime } from '../lib/format';
import { useNow } from '../lib/hooks';
import { useStore } from '../lib/store';
import { Empty, Seg, Switch } from '../components/ui';


/**
 * Live feed of everything agents and hooks did: MCP tool calls and hook
 * events, with their duration and errors. This is where you verify hooks
 * are actually firing.
 */
export function ActivityView() {
  const s = useStore();
  const nowMs = useNow(15_000);
  const [f, setF] = useState<ActivityFilters>(defaultActivityFilters);
  const feed = s.activity;
  const rows = useMemo(() => filterActivity(feed, f), [feed, f]);
  const harnesses = useMemo(() => distinct(feed, (a) => a.harness), [feed]);
  const tools = useMemo(() => distinct(feed, (a) => a.tool), [feed]);
  const projects = useMemo(() => distinct(feed, (a) => a.project), [feed]);
  const hooksSeen = feed.some((a) => a.source === 'hook');
  const set = (p: Partial<ActivityFilters>) => setF((x) => ({ ...x, ...p }));

  return (
    <div className="view">
      <div className="toolbar">
        <span className="title serif">Activity</span>
        <span className="faint">{plural(rows.length, 'event')}{rows.length !== feed.length ? ` of ${feed.length}` : ''}, live</span>
        <span className="live-dot" aria-hidden />
        <span className="grow" />
        <Seg<ActivityFilters['source']>
          label="Source"
          value={f.source}
          onChange={(source) => set({ source })}
          options={[{ value: 'all', label: 'All' }, { value: 'mcp', label: 'MCP' }, { value: 'hook', label: 'Hooks' }, { value: 'ui', label: 'UI' }]}
        />
        <select className="input select" style={{ width: 130 }} aria-label="Harness" value={f.harness} onChange={(e) => set({ harness: e.target.value })}>
          <option value="">Any harness</option>
          {harnesses.map((h) => <option key={h} value={h}>{h}</option>)}
        </select>
        <select className="input select" style={{ width: 150 }} aria-label="Tool" value={f.tool} onChange={(e) => set({ tool: e.target.value })}>
          <option value="">Any tool</option>
          {tools.map((t) => <option key={t} value={t}>{t}</option>)}
        </select>
        <select className="input select" style={{ width: 140 }} aria-label="Project" value={f.project} onChange={(e) => set({ project: e.target.value })}>
          <option value="">Any project</option>
          {projects.map((p) => <option key={p} value={p}>{projectName(p)}</option>)}
        </select>
        <select className="input select" style={{ width: 120 }} aria-label="Minimum duration" value={f.minDuration} onChange={(e) => set({ minDuration: Number(e.target.value) })}>
          <option value={0}>Any duration</option>
          <option value={50}>&ge; 50 ms</option>
          <option value={250}>&ge; 250 ms</option>
          <option value={1000}>&ge; 1 s</option>
        </select>
        <label className="row switch-row"><Switch on={f.errorsOnly} onChange={(errorsOnly) => set({ errorsOnly })} label="Errors only" /> Errors only</label>
      </div>

      {feed.length === 0 ? (
        <Empty icon={<Radio size={28} />} title="No activity yet">
          Nothing has talked to MemTree since it started. Install hooks so your harness recalls memories on every prompt and records sessions, and connect MCP so agents can call the tools. Their calls show up here the moment they happen.
          <div className="row" style={{ justifyContent: 'center', marginTop: 14 }}>
            <button className="btn primary" onClick={() => s.openView('settings', { section: 'hooks' })}><Webhook size={14} /> Install hooks</button>
            <button className="btn" onClick={() => s.setConnectOpen(true)}><Plug size={14} /> Connect MCP</button>
          </div>
        </Empty>
      ) : (
        <div className="scroll grow">
          {!hooksSeen && (
            <div className="notice" style={{ margin: 12 }}>
              <AlertTriangle size={14} />
              <span className="grow">No hook events yet, only MCP calls. If you installed hooks, restart the harness and send a prompt.</span>
              <button className="btn sm" onClick={() => s.openView('settings', { section: 'hooks' })}>Check hooks</button>
            </div>
          )}
          <div className="act-table" role="table" aria-label="Activity feed">
            <div className="act-row head" role="row">
              <span>When</span><span>Source</span><span>Harness</span><span>Tool</span><span className="grow">Summary</span><span>Project</span><span>Took</span>
            </div>
            {rows.map((a) => {
              const id = findMemoryId(a.summary);
              return (
                <div key={`${a.id}-${a.time}`} role="row" className={`act-row ${a.error ? 'err' : ''} ${id ? 'click' : ''}`} onClick={() => id && (s.openView('memories'), s.inspectMemory(id))} title={id ? 'Open this memory' : undefined}>
                  <span className="faint" title={a.time}>{relTime(a.time, nowMs)}</span>
                  <span><span className={`badge ${a.source === 'hook' ? 'violet' : a.source === 'mcp' ? 'accent' : ''}`}>{a.source}</span></span>
                  <span className="ellipsis">{a.harness}</span>
                  <span className="mono tool ellipsis">{a.tool}</span>
                  <span className="grow ellipsis" title={a.error || a.summary}>{a.error ? <span style={{ color: 'var(--err)' }}>{a.error}</span> : a.summary}</span>
                  <span className="ellipsis faint" title={a.project}>{a.project ? projectName(a.project) : ''}</span>
                  <span className="faint mono">{durationLabel(a.durationMs)}</span>
                </div>
              );
            })}
            {rows.length === 0 && <div className="faint" style={{ padding: 16 }}>No events match these filters.</div>}
          </div>
        </div>
      )}
    </div>
  );
}
