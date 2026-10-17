// Category path helpers: normalisation (port of store.NormalizePath) and
// the autocomplete used by every path input.
import type { CategoryNode } from './types';

/**
 * Lowercases a category path, strips anything but [a-z0-9_-] from each
 * segment and drops empty segments, exactly like the backend:
 * "Self / Reflection!!" becomes "self/reflection".
 */
export function normalizePath(path: string): string {
  return path
    .toLowerCase()
    .split('/')
    .map((s) => s.trim().replace(/[^a-z0-9_-]/g, ''))
    .filter(Boolean)
    .join('/');
}

export interface PathSuggestion {
  /** Normalised path. */
  path: string;
  /** True when the path is not in the lattice yet (it would be created). */
  isNew: boolean;
  linkCount: number;
}

/**
 * Suggests lattice paths for typed text. Ranking: exact, id prefix, a
 * segment prefix (typing "db" finds project/architecture/db), then
 * substring; ties break toward better-used and shorter paths. When the
 * normalised input is not an existing node it is offered last as a new path.
 * Paths in `exclude` are skipped (already chosen).
 */
export function suggestPaths(nodes: CategoryNode[], input: string, limit = 8, exclude: string[] = []): PathSuggestion[] {
  const q = normalizePath(input);
  const skip = new Set(exclude);
  const scored: { n: CategoryNode; rank: number }[] = [];
  for (const n of nodes) {
    if (skip.has(n.id)) continue;
    let rank = -1;
    if (!q) rank = 4;
    else if (n.id === q) rank = 0;
    else if (n.id.startsWith(q)) rank = 1;
    else if (n.id.split('/').some((seg) => seg.startsWith(q))) rank = 2;
    else if (n.id.includes(q)) rank = 3;
    if (rank >= 0) scored.push({ n, rank });
  }
  scored.sort((a, b) => a.rank - b.rank || b.n.linkCount - a.n.linkCount || a.n.id.length - b.n.id.length || a.n.id.localeCompare(b.n.id));
  const out: PathSuggestion[] = scored.slice(0, limit).map(({ n }) => ({ path: n.id, isNew: false, linkCount: n.linkCount }));
  if (q && !skip.has(q) && !nodes.some((n) => n.id === q)) {
    out.splice(Math.min(out.length, limit), 0, { path: q, isNew: true, linkCount: 0 });
  }
  return out;
}

/** Adds a path to a list if it normalises to something and is not already there. */
export function addPath(paths: string[], raw: string): string[] {
  const p = normalizePath(raw);
  return p && !paths.includes(p) ? [...paths, p] : paths;
}

/** All ancestors of a path, shallowest first: "a/b/c" gives ["a","a/b"]. */
export function ancestors(path: string): string[] {
  const segs = path.split('/');
  return segs.slice(0, -1).map((_, i) => segs.slice(0, i + 1).join('/'));
}
