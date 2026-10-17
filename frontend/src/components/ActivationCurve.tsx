import { useMemo } from 'react';
import { activationCurve, compressInDays, daysUntilBelow, memoryActivation } from '../lib/activation';
import { areaPath, linePath, xFor, yFor, type Plot } from '../lib/curve';
import { hoursLabel } from '../lib/format';
import type { MemoryRecord, TierConfig } from '../lib/types';

const PLOT: Plot = { width: 360, height: 168, padLeft: 30, padRight: 12, padTop: 12, padBottom: 26, days: 90, yMin: 0, yMax: 1 };

/**
 * Projects a memory's activation over the next 90 days, assuming nobody
 * recalls it again, and marks where compression becomes possible (the
 * activation ceiling and the age gate from the tier settings).
 */
export function ActivationCurve({ record, tiers, nowMs }: { record: MemoryRecord; tiers: TierConfig; nowMs: number }) {
  const { curve, now, eligible, crossing } = useMemo(() => {
    const c = activationCurve(record, nowMs, PLOT.days, 1);
    return {
      curve: c,
      now: memoryActivation(record, nowMs),
      eligible: compressInDays(record, tiers, nowMs, 365),
      crossing: daysUntilBelow(c, tiers.activationCeilingToCompress),
    };
  }, [record, tiers, nowMs]);
  const gate = record.tier === 'episodic' ? tiers.ageHoursToSemantic : tiers.ageHoursToCompact;
  const ceilingY = yFor(PLOT, tiers.activationCeilingToCompress);

  let note: string;
  if (record.pinned) note = 'Pinned: activation stays at 1 and the memory is never compressed.';
  else if (record.archivedAt || record.supersededBy) note = 'Archived or superseded memories are no longer compressed.';
  else if (eligible === 0) note = 'Already old and cool enough to be distilled (see Consolidation).';
  else if (eligible !== null && eligible <= 365) note = `Becomes a distillation candidate in about ${eligible} day${eligible === 1 ? '' : 's'}.`;
  else note = 'Stays above the compression ceiling for the next year.';

  return (
    <figure className="curve">
      <svg viewBox={`0 0 ${PLOT.width} ${PLOT.height}`} role="img" aria-label={`Activation decay over 90 days, now ${now.toFixed(2)}`}>
        {[0, 0.5, 1].map((v) => (
          <g key={v}>
            <line x1={PLOT.padLeft} x2={PLOT.width - PLOT.padRight} y1={yFor(PLOT, v)} y2={yFor(PLOT, v)} className="curve-grid" />
            <text x={PLOT.padLeft - 5} y={yFor(PLOT, v) + 3} textAnchor="end" className="curve-tick">{v}</text>
          </g>
        ))}
        {[0, 30, 60, 90].map((d) => (
          <text key={d} x={xFor(PLOT, d)} y={PLOT.height - 8} textAnchor="middle" className="curve-tick">{d === 0 ? 'now' : `+${d}d`}</text>
        ))}
        <path d={areaPath(PLOT, curve)} className="curve-area" />
        <path d={linePath(PLOT, curve)} className="curve-line" />
        <line x1={PLOT.padLeft} x2={PLOT.width - PLOT.padRight} y1={ceilingY} y2={ceilingY} className="curve-ceiling" />
        <text x={PLOT.width - PLOT.padRight} y={ceilingY - 4} textAnchor="end" className="curve-label">compress below {tiers.activationCeilingToCompress}</text>
        {!record.pinned && crossing !== null && crossing > 0 && (
          <line x1={xFor(PLOT, crossing)} x2={xFor(PLOT, crossing)} y1={PLOT.padTop} y2={yFor(PLOT, 0)} className="curve-cross" />
        )}
        {!record.pinned && eligible !== null && eligible > 0 && eligible <= PLOT.days && (
          <g>
            <line x1={xFor(PLOT, eligible)} x2={xFor(PLOT, eligible)} y1={PLOT.padTop} y2={yFor(PLOT, 0)} className="curve-eligible" />
            <text x={xFor(PLOT, eligible) - 4} y={PLOT.padTop + 10} textAnchor="end" className="curve-label">eligible +{eligible}d</text>
          </g>
        )}
        <circle cx={xFor(PLOT, 0)} cy={yFor(PLOT, now)} r={3.5} className="curve-dot" />
      </svg>
      <figcaption>
        <b>{now.toFixed(2)}</b> now, decay exponent {record.tier === 'semantic' ? '0.2' : '0.5'} ({record.tier}). Age gate: {hoursLabel(gate)}. {note}
      </figcaption>
    </figure>
  );
}
