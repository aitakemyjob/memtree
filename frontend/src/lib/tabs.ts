// Tab model: pure functions so they are trivially testable.

export type TabKind = 'memories' | 'lattice' | 'timeline' | 'traces' | 'captures' | 'consolidation' | 'activity' | 'settings';

export interface Tab {
  id: string;
  kind: TabKind;
  title: string;
  params: Record<string, unknown>;
}

export type NewTab = Omit<Tab, 'id'>;

/** Titles for each view, shared by the activity bar, palette and tabs. */
export const VIEW_TITLES: Record<TabKind, string> = {
  memories: 'Memories',
  lattice: 'Lattice',
  timeline: 'Timeline',
  traces: 'Traces',
  captures: 'Captures',
  consolidation: 'Consolidation',
  activity: 'Activity',
  settings: 'Settings',
};

/** Identity of a tab: every view is a singleton, so reopening re-focuses it. */
export function tabKey(kind: TabKind): string {
  return kind;
}

let seq = 0;
/** Generates a unique tab id. */
export const newTabId = () => `t${++seq}`;

export interface TabState {
  tabs: Tab[];
  active?: string;
}

/**
 * Opens a tab after the active one, or focuses an existing tab of the same
 * kind and merges its params (so "open Settings > Hooks" navigates an
 * already-open Settings tab).
 */
export function openTab(state: TabState, t: NewTab): TabState {
  const existing = state.tabs.find((x) => tabKey(x.kind) === tabKey(t.kind));
  if (existing) {
    const updated: Tab = { ...existing, title: t.title || existing.title, params: { ...existing.params, ...t.params } };
    return { tabs: state.tabs.map((x) => (x.id === existing.id ? updated : x)), active: existing.id };
  }
  const tab: Tab = { ...t, id: newTabId() };
  const idx = state.tabs.findIndex((x) => x.id === state.active);
  const tabs = [...state.tabs];
  tabs.splice(idx < 0 ? tabs.length : idx + 1, 0, tab);
  return { tabs, active: tab.id };
}

/** Closes a tab; if it was active, its right neighbour (or left) activates. */
export function closeTab(state: TabState, id: string): TabState {
  const idx = state.tabs.findIndex((x) => x.id === id);
  if (idx < 0) return state;
  const tabs = state.tabs.filter((x) => x.id !== id);
  let active = state.active;
  if (active === id) active = tabs[Math.min(idx, tabs.length - 1)]?.id;
  return { tabs, active };
}

/** Keeps only the given tab. */
export function closeOthers(state: TabState, id: string): TabState {
  if (!state.tabs.some((t) => t.id === id)) return state;
  return { tabs: state.tabs.filter((t) => t.id === id), active: id };
}

/** Activates a tab if it exists. */
export function activate(state: TabState, id: string): TabState {
  return state.tabs.some((t) => t.id === id) ? { ...state, active: id } : state;
}

/** Merges params into one tab. */
export function updateTabParams(state: TabState, id: string, params: Record<string, unknown>): TabState {
  return { ...state, tabs: state.tabs.map((t) => (t.id === id ? { ...t, params: { ...t.params, ...params } } : t)) };
}

/** Moves to the next/previous tab, wrapping around. */
export function cycleTab(state: TabState, dir: 1 | -1): TabState {
  if (state.tabs.length < 2) return state;
  const i = Math.max(0, state.tabs.findIndex((t) => t.id === state.active));
  const next = state.tabs[(i + dir + state.tabs.length) % state.tabs.length];
  return { ...state, active: next.id };
}
