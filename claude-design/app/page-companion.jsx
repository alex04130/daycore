// Daycore v2 — Companion chat: simulated SSE streaming + <plan_update>/<rule_update> action tags
(function () {
  'use strict';
  const { useState, useEffect, useRef } = React;
  const UI = window.DaycoreUI;
  const IC = window.DcIcons;
  const S = window.DcStore;
  const { t } = window.I18N;
  const { useStore, useToast } = window.DcUI;

  // ---------- scripted assistant (mirrors backend action-tag contract) ----------
  function scriptReply(msg, lang) {
    const zh = lang === 'zh-CN';
    const T = S.todayIso();
    // rule intent: 每 N 天 / every N days / 每天 / 每周
    let m = msg.match(/每\s*(\d+)\s*天(?:提醒我)?(.+)|every\s*(\d+)\s*days?\s*(?:remind me to)?(.+)/i);
    if (m) {
      const n = Number(m[1] || m[3]);
      const title = (m[2] || m[4] || '').replace(/[。.!！]$/, '').trim() || (zh ? '提醒' : 'Reminder');
      return {
        text: zh
          ? '好，这听起来是件要长期坚持的事，我把它记成了一条重复规则：**' + title + '**，每 ' + n + ' 天一次。以后自主规划时我都会带上它。'
          : 'Got it — that sounds like a long-term thing, so I saved it as a recurring rule: **' + title + '**, every ' + n + ' days. Auto-plan will include it from now on.',
        action: { kind: 'rule', payload: { title, kind: 'recurring', freq: 'every_n_days', interval: n, type: 'task', source: 'chat', start_date: T } },
      };
    }
    m = msg.match(/每天\s*(?:([0-2]?\d)[:点：](\d{0,2}))?\s*(?:提醒我)?(.+)|daily\s+(.+)/i);
    if (m && /每天|daily/i.test(msg)) {
      const title = (m[3] || m[4] || '').replace(/[。.!！]$/, '').trim();
      if (title) {
        const time = m[1] ? String(m[1]).padStart(2, '0') + ':' + (m[2] ? String(m[2]).padStart(2, '0') : '00') : null;
        return {
          text: zh ? '好，我把「' + title + '」记成每天的重复规则' + (time ? '（' + time + '）' : '') + '，以后都会自动出现在你的日程里。' : 'Saved “' + title + '” as a daily rule' + (time ? ' at ' + time : '') + ' — it will appear in your plan automatically.',
          action: { kind: 'rule', payload: { title, kind: 'recurring', freq: 'daily', interval: 1, time, type: 'task', source: 'chat', start_date: T } },
        };
      }
    }
    // plan tomorrow / 规划明天
    if (/明天|tomorrow/i.test(msg) && /规划|安排|计划|plan/i.test(msg)) {
      const d = S.addDays(T, 1);
      return {
        text: zh
          ? '交给我。我看了你明天的课和临期作业，把白天排给了 Problem Set 8（后天截止），晚上留了放松时间。你可以在「今日」页左右滑到明天查看，不合适随时说。'
          : 'On it. I checked tomorrow’s classes and upcoming deadlines — daytime goes to Problem Set 8 (due the day after), the evening stays light. Swipe to tomorrow on the Today page; tell me if anything feels off.',
        run: () => S.autoPlan({ from: d, to: d, mode: 'keep_manual' }),
        actionLabel: { key: 'cp_action_plan_updated', vars: { date: window.I18N.fmtDayLabel(d, T) } },
        link: 'plan',
      };
    }
    // schedule this week's assignments
    if (/(这周|本周|this week)/i.test(msg) && /(作业|assignment|排|schedule)/i.test(msg)) {
      return {
        text: zh
          ? '好，我把未来 7 天的作业都倒排进去了：越接近截止的越早开始，每天不超过 3 个专注块。Lab 5 明天截止，所以今天下午给了它完整的 90 分钟。'
          : 'Done — I backward-planned the next 7 days of assignments: closer deadlines start earlier, max 3 focus blocks a day. Lab 5 is due tomorrow, so it gets a full 90 minutes this afternoon.',
        run: () => S.autoPlan({ from: T, to: S.addDays(T, 6), mode: 'keep_manual' }),
        actionLabel: { key: 'cp_action_plan_updated', vars: { date: t('ap_range_week') } },
        link: 'plan',
      };
    }
    // add block today: 下午/晚上 X点 做Y
    m = msg.match(/(上午|下午|晚上|中午)?\s*([0-2]?\d)[:点：](\d{0,2})\s*(.+)/);
    if (m && /(加|安排|添加|add|schedule)/i.test(msg)) {
      let h = Number(m[2]); const mm = m[3] ? Number(m[3]) : 0;
      if ((/下午|晚上/.test(m[1] || '') && h < 12)) h += 12;
      const time = String(h).padStart(2, '0') + ':' + String(mm).padStart(2, '0');
      const title = m[4].replace(/[。.!！]$/, '').trim();
      return {
        text: zh ? '加好了：今天 ' + time + '「' + title + '」。我顺手检查过，和现有安排不冲突。' : 'Added: ' + time + ' today — “' + title + '”. I checked, no conflicts with the rest of your day.',
        run: () => S.patchPlan(T, { action: 'add', block: { title, time, type: 'task', duration_min: 60, origin: 'manual' } }),
        actionLabel: { key: 'cp_action_plan_updated', vars: { date: t('today') } },
        link: 'plan',
      };
    }
    // fallback empathetic
    const fall = zh ? [
      '嗯嗯，我在听。如果想让我动手改计划，直接说就行，比如「明天上午留给图书馆」。',
      '收到。今天的安排里 Lab 5 是重头，其他都可以灵活挪。需要我调整什么吗？',
      '好的。对了，你的 Essay 2 还有 6 天截止，我建议这两天开始列提纲——要我排进去吗？',
    ] : [
      'I’m listening. If you want me to change the plan, just say it — e.g. “keep tomorrow morning for the library”.',
      'Noted. Lab 5 is the heavy piece today; everything else can flex. Want me to adjust anything?',
      'Okay. By the way, Essay 2 is due in 6 days — I’d start outlining within two days. Want it on the plan?',
    ];
    return { text: fall[Math.floor(Math.random() * fall.length)] };
  }

  function CompanionPage({ nav }) {
    const st = useStore();
    const [items, setItems] = useState(() => S.state.chat.map((m) => ({ type: 'msg', ...m })));
    const [input, setInput] = useState('');
    const [streaming, setStreaming] = useState(false);
    const scrollRef = useRef(null);
    const toast = useToast();
    const lang = window.I18N.lang;
    const name = st.session.assistantName;

    useEffect(() => {
      const el = scrollRef.current;
      if (el) el.scrollTop = el.scrollHeight;
    }, [items, streaming]);

    // persist history (text messages only)
    function persist(next) {
      S.state.chat = next.filter((x) => x.type === 'msg').map(({ role, content, timestamp }) => ({ role, content, timestamp }));
      S.emit();
    }

    function send(text) {
      const msg = (text || input).trim();
      if (!msg || streaming) return;
      setInput('');
      const now = new Date().toISOString();
      const withUser = [...items, { type: 'msg', role: 'user', content: msg, timestamp: now }];
      setItems(withUser);
      setStreaming(true);
      const reply = scriptReply(msg, lang);
      // simulate SSE: stream chunks
      const full = reply.text;
      let i = 0;
      const asstIdx = withUser.length;
      const stepMs = 26;
      setItems([...withUser, { type: 'msg', role: 'assistant', content: '', timestamp: now, streaming: true }]);
      const iv = setInterval(() => {
        i = Math.min(full.length, i + 2 + Math.floor(Math.random() * 3));
        setItems((cur) => cur.map((x, j) => (j === asstIdx ? { ...x, content: full.slice(0, i) } : x)));
        if (i >= full.length) {
          clearInterval(iv);
          setItems((cur) => {
            let next = cur.map((x, j) => (j === asstIdx ? { ...x, streaming: false } : x));
            if (reply.action) {
              const r = S.createRule(reply.action.payload);
              next = [...next, { type: 'action', label: t('cp_action_rule_added', { title: r.title }), link: 'rules', ruleId: r.id }];
            }
            if (reply.run) {
              reply.run();
              next = [...next, { type: 'action', label: t(reply.actionLabel.key, reply.actionLabel.vars), link: reply.link }];
            }
            persist(next);
            return next;
          });
          setStreaming(false);
        }
      }, stepMs);
    }

    const hints = ['cp_hint_1', 'cp_hint_2', 'cp_hint_3'];
    return (
      <div className="dc-chat-page dc-page-enter" data-screen-label="陪伴聊天">
        <div className="dc-chat-scroll" ref={scrollRef}>
          {!items.length ? (
            <UI.EmptyState icon={<IC.MessageHeart size={30} />} title={name} description={t('cp_status')} />
          ) : null}
          {items.map((x, i) => x.type === 'action' ? (
            <div key={i} className="dc-chat-action-card">
              <IC.CheckCircle size={16} />
              <span>{x.label}</span>
              <button className="lnk" onClick={() => (x.link === 'rules' ? nav.go('materials', { rules: true }) : nav.go('today'))}>
                {x.link === 'rules' ? t('cp_view_rules') : t('cp_view_plan')}
              </button>
            </div>
          ) : (
            <UI.ChatBubble key={i} role={x.role}>
              {x.content}{x.streaming ? <UI.TypingDots /> : null}
            </UI.ChatBubble>
          ))}
        </div>
        {!items.length ? (
          <div className="dc-chat-hints">
            {hints.map((h) => <UI.Chip key={h} variant={(false) ? 'selected' : 'default'} onClick={() => send(t(h))}>{t(h)}</UI.Chip>)}
          </div>
        ) : null}
        <div className="dc-composer">
          <textarea rows={1} placeholder={t('cp_placeholder')} value={input}
            onChange={(e) => { setInput(e.target.value); e.target.style.height = 'auto'; e.target.style.height = Math.min(120, e.target.scrollHeight) + 'px'; }}
            onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); } }} />
          <button className="dc-send-btn" disabled={!input.trim() || streaming} aria-label={t('cp_placeholder')} onClick={() => send()}>
            <IC.ArrowUp size={20} />
          </button>
        </div>
      </div>
    );
  }

  window.DcPages = Object.assign(window.DcPages || {}, { CompanionPage });
})();
