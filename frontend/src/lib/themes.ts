// Themes are plain palettes. One palette drives every CSS variable and the
// heat scale, so all surfaces stay coherent when switching. The default
// themes are the Silver Falls Capital brand palette:
//   Rich Black 04131C, White FFFFFF, Medium Grey ACB6BC, Dark Sky 122630,
//   Deep Ocean 223F47, Stone Grey 708087, Mist E7EEF2, Ocean 266970,
//   Dark Grey 394851, Light Grey C5CED3.

export interface Theme {
  id: string;
  name: string;
  mode: 'dark' | 'light';
  bg: string;
  bg1: string;
  bg2: string;
  bg3: string;
  border: string;
  borderStrong: string;
  text: string;
  /** Secondary text; must stay >= 4.5:1 on bg and bg1. */
  text2: string;
  /** Captions and disabled hints only; >= 3:1. */
  text3: string;
  /** Fill colour for primary buttons, rings and active markers. */
  accent: string;
  /** Accent used as text or icon colour on bg; >= 4.5:1 (differs from accent when the brand accent is too dark to read). */
  accentText: string;
  accent2: string;
  /** Text on an accent fill. */
  accentFg: string;
  ok: string;
  warn: string;
  err: string;
  info: string;
  /** Five stops from cold to hot. */
  heat: [string, string, string, string, string];
}

export const themes: Theme[] = [
  {
    id: 'sfc-dark', name: 'SFC Dark', mode: 'dark',
    bg: '#04131c', bg1: '#0a1c26', bg2: '#122630', bg3: '#1a323d', border: '#1b3039', borderStrong: '#2c4852',
    text: '#e7eef2', text2: '#acb6bc', text3: '#708087',
    accent: '#266970', accentText: '#6cbcc4', accent2: '#e0b64f', accentFg: '#ffffff',
    ok: '#58cf91', warn: '#e0b64f', err: '#f27b72', info: '#6cbcc4',
    heat: ['#16303a', '#266970', '#6cbcc4', '#e0b64f', '#f27b72'],
  },
  {
    id: 'sfc-light', name: 'SFC Light', mode: 'light',
    bg: '#e7eef2', bg1: '#ffffff', bg2: '#f3f7f9', bg3: '#dde7ec', border: '#d3dde3', borderStrong: '#c5ced3',
    text: '#04131c', text2: '#394851', text3: '#708087',
    accent: '#266970', accentText: '#1e5a61', accent2: '#8a5d00', accentFg: '#ffffff',
    ok: '#1c7447', warn: '#8a5d00', err: '#b42318', info: '#1e5a61',
    heat: ['#e7eef2', '#b3d3d7', '#5aa1a9', '#e0a84a', '#c4473a'],
  },
  {
    id: 'tokyo', name: 'Tokyo Night', mode: 'dark',
    bg: '#14141c', bg1: '#1a1b26', bg2: '#1f2233', bg3: '#292e42', border: '#272b3f', borderStrong: '#3b4261',
    text: '#c0caf5', text2: '#9aa5ce', text3: '#6c75a3',
    accent: '#7aa2f7', accentText: '#7aa2f7', accent2: '#bb9af7', accentFg: '#0b1020',
    ok: '#9ece6a', warn: '#e0af68', err: '#f7768e', info: '#7dcfff',
    heat: ['#1f2a4a', '#3d59a1', '#9d7cd8', '#ff9e64', '#f7768e'],
  },
  {
    id: 'nord', name: 'Nord', mode: 'dark',
    bg: '#272c36', bg1: '#2e3440', bg2: '#353c4a', bg3: '#3b4252', border: '#3e4656', borderStrong: '#4c566a',
    text: '#eceff4', text2: '#c2cad8', text3: '#7b879c',
    accent: '#88c0d0', accentText: '#88c0d0', accent2: '#b48ead', accentFg: '#1c2129',
    ok: '#a3be8c', warn: '#ebcb8b', err: '#bf616a', info: '#81a1c1',
    heat: ['#3b4252', '#5e81ac', '#b48ead', '#d08770', '#bf616a'],
  },
  {
    id: 'dracula', name: 'Dracula', mode: 'dark',
    bg: '#1b1c25', bg1: '#21222c', bg2: '#282a36', bg3: '#343746', border: '#30323f', borderStrong: '#44475a',
    text: '#f8f8f2', text2: '#c3c4d4', text3: '#6272a4',
    accent: '#bd93f9', accentText: '#bd93f9', accent2: '#ff79c6', accentFg: '#17121f',
    ok: '#50fa7b', warn: '#f1fa8c', err: '#ff5555', info: '#8be9fd',
    heat: ['#2f3242', '#6272a4', '#bd93f9', '#ffb86c', '#ff5555'],
  },
  {
    id: 'paper', name: 'Paper', mode: 'light',
    bg: '#f3f4f7', bg1: '#ffffff', bg2: '#f6f7f9', bg3: '#eceef2', border: '#e2e5ea', borderStrong: '#cfd4dc',
    text: '#1a2130', text2: '#525d70', text3: '#8b94a5',
    accent: '#2563eb', accentText: '#1d4ed8', accent2: '#7c3aed', accentFg: '#ffffff',
    ok: '#047857', warn: '#a35a00', err: '#dc2626', info: '#0369a1',
    heat: ['#e0e7ff', '#93c5fd', '#a78bfa', '#fb923c', '#ef4444'],
  },
  {
    id: 'solarized', name: 'Solarized Light', mode: 'light',
    bg: '#eee8d5', bg1: '#fdf6e3', bg2: '#f7f0dc', bg3: '#ece5cf', border: '#e0d9c2', borderStrong: '#cfc7ad',
    text: '#073642', text2: '#4b6068', text3: '#74878a',
    accent: '#1a6aa3', accentText: '#1a6aa3', accent2: '#d33682', accentFg: '#fdf6e3',
    ok: '#6b7a00', warn: '#8f6a00', err: '#dc322f', info: '#1f7f78',
    heat: ['#eee8d5', '#93c5d8', '#6c71c4', '#cb4b16', '#dc322f'],
  },
];

/** The default theme id; its colours are also baked into index.html and the Wails window. */
export const DEFAULT_THEME = 'sfc-dark';

/** Looks up a theme, falling back to SFC Dark. */
export function themeById(id: string | undefined): Theme {
  return themes.find((t) => t.id === id) ?? themes[0];
}

/** Parses #rgb or #rrggbb. */
function hexToRgb(hex: string): [number, number, number] {
  const h = hex.replace('#', '');
  const n = parseInt(h.length === 3 ? h.split('').map((c) => c + c).join('') : h, 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

/** Formats an RGB triple as #rrggbb, clamping channels. */
function rgbToHex([r, g, b]: [number, number, number]): string {
  return '#' + [r, g, b].map((v) => Math.round(Math.max(0, Math.min(255, v))).toString(16).padStart(2, '0')).join('');
}

/** Linearly interpolates between two hex colors (t in 0..1). */
export function mix(a: string, b: string, t: number): string {
  const x = hexToRgb(a);
  const y = hexToRgb(b);
  return rgbToHex([x[0] + (y[0] - x[0]) * t, x[1] + (y[1] - x[1]) * t, x[2] + (y[2] - x[2]) * t]);
}

/** A hex color with alpha, as a CSS rgba() string. */
export function alpha(hex: string, a: number): string {
  const [r, g, b] = hexToRgb(hex);
  return `rgba(${r}, ${g}, ${b}, ${a})`;
}

/** WCAG relative luminance of a hex colour. */
export function luminance(hex: string): number {
  const [r, g, b] = hexToRgb(hex).map((v) => {
    const c = v / 255;
    return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/** WCAG contrast ratio between two hex colours (1..21). */
export function contrast(a: string, b: string): number {
  const la = luminance(a);
  const lb = luminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

/** Picks near-black or white text, whichever reads better on bg. */
export function readableOn(bg: string): string {
  return contrast(bg, '#04131c') >= contrast(bg, '#ffffff') ? '#04131c' : '#ffffff';
}

/** Maps t in [0,1] onto the theme's five heat stops. */
export function heatColor(theme: Theme, t: number): string {
  if (!Number.isFinite(t) || t <= 0) return theme.heat[0];
  const c = Math.min(1, t);
  const seg = c * (theme.heat.length - 1);
  const i = Math.min(theme.heat.length - 2, Math.floor(seg));
  return mix(theme.heat[i], theme.heat[i + 1], seg - i);
}

/** The CSS custom properties a theme defines. */
export function cssVars(t: Theme): Record<string, string> {
  return {
    '--bg': t.bg, '--bg-1': t.bg1, '--bg-2': t.bg2, '--bg-3': t.bg3,
    '--border': t.border, '--border-strong': t.borderStrong,
    '--text': t.text, '--text-2': t.text2, '--text-3': t.text3,
    '--accent': t.accent, '--accent-text': t.accentText, '--accent-2': t.accent2, '--accent-fg': t.accentFg,
    '--accent-soft': alpha(t.accent, t.mode === 'dark' ? 0.22 : 0.12),
    '--accent-2-soft': alpha(t.accent2, t.mode === 'dark' ? 0.16 : 0.12),
    '--ok': t.ok, '--warn': t.warn, '--err': t.err, '--info': t.info,
    '--ok-soft': alpha(t.ok, 0.14), '--warn-soft': alpha(t.warn, 0.16), '--err-soft': alpha(t.err, 0.14),
    '--shadow': t.mode === 'dark' ? '0 12px 40px rgba(0,0,0,.45)' : '0 12px 32px rgba(20,30,50,.12)',
    '--grid': alpha(t.text3, t.mode === 'dark' ? 0.18 : 0.25),
    '--heat-0': t.heat[0], '--heat-1': t.heat[1], '--heat-2': t.heat[2], '--heat-3': t.heat[3], '--heat-4': t.heat[4],
  };
}

const THEME_CACHE_KEY = 'mt.theme';

/**
 * Applies a theme to the document (CSS variables, data attributes and
 * color-scheme) and remembers its id so the next load paints it before the
 * backend has answered (no flash of the wrong theme).
 */
export function applyTheme(t: Theme, root: HTMLElement = document.documentElement) {
  for (const [k, v] of Object.entries(cssVars(t))) root.style.setProperty(k, v);
  root.dataset.theme = t.id;
  root.dataset.mode = t.mode;
  root.style.colorScheme = t.mode;
  try {
    localStorage.setItem(THEME_CACHE_KEY, t.id);
  } catch {
    /* private mode: the theme just loads after the backend answers */
  }
}

/** The theme id remembered by the last applyTheme, if any. */
export function cachedThemeId(): string | undefined {
  try {
    return localStorage.getItem(THEME_CACHE_KEY) ?? undefined;
  } catch {
    return undefined;
  }
}
