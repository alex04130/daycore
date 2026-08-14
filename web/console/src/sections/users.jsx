import { useCallback, useState } from 'react';
import * as api from '../api.js';
import * as I from '../icons.jsx';
import { AdmConfirm, Avatar, Notice, useSection, useToast } from '../ui.jsx';

// 用户. The prototype's table is about app users (注册/匿名 + 会话/计划/资料
// counts); the real GET /api/v2/admin/users returns console ACCOUNTS with owner/
// roles/usage. The table below renders the real columns in the prototype's form,
// with the one real write (DELETE /users/{id}) behind a confirm.
export function Users({ onUnauthorized }) {
  const toast = useToast();
  const load = useCallback(function () { return api.getUsers(); }, []);
  const state = useSection(load, { onUnauthorized });
  const [del, setDel] = useState(null);

  const d = state.data;
  const users = (d && d.users) || [];

  async function doDelete(u) {
    try {
      await api.deleteUser(u.id);
      toast('已删除');
      state.reload();
    } catch (e) {
      if (e instanceof api.Unauthorized) return onUnauthorized && onUnauthorized();
      toast(e.message);
    }
  }

  return (
    <div data-screen-label="用户管理">
      <header className="adm-head">
        <div>
          <h1 className="adm-title">用户</h1>
          <p className="adm-sub">
            {d ? (d.ownerCount || 0) + ' 个 owner · ' : ''}<span className="mono">GET /api/admin/users</span>
          </p>
        </div>
      </header>

      {state.status === 'loading' && <div className="adm-empty">读取中…</div>}
      {state.status === 'error' && <Notice kind="error">{state.error}</Notice>}

      {d && (
        <div className="adm-card">
          <div className="adm-table-wrap">
            <table className="adm-table">
              <thead><tr><th>用户</th><th>角色</th><th>用量</th><th>创建</th><th></th></tr></thead>
              <tbody>
                {users.map(function (u) {
                  return (
                    <tr key={u.id}>
                      <td>
                        {u.name ? (
                          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 10 }}>
                            <Avatar name={u.name} size={30} />
                            <span style={{ display: 'inline-flex', flexDirection: 'column' }}>
                              <span style={{ fontWeight: 650, display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                                {u.name}
                                {u.owner ? <span className="adm-pill anon" title="owner 是破窗标记">owner</span> : null}
                              </span>
                              <span className="mono" style={{ fontSize: 11, color: 'var(--color-text-muted)' }}>{u.email || u.id}</span>
                            </span>
                          </span>
                        ) : (
                          <span className="mono">{u.id}</span>
                        )}
                      </td>
                      <td>
                        {u.roles && u.roles.length ? (
                          <span style={{ display: 'inline-flex', gap: 5, flexWrap: 'wrap' }}>
                            {u.roles.map(function (r) { return <span key={r} className="adm-pill reg">{r}</span>; })}
                          </span>
                        ) : (
                          <span className="dim">—</span>
                        )}
                      </td>
                      <td className="mono">{usageLine(u.usage)}</td>
                      <td className="mono">{u.createdAt ? u.createdAt.slice(0, 10) : '—'}</td>
                      <td><button type="button" className="adm-trash" title="删除" onClick={function () { return setDel(u); }}><I.Trash size={15} /></button></td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            {users.length === 0 && <div className="adm-empty">还没有用户</div>}
          </div>
        </div>
      )}

      <AdmConfirm
        open={!!del}
        onClose={function () { return setDel(null); }}
        title={'删除 ' + (del ? (del.name || del.id) : '')}
        desc="将删除这个控制台账号（其数据会话的级联删除仍在后端待办）。不可恢复。"
        confirmLabel="永久删除"
        onConfirm={function () { if (del) doDelete(del); }}
      />
    </div>
  );
}

function usageLine(u) {
  if (!u) return '—';
  const calls = u.totalCalls || 0;
  const toks = (u.totalPromptTokens || 0) + (u.totalCompTokens || 0);
  return calls + ' 次 / ' + fmtTok(toks);
}

function fmtTok(n) {
  const v = n || 0;
  return v >= 10000 ? (v / 1000).toFixed(1) + 'k' : String(v);
}
