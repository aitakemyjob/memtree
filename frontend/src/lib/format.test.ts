import { describe, expect, it } from 'vitest';
import {
  ageHours, ageShort, clamp, durationLabel, findMemoryId, firstLine, fixed, hoursLabel, importanceName, kindColor, pct, plural, projectName, relTime,
  snapImportance, truncate,
} from './format';

const NOW = Date.parse('2026-03-01T12:00:00Z');
const m = 60_000;
const h = 60 * m;
const d = 24 * h;

describe('ageShort / relTime', () => {
  it('buckets from seconds to years', () => {
    expect(ageShort(10_000)).toBe('now');
    expect(ageShort(5 * m)).toBe('5m');
    expect(ageShort(3 * h)).toBe('3h');
    expect(ageShort(3 * d)).toBe('3d');
    expect(ageShort(100 * d)).toBe('3mo');
    expect(ageShort(800 * d)).toBe('2y');
    expect(ageShort(-5 * h)).toBe('now');
  });
  it('formats timestamps and tolerates junk', () => {
    expect(relTime(new Date(NOW - 2 * h).toISOString(), NOW)).toBe('2h ago');
    expect(relTime(new Date(NOW).toISOString(), NOW)).toBe('just now');
    expect(relTime(undefined, NOW)).toBe('');
    expect(relTime('garbage', NOW)).toBe('');
  });
  it('computes age in hours', () => {
    expect(ageHours(new Date(NOW - 36 * h).toISOString(), NOW)).toBeCloseTo(36);
    expect(ageHours('nope', NOW)).toBe(0);
  });
});

describe('importance', () => {
  it('names the three levels and snaps like the backend', () => {
    expect(importanceName(1)).toBe('routine');
    expect(importanceName(1.5)).toBe('decision');
    expect(importanceName(2.5)).toBe('lesson');
    expect(snapImportance(0)).toBe(1);
    expect(snapImportance(1.24)).toBe(1);
    expect(snapImportance(1.25)).toBe(1.5);
    expect(snapImportance(1.99)).toBe(1.5);
    expect(snapImportance(2)).toBe(2.5);
    expect(snapImportance(-3)).toBe(1);
  });
});

describe('misc formatters', () => {
  it('labels projects by their last segment', () => {
    expect(projectName('')).toBe('Global');
    expect(projectName('/home/u/code/memtree')).toBe('memtree');
    expect(projectName('C:\\work\\app\\')).toBe('app');
  });
  it('formats hours', () => {
    expect(hoursLabel(12)).toBe('12 h');
    expect(hoursLabel(72)).toBe('3 days');
    expect(hoursLabel(720)).toBe('30 days');
    expect(hoursLabel(60)).toBe('2.5 days');
  });
  it('formats numbers defensively', () => {
    expect(fixed(1.234)).toBe('1.23');
    expect(fixed(NaN)).toBe('-');
    expect(pct(0.256)).toBe('26%');
    expect(clamp(5, 0, 1)).toBe(1);
    expect(plural(1, 'memory', 'memories')).toBe('1 memory');
    expect(plural(2, 'memory', 'memories')).toBe('2 memories');
    expect(plural(0, 'node')).toBe('0 nodes');
    expect(truncate('abcdef', 4)).toBe('abc…');
    expect(truncate('abc', 4)).toBe('abc');
    expect(firstLine('a\nb')).toBe('a');
    expect(durationLabel(12)).toBe('12 ms');
    expect(durationLabel(1400)).toBe('1.4 s');
    expect(kindColor('decision')).toContain('accent');
  });
  it('finds a memory id inside text', () => {
    expect(findMemoryId('saved mem_0123456789abcdef (decision)')).toBe('mem_0123456789abcdef');
    expect(findMemoryId('no id here')).toBeUndefined();
  });
});
