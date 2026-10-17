// TypeScript port of internal/store/activation.go (ACT-R base-level
// activation). It exists so the UI can draw the future decay curve without
// a round trip; the numbers must agree with the Go service (the backend's
// memory.explain returns the authoritative current value).
import type { MemoryRecord, TierConfig } from './types';

/** Mirrors store.ActivationParams. */
export interface ActivationParams {
  /** Decay exponent d. */
  decay: number;
  /** Minimum multiplier. */
  floor: number;
  /** Logistic scale; larger = gentler slope. */
  ceiling: number;
}

export const DEFAULT_FLOOR = 0.15;
/** Episodic memories fade quickly (d = 0.5). */
export const EPISODIC_PARAMS: ActivationParams = { decay: 0.5, floor: DEFAULT_FLOOR, ceiling: 2 };
/** Semantic memories are distilled gist and fade slowly (d = 0.2). */
export const SEMANTIC_PARAMS: ActivationParams = { decay: 0.2, floor: DEFAULT_FLOOR, ceiling: 2 };
/** Lattice nodes (d = 0.35), driven by a single last-accessed time. */
export const NODE_PARAMS: ActivationParams = { decay: 0.35, floor: DEFAULT_FLOOR, ceiling: 2 };

const HOUR_MS = 3_600_000;
const DAY_MS = 24 * HOUR_MS;

/** Params for a memory tier (anything but "semantic" is episodic, as in Go). */
export function paramsFor(tier: string): ActivationParams {
  return tier === 'semantic' ? SEMANTIC_PARAMS : EPISODIC_PARAMS;
}

/**
 * ln(sum t_i^-d) where t_i is hours since access i, clamped to at least one
 * hour. An empty log is -Infinity.
 */
export function activationRaw(accessMs: number[], d: number, nowMs: number): number {
  if (accessMs.length === 0) return -Infinity;
  let sum = 0;
  for (const at of accessMs) {
    const hours = Math.max(1, (nowMs - at) / HOUR_MS);
    sum += Math.pow(hours, -d);
  }
  return Math.log(sum);
}

/**
 * Squashes raw activation through a logistic centred at 0 into (0,1), then
 * lifts it into [floor, 1]. With no accesses it is the floor.
 */
export function activationScore(accessMs: number[], p: ActivationParams, nowMs: number): number {
  const raw = activationRaw(accessMs, p.decay, nowMs);
  if (!Number.isFinite(raw)) return p.floor;
  const scale = p.ceiling > 0 ? p.ceiling : 1;
  const normalized = 1 / (1 + Math.exp(-raw / scale));
  return p.floor + (1 - p.floor) * normalized;
}

/** Parses an RFC 3339 string to epoch ms (NaN-safe: bad values are dropped by callers). */
export function parseTime(s: string | undefined): number {
  return s ? Date.parse(s) : NaN;
}

/** A record's access times as epoch ms, dropping unparseable entries. */
export function accessTimes(r: Pick<MemoryRecord, 'accessLog'>): number[] {
  return (r.accessLog ?? []).map(parseTime).filter((n) => Number.isFinite(n));
}

/** Current activation of a memory (pinned = 1), like Memory.ActivationOf. */
export function memoryActivation(r: MemoryRecord, nowMs: number): number {
  if (r.pinned) return 1;
  return activationScore(accessTimes(r), paramsFor(r.tier), nowMs);
}

/** Current activation of a lattice node (single last-accessed time). */
export function nodeActivation(lastAccessed: string, nowMs: number): number {
  return activationScore([parseTime(lastAccessed)].filter(Number.isFinite), NODE_PARAMS, nowMs);
}

export interface CurvePoint {
  /** Days from now. */
  day: number;
  value: number;
}

/**
 * Projects a memory's activation forward assuming it is never accessed
 * again: one point per `stepDays` from 0 to `days` inclusive. Pinned
 * memories stay at 1.
 */
export function activationCurve(r: MemoryRecord, nowMs: number, days = 90, stepDays = 1): CurvePoint[] {
  const log = accessTimes(r);
  const p = paramsFor(r.tier);
  const out: CurvePoint[] = [];
  for (let d = 0; d <= days + 1e-9; d += stepDays) {
    out.push({ day: d, value: r.pinned ? 1 : activationScore(log, p, nowMs + d * DAY_MS) });
  }
  return out;
}

/**
 * Days from now at which compression becomes allowed (store.ShouldCompress):
 * the record is old enough AND activation has fallen to the ceiling or
 * below. 0 means it already qualifies; null means never within `horizonDays`
 * (pinned, archived, superseded, or still too warm).
 */
export function compressInDays(r: MemoryRecord, cfg: TierConfig, nowMs: number, horizonDays = 365): number | null {
  if (r.pinned || r.archivedAt || r.supersededBy) return null;
  const created = parseTime(r.createdAt);
  const gateHours = r.tier === 'episodic' ? cfg.ageHoursToSemantic : cfg.ageHoursToCompact;
  const log = accessTimes(r);
  const p = paramsFor(r.tier);
  for (let d = 0; d <= horizonDays; d++) {
    const t = nowMs + d * DAY_MS;
    const ageOk = (t - created) / HOUR_MS >= gateHours;
    if (ageOk && activationScore(log, p, t) <= cfg.activationCeilingToCompress) return d;
  }
  return null;
}

/** Days from now until the curve first drops to or below `level`, or null. */
export function daysUntilBelow(curve: CurvePoint[], level: number): number | null {
  const hit = curve.find((c) => c.value <= level);
  return hit ? hit.day : null;
}
