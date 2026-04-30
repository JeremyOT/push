// Chat sidebar + thread list. Used directly on tablet, in a drawer on phone.

function StatusPill({ status, theme, mono = true }) {
  const s = STATUS[status];
  if (!s) return null;
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 6,
      fontFamily: mono ? FONT_MONO : FONT_SANS,
      fontSize: 11, color: theme.fgMuted, letterSpacing: 0.2,
      textTransform: 'lowercase', whiteSpace: 'nowrap', flexShrink: 0,
    }}>
      <IconDot size={6} color={s.dot} />
      {s.label}
    </span>
  );
}

function AgentMark({ agent, size = 22, theme, ring = false, status = null }) {
  const a = AGENTS[agent];
  if (!a) return null;
  const s = status ? STATUS[status] : null;
  return (
    <div style={{
      width: size, height: size, borderRadius: 6,
      background: a.colorSoft,
      border: `1px solid ${a.color}33`,
      color: a.color,
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      fontFamily: FONT_MONO, fontSize: Math.round(size * 0.42), fontWeight: 600,
      flexShrink: 0,
      boxShadow: ring ? `0 0 0 3px ${a.color}22` : 'none',
      position: 'relative',
    }}>
      {a.short}
      {s && (
        <div style={{
          position: 'absolute', bottom: -2, right: -2,
          width: 8, height: 8, borderRadius: 99,
          background: s.dot, border: `2px solid ${theme.panel}`,
        }} />
      )}
    </div>
  );
}

function SidebarThreadRow({ thread, active, theme, onClick, depth = 0 }) {
  const a = AGENTS[thread.agent];
  return (
    <button
      onClick={onClick}
      style={{
        all: 'unset', cursor: 'pointer', display: 'block',
        padding: `8px 12px 8px ${12 + (depth * 16)}px`, borderRadius: 10,
        background: active ? theme.panel2 : 'transparent',
        boxShadow: active ? `inset 0 0 0 1px ${theme.border}` : 'none',
        transition: 'background 0.15s',
        position: 'relative',
        marginBottom: 2,
      }}
      onMouseEnter={(e) => { if (!active) e.currentTarget.style.background = theme.panel2; }}
      onMouseLeave={(e) => { if (!active) e.currentTarget.style.background = 'transparent'; }}
    >
      {active && (
        <div style={{
          position: 'absolute', left: -1, top: 8, bottom: 8, width: 2,
          background: a.color, borderRadius: 2,
        }} />
      )}
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 2 }}>
        <AgentMark agent={thread.agent} size={18} theme={theme} />
        <div style={{
          flex: 1, minWidth: 0,
          fontFamily: FONT_SANS, fontSize: 13, fontWeight: 500,
          color: theme.fg,
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>{thread.title}</div>
        <span style={{
          fontFamily: FONT_MONO, fontSize: 9.5, color: theme.fgDim,
          whiteSpace: 'nowrap', flexShrink: 0,
        }}>{thread.updated}</span>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, paddingLeft: 28 }}>
        {thread.id !== 't1' && <StatusPill status={thread.status} theme={theme} />}
        <div style={{
          flex: 1, minWidth: 0,
          fontFamily: FONT_SANS, fontSize: 11.5, color: theme.fgMuted,
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>{thread.snippet}</div>
        {thread.unread > 0 && (
          <span style={{
            minWidth: 14, height: 14, padding: '0 4px', borderRadius: 999,
            background: theme.accent, color: theme.accentFg,
            fontFamily: FONT_MONO, fontSize: 9, fontWeight: 600,
            display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
          }}>{thread.unread}</span>
        )}
      </div>
    </button>
  );
}

function SidebarTree({ threads, activeId, theme, onSelect, depth = 0, statusOverride = null }) {
  // Sort threads by path length so parents come first
  const sorted = [...threads].sort((a, b) => (a.sessionPath || '').length - (b.sessionPath || '').length);
  
  const roots = [];
  const childrenMap = {};

  threads.forEach(t => {
    const path = t.sessionPath || '';
    // Find potential parent: another thread where our path starts with its path
    // and its path is shorter but the longest possible match.
    let parent = null;
    threads.forEach(p => {
      if (t.id === p.id) return;
      const pPath = p.sessionPath || '';
      if (!pPath) return;
      if (path.startsWith(pPath) && path !== pPath && path[pPath.length] === '/') {
        if (!parent || pPath.length > (parent.sessionPath || '').length) {
          parent = p;
        }
      }
    });

    if (parent) {
      childrenMap[parent.id] = childrenMap[parent.id] || [];
      childrenMap[parent.id].push(t);
    } else {
      roots.push(t);
    }
  });

  const renderNode = (t, d) => (
    <React.Fragment key={t.id}>
      <SidebarThreadRow 
        thread={statusOverride ? { ...t, status: statusOverride } : t} 
        active={t.id === activeId} theme={theme} onClick={() => onSelect(t.id)} depth={d} 
      />
      {childrenMap[t.id] && childrenMap[t.id].map(c => renderNode(c, d + 1))}
    </React.Fragment>
  );

  return (
    <div style={{ display: 'flex', flexDirection: 'column' }}>
      {roots.map(r => renderNode(r, depth))}
    </div>
  );
}

function Sidebar({ theme, threads, activeId, onSelect, onClose, onOpenPalette, dark, setDark, icon = APP_ICON }) {
  const now = Date.now();
  const oneDay = 24 * 60 * 60 * 1000;

  const mainFeed = threads.find(t => t.id === 't1');
  const activeThreads = threads.filter(t => t.id !== 't1' && t.active);
  const recentThreads = threads.filter(t => {
    if (t.id === 't1' || t.active) return false;
    const ts = typeof t.lastTimestamp === 'string' ? new Date(t.lastTimestamp).getTime() : t.lastTimestamp;
    if (!ts || isNaN(ts)) return false;
    return (now - ts) < oneDay;
  });

  return (
    <div style={{
      display: 'flex', flexDirection: 'column', height: '100%',
      background: theme.panel, color: theme.fg,
      borderRight: `1px solid ${theme.border}`,
    }}>
      {/* brand row */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 10,
        padding: '14px 14px 10px',
      }}>
        <BrandMark icon={icon} theme={theme} size={28} style={{ borderRadius: 7 }} />
        <div style={{ flex: 1, fontFamily: FONT_SANS, fontWeight: 600, fontSize: 15, letterSpacing: -0.2 }}>
          Push
        </div>
        <button onClick={() => setDark(!dark)} style={{
          all: 'unset', cursor: 'pointer', padding: 6, borderRadius: 6,
          color: theme.fgMuted, display: 'flex',
        }} title="Toggle theme">
          {dark ? <IconSun size={15} /> : <IconMoon size={15} />}
        </button>
        {onClose && (
          <button onClick={onClose} style={{
            all: 'unset', cursor: 'pointer', padding: 6, borderRadius: 6,
            color: theme.fgMuted, display: 'flex',
          }}><IconClose size={16} /></button>
        )}
      </div>

      {/* threads */}
      <div style={{ flex: 1, overflowY: 'auto', padding: '4px 8px 16px' }}>
        {mainFeed && (
            <div style={{ marginBottom: 12 }}>
                <SidebarThreadRow thread={mainFeed} active={mainFeed.id === activeId} theme={theme} onClick={() => onSelect(mainFeed.id)} />
            </div>
        )}

        {activeThreads.length > 0 && (
            <>
                <SectionLabel theme={theme}>Active</SectionLabel>
                <div style={{ marginBottom: 12 }}>
                    <SidebarTree threads={activeThreads} activeId={activeId} theme={theme} onSelect={onSelect} />
                </div>
            </>
        )}

        {recentThreads.length > 0 && (
            <>
                <SectionLabel theme={theme}>Recent</SectionLabel>
                <div>
                    <SidebarTree threads={recentThreads} activeId={activeId} theme={theme} onSelect={onSelect} statusOverride="passive" />
                </div>
            </>
        )}

        {activeThreads.length === 0 && recentThreads.length === 0 && !mainFeed && (
            <div style={{
              padding: 16, textAlign: 'center',
              fontFamily: FONT_SANS, fontSize: 12.5, color: theme.fgDim,
            }}>No threads</div>
        )}
      </div>

      {/* footer / agent fleet */}
      <div style={{
        padding: '10px 14px',
        borderTop: `1px solid ${theme.border}`,
        display: 'flex', alignItems: 'center', gap: 10,
      }}>
        <div style={{ display: 'flex', gap: -4 }}>
          {Object.values(AGENTS)
            .filter(a => {
              return threads.some(t => {
                if (t.agent !== a.id) return false;
                if (t.active) return true;
                if (t.id === 't1') return false;
                const ts = typeof t.lastTimestamp === 'string' ? new Date(t.lastTimestamp).getTime() : t.lastTimestamp;
                return !isNaN(ts) && (now - ts) < oneDay;
              });
            })
            .map((a, i) => {
              const agentThreads = threads.filter(t => {
                if (t.agent !== a.id) return false;
                if (t.active) return true;
                if (t.id === 't1') return false;
                const ts = typeof t.lastTimestamp === 'string' ? new Date(t.lastTimestamp).getTime() : t.lastTimestamp;
                return !isNaN(ts) && (now - ts) < oneDay;
              });

              let status = 'passive';
              if (agentThreads.some(t => t.active && t.status === 'working')) status = 'working';
              else if (agentThreads.some(t => t.active && t.status === 'ready')) status = 'ready';

              return (
                <button key={a.id} 
                  onClick={() => {
                    const t = agentThreads.find(th => th.active) || agentThreads[0];
                    if (t) onSelect(t.id);
                  }}
                  style={{ all: 'unset', cursor: 'pointer', marginLeft: i === 0 ? 0 : -6 }}
                >
                  <AgentMark agent={a.id} size={20} theme={theme} status={status} />
                </button>
              );
            })
          }
        </div>
        <div style={{ flex: 1, fontFamily: FONT_MONO, fontSize: 10.5, color: theme.fgMuted, letterSpacing: 0.3 }}>
          {activeThreads.length} active · {activeThreads.filter(t => t.status === 'awaiting' || t.status === 'ready' || t.status === 'idle').length} ready
        </div>
      </div>
    </div>
  );
}

function SectionLabel({ children, theme }) {
  return (
    <div style={{
      fontFamily: FONT_MONO, fontSize: 10, fontWeight: 500,
      color: theme.fgDim, letterSpacing: 0.6, textTransform: 'uppercase',
      padding: '8px 12px 6px',
    }}>{children}</div>
  );
}

Object.assign(window, { Sidebar, AgentMark, StatusPill });
