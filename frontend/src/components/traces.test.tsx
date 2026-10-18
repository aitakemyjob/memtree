import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useStore } from '../lib/store';
import { setTransport, type Transport } from '../lib/transport';
import type { AppState, MemoryRecord, TraceEvent } from '../lib/types';
import { TracesView } from '../views/TracesView';
import { MemoryTracePanel } from './MemoryTracePanel';
import { RetrievalCalendar } from './RetrievalCalendar';

const app = {
  version: '0.1.0', platform: 'linux', mcp: { running: true, url: '' }, projects: [], axisRoots: [], configPath: '', canPickDir: false,
  settings: { traceRetrievals: true, traceRetentionDays: 90 },
  stats: { total: 1, episodic: 1, semantic: 0, pinned: 0, superseded: 0, archived: 0, untagged: 0, nodes: 0, links: 0, capturesPending: 0 },
} as unknown as AppState;

const now = new Date().toISOString();
const mem = (id: string, summary: string) => ({ id, summary, project: '/p', kind: 'note' }) as MemoryRecord;
const event: TraceEvent = {
  id: 'ret_1', at: now, origin: 'mcp', tool: 'recall', harness: 'claude-code', sessionId: 'mcp-1', project: '/p', query: 'how does Redis evict keys', reinforced: true,
  matches: [{ token: 'redis', category: 'knowledge/tools/redis' }],
  hits: [
    { memoryId: 'mem_a', rank: 1, route: 'both', via: 'knowledge/tools/redis', seed: 'knowledge/tools/redis', depth: 0, factors: { text: 1, link: 0.4, importance: 1.5, activation: 0.8, projectBoost: 1.25, bothBonus: 1.15, final: 1.725 } },
    { memoryId: 'mem_b', rank: 2, route: 'text', depth: 0, factors: { text: 0.5, link: 0, importance: 1, activation: 1, projectBoost: 1, bothBonus: 1, final: 0.5 } },
  ],
  uses: [{ eventId: 'ret_1', memoryId: 'mem_a', evidence: 'fetched', detail: 'memory_get', at: now }],
};
const quiet: TraceEvent = { ...event, id: 'ret_2', origin: 'hook', tool: 'UserPromptSubmit', query: 'thanks', matches: [], hits: [], uses: [], reinforced: false };

function transport(): Transport & { calls: { method: string; args: unknown }[] } {
  const handlers: Record<string, (a: unknown) => unknown> = {
    'trace.list': () => [event, quiet],
    'trace.get': (a) => ((a as { id: string }).id === 'ret_2' ? quiet : event),
    'trace.forMemory': () => [event],
    'trace.daily': () => [{ day: now.slice(0, 10), retrieved: 3, used: 1 }],
    'memory.get': (a) => {
      const id = (a as { id: string }).id;
      if (id === 'mem_a') return mem('mem_a', 'Use allkeys-lru eviction');
      throw new Error('not found');
    },
  };
  const t = {
    kind: 'browser' as const, calls: [] as { method: string; args: unknown }[],
    async invoke(method: string, args?: unknown) {
      t.calls.push({ method, args });
      const h = handlers[method];
      if (!h) throw new Error(`unexpected ${method}`);
      return h(args) as never;
    },
    subscribe: () => () => {},
  };
  return t;
}

beforeEach(() => {
  useStore.setState({ ...useStore.getInitialState(), app, ready: true }, true);
});
afterEach(() => {
  cleanup();
  setTransport(undefined);
});

describe('TracesView', () => {
  it('lists retrievals and explains the selected one', async () => {
    const t = transport();
    setTransport(t);
    useStore.getState().openView('traces');
    render(<TracesView />);
    expect(await screen.findByText('Use allkeys-lru eviction')).toBeInTheDocument();
    expect(screen.getByText('Redis', { selector: 'mark' })).toHaveAttribute('title', 'fired knowledge/tools/redis');
    expect(screen.getByText('redis → knowledge/tools/redis')).toBeInTheDocument();
    expect(screen.getByText('matched the wording and filed under knowledge/tools/redis')).toBeInTheDocument();
    expect(screen.getByText('fetched')).toBeInTheDocument();
    expect(screen.getByText('mem_b')).toBeInTheDocument(); // a deleted memory still shows by id
    expect(screen.getAllByLabelText('Score decomposition')[0]).toHaveTextContent(/both 1\.15\s*=\s*1\.73/);
    const list = t.calls.find((c) => c.method === 'trace.list')!.args as { origins: string[] };
    expect(list.origins).toEqual(['mcp', 'hook']);

    await userEvent.click(screen.getByRole('button', { name: /thanks/ }));
    expect(await screen.findByText(/Nothing qualified/)).toBeInTheDocument();
    await waitFor(() => expect(useStore.getState().tabs[0].params.event).toBe('ret_2'));
  });
});

describe('MemoryTracePanel', () => {
  it('shows when and why a memory was retrieved and opens the trace', async () => {
    setTransport(transport());
    useStore.getState().openView('memories');
    render(<MemoryTracePanel memoryId="mem_a" />);
    const row = await screen.findByRole('button', { name: /claude-code · recall/ });
    expect(within(row).getByText('#1 of 2')).toBeInTheDocument();
    expect(within(row).getByText('fetched')).toBeInTheDocument();
    expect(row).toHaveTextContent('“how does Redis evict keys”');
    await userEvent.click(row);
    const tab = useStore.getState().tabs.find((x) => x.kind === 'traces');
    expect(tab?.params.event).toBe('ret_1');
  });
});

describe('RetrievalCalendar', () => {
  it('draws one cell per elapsed day and summarises the total', () => {
    const end = Date.parse('2026-03-04T12:00:00Z');
    const { container } = render(<RetrievalCalendar counts={[{ day: '2026-03-04', retrieved: 2, used: 1 }]} endMs={end} weeks={2} />);
    expect(container.querySelectorAll('rect')).toHaveLength(11); // 7 + Sun..Wed
    expect(container.querySelectorAll('circle')).toHaveLength(1);
    expect(screen.getByText(/2 retrievals in the last 2 weeks/)).toBeInTheDocument();
  });
});
