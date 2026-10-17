import { describe, expect, it } from 'vitest';
import { CLIENTS, connectSnippets, distilDraft, mcpUrl, mergedKeywords, summarisePrompt } from './snippets';
import type { MemoryRecord } from './types';

describe('connectSnippets', () => {
  const url = 'http://127.0.0.1:47493/mcp';
  const s = connectSnippets(url);
  it('covers every client with at least one snippet', () => {
    for (const c of CLIENTS) expect(s[c].length).toBeGreaterThan(0);
  });
  it('prefers the stdio bridge for Claude Code and offers HTTP as the alternative', () => {
    expect(s['Claude Code'][0].text).toBe('claude mcp add --scope user memtree -- memtree mcp');
    expect(s['Claude Code'][0].preferred).toBe(true);
    expect(s['Claude Code'][1].text).toContain(url);
  });
  it('writes valid JSON for Cursor and VS Code using the real endpoint', () => {
    expect(JSON.parse(s.Cursor[0].text)).toEqual({ mcpServers: { memtree: { url } } });
    expect(JSON.parse(s['VS Code'][0].text)).toEqual({ servers: { memtree: { type: 'http', url } } });
    expect(JSON.parse(s['Claude Desktop'][0].text).mcpServers.memtree).toEqual({ command: 'memtree', args: ['mcp'] });
  });
  it('writes TOML for Codex', () => {
    expect(s.Codex[0].text).toContain('[mcp_servers.memtree]');
    expect(s.Codex[0].text).toContain('args = ["mcp"]');
  });
});

describe('mcpUrl', () => {
  it('prefers the live URL and falls back to the configured port', () => {
    expect(mcpUrl('http://x/mcp', 1)).toBe('http://x/mcp');
    expect(mcpUrl('', 5000)).toBe('http://127.0.0.1:5000/mcp');
    expect(mcpUrl(undefined, undefined)).toBe('http://127.0.0.1:47475/mcp');
  });
});

describe('summarisePrompt', () => {
  it('names the session, tools and the kind to save', () => {
    const p = summarisePrompt({ id: 'sess-1', project: '/code/app', harness: 'claude-code', turns: 12 });
    expect(p).toContain('capture_transcript');
    expect(p).toContain('memory_save');
    expect(p).toContain('kind "session"');
    expect(p).toContain('session_id "sess-1"');
    expect(p).toContain('12 turns');
    expect(p).toContain('/code/app');
  });
  it('omits the project clause for global sessions', () => {
    expect(summarisePrompt({ id: 's', project: '', harness: 'codex', turns: 1 })).not.toContain('project "');
  });
});

describe('distil helpers', () => {
  const r = (summary: string, keywords: string[]) => ({ summary, keywords }) as MemoryRecord;
  it('builds a bullet draft with flattened lines', () => {
    expect(distilDraft([r('one\n\ntwo', []), r('three', [])])).toBe('- one two\n- three');
  });
  it('merges keywords by frequency', () => {
    expect(mergedKeywords([r('', ['a', 'b']), r('', ['b', 'c']), r('', ['b', 'a'])], 2)).toEqual(['b', 'a']);
  });
});
