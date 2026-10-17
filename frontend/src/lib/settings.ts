import type { Settings } from './types';

/** Human-readable problems with a settings object; empty means it can be saved. */
export function validateSettings(s: Settings): string[] {
  const out: string[] = [];
  const num = (v: number) => Number.isFinite(v);
  if (!num(s.fontSize) || s.fontSize < 10 || s.fontSize > 22) out.push('Font size must be between 10 and 22.');
  if (!Number.isInteger(s.mcpPort) || s.mcpPort < 1024 || s.mcpPort > 65535) out.push('Port must be a whole number between 1024 and 65535.');
  if (!num(s.surfaceThreshold) || s.surfaceThreshold < 0 || s.surfaceThreshold > 1) out.push('Surface threshold must be between 0 and 1.');
  if (!Number.isInteger(s.maxSurface) || s.maxSurface < 1 || s.maxSurface > 10) out.push('Max surfaced memories must be a whole number from 1 to 10.');
  if (!num(s.linkHalfLifeDays) || s.linkHalfLifeDays < 1) out.push('Link half-life must be at least 1 day.');
  if (!num(s.maintainHours) || s.maintainHours < 1) out.push('Maintenance interval must be at least 1 hour.');
  const t = s.tiers;
  if (!num(t.ageHoursToSemantic) || t.ageHoursToSemantic < 1) out.push('Episodic age gate must be at least 1 hour.');
  if (!num(t.ageHoursToCompact) || t.ageHoursToCompact < 1) out.push('Semantic age gate must be at least 1 hour.');
  if (!num(t.activationCeilingToCompress) || t.activationCeilingToCompress < 0.15 || t.activationCeilingToCompress > 1) out.push('Activation ceiling must be between 0.15 (the floor) and 1.');
  if (!Number.isInteger(t.minClusterSize) || t.minClusterSize < 2) out.push('A cluster needs at least 2 memories.');
  return out;
}

/** Whether two settings objects differ. */
export function settingsChanged(a: Settings, b: Settings): boolean {
  return JSON.stringify(a) !== JSON.stringify(b);
}

/** Whether changing from `a` to `b` needs a restart (the listening port is bound at startup). */
export function needsRestart(a: Settings, b: Settings): boolean {
  return a.mcpPort !== b.mcpPort;
}
