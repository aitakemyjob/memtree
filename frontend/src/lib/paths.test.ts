import { describe, expect, it } from 'vitest';
import { addPath, ancestors, normalizePath, suggestPaths } from './paths';
import type { CategoryNode } from './types';

const node = (id: string, linkCount = 0): CategoryNode => ({
  id, parentId: id.includes('/') ? id.slice(0, id.lastIndexOf('/')) : '', label: id.split('/').pop()!, createdAt: '', accessCount: 0, lastAccessed: '', linkCount, activation: 0.5,
});

const nodes = [node('project'), node('project/architecture', 3), node('project/architecture/db', 5), node('project/decisions', 1), node('self'), node('knowledge/tools/database', 2)];

describe('normalizePath (port of store.NormalizePath)', () => {
  it('lowercases, strips junk and empty segments', () => {
    expect(normalizePath('Self / Reflection!!')).toBe('self/reflection');
    expect(normalizePath('/project//Architecture/')).toBe('project/architecture');
    expect(normalizePath('  DB_v2 -x ')).toBe('db_v2-x');
    expect(normalizePath('///')).toBe('');
    expect(normalizePath('café/ünï')).toBe('caf/n');
  });
});

describe('suggestPaths', () => {
  it('ranks exact, then prefix, then segment prefix, then substring', () => {
    const r = suggestPaths(nodes, 'db').map((s) => s.path);
    expect(r[0]).toBe('project/architecture/db'); // segment prefix
    expect(suggestPaths(nodes, 'data').map((s) => s.path)[0]).toBe('knowledge/tools/database');
    expect(suggestPaths(nodes, 'ase').map((s) => s.path)).toContain('knowledge/tools/database'); // substring
    expect(suggestPaths(nodes, 'project').map((s) => s.path).slice(0, 2)).toEqual(['project', 'project/architecture/db']); // prefix ties: busiest first
    expect(suggestPaths(nodes, 'chit').map((s) => s.path).slice(0, 2)).toEqual(['project/architecture/db', 'project/architecture']);
  });
  it('offers the typed path as new only when it does not exist', () => {
    const fresh = suggestPaths(nodes, 'Project/Brand New');
    expect(fresh[fresh.length - 1]).toEqual({ path: 'project/brandnew', isNew: true, linkCount: 0 });
    expect(suggestPaths(nodes, 'self').some((s) => s.isNew)).toBe(false);
  });
  it('lists the busiest nodes for empty input and honours limit and exclude', () => {
    const r = suggestPaths(nodes, '', 3, ['project/architecture/db']);
    expect(r).toHaveLength(3);
    expect(r.map((s) => s.path)).not.toContain('project/architecture/db');
    expect(r[0].path).toBe('project/architecture');
    expect(r.some((s) => s.isNew)).toBe(false);
  });
  it('does not suggest creating something already chosen', () => {
    expect(suggestPaths(nodes, 'zzz', 8, ['zzz']).some((s) => s.isNew)).toBe(false);
  });
});

describe('addPath / ancestors', () => {
  it('normalises, de-duplicates and ignores blanks', () => {
    expect(addPath(['a'], 'B')).toEqual(['a', 'b']);
    expect(addPath(['a'], ' A ')).toEqual(['a']);
    expect(addPath(['a'], '///')).toEqual(['a']);
  });
  it('lists ancestors shallowest first', () => {
    expect(ancestors('a/b/c')).toEqual(['a', 'a/b']);
    expect(ancestors('a')).toEqual([]);
  });
});
