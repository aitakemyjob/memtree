// Loads a handful of memories by id (e.g. the hits of one retrieval),
// tolerating ids that no longer exist.
import { api } from './api';
import { useStore } from './store';
import { useAsync } from '../components/ui';
import type { MemoryRecord } from './types';

/** Fetches each id; missing memories are simply absent from the map. */
export function useMemoriesById(ids: string[]) {
  const v = useStore((s) => s.versions.memory);
  return useAsync(async () => {
    const recs = await Promise.all(ids.map((id) => api.getMemory(id).catch(() => undefined)));
    return new Map(recs.filter((r): r is MemoryRecord => !!r).map((r) => [r.id, r]));
  }, [ids.join(','), v]);
}
