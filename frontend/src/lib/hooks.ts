import { useEffect, useState } from 'react';
import { api } from './api';
import { pendingPaths, relevantWarnings } from './pathwarn';
import type { PathWarning } from './types';

/** Returns `value` after it has been stable for `ms`. */
export function useDebounced<T>(value: T, ms = 220): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return v;
}

/** A clock that re-renders every `ms` so ages and activation stay current. */
export function useNow(ms = 60_000): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), ms);
    return () => clearInterval(t);
  }, [ms]);
  return now;
}

/**
 * Debounced lattice.checkPaths for the paths being edited. Advisory only:
 * failures yield no warnings, and nothing here can block saving.
 */
export function usePathWarnings(paths: string[], ms = 300): PathWarning[] {
  const key = pendingPaths(paths).join('\n');
  const [found, setFound] = useState<PathWarning[]>([]);
  useEffect(() => {
    if (!key) {
      setFound([]);
      return;
    }
    let alive = true;
    const t = setTimeout(() => {
      api.checkPaths(key.split('\n')).then((w) => alive && setFound(w), () => alive && setFound([]));
    }, ms);
    return () => {
      alive = false;
      clearTimeout(t);
    };
  }, [key, ms]);
  return relevantWarnings(found, paths);
}
