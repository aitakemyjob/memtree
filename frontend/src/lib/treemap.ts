// Squarified treemap layout for the Timeline view: project -> kind -> memory.
import { hierarchy, treemap, treemapSquarify } from 'd3-hierarchy';
import { memoryActivation, DEFAULT_FLOOR } from './activation';
import { ageHours, clamp, firstLine, projectName } from './format';
import type { HeatRow, MemoryRecord } from './types';

export type SizeBy = 'importance' | 'equal';

export interface Cell {
  /** Unique key: path of group keys, or the memory id. */
  key: string;
  name: string;
  type: 'project' | 'kind' | 'memory';
  depth: number;
  x0: number;
  y0: number;
  x1: number;
  y1: number;
  /** Area weight (sum for groups). */
  size: number;
  /** Heat in [0,1]: 0 cold, 1 hot (mean of children for groups). */
  heat: number;
  /** Memory id for leaves. */
  id?: string;
  record?: MemoryRecord;
  /** Human-readable metric for tooltips, e.g. "activation 0.62". */
  metric?: string;
}

interface Node {
  key: string;
  name: string;
  type: Cell['type'];
  children?: Node[];
  size: number;
  heat: number;
  record?: MemoryRecord;
  metric?: string;
}

/** What a heat mode may read besides the record. */
export interface HeatContext {
  nowMs: number;
  /** Age of the oldest memory in view. */
  maxAgeHours: number;
  /** Retrieval counts by memory id (empty unless a mode needs traces). */
  traces: Map<string, HeatRow>;
  /** The most any memory in view was retrieved. */
  maxRetrieved: number;
}

/** One way to colour the heatmap. Add a mode by adding an entry to HEAT_MODES. */
export interface HeatMode {
  label: string;
  /** Legend text for the cold and hot ends. */
  legend: [string, string];
  /** The mode reads retrieval traces, so the view must load them. */
  needsTraces?: boolean;
  /** Maps a record to heat in [0,1] and a tooltip metric. */
  heat(r: MemoryRecord, ctx: HeatContext): { heat: number; metric: string };
}

/**
 * The colour modes. Activation maps [floor,1] onto [0,1]; age maps "now" to
 * 1 and the oldest memory in view to 0; importance maps 1..2.5 onto 0..1;
 * retrieved is log-scaled against the most retrieved memory; used is the
 * share of retrievals with evidence the agent relied on the memory.
 */
export const HEAT_MODES = {
  activation: {
    label: 'Activation',
    legend: ['cold (faded)', 'hot (recently used)'],
    heat: (r, { nowMs }) => {
      const a = memoryActivation(r, nowMs);
      return { heat: clamp((a - DEFAULT_FLOOR) / (1 - DEFAULT_FLOOR), 0, 1), metric: `activation ${a.toFixed(2)}` };
    },
  },
  age: {
    label: 'Age',
    legend: ['oldest', 'newest'],
    heat: (r, { nowMs, maxAgeHours }) => {
      const h = ageHours(r.createdAt, nowMs);
      return { heat: maxAgeHours > 0 ? clamp(1 - h / maxAgeHours, 0, 1) : 1, metric: `created ${h < 48 ? Math.round(h) + ' h' : Math.round(h / 24) + ' d'} ago` };
    },
  },
  importance: {
    label: 'Importance',
    legend: ['routine', 'lesson'],
    heat: (r) => ({ heat: clamp((r.importance - 1) / 1.5, 0, 1), metric: `importance ${r.importance}` }),
  },
  retrieved: {
    label: 'Retrieved',
    legend: ['never retrieved', 'most retrieved'],
    needsTraces: true,
    heat: (r, { traces, maxRetrieved }) => {
      const n = traces.get(r.id)?.retrieved ?? 0;
      return { heat: n > 0 && maxRetrieved > 0 ? Math.log1p(n) / Math.log1p(maxRetrieved) : 0, metric: `retrieved ${n}×` };
    },
  },
  used: {
    label: 'Used',
    legend: ['never used', 'used every time'],
    needsTraces: true,
    heat: (r, { traces }) => {
      const t = traces.get(r.id);
      if (!t?.retrieved) return { heat: 0, metric: 'never retrieved' };
      return { heat: t.used / t.retrieved, metric: `used ${t.used} of ${t.retrieved} retrievals` };
    },
  },
} satisfies Record<string, HeatMode>;

export type ColorBy = keyof typeof HEAT_MODES;

/** Builds the context heat modes read, from the records in view and their retrieval counts. */
export function heatContext(live: MemoryRecord[], nowMs: number, traces: HeatRow[] = []): HeatContext {
  const ids = new Set(live.map((r) => r.id));
  const inView = traces.filter((t) => ids.has(t.memoryId));
  return {
    nowMs,
    maxAgeHours: live.reduce((m, r) => Math.max(m, ageHours(r.createdAt, nowMs)), 0),
    traces: new Map(inView.map((t) => [t.memoryId, t])),
    maxRetrieved: inView.reduce((m, t) => Math.max(m, t.retrieved), 0),
  };
}

/** Normalises a record to a 0..1 heat value for the chosen colour mode. */
export function heatFor(mode: ColorBy, r: MemoryRecord, ctx: HeatContext): { heat: number; metric: string } {
  return HEAT_MODES[mode].heat(r, ctx);
}

/** Builds the project -> kind -> memory tree. Superseded/archived records are skipped. */
export function buildTree(memories: MemoryRecord[], nowMs: number, colorBy: ColorBy, sizeBy: SizeBy, traces: HeatRow[] = []): Node {
  const live = memories.filter((m) => !m.archivedAt && !m.supersededBy);
  const ctx = heatContext(live, nowMs, traces);
  const projects = new Map<string, Map<string, MemoryRecord[]>>();
  for (const r of live) {
    const byKind = projects.get(r.project) ?? new Map<string, MemoryRecord[]>();
    byKind.set(r.kind, [...(byKind.get(r.kind) ?? []), r]);
    projects.set(r.project, byKind);
  }
  const root: Node = { key: '', name: 'root', type: 'project', size: 0, heat: 0, children: [] };
  for (const [project, kinds] of [...projects].sort((a, b) => a[0].localeCompare(b[0]))) {
    const pNode: Node = { key: `p:${project}`, name: projectName(project), type: 'project', size: 0, heat: 0, children: [] };
    for (const [kind, recs] of [...kinds].sort((a, b) => a[0].localeCompare(b[0]))) {
      const kNode: Node = { key: `p:${project}/k:${kind}`, name: kind, type: 'kind', size: 0, heat: 0, children: [] };
      for (const r of recs) {
        const { heat, metric } = heatFor(colorBy, r, ctx);
        kNode.children!.push({ key: r.id, name: firstLine(r.summary), type: 'memory', size: sizeBy === 'importance' ? r.importance : 1, heat, record: r, metric });
      }
      pNode.children!.push(kNode);
    }
    root.children!.push(pNode);
  }
  return root;
}

/**
 * Lays out a squarified treemap. Area is the importance (or 1 each); group
 * cells reserve a header strip for their label.
 */
export function layoutMemories(
  memories: MemoryRecord[], nowMs: number, width: number, height: number, opts: { colorBy?: ColorBy; sizeBy?: SizeBy; traces?: HeatRow[] } = {},
): Cell[] {
  const tree = buildTree(memories, nowMs, opts.colorBy ?? 'activation', opts.sizeBy ?? 'importance', opts.traces);
  if (!tree.children?.length || width <= 0 || height <= 0) return [];
  const h = hierarchy<Node>(tree, (d) => d.children)
    .sum((d) => (d.children ? 0 : d.size))
    .sort((a, b) => (b.value ?? 0) - (a.value ?? 0));
  const laid = treemap<Node>()
    .tile(treemapSquarify.ratio(1.3))
    .size([width, height])
    .paddingOuter(3)
    .paddingTop((d) => (d.depth === 0 || d.data.type === 'memory' ? 0 : 20))
    .paddingInner(2)
    .round(true)(h);
  const cells: Cell[] = [];
  laid.each((n) => {
    if (n.depth === 0) return;
    const leaves = n.leaves();
    const heat = leaves.length ? leaves.reduce((s, l) => s + l.data.heat, 0) / leaves.length : 0;
    cells.push({
      key: n.data.key, name: n.data.name, type: n.data.type, depth: n.depth,
      x0: n.x0, y0: n.y0, x1: n.x1, y1: n.y1, size: n.value ?? 0, heat,
      ...(n.data.record ? { id: n.data.record.id, record: n.data.record, metric: n.data.metric } : {}),
    });
  });
  return cells;
}

/** Whether a colour mode reads retrieval traces. */
export const modeNeedsTraces = (mode: ColorBy): boolean => (HEAT_MODES[mode] as HeatMode).needsTraces === true;
