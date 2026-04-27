// Theme tokens for Push chat app — light + dark.
// Pro/utilitarian devtool feel. Single accent (blue), neutral grays.

const THEMES = {
  dark: {
    bg:        '#0b0d10',
    panel:     '#101317',
    panel2:    '#15191f',
    border:    'rgba(255,255,255,0.08)',
    borderStrong: 'rgba(255,255,255,0.14)',
    fg:        '#e7eaee',
    fgMuted:   '#9aa3ad',
    fgDim:     '#6b7480',
    accent:    '#3b82f6',
    accentFg:  '#ffffff',
    bubble:    '#1a1f27',
    bubbleFg:  '#e7eaee',
    user:      '#3b82f6',
    userFg:    '#ffffff',
    code:      '#0a0c0f',
    codeFg:    '#d8dde3',
    ok:        '#22c55e',
    warn:      '#f59e0b',
    err:       '#ef4444',
    info:      '#3b82f6',
    shadow:    '0 1px 0 rgba(255,255,255,0.04) inset, 0 8px 24px rgba(0,0,0,0.3)',
  },
  light: {
    bg:        '#f6f6f4',
    panel:     '#ffffff',
    panel2:    '#fafaf8',
    border:    'rgba(0,0,0,0.08)',
    borderStrong: 'rgba(0,0,0,0.14)',
    fg:        '#101316',
    fgMuted:   '#5a626c',
    fgDim:     '#8a929b',
    accent:    '#2563eb',
    accentFg:  '#ffffff',
    bubble:    '#ffffff',
    bubbleFg:  '#101316',
    user:      '#2563eb',
    userFg:    '#ffffff',
    code:      '#0e1116',
    codeFg:    '#e7eaee',
    ok:        '#16a34a',
    warn:      '#d97706',
    err:       '#dc2626',
    info:      '#2563eb',
    shadow:    '0 1px 0 rgba(0,0,0,0.02) inset, 0 8px 24px rgba(0,0,0,0.06)',
  },
};

const FONT_SANS = '"Inter", -apple-system, BlinkMacSystemFont, "SF Pro Text", system-ui, sans-serif';
const FONT_MONO = '"JetBrains Mono", "SF Mono", ui-monospace, Menlo, monospace';

Object.assign(window, { THEMES, FONT_SANS, FONT_MONO });
