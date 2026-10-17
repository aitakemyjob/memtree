// Converts the lattice (flat CategoryNode list with parent ids) into
// React Flow nodes/edges. ELK positions them (see layout.ts); this module
// only decides sizes, emphasis and edges so it stays pure and testable.
import type { Edge, Node } from '@xyflow/react';
import { clamp } from './format';
import { textWidth } from './layout';
import type { ActivationHit, CategoryNode } from './types';

export type Emphasis = 'seed' | 'via' | 'hit' | 'selected' | 'match' | 'dim' | 'none';

export interface LatticeNodeData extends Record<string, unknown> {
  id: string;
  label: string;
  linkCount: number;
  activation: number;
  axisRoot: boolean;
  emphasis: Emphasis;
  width: number;
  height: number;
  /** Visual opacity derived from activation (0.35..1). */
  opacity: number;
}

/** Node box size: wide enough for the label, taller with more links. */
export function nodeSize(label: string, linkCount: number): { width: number; height: number } {
  const width = clamp(Math.round(textWidth(label, 12.5) + 64 + Math.sqrt(linkCount) * 6), 104, 260);
  const height = clamp(Math.round(34 + Math.sqrt(linkCount) * 5), 34, 70);
  return { width, height };
}

/** Maps activation [0.15,1] onto an opacity of [0.4,1] so cold nodes fade but stay legible. */
export function activationOpacity(a: number): number {
  return clamp(0.4 + ((a - 0.15) / 0.85) * 0.6, 0.4, 1);
}

/** Parent-to-child edges; a node whose parent is missing becomes a root. */
export function latticeEdges(nodes: CategoryNode[]): { id: string; source: string; target: string }[] {
  const ids = new Set(nodes.map((n) => n.id));
  return nodes.filter((n) => n.parentId && ids.has(n.parentId)).map((n) => ({ id: `${n.parentId}->${n.id}`, source: n.parentId, target: n.id }));
}

export interface HighlightInput {
  seeds: string[];
  hits: ActivationHit[];
  selected?: string;
  /** Lower-cased search text; matching nodes get "match". */
  query?: string;
  /** Exact node ids to spotlight (e.g. from the lint panel); the rest dim. */
  focus?: string[];
}

/**
 * Decides each node's emphasis. A probe takes precedence: seeds (the nodes
 * the text fired), then categories that contributed hits, with everything
 * else dimmed. Without a probe, a search dims non-matches and selection
 * outlines one node.
 */
export function emphasisMap(nodes: CategoryNode[], h: HighlightInput): Map<string, Emphasis> {
  const out = new Map<string, Emphasis>();
  const probing = h.seeds.length > 0 || h.hits.length > 0;
  const via = new Set(h.hits.map((x) => x.viaCategory));
  const seeds = new Set(h.seeds);
  const q = (h.query ?? '').trim().toLowerCase();
  const focus = new Set(h.focus ?? []);
  for (const n of nodes) {
    let e: Emphasis = 'none';
    if (probing) e = seeds.has(n.id) ? 'seed' : via.has(n.id) ? 'via' : 'dim';
    else if (focus.size > 0) e = focus.has(n.id) ? 'match' : 'dim';
    else if (q) e = n.id.toLowerCase().includes(q) ? 'match' : 'dim';
    if (n.id === h.selected && e !== 'dim') e = e === 'none' ? 'selected' : e;
    out.set(n.id, e);
  }
  return out;
}

/** Builds React Flow nodes (positions filled in later) and edges. */
export function buildFlow(
  nodes: CategoryNode[], axisRoots: string[], emphasis: Map<string, Emphasis>, positions?: Map<string, { x: number; y: number }>,
): { nodes: Node<LatticeNodeData>[]; edges: Edge[] } {
  const roots = new Set(axisRoots);
  const flowNodes = nodes.map<Node<LatticeNodeData>>((n) => {
    const size = nodeSize(n.label, n.linkCount);
    const pos = positions?.get(n.id) ?? { x: 0, y: 0 };
    return {
      id: n.id,
      type: 'category',
      position: pos,
      width: size.width,
      height: size.height,
      selected: emphasis.get(n.id) === 'selected',
      data: {
        id: n.id, label: n.label, linkCount: n.linkCount, activation: n.activation, axisRoot: !n.parentId && roots.has(n.id),
        emphasis: emphasis.get(n.id) ?? 'none', ...size, opacity: activationOpacity(n.activation),
      },
    };
  });
  const flowEdges = latticeEdges(nodes).map<Edge>((e) => {
    const lit = emphasis.get(e.target) === 'via' || emphasis.get(e.target) === 'seed' || emphasis.get(e.source) === 'seed';
    const dim = emphasis.get(e.source) === 'dim' && emphasis.get(e.target) === 'dim';
    return { ...e, type: 'smoothstep', animated: false, className: lit ? 'edge-lit' : dim ? 'edge-dim' : undefined };
  });
  return { nodes: flowNodes, edges: flowEdges };
}

/** Total memories linked beneath a node (itself plus all descendants), for the inspector. */
export function subtreeLinkCount(nodes: CategoryNode[], id: string): number {
  return nodes.filter((n) => n.id === id || n.id.startsWith(id + '/')).reduce((s, n) => s + n.linkCount, 0);
}

/** Direct children of a node. */
export function childrenOf(nodes: CategoryNode[], id: string): CategoryNode[] {
  return nodes.filter((n) => n.parentId === id);
}
