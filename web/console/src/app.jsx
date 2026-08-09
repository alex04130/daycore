import { useCallback, useEffect, useState } from 'react';
import * as api from './api.js';
import { Login } from './login.jsx';
import { SECTIONS } from './sections.jsx';

// The console shell: which section is showing, and whether we are signed in.
//
// # Signed-in state is not stored anywhere
//
// The credential is an httpOnly cookie this code cannot read, so "am I signed
// in" is not a fact the page owns — it is a question only the server can
// answer, and the answer can change under us when the short TTL expires.
//
// So there is no isAuthed flag kept in sync with anything. Every section fetch
// either succeeds or throws Unauthorized, and Unauthorized anywhere drops the
// whole shell back to the login screen. That means a session that expires
// mid-use lands on a login prompt rather than on a screen of empty tables,
// which is the failure mode a stored boolean produces.
export function App() {
  const [authed, setAuthed] = useState(null); // null = not yet determined
  const [active, setActive] = useState(() => sectionFromHash());
  const [meta, setMeta] = useState(null);

  // Signal from any section that the credential is gone.
  const onUnauthorized = useCallback(() => setAuthed(false), []);

  useEffect(() => {
    // Probe with the cheapest authenticated endpoint. The config screen is the
    // right one: it is the only section guaranteed to work in degraded mode,
    // so this probe answers "is the credential good" without also asking "is
    // the database up".
    api
      .getConfig()
      .then(() => setAuthed(true))
      .catch((e) => setAuthed(!(e instanceof api.Unauthorized)));
  }, []);

  useEffect(() => {
    api.getMeta().then(setMeta).catch(() => {});
  }, [authed]);

  useEffect(() => {
    const onHash = () => setActive(sectionFromHash());
    window.addEventListener('hashchange', onHash);
    return () => window.removeEventListener('hashchange', onHash);
  }, []);

  if (authed === null) return <div className="boot">…</div>;
  if (!authed) return <Login onDone={() => setAuthed(true)} />;

  const Section = (SECTIONS.find((s) => s.id === active) || SECTIONS[0]).view;

  return (
    <div className="shell">
      <nav className="nav">
        <div className="brand">
          Daycore
          <span className="brand-sub">运维控制台</span>
        </div>
        <ul>
          {SECTIONS.map((s) => (
            <li key={s.id}>
              <a href={`#${s.id}`} className={s.id === active ? 'on' : ''}>
                {s.label}
              </a>
            </li>
          ))}
        </ul>
        <MetaFoot meta={meta} onLogout={() => api.logout().finally(() => setAuthed(false))} />
      </nav>
      <main className="main">
        <Section onUnauthorized={onUnauthorized} />
      </main>
    </div>
  );
}

// MetaFoot is the always-visible answer to "what am I looking at".
//
// Build and instance in particular: health is per process, so two consoles
// pointed at a load-balanced deployment legitimately disagree — without the
// instance visible somewhere, that is an unexplainable bug report.
function MetaFoot({ meta, onLogout }) {
  const v = meta?.version;
  const degraded = meta?.health && meta.health.status === 503;
  return (
    <div className="meta-foot">
      {degraded && (
        <div className="pill warn" title="存储不可用：只有管理面在服务，其它端点一律 503">
          降级运行
        </div>
      )}
      {v && (
        <div className="meta-line">
          {v.build}
          {v.channel ? `-${v.channel}` : ''}
        </div>
      )}
      <button className="linkish" onClick={onLogout}>
        退出
      </button>
    </div>
  );
}

function sectionFromHash() {
  const id = window.location.hash.replace(/^#/, '');
  return SECTIONS.some((s) => s.id === id) ? id : SECTIONS[0].id;
}
