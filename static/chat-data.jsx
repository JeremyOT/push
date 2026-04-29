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
  gemini: {
    id: 'gemini',
    name: 'Gemini',
    short: 'GM',
    model: '2.5-pro',
    color: '#4f8ef7',
    colorSoft: 'rgba(79,142,247,0.16)',
  },
  claude: {
    id: 'claude',
    name: 'Claude',
    short: 'CL',
    model: 'sonnet-4.5',
    color: '#d97757',
    colorSoft: 'rgba(217,119,87,0.16)',
  },
  tmux: {
    id: 'tmux',
    name: 'Tmux',
    short: 'TX',
    model: 'tmux-service',
    color: '#10b981',
    colorSoft: 'rgba(16,185,129,0.16)',
  },
};

// status: 'done' | 'idle' | 'working' | 'error' | 'awaiting'
const STATUS = {
  done:     { label: 'done',         dot: '#22c55e' },
  ready:    { label: 'awaiting',     dot: '#22c55e' },
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
    lastMsgId: 0,
  },
];

const MESSAGES_T1 = [];

Object.assign(window, { AGENTS, STATUS, THREADS, MESSAGES_T1 });
