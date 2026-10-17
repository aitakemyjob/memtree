import { useState } from 'react';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useStore } from '../lib/store';
import { setTransport, type Transport } from '../lib/transport';
import type { AppState, CategoryNode, MemoryRecord, Settings } from '../lib/types';
import { ActivityView } from '../views/ActivityView';
import { SettingsView } from '../views/SettingsView';
import { CommandPalette } from './CommandPalette';
import { needsProject, ResultBox } from './HooksPanel';
import { MemoryCard } from './MemoryCard';
import { PathInput, StringList } from './ui';
import { Welcome } from './Welcome';

const settings: Settings = {
  theme: 'sfc-dark', fontSize: 13, mcpPort: 47475, dbPath: '', agentWrites: true, captureHooks: true, surfaceThreshold: 0.25, maxSurface: 3,
  linkHalfLifeDays: 30, maintainHours: 12, tiers: { ageHoursToSemantic: 72, ageHoursToCompact: 720, activationCeilingToCompress: 0.5, minClusterSize: 2 },
};
const app: AppState = {
  version: '0.1.0', platform: 'linux', settings, mcp: { running: true, url: 'http://127.0.0.1:47475/mcp' },
  stats: { total: 3, episodic: 3, semantic: 0, pinned: 0, superseded: 0, archived: 0, untagged: 0, nodes: 2, links: 3, capturesPending: 0 },
  projects: [{ project: '/p/app', count: 3 }], axisRoots: ['project'], configPath: '/c.json', canPickDir: false,
};
const node = (id: string, linkCount = 0): CategoryNode => ({ id, parentId: id.includes('/') ? id.slice(0, id.lastIndexOf('/')) : '', label: id.split('/').pop()!, createdAt: '', accessCount: 0, lastAccessed: '', linkCount, activation: 0.5 });
const rec = (over: Partial<MemoryRecord> = {}): MemoryRecord => ({
  id: 'mem_1', createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(), project: '/p/app', harness: 'ui', tier: 'episodic', kind: 'decision', summary: 'Use SQLite with WAL',
  keywords: [], decisions: [], openLoops: [], importance: 1.5, provenance: 'HUMAN_CONFIRMED', pinned: false, accessLog: [new Date().toISOString()], latticePaths: ['project/db'], ...over,
});

function fakeTransport(handlers: Record<string, (a: unknown) => unknown>): Transport & { calls: { method: string; args: unknown }[] } {
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
  useStore.setState({ ...useStore.getInitialState(), app, ready: true, nodes: [node('project'), node('project/db', 3), node('project/architecture', 1)] }, true);
});
afterEach(() => {
  cleanup();
  setTransport(undefined);
});

describe('PathInput', () => {
  it('suggests existing paths, picks with the keyboard and clears', async () => {
    const onPick = vi.fn();
    render(<PathInput onPick={onPick} />);
    const input = screen.getByRole('combobox', { name: 'Category path' });
    await userEvent.type(input, 'arch');
    expect(await screen.findByRole('option', { name: /project\/architecture/ })).toBeInTheDocument();
    await userEvent.keyboard('{Enter}');
    expect(onPick).toHaveBeenCalledWith('project/architecture');
    expect(input).toHaveValue('');
  });
  it('offers a brand new path (normalised) and honours allowNew=false', async () => {
    const onPick = vi.fn();
    const { unmount } = render(<PathInput onPick={onPick} />);
    await userEvent.type(screen.getByRole('combobox'), 'Project/New Thing');
    const opt = await screen.findByRole('option', { name: /project\/newthing/ });
    expect(within(opt).getByText('new')).toBeInTheDocument();
    unmount();
    render(<PathInput onPick={onPick} allowNew={false} />);
    await userEvent.type(screen.getByRole('combobox'), 'zzz');
    expect(screen.queryByRole('option')).toBeNull();
  });
  it('never suggests a path that is already chosen', async () => {
    render(<PathInput onPick={() => {}} exclude={['project/db']} />);
    await userEvent.type(screen.getByRole('combobox'), 'db');
    expect(screen.queryByRole('option', { name: /^project\/db\b/ })).toBeNull();
  });
});

describe('StringList', () => {
  it('adds on Enter, ignores duplicates and removes', async () => {
    const changes: string[][] = [];
    function Host() {
      const [v, setV] = useState<string[]>(['a']);
      return <StringList label="keyword" values={v} placeholder="add" onChange={(n) => { changes.push(n); setV(n); }} />;
    }
    render(<Host />);
    const input = screen.getByLabelText('Add keyword', { selector: 'input' });
    await userEvent.type(input, 'b{Enter}');
    await userEvent.type(input, 'b{Enter}');
    expect(changes).toEqual([['a', 'b']]);
    await userEvent.click(screen.getByLabelText('Remove a'));
    expect(changes.at(-1)).toEqual(['b']);
  });
});

describe('MemoryCard', () => {
  it('shows badges, categories and the score decomposition for search results', () => {
    render(
      <MemoryCard
        nowMs={Date.now()} selected={false} onOpen={() => {}} record={rec({ pinned: true, supersededBy: 'mem_2' })}
        recall={{ record: rec(), score: { bm25Normalized: 0.8, importanceOffset: 1.5, activation: 0.5, projectBoost: 1.25, final: 0.75 }, source: 'both', via: 'project/db' }}
      />,
    );
    expect(screen.getByText('pinned')).toBeInTheDocument();
    expect(screen.getByText('superseded')).toBeInTheDocument();
    expect(screen.getByText('project/db', { selector: '.chip' })).toBeInTheDocument();
    expect(screen.getByLabelText('Score decomposition')).toHaveTextContent('relevance 0.80');
    expect(screen.getByLabelText('Score decomposition')).toHaveTextContent(/=\s*0\.75/);
    expect(screen.getByText('both')).toBeInTheDocument();
  });
  it('opens with Enter for keyboard users', () => {
    const onOpen = vi.fn();
    render(<MemoryCard nowMs={Date.now()} selected={false} onOpen={onOpen} record={rec()} />);
    fireEvent.keyDown(screen.getByRole('button'), { key: 'Enter' });
    expect(onOpen).toHaveBeenCalled();
  });
});

describe('CommandPalette', () => {
  it('runs a typed command with Enter instead of the generic search action', async () => {
    const t = fakeTransport({ 'memory.recall': () => ({ memories: [], categories: [] }), 'settings.update': (a) => a });
    setTransport(t);
    render(<CommandPalette />);
    await userEvent.type(screen.getByLabelText('Command or search'), 'toggle light');
    await userEvent.keyboard('{Enter}');
    await waitFor(() => expect(useStore.getState().theme.id).toBe('sfc-light'));
    expect((t.calls.find((c) => c.method === 'settings.update')!.args as Settings).theme).toBe('sfc-light');
  });
  it('offers search and probe for free text and probes through the store', async () => {
    setTransport(fakeTransport({ 'memory.recall': () => ({ memories: [{ record: rec(), score: { bm25Normalized: 1, importanceOffset: 1, activation: 1, projectBoost: 1, final: 0.9 }, source: 'text' }], categories: [] }) }));
    render(<CommandPalette />);
    await userEvent.type(screen.getByLabelText('Command or search'), 'database');
    expect(await screen.findByText(/Search memories for/)).toBeInTheDocument();
    expect(await screen.findByText('Use SQLite with WAL')).toBeInTheDocument();
    await userEvent.click(screen.getByText(/Probe lattice with/));
    expect(useStore.getState().probeRequest?.text).toBe('database');
    expect(useStore.getState().tabs.some((x) => x.kind === 'lattice')).toBe(true);
  });
  it('lists view shortcuts when empty and opens one', async () => {
    render(<CommandPalette />);
    await userEvent.click(screen.getByText('Captures'));
    expect(useStore.getState().tabs.some((x) => x.kind === 'captures')).toBe(true);
    expect(useStore.getState().palette).toBe(false);
  });
});

describe('Welcome', () => {
  it('jumps to hooks settings, Connect and the new-memory form', async () => {
    render(<Welcome />);
    await userEvent.click(screen.getByText('Install hooks', { selector: 'button' }));
    const t = useStore.getState().tabs.find((x) => x.kind === 'settings');
    expect(t?.params.section).toBe('hooks');
    await userEvent.click(screen.getByText('Connection snippets'));
    expect(useStore.getState().connectOpen).toBe(true);
    await userEvent.click(screen.getByText('New memory'));
    expect(useStore.getState().inspect?.kind).toBe('new');
  });
});

describe('ActivityView', () => {
  it('explains how to install hooks when the feed is empty', async () => {
    render(<ActivityView />);
    expect(screen.getByText('No activity yet')).toBeInTheDocument();
    await userEvent.click(screen.getAllByText('Install hooks')[0]);
    expect(useStore.getState().tabs.find((x) => x.kind === 'settings')?.params.section).toBe('hooks');
  });
  it('filters rows and warns when only MCP calls were seen', async () => {
    const mk = (id: number, over = {}) => ({ id, time: new Date(Date.now() - id * 1000).toISOString(), source: 'mcp', harness: 'cursor', tool: 'recall', summary: `q${id}`, project: '', durationMs: 3, ...over });
    useStore.setState({ activity: [mk(1), mk(2, { error: 'writes disabled', tool: 'memory_save' })] });
    render(<ActivityView />);
    expect(screen.getByText(/No hook events yet/)).toBeInTheDocument();
    expect(screen.getByText('q1')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('switch', { name: 'Errors only' }));
    expect(screen.queryByText('q1')).toBeNull();
    expect(screen.getByText('writes disabled')).toBeInTheDocument();
  });
});

describe('SettingsView', () => {
  it('saves the WHOLE settings object and shows a validation error instead of saving bad input', async () => {
    const t = fakeTransport({ 'settings.update': (a) => a });
    setTransport(t);
    useStore.getState().open({ kind: 'settings', title: 'Settings', params: { section: 'agents' } });
    render(<SettingsView />);
    await userEvent.click(screen.getByRole('switch', { name: 'Agent writes' }));
    expect(screen.getByText('Unsaved changes.')).toBeInTheDocument();
    await userEvent.click(screen.getByText('Save settings'));
    await waitFor(() => expect(t.calls.some((c) => c.method === 'settings.update')).toBe(true));
    const sent = t.calls.find((c) => c.method === 'settings.update')!.args as Settings;
    expect(sent.agentWrites).toBe(false);
    expect(sent.tiers).toEqual(settings.tiers);
    expect(sent.mcpPort).toBe(47475);
    // An invalid threshold blocks saving.
    await userEvent.click(screen.getByRole('switch', { name: 'Capture hooks' }));
    const max = screen.getByLabelText('Max surfaced per prompt');
    fireEvent.change(max, { target: { value: '0' } });
    expect(screen.getByText(/Max surfaced memories/)).toBeInTheDocument();
    expect(screen.getByText('Save settings')).toBeDisabled();
  });
});

describe('hook results', () => {
  it('requires a project for VS Code only', () => {
    expect(needsProject('vscode')).toBe(true);
    expect(needsProject('claude-code')).toBe(false);
  });
  it('labels dry runs and lists changes and notes', () => {
    render(<ResultBox result={{ harness: 'cursor', dryRun: true, changes: [{ file: '/x/hooks.json', action: 'add', detail: 'hook' }], notes: ['Restart it.'] }} />);
    expect(screen.getByText(/Dry run: nothing was written/)).toBeInTheDocument();
    expect(screen.getByText('/x/hooks.json')).toBeInTheDocument();
    expect(screen.getByText('Restart it.')).toBeInTheDocument();
  });
  it('tolerates null lists from the backend', () => {
    render(<ResultBox result={{ harness: 'cursor', dryRun: false, changes: null as never, notes: null as never }} />);
    expect(screen.getByText('No changes were needed.')).toBeInTheDocument();
  });
});

