import { useCallback, useEffect, useState } from 'react';
import * as api from './api.js';
import { Login } from './login.jsx';
import { visibleSections } from './sections.jsx';
import { Notice } from './ui.jsx';

// The console shell: which section is showing, and whether we are signed in.
//
// # Signed-in state is not stored anywhere
//
// The credential is an httpOnly cookie this code cannot read, so "am I signed
// in" is not a fact the page owns — it is a question only the server can
// answer, and the answer can change under us when the short TTL expires.
//
// So there is no isAuthed flag kept in sync with anything. The shell asks
// GET /api/admin/session — "who am I, and what may I do" — and every section
// fetch either succeeds or throws Unauthorized, which drops the whole shell back
// to the login screen. A session that expires mid-use lands on a login prompt
// rather than on a screen of empty tables, which is the failure mode a stored
// boolean produces.
//
// # Which sections render is decided from that answer, and it is only cosmetic
//
// The principal carries a permission list, and sections the caller cannot open
// are left out of the nav. ⚠️ Nothing about that is a security boundary — the
// server checks every request, and this exists so a person with users.read does
// not click into a 403 as their welcome.
export function App() {
  // null = not yet determined, false = signed out, object = the principal.
  const [principal, setPrincipal] = useState(null);
  const [active, setActive] = useState(() => window.location.hash.replace(/^#/, ''));
  const [meta, setMeta] = useState(null);

  // Signal from any section that the credential is gone.
  const onUnauthorized = useCallback(() => setPrincipal(false), []);

  useEffect(() => {
    // whoami rather than a section read. It is the one admin endpoint that
    // needs a credential and no permission, so its answer is "is this session
    // good" and nothing else — probing with a section would conflate an expired
    // cookie, a missing permission and a database that is down.
    api
      .whoami()
      .then(setPrincipal)
      .catch((e) => setPrincipal(e instanceof api.Unauthorized ? false : { root: false, permissions: [] }));
  }, []);

  useEffect(() => {
    api.getMeta().then(setMeta).catch(() => {});
  }, [principal]);

  useEffect(() => {
    const onHash = () => setActive(window.location.hash.replace(/^#/, ''));
    window.addEventListener('hashchange', onHash);
    return () => window.removeEventListener('hashchange', onHash);
  }, []);

  if (principal === null) return <div className="boot">…</div>;
  if (principal === false) return <Login onDone={setPrincipal} />;

  const sections = visibleSections(principal);
  const current = sections.find((s) => s.id === active) || sections[0];

  return (
    <div className="shell">
      <nav className="nav">
        <div className="brand">
          Daycore
          <span className="brand-sub">运维控制台</span>
        </div>
        <ul>
          {sections.map((s) => (
            <li key={s.id}>
              <a href={`#${s.id}`} className={s.id === current?.id ? 'on' : ''}>
                {s.label}
              </a>
            </li>
          ))}
        </ul>
        <MetaFoot
          meta={meta}
          principal={principal}
          onLogout={() => api.logout().finally(() => setPrincipal(false))}
        />
      </nav>
      <main className="main">
        {current ? (
          <current.view onUnauthorized={onUnauthorized} principal={principal} />
        ) : (
          <div className="screen">
            <Notice kind="info" title="没有可以打开的分区">
              这个账号持有的权限还没有对应的界面。让运维在「用户与权限」里看一眼这个账号在哪些组。
            </Notice>
          </div>
        )}
      </main>
    </div>
  );
}

// MetaFoot is the always-visible answer to "what am I looking at".
//
// Build and instance in particular: health is per process, so two consoles
// pointed at a load-balanced deployment legitimately disagree — without the
// instance visible somewhere, that is an unexplainable bug report.
function MetaFoot({ meta, principal, onLogout }) {
  const v = meta?.version;
  const degraded = meta?.health && meta.health.status === 503;
  return (
    <div className="meta-foot">
      {/* Who is holding this session. Worth a line of its own: root and a
          person with every permission look identical until something is
          refused, and "why can I not do this" is the question the console
          should never make somebody guess at. */}
      <div className="meta-line who">
        {principal?.root ? 'ADMIN_TOKEN' : principal?.owner ? `owner · ${principal.userId}` : principal?.userId}
      </div>
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
