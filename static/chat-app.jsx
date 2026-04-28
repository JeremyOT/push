// Main PushChat container — composes sidebar + chat view, handles state.
// Adapts to phone (drawer sidebar) vs tablet (split layout) based on `mode` prop.

function PushChat({ theme, dark, setDark, mode = 'tablet', icon = APP_ICON, solo = false, config = { interactive: false } }) {
  const [threads, setThreads] = React.useState(THREADS);
  const [activeId, setActiveId] = React.useState('t1');
  const [search, setSearch] = React.useState('');
  const [drawerOpen, setDrawerOpen] = React.useState(false);
  const [paletteOpen, setPaletteOpen] = React.useState(false);
  const [composerValue, setComposerValue] = React.useState('');
  const [expandedTools, setExpandedTools] = React.useState({});
  const [decisions, setDecisions] = React.useState({});
  const [messages, setMessages] = React.useState([]);
  const [typing, setTyping] = React.useState(null);
  const isPhone = mode === 'phone';

  const thread = threads.find((t) => t.id === activeId) || threads[0];
  const agent = AGENTS[thread.agent];

  const scrollRef = React.useRef(null);
  const newestId = React.useRef(0);

  const formatTime = (timestamp) => {
    const date = new Date(timestamp);
    let h = date.getHours();
    const m = String(date.getMinutes()).padStart(2, '0');
    const ap = h >= 12 ? 'PM' : 'AM';
    h = h % 12 || 12;
    return `${h}:${m} ${ap}`;
  };

  const mapMessage = (msg) => {
    const base = {
      id: msg.id,
      identifier: msg.identifier,
      update: msg.update,
      time: formatTime(msg.timestamp),
      text: msg.detailed_message || msg.message,
      link: msg.link,
      title: msg.title,
      sessionId: msg.session_id,
    };

    if (msg.is_user) {
      return {
        ...base,
        kind: 'user',
      };
    }

    let agentId = 'remote';
    
    if (msg.agent && AGENTS[msg.agent.toLowerCase()]) {
      agentId = msg.agent.toLowerCase();
    } else if (msg.title) {
      // Fallback: Parse agent from title prefix if present (e.g. "Gemini - Done")
      const match = msg.title.match(/^(\w+)\s+-\s+/);
      if (match) {
        const potential = match[1].toLowerCase();
        if (AGENTS[potential]) {
          agentId = potential;
        }
      }
    }

    // Try to detect tool or approval from title/message
    if (msg.title && (msg.title.includes('Approval') || msg.title.includes('Approve'))) {
      return {
        ...base,
        kind: 'approval',
        agent: agentId,
        summary: msg.message,
        risk: 'unknown',
        actions: ['Approve', 'Deny']
      };
    }

    if (msg.title && (msg.title.includes('Run') || msg.title.includes('$'))) {
      return {
        ...base,
        kind: 'tool',
        agent: agentId,
        tool: 'shell',
        duration: '',
        lines: msg.message.split('\n').map(l => ({ c: 'fg', t: l }))
      };
    }

    let status = null;
    if (msg.status === 'w') status = 'working';
    else if (msg.status === 'd') status = 'done';
    else if (msg.status === 'r') status = 'ready';
    else if (msg.title) {
      // Fallback: Parse status from title
      if (msg.title.endsWith(' - Done')) status = 'done';
      else if (msg.title.endsWith(' - Working')) status = 'working';
    }

    return {
      ...base,
      kind: 'agent',
      agent: agentId,
      status: status,
    };
  };

  const processMessage = (msg, setMessages, setThreads, isHistory = false) => {
    const mapped = mapMessage(msg);
    
    // Handle session registration and activity
    if (msg.title === 'session-register' && msg.session_id) {
        setThreads(prev => {
            const exists = prev.some(t => t.id === msg.session_id);
            if (exists) return prev;
            return [...prev, {
                id: msg.session_id,
                agent: mapped.agent || 'remote',
                title: msg.message.replace('Registered session: ', '') || 'CLI Agent',
                status: mapped.status || 'done',
                snippet: 'Active session',
                updated: mapped.time,
                unread: 0,
                pinned: false,
                sessionId: msg.session_id,
                active: !isHistory // Only mark active if real-time
            }];
        });
    }

    if (msg.title === 'session-active' && msg.session_id) {
        setThreads(prev => {
            return prev.map(t => t.id === msg.session_id ? { ...t, active: true } : t);
        });
    }

    if (msg.title === 'session-inactive' && msg.session_id) {
        setThreads(prev => {
            // Remove thread if it becomes inactive (unless it's pinned/main feed)
            const threadToRemove = prev.find(t => t.id === msg.session_id);
            if (threadToRemove && !threadToRemove.pinned) {
                if (activeId === msg.session_id) {
                    setActiveId('t1');
                }
                return prev.filter(t => t.id !== msg.session_id);
            }
            return prev.map(t => t.id === msg.session_id ? { ...t, active: false } : t);
        });
    }

    if (msg.title === 'heartbeat' && msg.message !== undefined) {
        const activeIds = msg.message ? msg.message.split(',') : [];
        setThreads(prev => {
            let changed = false;
            const updated = prev.map(t => {
                if (activeIds.includes(t.id) && !t.active) {
                    changed = true;
                    return { ...t, active: true };
                }
                return t;
            });

            const filtered = updated.filter(t => {
                if (t.pinned || t.id === 't1' || activeIds.includes(t.id)) {
                    return true;
                }
                if (activeId === t.id) {
                    setActiveId('t1');
                }
                changed = true;
                return false;
            });
            return changed ? filtered : prev;
        });
    }

    if (msg.id === 0) return; // Don't add status-only messages to message list

    setMessages(prev => {
      if (msg.identifier) {
        const idx = prev.findIndex(m => m.identifier === msg.identifier);
        if (idx !== -1) {
          const next = [...prev];
          next[idx] = mapped;
          return next;
        }
      }
      if (!prev.some(m => m.id === msg.id)) {
        return [...prev, mapped];
      }
      return prev;
    });

    if (msg.id > newestId.current) {
      newestId.current = msg.id;
    }

    // Update thread snippet and timestamp
    setThreads(prev => {
        let changed = false;
        const next = prev.map(t => {
            // Update the specific session thread
            if (msg.session_id && t.id === msg.session_id) {
                changed = true;
                return {
                    ...t,
                    snippet: mapped.text || t.snippet,
                    updated: mapped.time,
                    // If we receive a message from a session in real-time, it must be active
                    active: isHistory ? t.active : true 
                };
            }
            // Update main feed snippet/time
            if (t.id === 't1') {
                changed = true;
                return {
                    ...t,
                    snippet: (mapped.title ? mapped.title + ': ' : '') + (mapped.text || ''),
                    updated: mapped.time
                };
            }
            return t;
        });
        return changed ? next : prev;
    });
  };

  const fetchInitial = async () => {
    try {
      const response = await fetch('/interactions');
      if (!response.ok) throw new Error('Failed to fetch');
      const data = await response.json();
      if (data.length > 0) {
        data.forEach(msg => processMessage(msg, setMessages, setThreads, true));
      }
    } catch (error) {
      console.error('Error fetching initial messages:', error);
    }
  };

  const startStreaming = () => {
    let reconnectTimeout = null;
    
    const connect = async () => {
      try {
        const response = await fetch(`/service?timestamp=${Date.now()}`);
        if (!response.ok) throw new Error('Stream failed');
        
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          
          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop();

          for (const line of lines) {
            if (!line.trim()) continue;
            try {
              const msg = JSON.parse(line);
              processMessage(msg, setMessages, setThreads, false);
            } catch (e) {
              console.error('Error parsing stream line:', e);
            }
          }
        }
      } catch (error) {
        console.error('Stream error, reconnecting in 3s...', error);
        reconnectTimeout = setTimeout(connect, 3000);
      }
    };

    connect();
    return () => clearTimeout(reconnectTimeout);
  };

  React.useEffect(() => {
    fetchInitial().then(startStreaming);
    // fallback polling just in case stream dies or is not supported
    const interval = setInterval(async () => {
        try {
            const response = await fetch(`/interactions?after=${newestId.current}`);
            const data = await response.json();
            if (data.length > 0) {
                data.forEach(msg => processMessage(msg, setMessages, setThreads));
            }
        } catch (e) {}
    }, 5000);
    return () => clearInterval(interval);
  }, []);

  const handleSend = async (text) => {
    if (!config.interactive) return;
    
    try {
        const payload = { message: text, is_user: true };
        if (thread.sessionId) {
            payload.session_id = thread.sessionId;
        }
        const response = await fetch('/interactions', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(payload)
        });
        if (!response.ok) throw new Error('Failed to send');
        
        // Polling or stream will pick it up
    } catch (error) {
        console.error('Error sending message:', error);
    }
  };

  const handleDecide = (msgId, decision) => {
    setDecisions((d) => ({ ...d, [msgId]: decision }));
    // In a real app, this would call the backend
    if (decision === 'Approve') {
        handleSend(`/approve ${msgId}`);
    } else {
        handleSend(`/deny ${msgId}`);
    }
  };

  const renderMessage = (m) => {
    if (m.kind === 'user') return <UserBubble key={m.id} msg={m} theme={theme} />;
    if (m.kind === 'status') return <StatusNote key={m.id} msg={m} theme={theme} />;
    if (m.kind === 'tool') return (
      <ToolBlock key={m.id} msg={m} theme={theme}
        expanded={!!expandedTools[m.id]}
        onToggle={() => setExpandedTools((e) => ({ ...e, [m.id]: !e[m.id] }))} />
    );
    if (m.kind === 'approval') return (
      <ApprovalCard key={m.id} msg={m} theme={theme}
        decision={decisions[m.id]}
        onDecide={(d) => handleDecide(m.id, d)} />
    );
    return <AgentBubble key={m.id} msg={m} theme={theme} />;
  };

  const filteredMessages = messages.filter(m => {
    if (!thread || thread.id === 't1') return true; // Main feed shows everything
    return m.sessionId === thread.sessionId;
  });

  React.useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [filteredMessages.length, typing, activeId]);

  // Cmd-K binding
  React.useEffect(() => {
    const h = (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setPaletteOpen(true);
      }
    };
    window.addEventListener('keydown', h);
    return () => window.removeEventListener('keydown', h);
  }, []);

  const sidebar = (
    <Sidebar
      theme={theme} threads={threads} activeId={activeId}
      search={search} setSearch={setSearch}
      onSelect={(id) => { setActiveId(id); setDrawerOpen(false); }}
      onClose={isPhone ? () => setDrawerOpen(false) : null}
      onOpenPalette={() => { setPaletteOpen(true); setDrawerOpen(false); }}
      dark={dark} setDark={setDark}
      icon={icon}
    />
  );

  return (
    <div style={{
      width: '100%', height: '100%',
      display: 'flex', flexDirection: 'row',
      background: theme.bg, color: theme.fg,
      fontFamily: FONT_SANS,
      position: 'relative', overflow: 'hidden',
    }}>
      {/* agent rail (tablet only, hidden in solo mode) */}
      {!isPhone && !solo && (
        <AgentRail theme={theme} threads={threads} activeAgent={thread.agent}
          icon={icon}
          onSelectAgent={(aid) => {
            const t = threads.find((th) => th.agent === aid);
            if (t) setActiveId(t.id);
          }}
        />
      )}
      {/* sidebar — split on tablet, drawer on phone, hidden in solo */}
      {!isPhone && !solo && (
        <div style={{ width: 280, height: '100%', flexShrink: 0 }}>
          {sidebar}
        </div>
      )}
      {/* solo mode: a single brand mark column */}
      {!isPhone && solo && (
        <div style={{
          width: 72, flexShrink: 0, height: '100%',
          borderRight: `1px solid ${theme.border}`,
          display: 'flex', flexDirection: 'column', alignItems: 'center',
          padding: '14px 0', gap: 14,
        }}>
          <div style={{ padding: '4px 6px 0' }}>
            <BrandMark icon={icon} theme={theme} size={36} pulse />
          </div>
          <div style={{
            fontFamily: FONT_MONO, fontSize: 9, color: theme.fgDim,
            letterSpacing: 0.6, textTransform: 'uppercase', writingMode: 'vertical-rl',
            transform: 'rotate(180deg)', marginTop: 8,
          }}>solo session</div>
        </div>
      )}
      {isPhone && drawerOpen && (
        <>
          <div onClick={() => setDrawerOpen(false)} style={{
            position: 'absolute', inset: 0, background: 'rgba(0,0,0,0.5)',
            zIndex: 50,
          }} />
          <div style={{
            position: 'absolute', left: 0, top: 0, bottom: 0,
            width: 'min(86%, 320px)', zIndex: 51,
            boxShadow: '0 0 30px rgba(0,0,0,0.4)',
          }}>
            {sidebar}
          </div>
        </>
      )}

      {/* chat column */}
      <div style={{
        flex: 1, minWidth: 0, height: '100%',
        display: 'flex', flexDirection: 'column',
        background: theme.bg,
      }}>
        <ChatHeader theme={theme} thread={thread} isPhone={isPhone} solo={solo} onMenu={() => setDrawerOpen(true)} />
        <div ref={scrollRef} style={{
          flex: 1, overflowY: 'auto',
          padding: isPhone ? '16px 12px' : '20px 24px',
          display: 'flex', flexDirection: 'column', gap: 14,
        }}>
          <DateDivider theme={theme} label="Today" />
          {filteredMessages.map(renderMessage)}
          {typing && (
            <TypingBubble agent={typing} theme={theme} />
          )}
        </div>
        {config.interactive && thread.id !== 't1' && (
          <Composer
            theme={theme}
            value={composerValue}
            setValue={setComposerValue}
            onSend={handleSend}
            onOpenPalette={() => setPaletteOpen(true)}
            agentColor={agent.color}
            isWorking={!!typing}
            onStop={() => setTyping(null)}
          />
        )}
      </div>

      <CommandPalette
        theme={theme} open={paletteOpen}
        onClose={() => setPaletteOpen(false)}
        onPick={(item) => setComposerValue(item.cmd + ' ')}
      />
    </div>
  );
}

function DateDivider({ theme, label }) {
  return (
    <div style={{
      display: 'flex', justifyContent: 'center',
      padding: '4px 0',
    }}>
      <span style={{
        fontFamily: FONT_MONO, fontSize: 11, color: theme.fgDim,
      }}>{label}</span>
    </div>
  );
}

Object.assign(window, { PushChat });
