// Main PushChat container — composes sidebar + chat view, handles state.
// Adapts to phone (drawer sidebar) vs tablet (split layout) based on `mode` prop.

function PushChat({ theme, dark, setDark, mode = 'tablet', icon = APP_ICON, solo = false, config = { interactive: false } }) {
  const [activeId, setActiveId] = React.useState('t1');
  const [search, setSearch] = React.useState('');
  const [drawerOpen, setDrawerOpen] = React.useState(false);
  const [paletteOpen, setPaletteOpen] = React.useState(false);
  const [composerValue, setComposerValue] = React.useState('');
  const [expandedTools, setExpandedTools] = React.useState({});
  const [decisions, setDecisions] = React.useState({});
  const [messages, setMessages] = React.useState([]);
  const [typing, setTyping] = React.useState(null);
  const [initialLoadComplete, setInitialLoadComplete] = React.useState(false);
  const isPhone = mode === 'phone';

  const thread = THREADS.find((t) => t.id === activeId) || THREADS[0];
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
    if (msg.is_user) {
      return {
        id: msg.id,
        kind: 'user',
        time: formatTime(msg.timestamp),
        text: msg.message,
        link: msg.link
      };
    }

    // Try to detect tool or approval from title/message
    if (msg.title && (msg.title.includes('Approval') || msg.title.includes('Approve'))) {
      return {
        id: msg.id,
        kind: 'approval',
        agent: 'remote',
        time: formatTime(msg.timestamp),
        title: msg.title,
        summary: msg.message,
        risk: 'unknown',
        actions: ['Approve', 'Deny']
      };
    }

    if (msg.title && (msg.title.includes('Run') || msg.title.includes('$'))) {
      return {
        id: msg.id,
        kind: 'tool',
        agent: 'remote',
        time: formatTime(msg.timestamp),
        tool: 'shell',
        title: msg.title,
        duration: '',
        lines: msg.message.split('\n').map(l => ({ c: 'fg', t: l }))
      };
    }

    return {
      id: msg.id,
      kind: 'agent',
      agent: 'remote',
      status: 'done',
      time: formatTime(msg.timestamp),
      text: msg.detailed_message || msg.message,
      title: msg.title,
      link: msg.link
    };
  };

  const fetchMessages = async (type = 'initial') => {
    let url = '/interactions';
    if (type === 'poll') {
      if (!initialLoadComplete) return;
      url += `?after=${newestId.current}`;
    }

    try {
      const response = await fetch(url);
      if (!response.ok) throw new Error('Failed to fetch');
      const data = await response.json();

      if (data.length === 0) {
          if (type === 'initial') setInitialLoadComplete(true);
          return;
      }

      const mapped = data.map(mapMessage);
      
      if (type === 'initial') {
        setMessages(mapped);
        newestId.current = data[data.length - 1].id;
        setInitialLoadComplete(true);
      } else {
        setMessages(prev => [...prev, ...mapped]);
        newestId.current = data[data.length - 1].id;
      }
    } catch (error) {
      console.error('Error fetching messages:', error);
    }
  };

  React.useEffect(() => {
    fetchMessages('initial');
    const interval = setInterval(() => fetchMessages('poll'), 3000);
    return () => clearInterval(interval);
  }, []);

  React.useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages.length, typing]);

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

  const handleSend = async (text) => {
    if (!config.interactive) return;
    
    try {
        const response = await fetch('/interactions', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ message: text, is_user: true })
        });
        if (!response.ok) throw new Error('Failed to send');
        
        // Let the polling pick it up, or fetch immediately
        fetchMessages('poll');
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

  const sidebar = (
    <Sidebar
      theme={theme} threads={THREADS} activeId={activeId}
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
        <AgentRail theme={theme} threads={THREADS} activeAgent={thread.agent}
          icon={icon}
          onSelectAgent={(aid) => {
            const t = THREADS.find((th) => th.agent === aid);
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
          {messages.map(renderMessage)}
          {typing && (
            <TypingBubble agent={typing} theme={theme} />
          )}
        </div>
        {config.interactive && (
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
