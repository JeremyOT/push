// Theme tokens for Push chat app — light + dark.
// Pro/utilitarian devtool feel. Single accent (blue), neutral grays.

const THEMES = {
  dark: {
    dark:      true,
    bg:        '#0b0f19', // rich deep navy-black
    panel:     '#111420', // dark slate-navy panel
    panel2:    '#171c2c', // card/hover background
    border:    'rgba(148,163,184,0.08)', // fine slate border
    borderStrong: 'rgba(148,163,184,0.16)',
    fg:        '#f1f5f9', // slate-100 (clean off-white)
    fgMuted:   '#cbd5e1', // slate-300
    fgDim:     '#64748b', // slate-500
    accent:    '#6366f1', // modern indigo-500
    accentFg:  '#ffffff',
    link:      '#818cf8', // legible indigo link on navy
    bubble:    '#1e2438', // bubble slate-navy
    bubbleFg:  '#f8fafc',
    user:      '#4f46e5', // indigo-600 user bubble
    userFg:    '#ffffff',
    code:      '#0a0c16', // code block background
    codeFg:    '#e2e8f0',
    ok:        '#10b981', // emerald-500
    warn:      '#f59e0b', // amber-500
    err:       '#ef4444', // red-500
    info:      '#3b82f6',
    shadow:    '0 1px 0 rgba(255,255,255,0.02) inset, 0 8px 24px rgba(0,0,0,0.4)',
  },
  light: {
    dark:      false,
    bg:        '#f8fafc', // slate-50
    panel:     '#ffffff',
    panel2:    '#f1f5f9', // slate-100
    border:    'rgba(15,23,42,0.06)',
    borderStrong: 'rgba(15,23,42,0.12)',
    fg:        '#0f172a', // slate-900
    fgMuted:   '#475569', // slate-600
    fgDim:     '#64748b', // slate-500
    accent:    '#4f46e5', // indigo-600
    accentFg:  '#ffffff',
    link:      '#3b82f6',
    bubble:    '#f1f5f9', // slate-100
    bubbleFg:  '#0f172a',
    user:      '#4f46e5',
    userFg:    '#ffffff',
    code:      '#0f172a',
    codeFg:    '#e2e8f0',
    ok:        '#10b981',
    warn:      '#f59e0b',
    err:       '#ef4444',
    info:      '#3b82f6',
    shadow:    '0 1px 0 rgba(0,0,0,0.02) inset, 0 8px 24px rgba(0,0,0,0.06)',
  },
};

const FONT_SANS = '"Inter", -apple-system, BlinkMacSystemFont, "SF Pro Text", system-ui, sans-serif';
const FONT_MONO = '"JetBrains Mono", "SF Mono", ui-monospace, Menlo, monospace';

Object.assign(window, { THEMES, FONT_SANS, FONT_MONO });
