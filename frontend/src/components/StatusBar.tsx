import { Brain, Layers, Pin, Plug, Shield, ShieldOff, Tag } from 'lucide-react';
import { plural } from '../lib/format';
import { useStore } from '../lib/store';

/**
 * Bottom bar: MCP status and URL (green/red), memory counts, agent-write
 * state and the app version.
 */
export function StatusBar() {
  const s = useStore();
  const mcp = s.app?.mcp;
  const st = s.app?.stats;
  const writes = s.app?.settings.agentWrites ?? true;
  return (
    <footer className="statusbar">
      <span className="item click" onClick={() => s.setConnectOpen(true)} title={mcp?.error ?? 'Agent endpoint: click for connection snippets'} role="button" tabIndex={0} onKeyDown={(e) => e.key === 'Enter' && s.setConnectOpen(true)}>
        <span className={`dot ${mcp?.running ? 'ok' : mcp?.error ? 'err' : 'idle'}`} />
        <Plug size={12} />
        {mcp?.running ? <span className="mono">{mcp.url}</span> : mcp?.error ? <span style={{ color: 'var(--err)' }}>MCP error: {mcp.error}</span> : 'MCP off'}
      </span>
      {st && (
        <>
          <span className="item" title="Live memories (not archived or superseded)"><Brain size={12} /> {plural(Math.max(0, st.total - st.superseded - st.archived), 'memory', 'memories')}</span>
          <span className="item" title="Pinned memories"><Pin size={12} /> {st.pinned}</span>
          <span className="item click" title="Lattice categories" onClick={() => s.openView('lattice')}><Layers size={12} /> {plural(st.nodes, 'category', 'categories')}</span>
          <span className="item click" title="Memories waiting for a category" onClick={() => s.openView('lattice')}><Tag size={12} /> {st.untagged} unclassified</span>
          <span className="item click" title="Captured sessions waiting for a summary" onClick={() => s.openView('captures')}>{plural(st.capturesPending, 'capture')} pending</span>
        </>
      )}
      <div className="right">
        <span className="item click" onClick={() => s.openView('settings', { section: 'agents' })} title={writes ? 'Agents can save, supersede and archive memories' : 'Agents are read-only: they can recall but not write'}>
          {writes ? <Shield size={12} /> : <ShieldOff size={12} style={{ color: 'var(--warn)' }} />}
          {writes ? 'agent writes on' : <span style={{ color: 'var(--warn)' }}>agents read-only</span>}
        </span>
        <span className="item">memtree {s.app?.version ? (s.app.version.startsWith('v') ? s.app.version : `v${s.app.version}`) : ''}</span>
      </div>
    </footer>
  );
}
