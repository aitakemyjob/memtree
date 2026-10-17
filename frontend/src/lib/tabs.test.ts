import { describe, expect, it } from 'vitest';
import { activate, closeOthers, closeTab, cycleTab, openTab, updateTabParams, type TabState } from './tabs';

const empty: TabState = { tabs: [] };
const mk = (kind: 'memories' | 'lattice' | 'settings' | 'activity', params = {}) => ({ kind, title: kind, params });

describe('openTab', () => {
  it('opens and activates a new tab after the active one', () => {
    let s = openTab(empty, mk('memories'));
    s = openTab(s, mk('lattice'));
    s = openTab(s, mk('settings'));
    s = activate(s, s.tabs[0].id);
    s = openTab(s, mk('activity'));
    expect(s.tabs.map((t) => t.kind)).toEqual(['memories', 'activity', 'lattice', 'settings']);
    expect(s.tabs.find((t) => t.id === s.active)?.kind).toBe('activity');
  });
  it('re-focuses a view that is already open and merges params', () => {
    let s = openTab(empty, mk('settings', { section: 'general' }));
    s = openTab(s, mk('memories'));
    s = openTab(s, mk('settings', { section: 'hooks' }));
    expect(s.tabs).toHaveLength(2);
    const t = s.tabs.find((x) => x.id === s.active)!;
    expect(t.kind).toBe('settings');
    expect(t.params).toEqual({ section: 'hooks' });
  });
});

describe('closing', () => {
  it('activates the right neighbour, else the left', () => {
    let s = openTab(empty, mk('memories'));
    s = openTab(s, mk('lattice'));
    s = openTab(s, mk('settings'));
    const [a, b, c] = s.tabs.map((t) => t.id);
    s = activate(s, b);
    s = closeTab(s, b);
    expect(s.active).toBe(c);
    s = closeTab(s, c);
    expect(s.active).toBe(a);
    s = closeTab(s, a);
    expect(s.active).toBeUndefined();
    expect(s.tabs).toEqual([]);
  });
  it('closing an inactive tab keeps the active one; unknown ids are no-ops', () => {
    let s = openTab(empty, mk('memories'));
    s = openTab(s, mk('lattice'));
    const [a, b] = s.tabs.map((t) => t.id);
    expect(closeTab(s, a).active).toBe(b);
    expect(closeTab(s, 'nope')).toBe(s);
  });
  it('closeOthers keeps one', () => {
    let s = openTab(empty, mk('memories'));
    s = openTab(s, mk('lattice'));
    s = closeOthers(s, s.tabs[0].id);
    expect(s.tabs).toHaveLength(1);
    expect(s.active).toBe(s.tabs[0].id);
    expect(closeOthers(s, 'zzz')).toBe(s);
  });
});

describe('activate / update / cycle', () => {
  it('ignores unknown ids', () => {
    const s = openTab(empty, mk('memories'));
    expect(activate(s, 'zzz')).toBe(s);
  });
  it('updates params of one tab only', () => {
    let s = openTab(empty, mk('memories'));
    s = openTab(s, mk('lattice'));
    const id = s.tabs[0].id;
    s = updateTabParams(s, id, { q: 'x' });
    expect(s.tabs[0].params).toEqual({ q: 'x' });
    expect(s.tabs[1].params).toEqual({});
  });
  it('cycles with wraparound', () => {
    let s = openTab(empty, mk('memories'));
    s = openTab(s, mk('lattice'));
    s = openTab(s, mk('settings'));
    const ids = s.tabs.map((t) => t.id);
    expect(cycleTab(s, 1).active).toBe(ids[0]);
    expect(cycleTab(cycleTab(s, 1), -1).active).toBe(ids[2]);
    expect(cycleTab({ tabs: [] }, 1).tabs).toEqual([]);
  });
});
