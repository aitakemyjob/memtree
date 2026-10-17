import { describe, expect, it } from 'vitest';
import { layout, toElkDirection } from './layout';

describe('layout (ELK)', () => {
  it('places children to the right of their parents without overlap', async () => {
    const nodes = [
      { id: 'a', width: 100, height: 40 },
      { id: 'a/b', width: 100, height: 40 },
      { id: 'a/c', width: 100, height: 40 },
      { id: 'z', width: 100, height: 40 },
    ];
    const edges = [
      { id: 'a->a/b', source: 'a', target: 'a/b' },
      { id: 'a->a/c', source: 'a', target: 'a/c' },
      { id: 'ghost', source: 'a', target: 'missing' },
      { id: 'self', source: 'z', target: 'z' },
    ];
    const pos = await layout(nodes, edges, 'RIGHT');
    expect(pos.size).toBe(4);
    expect(pos.get('a/b')!.x).toBeGreaterThan(pos.get('a')!.x);
    expect(pos.get('a/c')!.x).toBeGreaterThan(pos.get('a')!.x);
    expect(pos.get('a/b')!.y).not.toBe(pos.get('a/c')!.y);
  });
  it('maps direction aliases', () => {
    expect(toElkDirection('TB')).toBe('DOWN');
    expect(toElkDirection(undefined)).toBe('RIGHT');
  });
});
