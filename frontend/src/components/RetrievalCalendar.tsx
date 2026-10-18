import { useMemo } from 'react';
import { buildCalendar, dayHeat } from '../lib/calendar';
import { plural } from '../lib/format';
import { useStore } from '../lib/store';
import { heatColor } from '../lib/themes';
import type { DayCount } from '../lib/types';

const CELL = 11;
const GAP = 2;

/**
 * GitHub-style grid of memory retrievals per day (weeks across, weekdays
 * down, UTC days). Colour is log-scaled retrieval count; a dot marks days
 * where a retrieved memory was then used.
 */
export function RetrievalCalendar({ counts, endMs, weeks = 26, label = 'Retrievals per day' }: { counts: DayCount[]; endMs: number; weeks?: number; label?: string }) {
  const theme = useStore((s) => s.theme);
  const cal = useMemo(() => buildCalendar(counts, endMs, weeks), [counts, endMs, weeks]);
  const step = CELL + GAP;
  return (
    <figure className="ret-cal" aria-label={label}>
      <svg width={weeks * step} height={7 * step} role="img" aria-label={`${label}: ${plural(cal.total, 'retrieval')} in ${weeks} weeks`}>
        {cal.weeks.map((week, w) =>
          week.map((d, i) =>
            d.future ? null : (
              <g key={d.day}>
                <rect
                  x={w * step} y={i * step} width={CELL} height={CELL} rx={2}
                  fill={d.retrieved ? heatColor(theme, dayHeat(d.retrieved, cal.max)) : 'var(--bg-3)'}
                >
                  <title>{`${d.day}: ${plural(d.retrieved, 'retrieval')}${d.used ? `, ${d.used} used` : ''}`}</title>
                </rect>
                {d.used > 0 && <circle cx={w * step + CELL / 2} cy={i * step + CELL / 2} r={1.6} fill="var(--text)" pointerEvents="none" />}
              </g>
            ),
          ),
        )}
      </svg>
      <figcaption className="faint">{plural(cal.total, 'retrieval')} in the last {weeks} weeks · dot = used</figcaption>
    </figure>
  );
}
