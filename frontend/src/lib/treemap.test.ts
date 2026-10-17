import { describe, expect, it } from 'vitest';
import { buildTree, heatFor, layoutMemories } from './treemap';
import type { MemoryRecord } from './types';

const NOW = Date.parse('2026-03-01T12:00:00Z');
const DAY = 86_400_000;
const iso = (ms: number) => new Date(ms).toISOString();

function rec(id: string, over: Partial<MemoryRecord> = {}): MemoryRecord {
  return {
    id, createdAt: iso(NOW - DAY), updatedAt: iso(NOW), project: '/code/app', harness: 'ui', tier: 'episodic', kind: 'note', summary: `Summary of ${id}`,
    keywords: [], decisions: [], openLoops: [], importance: 1, provenance: 'HUMAN_CONFIRMED', pinned: false, accessLog: [iso(NOW - DAY)], latticePaths: [], ...over,
  };
}

describe('buildTree', () => {
  it('groups by project then kind and skips archived/superseded', () => {
    const t = buildTree(
      [rec('a'), rec('b', { kind: 'lesson' }), rec('c', { project: '' }), rec('d', { archivedAt: iso(NOW) }), rec('e', { supersededBy: 'a' })],
      NOW, 'activation', 'importance',
    );
    const names = t.children!.map((p) => p.name);
    expect(names).toEqual(['Global', 'app']);
    const app = t.children!.find((p) => p.name === 'app')!;
    expect(app.children!.map((k) => k.name)).toEqual(['lesson', 'note']);
    const ids = t.children!.flatMap((p) => p.children!.flatMap((k) => k.children!.map((m) => m.key)));
    expect(ids.sort()).toEqual(['a', 'b', 'c']);
  });
  it('distinguishes two projects that share a basename', () => {
    const t = buildTree([rec('a', { project: '/x/app' }), rec('b', { project: '/y/app' })], NOW, 'age', 'equal');
    expect(t.children).toHaveLength(2);
    expect(new Set(t.children!.map((c) => c.key)).size).toBe(2);
  });
});

describe('heatFor', () => {
  it('maps activation, age and importance to 0..1', () => {
    expect(heatFor('importance', rec('a', { importance: 1 }), NOW, 10).heat).toBe(0);
    expect(heatFor('importance', rec('a', { importance: 2.5 }), NOW, 10).heat).toBe(1);
    expect(heatFor('importance', rec('a', { importance: 1.5 }), NOW, 10).heat).toBeCloseTo(1 / 3);
    const fresh = heatFor('age', rec('a', { createdAt: iso(NOW) }), NOW, 24 * 10).heat;
    const old = heatFor('age', rec('a', { createdAt: iso(NOW - 10 * DAY) }), NOW, 24 * 10).heat;
    expect(fresh).toBe(1);
    expect(old).toBe(0);
    const pinned = heatFor('activation', rec('a', { pinned: true }), NOW, 0).heat;
    expect(pinned).toBe(1);
    const cold = heatFor('activation', rec('a', { accessLog: [] }), NOW, 0).heat;
    expect(cold).toBe(0);
  });
});

describe('layoutMemories', () => {
  const memories = [
    rec('a', { importance: 2.5 }), rec('b'), rec('c', { kind: 'decision', importance: 1.5 }), rec('d', { project: '' }),
  ];

  it('emits project, kind and memory cells inside the bounds', () => {
    const cells = layoutMemories(memories, NOW, 800, 500);
    expect(cells.filter((c) => c.type === 'memory').map((c) => c.id).sort()).toEqual(['a', 'b', 'c', 'd']);
    expect(cells.some((c) => c.type === 'project')).toBe(true);
    expect(cells.some((c) => c.type === 'kind')).toBe(true);
    for (const c of cells) {
      expect(c.x0).toBeGreaterThanOrEqual(0);
      expect(c.y0).toBeGreaterThanOrEqual(0);
      expect(c.x1).toBeLessThanOrEqual(800);
      expect(c.y1).toBeLessThanOrEqual(500);
      expect(c.x1).toBeGreaterThanOrEqual(c.x0);
    }
  });

  it('gives higher-importance memories more area; equal mode evens them out', () => {
    const area = (cells: ReturnType<typeof layoutMemories>, id: string) => {
      const c = cells.find((x) => x.id === id)!;
      return (c.x1 - c.x0) * (c.y1 - c.y0);
    };
    const same = [rec('big', { importance: 2.5 }), rec('small')];
    const byImp = layoutMemories(same, NOW, 600, 400);
    expect(area(byImp, 'big')).toBeGreaterThan(area(byImp, 'small') * 1.5);
    const equal = layoutMemories(same, NOW, 600, 400, { sizeBy: 'equal' });
    expect(Math.abs(area(equal, 'big') - area(equal, 'small'))).toBeLessThan(area(equal, 'small') * 0.2);
  });

  it('rolls leaf heat up to the group (mean)', () => {
    const cells = layoutMemories([rec('hot', { importance: 2.5 }), rec('cold')], NOW, 600, 400, { colorBy: 'importance', sizeBy: 'equal' });
    const kind = cells.find((c) => c.type === 'kind')!;
    expect(kind.heat).toBeCloseTo(0.5);
  });

  it('returns nothing for empty input or a zero-size canvas', () => {
    expect(layoutMemories([], NOW, 800, 500)).toEqual([]);
    expect(layoutMemories(memories, NOW, 0, 0)).toEqual([]);
    expect(layoutMemories([rec('x', { archivedAt: iso(NOW) })], NOW, 800, 500)).toEqual([]);
  });
});
