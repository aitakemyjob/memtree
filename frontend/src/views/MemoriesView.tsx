import { useEffect, useMemo } from 'react';
import { Plus, Search, SearchX, X } from 'lucide-react';
import { api } from '../lib/api';
import { listArgs, recallArgs, sortMemories } from '../lib/filters';
import { useDebounced, useNow } from '../lib/hooks';
import { plural } from '../lib/format';
import { useStore } from '../lib/store';
import { MemoryCard } from '../components/MemoryCard';
import { Welcome } from '../components/Welcome';
import { Empty, ErrorBox, Spinner, useAsync } from '../components/ui';

/**
 * The default view: a filterable list of memory cards, or ranked search
 * results (with score decomposition) when the search box has text.
 */
export function MemoriesView() {
  const s = useStore();
  const { filters: f, query } = s;
  const nowMs = useNow();
  const dq = useDebounced(query.trim(), 220);
  const searching = dq.length > 0;
  const selected = s.inspect?.kind === 'memory' ? s.inspect.id : undefined;
  const listKey = JSON.stringify({ ...f, sort: '' });

  const list = useAsync(() => (searching ? Promise.resolve([]) : api.listMemories(listArgs(f))), [listKey, searching, s.versions.memory]);
  const found = useAsync(() => (searching ? api.recall(recallArgs(f, dq)) : Promise.resolve(undefined)), [dq, listKey, s.versions.memory]);

  // Cheap count of the captures the list hides, so the user knows where they went.
  const hiddenCaps = useAsync(
    () => (f.showCaptures ? Promise.resolve([]) : api.listMemories({ ...listArgs({ ...f, kind: 'capture', showCaptures: true, pinnedOnly: false }), limit: 1000 })),
    [listKey, s.versions.memory],
  );
  const hidden = searching ? 0 : (hiddenCaps.data ?? []).length;

  const { setRecallInfo } = s;
  useEffect(() => {
    if (found.data) setRecallInfo(found.data.memories);
  }, [found.data, setRecallInfo]);

  const sorted = useMemo(() => sortMemories(list.data ?? [], f.sort, nowMs), [list.data, f.sort, nowMs]);
  const loading = searching ? found.loading && !found.data : list.loading && !list.data;
  const error = searching ? found.error : list.error;
  const total = s.app?.stats.total ?? 0;
  const pristine = !searching && total === 0 && sorted.length === 0 && !list.loading;

  if (pristine) return <div className="view scroll"><Welcome /></div>;

  return (
    <div className="view">
      <div className="toolbar">
        <div className="search-box">
          <Search size={14} className="faint" />
          <input
            className="search-input"
            aria-label="Search memories"
            placeholder="Search memories by wording and by lattice category…"
            value={query}
            onChange={(e) => s.setQuery(e.target.value)}
          />
          {query && (
            <button className="btn ghost icon sm" aria-label="Clear search" onClick={() => s.setQuery('')}>
              <X size={13} />
            </button>
          )}
        </div>
        <span className="faint">
          {searching
            ? found.data ? `${plural(found.data.memories.length, 'result')}, ranked` : ''
            : `${plural(sorted.length, 'memory', 'memories')}`}
        </span>
        <span className="grow" />
        <button className="btn primary sm" onClick={() => s.newMemory()}>
          <Plus size={14} /> New memory
        </button>
      </div>

      {searching && found.data && found.data.categories.length > 0 && (
        <div className="fired-bar">
          <span className="faint">Lattice categories this query lit up:</span>
          {found.data.categories.map((c) => (
            <button key={c} className="chip mono clickable" onClick={() => s.inspectNode(c)}>{c}</button>
          ))}
        </div>
      )}

      {hidden > 0 && (
        <div className="fired-bar">
          <span className="faint">{plural(hidden, 'raw capture')} hidden.</span>
          <button className="link" onClick={() => s.openView('captures')}>View in Captures</button>
        </div>
      )}

      <div className="scroll grow mem-list" style={{ position: 'relative' }}>
        {loading && <Spinner />}
        <ErrorBox error={error} />
        {searching
          ? (found.data?.memories ?? []).map((r) => (
              <MemoryCard key={r.record.id} record={r.record} recall={r} selected={selected === r.record.id} onOpen={() => s.inspectMemory(r.record.id)} nowMs={nowMs} />
            ))
          : sorted.map((r) => <MemoryCard key={r.id} record={r} selected={selected === r.id} onOpen={() => s.inspectMemory(r.id)} nowMs={nowMs} />)}
        {!loading && !error && (searching ? (found.data?.memories.length ?? 0) === 0 : sorted.length === 0) && (
          <Empty icon={<SearchX size={28} />} title={searching ? 'Nothing matched' : 'No memories here'}>
            {searching
              ? 'Try different words, widen the project scope in the sidebar, or include superseded memories. Search matches wording and the lattice categories your words light up.'
              : 'No memories match the current filters. Reset the filters in the sidebar or create a memory.'}
          </Empty>
        )}
      </div>
    </div>
  );
}
