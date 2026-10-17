import { Palette, PanelRight, Plus, Search } from 'lucide-react';
import { useStore } from '../lib/store';
import { themes } from '../lib/themes';
import { Menu } from './Menu';

/**
 * Top bar: logo, command-palette omnibox, MCP status pill, new memory,
 * theme menu and inspector toggle. Hidden-inset on macOS (leaves room for
 * the traffic lights) and a drag region everywhere else.
 */
export function TitleBar() {
  const s = useStore();
  const isMac = s.app?.platform === 'darwin';
  const mod = isMac ? '⌘' : 'Ctrl';
  const mcp = s.app?.mcp;

  return (
    <header className={`titlebar ${isMac ? 'mac' : ''}`}>
      <div className="brand">
        <img className="brand-mark" src="./favicon.svg" alt="" draggable={false} />
        memtree
      </div>

      <div className="spacer" />

      <button className="omnibox" onClick={() => s.setPalette(true)} aria-label="Open command palette">
        <Search size={14} />
        <span>Search memories, probe the lattice, run commands…</span>
        <span className="kbd">{mod} K</span>
      </button>

      <div className="spacer" />

      <button className="btn sm" onClick={() => s.newMemory()} title="New memory">
        <Plus size={14} /> New memory
      </button>

      <button className="pill" onClick={() => s.setConnectOpen(true)} title={mcp?.error || mcp?.url || 'MCP server'} aria-label="Connect an agent over MCP">
        <span className={`dot ${mcp?.running ? 'ok' : mcp?.error ? 'err' : 'idle'}`} />
        MCP {mcp?.running ? 'live' : mcp?.error ? 'error' : 'off'}
      </button>

      <Menu
        align="right"
        width={220}
        trigger={() => (
          <button className="btn ghost icon" title="Theme" aria-label="Theme">
            <Palette size={16} />
          </button>
        )}
        items={themes.map((t) => ({
          key: t.id,
          label: t.name,
          active: t.id === s.theme.id,
          icon: <span style={{ width: 14, height: 14, borderRadius: 4, background: t.accent, display: 'block', border: '1px solid var(--border-strong)' }} />,
          hint: t.mode,
          onSelect: () => s.setTheme(t.id),
        }))}
      />
      <button className={`btn ghost icon ${s.inspectorOpen ? 'active' : ''}`} title={`Inspector (${mod}+I)`} aria-label="Toggle inspector" aria-pressed={s.inspectorOpen} onClick={() => s.toggleInspector()}>
        <PanelRight size={16} />
      </button>
    </header>
  );
}
