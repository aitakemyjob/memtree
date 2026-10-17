// The editable shape of a memory and how it maps to backend calls.
import { snapImportance } from './format';
import type { MemoryDraft, MemoryRecord, UpdateArgs } from './types';

/** A blank draft for the new-memory form. */
export function emptyDraft(seed: Partial<MemoryDraft> = {}): MemoryDraft {
  return {
    summary: '', keywords: [], decisions: [], openLoops: [], importance: 1, kind: 'note', project: '', provenance: 'HUMAN_CONFIRMED', latticePaths: [], tier: 'episodic',
    ...seed,
  };
}

/** The editable fields of an existing record. */
export function draftFromRecord(r: MemoryRecord): MemoryDraft {
  return {
    summary: r.summary, keywords: [...(r.keywords ?? [])], decisions: [...(r.decisions ?? [])], openLoops: [...(r.openLoops ?? [])],
    importance: snapImportance(r.importance), kind: r.kind, project: r.project, provenance: r.provenance, latticePaths: [...(r.latticePaths ?? [])], tier: r.tier,
  };
}

const sameList = (a: string[], b: string[]) => a.length === b.length && a.every((x, i) => x === b[i]);
const sameSet = (a: string[], b: string[]) => a.length === b.length && [...a].sort().every((x, i) => x === [...b].sort()[i]);

/**
 * Builds a memory.update call containing only what changed, or null when
 * nothing did. Lattice paths are sent with setPaths so removing the last
 * path really clears the links (an omitted/empty list would leave them).
 */
export function updateFromDraft(id: string, original: MemoryDraft, draft: MemoryDraft): UpdateArgs | null {
  const u: UpdateArgs = { id };
  let changed = false;
  const set = <K extends keyof UpdateArgs>(k: K, v: UpdateArgs[K]) => {
    u[k] = v;
    changed = true;
  };
  if (draft.summary.trim() !== original.summary.trim()) set('summary', draft.summary);
  if (!sameList(draft.keywords, original.keywords)) set('keywords', draft.keywords);
  if (!sameList(draft.decisions, original.decisions)) set('decisions', draft.decisions);
  if (!sameList(draft.openLoops, original.openLoops)) set('openLoops', draft.openLoops);
  if (draft.importance !== original.importance) set('importance', draft.importance);
  if (draft.kind.trim() !== original.kind) set('kind', draft.kind.trim());
  if (draft.project !== original.project) set('project', draft.project);
  if (draft.provenance !== original.provenance) set('provenance', draft.provenance);
  if (!sameSet(draft.latticePaths, original.latticePaths)) {
    set('latticePaths', draft.latticePaths);
    set('setPaths', true);
  }
  return changed ? u : null;
}

/** Whether a new-memory draft can be saved (the backend requires a summary). */
export function canSave(d: MemoryDraft): boolean {
  return d.summary.trim().length > 0 && d.kind.trim().length > 0;
}

/** The record payload for memory.save. */
export function saveFromDraft(d: MemoryDraft): Partial<MemoryRecord> {
  return {
    summary: d.summary.trim(), keywords: d.keywords, decisions: d.decisions, openLoops: d.openLoops, importance: d.importance, kind: d.kind.trim() || 'note',
    project: d.project, provenance: d.provenance, latticePaths: d.latticePaths, tier: d.tier,
  };
}
