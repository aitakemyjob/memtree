// Turns recorded retrieval traces into display terms. Pure, so the views
// stay presentational and every rule is unit-tested.
import { fixed } from './format';
import type { ScoreDecomposition, TraceEvent, TraceFactors, TraceHit, TraceMatch, TraceUse } from './types';

/** One labelled multiplicand of a hit's score. */
export interface FactorTerm {
  label: string;
  value: string;
  title: string;
}

/**
 * The terms whose product is a hit's final score. Relevance is the larger
 * of the text and link terms (only those that contributed are named);
 * multipliers that are exactly 1 are left out unless they are the only
 * ones, so a reader sees what actually moved the score.
 */
export function factorTerms(f: TraceFactors): FactorTerm[] {
  const out: FactorTerm[] = [];
  if (f.text > 0 && f.link > 0) {
    out.push({ label: f.text >= f.link ? 'text' : 'link', value: fixed(Math.max(f.text, f.link)), title: `Best route: text relevance ${fixed(f.text)}, lattice link ${fixed(f.link)}` });
  } else if (f.text > 0) {
    out.push({ label: 'text', value: fixed(f.text), title: 'Text relevance relative to the best match (BM25)' });
  } else if (f.link > 0) {
    out.push({ label: 'link', value: fixed(f.link), title: 'Lattice link weight x node activation' });
  }
  out.push({ label: 'importance', value: fixed(f.importance, 1), title: 'Importance: 1 routine, 1.5 decision, 2.5 lesson' });
  if (f.activation !== 1) out.push({ label: 'activation', value: fixed(f.activation), title: 'Recency/frequency decay, 0.15 to 1' });
  if (f.projectBoost !== 1) out.push({ label: 'project', value: fixed(f.projectBoost), title: 'Same-project boost' });
  if (f.bothBonus !== 1) out.push({ label: 'both', value: fixed(f.bothBonus), title: 'Text and lattice agreed' });
  return out;
}

/** A run of query text, with the categories it fired if it is a matched token. */
export interface QuerySegment {
  text: string;
  categories: string[];
}

/**
 * Splits a query into plain runs and matched tokens. Tokens match the way
 * the backend extracts keywords: case-insensitive runs of letters and digits.
 */
export function highlightQuery(query: string, matches: TraceMatch[]): QuerySegment[] {
  const byToken = new Map<string, string[]>();
  for (const m of matches) byToken.set(m.token, [...(byToken.get(m.token) ?? []), m.category]);
  const out: QuerySegment[] = [];
  let last = 0;
  const push = (text: string, categories: string[] = []) => {
    if (!text) return;
    const prev = out[out.length - 1];
    if (prev && !prev.categories.length && !categories.length) prev.text += text;
    else out.push({ text, categories });
  };
  for (const m of query.matchAll(/[a-z0-9]+/gi)) {
    const cats = byToken.get(m[0].toLowerCase());
    if (!cats) continue;
    push(query.slice(last, m.index));
    push(m[0], cats);
    last = m.index! + m[0].length;
  }
  push(query.slice(last));
  return out;
}

/** Use evidence grouped by memory id. */
export function usesByMemory(uses: TraceUse[]): Map<string, TraceUse[]> {
  const out = new Map<string, TraceUse[]>();
  for (const u of uses) out.set(u.memoryId, [...(out.get(u.memoryId) ?? []), u]);
  return out;
}

/** A plain-language reason a hit was returned. */
export function hitReason(h: TraceHit): string {
  switch (h.route) {
    case 'text':
      return 'matched the wording';
    case 'lattice':
      return pathReason(h);
    case 'both':
      return `matched the wording and ${pathReason(h)}`;
    case 'anchor':
      return h.via === 'pinned' ? 'pinned, injected at session start' : h.via === 'landmark' ? 'landmark lesson, injected at session start' : 'latest note, injected at session start';
    default:
      return h.route;
  }
}

/** Describes a lattice path: "filed under x" or "filed under x, reached from y in n hops". */
function pathReason(h: TraceHit): string {
  if (!h.via) return 'found through the lattice';
  if (!h.seed || h.seed === h.via) return `filed under ${h.via}`;
  return `filed under ${h.via}, reached from ${h.seed} (${h.depth} hop${h.depth === 1 ? '' : 's'})`;
}

/** Short label for who asked. */
export function originLabel(e: Pick<TraceEvent, 'origin' | 'tool' | 'harness'>): string {
  const who = e.harness && e.harness !== 'memtree' ? e.harness : '';
  switch (e.origin) {
    case 'mcp':
      return `${who || 'agent'} · ${e.tool}`;
    case 'hook':
      return `${who || 'hook'} · ${e.tool}`;
    case 'ui':
      return `UI · ${e.tool}`;
    case 'cli':
      return 'memtree recall';
    default:
      return e.tool || e.origin;
  }
}

/** Whether any hit of the event has use evidence. */
export const wasUsed = (e: TraceEvent) => e.uses.length > 0;

/** Reads a live recall score as trace factors, so both render the same way. */
export function factorsFromScore(s: ScoreDecomposition): TraceFactors {
  return {
    text: s.bm25Normalized, link: s.linkScore ?? 0, importance: s.importanceOffset, activation: s.activation,
    projectBoost: s.projectBoost || 1, bothBonus: s.bothBonus || 1, final: s.final,
  };
}
