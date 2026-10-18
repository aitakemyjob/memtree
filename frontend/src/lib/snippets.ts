// Copy-paste connection snippets for each MCP client and the prompts the
// UI hands to agents. Pure so they can be tested.
import type { CaptureSession, MemoryRecord } from './types';

export const CLIENTS = ['Claude Code', 'Cursor', 'VS Code', 'Codex', 'Claude Desktop', 'Other'] as const;
export type Client = (typeof CLIENTS)[number];

export interface Snippet {
  note: string;
  text: string;
  /** Preferred option for this client. */
  preferred?: boolean;
}

/** The MCP URL to show: the running server's, else the configured port. */
export function mcpUrl(running: string | undefined, port: number | undefined): string {
  return running || `http://127.0.0.1:${port || 47475}/mcp`;
}

/** Connection snippets per client, built around the real endpoint. */
export function connectSnippets(url: string): Record<Client, Snippet[]> {
  const stdioJson = JSON.stringify({ mcpServers: { memtree: { command: 'memtree', args: ['mcp'] } } }, null, 2);
  return {
    'Claude Code': [
      { note: 'Stdio bridge (preferred: works whether or not the window is open). Run once in your terminal:', text: 'claude mcp add --scope user memtree -- memtree mcp', preferred: true },
      { note: 'Or connect straight to the running app over HTTP:', text: `claude mcp add --transport http --scope user memtree ${url}` },
    ],
    Cursor: [{ note: 'Add to ~/.cursor/mcp.json:', text: JSON.stringify({ mcpServers: { memtree: { url } } }, null, 2) }],
    'VS Code': [{ note: 'Add to .vscode/mcp.json in the workspace:', text: JSON.stringify({ servers: { memtree: { type: 'http', url } } }, null, 2) }],
    Codex: [{ note: 'Add to ~/.codex/config.toml:', text: '[mcp_servers.memtree]\ncommand = "memtree"\nargs = ["mcp"]' }],
    'Claude Desktop': [{ note: 'Claude Desktop speaks stdio; MemTree bridges it to the running app. Add to claude_desktop_config.json:', text: stdioJson }],
    Other: [
      { note: 'Streamable HTTP endpoint:', text: url },
      { note: 'Or launch over stdio (bridges to the running app):', text: 'memtree mcp' },
    ],
  };
}

/** The prompt that asks an agent to summarise a captured session. */
export function summarisePrompt(s: Pick<CaptureSession, 'id' | 'project' | 'harness' | 'turns'>): string {
  return [
    `Use the memtree MCP server to summarise the captured session "${s.id}"${s.project ? ` (project ${s.project})` : ''}.`,
    '',
    `1. Call capture_transcript with session "${s.id}" and read all ${s.turns} turns.`,
    '2. Call lattice_browse so you reuse existing categories.',
    `3. Call memory_save with kind "session", session_id "${s.id}"${s.project ? `, project "${s.project}"` : ''}, a short summary of what was done and why,`,
    '   the decisions made, any open loops, importance (1 routine, 1.5 decision, 2.5 lesson) and lattice_paths.',
    'Keep it factual. Do not copy the transcript.',
  ].join('\n');
}

/** Prefill for the manual distil form: the summaries, newest first, as bullets. */
export function distilDraft(records: MemoryRecord[]): string {
  return records.map((r) => `- ${r.summary.trim().replace(/\n+/g, ' ')}`).join('\n');
}

/** Union of keywords across records, most frequent first, capped. */
export function mergedKeywords(records: MemoryRecord[], cap = 12): string[] {
  const count = new Map<string, number>();
  for (const r of records) for (const k of r.keywords) count.set(k, (count.get(k) ?? 0) + 1);
  return [...count].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).slice(0, cap).map(([k]) => k);
}
