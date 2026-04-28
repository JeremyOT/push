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

function AgentMark({ agent, size = 22, theme, ring = false }) {
  const a = AGENTS[agent];
  if (!a) return null;
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
    }}>
      {a.short}
    </div>
  );
}

function SidebarThreadRow({ thread, active, theme, onClick }) {
  const a = AGENTS[thread.agent];
  return (
    <button
      onClick={onClick}
      style={{
        all: 'unset', cursor: 'pointer', display: 'block',
        padding: '10px 12px', borderRadius: 10,
        background: active ? theme.panel2 : 'transparent',
        boxShadow: active ? `inset 0 0 0 1px ${theme.border}` : 'none',
        transition: 'background 0.15s',
        position: 'relative',
      }}
      onMouseEnter={(e) => { if (!active) e.currentTarget.style.background = theme.panel2; }}
      onMouseLeave={(e) => { if (!active) e.currentTarget.style.background = 'transparent'; }}
    >
      {active && (
        <div style={{
          position: 'absolute', left: -1, top: 10, bottom: 10, width: 2,
          background: a.color, borderRadius: 2,
        }} />
      )}
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 4 }}>
        <AgentMark agent={thread.agent} size={20} theme={theme} />
        <div style={{
          flex: 1, minWidth: 0,
          fontFamily: FONT_SANS, fontSize: 13.5, fontWeight: 500,
          color: theme.fg,
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>{thread.title}</div>
        <span style={{
          fontFamily: FONT_MONO, fontSize: 10.5, color: theme.fgDim,
          whiteSpace: 'nowrap', flexShrink: 0,
        }}>{thread.updated}</span>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, paddingLeft: 30 }}>
        <StatusPill status={thread.status} theme={theme} />
        <div style={{
          flex: 1, minWidth: 0,
          fontFamily: FONT_SANS, fontSize: 12, color: theme.fgMuted,
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>{thread.snippet}</div>
        {thread.unread > 0 && (
          <span style={{
            minWidth: 16, height: 16, padding: '0 5px', borderRadius: 999,
            background: theme.accent, color: theme.accentFg,
            fontFamily: FONT_MONO, fontSize: 10, fontWeight: 600,
            display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
          }}>{thread.unread}</span>
        )}
      </div>
    </button>
  );
}

function Sidebar({ theme, threads, activeId, onSelect, onClose, onOpenPalette, search, setSearch, dark, setDark, icon = APP_ICON }) {
  const filtered = threads.filter((t) =>
    t.title.toLowerCase().includes(search.toLowerCase()) ||
    t.snippet.toLowerCase().includes(search.toLowerCase())
  );
  const pinned = filtered.filter((t) => t.pinned);
  const rest = filtered.filter((t) => !t.pinned);

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

      {/* search */}
      <div style={{ padding: '0 14px 8px' }}>
        <div style={{
          display: 'flex', alignItems: 'center', gap: 8,
          padding: '7px 10px', borderRadius: 8,
          background: theme.panel2,
          border: `1px solid ${theme.border}`,
        }}>
          <IconSearch size={14} style={{ color: theme.fgDim }} />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search threads"
            style={{
              all: 'unset', flex: 1, minWidth: 0,
              fontFamily: FONT_SANS, fontSize: 13, color: theme.fg,
            }}
          />
          <span style={{
            fontFamily: FONT_MONO, fontSize: 10, color: theme.fgDim,
            border: `1px solid ${theme.border}`, padding: '1px 5px', borderRadius: 4,
          }}>⌘K</span>
        </div>
      </div>

      {/* command palette button */}
      <div style={{ padding: '0 14px 8px' }}>
        <button onClick={onOpenPalette} style={{
          all: 'unset', cursor: 'pointer', boxSizing: 'border-box',
          width: '100%', padding: '8px 10px', borderRadius: 8,
          background: theme.accent, color: theme.accentFg,
          fontFamily: FONT_SANS, fontSize: 13, fontWeight: 500,
          display: 'flex', alignItems: 'center', gap: 8, justifyContent: 'center',
          boxShadow: `0 6px 14px ${theme.accent}33`,
        }}>
          <IconPlus size={14} />
          New task
        </button>
      </div>

      {/* threads */}
      <div style={{ flex: 1, overflowY: 'auto', padding: '4px 8px 16px' }}>
        {pinned.length > 0 && (
          <>
            <SectionLabel theme={theme}>Pinned</SectionLabel>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 2, marginBottom: 12 }}>
              {pinned.map((t) => (
                <SidebarThreadRow key={t.id} thread={t} active={t.id === activeId} theme={theme} onClick={() => onSelect(t.id)} />
              ))}
            </div>
          </>
        )}
        <SectionLabel theme={theme}>Active</SectionLabel>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          {rest.map((t) => (
            <SidebarThreadRow key={t.id} thread={t} active={t.id === activeId} theme={theme} onClick={() => onSelect(t.id)} />
          ))}
          {rest.length === 0 && pinned.length === 0 && (
            <div style={{
              padding: 16, textAlign: 'center',
              fontFamily: FONT_SANS, fontSize: 12.5, color: theme.fgDim,
            }}>No threads match "{search}"</div>
          )}
        </div>
      </div>

      {/* footer / agent fleet */}
      <div style={{
        padding: '10px 14px',
        borderTop: `1px solid ${theme.border}`,
        display: 'flex', alignItems: 'center', gap: 10,
      }}>
        <div style={{ display: 'flex', gap: -4 }}>
          {Object.values(AGENTS).map((a, i) => (
            <div key={a.id} style={{ marginLeft: i === 0 ? 0 : -6 }}>
              <AgentMark agent={a.id} size={20} theme={theme} />
            </div>
          ))}
        </div>
        <div style={{ flex: 1, fontFamily: FONT_MONO, fontSize: 10.5, color: theme.fgMuted, letterSpacing: 0.3 }}>
          2 agents · 1 awaiting
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
