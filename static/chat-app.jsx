// Main PushChat container — composes sidebar + chat view, handles state.
// Adapts to phone (drawer sidebar) vs tablet (split layout) based on `mode` prop.

function PushChat({ theme, dark, setDark, mode = 'tablet', icon = APP_ICON, solo = false, config = { interactive: false } }) {
  const [threads, setThreads] = React.useState(THREADS);
  const [activeId, setActiveId] = React.useState(() => {
    return localStorage.getItem('push_active_id') || 't1';
  });
  const [drawerOpen, setDrawerOpen] = React.useState(false);
  const [paletteOpen, setPaletteOpen] = React.useState(false);
  const [composerValue, setComposerValue] = React.useState('');
  const [expandedTools, setExpandedTools] = React.useState({});
  const [decisions, setDecisions] = React.useState({});
  const [messages, setMessages] = React.useState([]);
  const [toast, setToast] = React.useState(null);
  const isPhone = mode === 'phone';

  const showToast = (message) => {
    setToast(message);
    setTimeout(() => setToast(null), 2000);
  };

  const thread = threads.find((t) => t.id === activeId) || threads[0];
  const agent = AGENTS[thread.agent];

  // Typing indicator derived from thread status
  const isTyping = thread.id !== 't1' && thread.status !== 'ready' && thread.status !== 'idle';
  const typingAgent = isTyping ? thread.agent : null;

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
    const sid = msg.session_id ? String(msg.session_id).trim() : '';
    const base = {
      id: msg.id,
      identifier: msg.identifier,
      update: msg.update,
      time: formatTime(msg.timestamp),
      text: msg.detailed_message || msg.message,
      link: msg.link,
      title: msg.title,
      sessionId: sid,
      sessionPath: msg.session_path,
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

    if (msg.title === 'session-register' || msg.agent === 'tmux') {
      return {
        ...base,
        kind: 'status',
        agent: agentId,
        status: status || 'ready',
      };
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
    const sid = mapped.sessionId;
    const msgTs = msg.timestamp ? (typeof msg.timestamp === 'string' ? new Date(msg.timestamp).getTime() : msg.timestamp) : Date.now();
    
    // Handle heartbeats separately as they don't add to the message list
    if (msg.title === 'heartbeat' && msg.message !== undefined) {
        const activeIds = (msg.message ? msg.message.split(',') : []).map(id => id.trim()).filter(id => id && id !== 't1');
        setThreads(prev => {
            let next = [...prev];
            let changed = false;

            activeIds.forEach(id => {
                const idx = next.findIndex(t => t.id === id);
                if (idx === -1) {
                    changed = true;
                    setTimeout(() => fetchThreadInfo(id), 0);
                    next.push({
                        id: id,
                        agent: 'remote',
                        title: 'CLI Agent',
                        status: 'ready',
                        snippet: 'Active session',
                        updated: mapped.time,
                        lastTimestamp: Date.now(),
                        unread: 0,
                        pinned: false,
                        sessionId: id,
                        active: true,
                        placeholder: true,
                        lastMsgId: 0
                    });
                } else if (!next[idx].active) {
                    changed = true;
                    next[idx] = { ...next[idx], active: true, lastTimestamp: Date.now() };
                }
            });

            // Mark others as inactive
            next = next.map(t => {
                if (t.id !== 't1' && t.active && !activeIds.includes(t.id)) {
                    changed = true;
                    return { ...t, active: false, lastTimestamp: Date.now() };
                }
                return t;
            });

            return changed ? next : prev;
        });
        return;
    }

    if (msg.id !== 0) {
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
    }

    // Update threads
    setThreads(prev => {
        let changed = false;
        const next = [...prev];

        // 1. Update/Create session-specific thread
        if (sid && sid !== 't1' && sid !== 'undefined' && sid !== 'null') {
            const idx = next.findIndex(t => t.id === sid);
            const agent = mapped.agent || 'remote';
            const sysTitles = ['session-register', 'session-active', 'session-inactive', 'heartbeat', 'tmux-service'];
            
            let title = '';
            if (msg.title === 'session-register') {
                title = msg.message.replace('Registered session: ', '') || 'CLI Agent';
            } else if (msg.title && !sysTitles.includes(msg.title)) {
                title = msg.title.replace(/ - (Done|Working|Awaiting)$/, '');
            }

            if (idx !== -1) {
                const t = next[idx];
                if (msg.id >= t.lastMsgId) {
                    changed = true;
                    let nextActive = t.active;
                    if (msg.title === 'session-active') nextActive = true;
                    else if (msg.title === 'session-inactive') nextActive = false;
                    else if (!isHistory) nextActive = true;

                    next[idx] = {
                        ...t,
                        title: (title || t.title).trim(),
                        agent: agent !== 'remote' ? agent : t.agent,
                        snippet: mapped.text || t.snippet,
                        updated: mapped.time,
                        lastTimestamp: isNaN(msgTs) ? t.lastTimestamp : msgTs,
                        sessionPath: msg.session_path || t.sessionPath,
                        status: msg.is_user ? 'working' : (mapped.status === 'done' ? 'working' : (mapped.status || t.status)),
                        active: nextActive,
                        lastMsgId: msg.id > 0 ? msg.id : t.lastMsgId,
                        placeholder: false
                    };
                }
            } else {
                changed = true;
                next.push({
                    id: sid,
                    agent,
                    title: (title || 'CLI Agent').trim(),
                    status: mapped.status === 'done' ? 'working' : (mapped.status || 'ready'),
                    snippet: mapped.text || 'Active session',
                    updated: mapped.time,
                    lastTimestamp: isNaN(msgTs) ? Date.now() : msgTs,
                    sessionPath: msg.session_path || '',
                    unread: 0,
                    pinned: false,
                    sessionId: sid,
                    active: msg.title === 'session-active' || (!isHistory && msg.title !== 'session-inactive'),
                    lastMsgId: msg.id > 0 ? msg.id : 0,
                    placeholder: false
                });
            }
        }

        // 2. Update main feed thread
        const t1Idx = next.findIndex(t => t.id === 't1');
        if (t1Idx !== -1 && msg.id > 0) {
            const t1 = next[t1Idx];
            if (msg.id >= t1.lastMsgId) {
                changed = true;
                next[t1Idx] = {
                    ...t1,
                    snippet: (mapped.title ? mapped.title + ': ' : '') + (mapped.text || ''),
                    updated: mapped.time,
                    lastTimestamp: isNaN(msgTs) ? t1.lastTimestamp : msgTs,
                    lastMsgId: msg.id
                };
            }
        }

        return changed ? next : prev;
    });
  };

  const fetchThreadInfo = async (sessionId) => {
    try {
        const response = await fetch(`/interactions?session_id=${sessionId}&limit=20`);
        if (!response.ok) return;
        const data = await response.json();
        if (data.length > 0) {
            // Process in order so newest metadata wins
            data.forEach(msg => processMessage(msg, setMessages, setThreads, true));
        }
    } catch (e) {
        console.error('Error fetching thread info:', e);
    }
  };

  const fetchInitial = async () => {
    try {
      // 1. Fetch latest interaction for every session to populate sidebar accurately
      const sessionResponse = await fetch('/interactions?latest_per_session=true');
      if (sessionResponse.ok) {
        const sessionData = await sessionResponse.json();
        sessionData.forEach(msg => processMessage(msg, setMessages, setThreads, true));
      }

      // 2. Fetch recent interactions for the main feed
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
        const url = newestId.current > 0 
            ? `/service?after=${newestId.current}`
            : `/service?timestamp=${Date.now()}`;
        const response = await fetch(url);
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
    if (m.kind === 'user') return <UserBubble key={m.id} msg={m} theme={theme} onCopy={() => showToast('Copied to clipboard')} />;
    if (m.kind === 'status') return <StatusNote key={m.id} msg={m} theme={theme} onCopy={() => showToast('Copied to clipboard')} />;
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
    return <AgentBubble key={m.id} msg={m} theme={theme} onCopy={() => showToast('Copied to clipboard')} />;
  };

  const filteredMessages = React.useMemo(() => messages.filter(m => {
    if (!thread || thread.id === 't1') return true; // Main feed shows everything
    return m.sessionId === thread.sessionId;
  }), [messages, thread]);

  React.useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [filteredMessages, isTyping, activeId]);

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
      onSelect={(id) => { 
        setActiveId(id); 
        localStorage.setItem('push_active_id', id);
        setDrawerOpen(false); 
      }}
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
          {isTyping && (
            <TypingBubble agent={typingAgent} theme={theme} />
          )}
        </div>
        {config.interactive && thread.id !== 't1' && thread.active && (
          <Composer
            theme={theme}
            value={composerValue}
            setValue={setComposerValue}
            onSend={handleSend}
            onOpenPalette={() => setPaletteOpen(true)}
            agentColor={agent.color}
            isWorking={isTyping}
          />
        )}
      </div>

      <CommandPalette
        theme={theme} open={paletteOpen}
        onClose={() => setPaletteOpen(false)}
        onPick={(item) => setComposerValue(item.cmd + ' ')}
      />

      {toast && (
        <Toast message={toast} theme={theme} />
      )}
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

function Toast({ message, theme }) {
  return (
    <div style={{
      position: 'absolute', bottom: 100, left: '50%', transform: 'translateX(-50%)',
      background: theme.fg, color: theme.bg,
      padding: '8px 16px', borderRadius: 20,
      fontSize: 12, fontWeight: 500, fontFamily: FONT_SANS,
      zIndex: 1000, boxShadow: '0 4px 12px rgba(0,0,0,0.2)',
      animation: 'pushToast 0.2s ease-out',
    }}>
      {message}
    </div>
  );
}

Object.assign(window, { PushChat });
