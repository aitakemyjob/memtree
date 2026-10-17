// SVG geometry for the activation decay chart. Kept pure so the scaling
// (which is easy to get subtly wrong, e.g. an inverted y axis) is tested.
import type { CurvePoint } from './activation';

export interface Plot {
  width: number;
  height: number;
  padLeft: number;
  padRight: number;
  padTop: number;
  padBottom: number;
  /** x domain in days. */
  days: number;
  /** y domain. */
  yMin: number;
  yMax: number;
}

/** x pixel for a day. */
export function xFor(p: Plot, day: number): number {
  return p.padLeft + (day / p.days) * (p.width - p.padLeft - p.padRight);
}

/** y pixel for a value; larger values are higher on screen. */
export function yFor(p: Plot, value: number): number {
  const t = (value - p.yMin) / (p.yMax - p.yMin);
  return p.height - p.padBottom - t * (p.height - p.padTop - p.padBottom);
}

/** SVG path "M x y L x y ..." for the curve. */
export function linePath(p: Plot, pts: CurvePoint[]): string {
  return pts.map((c, i) => `${i === 0 ? 'M' : 'L'}${xFor(p, c.day).toFixed(1)} ${yFor(p, c.value).toFixed(1)}`).join(' ');
}

/** Closed path under the curve down to the x axis, for the soft fill. */
export function areaPath(p: Plot, pts: CurvePoint[]): string {
  if (pts.length === 0) return '';
  const base = yFor(p, p.yMin).toFixed(1);
  return `${linePath(p, pts)} L${xFor(p, pts[pts.length - 1].day).toFixed(1)} ${base} L${xFor(p, pts[0].day).toFixed(1)} ${base} Z`;
}
