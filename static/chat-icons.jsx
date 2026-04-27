// Minimal stroke icons. All currentColor, 1.5 stroke.
const Icon = ({ children, size = 18, style = {} }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none"
       stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"
       style={{ display: 'block', flexShrink: 0, ...style }}>
    {children}
  </svg>
);

const IconSearch = (p) => <Icon {...p}><circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/></Icon>;
const IconSend = (p) => <Icon {...p}><path d="M4 12 20 4l-4 16-4-7-8-1z" fill="currentColor" stroke="none"/></Icon>;
const IconPlus = (p) => <Icon {...p}><path d="M12 5v14M5 12h14"/></Icon>;
const IconCommand = (p) => <Icon {...p}><path d="M9 6a3 3 0 1 0-3 3h12a3 3 0 1 0-3-3v12a3 3 0 1 0 3-3H6a3 3 0 1 0 3 3z"/></Icon>;
const IconChevron = (p) => <Icon {...p}><path d="m6 9 6 6 6-6"/></Icon>;
const IconChevronRight = (p) => <Icon {...p}><path d="m9 6 6 6-6 6"/></Icon>;
const IconMenu = (p) => <Icon {...p}><path d="M4 6h16M4 12h16M4 18h16"/></Icon>;
const IconClose = (p) => <Icon {...p}><path d="M6 6l12 12M18 6 6 18"/></Icon>;
const IconCheck = (p) => <Icon {...p}><path d="m5 12 5 5 9-11"/></Icon>;
const IconX = (p) => <Icon {...p}><path d="M6 6l12 12M18 6 6 18"/></Icon>;
const IconTerminal = (p) => <Icon {...p}><path d="m4 8 4 4-4 4M12 16h8"/></Icon>;
const IconFile = (p) => <Icon {...p}><path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><path d="M14 3v6h6"/></Icon>;
const IconSparkle = (p) => <Icon {...p}><path d="M12 3v6M12 15v6M3 12h6M15 12h6" /></Icon>;
const IconCircle = (p) => <Icon {...p}><circle cx="12" cy="12" r="9"/></Icon>;
const IconSun = (p) => <Icon {...p}><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4 12H2M22 12h-2M5 5l1.5 1.5M17.5 17.5 19 19M5 19l1.5-1.5M17.5 6.5 19 5"/></Icon>;
const IconMoon = (p) => <Icon {...p}><path d="M21 13a9 9 0 1 1-9-10 7 7 0 0 0 9 10z"/></Icon>;
const IconDot = ({ size = 8, color = 'currentColor' }) => (
  <span style={{ width: size, height: size, borderRadius: 999, background: color, display: 'inline-block', flexShrink: 0 }} />
);
const IconCopy = (p) => <Icon {...p}><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/></Icon>;
const IconRefresh = (p) => <Icon {...p}><path d="M3 12a9 9 0 0 1 15-6.7L21 8M21 3v5h-5M21 12a9 9 0 0 1-15 6.7L3 16M3 21v-5h5"/></Icon>;
const IconStop = (p) => <Icon {...p}><rect x="6" y="6" width="12" height="12" rx="1.5" fill="currentColor" stroke="none"/></Icon>;
const IconArrowUp = (p) => <Icon {...p}><path d="M12 19V5M5 12l7-7 7 7"/></Icon>;
const IconAttach = (p) => <Icon {...p}><path d="M21 11.5 12.5 20a5.5 5.5 0 1 1-7.8-7.8l8.5-8.5a3.7 3.7 0 1 1 5.2 5.2l-8.5 8.5a1.8 1.8 0 1 1-2.6-2.6l7.8-7.8"/></Icon>;

Object.assign(window, {
  IconSearch, IconSend, IconPlus, IconCommand, IconChevron, IconChevronRight,
  IconMenu, IconClose, IconCheck, IconX, IconTerminal, IconFile, IconSparkle,
  IconCircle, IconSun, IconMoon, IconDot, IconCopy, IconRefresh, IconStop, IconArrowUp, IconAttach,
});
