// Daycore Admin — gate, shell, overview + shared bits
(function () {
  'use strict';
  const { useState, useRef, useSyncExternalStore, useContext } = React;
  const UI = window.DaycoreUI;
  const IC = window.DcIcons;
  const A = window.AdminStore;

  const useAdmin = () => useSyncExternalStore(A.subscribe, A.get);

  // ---------- toast ----------
  const ToastCtx = React.createContext(() => {});
  function ToastHost({ children }) {
    const [msg, setMsg] = useState(null);
    const ref = useRef(null);
    const show = (m) => { setMsg(m); clearTimeout(ref.current); ref.current = setTimeout(() => setMsg(null), 2200); };
    return (
      <ToastCtx.Provider value={show}>
        {children}
        {msg ? <div className="adm-toast">{msg}</div> : null}
      </ToastCtx.Provider>
    );
  }

  // ---------- confirm modal ----------
  function AdmConfirm({ open, title, desc, confirmLabel, onConfirm, onClose }) {
    if (!open) return null;
    return (
      <div className="adm-modal-scrim" onClick={onClose}>
        <div className="adm-modal" onClick={(e) => e.stopPropagation()}>
          <div className="adm-modal-ic"><IC.AlertTriangle size={22} /></div>
          <h3>{title}</h3>
          {desc ? <p>{desc}</p> : null}
          <div className="row">
            <UI.Button variant="outline" fullWidth onClick={onClose}>取消</UI.Button>
            <UI.Button variant="primary" fullWidth className="adm-danger-btn" onClick={() => { onConfirm(); onClose(); }}>{confirmLabel || '确认'}</UI.Button>
          </div>
        </div>
      </div>
    );
  }

  function Seg({ value, onChange, options }) {
    return (
      <div className="adm-seg">
        {options.map((o) => (
          <button key={o.value} type="button" className={value === o.value ? 'is-on' : ''} onClick={() => onChange(o.value)}>{o.label}</button>
        ))}
      </div>
    );
  }

  const fmtNum = (n) => (n >= 1e6 ? (n / 1e6).toFixed(1) + 'M' : n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n));

  // ---------- gate ----------
  function Gate() {
    const [tok, setTok] = useState('');
    const [err, setErr] = useState(false);
    const [busy, setBusy] = useState(false);
    function go() {
      if (!tok.trim() || busy) return;
      setBusy(true);
      setTimeout(() => { const r = A.login(tok.trim()); setBusy(false); if (r.error) setErr(true); }, 450);
    }
    return (
      <div className="adm-gate" data-screen-label="管理登录">
        <div className="adm-gate-card">
          <div className="adm-gate-ic"><IC.ShieldCheck size={30} /></div>
          <h1>Daycore 管理控制台</h1>
          <p className="sub">输入 X-Admin-Token 连接服务端。<br />所有管理请求仅通过该请求头鉴权，与用户会话完全隔离。</p>
          <UI.Input type="password" placeholder="X-Admin-Token" value={tok}
            onChange={(e) => { setTok(e.target.value); setErr(false); }}
            onKeyDown={(e) => { if (e.key === 'Enter') go(); }} />
          {err ? <p className="err">Token 无效（401 unauthorized）</p> : null}
          <div className="btnrow">
            <UI.Button variant="primary" size="lg" fullWidth disabled={busy || !tok.trim()} onClick={go}>{busy ? '连接中…' : '连接'}</UI.Button>
          </div>
          <p className="hint">演示环境 token：<code>daycore-admin</code></p>
        </div>
      </div>
    );
  }

  // ---------- overview ----------
  function Overview({ go }) {
    useAdmin();
    const toast = useContext(ToastCtx);
    const fileRef = useRef(null);
    const s = A.stats;
    const cards = [
      { lbl: '注册用户', num: s.users, icon: IC.Users },
      { lbl: '会话总数', num: s.sessions, icon: IC.User },
      { lbl: 'AI 调用', num: fmtNum(s.aiCalls), icon: IC.Sparkles },
      { lbl: 'Token 消耗', num: fmtNum(s.tokenUsed), icon: IC.Zap },
      { lbl: '反馈有用率', num: Math.round((s.feedbackUseful / s.feedbackTotal) * 100) + '%', sub: s.feedbackUseful + ' / ' + s.feedbackTotal, icon: IC.Heart },
    ];
    return (
      <div data-screen-label="管理总览">
        <header className="adm-head">
          <div>
            <h1 className="adm-title">总览</h1>
            <p className="adm-sub mono">GET /api/admin/stats</p>
          </div>
        </header>
        <div className="adm-stat-grid">
          {cards.map((c) => (
            <div key={c.lbl} className="adm-stat">
              <div className="top">{c.lbl}<c.icon size={16} /></div>
              <div className="num">{c.num}</div>
              {c.sub ? <div className="sub">{c.sub}</div> : null}
            </div>
          ))}
        </div>
        <div className="adm-two">
          <div className="adm-card">
            <div className="adm-card-head"><IC.Server size={16} /> 服务状态</div>
            <div className="adm-kv">
              <span className="k">服务</span><span className="v"><span className="dot"></span>正常</span>
              <span className="k">数据库</span><span className="v"><span className="dot"></span>ok</span>
              <span className="k">构建</span><span className="v mono">{A.health.version}<UI.Badge tone="warning">{A.health.channel}</UI.Badge></span>
              <span className="k">API 契约</span><span className="v mono">v{A.versionInfo.apiVersion}.{A.versionInfo.apiMinor} · minClient {A.versionInfo.minClient}</span>
              <span className="k">环境</span><span className="v mono">{A.health.env}</span>
              <span className="k">已运行</span><span className="v">{A.health.uptime}</span>
            </div>
          </div>
          <div className="adm-card">
            <div className="adm-card-head"><IC.Database size={16} /> 快捷操作</div>
            <div className="adm-act-row">
              <div className="adm-act-main">
                <div className="adm-act-title">下载数据库备份</div>
                <div className="adm-act-sub mono">GET /api/admin/db/backup（.db 快照）</div>
              </div>
              <UI.Button variant="outline" onClick={() => { A.backup(); toast('备份已开始下载'); }}><IC.Download size={15} /> 备份</UI.Button>
            </div>
            <div className="adm-act-row">
              <div className="adm-act-main">
                <div className="adm-act-title">导出全库 JSON</div>
                <div className="adm-act-sub mono">GET /api/admin/db/export</div>
              </div>
              <UI.Button variant="outline" onClick={() => { A.exportAll(); toast('导出已开始下载'); }}><IC.FileJson size={15} /> 导出</UI.Button>
            </div>
            <div className="adm-act-row">
              <div className="adm-act-main">
                <div className="adm-act-title">导入 JSON</div>
                <div className="adm-act-sub mono">POST /api/admin/db/import（合并写入）</div>
              </div>
              <UI.Button variant="outline" onClick={() => fileRef.current && fileRef.current.click()}><IC.Upload size={15} /> 导入</UI.Button>
              <input ref={fileRef} type="file" accept="application/json" hidden
                onChange={(e) => { const f = e.target.files[0]; if (f) A.importJson(f, (r) => toast(r.ok ? '导入成功：' + r.tables + ' 张表' : '导入失败：不是合法 JSON')); e.target.value = ''; }} />
            </div>
          </div>
        </div>
        <div className="adm-card">
          <div className="adm-card-head"><IC.ScrollText size={16} /> 最近 AI 调用<button type="button" className="lnk" onClick={() => go('logs')}>查看全部 →</button></div>
          <div className="adm-table-wrap">
            <table className="adm-table">
              <thead><tr><th>时间</th><th>类型</th><th>模型</th><th>Tokens</th><th>耗时</th><th>状态</th></tr></thead>
              <tbody>
                {A.ailogs.slice(0, 5).map((l) => (
                  <tr key={l.id}>
                    <td className="mono">{A.fmtT(l.ts)}</td>
                    <td>{A.KIND_LABEL[l.kind]}</td>
                    <td className="mono">{l.model}</td>
                    <td className="mono">{l.tin} → {l.tout}</td>
                    <td className="mono">{(l.dur / 1000).toFixed(1)}s</td>
                    <td><span className={'adm-pill ' + (l.status === 'ok' ? 'ok' : 'fail')}>{l.status === 'ok' ? '成功' : l.err}</span></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    );
  }

  // ---------- shell ----------
  const NAV = [
    { key: 'overview', label: '总览', icon: 'Gauge' },
    { key: 'prompts', label: 'Prompt 管理', icon: 'FileText' },
    { key: 'models', label: '模型', icon: 'Cpu' },
    { key: 'oauth', label: 'OAuth', icon: 'KeyRound' },
    { key: 'config', label: '服务配置', icon: 'Sliders' },
    { key: 'logs', label: 'AI 日志', icon: 'ScrollText' },
    { key: 'users', label: '用户', icon: 'Users' },
    { key: 'db', label: '数据库', icon: 'Database' },
  ];
  function Shell() {
    useAdmin();
    const [view, setView] = useState('overview');
    const V = window.AdminViews;
    return (
      <div className="adm-app">
        <aside className="adm-side">
          <div className="adm-logo"><span className="dot"></span>Daycore Ops<span className="env">{A.health.channel}</span></div>
          <nav className="adm-nav">
            {NAV.map((it) => { const Icon = IC[it.icon]; return (
              <button key={it.key} type="button" className={'adm-nav-item' + (view === it.key ? ' is-active' : '')} onClick={() => setView(it.key)}>
                <Icon size={18} />{it.label}
              </button>
            ); })}
          </nav>
          <div className="adm-side-foot">
            <div className="adm-health"><span className="dot"></span>服务正常 · db ok</div>
            <div className="adm-health" style={{ paddingBottom: 6 }}><span className="mono">build {A.health.version}</span></div>
            <a className="adm-nav-item" href="Daycore App v2.2.html"><IC.ExternalLink size={17} />返回应用</a>
            <button type="button" className="adm-nav-item" onClick={() => A.logout()}><IC.LogOut size={17} />退出控制台</button>
          </div>
        </aside>
        <main className="adm-body">
          {view === 'overview' ? <Overview go={setView} /> : null}
          {view === 'prompts' ? <V.Prompts /> : null}
          {view === 'models' ? <V.ModelsView /> : null}
          {view === 'oauth' ? <V.OAuthView /> : null}
          {view === 'config' ? <V.ConfigView /> : null}
          {view === 'logs' ? <V.Logs /> : null}
          {view === 'users' ? <V.UsersView /> : null}
          {view === 'db' ? <V.DbView /> : null}
        </main>
      </div>
    );
  }

  window.AdminShell = { useAdmin, ToastCtx, ToastHost, AdmConfirm, Seg, Gate, Shell };
})();
