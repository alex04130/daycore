import { useCallback, useEffect, useState } from 'react';
import * as api from './api.js';
import { Login } from './login.jsx';
import { SECTIONS } from './sections.jsx';
import { ToastHost } from './ui.jsx';
import * as I from './icons.jsx';

// The shell: sidebar with the prototype's eight nav items, and the footer
// health/build/exit. The credential is the httpOnly cookie minted by the gate;
// this code never holds the token, so "am I signed in" is answered by asking
// GET /api/v2/admin/session, and any section fetch that gets a 401 drops the
// whole shell back to the gate.
const NAV = [
  { id: 'overview', label: '总览', icon: I.Gauge },
  { id: 'prompts', label: 'Prompt 管理', icon: I.FileText },
  { id: 'models', label: '模型', icon: I.Cpu },
  { id: 'oauth', label: 'OAuth', icon: I.KeyRound },
  { id: 'config', label: '服务配置', icon: I.Sliders },
  { id: 'ailogs', label: 'AI 日志', icon: I.ScrollText },
  { id: 'users', label: '用户', icon: I.Users },
  { id: 'db', label: '数据库', icon: I.Database },
];

export function App() {
  // null = not yet known, false = signed out, object = the principal.
  const [principal, setPrincipal] = useState(null);
  const [view, setView] = useState('overview');
  const [meta, setMeta] = useState({ health: null, version: null });

  const onUnauthorized = useCallback(() => setPrincipal(false), []);

  useEffect(() => {
    api
      .whoami()
      .then(setPrincipal)
      .catch((e) => setPrincipal(e instanceof api.Unauthorized ? false : { root: false, permissions: [] }));
  }, []);

  useEffect(() => {
    if (!principal || principal === false) return;
    // Authenticated health (the rich view) + public version (API contract).
    api.getHealth().then((h) => setMeta((m) => ({ ...m, health: h }))).catch(() => {});
    api.getMeta().then((m) => setMeta((p) => ({ ...p, version: m.version }))).catch(() => {});
  }, [principal]);

  let content;
  if (principal === null) {
    content = <div className="adm-boot">…</div>;
  } else if (principal === false) {
    content = <Login onDone={setPrincipal} />;
  } else {
    content = (
      <Shell
        principal={principal}
        view={view}
        setView={setView}
        meta={meta}
        onUnauthorized={onUnauthorized}
        onLogout={() => api.logout().finally(() => setPrincipal(false))}
      />
    );
  }

  return <ToastHost>{content}</ToastHost>;
}

function Shell({ principal, view, setView, meta, onUnauthorized, onLogout }) {
  const current = SECTIONS.find((s) => s.id === view) || SECTIONS[0];
  const View = current.view;
  const health = meta.health;
  const version = meta.version;

  return (
    <div className="adm-app">
      <aside className="adm-side">
        <div className="adm-logo">
          <span className="dot"></span>
          Daycore Ops
          <span className="env">{health?.channel || version?.channel || ''}</span>
        </div>
        <nav className="adm-nav">
          {NAV.map((it) => {
            const Icon = it.icon;
            return (
              <button
                key={it.id}
                type="button"
                className={'adm-nav-item' + (view === it.id ? ' is-active' : '')}
                onClick={() => setView(it.id)}
              >
                <Icon size={18} />
                {it.label}
              </button>
            );
          })}
        </nav>
        <div className="adm-side-foot">
          <div className="adm-health">
            <span className={'dot' + (health?.degraded ? ' bad' : '')}></span>
            {health ? (health.degraded ? '降级运行' : '服务正常') + ' · db ' + (health.dbReachable ? 'ok' : '不可达') : '读取健康状态…'}
          </div>
          <div className="adm-health" style={{ paddingBottom: 6 }}>
            <span className="mono">build {health?.build || version?.build || '—'}</span>
          </div>
          <a className="adm-nav-item" href="/">
            <I.ExternalLink size={17} />
            返回应用
          </a>
          <button type="button" className="adm-nav-item" onClick={onLogout}>
            <I.LogOut size={17} />
            退出控制台
          </button>
        </div>
      </aside>
      <main className="adm-body">
        <View
          onUnauthorized={onUnauthorized}
          principal={principal}
          health={health}
          version={version}
          go={setView}
        />
      </main>
    </div>
  );
}
