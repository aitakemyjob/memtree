import { describe, expect, it } from 'vitest';
import { areaPath, linePath, xFor, yFor, type Plot } from './curve';

const p: Plot = { width: 200, height: 100, padLeft: 20, padRight: 10, padTop: 10, padBottom: 20, days: 90, yMin: 0, yMax: 1 };

describe('plot scaling', () => {
  it('maps the domain corners to the padded plot area', () => {
    expect(xFor(p, 0)).toBe(20);
    expect(xFor(p, 90)).toBe(190);
    expect(yFor(p, 0)).toBe(80);
    expect(yFor(p, 1)).toBe(10);
  });
  it('puts higher values higher on screen (smaller y)', () => {
    expect(yFor(p, 0.8)).toBeLessThan(yFor(p, 0.2));
  });
  it('builds line and area paths', () => {
    const pts = [{ day: 0, value: 1 }, { day: 90, value: 0 }];
    expect(linePath(p, pts)).toBe('M20.0 10.0 L190.0 80.0');
    expect(areaPath(p, pts)).toBe('M20.0 10.0 L190.0 80.0 L190.0 80.0 L20.0 80.0 Z');
    expect(areaPath(p, [])).toBe('');
  });
});
