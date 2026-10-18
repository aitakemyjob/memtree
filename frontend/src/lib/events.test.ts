import { describe, expect, it } from 'vitest';
import { planEvent } from './events';
import type { Activity } from './types';

const ctx = { knownSessions: new Set(['known']) };
const act = (over: Partial<Activity>): Activity => ({ id: 1, time: '2026-03-01T00:00:00Z', source: 'mcp', harness: 'claude-code', tool: 'recall', summary: 'q', project: '', durationMs: 3, ...over });

describe('planEvent', () => {
  it('marks memory data stale and remembers saved ids', () => {
    const fx = planEvent({ type: 'memory', data: { id: 'mem_1', op: 'save' } }, ctx);
    expect(fx.stale).toEqual({ memory: true });
    expect(fx.refreshStats).toBe(true);
    expect(fx.savedMemory).toBe('mem_1');
    expect(planEvent({ type: 'memory', data: { id: 'mem_1', op: 'touch' } }, ctx).savedMemory).toBeUndefined();
  });
  it('lattice events refresh the lattice and the memory paths', () => {
    expect(planEvent({ type: 'lattice', data: { op: 'tag' } }, ctx).stale).toEqual({ lattice: true, memory: true });
  });
  it('trace events refresh trace views only', () => {
    const fx = planEvent({ type: 'trace', data: { op: 'record', id: 'ret_1' } }, ctx);
    expect(fx.stale).toEqual({ trace: true });
    expect(fx.refreshStats).toBe(false);
  });
  it('appends activity and toasts only for notable agent writes', () => {
    expect(planEvent({ type: 'activity', data: act({}) }, ctx).toast).toBeUndefined();
    const saved = planEvent({ type: 'activity', data: act({ tool: 'memory_save', summary: 'Use WAL mode' }) }, ctx);
    expect(saved.activity?.tool).toBe('memory_save');
    expect(saved.toast).toMatchObject({ kind: 'agent', title: 'claude-code saved a memory', body: 'Use WAL mode', open: 'latest-memory' });
    expect(planEvent({ type: 'activity', data: act({ tool: 'memory_compress' }) }, ctx).toast?.title).toContain('distilled');
    const failed = planEvent({ type: 'activity', data: act({ tool: 'memory_save', error: 'agent writes are off' }) }, ctx);
    expect(failed.toast).toMatchObject({ kind: 'err', body: 'agent writes are off' });
    expect(planEvent({ type: 'activity', data: act({ tool: 'connect', summary: 'cursor connected' }) }, ctx).toast?.kind).toBe('agent');
    expect(planEvent({ type: 'activity', data: act({ source: 'hook', tool: 'prompt', error: 'bad payload' }) }, ctx).toast?.kind).toBe('warn');
    // Reads and healthy hook traffic stay quiet.
    expect(planEvent({ type: 'activity', data: act({ source: 'hook', tool: 'prompt' }) }, ctx).toast).toBeUndefined();
  });
  it('toasts a capture session once, then only for summaries', () => {
    const first = planEvent({ type: 'capture', data: { session: 'new-1', op: 'turn' } }, ctx);
    expect(first.newSession).toBe('new-1');
    expect(first.toast?.open).toBe('captures');
    expect(first.stale).toEqual({ capture: true });
    const again = planEvent({ type: 'capture', data: { session: 'known', op: 'turn' } }, ctx);
    expect(again.toast).toBeUndefined();
    expect(again.refreshStats).toBe(false);
    const done = planEvent({ type: 'capture', data: { session: 'known', op: 'summarized' } }, ctx);
    expect(done.toast?.kind).toBe('ok');
    expect(done.refreshStats).toBe(true);
  });
  it('passes config and mcp payloads through', () => {
    expect(planEvent({ type: 'mcp', data: { running: true, url: 'u' } }, ctx).mcp?.url).toBe('u');
    expect(planEvent({ type: 'config', data: { theme: 'nord' } }, ctx).settings?.theme).toBe('nord');
  });
  it('ignores unknown and malformed events', () => {
    for (const ev of [{ type: 'focus', data: {} }, { type: 'wat' }, { type: 'mcp', data: 5 }, { type: 'activity' }, { type: 'config', data: null }]) {
      expect(planEvent(ev, ctx)).toEqual({ stale: {}, refreshStats: false });
    }
    expect(planEvent({ type: 'capture', data: 'x' }, ctx).stale).toEqual({ capture: true });
  });
});
