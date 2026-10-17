import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useStore } from '../lib/store';
import { setTransport } from '../lib/transport';
import type { AppState, LintReport } from '../lib/types';
import { defaultTarget, hiddenFindings, LintPanel } from './LintPanel';
import { PathEditor } from './ui';

const calls: { method: string; args: unknown }[] = [];
function transport(handlers: Record<string, (a: unknown) => unknown>) {
  calls.length = 0;
  setTransport({
    kind: 'browser',
    async invoke(method: string, args?: unknown) {
      calls.push({ method, args });
      const h = handlers[method];
      if (!h) throw new Error(`unexpected ${method}`);
      return h(args) as never;
    },
    subscribe: () => () => {},
  });
}
const report = (over: Partial<LintReport> = {}): LintReport => ({ nearDuplicates: [], outsideAxes: [], singleUse: [], weaklyFiled: [], total: 0, ...over });

beforeEach(() => {
  useStore.setState({ ...useStore.getInitialState(), app: { axisRoots: ['project', 'self'], settings: {} } as unknown as AppState, ready: true }, true);
});
afterEach(() => {
  cleanup();
  setTransport(undefined);
});

describe('lint helpers', () => {
  it('counts findings the server capped', () => {
    expect(hiddenFindings(report({ total: 40, outsideAxes: ['a'], singleUse: [{ category: 'c', memoryId: 'm', summary: '' }] }))).toBe(38);
    expect(hiddenFindings(report({ total: 0 }))).toBe(0);
  });
  it('defaults the merge target to the larger category', () => {
    expect(defaultTarget({ a: 'x/db', b: 'x/database', linksA: 1, linksB: 5 })).toBe('b');
    expect(defaultTarget({ a: 'x/db', b: 'x/database', linksA: 3, linksB: 3 })).toBe('a');
  });
});

describe('LintPanel', () => {
  const noop = () => {};
  it('says the lattice is tidy when there are no findings', () => {
    render(<LintPanel report={report()} loading={false} error={undefined} onShow={noop} onChanged={noop} />);
    expect(screen.getByText('The lattice is tidy.')).toBeInTheDocument();
  });
  it('confirms, then merges the smaller category into the larger one; swap flips it', async () => {
    transport({ 'lattice.merge': () => ({ ok: true }), 'lattice.nodes': () => [] });
    const r = report({ total: 1, nearDuplicates: [{ a: 'project/db', b: 'project/database', reason: 'abbrev', linksA: 1, linksB: 4 }] });
    render(<LintPanel report={r} loading={false} error={undefined} onShow={noop} onChanged={noop} />);
    await userEvent.click(screen.getByRole('button', { name: /Merge into database/ }));
    expect(screen.getByText(/Moves 1 memory link from/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Merge into project/database' }));
    await waitFor(() => expect(calls.find((c) => c.method === 'lattice.merge')?.args).toEqual({ from: 'project/db', to: 'project/database' }));
  });
  it('swap changes the direction', async () => {
    transport({ 'lattice.merge': () => ({ ok: true }), 'lattice.nodes': () => [] });
    const r = report({ total: 1, nearDuplicates: [{ a: 'project/db', b: 'project/database', reason: 'abbrev', linksA: 1, linksB: 4 }] });
    render(<LintPanel report={r} loading={false} error={undefined} onShow={noop} onChanged={noop} />);
    await userEvent.click(screen.getByLabelText('Swap merge direction'));
    await userEvent.click(screen.getByRole('button', { name: /Merge into db/ }));
    await userEvent.click(screen.getByRole('button', { name: 'Merge into project/db' }));
    await waitFor(() => expect(calls.find((c) => c.method === 'lattice.merge')?.args).toEqual({ from: 'project/database', to: 'project/db' }));
  });
  it('files a weakly filed memory with lattice.tag and shows the +N more note', async () => {
    transport({ 'lattice.tag': () => ({ ok: true }), 'lattice.nodes': () => [], 'lattice.checkPaths': () => [] });
    useStore.setState({ nodes: [{ id: 'project/db', parentId: 'project', label: 'db', createdAt: '', accessCount: 0, lastAccessed: '', linkCount: 1, activation: 0.5 }] });
    const r = report({ total: 30, weaklyFiled: [{ memoryId: 'mem_1', summary: 'Loose note', paths: ['guess/x'] }] });
    render(<LintPanel report={r} loading={false} error={undefined} onShow={noop} onChanged={noop} />);
    expect(screen.getByText('guess/x')).toBeInTheDocument();
    expect(screen.getByText(/\+29 more not shown/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'File' })).toBeDisabled();
    await userEvent.type(screen.getByLabelText('Add category'), 'project/db{Enter}');
    await userEvent.click(screen.getByRole('button', { name: 'File' }));
    await waitFor(() => expect(calls.find((c) => c.method === 'lattice.tag')?.args).toEqual({ id: 'mem_1', paths: ['project/db'] }));
  });
});

describe('path warnings in PathEditor', () => {
  function Host() {
    const [p, setP] = useState(['project/database']);
    return (
      <>
        <PathEditor paths={p} onChange={setP} />
        <output data-testid="paths">{p.join(',')}</output>
      </>
    );
  }
  it('debounces the check, shows the advice and swaps in the suggestion with one click', async () => {
    transport({ 'lattice.checkPaths': () => [{ path: 'project/database', message: 'similar to project/db', suggest: 'project/db' }] });
    render(<Host />);
    expect(await screen.findByText(/similar to project\/db/, undefined, { timeout: 2000 })).toBeInTheDocument();
    expect(calls.filter((c) => c.method === 'lattice.checkPaths')).toHaveLength(1);
    expect(calls[0].args).toEqual({ paths: ['project/database'] });
    await userEvent.click(screen.getByRole('button', { name: /Use project\/db/ }));
    expect(screen.getByTestId('paths')).toHaveTextContent('project/db');
    expect(screen.getByTestId('paths').textContent).toBe('project/db');
  });
  it('stays silent when the check fails (advisory only)', async () => {
    transport({ 'lattice.checkPaths': () => { throw new Error('nope'); } });
    render(<Host />);
    await waitFor(() => expect(calls.length).toBe(1), { timeout: 2000 });
    expect(screen.queryByRole('status', { name: 'Category warnings' })).toBeNull();
  });
});
