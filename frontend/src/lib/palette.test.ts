import { describe, expect, it } from 'vitest';
import { fuzzyScore } from './palette';

describe('fuzzyScore', () => {
  it('ranks earlier and shorter substring hits higher', () => {
    expect(fuzzyScore('Settings', 'set')).toBeGreaterThan(fuzzyScore('Toggle light/dark theme', 'set'));
    expect(fuzzyScore('Lattice', 'lat')).toBeGreaterThan(fuzzyScore('Connect an agent (lattice)', 'lat'));
  });
  it('is case-insensitive and treats empty queries as a match', () => {
    expect(fuzzyScore('New memory', 'NEW')).toBeGreaterThan(0);
    expect(fuzzyScore('anything', '')).toBe(1);
  });
  it('accepts mostly-adjacent subsequences but rejects scattered letters', () => {
    expect(fuzzyScore('Install hooks', 'ihk')).toBe(0);
    expect(fuzzyScore('Install hooks', 'inh')).toBeGreaterThan(0);
    expect(fuzzyScore('abc', 'xyz')).toBe(0);
  });
});
