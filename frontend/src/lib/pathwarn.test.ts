import { describe, expect, it } from 'vitest';
import { applySuggestion, pendingPaths, relevantWarnings, warningsByPath } from './pathwarn';

const w = (path: string, suggest?: string) => ({ path, message: `check ${path}`, ...(suggest ? { suggest } : {}) });

describe('path warnings', () => {
  it('normalises and de-duplicates the paths to check', () => {
    expect(pendingPaths(['Project/DB', 'project/db', '///', 'self'])).toEqual(['project/db', 'self']);
  });
  it('groups by path and drops warnings for removed paths', () => {
    const ws = [w('a'), w('a', 'b'), w('c')];
    expect(warningsByPath(ws).get('a')).toHaveLength(2);
    expect(relevantWarnings(ws, ['A', 'z'])).toEqual([w('a'), w('a', 'b')]);
  });
  it('swaps a path for its suggestion in place', () => {
    expect(applySuggestion(['x', 'project/database', 'y'], w('project/database', 'project/db'))).toEqual(['x', 'project/db', 'y']);
  });
  it('does not duplicate a suggestion that is already chosen, and ignores warnings without one', () => {
    expect(applySuggestion(['project/db', 'project/database'], w('project/database', 'project/db'))).toEqual(['project/db']);
    expect(applySuggestion(['a'], w('a'))).toEqual(['a']);
  });
});
