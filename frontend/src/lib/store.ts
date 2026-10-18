import { create } from 'zustand';
import { api } from './api';
import { planEvent, type ToastPlan } from './events';
import { appendCapped, defaultFilters, type MemoryFilters } from './filters';

const CAPTURES_PREF = 'mt.showCaptures';

/** Reads the persisted "show raw captures" preference, tolerating storage failures. */
function readShowCaptures(): boolean {
  try {
    return localStorage.getItem(CAPTURES_PREF) === '1';
  } catch {
    return false;
  }
}

function writeShowCaptures(on: boolean) {
  try {
    localStorage.setItem(CAPTURES_PREF, on ? '1' : '0');
  } catch {
    /* the preference just will not persist */
  }
}
import { getTransport } from './transport';
import { activate, closeOthers, closeTab, cycleTab, openTab, updateTabParams, VIEW_TITLES, type NewTab, type Tab, type TabKind } from './tabs';
import { applyTheme, cachedThemeId, themeById, type Theme } from './themes';
import type { Activity, AppState, BusEvent, CategoryNode, MemoryDraft, Recalled, Settings } from './types';

/** What the inspector pane shows. */
export type Inspect =
  | { kind: 'memory'; id: string }
  | { kind: 'node'; id: string }
  | { kind: 'new'; seed?: Partial<MemoryDraft> };

export interface Toast {
  id: number;
  kind: 'info' | 'ok' | 'warn' | 'err' | 'agent';
  title: string;
  body?: string;
  action?: { label: string; run: () => void };
}

/** Data sets views refetch when their version changes. */
export interface Versions {
  memory: number;
  lattice: number;
  capture: number;
  trace: number;
}

interface Store {
  ready: boolean;
  fatal?: string;
  app?: AppState;
  theme: Theme;
  tabs: Tab[];
  active?: string;
  sidebarOpen: boolean;
  inspectorOpen: boolean;
  inspect?: Inspect;
  activity: Activity[];
  toasts: Toast[];
  palette: boolean;
  connectOpen: boolean;
  filters: MemoryFilters;
  /** Text in the memory search box ("" = plain list). */
  query: string;
  versions: Versions;
  /** A probe requested from outside the lattice view (palette); the view consumes it. */
  probeRequest?: { text: string; nonce: number };
  knownSessions: Set<string>;
  lastSavedId?: string;
  /** The lattice, shared by every path autocomplete; refetched when the lattice version bumps. */
  nodes: CategoryNode[];
  /** Score details from the latest search, keyed by memory id, for the inspector's "why this score". */
  recallInfo: Record<string, Recalled>;

  /** Connects to the backend, loads state and opens the Memories tab. */
  init(): Promise<void>;
  /** Reloads app state (settings, counts, MCP) and applies the theme. */
  refreshApp(): Promise<void>;
  /** Reloads the lattice nodes. */
  refreshNodes(): Promise<void>;
  /** Remembers search-result scores for the inspector. */
  setRecallInfo(results: Recalled[]): void;
  /** Applies and persists a theme. */
  setTheme(id: string): Promise<void>;
  /** Saves settings: always sends the WHOLE object (the backend replaces it); reverts and toasts on failure. */
  saveSettings(next: Settings): Promise<boolean>;
  /** Opens a tab, or focuses the view if already open. */
  open(t: NewTab): void;
  /** Opens a view by kind with its default title. */
  openView(kind: TabKind, params?: Record<string, unknown>): void;
  close(id: string): void;
  closeOthers(id: string): void;
  activate(id: string): void;
  updateTab(id: string, params: Record<string, unknown>): void;
  cycle(dir: 1 | -1): void;
  toggleSidebar(): void;
  toggleInspector(open?: boolean): void;
  /** Shows a memory in the inspector. */
  inspectMemory(id: string): void;
  /** Shows a lattice category in the inspector. */
  inspectNode(id: string): void;
  /** Opens the inspector on a blank new-memory form. */
  newMemory(seed?: Partial<MemoryDraft>): void;
  closeInspector(): void;
  setFilters(f: Partial<MemoryFilters>): void;
  resetFilters(): void;
  setQuery(q: string): void;
  toast(t: Omit<Toast, 'id'>): void;
  dismissToast(id: number): void;
  /** Reports a failed backend call as a toast; never throws. */
  fail(title: string, e: unknown): void;
  setPalette(open: boolean): void;
  setConnectOpen(open: boolean): void;
  /** Opens the Lattice view and runs a probe with the text. */
  requestProbe(text: string): void;
  /** Applies a backend bus event. */
  handleEvent(ev: BusEvent): void;
  /** Bumps data versions so views refetch. */
  bump(which: Partial<Record<keyof Versions, boolean>>): void;
}

let toastSeq = 0;
let staleTimer: ReturnType<typeof setTimeout> | undefined;
let pending: { memory?: boolean; lattice?: boolean; capture?: boolean; trace?: boolean; stats?: boolean } = {};

/** How long to coalesce a burst of events (an agent saving ten memories) into one refetch. */
export const REFRESH_DEBOUNCE_MS = 250;

export const useStore = create<Store>((set, get) => ({
  ready: false,
  theme: themeById(cachedThemeId()),
  tabs: [],
  sidebarOpen: true,
  inspectorOpen: false,
  activity: [],
  toasts: [],
  palette: false,
  connectOpen: false,
  filters: { ...defaultFilters, showCaptures: readShowCaptures() },
  query: '',
  versions: { memory: 0, lattice: 0, capture: 0, trace: 0 },
  knownSessions: new Set(),
  nodes: [],
  recallInfo: {},

  /** See Store.init. */
  async init() {
    try {
      const t = await getTransport();
      t.subscribe((ev) => get().handleEvent(ev));
      await get().refreshApp();
      const [activity, sessions] = await Promise.all([api.activity().catch(() => []), api.captures(500).catch(() => [])]);
      set({ activity: activity.slice(-300), knownSessions: new Set(sessions.map((s) => s.id)), ready: true });
      get().openView('memories');
      void get().refreshNodes();
    } catch (e) {
      set({ fatal: (e as Error).message, ready: true });
    }
  },

  /** See Store.refreshApp. */
  async refreshApp() {
    const app = await api.state();
    const theme = themeById(app.settings.theme);
    if (theme.id !== get().theme.id || !get().ready) applyTheme(theme);
    document.documentElement.style.setProperty('--ui-font-size', `${app.settings.fontSize}px`);
    set({ app, theme });
  },

  /** See Store.refreshNodes. */
  async refreshNodes() {
    try {
      set({ nodes: await api.nodes() });
    } catch {
      /* autocomplete just stays stale; views surface their own load errors */
    }
  },

  setRecallInfo(results) {
    set({ recallInfo: Object.fromEntries(results.map((r) => [r.record.id, r])) });
  },

  /** See Store.setTheme. */
  async setTheme(id) {
    const app = get().app;
    const theme = themeById(id);
    applyTheme(theme);
    set({ theme });
    if (app) await get().saveSettings({ ...app.settings, theme: theme.id });
  },

  /** See Store.saveSettings. */
  async saveSettings(next) {
    const app = get().app;
    if (!app) return false;
    const prev = app.settings;
    set({ app: { ...app, settings: next } });
    try {
      const saved = await api.updateSettings(next);
      const cur = get().app;
      if (cur) set({ app: { ...cur, settings: saved ?? next } });
      document.documentElement.style.setProperty('--ui-font-size', `${next.fontSize}px`);
      return true;
    } catch (e) {
      const cur = get().app;
      if (cur) set({ app: { ...cur, settings: prev } });
      applyTheme(themeById(prev.theme));
      set({ theme: themeById(prev.theme) });
      get().fail('Could not save settings', e);
      return false;
    }
  },

  open(t) {
    set((s) => openTab(s, t));
  },
  openView(kind, params = {}) {
    set((s) => openTab(s, { kind, title: VIEW_TITLES[kind], params }));
  },
  close(id) {
    set((s) => closeTab(s, id));
  },
  closeOthers(id) {
    set((s) => closeOthers(s, id));
  },
  activate(id) {
    set((s) => activate(s, id));
  },
  updateTab(id, params) {
    set((s) => updateTabParams(s, id, params));
  },
  cycle(dir) {
    set((s) => cycleTab(s, dir));
  },
  toggleSidebar() {
    set((s) => ({ sidebarOpen: !s.sidebarOpen }));
  },
  toggleInspector(open) {
    set((s) => ({ inspectorOpen: open ?? !s.inspectorOpen }));
  },
  inspectMemory(id) {
    set({ inspect: { kind: 'memory', id }, inspectorOpen: true });
  },
  inspectNode(id) {
    set({ inspect: { kind: 'node', id }, inspectorOpen: true });
  },
  newMemory(seed) {
    set({ inspect: { kind: 'new', seed }, inspectorOpen: true });
  },
  closeInspector() {
    set({ inspect: undefined, inspectorOpen: false });
  },
  setFilters(f) {
    if (f.showCaptures !== undefined) writeShowCaptures(f.showCaptures);
    set((s) => ({ filters: { ...s.filters, ...f } }));
  },
  resetFilters() {
    // Reset keeps the raw-captures preference: it is a standing choice, not a filter.
    set((s) => ({ filters: { ...defaultFilters, showCaptures: s.filters.showCaptures } }));
  },
  setQuery(q) {
    set({ query: q });
  },

  toast(t) {
    const id = ++toastSeq;
    set((s) => ({ toasts: [...s.toasts.slice(-4), { ...t, id }] }));
    setTimeout(() => get().dismissToast(id), t.action ? 9000 : 4500);
  },
  dismissToast(id) {
    set((s) => ({ toasts: s.toasts.filter((x) => x.id !== id) }));
  },
  fail(title, e) {
    get().toast({ kind: 'err', title, body: e instanceof Error ? e.message : String(e) });
  },
  setPalette(open) {
    set({ palette: open });
  },
  setConnectOpen(open) {
    set({ connectOpen: open });
  },
  requestProbe(text) {
    get().openView('lattice');
    set({ probeRequest: { text, nonce: Date.now() } });
  },

  /** See Store.handleEvent. Effects come from the pure planEvent. */
  handleEvent(ev) {
    const fx = planEvent(ev, { knownSessions: get().knownSessions });
    if (fx.activity) set((s) => ({ activity: appendCapped(s.activity, fx.activity!) }));
    if (fx.mcp && get().app) set((s) => ({ app: { ...s.app!, mcp: fx.mcp! } }));
    if (fx.settings && get().app) {
      const theme = themeById(fx.settings.theme);
      if (theme.id !== get().theme.id) applyTheme(theme);
      set((s) => ({ app: { ...s.app!, settings: fx.settings! }, theme }));
    }
    if (fx.savedMemory) set({ lastSavedId: fx.savedMemory });
    if (fx.newSession) set((s) => ({ knownSessions: new Set(s.knownSessions).add(fx.newSession!) }));
    if (fx.toast) {
      const t = toToast(fx.toast, get());
      get().toast(t);
    }
    if (fx.stale.memory || fx.stale.lattice || fx.stale.capture || fx.stale.trace || fx.refreshStats) {
      pending = {
        memory: pending.memory || fx.stale.memory, lattice: pending.lattice || fx.stale.lattice,
        capture: pending.capture || fx.stale.capture, trace: pending.trace || fx.stale.trace, stats: pending.stats || fx.refreshStats,
      };
      clearTimeout(staleTimer);
      staleTimer = setTimeout(() => {
        const p = pending;
        pending = {};
        get().bump(p);
        if (p.stats) get().refreshApp().catch(() => {});
      }, REFRESH_DEBOUNCE_MS);
    }
  },

  bump(which) {
    set((s) => ({
      versions: {
        memory: s.versions.memory + (which.memory ? 1 : 0),
        lattice: s.versions.lattice + (which.lattice ? 1 : 0),
        capture: s.versions.capture + (which.capture ? 1 : 0),
        trace: s.versions.trace + (which.trace ? 1 : 0),
      },
    }));
  },
}));

/** Turns a toast plan into a store toast with a working action button. */
function toToast(p: ToastPlan, s: Store): Omit<Toast, 'id'> {
  const base = { kind: p.kind, title: p.title, body: p.body };
  if (p.open === 'latest-memory') {
    return { ...base, action: { label: 'Open', run: () => {
      const st = useStore.getState();
      st.openView('memories');
      if (st.lastSavedId) st.inspectMemory(st.lastSavedId);
    } } };
  }
  if (p.open === 'captures') return { ...base, action: { label: 'View', run: () => useStore.getState().openView('captures') } };
  void s;
  return base;
}

/** For tests: clears the pending-refresh debounce state. */
export function resetEventDebounce() {
  clearTimeout(staleTimer);
  pending = {};
}
