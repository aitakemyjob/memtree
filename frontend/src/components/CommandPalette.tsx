import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { MessageSquareText, Network, Palette, PanelRight, Plug, Plus, Search, Settings, Sun, Wrench } from 'lucide-react';
import { api } from '../lib/api';
import { recallArgs } from '../lib/filters';
import { firstLine, truncate } from '../lib/format';
import { useStore } from '../lib/store';
import { themes } from '../lib/themes';
import { VIEW_TITLES, type TabKind } from '../lib/tabs';
import type { Recalled } from '../lib/types';
import { VIEW_ICONS } from './ActivityBar';
import { fuzzyScore } from '../lib/palette';

interface Item {
  key: string;
  group: string;
  icon: ReactNode;
  label: string;
  sub?: string;
  run: () => void;
}

/**
 * Ctrl/Cmd K palette: switch views, create a memory, search memories (live
 * recall), probe the lattice with typed text, open Connect, flip or pick a
 * theme.
 */
export function CommandPalette() {
  const s = useStore();
  const [q, setQ] = useState('');
  const [sel, setSel] = useState(0);
  const [found, setFound] = useState<Recalled[]>([]);
  const listRef = useRef<HTMLDivElement>(null);
  const close = () => s.setPalette(false);
  const query = q.trim();

  useEffect(() => {
    if (query.length < 2) {
      setFound([]);
      return;
    }
    let alive = true;
    const t = setTimeout(() => {
      api.recall({ ...recallArgs({ ...s.filters, project: '*' }, query, 6), reinforce: false }).then(
        (r) => alive && setFound(r.memories),
        () => alive && setFound([]),
      );
    }, 160);
    return () => {
      alive = false;
      clearTimeout(t);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query]);

  const commands: Item[] = useMemo(
    () => [
      ...(Object.keys(VIEW_TITLES) as TabKind[]).map((k) => {
        const Icon = VIEW_ICONS[k];
        return { key: `v-${k}`, group: 'Go to', icon: <Icon size={14} />, label: VIEW_TITLES[k], run: () => s.openView(k) };
      }),
      { key: 'c-new', group: 'Commands', icon: <Plus size={14} />, label: 'New memory', run: () => s.newMemory() },
      { key: 'c-connect', group: 'Commands', icon: <Plug size={14} />, label: 'Connect an agent (MCP)', run: () => s.setConnectOpen(true) },
      { key: 'c-hooks', group: 'Commands', icon: <Wrench size={14} />, label: 'Install hooks', run: () => s.openView('settings', { section: 'hooks' }) },
      { key: 'c-settings', group: 'Commands', icon: <Settings size={14} />, label: 'Settings', run: () => s.openView('settings') },
      { key: 'c-inspector', group: 'Commands', icon: <PanelRight size={14} />, label: 'Toggle inspector', run: () => s.toggleInspector() },
      { key: 'c-toggle-theme', group: 'Commands', icon: <Sun size={14} />, label: 'Toggle light/dark theme', run: () => s.setTheme(s.theme.mode === 'dark' ? 'sfc-light' : 'sfc-dark') },
      ...themes.map((t) => ({ key: `t-${t.id}`, group: 'Themes', icon: <Palette size={14} />, label: `Theme: ${t.name}`, sub: t.mode, run: () => s.setTheme(t.id) })),
    ],
    [s],
  );

  const items: Item[] = useMemo(() => {
    const scored = commands
      .map((c) => ({ c, score: fuzzyScore(c.label, query) }))
      .filter((x) => x.score > 0)
      .sort((a, b) => b.score - a.score);
    // A typed command name ("toggle light") must win Enter over the generic
    // search/probe actions; fuzzy-only command matches come after them.
    const direct = query ? scored.filter((x) => x.c.label.toLowerCase().includes(query.toLowerCase())).slice(0, 5).map((x) => x.c) : [];
    const rest = scored.map((x) => x.c).filter((c) => !direct.includes(c)).slice(0, query ? 4 : 20);
    const actions: Item[] = [];
    if (query) {
      actions.push({
        key: 'search', group: 'Actions', icon: <Search size={14} />, label: `Search memories for “${truncate(query, 50)}”`,
        run: () => { s.setQuery(query); s.openView('memories'); },
      });
      actions.push({ key: 'probe', group: 'Actions', icon: <Network size={14} />, label: `Probe lattice with “${truncate(query, 50)}”`, sub: 'what would this prompt light up?', run: () => s.requestProbe(query) });
    }
    const mems: Item[] = found.map((r) => ({
      key: `m-${r.record.id}`, group: 'Memories', icon: <MessageSquareText size={14} />, label: truncate(firstLine(r.record.summary), 90), sub: r.score.final.toFixed(2),
      run: () => { s.openView('memories'); s.inspectMemory(r.record.id); },
    }));
    return [...direct, ...actions, ...mems, ...rest];
  }, [query, commands, found, s]);

  useEffect(() => setSel(0), [q]);
  useEffect(() => {
    listRef.current?.querySelector('.palette-item.on')?.scrollIntoView({ block: 'nearest' });
  }, [sel]);

  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSel((x) => Math.min(items.length - 1, x + 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSel((x) => Math.max(0, x - 1));
    } else if (e.key === 'Enter' && items[sel]) {
      items[sel].run();
      close();
    } else if (e.key === 'Escape') {
      close();
    }
  };

  let lastGroup = '';
  return (
    <div className="overlay" onMouseDown={(e) => e.target === e.currentTarget && close()}>
      <div className="modal" role="dialog" aria-modal="true" aria-label="Command palette" style={{ width: 'min(680px, 92vw)' }}>
        <input className="palette-input" autoFocus aria-label="Command or search" placeholder="Search memories, probe the lattice, jump to a view…" value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={onKey} />
        <div className="palette-list" ref={listRef}>
          {items.length === 0 && <div className="faint" style={{ padding: 14 }}>No matches.</div>}
          {items.map((it, i) => {
            const header = it.group !== lastGroup ? <div className="palette-group">{it.group}</div> : null;
            lastGroup = it.group;
            return (
              <div key={it.key}>
                {header}
                <div
                  className={`palette-item ${i === sel ? 'on' : ''}`}
                  onMouseEnter={() => setSel(i)}
                  onClick={() => {
                    it.run();
                    close();
                  }}
                >
                  <span className="pi-icon">{it.icon}</span>
                  <span className="ellipsis">{it.label}</span>
                  {it.sub && <span className="pi-sub ellipsis">{it.sub}</span>}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
