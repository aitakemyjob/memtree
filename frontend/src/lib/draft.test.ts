import { describe, expect, it } from 'vitest';
import { canSave, draftFromRecord, emptyDraft, saveFromDraft, updateFromDraft } from './draft';
import type { MemoryRecord } from './types';

const rec: MemoryRecord = {
  id: 'mem_1', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z', project: '/p', harness: 'mcp', tier: 'episodic', kind: 'decision',
  summary: 'Use SQLite', keywords: ['sqlite', 'db'], decisions: ['WAL on'], openLoops: [], importance: 1.5, provenance: 'MODEL_INFERRED', pinned: false, accessLog: [],
  latticePaths: ['project/architecture/db'],
};

describe('updateFromDraft', () => {
  const orig = draftFromRecord(rec);
  it('returns null when nothing changed', () => {
    expect(updateFromDraft(rec.id, orig, draftFromRecord(rec))).toBeNull();
    // Whitespace around the summary is not a change.
    expect(updateFromDraft(rec.id, orig, { ...orig, summary: '  Use SQLite ' })).toBeNull();
  });
  it('sends only the changed fields', () => {
    const u = updateFromDraft(rec.id, orig, { ...orig, summary: 'Use SQLite with WAL', importance: 2.5 });
    expect(u).toEqual({ id: 'mem_1', summary: 'Use SQLite with WAL', importance: 2.5 });
  });
  it('replaces links with setPaths, including clearing them', () => {
    expect(updateFromDraft(rec.id, orig, { ...orig, latticePaths: [] })).toEqual({ id: 'mem_1', latticePaths: [], setPaths: true });
    expect(updateFromDraft(rec.id, orig, { ...orig, latticePaths: ['project/architecture/db', 'self'] })).toMatchObject({ setPaths: true, latticePaths: ['project/architecture/db', 'self'] });
  });
  it('does not treat a reordering of paths as a change', () => {
    const two = { ...orig, latticePaths: ['a', 'b'] };
    expect(updateFromDraft(rec.id, two, { ...two, latticePaths: ['b', 'a'] })).toBeNull();
  });
  it('sends an emptied list so it can be cleared', () => {
    expect(updateFromDraft(rec.id, orig, { ...orig, decisions: [] })).toEqual({ id: 'mem_1', decisions: [] });
  });
  it('detects project, kind and provenance edits', () => {
    expect(updateFromDraft(rec.id, orig, { ...orig, project: '', kind: ' lesson ', provenance: 'HUMAN_CONFIRMED' })).toEqual({
      id: 'mem_1', project: '', kind: 'lesson', provenance: 'HUMAN_CONFIRMED',
    });
  });
});

describe('draft helpers', () => {
  it('snaps odd importance values from older data when editing', () => {
    expect(draftFromRecord({ ...rec, importance: 2.2 }).importance).toBe(2.5);
  });
  it('tolerates missing lists on a record', () => {
    const d = draftFromRecord({ ...rec, keywords: null as unknown as string[], latticePaths: undefined as unknown as string[] });
    expect(d.keywords).toEqual([]);
    expect(d.latticePaths).toEqual([]);
  });
  it('requires a summary to save', () => {
    expect(canSave(emptyDraft())).toBe(false);
    expect(canSave(emptyDraft({ summary: '   ' }))).toBe(false);
    expect(canSave(emptyDraft({ summary: 'x' }))).toBe(true);
    expect(canSave(emptyDraft({ summary: 'x', kind: ' ' }))).toBe(false);
  });
  it('builds the save payload from a draft', () => {
    const p = saveFromDraft(emptyDraft({ summary: ' hello ', project: '/p', latticePaths: ['self'], importance: 2.5 }));
    expect(p).toMatchObject({ summary: 'hello', project: '/p', latticePaths: ['self'], importance: 2.5, kind: 'note', provenance: 'HUMAN_CONFIRMED', tier: 'episodic' });
  });
});
