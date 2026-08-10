import { useCallback, useState } from 'react';
import * as api from '../api.js';
import { Confirm, Empty, Notice, Screen, useSection } from '../ui.jsx';

// External consoles attached to this deployment.
//
// # The screen exists to make attaching a DECIDED act
//
// The alternative — an external console configured with some shared secret —
// has no moment at which anybody decided, no list of what is attached, and no
// way to detach one thing. This screen is that list.
//
// # ⚠️ The key is shown once, and the UI has to mean it
//
// After the dialog closes, the server holds only a hash. So the dialog does not
// auto-dismiss, does not close on a stray click, and says plainly that this is
// the only time. A console that showed it in a toast would be a console that
// loses somebody's key to a mistimed blink.
export function Pairings({ onUnauthorized, principal }) {
  const load = useCallback(() => api.getPairings(), []);
  const { reload, ...state } = useSection(load, { onUnauthorized });
  const [issued, setIssued] = useState(null);
  const [creating, setCreating] = useState(false);

  const canManage = principal?.root || principal?.permissions?.includes('pairings.manage');

  return (
    <Screen
      title="集群与外部控制台"
      sub="后端发一把钥匙，人自己搬到那个控制台去。不是那边猜这边的口令 —— 所以「有哪些东西接在这个部署上」是一份可以看、可以逐个撤销的清单。"
      state={state}
      actions={
        canManage && !creating && <button onClick={() => setCreating(true)}>发一把新钥匙</button>
      }
    >
      {issued && <IssuedKey issued={issued} onClose={() => setIssued(null)} />}

      {state.data && (
        <>
          {creating && (
            <Creator
              header={state.data.header}
              onCancel={() => setCreating(false)}
              onIssued={(res) => {
                setCreating(false);
                setIssued({ ...res, header: state.data.header });
                reload();
              }}
            />
          )}

          {state.data.pairings.length === 0 && !creating && (
            <Empty>还没有任何外部控制台接进来。</Empty>
          )}
          {state.data.pairings.map((p) => (
            <div className="row" key={p.id}>
              <div className="row-main">
                <div className="row-title">
                  <strong>{p.name}</strong>
                  <code className="muted">{p.id}</code>
                </div>
                {p.description && <div className="sub">{p.description}</div>}
                <div className="perm-chips">
                  {p.roles.length === 0 ? (
                    <span className="muted">不在任何组里 —— 能通过认证，但什么都做不了</span>
                  ) : (
                    p.roles.map((r) => (
                      <code key={r} className="chip">
                        {r}
                      </code>
                    ))
                  )}
                </div>
                <div className="usage-line">
                  <span title="按五分钟的粒度记，不是每次请求都写 —— 它要回答的只是「还在用吗」。">
                    上次用 <b>{p.lastSeenAt ? p.lastSeenAt.slice(0, 16).replace('T', ' ') : '从来没有'}</b>
                  </span>
                  <span>
                    发出于 <b>{p.createdAt ? p.createdAt.slice(0, 10) : '—'}</b>
                  </span>
                  <span title={p.permissions.join('\n') || '没有任何权限'}>
                    合计 <b>{p.permissions.length}</b> 项权限
                  </span>
                </div>
              </div>
              {canManage && (
                <div className="row-actions">
                  <Confirm
                    word={p.name}
                    label="撤销"
                    danger={`那个控制台会在下一次请求时被拒。没有「停用」这一档 —— 一个「关着」的凭据是一个别人不用重新决定就能打开的凭据。`}
                    onConfirm={() => api.deletePairing(p.id).then(reload, (e) => alert(e.message))}
                  />
                </div>
              )}
            </div>
          ))}
        </>
      )}
    </Screen>
  );
}

function Creator({ header, onCancel, onIssued }) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  async function go() {
    setBusy(true);
    setErr('');
    try {
      onIssued(await api.createPairing({ name: name.trim(), description }));
    } catch (e) {
      setErr(e.message);
      setBusy(false);
    }
  }

  return (
    <div className="row role-editor">
      <div className="field">
        <div className="field-label">名字</div>
        <div className="field-value">
          <input value={name} onChange={(e) => setName(e.target.value)} autoFocus maxLength={64}
                 placeholder="运营台 / cluster-eu" />
        </div>
      </div>
      <div className="field">
        <div className="field-label">说明</div>
        <div className="field-value">
          <input value={description} onChange={(e) => setDescription(e.target.value)}
                 placeholder="半年后还看得懂这是什么、跑在哪" />
        </div>
      </div>
      <Notice kind="info" title="先不给权限">
        新发的钥匙<strong>不在任何组里</strong> —— 能通过认证，但什么都做不了。
        这是有意的：先确认那边接上了，再决定给什么。之后在「用户与权限」里建好组，
        回来把它放进去（放进带权限的组还需要「改组权限」这一项）。
      </Notice>
      {err && <Notice kind="error">{err}</Notice>}
      <div className="row-actions">
        <button disabled={busy || !name.trim()} onClick={go}>
          {busy ? '…' : '生成'}
        </button>
        <button className="linkish" onClick={onCancel}>
          取消
        </button>
        <span className="muted">对方要把它放在 <code>{header}</code> 请求头里</span>
      </div>
    </div>
  );
}

// ⚠️ Not a toast, not auto-dismissing, and it does not close on a background
// click. This is the only time this string exists outside the other console.
function IssuedKey({ issued, onClose }) {
  const [copied, setCopied] = useState(false);
  return (
    <>
      <div className="scrim" />
      <div className="key-dialog">
        <h3>{issued.pairing.name} 的钥匙</h3>
        <Notice kind="warn" title="只显示这一次">
          {issued.notice}
        </Notice>
        <pre className="key-value">{issued.key}</pre>
        <p className="sub">
          在那个控制台里把它放进 <code>{issued.header}</code> 请求头。
        </p>
        <div className="row-actions">
          <button
            onClick={() => {
              navigator.clipboard?.writeText(issued.key).then(
                () => setCopied(true),
                () => {},
              );
            }}
          >
            {copied ? '已复制' : '复制'}
          </button>
          <button className="linkish" onClick={onClose}>
            我已经搬走了，关掉
          </button>
        </div>
      </div>
    </>
  );
}
