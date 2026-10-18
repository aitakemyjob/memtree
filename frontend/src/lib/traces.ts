// Data hooks for retrieval traces. Views and panels read traces only
// through these, so fetching and live refresh live in one place: each hook
// refetches when a `trace` bus event bumps the trace version.
import { api } from './api';
import { useStore } from './store';
import { useAsync } from '../components/ui';
import type { TraceFilter } from './types';

/** Recent retrieval events matching the filter, newest first. */
export function useTraceEvents(f: TraceFilter) {
  const v = useStore((s) => s.versions.trace);
  return useAsync(() => api.traces(f), [JSON.stringify(f), v]);
}

/** One retrieval event, or nothing when id is empty. */
export function useTraceEvent(id: string | undefined) {
  const v = useStore((s) => s.versions.trace);
  return useAsync(() => (id ? api.trace(id) : Promise.resolve(undefined)), [id, v]);
}

/** The retrievals that returned one memory, newest first. */
export function useMemoryTrace(id: string, limit = 50) {
  const v = useStore((s) => s.versions.trace);
  return useAsync(() => api.memoryTrace(id, limit), [id, limit, v]);
}

/** Retrieval counts per memory; skipped (resolves empty) while disabled. */
export function useHeat(f: TraceFilter, enabled = true) {
  const v = useStore((s) => s.versions.trace);
  return useAsync(() => (enabled ? api.heat(f) : Promise.resolve([])), [JSON.stringify(f), enabled, v]);
}

/** Memory retrievals per UTC day. */
export function useDaily(f: TraceFilter) {
  const v = useStore((s) => s.versions.trace);
  return useAsync(() => api.daily(f), [JSON.stringify(f), v]);
}
