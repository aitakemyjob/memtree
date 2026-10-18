import { describe, expect, it } from 'vitest';
import { buildCalendar, calendarSince, dayHeat } from './calendar';

// Wednesday 2026-03-04, late in the UTC day.
const END = Date.parse('2026-03-04T23:30:00Z');

describe('buildCalendar', () => {
  it('lays out whole weeks ending in the current week, Sunday first', () => {
    const cal = buildCalendar([{ day: '2026-03-04', retrieved: 4, used: 1 }, { day: '2026-03-01', retrieved: 1, used: 0 }, { day: '2025-01-01', retrieved: 99, used: 0 }], END, 2);
    expect(cal.weeks).toHaveLength(2);
    expect(cal.weeks.every((w) => w.length === 7)).toBe(true);
    expect(cal.weeks[0][0].day).toBe('2026-02-22');
    expect(cal.weeks[1][0].day).toBe('2026-03-01');
    expect(cal.weeks[1][3]).toEqual({ day: '2026-03-04', retrieved: 4, used: 1, future: false });
    expect(cal.weeks[1][4].future).toBe(true);
    expect(cal.max).toBe(4);
    expect(cal.total).toBe(5);
  });
  it('gives the matching query start', () => {
    expect(calendarSince(END, 2)).toBe('2026-02-22T00:00:00Z');
  });
});

describe('dayHeat', () => {
  it('log-scales against the busiest day', () => {
    expect(dayHeat(0, 10)).toBe(0);
    expect(dayHeat(10, 10)).toBe(1);
    expect(dayHeat(3, 10)).toBeCloseTo(Math.log1p(3) / Math.log1p(10));
  });
});
