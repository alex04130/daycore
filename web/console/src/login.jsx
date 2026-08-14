import { useState } from 'react';
import * as api from './api.js';
import { ShieldCheck } from './icons.jsx';
import { Button, Input } from './ui.jsx';

// The gate. Visually the design prototype's Gate (admin-shell.jsx), but the
// credential path is the real one: the token is posted ONCE to
// POST /api/v2/admin/session and exchanged for a short-lived httpOnly cookie.
// The token itself is never stored — that is the whole point of the exchange,
// and it is why the prototype's sessionStorage arrangement is not reproduced.
export function Login({ onDone }) {
  const [tok, setTok] = useState('');
  const [err, setErr] = useState(false);
  const [busy, setBusy] = useState(false);

  async function go() {
    if (!tok.trim() || busy) return;
    setBusy(true);
    setErr(false);
    try {
      const principal = await api.login(tok.trim());
      setTok('');
      onDone(principal);
    } catch (e) {
      setErr(true);
      setBusy(false);
    }
  }

  return (
    <div className="adm-gate">
      <div className="adm-gate-card">
        <div className="adm-gate-ic">
          <ShieldCheck size={30} />
        </div>
        <h1>Daycore 管理控制台</h1>
        <p className="sub">
          输入 X-Admin-Token 连接服务端。
          <br />
          口令只出现这一次，之后换成一枚短期 httpOnly 管理 cookie，与用户会话完全隔离。
        </p>
        <Input
          type="password"
          placeholder="X-Admin-Token"
          value={tok}
          onChange={(e) => {
            setTok(e.target.value);
            setErr(false);
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') go();
          }}
        />
        {err ? <p className="err">Token 无效（401 unauthorized）</p> : null}
        <div className="btnrow">
          <Button variant="primary" size="lg" fullWidth disabled={busy || !tok.trim()} onClick={go}>
            {busy ? '连接中…' : '连接'}
          </Button>
        </div>
        <p className="hint">
          演示环境 token：<code>daycore-admin</code>
        </p>
      </div>
    </div>
  );
}
