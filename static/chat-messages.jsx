// Message rendering — bubbles, status notes, tool blocks, approvals, typing.

function MessageMeta({ theme, agent, status, time, align = 'left' }) {
  const a = agent ? AGENTS[agent] : null;
  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'nowrap',
      justifyContent: align === 'right' ? 'flex-end' : 'flex-start',
      fontFamily: FONT_MONO, fontSize: 10.5, color: theme.fgDim,
      padding: align === 'right' ? '0 4px 0 0' : '0 0 0 4px',
      minWidth: 0, overflow: 'hidden',
    }}>
      {a && align === 'left' && (
        <span style={{
          display: 'inline-flex', alignItems: 'center', gap: 6,
          minWidth: 0, overflow: 'hidden', whiteSpace: 'nowrap',
          textOverflow: 'ellipsis',
        }}>
          <span style={{ color: a.color, fontWeight: 600 }}>{a.name}</span>
          {status && (
            <>
              <span style={{ color: theme.fgDim }}>·</span>
              <StatusPill status={status} theme={theme} />
            </>
          )}
        </span>
      )}
      <span style={{ flex: align === 'right' ? 0 : 1, minWidth: 4 }} />
      <span style={{ whiteSpace: 'nowrap', flexShrink: 0 }}>{time}</span>
    </div>
  );
}

function UserBubble({ msg, theme }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 4, alignItems: 'flex-end', maxWidth: '85%', marginLeft: 'auto' }}>
      <div style={{
        background: theme.user, color: theme.userFg,
        padding: '10px 14px',
        borderRadius: '14px 14px 4px 14px',
        fontFamily: FONT_SANS, fontSize: 14, lineHeight: 1.45,
        wordBreak: 'break-word',
      }}>
        {msg.link ? (
            <a href={msg.link} target="_blank" style={{ color: 'inherit', textDecoration: 'underline' }}>{msg.text}</a>
        ) : msg.text}
      </div>
      <MessageMeta theme={theme} time={msg.time} align="right" />
    </div>
  );
}

function AgentBubble({ msg, theme }) {
  const a = AGENTS[msg.agent];
  return (
    <div style={{ display: 'flex', gap: 10, maxWidth: '92%' }}>
      <AgentMark agent={msg.agent} size={26} theme={theme} />
      <div style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 5 }}>
        <MessageMeta theme={theme} agent={msg.agent} status={msg.status} time={msg.time} />
        <div style={{
          background: theme.bubble, color: theme.bubbleFg,
          padding: '10px 14px', borderRadius: '4px 14px 14px 14px',
          fontFamily: FONT_SANS, fontSize: 14, lineHeight: 1.5,
          border: `1px solid ${theme.border}`,
          wordBreak: 'break-word',
        }}>
          {msg.title && <div style={{ fontWeight: 'bold', marginBottom: 5 }}>{msg.title}</div>}
          {msg.link ? (
              <a href={msg.link} target="_blank" style={{ color: 'inherit', textDecoration: 'underline' }}>
                  {renderInline(msg.text, theme)}
              </a>
          ) : renderInline(msg.text, theme)}
        </div>
      </div>
    </div>
  );
}

// Render inline `code` segments
function renderInline(text, theme) {
  if (!text) return null;
  const parts = text.split(/(`[^`]+`)/g);
  return parts.map((p, i) => {
    if (p.startsWith('`') && p.endsWith('`')) {
      return (
        <code key={i} style={{
          fontFamily: FONT_MONO, fontSize: 12.5,
          padding: '1px 5px', borderRadius: 4,
          background: theme.panel2, border: `1px solid ${theme.border}`,
          color: theme.fg,
        }}>{p.slice(1, -1)}</code>
      );
    }
    return <span key={i}>{p}</span>;
  });
}

function StatusNote({ msg, theme }) {
  const a = AGENTS[msg.agent];
  const s = STATUS[msg.status];
  // Strip leading agent-name from text so we don't repeat it.
  const cleaned = (msg.text || '').replace(new RegExp('^' + a.name + ' ', 'i'), '');
  return (
    <div style={{
      display: 'inline-flex', alignItems: 'center', gap: 8,
      padding: '5px 12px', alignSelf: 'center', maxWidth: '90%',
      fontFamily: FONT_MONO, fontSize: 11, color: theme.fgMuted,
      background: theme.panel2, border: `1px solid ${theme.border}`,
      borderRadius: 999, whiteSpace: 'nowrap',
      overflow: 'hidden', textOverflow: 'ellipsis',
    }}>
      <IconDot size={6} color={s.dot} />
      <span style={{ color: a.color, fontWeight: 600 }}>{a.name}</span>
      <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{cleaned}</span>
      <span style={{ color: theme.fgDim, flexShrink: 0 }}>· {msg.time}</span>
    </div>
  );
}

function ToolBlock({ msg, theme, expanded, onToggle }) {
  const a = AGENTS[msg.agent];
  return (
    <div style={{ display: 'flex', gap: 10, maxWidth: '92%' }}>
      <AgentMark agent={msg.agent} size={26} theme={theme} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{
          borderRadius: 10, overflow: 'hidden',
          border: `1px solid ${theme.border}`,
          background: theme.bubble,
        }}>
          <button onClick={onToggle} style={{
            all: 'unset', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 10,
            width: '100%', padding: '8px 12px', boxSizing: 'border-box',
            background: theme.panel2,
            borderBottom: expanded ? `1px solid ${theme.border}` : 'none',
          }}>
            <IconTerminal size={14} style={{ color: a.color }} />
            <span style={{
              flex: 1, minWidth: 0,
              fontFamily: FONT_MONO, fontSize: 12, color: theme.fg,
              overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
            }}>{msg.title}</span>
            <span style={{ fontFamily: FONT_MONO, fontSize: 10.5, color: theme.fgDim }}>{msg.duration}</span>
            <span style={{
              transform: expanded ? 'rotate(90deg)' : 'rotate(0deg)',
              transition: 'transform 0.15s', display: 'flex', color: theme.fgMuted,
            }}><IconChevronRight size={14} /></span>
          </button>
          {expanded && (
            <div style={{
              background: theme.code, color: theme.codeFg,
              padding: '10px 14px',
              fontFamily: FONT_MONO, fontSize: 12, lineHeight: 1.55,
              maxHeight: 220, overflowY: 'auto',
            }}>
              {msg.lines.map((l, i) => (
                <div key={i} style={{
                  color: l.c === 'dim' ? '#7a8693'
                    : l.c === 'ok' ? '#4ade80'
                    : l.c === 'err' ? '#f87171'
                    : l.c === 'warn' ? '#fbbf24'
                    : theme.codeFg,
                }}>{l.t}</div>
              ))}
            </div>
          )}
        </div>
        <div style={{ marginTop: 4 }}>
          <MessageMeta theme={theme} time={msg.time} />
        </div>
      </div>
    </div>
  );
}

function ApprovalCard({ msg, theme, decision, onDecide }) {
  const a = AGENTS[msg.agent];
  const decided = !!decision;
  return (
    <div style={{ display: 'flex', gap: 10, maxWidth: '92%' }}>
      <AgentMark agent={msg.agent} size={26} theme={theme} ring />
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{
          borderRadius: 10, overflow: 'hidden',
          border: `1.5px solid ${decided ? theme.border : theme.accent}`,
          background: theme.bubble,
          boxShadow: decided ? 'none' : `0 0 0 4px ${theme.accent}1a`,
          transition: 'all 0.2s',
        }}>
          <div style={{ padding: '12px 14px 10px' }}>
            <div style={{
              display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8,
              fontFamily: FONT_MONO, fontSize: 10.5, color: theme.fgMuted,
              textTransform: 'uppercase', letterSpacing: 0.6,
            }}>
              <IconSparkle size={12} style={{ color: theme.accent }} />
              Approval requested
              <span style={{ flex: 1 }} />
              <span style={{
                padding: '1px 6px', borderRadius: 4,
                background: theme.ok + '22', color: theme.ok,
                textTransform: 'uppercase', fontSize: 10,
              }}>{msg.risk} risk</span>
            </div>
            <div style={{
              fontFamily: FONT_SANS, fontSize: 14, fontWeight: 500, color: theme.fg,
              marginBottom: 4,
            }}>{msg.title}</div>
            <div style={{
              fontFamily: FONT_MONO, fontSize: 11.5, color: theme.fgMuted,
            }}>{msg.summary}</div>
          </div>
          <div style={{
            display: 'flex', borderTop: `1px solid ${theme.border}`,
          }}>
            {decided ? (
              <div style={{
                flex: 1, padding: '10px 14px',
                fontFamily: FONT_MONO, fontSize: 12, color: theme.fgMuted,
                display: 'flex', alignItems: 'center', gap: 8,
              }}>
                {decision === 'Approve' ? (
                  <><IconCheck size={14} style={{ color: theme.ok }} /> Approved · running now</>
                ) : (
                  <><IconX size={14} style={{ color: theme.err }} /> Denied</>
                )}
              </div>
            ) : (
              <>
                <button onClick={() => onDecide('Deny')} style={{
                  all: 'unset', cursor: 'pointer', flex: 1,
                  padding: '10px 14px', textAlign: 'center',
                  fontFamily: FONT_SANS, fontSize: 13, fontWeight: 500,
                  color: theme.fgMuted,
                  borderRight: `1px solid ${theme.border}`,
                }}>Deny</button>
                <button onClick={() => onDecide('Approve')} style={{
                  all: 'unset', cursor: 'pointer', flex: 1.4,
                  padding: '10px 14px', textAlign: 'center',
                  fontFamily: FONT_SANS, fontSize: 13, fontWeight: 600,
                  color: theme.accentFg, background: theme.accent,
                  display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6,
                }}>
                  <IconCheck size={14} />
                  Approve
                </button>
              </>
            )}
          </div>
        </div>
        <div style={{ marginTop: 4 }}>
          <MessageMeta theme={theme} time={msg.time} />
        </div>
      </div>
    </div>
  );
}

function TypingBubble({ agent, theme }) {
  const a = AGENTS[agent];
  return (
    <div style={{ display: 'flex', gap: 10 }}>
      <AgentMark agent={agent} size={26} theme={theme} />
      <div style={{
        background: theme.bubble, border: `1px solid ${theme.border}`,
        padding: '10px 14px', borderRadius: '4px 14px 14px 14px',
        display: 'flex', gap: 4, alignItems: 'center',
      }}>
        {[0, 1, 2].map((i) => (
          <span key={i} style={{
            width: 6, height: 6, borderRadius: 999, background: a.color,
            opacity: 0.4,
            animation: `pushDot 1.2s ${i * 0.15}s infinite ease-in-out`,
          }} />
        ))}
      </div>
    </div>
  );
}

Object.assign(window, { UserBubble, AgentBubble, StatusNote, ToolBlock, ApprovalCard, TypingBubble, MessageMeta });
