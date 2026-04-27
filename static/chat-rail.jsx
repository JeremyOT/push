// Agent Rail — distinctive vertical activity stream shown on tablet.
// Shows all agents with live status, a heartbeat sparkline, and current task.

function AgentRail({ theme, threads, activeAgent, onSelectAgent, icon = APP_ICON }) {
  const agents = Object.values(AGENTS);

  // Aggregate stats per agent
  const stats = agents.map((a) => {
    const list = threads.filter((t) => t.agent === a.id);
    const working = list.find((t) => t.status === 'working');
    const awaiting = list.find((t) => t.status === 'awaiting');
    return {
      agent: a,
      count: list.length,
      current: working || awaiting || list[0],
      status: working ? 'working' : awaiting ? 'awaiting' : list[0]?.status || 'idle',
    };
  });

  return (
    <div style={{
      width: 80, height: '100%', flexShrink: 0,
      background: theme.bg,
      borderRight: `1px solid ${theme.border}`,
      display: 'flex', flexDirection: 'column',
      padding: '14px 0 14px',
      position: 'relative',
    }}>
      {/* brand mark */}
      <div style={{
        display: 'flex', justifyContent: 'center', marginBottom: 18,
        padding: '4px 6px 0',
      }}>
        <BrandMark icon={icon} theme={theme} size={36} pulse />
      </div>

      {/* agent list */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 14, alignItems: 'center' }}>
        {stats.map(({ agent: a, status, current }) => {
          const isActive = a.id === activeAgent;
          const s = STATUS[status];
          return (
            <button key={a.id} onClick={() => onSelectAgent && onSelectAgent(a.id)} style={{
              all: 'unset', cursor: 'pointer',
              display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 4,
              position: 'relative',
            }}>
              {/* active indicator */}
              {isActive && (
                <div style={{
                  position: 'absolute', left: -14, top: '50%', transform: 'translateY(-50%)',
                  width: 3, height: 28, borderRadius: 2,
                  background: a.color,
                  boxShadow: `0 0 12px ${a.color}`,
                }} />
              )}
              <div style={{
                position: 'relative',
                width: 44, height: 44, borderRadius: 12,
                background: isActive ? a.color : a.colorSoft,
                border: `1.5px solid ${isActive ? a.color : a.color + '44'}`,
                color: isActive ? '#fff' : a.color,
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontFamily: FONT_MONO, fontSize: 14, fontWeight: 700,
                boxShadow: isActive ? `0 8px 22px ${a.color}55` : 'none',
                transition: 'all 0.2s',
              }}>
                {a.short}
                {/* status dot */}
                <span style={{
                  position: 'absolute', bottom: -3, right: -3,
                  width: 12, height: 12, borderRadius: 999,
                  background: s.dot,
                  border: `2.5px solid ${theme.bg}`,
                  animation: status === 'working' ? 'pushPulse 1.4s infinite' : 'none',
                }} />
              </div>
              <span style={{
                fontFamily: FONT_MONO, fontSize: 9.5, color: theme.fgDim,
                letterSpacing: 0.4, textTransform: 'uppercase',
              }}>{s.label.split(' ')[0]}</span>
            </button>
          );
        })}

        {/* + new agent */}
        <button style={{
          all: 'unset', cursor: 'pointer',
          width: 44, height: 44, borderRadius: 12,
          border: `1.5px dashed ${theme.borderStrong}`,
          color: theme.fgDim,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          marginTop: 4,
        }}>
          <IconPlus size={16} />
        </button>
      </div>

      {/* sparkline / heartbeat */}
    </div>
  );
}

function Sparkline({ theme }) {
  return null;
}

Object.assign(window, { AgentRail });
