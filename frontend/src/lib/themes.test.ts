import { afterEach, describe, expect, it } from 'vitest';
import { alpha, applyTheme, cachedThemeId, contrast, cssVars, heatColor, luminance, mix, readableOn, themeById, themes } from './themes';

describe('contrast', () => {
  it('matches the WCAG reference values', () => {
    expect(contrast('#000000', '#ffffff')).toBeCloseTo(21, 5);
    expect(contrast('#ffffff', '#ffffff')).toBeCloseTo(1, 5);
    // #777 on white is the textbook 4.48:1.
    expect(contrast('#777777', '#ffffff')).toBeCloseTo(4.48, 2);
    expect(luminance('#ffffff')).toBeCloseTo(1, 5);
  });
});

describe('palettes', () => {
  it('has unique ids and the SFC themes first', () => {
    const ids = themes.map((t) => t.id);
    expect(new Set(ids).size).toBe(ids.length);
    expect(ids.slice(0, 2)).toEqual(['sfc-dark', 'sfc-light']);
    expect(themes.length).toBeGreaterThanOrEqual(6);
  });

  it('uses the brand colours for the SFC themes', () => {
    const d = themeById('sfc-dark');
    expect(d.bg).toBe('#04131c');
    expect(d.accent).toBe('#266970');
    expect(d.text2).toBe('#acb6bc');
    const l = themeById('sfc-light');
    expect(l.bg1).toBe('#ffffff');
    expect(l.text).toBe('#04131c');
  });

  describe.each(themes.map((t) => [t.id, t] as const))('%s', (_id, t) => {
    it('primary and secondary text reach 4.5:1 on both surfaces', () => {
      for (const surface of [t.bg, t.bg1, t.bg2]) {
        expect(contrast(t.text, surface)).toBeGreaterThanOrEqual(4.5);
        expect(contrast(t.text2, surface)).toBeGreaterThanOrEqual(4.5);
      }
    });
    it('accent text reads on the surfaces and fg reads on the accent fill', () => {
      expect(contrast(t.accentText, t.bg1)).toBeGreaterThanOrEqual(4.5);
      expect(contrast(t.accentFg, t.accent)).toBeGreaterThanOrEqual(4.5);
    });
    it('status colours are legible as text', () => {
      for (const c of [t.ok, t.warn, t.err]) expect(contrast(c, t.bg1)).toBeGreaterThanOrEqual(3);
    });
    it('captions stay at least 3:1 on the main surface', () => {
      expect(contrast(t.text3, t.bg1)).toBeGreaterThanOrEqual(3);
    });
    it('is a well-formed dark/light palette', () => {
      const hex = /^#[0-9a-f]{6}$/;
      for (const v of [t.bg, t.bg1, t.bg2, t.bg3, t.border, t.text, t.accent, ...t.heat]) expect(v).toMatch(hex);
      // Dark themes have light text and vice versa.
      expect(luminance(t.text) > luminance(t.bg)).toBe(t.mode === 'dark');
    });
  });
});

describe('colour helpers', () => {
  it('mixes and applies alpha', () => {
    expect(mix('#000000', '#ffffff', 0.5)).toBe('#808080');
    expect(alpha('#266970', 0.5)).toBe('rgba(38, 105, 112, 0.5)');
  });
  it('maps heat onto the stops', () => {
    const t = themeById('sfc-dark');
    expect(heatColor(t, 0)).toBe(t.heat[0]);
    expect(heatColor(t, 1)).toBe(t.heat[4]);
    expect(heatColor(t, NaN)).toBe(t.heat[0]);
    expect(heatColor(t, 0.25)).toBe(t.heat[1]);
  });
  it('picks readable label colours', () => {
    expect(readableOn('#04131c')).toBe('#ffffff');
    expect(readableOn('#ffffff')).toBe('#04131c');
  });
  it('falls back to SFC Dark for unknown ids', () => {
    expect(themeById('nope').id).toBe('sfc-dark');
    expect(themeById(undefined).id).toBe('sfc-dark');
  });
});

describe('applyTheme', () => {
  afterEach(() => document.documentElement.removeAttribute('data-theme'));
  it('writes css variables, mode, and remembers the id', () => {
    const t = themeById('sfc-light');
    applyTheme(t);
    const root = document.documentElement;
    expect(root.style.getPropertyValue('--bg')).toBe(t.bg);
    expect(root.dataset.mode).toBe('light');
    expect(root.dataset.theme).toBe('sfc-light');
    expect(cachedThemeId()).toBe('sfc-light');
  });
  it('defines every variable the stylesheet relies on', () => {
    const vars = Object.keys(cssVars(themeById('nord')));
    for (const v of ['--bg', '--bg-1', '--accent', '--accent-text', '--heat-4', '--ok-soft', '--shadow']) expect(vars).toContain(v);
  });
});
