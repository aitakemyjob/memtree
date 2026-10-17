// Small pure formatting helpers shared by views.

/** The three importance levels the backend snaps to (store.SnapImportance). */
export const IMPORTANCE_LEVELS = [
  { value: 1, name: 'routine', hint: 'Everyday context' },
  { value: 1.5, name: 'decision', hint: 'A choice and its reason' },
  { value: 2.5, name: 'lesson', hint: 'Hard-won, worth always surfacing' },
] as const;

/** Snaps any number to the nearest importance level like the backend (>=2 lesson, >=1.25 decision). */
export function snapImportance(v: number): number {
  if (v >= 2) return 2.5;
  if (v >= 1.25) return 1.5;
  return 1;
}

/** "routine" | "decision" | "lesson" for an importance value. */
export function importanceName(v: number): string {
  return IMPORTANCE_LEVELS.find((l) => l.value === snapImportance(v))!.name;
}

/** Compact age such as "just now", "5m", "3h", "12d", "4mo", "2y". */
export function ageShort(ms: number): string {
  const s = Math.max(0, ms) / 1000;
  if (s < 45) return 'now';
  const m = s / 60;
  if (m < 60) return `${Math.round(m)}m`;
  const h = m / 60;
  if (h < 36) return `${Math.round(h)}h`;
  const d = h / 24;
  if (d < 45) return `${Math.round(d)}d`;
  const mo = d / 30.44;
  if (mo < 18) return `${Math.round(mo)}mo`;
  return `${Math.round(d / 365.25)}y`;
}

/** "5m ago" style relative time for an RFC 3339 timestamp. */
export function relTime(iso: string | undefined, nowMs: number = Date.now()): string {
  if (!iso) return '';
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return '';
  const a = ageShort(nowMs - t);
  return a === 'now' ? 'just now' : `${a} ago`;
}

/** Age in hours between an RFC 3339 timestamp and now (0 when unparseable). */
export function ageHours(iso: string | undefined, nowMs: number = Date.now()): number {
  const t = iso ? Date.parse(iso) : NaN;
  return Number.isFinite(t) ? Math.max(0, (nowMs - t) / 3_600_000) : 0;
}

/** Hours as a friendly duration: "72h" becomes "3 days", "720h" "30 days". */
export function hoursLabel(h: number): string {
  if (h < 48) return `${Math.round(h)} h`;
  const d = h / 24;
  return Number.isInteger(d) ? `${d} days` : `${d.toFixed(1)} days`;
}

/** Last path segment of a project path; "" is the global scope. */
export function projectName(project: string): string {
  if (!project) return 'Global';
  const parts = project.split(/[\\/]/).filter(Boolean);
  return parts[parts.length - 1] ?? project;
}

/** Number with fixed decimals, "-" for non-finite. */
export function fixed(n: number, digits = 2): string {
  return Number.isFinite(n) ? n.toFixed(digits) : '-';
}

/** Percentage string for 0..1. */
export function pct(n: number): string {
  return Number.isFinite(n) ? `${Math.round(n * 100)}%` : '-';
}

/** Clamps n into [lo, hi]. */
export function clamp(n: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, n));
}

/** Pluralises a noun for a count: plural(1,'memory','memories'). */
export function plural(n: number, one: string, many = `${one}s`): string {
  return `${n} ${n === 1 ? one : many}`;
}

/** Truncates with an ellipsis. */
export function truncate(s: string, max: number): string {
  return s.length <= max ? s : s.slice(0, Math.max(0, max - 1)).trimEnd() + '…';
}

/** First line of a summary, for list titles. */
export function firstLine(s: string): string {
  return s.split('\n', 1)[0] ?? '';
}

/** Duration in ms as "12 ms" / "1.4 s". */
export function durationLabel(ms: number): string {
  if (!Number.isFinite(ms)) return '-';
  return ms < 1000 ? `${Math.round(ms)} ms` : `${(ms / 1000).toFixed(1)} s`;
}

/** CSS colour variable for a memory kind (chips and treemap legends). */
export function kindColor(kind: string): string {
  switch (kind) {
    case 'decision': return 'var(--accent-text)';
    case 'lesson': return 'var(--warn)';
    case 'session': return 'var(--info)';
    case 'capture': return 'var(--text-3)';
    default: return 'var(--text-2)';
  }
}

/** Extracts a memory id (mem_xxxxxxxx) from free text, e.g. an activity summary. */
export function findMemoryId(text: string): string | undefined {
  return /\bmem_[0-9a-f]{8,}\b/.exec(text)?.[0];
}
