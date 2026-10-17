import { describe, expect, it } from 'vitest';
import { needsRestart, settingsChanged, validateSettings } from './settings';
import type { Settings } from './types';

const ok: Settings = {
  theme: 'sfc-dark', fontSize: 13, mcpPort: 47475, dbPath: '', agentWrites: true, captureHooks: true, surfaceThreshold: 0.25, maxSurface: 3,
  linkHalfLifeDays: 30, maintainHours: 12, tiers: { ageHoursToSemantic: 72, ageHoursToCompact: 720, activationCeilingToCompress: 0.5, minClusterSize: 2 },
};

describe('validateSettings', () => {
  it('accepts the defaults', () => {
    expect(validateSettings(ok)).toEqual([]);
  });
  it('rejects out-of-range and non-finite values', () => {
    expect(validateSettings({ ...ok, mcpPort: 80 })).toHaveLength(1);
    expect(validateSettings({ ...ok, mcpPort: 47475.5 })).toHaveLength(1);
    expect(validateSettings({ ...ok, surfaceThreshold: 1.2 })).toHaveLength(1);
    expect(validateSettings({ ...ok, maxSurface: 0 })).toHaveLength(1);
    expect(validateSettings({ ...ok, linkHalfLifeDays: NaN })).toHaveLength(1);
    expect(validateSettings({ ...ok, tiers: { ...ok.tiers, activationCeilingToCompress: 0.1 } })).toHaveLength(1);
    expect(validateSettings({ ...ok, tiers: { ...ok.tiers, minClusterSize: 1 } })).toHaveLength(1);
  });
  it('reports every problem', () => {
    expect(validateSettings({ ...ok, fontSize: 5, maintainHours: 0, tiers: { ...ok.tiers, ageHoursToSemantic: 0 } })).toHaveLength(3);
  });
});

describe('change detection', () => {
  it('detects any difference, including nested tiers', () => {
    expect(settingsChanged(ok, { ...ok })).toBe(false);
    expect(settingsChanged(ok, { ...ok, tiers: { ...ok.tiers, minClusterSize: 3 } })).toBe(true);
  });
  it('flags only a port change as needing a restart', () => {
    expect(needsRestart(ok, { ...ok, mcpPort: 5000 })).toBe(true);
    expect(needsRestart(ok, { ...ok, agentWrites: false })).toBe(false);
  });
});
