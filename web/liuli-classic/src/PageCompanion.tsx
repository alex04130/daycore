import { useCallback, useEffect, useRef, useState } from 'react';
import * as api from '@daycore/core';
import type { Boot, ChatMessage, ChatThread } from '@daycore/core';

// 陪伴：一个可以走开的对话。
//
// ⚠️ The async endpoint, not the streaming one, and that is a paradigm decision
// rather than a technical preference. A five-page app's whole premise is that
// you can leave a page — so a turn that dies when this tab loses focus is a
// turn the reader has to sit and watch, which is exactly the thing the other
// three frontends do not have to worry about because they never navigate away.
//
// POST /api/ai/companion/async answers 202 with a message id; the reply is
// written into the thread by a background turn and this polls for it.

const POLL_MS = 2500;
/** ⚠️ A ceiling, so a turn that dies leaves a stopped spinner rather than a
 *  page that polls a dead id until the tab closes. */
const POLL_MAX = 120;

export function PageCompanion({ boot }: { boot: Boot }) {
  const t = boot.catalog.t;
  const [threads, setThreads] = useState<ChatThread[]>([]);
  const [current, setCurrent] = useState<string | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [draft, setDraft] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const stop = useRef(false);

  useEffect(() => () => { stop.current = true; }, []);

  const loadThreads = useCallback(async () => {
    try {
      const { threads: list } = await api.threads();
      setThreads(list ?? []);
      setCurrent((c) => c ?? list?.[0]?.id ?? null);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  const loadMessages = useCallback(async (id: string) => {
    try {
      const { messages: list } = await api.threadMessages(id);
      // ⚠️ The endpoint answers newest-first (it is a cursor over a descending
      // index). Rendering it as-is puts the conversation in reverse and reads as
      // the assistant answering before you asked.
      setMessages([...(list ?? [])].reverse());
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    void loadThreads();
  }, [loadThreads]);

  useEffect(() => {
    if (current) void loadMessages(current);
  }, [current, loadMessages]);

  async function send() {
    const text = draft.trim();
    if (!text) return;
    let threadId = current;
    setBusy(true);
    setError('');
    try {
      if (!threadId) {
        const th = await api.createThread();
        threadId = th.id;
        setCurrent(th.id);
        await loadThreads();
      }
      setDraft('');
      const { messageId } = await api.askCompanion(threadId, text);
      await loadMessages(threadId);
      for (let i = 0; i < POLL_MAX && !stop.current; i++) {
        await new Promise((r) => setTimeout(r, POLL_MS));
        const m = await api.chatMessage(messageId);
        // ⚠️ An empty status means done. Old rows predate the column, and
        // treating "" as pending makes a thread that can never be posted to
        // again — the composer stays locked forever with no way back.
        if (m.status !== 'pending') {
          await loadMessages(threadId);
          if (m.status === 'error') setError(t('companion.failed'));
          break;
        }
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="lc-page">
      <header className="lc-head">
        <h1 className="lc-title">{t('nav.companion')}</h1>
        <button className="lc-btn sec" onClick={() => void api.createThread().then(loadThreads)}>
          {t('companion.newThread')}
        </button>
      </header>

      {threads.length > 1 && (
        <div className="lc-seg">
          {threads.map((th) => (
            <button
              key={th.id}
              className={'lc-segitem' + (current === th.id ? ' on' : '')}
              onClick={() => setCurrent(th.id)}
            >
              {/* ⚠️ No 「{n} 条消息」 subtitle. ChatThread carries no message
                  count, and getting one would mean a request per thread on every
                  render of this list. A relative time is what the shape can
                  honestly support. */}
              {th.title || t('companion.untitled')}
            </button>
          ))}
        </div>
      )}

      {error && <p className="lc-err">{error}</p>}

      {messages.length === 0 ? (
        <p className="lc-empty">{t('companion.empty')}</p>
      ) : (
        <ul className="lc-list">
          {messages.map((m) => (
            <li key={m.id}>
              <article className={'lc-bubble ' + (m.role === 'user' ? 'me' : 'them')}>
                {m.status === 'pending' ? <span className="lc-sub">{t('companion.thinking')}</span> : m.content}
              </article>
            </li>
          ))}
        </ul>
      )}

      <form
        className="lc-form"
        onSubmit={(e) => {
          e.preventDefault();
          void send();
        }}
      >
        <input
          className="lc-input"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder={t('companion.placeholder')}
          aria-label={t('companion.placeholder')}
          disabled={busy}
        />
        <button className="lc-btn pri" type="submit" disabled={busy || !draft.trim()}>
          {t('companion.send')}
        </button>
      </form>
    </div>
  );
}
