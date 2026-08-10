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
//
// # Two ways in, and the second one is the ordinary one
//
// Typing ADMIN_TOKEN is the root credential and the recovery path. An
// administrator with an account signs in to the app normally and presses the
// second button — the browser already holds their dc_auth cookie, so the
// request body is empty and the server exchanges that login for a console
// session carrying THEIR permissions.
//
// The second button is offered unconditionally rather than only when a login is
// detected: this page cannot read dc_auth (it is httpOnly and scoped to /), so
// "are you signed in" is not a question it can answer. Offering it and letting
// the server say no is honest; hiding it based on a guess would hide the normal
// path from the normal user.
export function Login({ onDone }) {
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  async function submit(e) {
    e.preventDefault();
    setBusy(true);
    setErr('');
    try {
      const principal = await api.login(token);
      // Cleared before the state change, so the value is not sitting in a
      // React fibre after the screen is gone.
      setToken('');
      onDone(principal);
    } catch (e2) {
      setErr(e2 instanceof api.Unauthorized ? '口令不对' : e2.message);
      setBusy(false);
    }
  }

  async function useMyAccount() {
    setBusy(true);
    setErr('');
    try {
      onDone(await api.loginAsMyself());
    } catch (e2) {
      setErr(
        e2 instanceof api.Unauthorized
          ? '这个浏览器里没有登录状态 —— 先在前台登录，再回来。'
          : e2.message,
      );
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
        <div className="or">或者</div>
        <button type="button" className="secondary" disabled={busy} onClick={useMyAccount}>
          用我自己的账号进
        </button>
        <p className="hint small">
          已经在前台登录过的管理员走这条 —— 拿到的权限来自你所在的组，不是全部。
          没有被分配任何控制台权限的账号会被拒绝，而不是拿到一张什么都做不了的凭据。
        </p>
      </form>
    </div>
  );
}
