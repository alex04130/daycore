// Daycore v2.2 — Companion: chat threads + SSE v2 frames (delta / tool cards / decision cards) + async pending turns
(function () {
  'use strict';
  const { useState, useEffect, useRef } = React;
  const UI = window.DaycoreUI;
  const IC = window.DcIcons;
  const S = window.DcStore;
  const { t } = window.I18N;
  const { useStore, Sheet, Confirm, useToast } = window.DcUI;

  const TOOL_ICON = {
    get_weather: IC.Sun, web_search: IC.Search, list_upcoming: IC.ListChecks,
    plan_add: IC.CalendarPlus, plan_update: IC.CalendarDays, plan_remove: IC.Trash,
    rule_upsert: IC.Repeat, rule_remove: IC.Repeat, memory_add: IC.Brain, memory_remove: IC.Brain,
  };
  const toolName = (tool) => t('tool_' + tool);
  const toolLink = (tool) => {
    if (tool === 'rule_upsert' || tool === 'rule_remove') return 'rules';
    if (tool === 'memory_add' || tool === 'memory_remove') return 'memory';
    if (tool && tool.indexOf('plan') === 0) return 'plan';
    return null;
  };

  // ---------- tool action card (tool_start → tool_result) ----------
  function ToolCard({ ev, nav }) {
    const st = useStore();
    const toast = useToast();
    const Icon = TOOL_ICON[ev.tool] || IC.Sparkles;
    if (ev.type === 'tool_start') {
      return (
        <div className="dc-toolcard is-running">
          <i className="dc-spin" />
          <span>{t('tool_running', { name: toolName(ev.tool) })}</span>
        </div>
      );
    }
    const op = ev.opId ? st.ops.find((o) => o.id === ev.opId) : null;
    const undone = op && op.reverted;
    if (!ev.ok) {
      return <div className="dc-toolcard is-fail"><IC.X size={15} /><span>{toolName(ev.tool)} · {t('tool_fail')}</span></div>;
    }
    const link = toolLink(ev.tool);
    return (
      <div className={'dc-toolcard' + (undone ? ' is-undone' : ' is-ok')}>
        <Icon size={15} />
        <span className="tc-sum">{ev.summary || toolName(ev.tool)}</span>
        {undone ? <span className="tc-undone">{t('tool_undone')}</span> : (
          <React.Fragment>
            {link ? (
              <button className="lnk" onClick={() => (link === 'rules' ? nav.go('materials', { rules: true }) : link === 'memory' ? nav.go('settings') : nav.go('today'))}>
                {link === 'rules' ? t('cp_view_rules') : link === 'memory' ? t('cp_view_memory') : t('cp_view_plan')}
              </button>
            ) : null}
            {ev.opId ? (
              <button className="lnk tc-undo" onClick={() => { const r = S.revertOp(ev.opId); if (r.ok) toast(t('tool_undone_toast')); }}>
                <IC.Undo2 size={13} /> {t('undo')}
              </button>
            ) : null}
          </React.Fragment>
        )}
      </div>
    );
  }

  // ---------- decision card ----------
  function DecisionCard({ ev }) {
    const [text, setText] = useState('');
    const pending = ev.state === 'pending';
    return (
      <div className={'dc-decision' + (pending ? '' : ' is-done')}>
        <div className="dc-dec-head">
          <span className="dc-dec-ic"><IC.MessageCircle size={15} /></span>
          <span className="dc-dec-title">{ev.title}</span>
        </div>
        <p className="dc-dec-sum">{ev.summary}</p>
        {pending ? (
          <React.Fragment>
            <div className="dc-dec-opts">
              {ev.options.map((o) => (
                <button key={o.id} className="dc-dec-opt" onClick={() => S.respondDecision(ev.id, o.id)}>{o.label}</button>
              ))}
            </div>
            <div className="dc-dec-custom">
              <input placeholder={t('dc_custom_ph')} value={text} onChange={(e) => setText(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter' && text.trim()) { S.respondDecision(ev.id, text.trim()); } }} />
              <button disabled={!text.trim()} aria-label={t('ok')} onClick={() => text.trim() && S.respondDecision(ev.id, text.trim())}><IC.ArrowUp size={15} /></button>
            </div>
            <p className="dc-dec-hint">{t('dc_timeout_hint')}</p>
          </React.Fragment>
        ) : (
          <div className="dc-dec-answered">
            <IC.CheckCircle size={14} />
            <span>{ev.state === 'timeout' ? t('dc_timeout') : t('dc_answered', { label: (ev.options.find((o) => o.id === ev.choice) || { label: ev.choice }).label })}</span>
          </div>
        )}
      </div>
    );
  }

  // ---------- thread switcher ----------
  function ThreadSheet({ open, onClose, activeId, onPick }) {
    const st = useStore();
    const toast = useToast();
    const [confirmDel, setConfirmDel] = useState(null);
    const threads = S.listThreads();
    return (
      <React.Fragment>
        <Sheet open={open} onClose={onClose} title={t('th_switch')}>
          <div className="dc-stack-8">
            <UI.Button variant="secondary" fullWidth onClick={() => { const th = S.createThread(); onPick(th.id); toast(t('th_new_created')); onClose(); }}>
              <IC.Plus size={16} /> {t('th_new')}
            </UI.Button>
            {threads.map((th) => {
              const n = (st.messages[th.id] || []).length;
              return (
                <UI.GlassCard key={th.id} padding="none" className={'dc-thread-row' + (th.id === activeId ? ' is-active' : '')}>
                  <button className="dc-thread-main" onClick={() => { onPick(th.id); onClose(); }}>
                    <span className="dc-thread-title">{th.title}</span>
                    <span className="dc-thread-sub">{t('th_n_msgs', { n })} · {window.I18N.fmtDayLabel((th.updatedAt || th.createdAt).slice(0, 10), S.todayIso())}</span>
                  </button>
                  {th.id === activeId ? <IC.Check size={16} style={{ color: 'var(--color-primary)', flexShrink: 0 }} /> : null}
                  {threads.length > 1 ? (
                    <button className="dc-asg-icon-btn" aria-label={t('del')} onClick={() => setConfirmDel(th)}><IC.Trash size={15} /></button>
                  ) : null}
                </UI.GlassCard>
              );
            })}
          </div>
        </Sheet>
        <Confirm open={!!confirmDel} onClose={() => setConfirmDel(null)} title={t('th_delete_confirm_title')} body={t('th_delete_confirm')} danger confirmLabel={t('del')}
          onConfirm={() => { const wasActive = confirmDel.id === activeId; S.deleteThread(confirmDel.id); if (wasActive) { const next = S.listThreads()[0]; onPick(next ? next.id : S.createThread().id); } }} />
      </React.Fragment>
    );
  }

  // ---------- page ----------
  function CompanionPage({ nav }) {
    const st = useStore();
    const [threadId, setThreadId] = useState(() => { const ths = S.listThreads(); return ths.length ? ths[0].id : S.createThread().id; });
    const [input, setInput] = useState('');
    const [thOpen, setThOpen] = useState(false);
    const scrollRef = useRef(null);
    const name = st.session.assistantName;

    const thread = st.threads.find((x) => x.id === threadId) || st.threads[0];
    const msgs = thread ? (st.messages[thread.id] || []) : [];
    useEffect(() => { if (!thread && st.threads.length) setThreadId(st.threads[0].id); }, [thread, st.threads.length]);

    const last = msgs[msgs.length - 1];
    const busy = !!(last && last.role === 'assistant' && last.status === 'pending');
    const hasPendingDecision = msgs.some((m) => m.toolEvents.some((e) => e.type === 'decision_card' && e.state === 'pending'));

    useEffect(() => {
      const el = scrollRef.current;
      if (el) el.scrollTop = el.scrollHeight;
    }, [msgs.length, last && last.content, last && last.toolEvents.length, busy]);

    function send(text) {
      const v = (text || input).trim();
      if (!v || !thread || busy || hasPendingDecision) return;
      setInput('');
      S.sendAsync(thread.id, v);
    }

    const hints = ['cp_hint_1', 'cp_hint_2', 'cp_hint_3', 'cp_hint_conflict', 'cp_hint_weather'];
    return (
      <div className="dc-chat-page dc-page-enter" data-screen-label="陪伴聊天">
        <div className="dc-thread-bar">
          <button className="dc-thread-pill" onClick={() => setThOpen(true)}>
            <IC.MessagesSquare size={15} />
            <span>{thread ? thread.title : ''}</span>
            <IC.ChevronDown size={14} />
          </button>
          <span className="dc-thread-async">{t('cp_async_note')}</span>
        </div>

        <div className="dc-chat-scroll" ref={scrollRef}>
          {!msgs.length ? (
            <UI.EmptyState icon={<IC.MessageHeart size={30} />} title={name} description={t('cp_status')} />
          ) : null}
          {msgs.map((m) => (
            <React.Fragment key={m.id}>
              {m.toolEvents.map((ev, i) => (
                ev.type === 'decision_card'
                  ? <DecisionCard key={ev.id || i} ev={ev} />
                  : <ToolCard key={(ev.callId || i) + ev.type} ev={ev} nav={nav} />
              ))}
              {m.role === 'user' ? (
                <UI.ChatBubble role="user">{m.content}</UI.ChatBubble>
              ) : m.content || (m.status === 'pending' && !m.toolEvents.length) ? (
                <UI.ChatBubble role="assistant" className={m.status === 'error' ? 'dc-bubble-error' : ''}>
                  {m.content}{m.status === 'pending' ? <UI.TypingDots /> : null}
                </UI.ChatBubble>
              ) : null}
              {m.role === 'assistant' && m.status === 'pending' ? (
                <div className="dc-pending-chip"><i className="dc-spin" />{t('cp_pending_chip')}</div>
              ) : null}
            </React.Fragment>
          ))}
        </div>

        {!msgs.length ? (
          <div className="dc-chat-hints">
            {hints.map((h) => <UI.Chip key={h} variant="default" onClick={() => send(t(h))}>{t(h)}</UI.Chip>)}
          </div>
        ) : null}

        <div className="dc-composer">
          <textarea rows={1} placeholder={hasPendingDecision ? t('cp_decision_wait') : t('cp_placeholder')} value={input}
            disabled={hasPendingDecision}
            onChange={(e) => { setInput(e.target.value); e.target.style.height = 'auto'; e.target.style.height = Math.min(120, e.target.scrollHeight) + 'px'; }}
            onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); } }} />
          <button className="dc-send-btn" disabled={!input.trim() || busy || hasPendingDecision} aria-label={t('cp_placeholder')} onClick={() => send()}>
            <IC.ArrowUp size={20} />
          </button>
        </div>

        <ThreadSheet open={thOpen} onClose={() => setThOpen(false)} activeId={threadId} onPick={setThreadId} />
      </div>
    );
  }

  window.DcPages = Object.assign(window.DcPages || {}, { CompanionPage });
})();
