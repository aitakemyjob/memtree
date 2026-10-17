// Memory and activity filtering as pure functions, so the sidebar, views
// and palette share one definition of "what the user is looking at".
import { memoryActivation } from './activation';
import type { Activity, ListArgs, MemoryRecord, RecallArgs } from './types';

/** Project scope value meaning "every project". "" is the global scope; anything else is a project path. */
export const ALL_PROJECTS = '*';

export type SortKey = 'newest' | 'oldest' | 'importance' | 'activation';

export interface MemoryFilters {
  project: string;
  tier: '' | 'episodic' | 'semantic';
  kind: string;
  pinnedOnly: boolean;
  includeSuperseded: boolean;
  includeArchived: boolean;
  /** Show raw hook captures in the list (hidden by default; the Captures view is their home). */
  showCaptures: boolean;
  sort: SortKey;
}

export const defaultFilters: MemoryFilters = {
  project: ALL_PROJECTS, tier: '', kind: '', pinnedOnly: false, includeSuperseded: false, includeArchived: false, showCaptures: false, sort: 'newest',
};

/** Whether any filter differs from the defaults (to show a "reset" affordance). */
export function filtersActive(f: MemoryFilters): boolean {
  return (Object.keys(defaultFilters) as (keyof MemoryFilters)[]).some((k) => k !== 'showCaptures' && f[k] !== defaultFilters[k]);
}

/** Maps the UI scope to the backend's project/allProjects pair. */
export function scopeArgs(project: string): { project: string; allProjects: boolean } {
  return project === ALL_PROJECTS ? { project: '', allProjects: true } : { project, allProjects: false };
}

/** Arguments for memory.list. */
export function listArgs(f: MemoryFilters, limit = 1000): ListArgs {
  return {
    ...scopeArgs(f.project), tier: f.tier, kind: f.kind, limit,
    // Asking for kind=capture explicitly must not be contradicted by the exclusion.
    ...(f.showCaptures || f.kind === 'capture' ? {} : { excludeKind: 'capture' }),
    includeSuperseded: f.includeSuperseded, includeArchived: f.includeArchived, pinnedOnly: f.pinnedOnly,
  };
}

/** Arguments for memory.recall; pinned/sort do not apply to ranked results. */
export function recallArgs(f: MemoryFilters, query: string, limit = 25): RecallArgs {
  return {
    query, ...scopeArgs(f.project), tier: f.tier, kind: f.kind, limit, includeSuperseded: f.includeSuperseded, reinforce: false,
  };
}

/** Sorts a copy: pinned first (like the backend), then by the chosen key. Ties keep newest first. */
export function sortMemories(list: MemoryRecord[], sort: SortKey, nowMs: number): MemoryRecord[] {
  const created = (r: MemoryRecord) => Date.parse(r.createdAt) || 0;
  const key = (r: MemoryRecord): number => {
    switch (sort) {
      case 'oldest': return -created(r);
      case 'importance': return r.importance;
      case 'activation': return memoryActivation(r, nowMs);
      default: return created(r);
    }
  };
  return [...list].sort((a, b) => Number(b.pinned) - Number(a.pinned) || key(b) - key(a) || created(b) - created(a));
}

/** The kinds worth offering in a filter: the backend's well-known ones plus any seen in data. */
export function knownKinds(records: MemoryRecord[]): string[] {
  const base = ['note', 'decision', 'lesson', 'session', 'capture'];
  const extra = [...new Set(records.map((r) => r.kind))].filter((k) => k && !base.includes(k)).sort();
  return [...base, ...extra];
}

export interface ActivityFilters {
  source: 'all' | 'mcp' | 'hook' | 'ui';
  harness: string;
  tool: string;
  project: string;
  errorsOnly: boolean;
  /** Minimum duration in ms. */
  minDuration: number;
}

export const defaultActivityFilters: ActivityFilters = { source: 'all', harness: '', tool: '', project: '', errorsOnly: false, minDuration: 0 };

/** Applies the activity filters, newest first. */
export function filterActivity(list: Activity[], f: ActivityFilters): Activity[] {
  return list
    .filter((a) => (f.source === 'all' || a.source === f.source)
      && (!f.harness || a.harness === f.harness)
      && (!f.tool || a.tool === f.tool)
      && (!f.project || a.project === f.project)
      && (!f.errorsOnly || !!a.error)
      && a.durationMs >= f.minDuration)
    .sort((a, b) => Date.parse(b.time) - Date.parse(a.time) || b.id - a.id);
}

/** Distinct non-empty values of a string field, sorted. */
export function distinct<T>(list: T[], pick: (t: T) => string): string[] {
  return [...new Set(list.map(pick).filter(Boolean))].sort();
}

/** Appends to a capped feed (oldest dropped), skipping an id already present. */
export function appendCapped(feed: Activity[], a: Activity, cap = 300): Activity[] {
  if (feed.some((x) => x.id === a.id && x.time === a.time)) return feed;
  const next = [...feed, a];
  return next.length > cap ? next.slice(next.length - cap) : next;
}
