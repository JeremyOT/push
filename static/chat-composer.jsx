// Composer (input area) + command palette + chat header + main chat view.

function ChatHeader({ theme, thread, onMenu, isPhone, solo = false }) {
  const a = AGENTS[thread.agent];
  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 12,
      padding: '14px 20px',
      paddingTop: isPhone ? 'calc(14px + env(safe-area-inset-top, 0px))' : '14px',
      borderBottom: `1px solid ${theme.border}`,
      background: theme.panel,
      flexShrink: 0,
    }}>
      {isPhone && !solo && (
        <button onClick={onMenu} style={{
          all: 'unset', cursor: 'pointer', padding: 6, borderRadius: 6,
          color: theme.fgMuted, display: 'flex',
        }}><IconMenu size={18} /></button>
      )}
      {isPhone && solo && (
        <BrandMark icon={APP_ICON} theme={theme} size={28} pulse style={{ marginRight: 4 }} />
      )}
      <AgentMark agent={thread.agent} size={32} theme={theme} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{
          fontFamily: FONT_SANS, fontSize: 15, fontWeight: 600, color: theme.fg,
          letterSpacing: -0.2,
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>{thread.title}</div>
        <div style={{
          display: 'flex', alignItems: 'center', gap: 6,
          fontFamily: FONT_MONO, fontSize: 11, color: theme.fgMuted,
          whiteSpace: 'nowrap', overflow: 'hidden',
        }}>
          <span style={{ color: a.color, fontWeight: 600 }}>{a.name}</span>
          {thread.id !== 't1' && (
            <>
              <span>·</span>
              <StatusPill status={thread.status} theme={theme} />
            </>
          )}
        </div>
      </div>
    </div>
  );
}

function Composer({ theme, value, setValue, onSend, onOpenPalette, agentColor, isWorking, onStop }) {
  const taRef = React.useRef(null);
  React.useEffect(() => {
    const ta = taRef.current;
    if (!ta) return;
    ta.style.height = 'auto';
    ta.style.height = Math.min(ta.scrollHeight, 140) + 'px';
  }, [value]);

  const send = () => {
    if (!value.trim()) return;
    onSend(value.trim());
    setValue('');
  };

  return (
    <div style={{
      padding: 12, 
      paddingBottom: 'calc(12px + env(safe-area-inset-bottom, 0px))',
      borderTop: `1px solid ${theme.border}`,
      background: theme.panel, flexShrink: 0,
    }}>
      <div style={{
        display: 'flex', alignItems: 'flex-end', gap: 8,
        padding: 8, borderRadius: 14,
        background: theme.panel2,
        border: `1px solid ${theme.border}`,
        boxShadow: `0 1px 0 ${theme.border} inset`,
      }}>
        <button onClick={onOpenPalette} style={{
          all: 'unset', cursor: 'pointer',
          width: 32, height: 32, borderRadius: 8,
          color: theme.fgMuted,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          flexShrink: 0,
        }} title="Commands (⌘K)"><IconPlus size={16} /></button>
        <textarea
          ref={taRef}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault();
              send();
            }
          }}
          rows={1}
          placeholder="Send a message, or / for commands…"
          style={{
            all: 'unset', flex: 1, minWidth: 0,
            fontFamily: FONT_SANS, fontSize: 14, lineHeight: 1.45,
            color: theme.fg,
            padding: '6px 4px',
            resize: 'none', maxHeight: 140, overflowY: 'auto',
          }}
        />
        <button onClick={send} disabled={!value.trim()} style={{
          all: 'unset', cursor: value.trim() ? 'pointer' : 'not-allowed',
          width: 32, height: 32, borderRadius: 8,
          background: value.trim() ? agentColor : theme.borderStrong,
          color: '#fff',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          flexShrink: 0,
          transition: 'background 0.15s',
          boxShadow: value.trim() ? `0 4px 12px ${agentColor}44` : 'none',
        }} title="Send (Enter)"><IconArrowUp size={15} /></button>
      </div>
      <div style={{
        display: 'flex', gap: 6, marginTop: 8, flexWrap: 'wrap',
      }}>
        {['/run', '/diff', '/explain', '/test'].map((s) => (
          <button key={s} onClick={() => setValue(s + ' ')} style={{
            all: 'unset', cursor: 'pointer',
            padding: '4px 9px', borderRadius: 999,
            fontFamily: FONT_MONO, fontSize: 11, color: theme.fgMuted,
            background: theme.panel2, border: `1px solid ${theme.border}`,
          }}>{s}</button>
        ))}
      </div>
    </div>
  );
}

const PALETTE_ITEMS = [
  { id: 'p1', cmd: '/run', label: 'Run a script', desc: 'Execute a shell command via the active agent', icon: 'terminal' },
  { id: 'p2', cmd: '/diff', label: 'Show diff', desc: 'Inline diff between current branch and main', icon: 'file' },
  { id: 'p3', cmd: '/explain', label: 'Explain code', desc: 'Walk through a file or selection', icon: 'sparkle' },
  { id: 'p4', cmd: '/test', label: 'Run tests', desc: 'Execute test suite and summarize failures', icon: 'check' },
];

function CommandPalette({ theme, open, onClose, onPick }) {
  const [q, setQ] = React.useState('');
  const [sel, setSel] = React.useState(0);
  const inputRef = React.useRef(null);
  React.useEffect(() => {
    if (open) {
      setQ(''); setSel(0);
      setTimeout(() => inputRef.current?.focus(), 30);
    }
  }, [open]);
  if (!open) return null;
  const filtered = PALETTE_ITEMS.filter((i) =>
    i.label.toLowerCase().includes(q.toLowerCase()) || i.cmd.includes(q.toLowerCase())
  );
  const pick = (item) => { onPick(item); onClose(); };
  const iconFor = (k) => ({
    terminal: <IconTerminal size={14} />, file: <IconFile size={14} />,
    sparkle: <IconSparkle size={14} />, check: <IconCheck size={14} />,
    plus: <IconPlus size={14} />, search: <IconSearch size={14} />,
  }[k]);

  return (
    <div onClick={onClose} style={{
      position: 'absolute', inset: 0, zIndex: 100,
      background: 'rgba(0,0,0,0.45)',
      backdropFilter: 'blur(4px)', WebkitBackdropFilter: 'blur(4px)',
      display: 'flex', alignItems: 'flex-start', justifyContent: 'center',
      paddingTop: '14%',
    }}>
      <div onClick={(e) => e.stopPropagation()} style={{
        width: 'min(440px, 88%)',
        background: theme.panel, color: theme.fg,
        borderRadius: 14, overflow: 'hidden',
        border: `1px solid ${theme.borderStrong}`,
        boxShadow: '0 20px 60px rgba(0,0,0,0.5), 0 8px 16px rgba(0,0,0,0.3)',
      }}>
        <div style={{
          display: 'flex', alignItems: 'center', gap: 10,
          padding: '14px 16px', borderBottom: `1px solid ${theme.border}`,
        }}>
          <IconSearch size={15} style={{ color: theme.fgMuted }} />
          <input
            ref={inputRef}
            value={q}
            onChange={(e) => { setQ(e.target.value); setSel(0); }}
            onKeyDown={(e) => {
              if (e.key === 'ArrowDown') { e.preventDefault(); setSel((s) => Math.min(s + 1, filtered.length - 1)); }
              if (e.key === 'ArrowUp') { e.preventDefault(); setSel((s) => Math.max(s - 1, 0)); }
              if (e.key === 'Enter' && filtered[sel]) pick(filtered[sel]);
              if (e.key === 'Escape') onClose();
            }}
            placeholder="Type a command or search…"
            style={{
              all: 'unset', flex: 1,
              fontFamily: FONT_SANS, fontSize: 14, color: theme.fg,
            }}
          />
          <span style={{
            fontFamily: FONT_MONO, fontSize: 10, color: theme.fgDim,
            border: `1px solid ${theme.border}`, padding: '1px 5px', borderRadius: 4,
          }}>esc</span>
        </div>
        <div style={{ maxHeight: 320, overflowY: 'auto', padding: 6 }}>
          {filtered.map((item, i) => (
            <button key={item.id}
              onMouseEnter={() => setSel(i)}
              onClick={() => pick(item)}
              style={{
                all: 'unset', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 12,
                width: '100%', boxSizing: 'border-box',
                padding: '8px 10px', borderRadius: 8,
                background: i === sel ? theme.panel2 : 'transparent',
                boxShadow: i === sel ? `inset 0 0 0 1px ${theme.border}` : 'none',
              }}>
              <span style={{
                width: 26, height: 26, borderRadius: 6,
                background: theme.panel2, color: theme.fg,
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                border: `1px solid ${theme.border}`,
              }}>{iconFor(item.icon)}</span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontFamily: FONT_SANS, fontSize: 13, color: theme.fg }}>{item.label}</div>
                <div style={{ fontFamily: FONT_MONO, fontSize: 11, color: theme.fgMuted, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{item.desc}</div>
              </div>
              <code style={{
                fontFamily: FONT_MONO, fontSize: 11, color: theme.fgMuted,
                padding: '2px 6px', borderRadius: 4,
                background: theme.panel2, border: `1px solid ${theme.border}`,
              }}>{item.cmd}</code>
            </button>
          ))}
          {filtered.length === 0 && (
            <div style={{ padding: 18, textAlign: 'center', color: theme.fgDim, fontFamily: FONT_SANS, fontSize: 13 }}>
              No commands match "{q}"
            </div>
          )}
        </div>
        <div style={{
          padding: '8px 14px', borderTop: `1px solid ${theme.border}`,
          fontFamily: FONT_MONO, fontSize: 10.5, color: theme.fgDim,
          display: 'flex', gap: 14,
        }}>
          <span>↑↓ navigate</span>
          <span>↵ select</span>
          <span>esc close</span>
        </div>
      </div>
    </div>
  );
}

Object.assign(window, { ChatHeader, Composer, CommandPalette });
