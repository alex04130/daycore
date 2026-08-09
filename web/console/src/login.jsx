import { useState } from 'react';
import * as api from './api.js';

// The login screen.
//
// # The token is never stored, not even in a variable that outlives the request
//
// It lives in a controlled input for as long as somebody is typing it, is posted
// once, and the component unmounts. What comes back is an httpOnly cookie this
// code cannot read — which is the whole point: a credential JavaScript cannot
// reach is a credential an injected script cannot steal.
//
// The design prototype put the raw token in sessionStorage and re-sent it as a
// header forever (admin-store.js:19). That is the arrangement θ-F4a removed:
// long-lived, script-readable, no expiry, no revocation, no identity.
export function Login({ onDone }) {
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  async function submit(e) {
    e.preventDefault();
    setBusy(true);
    setErr('');
    try {
      await api.login(token);
      // Cleared before the state change, so the value is not sitting in a
      // React fibre after the screen is gone.
      setToken('');
      onDone();
    } catch (e2) {
      setErr(e2 instanceof api.Unauthorized ? '口令不对' : e2.message);
      setBusy(false);
    }
  }

  return (
    <div className="login">
      <form onSubmit={submit}>
        <h1>Daycore 运维控制台</h1>
        <p className="hint">
          用 <code>ADMIN_TOKEN</code> 换一张短期凭据。没设过它的话，启动日志里有一个自动生成的。
        </p>
        <input
          type="password"
          autoFocus
          autoComplete="off"
          spellCheck={false}
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder="ADMIN_TOKEN"
          aria-label="ADMIN_TOKEN"
        />
        {err && <div className="err">{err}</div>}
        <button disabled={busy || !token}>{busy ? '…' : '进入'}</button>
        <p className="hint small">
          换到的是一张 httpOnly 的短期 cookie，这个页面读不到它 —— 口令本身不会被存下来。
        </p>
      </form>
    </div>
  );
}
