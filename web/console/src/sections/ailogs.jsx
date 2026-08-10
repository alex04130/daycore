import { useCallback, useState } from 'react';
import * as api from '../api.js';
import { Empty, Notice, Screen, useSection } from '../ui.jsx';
import { shortTime } from './overview.jsx';

// The AI call ledger.
//
// # Paging is "load more", not page numbers
//
// The cursor comes back from the server (`nextBeforeAt` / `nextBeforeId`) and
// this screen hands it straight back. It never computes one, and it never
// computes an offset — the ledger grows at the head, so an offset would repeat
// and skip rows, which on a log reads as the log lying.
//
// The consequence is that there are no page numbers and no jump-to-end. That is
// the honest shape for a stream: "how many pages of calls are there" is not a
// question with a stable answer while calls are being made.
//
// # No request or response bodies, and the screen says so
//
// The prototype's detail drawer shows both (admin-views.jsx:150). They are not
// stored, deliberately — a companion request body IS the user's conversation.
// Rather than leave an operator wondering where the drawer went, the empty
// state and the detail panel both say it out loud.
export function AILogs({ onUnauthorized }) {
  const [filter, setFilter] = useState({ endpoint: '', status: '', model: '', sessionId: '' });
  const [extra, setExtra] = useState([]); // pages loaded past the first
  const [cursor, setCursor] = useState(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [sel, setSel] = useState(null);

  // The first page refetches whenever the filter changes; later pages are
  // accumulated separately so changing a filter throws them away rather than
  // stitching two different queries together.
  const load = useCallback(() => {
    return api.getAILogs({ ...filter, limit: 50 }).then((d) => {
      setExtra([]);
      setCursor(d.nextBeforeAt ? { beforeAt: d.nextBeforeAt, beforeId: d.nextBeforeId } : null);
      return d;
    });
  }, [filter]);
  const state = useSection(load, { onUnauthorized });
  const d = state.data;
  const rows = [...(d?.logs || []), ...extra];

  async function more() {
    if (!cursor) return;
    setLoadingMore(true);
    try {
      const next = await api.getAILogs({ ...filter, ...cursor, limit: 50 });
      setExtra((prev) => [...prev, ...(next.logs || [])]);
      setCursor(next.nextBeforeAt ? { beforeAt: next.nextBeforeAt, beforeId: next.nextBeforeId } : null);
    } catch (e) {
      if (e instanceof api.Unauthorized) onUnauthorized?.();
    } finally {
      setLoadingMore(false);
    }
  }

  const set = (patch) => setFilter((f) => ({ ...f, ...patch }));

  return (
    <Screen
      title="AI 日志"
      sub={
        d?.retentionDays
          ? `每一次模型调用一行，保留 ${d.retentionDays} 天。更早的已经被清掉了 —— 查不到不等于没发生。`
          : '每一次模型调用一行。'
      }
      state={state}
    >
      {d && (
        <>
          <div className="filters">
            <button
              className={`chip ${filter.endpoint === '' ? 'on' : ''}`}
              onClick={() => set({ endpoint: '' })}
            >
              全部
            </button>
            {(d.endpoints || []).map((e) => (
              <button
                key={e}
                className={`chip ${filter.endpoint === e ? 'on' : ''}`}
                onClick={() => set({ endpoint: filter.endpoint === e ? '' : e })}
              >
                {e}
              </button>
            ))}
            <span className="filters-gap" />
            <button
              className={`chip ${filter.status === 'error' ? 'on' : ''}`}
              onClick={() => set({ status: filter.status === 'error' ? '' : 'error' })}
            >
              仅失败
            </button>
            <input
              className="filter-input"
              placeholder="模型"
              value={filter.model}
              onChange={(e) => set({ model: e.target.value.trim() })}
            />
            <input
              className="filter-input"
              placeholder="会话 id"
              value={filter.sessionId}
              onChange={(e) => set({ sessionId: e.target.value.trim() })}
            />
          </div>

          {rows.length === 0 ? (
            <Empty>没有匹配的调用。</Empty>
          ) : (
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>时间</th>
                    <th>用途</th>
                    <th>模型</th>
                    <th>Tokens</th>
                    <th>耗时</th>
                    <th>结果</th>
                    <th>会话</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((l) => (
                    <tr key={l.id} className="clickable" onClick={() => setSel(l)}>
                      <td className="mono">{shortTime(l.createdAt)}</td>
                      <td>{l.endpoint}</td>
                      <td className="mono">{l.model}</td>
                      <td className="mono">
                        {l.promptTokens} → {l.compTokens}
                      </td>
                      <td className="mono">{(l.durationMs / 1000).toFixed(1)}s</td>
                      <td>
                        <span className={`pill ${l.status === 'ok' ? 'ok' : 'err'}`}>
                          {l.status === 'ok' ? '成功' : '失败'}
                        </span>
                      </td>
                      <td className="mono dim">{l.sessionId}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <div className="pager">
            <span className="muted">已加载 {rows.length} 条</span>
            {cursor ? (
              <button disabled={loadingMore} onClick={more}>
                {loadingMore ? '…' : '再加载 50 条'}
              </button>
            ) : (
              rows.length > 0 && <span className="muted">到底了</span>
            )}
          </div>

          {sel && <Detail log={sel} onClose={() => setSel(null)} />}
        </>
      )}
    </Screen>
  );
}

function Detail({ log, onClose }) {
  return (
    <>
      <div className="scrim" onClick={onClose} />
      <aside className="drawer">
        <div className="drawer-head">
          <h3>调用详情</h3>
          <button className="linkish" onClick={onClose}>
            关闭
          </button>
        </div>
        <div className="kv">
          <Row k="ID" v={log.id} mono />
          <Row k="时间" v={shortTime(log.createdAt)} mono />
          <Row k="用途" v={log.endpoint} />
          <Row k="模型" v={log.model} mono />
          <Row k="Tokens" v={`${log.promptTokens} in / ${log.compTokens} out`} mono />
          <Row k="耗时" v={`${log.durationMs} ms`} mono />
          <Row k="结果" v={log.status === 'ok' ? '成功' : log.error || '失败'} />
          <Row k="会话" v={log.sessionId} mono />
          <Row k="请求 id" v={log.requestId || '—'} mono />
        </div>
        <Notice kind="info" title="这里没有请求与响应正文">
          不是漏了 —— 陪伴调用的请求体<strong>就是用户的对话</strong>，所以它从来没有被存下来。
          「看 AI 日志」这一项权限的说明里写的也是这个。真要留存正文，那要另一条权限，不能挂在这一条下面。
        </Notice>
      </aside>
    </>
  );
}

function Row({ k, v, mono }) {
  return (
    <div className="field">
      <div className="field-label">{k}</div>
      <div className={`field-value ${mono ? 'mono' : ''}`}>{v}</div>
    </div>
  );
}
