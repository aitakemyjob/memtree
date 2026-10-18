import { useState } from 'react';
import { ShieldCheck } from 'lucide-react';
import { CLIENTS, connectSnippets, mcpUrl, type Client } from '../lib/snippets';
import { useStore } from '../lib/store';
import { Seg, Snippet } from './ui';

/**
 * MCP connection snippets for each client, built from the live endpoint.
 * Used by the Connect modal and Settings > Connect.
 */
export function ConnectPanel() {
  const s = useStore();
  const [client, setClient] = useState<Client>('Claude Code');
  const mcp = s.app?.mcp;
  const url = mcpUrl(mcp?.url, s.app?.settings.mcpPort);
  const snippets = connectSnippets(url)[client];
  return (
    <div className="col" style={{ gap: 14 }}>
      <div className="row">
        <span className={`dot ${mcp?.running ? 'ok' : 'err'}`} />
        <div className="grow">
          {mcp?.running ? (
            <>MCP server is live at <span className="mono">{url}</span></>
          ) : (
            <span style={{ color: 'var(--err)' }}>{mcp?.error || 'The MCP server is not running.'} Change the port in Settings and restart MemTree. The stdio bridge below still works once it is running.</span>
          )}
        </div>
      </div>
      <Seg<Client> label="Client" value={client} onChange={setClient} options={CLIENTS.map((c) => ({ value: c, label: c }))} />
      {snippets.map((sn, i) => (
        <div key={i} className="col" style={{ gap: 6 }}>
          <div className="muted">{sn.note}{sn.preferred && <span className="badge accent" style={{ marginLeft: 8 }}>preferred</span>}</div>
          <Snippet text={sn.text} />
        </div>
      ))}
      <div className="row muted" style={{ alignItems: 'flex-start' }}>
        <ShieldCheck size={16} style={{ color: 'var(--ok)', flex: 'none', marginTop: 2 }} />
        <span>
          MemTree listens on <span className="mono">127.0.0.1</span> only. It runs no model: agents summarise and classify, MemTree stores and ranks.
          Turn <b>Agent writes</b> off in Settings for a read-only recall setup.
        </span>
      </div>
    </div>
  );
}
