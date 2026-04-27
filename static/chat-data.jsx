// Mock data for the Push AI chat app.
// Agents, threads, messages with status indicators, tool outputs, approvals.

const AGENTS = {
  remote: {
    id: 'remote',
    name: 'Remote',
    short: 'RM',
    model: 'push-service',
    color: '#9aa3ad',
    colorSoft: 'rgba(154,163,173,0.16)',
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
    agent: 'remote',
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
