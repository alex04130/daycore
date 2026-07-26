// Ported from app/page-companion.jsx: the scripted reply simulator is replaced
// by the SSE v2 agent stream (POST /api/ai/companion). Frames arrive as typed
// events — text/reasoning deltas, tool_start/tool_result action cards (with
// op revert), and non-modal decision cards answered via /api/decisions.
import React, { useState, useEffect, useRef } from 'react';
import UI from '../boot/ds.js';
import IC from '../icons.js';
import S from '../store.js';
import { t } from '../i18n.js';
import * as api from '../api.js';
import { useStore, useToast } from '../ui.jsx';
import ChatMarkdown from '../components/ChatMarkdown.jsx';

// Tools with a dedicated action-card label; anything else falls back to the
// generic running/summary text so new server tools degrade gracefully.
const LABELED_TOOLS = new Set([
  'plan_add', 'plan_update', 'plan_remove', 'rule_upsert', 'rule_remove',
  'memory_add', 'memory_remove', 'get_weather', 'web_search', 'list_upcoming',
]);
const toolLabel = (tool) => (LABELED_TOOLS.has(tool) ? t('cp_tool_' + tool) : null);

// DecisionCard floats above the composer while the agent waits on
// /api/decisions/{id}/respond; ignoring it just closes (the server times out).
function DecisionCard({ card, onClose }) {
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  async function respond(payload) {
    if (busy) return;
    setBusy(true);
    try {
      await api.post('/api/decisions/' + card.id + '/respond', payload);
    } catch (e) {
      if (e.code !== 'decision_not_found') toast(t('err_generic'));
    }
    onClose();
  }
  return (
    <UI.GlassCard className="dc-decision-card">
      <div className="dc-decision-head">
        <strong>{card.title}</strong>
        <button className="lnk" onClick={onClose}>{t('cp_decision_ignore')}</button>
      </div>
      {card.summary ? <p className="dc-decision-summary">{card.summary}</p> : null}
      <div className="dc-decision-opts">
        {(card.options || []).map((o) => (
          <UI.Chip key={o.id} variant="default" onClick={() => respond({ choice: o.id })}>{o.label}</UI.Chip>
        ))}
      </div>
      <input className="dc-decision-input" placeholder={t('cp_decision_placeholder')} value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => { if (e.key === 'Enter' && text.trim()) { e.preventDefault(); respond({ text: text.trim() }); } }} />
    </UI.GlassCard>
  );
}

export default function CompanionPage({ nav }) {
  const st = useStore();
  const [items, setItems] = useState(() => S.state.chat.map((m) => ({ type: 'msg', ...m })));
  const [input, setInput] = useState('');
  const [streaming, setStreaming] = useState(false);
  const [decisionCard, setDecisionCard] = useState(null);
  const scrollRef = useRef(null);
  const toast = useToast();
  const name = st.session.assistantName;

  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [items, streaming, decisionCard]);

  useEffect(() => () => S.abortChat(), []);

  function persist(next) {
    S.persistChat(next.filter((x) => x.type === 'msg' && !x.streaming).map(({ role, content, timestamp }) => ({ role, content, timestamp })));
  }

  async function undoOp(card, idx) {
    try {
      await api.post('/api/ops/' + card.opId + '/revert');
      setItems((cur) => cur.map((y, j) => (j === idx ? { ...y, label: t('cp_action_undone'), opId: null } : y)));
      const tool = card.tool || '';
      if (tool.startsWith('plan_')) S.invalidatePlans();
      else if (tool.startsWith('rule_')) S.reloadRules();
      else if (tool.startsWith('memory_')) S.reloadMemory();
    } catch (e) {
      if (e.code === 'already_reverted') {
        setItems((cur) => cur.map((y, j) => (j === idx ? { ...y, label: t('cp_action_undone'), opId: null } : y)));
      } else {
        toast(t('err_generic'));
      }
    }
  }

  async function send(text) {
    const msg = (text || input).trim();
    if (!msg || streaming) return;
    setInput('');
    setDecisionCard(null);
    const now = new Date().toISOString();
    const history = items.filter((x) => x.type === 'msg').map(({ role, content }) => ({ role, content }));
    const withUser = [...items, { type: 'msg', role: 'user', content: msg, timestamp: now }];
    setStreaming(true);
    // Stream state lives in locals; buildItems() rebuilds the list from them
    // so the final result never depends on interleaved setState timing.
    let content = '';
    let reasoning = '';
    let failed = false;
    const cards = [];
    const buildItems = (live) => [
      ...withUser,
      { type: 'msg', role: 'assistant', content, reasoning, timestamp: now, streaming: live, failed },
      ...cards.map((c) => ({ ...c })),
    ];
    const render = (live) => setItems(buildItems(live));
    render(true);
    try {
      await S.streamChat(msg, [...history, { role: 'user', content: msg }], {
        delta(txt) { content += txt; render(true); },
        reasoning(txt) { reasoning += txt; render(true); },
        toolStart(f) {
          cards.push({ type: 'action', pending: true, callId: f.callId, tool: f.tool, label: toolLabel(f.tool) || t('cp_tool_running') });
          render(true);
        },
        toolResult(f) {
          if (f.tool === 'propose_decision') setDecisionCard(null);
          const card = cards.find((c) => c.callId === f.callId);
          if (!card) return;
          card.pending = false;
          if (f.ok) {
            const base = toolLabel(f.tool);
            card.label = base ? (f.summary ? base + ' · ' + f.summary : base) : (f.summary || f.tool);
            if (f.opId) card.opId = f.opId;
          } else {
            card.failed = true;
            card.label = t('cp_action_failed') + (f.error ? ' · ' + f.error : '');
          }
          render(true);
        },
        decision(f) { setDecisionCard(f); },
      });
      render(false);
      setItems((cur) => { persist(cur); return cur; });
    } catch (e) {
      if (e && e.name === 'AbortError') return; // page navigated away — stay silent
      const code = e.code || 'server_error';
      const errMsg = code === 'rate_limited' ? t('err_rate_limited') : t('err_generic');
      content = errMsg;
      render(false);
      setItems((cur) => cur.map((x) => (x.type === 'msg' && x.role === 'assistant' && x.content === errMsg ? { ...x, failed: true } : x)));
      toast(errMsg);
    } finally {
      setStreaming(false);
    }
  }

  const hints = ['cp_hint_1', 'cp_hint_2', 'cp_hint_3'];
  return (
    <div className="dc-chat-page dc-page-enter" data-screen-label="陪伴聊天">
      <div className="dc-chat-scroll" ref={scrollRef}>
        {!items.length ? (
          <UI.EmptyState icon={<IC.MessageHeart size={30} />} title={name} description={t('cp_status')} />
        ) : null}
        {items.map((x, i) => x.type === 'action' ? (
          <div key={i} className={'dc-chat-action-card' + (x.pending ? ' is-pending' : '') + (x.failed ? ' is-fail' : '')}>
            {x.pending ? <IC.RefreshCw size={16} /> : x.failed ? <IC.X size={16} /> : <IC.CheckCircle size={16} />}
            <span>{x.label}</span>
            {!x.pending && !x.failed && x.opId ? (
              <button className="lnk" onClick={() => undoOp(x, i)}>{t('undo')}</button>
            ) : null}
          </div>
        ) : (
          <UI.ChatBubble key={i} role={x.role}>
            {x.role === 'assistant' && x.reasoning ? (
              <details className="dc-reasoning">
                <summary>{t('cp_reasoning')}</summary>
                <div className="dc-reasoning-body">{x.reasoning}</div>
              </details>
            ) : null}
            {x.role === 'assistant' ? <ChatMarkdown>{x.content}</ChatMarkdown> : x.content}
            {x.streaming ? <UI.TypingDots /> : null}
          </UI.ChatBubble>
        ))}
      </div>
      {!items.length ? (
        <div className="dc-chat-hints">
          {hints.map((h) => <UI.Chip key={h} variant="default" onClick={() => send(t(h))}>{t(h)}</UI.Chip>)}
        </div>
      ) : null}
      {decisionCard ? <DecisionCard card={decisionCard} onClose={() => setDecisionCard(null)} /> : null}
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
