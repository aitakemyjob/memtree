import { describe, expect, it } from 'vitest';
import {
  activationCurve, activationRaw, activationScore, compressInDays, daysUntilBelow, EPISODIC_PARAMS, memoryActivation, nodeActivation, paramsFor, SEMANTIC_PARAMS,
} from './activation';
import type { MemoryRecord, TierConfig } from './types';

const NOW = Date.parse('2026-03-01T12:00:00Z');
const H = 3_600_000;
const D = 24 * H;
const iso = (ms: number) => new Date(ms).toISOString();

function rec(over: Partial<MemoryRecord> = {}): MemoryRecord {
  return {
    id: 'mem_1', createdAt: iso(NOW - 100 * D), updatedAt: iso(NOW), project: '', harness: 'ui', tier: 'episodic', kind: 'note', summary: 's',
    keywords: [], decisions: [], openLoops: [], importance: 1, provenance: 'HUMAN_CONFIRMED', pinned: false, accessLog: [iso(NOW - 100 * D)],
    latticePaths: [], ...over,
  };
}

const tiers: TierConfig = { ageHoursToSemantic: 72, ageHoursToCompact: 720, activationCeilingToCompress: 0.5, minClusterSize: 2 };

describe('activationScore (port of Go ActivationScore)', () => {
  it('is the floor with no accesses', () => {
    expect(activationScore([], EPISODIC_PARAMS, NOW)).toBe(0.15);
    expect(activationRaw([], 0.5, NOW)).toBe(-Infinity);
  });

  it('clamps age to a minimum of one hour (a brand new memory is 0.575)', () => {
    // t = max(1h, age) => raw = ln(1) = 0 => logistic(0) = 0.5 => 0.15 + 0.85*0.5
    expect(activationScore([NOW], EPISODIC_PARAMS, NOW)).toBeCloseTo(0.575, 10);
    expect(activationScore([NOW - 30 * 60_000], EPISODIC_PARAMS, NOW)).toBeCloseTo(0.575, 10);
    // An access in the future is also clamped, not boosted.
    expect(activationScore([NOW + 5 * H], EPISODIC_PARAMS, NOW)).toBeCloseTo(0.575, 10);
  });

  it('matches hand-computed values for the episodic exponent (d=0.5)', () => {
    // raw = -0.5*ln(24); logistic(raw/2) = 0.31123..; 0.15 + 0.85*that
    expect(activationScore([NOW - 24 * H], EPISODIC_PARAMS, NOW)).toBeCloseTo(0.4146, 3);
  });

  it('decays slower for semantic memories than episodic at the same age', () => {
    const log = [NOW - 30 * D];
    const e = activationScore(log, EPISODIC_PARAMS, NOW);
    const s = activationScore(log, SEMANTIC_PARAMS, NOW);
    expect(s).toBeGreaterThan(e);
    // Semantic at 30 days: raw = -0.2*ln(720) => 0.15 + 0.85*logistic(raw/2)
    expect(s).toBeCloseTo(0.15 + 0.85 / (1 + Math.exp((0.2 * Math.log(720)) / 2)), 10);
  });

  it('stays within [floor, 1] and grows with more recent accesses', () => {
    const one = activationScore([NOW - 10 * D], EPISODIC_PARAMS, NOW);
    const two = activationScore([NOW - 10 * D, NOW - 1 * H], EPISODIC_PARAMS, NOW);
    expect(two).toBeGreaterThan(one);
    const many = activationScore(Array(50).fill(NOW), EPISODIC_PARAMS, NOW);
    expect(many).toBeLessThanOrEqual(1);
    expect(one).toBeGreaterThanOrEqual(0.15);
  });

  it('treats anything but semantic as episodic', () => {
    expect(paramsFor('semantic')).toBe(SEMANTIC_PARAMS);
    expect(paramsFor('weird')).toBe(EPISODIC_PARAMS);
  });
});

describe('memoryActivation', () => {
  it('is exactly 1 when pinned, regardless of age', () => {
    expect(memoryActivation(rec({ pinned: true }), NOW)).toBe(1);
  });
  it('uses the record tier and ignores malformed log entries', () => {
    const r = rec({ tier: 'semantic', accessLog: [iso(NOW - 24 * H), 'not-a-date'] });
    expect(memoryActivation(r, NOW)).toBeCloseTo(activationScore([NOW - 24 * H], SEMANTIC_PARAMS, NOW), 10);
  });
  it('scores a node from its last access', () => {
    const fresh = nodeActivation(iso(NOW), NOW);
    const stale = nodeActivation(iso(NOW - 60 * D), NOW);
    expect(fresh).toBeCloseTo(0.575, 10);
    expect(stale).toBeLessThan(fresh);
    expect(nodeActivation('', NOW)).toBe(0.15);
  });
});

describe('activationCurve', () => {
  it('spans 0..90 days inclusive and never rises', () => {
    const c = activationCurve(rec(), NOW);
    expect(c).toHaveLength(91);
    expect(c[0].day).toBe(0);
    expect(c[90].day).toBe(90);
    for (let i = 1; i < c.length; i++) expect(c[i].value).toBeLessThanOrEqual(c[i - 1].value + 1e-12);
    expect(c[0].value).toBeCloseTo(memoryActivation(rec(), NOW), 10);
  });
  it('is flat at 1 when pinned', () => {
    expect(activationCurve(rec({ pinned: true }), NOW).every((p) => p.value === 1)).toBe(true);
  });
  it('finds when the curve crosses a level', () => {
    const c = activationCurve(rec({ createdAt: iso(NOW), accessLog: [iso(NOW)] }), NOW, 90);
    const d = daysUntilBelow(c, 0.4);
    expect(d).not.toBeNull();
    expect(c[d!].value).toBeLessThanOrEqual(0.4);
    expect(c[d! - 1].value).toBeGreaterThan(0.4);
    expect(daysUntilBelow(c, 0)).toBeNull();
  });
});

describe('compressInDays (mirror of store.ShouldCompress over time)', () => {
  it('is 0 when old and cold enough already', () => {
    expect(compressInDays(rec(), tiers, NOW)).toBe(0);
  });
  it('waits for the age gate even when activation is already low', () => {
    // 1 day old, cold log. Gate is 72h => 2 days from now it becomes 3 days old.
    const r = rec({ createdAt: iso(NOW - 1 * D), accessLog: [iso(NOW - 100 * D)] });
    expect(compressInDays(r, tiers, NOW)).toBe(2);
  });
  it('waits for activation to cool when old but warm', () => {
    const r = rec({ createdAt: iso(NOW - 100 * D), accessLog: [iso(NOW)] });
    const d = compressInDays(r, tiers, NOW);
    expect(d).toBeGreaterThan(0);
    expect(activationScore([NOW], EPISODIC_PARAMS, NOW + d! * D)).toBeLessThanOrEqual(0.5);
    expect(activationScore([NOW], EPISODIC_PARAMS, NOW + (d! - 1) * D)).toBeGreaterThan(0.5);
  });
  it('uses the longer gate for semantic memories', () => {
    const r = rec({ tier: 'semantic', createdAt: iso(NOW - 10 * D), accessLog: [iso(NOW - 100 * D)] });
    // 720h = 30 days; 10 days old => 20 more.
    expect(compressInDays(r, tiers, NOW)).toBe(20);
  });
  it('never compresses pinned, archived or superseded records', () => {
    expect(compressInDays(rec({ pinned: true }), tiers, NOW)).toBeNull();
    expect(compressInDays(rec({ archivedAt: iso(NOW) }), tiers, NOW)).toBeNull();
    expect(compressInDays(rec({ supersededBy: 'x' }), tiers, NOW)).toBeNull();
  });
});
