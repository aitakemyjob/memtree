import { describe, expect, it } from 'vitest';
import { factorsFromScore, factorTerms, highlightQuery, hitReason, originLabel, usesByMemory } from './traceExplain';
import type { TraceFactors, TraceHit } from './types';

const f = (over: Partial<TraceFactors> = {}): TraceFactors => ({ text: 0, link: 0, importance: 1, activation: 1, projectBoost: 1, bothBonus: 1, final: 1, ...over });
const hit = (over: Partial<TraceHit> = {}): TraceHit => ({ memoryId: 'mem_a', rank: 1, route: 'text', depth: 0, factors: f(), ...over });

describe('factorTerms', () => {
  it('names only the terms that moved the score', () => {
    expect(factorTerms(f({ text: 0.8 })).map((t) => t.label)).toEqual(['text', 'importance']);
    expect(factorTerms(f({ link: 0.5, activation: 0.4, projectBoost: 1.25 })).map((t) => `${t.label} ${t.value}`)).toEqual(['link 0.50', 'importance 1.0', 'activation 0.40', 'project 1.25']);
  });
  it('uses the stronger route when both found it, and shows the agreement bonus', () => {
    const terms = factorTerms(f({ text: 0.3, link: 0.6, bothBonus: 1.15 }));
    expect(terms[0]).toMatchObject({ label: 'link', value: '0.60' });
    expect(terms.at(-1)).toMatchObject({ label: 'both', value: '1.15' });
  });
  it('reads live recall scores the same way', () => {
    expect(factorsFromScore({ bm25Normalized: 0.8, importanceOffset: 1.5, activation: 0.5, projectBoost: 0, final: 0.6 })).toEqual(f({ text: 0.8, importance: 1.5, activation: 0.5, final: 0.6 }));
  });
});

describe('highlightQuery', () => {
  it('marks matched tokens case-insensitively and keeps the rest verbatim', () => {
    const segs = highlightQuery('Why does Redis evict keys?', [{ token: 'redis', category: 'knowledge/tools/redis' }, { token: 'redis', category: 'project/cache' }]);
    expect(segs).toEqual([
      { text: 'Why does ', categories: [] },
      { text: 'Redis', categories: ['knowledge/tools/redis', 'project/cache'] },
      { text: ' evict keys?', categories: [] },
    ]);
    expect(highlightQuery('nothing here', [])).toEqual([{ text: 'nothing here', categories: [] }]);
  });
});

describe('reasons and labels', () => {
  it('explains each route', () => {
    expect(hitReason(hit())).toBe('matched the wording');
    expect(hitReason(hit({ route: 'lattice', via: 'a/b' }))).toBe('filed under a/b');
    expect(hitReason(hit({ route: 'both', via: 'a/b/c', seed: 'a', depth: 2 }))).toBe('matched the wording and filed under a/b/c, reached from a (2 hops)');
    expect(hitReason(hit({ route: 'anchor', via: 'pinned' }))).toMatch(/pinned/);
  });
  it('labels origins', () => {
    expect(originLabel({ origin: 'mcp', tool: 'recall', harness: 'claude-code' })).toBe('claude-code · recall');
    expect(originLabel({ origin: 'ui', tool: 'memory.recall', harness: 'memtree' })).toBe('UI · memory.recall');
  });
  it('groups uses by memory', () => {
    const m = usesByMemory([{ eventId: 'e', memoryId: 'a', evidence: 'fetched', at: '' }, { eventId: 'e', memoryId: 'a', evidence: 'cited', at: '' }]);
    expect(m.get('a')?.map((u) => u.evidence)).toEqual(['fetched', 'cited']);
  });
});
