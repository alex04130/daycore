import { useCallback, useEffect, useMemo, useState } from 'react';
import * as api from '../api.js';
import { Chip, Drawer, Input, Notice, useSection } from '../ui.jsx';

function fmtT(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const p = function (n) { return String(n).padStart(2, '0'); };
  return p(d.getMonth() + 1) + '-' + p(d.getDate()) + ' ' + p(d.getHours()) + ':' + p(d.getMinutes()) + ':' + p(d.getSeconds());
}

// AI call ledger. Paging is keyset (the server hands back nextBeforeAt/Id and
// this screen hands it straight back), so there are no page numbers — the
// prototype's page-number pager does not survive the real cursor shape. The
// detail drawer has no request/response bodies: they are deliberately not
// stored (a companion request body IS the user's conversation).
export function AILogs({ onUnauthorized }) {
  const [filter, setFilter] = useState({ endpoint: '', status: '' });
  const [q, setQ] = useState('');
  const [extra, setExtra] = useState([]);
  const [cursor, setCursor] = useState(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [sel, setSel] = useState(null);

  const load = useCallback(function () {
    return api.getAILogs(Object.assign({}, filter, { limit: 50 })).then(function (d) {
      setExtra([]);
      setCursor(d.nextBeforeAt ? { beforeAt: d.nextBeforeAt, beforeId: d.nextBeforeId } : null);
      return d;
    });
  }, [filter]);
  const state = useSection(load, { onUnauthorized });
  const d = state.data;

  const all = (d && d.logs ? d.logs : []).concat(extra);
  const rows = useMemo(function () {
    const t = q.trim().toLowerCase();
    return all.filter(function (l) {
      if (!t) return true;
      return ((l.sessionId || '') + ' ' + (l.model || '') + ' ' + (l.id || '')).toLowerCase().indexOf(t) >= 0;
    });
  }, [all, q]);

  async function more() {
    if (!cursor) return;
    setLoadingMore(true);
    try {
      const next = await api.getAILogs(Object.assign({}, filter, cursor, { limit: 50 }));
      setExtra(function (prev) { return prev.concat(next.logs || []); });
      setCursor(next.nextBeforeAt ? { beforeAt: next.nextBeforeAt, beforeId: next.nextBeforeId } : null);
    } catch (e) {
      if (e instanceof api.Unauthorized) onUnauthorized && onUnauthorized();
    } finally {
      setLoadingMore(false);
    }
  }

  const set = function (patch) { setFilter(function (f) { return Object.assign({}, f, patch); }); };

  return (
    <div data-screen-label="AI 日志">
      <header className="adm-head">
        <div>
          <h1 className="adm-title">AI 日志</h1>
          <p className="adm-sub">
            {d && d.retentionDays ? '保留 ' + d.retentionDays + ' 天，更早的已被清掉 · ' : ''}<span className="mono">GET /api/admin/ailogs</span>
          </p>
        </div>
      </header>

      {state.status === 'loading' && <div className="adm-empty">读取中…</div>}
      {state.status === 'error' && <Notice kind="error">{state.error}</Notice>}

      {d && (
        <div className="adm-card">
          <div className="adm-filters">
            <Chip selected={filter.endpoint === ''} onClick={function () { return set({ endpoint: '' }); }}>全部</Chip>
            {(d.endpoints || []).map(function (e) {
              return (
                <Chip key={e} selected={filter.endpoint === e} onClick={function () { return set({ endpoint: filter.endpoint === e ? '' : e }); }}>{e}</Chip>
              );
            })}
            <span className="gap"></span>
            <Chip selected={filter.status === 'error'} onClick={function () { return set({ status: filter.status === 'error' ? '' : 'error' }); }}>仅失败</Chip>
            <div className="adm-search"><Input placeholder="搜索会话 / 模型…" value={q} onChange={function (e) { return setQ(e.target.value); }} /></div>
          </div>
          <div className="adm-table-wrap">
            <table className="adm-table">
              <thead><tr><th>时间</th><th>类型</th><th>模型</th><th>Tokens</th><th>耗时</th><th>状态</th><th>会话</th></tr></thead>
              <tbody>
                {rows.map(function (l) {
                  return (
                    <tr key={l.id} className="is-click" onClick={function () { return setSel(l); }}>
                      <td className="mono">{fmtT(l.createdAt)}</td>
                      <td>{l.endpoint}</td>
                      <td className="mono">{l.model}</td>
                      <td className="mono">{l.promptTokens} → {l.compTokens}</td>
                      <td className="mono">{(l.durationMs / 1000).toFixed(1)}s</td>
                      <td><span className={'adm-pill ' + (l.status === 'ok' ? 'ok' : 'fail')}>{l.status === 'ok' ? '成功' : l.error || '失败'}</span></td>
                      <td className="mono">{l.sessionId}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            {!rows.length && <div className="adm-empty">没有匹配的日志</div>}
          </div>
          <div className="adm-pager">
            <span>{rows.length} 条</span>
            {cursor ? (
              <button type="button" className="adm-pgbtn" style={{ width: 'auto', padding: '0 12px' }} disabled={loadingMore} onClick={more}>{loadingMore ? '…' : '加载更早'}</button>
            ) : rows.length > 0 ? (
              <span>到底了</span>
            ) : null}
          </div>
        </div>
      )}

      {sel && (
        <Drawer title="调用详情" onClose={function () { return setSel(null); }}>
          <div className="adm-kv" style={{ gridTemplateColumns: '84px 1fr' }}>
            <span className="k">ID</span><span className="v mono">{sel.id}</span>
            <span className="k">时间</span><span className="v mono">{fmtT(sel.createdAt)}</span>
            <span className="k">类型</span><span className="v">{sel.endpoint}</span>
            <span className="k">模型</span><span className="v mono">{sel.model}</span>
            <span className="k">Tokens</span><span className="v mono">{sel.promptTokens} in / {sel.compTokens} out</span>
            <span className="k">耗时</span><span className="v mono">{sel.durationMs} ms</span>
            <span className="k">状态</span><span className="v"><span className={'adm-pill ' + (sel.status === 'ok' ? 'ok' : 'fail')}>{sel.status === 'ok' ? '成功' : sel.error || '失败'}</span></span>
            <span className="k">会话</span><span className="v mono">{sel.sessionId}</span>
            <span className="k">请求 id</span><span className="v mono">{sel.requestId || '—'}</span>
          </div>
          <Notice kind="info" title="这里没有请求与响应正文">
            不是漏了 —— 陪伴调用的请求体<strong>就是用户的对话</strong>，所以它从来没有被存下来。
            「看 AI 日志」这一项权限的说明里写的也是这个。真要留存正文，那要另一条权限，不能挂在这一条下面。
          </Notice>
        </Drawer>
      )}
    </div>
  );
}
