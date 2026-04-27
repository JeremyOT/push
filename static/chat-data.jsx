// Mock data for the Push AI chat app.
// Agents, threads, messages with status indicators, tool outputs, approvals.

const AGENTS = {
  gemini: {
    id: 'gemini',
    name: 'Gemini',
    short: 'GM',
    model: '2.5-pro',
    color: '#4f8ef7',
    colorSoft: 'rgba(79,142,247,0.16)',
  },
};

// status: 'done' | 'idle' | 'working' | 'error' | 'awaiting'
const STATUS = {
  done:     { label: 'done',         dot: '#22c55e' },
  idle:     { label: 'idle',         dot: '#9ca3af' },
  working:  { label: 'working',      dot: '#f59e0b' },
  error:    { label: 'error',        dot: '#ef4444' },
  awaiting: { label: 'awaiting you', dot: '#3b82f6' },
};

const THREADS = [
  {
    id: 't1',
    agent: 'gemini',
    title: 'Main Feed',
    status: 'done',
    snippet: 'Push notification system active.',
    updated: 'Now',
    unread: 0,
    pinned: true,
  },
];

const MESSAGES_T1 = [];

Object.assign(window, { AGENTS, STATUS, THREADS, MESSAGES_T1 });
