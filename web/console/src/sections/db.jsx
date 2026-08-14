import { useCallback, useEffect, useRef, useState } from 'react';
import * as api from '../api.js';
import * as I from '../icons.jsx';
import { AdmConfirm, Button, Notice, useSection, useToast } from '../ui.jsx';

// 数据库 browser. Grid of tables → paged row view, with the one real delete
// (DELETE /db/table/{name}/{id}). The prototype's 备份/导入 hit endpoints the
// backend refuses on purpose (501, naming the right engine tool), so those
// buttons surface that honest message; 导出 works as a download link.
export function DB({ onUnauthorized }) {
  const [open, setOpen] = useState(null);
  const load = useCallback(function () { return api.getDBTables(); }, []);
  const state = useSection(load, { onUnauthorized });
  const toast = useToast();
  const fileRef = useRef(null);

  if (open) {
    return <TableView card={open} onBack={function () { setOpen(null); state.reload(); }} onUnauthorized={onUnauthorized} />;
  }

  async function doBackup() {
    try { await api.backup(); toast('备份已开始下载'); }
    catch (e) { toast('备份不可用：' + e.message); }
  }

  async function doImport(file) {
    if (!file) return;
    const rd = new FileReader();
    rd.onload = async function () {
      try { await api.importDB(JSON.parse(rd.result)); toast('导入成功'); }
      catch (e) { toast('导入不可用：' + e.message); }
    };
    rd.onerror = function () { toast('读取文件失败'); };
    rd.readAsText(file);
  }

  const tables = (state.data && state.data.tables) || [];

  return (
    <div data-screen-label="数据库">
      <header className="adm-head">
        <div>
          <h1 className="adm-title">数据库</h1>
          <p className="adm-sub mono">GET /api/admin/db/tables</p>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <Button variant="outline" onClick={doBackup}><I.Download size={15} /> 备份 .db</Button>
          <a href={api.exportURL()} className="dc-btn dc-btn--outline dc-btn--md" style={{ textDecoration: 'none' }}><I.FileJson size={15} /> 导出 JSON</a>
          <Button variant="outline" onClick={function () { return fileRef.current && fileRef.current.click(); }}><I.Upload size={15} /> 导入</Button>
          <input ref={fileRef} type="file" accept="application/json" hidden onChange={function (e) { const f = e.target.files[0]; if (f) doImport(f); e.target.value = ''; }} />
        </div>
      </header>

      {state.status === 'loading' && <div className="adm-empty">读取中…</div>}
      {state.status === 'error' && <Notice kind="error">{state.error}</Notice>}

      {state.data && (
        <>
          <div className="adm-tbl-grid">
            {tables.map(function (t) {
              return (
                <button key={t.name} type="button" className="adm-tbl-card" disabled={!t.readable} title={t.readable ? '' : '需要「浏览用户内容」这一项权限'} onClick={function () { return setOpen(t); }}>
                  <I.Database size={19} />
                  <span>
                    <span className="nm" style={{ display: 'block' }}>{t.name}</span>
                    <span className="ct" style={{ display: 'block' }}>
                      {t.rows < 0 ? '数不出来' : t.rows.toLocaleString() + ' 行'}
                      {!t.readable ? ' · 看不了' : ''}
                    </span>
                  </span>
                </button>
              );
            })}
          </div>
          <p className="adm-note">导出是 JSON 清单，不是一致快照；真要备份用引擎自己的工具（sqlite3 .backup / pg_dump / mysqldump / mongodump）。</p>
        </>
      )}
    </div>
  );
}

const PAGE = 50;
const cut = function (v) {
  if (v === null || v === undefined) return 'NULL';
  const s = typeof v === 'string' ? v : JSON.stringify(v);
  return s.length > 38 ? s.slice(0, 38) + '…' : s;
};

function TableView({ card, onBack, onUnauthorized }) {
  const [offset, setOffset] = useState(0);
  const [data, setData] = useState(null);
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(true);
  const [delRow, setDelRow] = useState(null);
  const toast = useToast();

  const fetch = useCallback(function () {
    setBusy(true); setErr('');
    api.getDBTable(card.name, PAGE, offset)
      .then(setData)
      .catch(function (e) {
        if (e instanceof api.Unauthorized) onUnauthorized && onUnauthorized();
        else setErr(e.message);
      })
      .finally(function () { setBusy(false); });
  }, [card.name, offset, onUnauthorized]);

  useEffect(fetch, [fetch]);

  async function del(id) {
    try { await api.deleteDBRow(card.name, id); toast('已删除'); fetch(); }
    catch (e) { toast(e.message); }
  }

  const cols = (data && data.columns) || [];
  const rows = (data && data.rows) || [];
  const idAt = cols.findIndex(function (c) { return c === 'id' || c === '_id'; });
  const redacted = new Set((data && data.redacted) || []);
  const total = (data && data.total) || 0;
  const pages = Math.max(1, Math.ceil(total / PAGE));
  const page = Math.floor(offset / PAGE) + 1;

  return (
    <div data-screen-label="数据表浏览">
      <header className="adm-head">
        <div>
          <button type="button" className="adm-back" onClick={onBack}><I.ChevronLeft size={16} /> 数据表</button>
          <h1 className="adm-title mono" style={{ marginTop: 2 }}>{card.name}</h1>
          <p className="adm-sub">
            {total.toLocaleString()} 行 · {card.class === 'user_content' ? '用户写下的内容' : '运维数据'} · <span className="mono">GET /api/admin/db/table/{card.name}</span>
          </p>
        </div>
      </header>

      {err && <Notice kind="error">{err}</Notice>}
      {redacted.size > 0 && (
        <Notice kind="info" title="有列不给看"><code>{Array.from(redacted).join(', ')}</code> 是凭据，任何情况下都不从这里发出去。</Notice>
      )}
      {data && data.whyNot && !data.deletable && (
        <Notice kind="info" title="不能从这里删行">{data.whyNot}</Notice>
      )}

      <div className="adm-card">
        <div className="adm-table-wrap">
          <table className="adm-table">
            <thead><tr>{cols.map(function (c) { return <th key={c}>{c}</th>; })}{data && data.deletable && <th></th>}</tr></thead>
            <tbody>
              {rows.map(function (r, i) {
                return (
                  <tr key={idAt >= 0 ? String(r[idAt]) : i}>
                    {r.map(function (v, j) {
                      return <td key={j} className="mono" title={redacted.has(cols[j]) ? '•••' : cut(v)}>{cut(v)}</td>;
                    })}
                    {data && data.deletable && idAt >= 0 && (
                      <td><button type="button" className="adm-trash" title="删除行" onClick={function () { return setDelRow(r); }}><I.Trash size={15} /></button></td>
                    )}
                  </tr>
                );
              })}
            </tbody>
          </table>
          {!rows.length && !busy && <div className="adm-empty">没有可展示的行</div>}
        </div>
        <div className="adm-pager">
          <span>第 {page} / {pages} 页</span>
          <button type="button" className="adm-pgbtn" disabled={offset === 0 || busy} onClick={function () { return setOffset(Math.max(0, offset - PAGE)); }}><I.ChevronLeft size={16} /></button>
          <button type="button" className="adm-pgbtn" disabled={offset + PAGE >= total || busy} onClick={function () { return setOffset(offset + PAGE); }}><I.ChevronRight size={16} /></button>
        </div>
      </div>

      <AdmConfirm
        open={!!delRow}
        onClose={function () { return setDelRow(null); }}
        title="删除这一行"
        desc={delRow ? card.name + ' · ' + (idAt >= 0 ? String(delRow[idAt]) : '') + ' 将被永久删除，不走撤销日志。' : ''}
        confirmLabel="删除"
        onConfirm={function () { if (delRow && idAt >= 0) del(String(delRow[idAt])); }}
      />
    </div>
  );
}
