// Bucketing for the retrieval calendar: a GitHub-style grid of weeks
// (columns) by weekday (rows, Sunday first). Days are UTC, matching the
// backend's daily aggregation.
import type { DayCount } from './types';

export interface CalendarDay {
  day: string;
  retrieved: number;
  used: number;
  /** After the end day: drawn empty. */
  future: boolean;
}

export interface Calendar {
  /** Each week is 7 days, Sunday first. */
  weeks: CalendarDay[][];
  max: number;
  total: number;
}

const DAY_MS = 86_400_000;

/** Formats a time as its UTC day, YYYY-MM-DD. */
export const utcDay = (ms: number) => new Date(ms).toISOString().slice(0, 10);

/** Lays out counts over the `weeks` weeks ending in the week of `endMs`. */
export function buildCalendar(counts: DayCount[], endMs: number, weeks = 26): Calendar {
  const by = new Map(counts.map((c) => [c.day, c]));
  const end = Date.parse(utcDay(endMs));
  const start = end - (new Date(end).getUTCDay() + (weeks - 1) * 7) * DAY_MS;
  const out: CalendarDay[][] = [];
  let max = 0;
  let total = 0;
  for (let w = 0; w < weeks; w++) {
    const week: CalendarDay[] = [];
    for (let d = 0; d < 7; d++) {
      const ms = start + (w * 7 + d) * DAY_MS;
      const day = utcDay(ms);
      const c = by.get(day);
      const future = ms > end;
      const retrieved = future ? 0 : c?.retrieved ?? 0;
      max = Math.max(max, retrieved);
      total += retrieved;
      week.push({ day, retrieved, used: future ? 0 : c?.used ?? 0, future });
    }
    out.push(week);
  }
  return { weeks: out, max, total };
}

/** Heat in [0,1] for a day: log-scaled against the busiest day so one spike does not flatten the rest. */
export function dayHeat(retrieved: number, max: number): number {
  if (retrieved <= 0 || max <= 0) return 0;
  return Math.log1p(retrieved) / Math.log1p(max);
}

/** ISO date of the first day in the grid, for the matching API query. */
export const calendarSince = (endMs: number, weeks = 26) => `${buildCalendar([], endMs, weeks).weeks[0][0].day}T00:00:00Z`;
