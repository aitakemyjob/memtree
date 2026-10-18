import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { REFRESH_DEBOUNCE_MS, resetEventDebounce, useStore } from './store';
import { setTransport, type Transport } from './transport';
import type { Activity, AppState, BusEvent, Settings } from './types';

const settings: Settings = {
  theme: 'sfc-dark', fontSize: 13, mcpPort: 47475, dbPath: '', agentWrites: true, captureHooks: true, surfaceThreshold: 0.25, maxSurface: 3,
  linkHalfLifeDays: 30, maintainHours: 12, traceRetrievals: true, traceRetentionDays: 90, tiers: { ageHoursToSemantic: 72, ageHoursToCompact: 720, activationCeilingToCompress: 0.5, minClusterSize: 2 },
};
const appState = (over: Partial<AppState> = {}): AppState => ({
  version: '0.1.0', platform: 'linux', settings, mcp: { running: true, url: 'http://127.0.0.1:47475/mcp' },
  stats: { total: 0, episodic: 0, semantic: 0, pinned: 0, superseded: 0, archived: 0, untagged: 0, nodes: 0, links: 0, capturesPending: 0 },
  projects: [], axisRoots: ['project'], configPath: '/c.json', canPickDir: false, ...over,
});

interface Fake extends Transport {
  calls: { method: string; args: unknown }[];
  handlers: Record<string, (args: unknown) => unknown>;
  emit(ev: BusEvent): void;
}

function fake(handlers: Fake['handlers'] = {}): Fake {
  let listener: ((ev: BusEvent) => void) | undefined;
  const t: Fake = {
    kind: 'browser', calls: [], handlers,
    async invoke(method, args) {
      t.calls.push({ method, args });
      const h = t.handlers[method];
      if (!h) throw new Error(`unexpected ${method}`);
      return h(args) as never;
    },
    subscribe(fn) {
      listener = fn;
      return () => (listener = undefined);
    },
    emit(ev) {
      listener?.(ev);
    },
  };
  return t;
}

const base = () => ({ 'app.state': () => appState(), 'activity.list': () => [], 'captures.list': () => [{ id: 'old-session' }] });

beforeEach(() => {
  useStore.setState(useStore.getInitialState(), true);
  resetEventDebounce();
});
afterEach(() => {
  setTransport(undefined);
  vi.useRealTimers();
});

describe('init', () => {
  it('loads state, applies the theme, seeds known sessions and opens Memories', async () => {
    const t = fake({ ...base(), 'app.state': () => appState({ settings: { ...settings, theme: 'sfc-light' } }) });
    setTransport(t);
    await useStore.getState().init();
    const s = useStore.getState();
    expect(s.ready).toBe(true);
    expect(s.fatal).toBeUndefined();
    expect(s.theme.id).toBe('sfc-light');
    expect(document.documentElement.dataset.theme).toBe('sfc-light');
    expect(s.tabs.map((x) => x.kind)).toEqual(['memories']);
    expect(s.knownSessions.has('old-session')).toBe(true);
  });
  it('reports a backend that cannot be reached instead of throwing', async () => {
    setTransport(fake({ 'app.state': () => { throw new Error('connection refused'); } }));
    await useStore.getState().init();
    expect(useStore.getState().fatal).toBe('connection refused');
    expect(useStore.getState().ready).toBe(true);
  });
  it('tolerates null lists from the wire', async () => {
    setTransport(fake({ ...base(), 'activity.list': () => null, 'captures.list': () => null }));
    await useStore.getState().init();
    expect(useStore.getState().activity).toEqual([]);
    expect(useStore.getState().ready).toBe(true);
  });
});

describe('saveSettings', () => {
  it('sends the WHOLE settings object and keeps the saved result', async () => {
    const t = fake({ ...base(), 'settings.update': (a) => a });
    setTransport(t);
    await useStore.getState().init();
    const ok = await useStore.getState().saveSettings({ ...settings, agentWrites: false });
    expect(ok).toBe(true);
    const call = t.calls.find((c) => c.method === 'settings.update')!;
    expect(call.args).toEqual({ ...settings, agentWrites: false });
    expect(useStore.getState().app!.settings.agentWrites).toBe(false);
  });
  it('reverts the optimistic change and toasts when the backend refuses', async () => {
    const t = fake({ ...base(), 'settings.update': () => { throw new Error('disk full'); } });
    setTransport(t);
    await useStore.getState().init();
    const ok = await useStore.getState().saveSettings({ ...settings, theme: 'nord' });
    expect(ok).toBe(false);
    expect(useStore.getState().app!.settings.theme).toBe('sfc-dark');
    expect(useStore.getState().theme.id).toBe('sfc-dark');
    expect(useStore.getState().toasts.at(-1)).toMatchObject({ kind: 'err', body: 'disk full' });
  });
  it('setTheme applies immediately and persists the whole object', async () => {
    const t = fake({ ...base(), 'settings.update': (a) => a });
    setTransport(t);
    await useStore.getState().init();
    await useStore.getState().setTheme('nord');
    expect(document.documentElement.dataset.theme).toBe('nord');
    expect((t.calls.find((c) => c.method === 'settings.update')!.args as Settings).theme).toBe('nord');
    expect((t.calls.find((c) => c.method === 'settings.update')!.args as Settings).tiers.minClusterSize).toBe(2);
  });
});

describe('handleEvent', () => {
  const activity = (id: number, over: Partial<Activity> = {}): Activity => ({ id, time: new Date(1_700_000_000_000 + id * 1000).toISOString(), source: 'mcp', harness: 'claude-code', tool: 'recall', summary: 'q', project: '', durationMs: 1, ...over });

  it('appends activity capped at 300', async () => {
    setTransport(fake(base()));
    await useStore.getState().init();
    for (let i = 1; i <= 305; i++) useStore.getState().handleEvent({ type: 'activity', data: activity(i) });
    const feed = useStore.getState().activity;
    expect(feed).toHaveLength(300);
    expect(feed[0].id).toBe(6);
    expect(feed.at(-1)!.id).toBe(305);
  });

  it('coalesces a burst of memory/lattice events into one refetch', async () => {
    const t = fake({ ...base() });
    setTransport(t);
    await useStore.getState().init();
    vi.useFakeTimers();
    const before = useStore.getState().versions;
    const statsCalls = () => t.calls.filter((c) => c.method === 'app.state').length;
    const n0 = statsCalls();
    for (let i = 0; i < 10; i++) useStore.getState().handleEvent({ type: 'memory', data: { id: `m${i}`, op: 'save' } });
    useStore.getState().handleEvent({ type: 'lattice', data: { op: 'link' } });
    expect(useStore.getState().versions).toEqual(before);
    await vi.advanceTimersByTimeAsync(REFRESH_DEBOUNCE_MS + 1);
    const after = useStore.getState().versions;
    expect(after.memory).toBe(before.memory + 1);
    expect(after.lattice).toBe(before.lattice + 1);
    expect(after.capture).toBe(before.capture);
    expect(statsCalls()).toBe(n0 + 1);
    expect(useStore.getState().lastSavedId).toBe('m9');
  });

  it('toasts when an agent saves a memory and the toast opens it', async () => {
    setTransport(fake(base()));
    await useStore.getState().init();
    useStore.getState().handleEvent({ type: 'memory', data: { id: 'mem_abc', op: 'save' } });
    useStore.getState().handleEvent({ type: 'activity', data: activity(1, { tool: 'memory_save', summary: 'Prefer sqlite' }) });
    const toast = useStore.getState().toasts.at(-1)!;
    expect(toast.title).toBe('claude-code saved a memory');
    toast.action!.run();
    expect(useStore.getState().inspect).toEqual({ kind: 'memory', id: 'mem_abc' });
    expect(useStore.getState().inspectorOpen).toBe(true);
  });

  it('toasts a brand-new capture session once', async () => {
    setTransport(fake(base()));
    await useStore.getState().init();
    useStore.getState().handleEvent({ type: 'capture', data: { session: 'fresh', op: 'turn' } });
    useStore.getState().handleEvent({ type: 'capture', data: { session: 'fresh', op: 'turn' } });
    useStore.getState().handleEvent({ type: 'capture', data: { session: 'old-session', op: 'turn' } });
    expect(useStore.getState().toasts.filter((x) => x.title === 'New session capture')).toHaveLength(1);
  });

  it('updates MCP status and settings from events', async () => {
    setTransport(fake(base()));
    await useStore.getState().init();
    useStore.getState().handleEvent({ type: 'mcp', data: { running: false, url: '', error: 'port in use' } });
    expect(useStore.getState().app!.mcp.error).toBe('port in use');
    useStore.getState().handleEvent({ type: 'config', data: { ...settings, theme: 'dracula' } });
    expect(useStore.getState().theme.id).toBe('dracula');
  });
});

describe('inspector, filters and probe', () => {
  it('opens the right inspector panes', () => {
    const s = useStore.getState();
    s.inspectNode('project/db');
    expect(useStore.getState().inspect).toEqual({ kind: 'node', id: 'project/db' });
    s.newMemory({ summary: 'seed' });
    expect(useStore.getState().inspect).toEqual({ kind: 'new', seed: { summary: 'seed' } });
    s.closeInspector();
    expect(useStore.getState().inspect).toBeUndefined();
    expect(useStore.getState().inspectorOpen).toBe(false);
  });
  it('merges and resets filters', () => {
    useStore.getState().setFilters({ kind: 'lesson', pinnedOnly: true });
    expect(useStore.getState().filters).toMatchObject({ kind: 'lesson', pinnedOnly: true, project: '*' });
    useStore.getState().resetFilters();
    expect(useStore.getState().filters.kind).toBe('');
  });
  it('requestProbe opens the lattice view and records the request', () => {
    useStore.getState().requestProbe('database migration');
    expect(useStore.getState().tabs.some((t) => t.kind === 'lattice')).toBe(true);
    expect(useStore.getState().probeRequest?.text).toBe('database migration');
  });
  it('fail turns any error into a toast', () => {
    useStore.getState().fail('Could not archive', 'nope');
    expect(useStore.getState().toasts.at(-1)).toMatchObject({ kind: 'err', title: 'Could not archive', body: 'nope' });
  });
});

describe('raw captures preference', () => {
  afterEach(() => localStorage.removeItem('mt.showCaptures'));
  it('persists the toggle, and reset filters keeps it', () => {
    useStore.getState().setFilters({ showCaptures: true });
    expect(localStorage.getItem('mt.showCaptures')).toBe('1');
    useStore.getState().setFilters({ kind: 'note' });
    useStore.getState().resetFilters();
    expect(useStore.getState().filters.kind).toBe('');
    expect(useStore.getState().filters.showCaptures).toBe(true);
    useStore.getState().setFilters({ showCaptures: false });
    expect(localStorage.getItem('mt.showCaptures')).toBe('0');
  });
});
