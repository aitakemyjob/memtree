// Advisory path warnings (lattice.checkPaths): mapping them onto the paths
// being edited and applying their one-click suggestions.
import { normalizePath } from './paths';
import type { PathWarning } from './types';

/** Paths worth checking: normalised, non-empty, de-duplicated. */
export function pendingPaths(paths: string[]): string[] {
  return [...new Set(paths.map(normalizePath).filter(Boolean))];
}

/** Groups warnings by the path they apply to. */
export function warningsByPath(warnings: PathWarning[]): Map<string, PathWarning[]> {
  const m = new Map<string, PathWarning[]>();
  for (const w of warnings) m.set(w.path, [...(m.get(w.path) ?? []), w]);
  return m;
}

/** Only warnings for paths still present (the list may have changed while the request ran). */
export function relevantWarnings(warnings: PathWarning[], paths: string[]): PathWarning[] {
  const live = new Set(pendingPaths(paths));
  return warnings.filter((w) => live.has(w.path));
}

/** Replaces the warned path with its suggestion, dropping a duplicate if the suggestion is already chosen. */
export function applySuggestion(paths: string[], w: PathWarning): string[] {
  if (!w.suggest) return paths;
  const target = normalizePath(w.suggest);
  const out: string[] = [];
  for (const p of paths) {
    const n = p === w.path || normalizePath(p) === w.path ? target : p;
    if (!out.includes(n)) out.push(n);
  }
  return out;
}
