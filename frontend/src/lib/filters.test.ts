import { describe, expect, it } from 'vitest';
import {
  ALL_PROJECTS, appendCapped, defaultActivityFilters, defaultFilters, distinct, filterActivity, filtersActive, knownKinds, listArgs, recallArgs, scopeArgs, sortMemories,
} from './filters';
import type { Activity, MemoryRecord } from './types';

const NOW = Date.parse('2026-03-01T12:00:00Z');
const DAY = 86_400_000;
const iso = (ms: number) => new Date(ms).toISOString();

function rec(id: string, over: Partial<MemoryRecord> = {}): MemoryRecord {
  return {
    id, createdAt: iso(NOW - DAY), updatedAt: iso(NOW), project: '', harness: 'ui', tier: 'episodic', kind: 'note', summary: id, keywords: [], decisions: [], openLoops: [],
    importance: 1, provenance: 'HUMAN_CONFIRMED', pinned: false, accessLog: [iso(NOW - DAY)], latticePaths: [], ...over,
  };
}

describe('scope and list args', () => {
  it('maps the UI scope to project/allProjects', () => {
    expect(scopeArgs(ALL_PROJECTS)).toEqual({ project: '', allProjects: true });
    expect(scopeArgs('')).toEqual({ project: '', allProjects: false });
    expect(scopeArgs('/code/app')).toEqual({ project: '/code/app', allProjects: false });
  });
  it('builds memory.list args from filters', () => {
    const a = listArgs({ ...defaultFilters, project: '/p', tier: 'semantic', kind: 'lesson', pinnedOnly: true, includeArchived: true });
    expect(a).toMatchObject({ project: '/p', allProjects: false, tier: 'semantic', kind: 'lesson', pinnedOnly: true, includeArchived: true, includeSuperseded: false, limit: 1000 });
  });
  it('builds recall args without reinforcing (looking must not change the data)', () => {
    const a = recallArgs(defaultFilters, 'database', 10);
    expect(a).toMatchObject({ query: 'database', allProjects: true, limit: 10, reinforce: false });
  });
  it('detects changed filters', () => {
    expect(filtersActive(defaultFilters)).toBe(false);
    expect(filtersActive({ ...defaultFilters, kind: 'note' })).toBe(true);
    expect(filtersActive({ ...defaultFilters, sort: 'oldest' })).toBe(true);
  });
});

describe('sortMemories', () => {
  const list = [
    rec('old', { createdAt: iso(NOW - 10 * DAY), importance: 2.5, accessLog: [iso(NOW - 10 * DAY)] }),
    rec('new', { createdAt: iso(NOW - 1 * DAY), importance: 1 }),
    rec('mid', { createdAt: iso(NOW - 5 * DAY), importance: 1.5, accessLog: [iso(NOW - 10 * 60_000)] }),
    rec('pin', { createdAt: iso(NOW - 30 * DAY), pinned: true }),
  ];
  const ids = (s: ReturnType<typeof sortMemories>) => s.map((r) => r.id);
  it('keeps pinned first and orders the rest', () => {
    expect(ids(sortMemories(list, 'newest', NOW))).toEqual(['pin', 'new', 'mid', 'old']);
    expect(ids(sortMemories(list, 'oldest', NOW))).toEqual(['pin', 'old', 'mid', 'new']);
    expect(ids(sortMemories(list, 'importance', NOW))).toEqual(['pin', 'old', 'mid', 'new']);
  });
  it('sorts by current activation', () => {
    // mid was just accessed so it is the hottest unpinned memory.
    expect(ids(sortMemories(list, 'activation', NOW))[1]).toBe('mid');
  });
  it('does not mutate the input', () => {
    const copy = [...list];
    sortMemories(list, 'oldest', NOW);
    expect(list).toEqual(copy);
  });
});

describe('knownKinds', () => {
  it('adds unseen custom kinds after the standard ones', () => {
    expect(knownKinds([rec('a', { kind: 'preference' }), rec('b', { kind: 'note' })])).toEqual(['note', 'decision', 'lesson', 'session', 'capture', 'preference']);
  });
});

const act = (id: number, over: Partial<Activity> = {}): Activity => ({
  id, time: iso(NOW - id * 1000), source: 'mcp', harness: 'claude-code', tool: 'recall', summary: 's', project: '/p', durationMs: 5, ...over,
});

describe('activity filters', () => {
  const list = [act(1), act(2, { source: 'hook', tool: 'prompt', harness: 'codex' }), act(3, { error: 'boom' }), act(4, { durationMs: 900 }), act(5, { project: '/q' })];
  it('filters by source, harness, tool, project, errors and duration', () => {
    const f = defaultActivityFilters;
    expect(filterActivity(list, { ...f, source: 'hook' }).map((a) => a.id)).toEqual([2]);
    expect(filterActivity(list, { ...f, harness: 'codex' }).map((a) => a.id)).toEqual([2]);
    expect(filterActivity(list, { ...f, tool: 'prompt' }).map((a) => a.id)).toEqual([2]);
    expect(filterActivity(list, { ...f, project: '/q' }).map((a) => a.id)).toEqual([5]);
    expect(filterActivity(list, { ...f, errorsOnly: true }).map((a) => a.id)).toEqual([3]);
    expect(filterActivity(list, { ...f, minDuration: 100 }).map((a) => a.id)).toEqual([4]);
  });
  it('returns newest first', () => {
    expect(filterActivity(list, defaultActivityFilters).map((a) => a.id)).toEqual([1, 2, 3, 4, 5]);
  });
  it('lists distinct values', () => {
    expect(distinct(list, (a) => a.harness)).toEqual(['claude-code', 'codex']);
  });
});

describe('appendCapped', () => {
  it('caps the feed at the newest entries and ignores duplicates', () => {
    let feed: Activity[] = [];
    for (let i = 1; i <= 5; i++) feed = appendCapped(feed, act(i), 3);
    expect(feed.map((a) => a.id)).toEqual([3, 4, 5]);
    expect(appendCapped(feed, act(5), 3)).toBe(feed);
  });
});

describe('raw captures', () => {
  it('excludes kind=capture from the list unless shown or explicitly requested', () => {
    expect(listArgs(defaultFilters).excludeKind).toBe('capture');
    expect(listArgs({ ...defaultFilters, showCaptures: true }).excludeKind).toBeUndefined();
    expect(listArgs({ ...defaultFilters, kind: 'capture' }).excludeKind).toBeUndefined();
  });
  it('never excludes from search args and does not count the toggle as an active filter', () => {
    expect(recallArgs(defaultFilters, 'x')).not.toHaveProperty('excludeKind');
    expect(filtersActive({ ...defaultFilters, showCaptures: true })).toBe(false);
  });
});
