// Daycore Admin — prompts / AI logs / users / database views
(function () {
  'use strict';
  const { useState, useEffect, useRef, useContext, useMemo } = React;
  const UI = window.DaycoreUI;
  const IC = window.DcIcons;
  const A = window.AdminStore;
  const { useAdmin, ToastCtx, AdmConfirm, Seg } = window.AdminShell;

  // ---------- prompts ----------
  function Prompts() {
    useAdmin();
    const toast = useContext(ToastCtx);
    const [key, setKey] = useState(A.prompts[0].key);
    const [loc, setLoc] = useState('zh-CN');
    const [draft, setDraft] = useState(() => A.getPrompt(key, loc));
    const taRef = useRef(null);
    useEffect(() => { setDraft(A.getPrompt(key, loc)); }, [key, loc]);
    function insertPh(name) {
      const token = '{{' + name + '}}';
      const ta = taRef.current;
      if (!ta) { setDraft(draft + token); return; }
      const s = ta.selectionStart == null ? draft.length : ta.selectionStart;
      const e2 = ta.selectionEnd == null ? s : ta.selectionEnd;
      setDraft(draft.slice(0, s) + token + draft.slice(e2));
      requestAnimationFrame(() => { ta.focus(); ta.selectionStart = ta.selectionEnd = s + token.length; });
    }
    const meta = A.prompts.find((p) => p.key === key);
    const saved = A.getPrompt(key, loc);
    const dirty = draft !== saved;
    const modified = A.isModified(key, loc);
    return (
      <div data-screen-label="Prompt 管理">
        <header className="adm-head">
          <div>
            <h1 className="adm-title">Prompt 管理</h1>
            <p className="adm-sub">改动保存后立即生效，按 locale 分别存储 · <span className="mono">PUT /api/admin/prompts/{'{key}'}?locale=</span></p>
          </div>
        </header>
        <div className="adm-prompts">
          <div className="adm-plist">
            {A.prompts.map((p) => (
              <button key={p.key} type="button" className={'adm-pitem' + (p.key === key ? ' is-active' : '')} onClick={() => setKey(p.key)}>
                <span className="nm">{p.name}{A.isModified(p.key) ? <i className="mdot" title="已修改"></i> : null}</span>
                <span className="ky">{p.key}</span>
              </button>
            ))}
          </div>
          <div className="adm-card adm-editor">
            <div className="adm-card-head" style={{ marginBottom: 6 }}>
              <IC.FileText size={16} /> {meta.name}
              <span style={{ marginLeft: 'auto' }}><Seg value={loc} onChange={setLoc} options={[{ value: 'zh-CN', label: '中文' }, { value: 'en-US', label: 'English' }]} /></span>
            </div>
            <p className="adm-sub" style={{ margin: '0 0 12px' }}>{meta.desc}{modified ? ' · 当前为修改版' : ''}</p>
            <textarea ref={taRef} spellCheck={false} value={draft} onChange={(e) => setDraft(e.target.value)}></textarea>
            <p className="adm-ph-label">可用占位符（点击插入，后端运行时注入）</p>
            <div className="adm-ph-row">
              {A.promptPlaceholders(key).map(([name, desc]) => (
                <button key={name} type="button" className="adm-ph-chip" title={desc} onClick={() => insertPh(name)}>{'{{' + name + '}}'}<span className="pd">{desc}</span></button>
              ))}
            </div>
            <div className="adm-editor-foot">
              <span>{draft.length} 字 · {'{{占位符}}'} 由后端注入</span>
              <span className="sp"></span>
              {modified || dirty ? <UI.Button variant="ghost" onClick={() => { A.resetPrompt(key, loc); setDraft(A.getPrompt(key, loc)); toast('已还原为默认'); }}>还原默认</UI.Button> : null}
              <UI.Button variant="primary" disabled={!dirty} onClick={() => { A.savePrompt(key, loc, draft); toast('已保存，立即生效'); }}>保存</UI.Button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // ---------- AI logs ----------
  const PER = 12;
  function Logs() {
    useAdmin();
    const [kind, setKind] = useState('all');
    const [status, setStatus] = useState('all');
    const [q, setQ] = useState('');
    const [page, setPage] = useState(0);
    const [sel, setSel] = useState(null);
    const rows = useMemo(() => A.ailogs.filter((l) =>
      (kind === 'all' || l.kind === kind) &&
      (status === 'all' || l.status === status) &&
      (!q.trim() || (l.sessionId + ' ' + l.model + ' ' + l.id).toLowerCase().includes(q.trim().toLowerCase()))
    ), [kind, status, q, useAdmin]);
    const pages = Math.max(1, Math.ceil(rows.length / PER));
    const pg = Math.min(page, pages - 1);
    useEffect(() => { setPage(0); }, [kind, status, q]);
    return (
      <div data-screen-label="AI 日志">
        <header className="adm-head">
          <div>
            <h1 className="adm-title">AI 日志</h1>
            <p className="adm-sub">最近 48 小时 · <span className="mono">GET /api/admin/ailogs</span></p>
          </div>
        </header>
        <div className="adm-card">
          <div className="adm-filters">
            <UI.Chip variant={kind === 'all' ? 'selected' : 'default'} onClick={() => setKind('all')}>全部</UI.Chip>
            {A.KINDS.map((k) => <UI.Chip key={k} variant={kind === k ? 'selected' : 'default'} onClick={() => setKind(k)}>{A.KIND_LABEL[k]}</UI.Chip>)}
            <span className="gap"></span>
            <UI.Chip variant={status === 'failed' ? 'selected' : 'default'} onClick={() => setStatus(status === 'failed' ? 'all' : 'failed')}>仅失败</UI.Chip>
            <div className="adm-search"><UI.Input placeholder="搜索会话 / 模型…" value={q} onChange={(e) => setQ(e.target.value)} /></div>
          </div>
          <div className="adm-table-wrap">
            <table className="adm-table">
              <thead><tr><th>时间</th><th>类型</th><th>模型</th><th>Tokens</th><th>耗时</th><th>状态</th><th>会话</th></tr></thead>
              <tbody>
                {rows.slice(pg * PER, pg * PER + PER).map((l) => (
                  <tr key={l.id} className="is-click" onClick={() => setSel(l)}>
                    <td className="mono">{A.fmtT(l.ts)}</td>
                    <td>{A.KIND_LABEL[l.kind]}</td>
                    <td className="mono">{l.model}</td>
                    <td className="mono">{l.tin} → {l.tout}</td>
                    <td className="mono">{(l.dur / 1000).toFixed(1)}s</td>
                    <td><span className={'adm-pill ' + (l.status === 'ok' ? 'ok' : 'fail')}>{l.status === 'ok' ? '成功' : l.err}</span></td>
                    <td className="mono">{l.sessionId}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {!rows.length ? <div className="adm-empty">没有匹配的日志</div> : null}
          </div>
          <div className="adm-pager">
            <span>{rows.length} 条 · 第 {pg + 1} / {pages} 页</span>
            <button type="button" className="adm-pgbtn" disabled={pg === 0} onClick={() => setPage(pg - 1)}><IC.ChevronLeft size={16} /></button>
            <button type="button" className="adm-pgbtn" disabled={pg >= pages - 1} onClick={() => setPage(pg + 1)}><IC.ChevronRight size={16} /></button>
          </div>
        </div>
        {sel ? (
          <React.Fragment>
            <div className="adm-scrim" onClick={() => setSel(null)}></div>
            <div className="adm-drawer">
              <div className="adm-drawer-head">
                <h3>调用详情</h3>
                <button type="button" className="adm-x" onClick={() => setSel(null)}><IC.X size={17} /></button>
              </div>
              <div className="adm-kv" style={{ gridTemplateColumns: '84px 1fr' }}>
                <span className="k">ID</span><span className="v mono">{sel.id}</span>
                <span className="k">时间</span><span className="v mono">{A.fmtT(sel.ts)}</span>
                <span className="k">类型</span><span className="v">{A.KIND_LABEL[sel.kind]}</span>
                <span className="k">模型</span><span className="v mono">{sel.model}</span>
                <span className="k">Tokens</span><span className="v mono">{sel.tin} in / {sel.tout} out</span>
                <span className="k">耗时</span><span className="v mono">{sel.dur} ms</span>
                <span className="k">状态</span><span className="v"><span className={'adm-pill ' + (sel.status === 'ok' ? 'ok' : 'fail')}>{sel.status === 'ok' ? '成功' : sel.err}</span></span>
                <span className="k">会话</span><span className="v mono">{sel.sessionId}</span>
              </div>
              <p className="adm-h4">请求</p>
              <pre className="adm-pre">{sel.req}</pre>
              <p className="adm-h4">响应</p>
              <pre className="adm-pre">{sel.resp}</pre>
            </div>
          </React.Fragment>
        ) : null}
      </div>
    );
  }

  // ---------- users ----------
  function UsersView() {
    useAdmin();
    const toast = useContext(ToastCtx);
    const [del, setDel] = useState(null);
    const users = A.usersList();
    const nReg = users.filter((u) => u.kind === 'registered').length;
    return (
      <div data-screen-label="用户管理">
        <header className="adm-head">
          <div>
            <h1 className="adm-title">用户</h1>
            <p className="adm-sub">注册 {nReg} · 匿名会话 {users.length - nReg} · <span className="mono">GET /api/admin/users</span></p>
          </div>
        </header>
        <div className="adm-card">
          <div className="adm-table-wrap">
            <table className="adm-table">
              <thead><tr><th>用户</th><th>类型</th><th>会话</th><th>计划</th><th>资料</th><th>最近活跃</th><th>创建</th><th></th></tr></thead>
              <tbody>
                {users.map((u) => (
                  <tr key={u.id}>
                    <td>
                      {u.kind === 'registered' ? (
                        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 10 }}>
                          <UI.Avatar name={u.name} size={30} />
                          <span style={{ display: 'inline-flex', flexDirection: 'column' }}>
                            <span style={{ fontWeight: 650 }}>{u.name}</span>
                            <span className="mono" style={{ fontSize: 11, color: 'var(--color-text-muted)' }}>{u.email}</span>
                          </span>
                        </span>
                      ) : <span className="mono">{u.id}</span>}
                    </td>
                    <td><span className={'adm-pill ' + (u.kind === 'registered' ? 'reg' : 'anon')}>{u.kind === 'registered' ? '注册' : '匿名'}</span></td>
                    <td className="mono">{u.sessions}</td>
                    <td className="mono">{u.plans}</td>
                    <td className="mono">{u.materials}</td>
                    <td className="mono">{A.relT(u.lastActive)}</td>
                    <td className="mono">{u.createdAt}</td>
                    <td><button type="button" className="adm-trash" title="删除" onClick={() => setDel(u)}><IC.Trash size={15} /></button></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
        <AdmConfirm open={!!del} onClose={() => setDel(null)} title={'删除 ' + (del ? (del.name || del.id) : '')}
          desc="将删除其全部数据：计划、资料、规则、记忆、主题与聊天记录，不可恢复。"
          confirmLabel="永久删除" onConfirm={() => { A.deleteUser(del.id); toast('已删除'); }} />
      </div>
    );
  }

  // ---------- database ----------
  const cut = (v) => { if (v === null || v === undefined) return 'NULL'; const s = typeof v === 'string' ? v : JSON.stringify(v); return s.length > 38 ? s.slice(0, 38) + '…' : s; };
  function DbView() {
    useAdmin();
    const toast = useContext(ToastCtx);
    const [table, setTable] = useState(null);
    const [delRow, setDelRow] = useState(null);
    const fileRef = useRef(null);
    if (!table) {
      return (
        <div data-screen-label="数据库">
          <header className="adm-head">
            <div>
              <h1 className="adm-title">数据库</h1>
              <p className="adm-sub mono">GET /api/admin/db/tables</p>
            </div>
            <div style={{ display: 'flex', gap: 8 }}>
              <UI.Button variant="outline" onClick={() => { A.backup(); toast('备份已开始下载'); }}><IC.Download size={15} /> 备份 .db</UI.Button>
              <UI.Button variant="outline" onClick={() => { A.exportAll(); toast('导出已开始下载'); }}><IC.FileJson size={15} /> 导出 JSON</UI.Button>
              <UI.Button variant="outline" onClick={() => fileRef.current && fileRef.current.click()}><IC.Upload size={15} /> 导入</UI.Button>
              <input ref={fileRef} type="file" accept="application/json" hidden
                onChange={(e) => { const f = e.target.files[0]; if (f) A.importJson(f, (r) => toast(r.ok ? '导入成功：' + r.tables + ' 张表' : '导入失败：不是合法 JSON')); e.target.value = ''; }} />
            </div>
          </header>
          <div className="adm-tbl-grid">
            {A.tablesList().map((t) => (
              <button key={t.name} type="button" className="adm-tbl-card" onClick={() => setTable(t.name)}>
                <IC.Database size={19} />
                <span>
                  <span className="nm" style={{ display: 'block' }}>{t.name}</span>
                  <span className="ct" style={{ display: 'block' }}>{t.rows.toLocaleString()} 行</span>
                </span>
              </button>
            ))}
          </div>
        </div>
      );
    }
    const rows = A.tableRows(table);
    const cols = rows.length ? Object.keys(rows[0]) : [];
    const meta = A.tablesList().find((t) => t.name === table);
    return (
      <div data-screen-label="数据表浏览">
        <header className="adm-head">
          <div>
            <button type="button" className="adm-back" onClick={() => setTable(null)}><IC.ChevronLeft size={16} /> 数据表</button>
            <h1 className="adm-title mono" style={{ marginTop: 2 }}>{table}</h1>
            <p className="adm-sub">{meta ? meta.rows.toLocaleString() : rows.length} 行 · <span className="mono">GET /api/admin/db/table/{table}</span></p>
          </div>
        </header>
        <div className="adm-card">
          <div className="adm-table-wrap">
            <table className="adm-table">
              <thead><tr>{cols.map((c) => <th key={c}>{c}</th>)}<th></th></tr></thead>
              <tbody>
                {rows.map((r) => (
                  <tr key={r.id}>
                    {cols.map((c) => <td key={c} className="mono">{cut(r[c])}</td>)}
                    <td><button type="button" className="adm-trash" title="删除行" onClick={() => setDelRow(r)}><IC.Trash size={15} /></button></td>
                  </tr>
                ))}
              </tbody>
            </table>
            {!rows.length ? <div className="adm-empty">没有可展示的行</div> : null}
          </div>
          <p className="adm-note">演示环境仅展示样例行；生产环境支持分页浏览全部数据。</p>
        </div>
        <AdmConfirm open={!!delRow} onClose={() => setDelRow(null)} title="删除这一行"
          desc={delRow ? table + ' · ' + delRow.id + ' 将被永久删除。' : ''}
          confirmLabel="删除" onConfirm={() => { A.deleteRow(table, delRow.id); toast('已删除'); }} />
      </div>
    );
  }

  window.AdminViews = { Prompts, Logs, UsersView, DbView };
})();
