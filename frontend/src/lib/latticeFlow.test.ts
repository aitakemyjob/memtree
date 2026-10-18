import { describe, expect, it } from 'vitest';
import { activationOpacity, buildFlow, childrenOf, emphasisMap, latticeEdges, nodeSize, subtreeLinkCount } from './latticeFlow';
import type { CategoryNode } from './types';

const node = (id: string, linkCount = 0, activation = 0.5, parentId = id.includes('/') ? id.slice(0, id.lastIndexOf('/')) : ''): CategoryNode => ({
  id, parentId, label: id.split('/').pop()!, createdAt: '', accessCount: 0, lastAccessed: '', linkCount, activation,
});

const nodes = [node('project', 0, 1), node('project/architecture', 4), node('project/architecture/db', 9, 0.9), node('self'), node('orphan/leaf', 1, 0.2, 'orphan')];

describe('latticeEdges', () => {
  it('connects parents to children and skips missing parents', () => {
    const e = latticeEdges(nodes);
    expect(e.map((x) => x.id)).toEqual(['project->project/architecture', 'project/architecture->project/architecture/db']);
  });
});

describe('sizes and opacity', () => {
  it('grows with link count within bounds', () => {
    const small = nodeSize('db', 0);
    const big = nodeSize('db', 100);
    expect(big.width).toBeGreaterThan(small.width);
    expect(big.height).toBeGreaterThan(small.height);
    expect(small.width).toBeGreaterThanOrEqual(104);
    expect(nodeSize('x'.repeat(200), 0).width).toBe(260);
    expect(big.height).toBeLessThanOrEqual(70);
  });
  it('maps activation to opacity 0.4..1', () => {
    expect(activationOpacity(0.15)).toBeCloseTo(0.4);
    expect(activationOpacity(1)).toBeCloseTo(1);
    expect(activationOpacity(5)).toBe(1);
    expect(activationOpacity(0)).toBe(0.4);
  });
});

describe('emphasisMap', () => {
  it('marks seeds, via categories and dims the rest while probing', () => {
    const m = emphasisMap(nodes, {
      seeds: ['project/architecture/db'],
      hits: [{ itemType: 'memory', itemId: 'm1', score: 0.5, viaCategory: 'project/architecture', seed: 'project/architecture/db', depth: 1, summary: '' }],
    });
    expect(m.get('project/architecture/db')).toBe('seed');
    expect(m.get('project/architecture')).toBe('via');
    expect(m.get('self')).toBe('dim');
  });
  it('dims non-matches for a search and outlines the selection', () => {
    const m = emphasisMap(nodes, { seeds: [], hits: [], query: 'DB', selected: 'self' });
    expect(m.get('project/architecture/db')).toBe('match');
    expect(m.get('self')).toBe('dim');
    const sel = emphasisMap(nodes, { seeds: [], hits: [], selected: 'self' });
    expect(sel.get('self')).toBe('selected');
    expect(sel.get('project')).toBe('none');
  });
  it('spotlights exact focus ids and dims the rest', () => {
    const m = emphasisMap(nodes, { seeds: [], hits: [], focus: ['self', 'project'] });
    expect(m.get('self')).toBe('match');
    expect(m.get('project')).toBe('match');
    expect(m.get('project/architecture')).toBe('dim');
  });
  it('keeps a seed marked even when selected', () => {
    const m = emphasisMap(nodes, { seeds: ['self'], hits: [], selected: 'self' });
    expect(m.get('self')).toBe('seed');
  });
});

describe('buildFlow', () => {
  it('produces one flow node per category with axis roots flagged', () => {
    const f = buildFlow(nodes, ['project', 'self'], emphasisMap(nodes, { seeds: [], hits: [] }));
    expect(f.nodes).toHaveLength(5);
    expect(f.edges).toHaveLength(2);
    const byId = new Map(f.nodes.map((n) => [n.id, n]));
    expect(byId.get('project')!.data.axisRoot).toBe(true);
    expect(byId.get('project/architecture')!.data.axisRoot).toBe(false);
    // A parentless node that isn't a configured axis root is not an axis root.
    expect(byId.get('orphan/leaf')!.data.axisRoot).toBe(false);
    expect(byId.get('project/architecture/db')!.data.opacity).toBeGreaterThan(byId.get('orphan/leaf')!.data.opacity);
  });
  it('applies layout positions and lights edges toward probe hits', () => {
    const em = emphasisMap(nodes, { seeds: ['project/architecture/db'], hits: [] });
    const f = buildFlow(nodes, [], em, new Map([['self', { x: 10, y: 20 }]]));
    expect(f.nodes.find((n) => n.id === 'self')!.position).toEqual({ x: 10, y: 20 });
    expect(f.edges.find((e) => e.target === 'project/architecture/db')!.className).toBe('edge-lit');
  });
});

describe('tree queries', () => {
  it('sums links under a subtree and lists children', () => {
    expect(subtreeLinkCount(nodes, 'project')).toBe(13);
    // Not a prefix-only match: "project" must not include "projectx".
    expect(subtreeLinkCount([...nodes, node('projectx', 50)], 'project')).toBe(13);
    expect(childrenOf(nodes, 'project').map((n) => n.id)).toEqual(['project/architecture']);
  });
});
