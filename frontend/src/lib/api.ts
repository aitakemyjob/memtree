// Typed wrappers over the backend method table (internal/api/api.go).
// Lists are normalised with `?? []` so a null from the wire can never
// crash a render.
import { getTransport } from './transport';
import type {
  Activity, ActivationHit, AppState, CaptureSession, CaptureTurn, CategoryLink, CategoryNode, Cluster, Explained, FireResult,
  HookResult, HooksStatus, ListArgs, MemoryRecord, ProjectCount, RecallArgs, RecallResult, Settings, UntaggedItem, UpdateArgs, LintReport, PathWarning,
} from './types';

/** Invokes a backend method through the active transport. */
async function call<T>(method: string, args?: unknown): Promise<T> {
  return (await getTransport()).invoke<T>(method, args);
}

const list = async <T>(method: string, args?: unknown): Promise<T[]> => (await call<T[] | null>(method, args)) ?? [];

/** One entry per backend method the UI uses. */
export const api = {
  state: () => call<AppState>('app.state'),
  updateSettings: (s: Settings) => call<Settings>('settings.update', s),
  pickDirectory: () => call<{ path: string }>('app.pickDirectory'),
  activity: () => list<Activity>('activity.list'),
  projects: () => list<ProjectCount>('projects.list'),

  listMemories: (a: ListArgs) => list<MemoryRecord>('memory.list', a),
  getMemory: (id: string) => call<MemoryRecord>('memory.get', { id }),
  recall: async (a: RecallArgs): Promise<RecallResult> => {
    const r = await call<RecallResult | null>('memory.recall', a);
    return { memories: r?.memories ?? [], categories: r?.categories ?? [] };
  },
  saveMemory: (r: Partial<MemoryRecord>) => call<MemoryRecord>('memory.save', r),
  updateMemory: (a: UpdateArgs) => call<MemoryRecord>('memory.update', a),
  pin: (id: string, pinned: boolean) => call<{ ok: boolean }>('memory.pin', { id, pinned }),
  archive: (id: string) => call<{ ok: boolean }>('memory.archive', { id }),
  restore: (id: string) => call<{ ok: boolean }>('memory.restore', { id }),
  remove: (id: string) => call<{ ok: boolean }>('memory.delete', { id }),
  supersede: (id: string, by: string) => call<{ ok: boolean }>('memory.supersede', { id, by }),
  unsupersede: (id: string) => call<{ ok: boolean }>('memory.unsupersede', { id }),
  lineage: (id: string) => list<MemoryRecord>('memory.lineage', { id }),
  touch: (ids: string[]) => call<{ ok: boolean }>('memory.touch', { ids }),
  explain: (id: string) => call<Explained>('memory.explain', { id }),

  nodes: () => list<CategoryNode>('lattice.nodes'),
  links: (category: string) => list<CategoryLink>('lattice.links', { category }),
  untagged: (limit = 200) => list<UntaggedItem>('lattice.untagged', { limit }),
  tag: (id: string, paths: string[]) => call<{ ok: boolean }>('lattice.tag', { id, paths }),
  merge: (from: string, to: string) => call<{ ok: boolean }>('lattice.merge', { from, to }),
  prune: () => call<{ removed: number }>('lattice.prune'),
  ensure: (path: string) => call<{ id: string }>('lattice.ensure', { path }),
  fire: async (text: string, max = 20): Promise<FireResult> => {
    const r = await call<FireResult | null>('lattice.fire', { text, max });
    return { seeds: r?.seeds ?? [], hits: (r?.hits ?? []) as ActivationHit[] };
  },

  lint: async (): Promise<LintReport> => {
    const r = await call<LintReport | null>('lattice.lint');
    return { nearDuplicates: r?.nearDuplicates ?? [], outsideAxes: r?.outsideAxes ?? [], singleUse: r?.singleUse ?? [], weaklyFiled: (r?.weaklyFiled ?? []).map((w) => ({ ...w, paths: w.paths ?? [] })), total: r?.total ?? 0 };
  },
  checkPaths: (paths: string[]) => list<PathWarning>('lattice.checkPaths', { paths }),

  candidates: (a: { project?: string; allProjects?: boolean }) => list<Cluster>('consolidation.candidates', a),
  compress: (ids: string[], gist: Partial<MemoryRecord>) => call<MemoryRecord>('consolidation.compress', { ids, gist }),

  captures: (limit = 200) => list<CaptureSession>('captures.list', { limit }),
  pendingCaptures: (a: { project?: string; allProjects?: boolean }) => list<CaptureSession>('captures.pending', a),
  turns: (session: string) => list<CaptureTurn>('captures.turns', { session }),
  closeCapture: (session: string) => call<{ ok: boolean }>('captures.close', { session }),

  hooksStatus: () => call<HooksStatus>('hooks.status'),
  installHooks: (harness: string, project: string, dryRun: boolean) => call<HookResult>('hooks.install', { harness, project, dryRun }),
  uninstallHooks: (harness: string, project: string) => call<HookResult>('hooks.uninstall', { harness, project }),
};
