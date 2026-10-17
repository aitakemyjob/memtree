import { MessagesSquare, Archive, Globe, FolderGit2, GitMerge, Layers, Pin, RotateCcw } from 'lucide-react';
import { ALL_PROJECTS, filtersActive, knownKinds, type SortKey } from '../lib/filters';
import { plural, projectName } from '../lib/format';
import { useStore } from '../lib/store';
import { Seg, Switch } from './ui';

/**
 * Left sidebar. The project scope applies to Memories, Timeline,
 * Consolidation and Captures; the memory filters (tier, kind, flags, sort)
 * only show on the Memories view, other views show the library summary.
 */
export function Sidebar() {
  const s = useStore();
  const kind = s.tabs.find((t) => t.id === s.active)?.kind;
  const projects = s.app?.projects ?? [];
  const stats = s.app?.stats;
  const globalCount = projects.find((p) => p.project === '')?.count ?? 0;
  const f = s.filters;
  const showFilters = kind === 'memories';
  const scoped = kind === 'memories' || kind === 'timeline' || kind === 'consolidation' || kind === 'captures';

  const row = (key: string, icon: React.ReactNode, label: string, count: number, title?: string) => (
    <button key={key} className={`tree-row ${f.project === key ? 'on' : ''}`} onClick={() => s.setFilters({ project: key })} title={title ?? label} aria-pressed={f.project === key} style={{ width: '100%', border: 0, background: 'transparent', textAlign: 'left' }}>
      <span className="chev" style={{ display: 'grid', placeItems: 'center' }}>{icon}</span>
      <span className="name grow">{label}</span>
      <span className="faint mono" style={{ fontSize: 11 }}>{count}</span>
    </button>
  );

  return (
    <aside className="sidebar" aria-label="Filters">
      <div className="panel-head">
        {showFilters ? 'Filters' : scoped ? 'Scope' : 'Library'}
        {showFilters && filtersActive(f) && (
          <span className="actions">
            <button className="btn ghost sm" onClick={() => s.resetFilters()} aria-label="Reset filters">
              <RotateCcw size={12} /> Reset
            </button>
          </span>
        )}
      </div>
      <div className="panel-body">
        {scoped && (
          <>
            <div className="section-title">Project</div>
            {row(ALL_PROJECTS, <Layers size={14} />, 'All projects', projects.reduce((n, p) => n + p.count, 0))}
            {row('', <Globe size={14} />, 'Global', globalCount, 'Memories that apply everywhere')}
            {projects.filter((p) => p.project !== '').map((p) => row(p.project, <FolderGit2 size={14} />, projectName(p.project), p.count, p.project))}
            {showFilters && !f.showCaptures && <div className="faint side-note">Project counts include raw captures, which the list hides.</div>}
            {f.project !== ALL_PROJECTS && f.project !== '' && <div className="faint side-note">Also includes global memories, as agents see them.</div>}
          </>
        )}

        {showFilters && (
          <>
            <div className="section-title">Tier</div>
            <div style={{ padding: '0 8px' }}>
              <Seg
                label="Tier"
                value={f.tier}
                onChange={(tier) => s.setFilters({ tier })}
                options={[
                  { value: '', label: 'All' },
                  { value: 'episodic', label: 'Episodic', title: 'Specific events and sessions; fade quickly' },
                  { value: 'semantic', label: 'Semantic', title: 'Distilled gist; fades slowly' },
                ]}
              />
            </div>
            <div className="section-title">Kind</div>
            <div style={{ padding: '0 8px' }}>
              <select className="input select" aria-label="Kind" value={f.kind} onChange={(e) => s.setFilters({ kind: e.target.value })}>
                <option value="">Any kind</option>
                {[...knownKinds([]), ...(f.kind && !knownKinds([]).includes(f.kind) ? [f.kind] : [])].map((k) => (
                  <option key={k} value={k}>{k}</option>
                ))}
              </select>
            </div>
            <div className="section-title">Sort</div>
            <div style={{ padding: '0 8px' }}>
              <select className="input select" aria-label="Sort" value={f.sort} onChange={(e) => s.setFilters({ sort: e.target.value as SortKey })}>
                <option value="newest">Newest first</option>
                <option value="oldest">Oldest first</option>
                <option value="importance">Importance</option>
                <option value="activation">Activation (how warm)</option>
              </select>
            </div>
            <div className="section-title">Show</div>
            <div className="col" style={{ padding: '0 8px', gap: 10 }}>
              <label className="row switch-row"><Switch on={f.pinnedOnly} onChange={(v) => s.setFilters({ pinnedOnly: v })} label="Pinned only" /><Pin size={13} className="faint" /> Pinned only</label>
              <label className="row switch-row"><Switch on={f.includeSuperseded} onChange={(v) => s.setFilters({ includeSuperseded: v })} label="Include superseded" /><GitMerge size={13} className="faint" /> Include superseded</label>
              <label className="row switch-row"><Switch on={f.showCaptures} onChange={(v) => s.setFilters({ showCaptures: v })} label="Raw captures" /><MessagesSquare size={13} className="faint" /> Raw captures</label>
              <label className="row switch-row"><Switch on={f.includeArchived} onChange={(v) => s.setFilters({ includeArchived: v })} label="Include archived" /><Archive size={13} className="faint" /> Include archived</label>
            </div>
          </>
        )}

        {!scoped && stats && (
          <div className="col" style={{ padding: '4px 8px', gap: 10 }}>
            <div className="stats">
              <div className="stat"><b>{stats.total}</b><span>memories</span></div>
              <div className="stat"><b>{stats.nodes}</b><span>categories</span></div>
            </div>
            <div className="faint">
              {plural(stats.episodic, 'episodic memory', 'episodic memories')}, {stats.semantic} semantic, {stats.pinned} pinned, {stats.superseded} superseded, {stats.archived} archived.
            </div>
          </div>
        )}
      </div>
    </aside>
  );
}
