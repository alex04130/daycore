import { useCallback, useEffect, useState } from 'react';
import * as api from '../api.js';
import { Confirm, Empty, Notice, Screen, useSection } from '../ui.jsx';

// The database browser.
//
// # Two classes of table, shown as two groups
//
// The catalogue splits every table into operational and user content, and the
// console renders that split rather than hiding it. A person with only
// db.operational still SEES the user-content group with its row counts — "how
// big is chat_messages" is an operational fact — and the cards say plainly that
// opening them needs another permission. Hiding their existence from somebody
// who can see every other table just makes the console look broken.
//
// # The delete button is the most dangerous control in this console
//
// It bypasses every business rule and writes nothing to the undo ledger. So it
// is behind Confirm, which requires typing the row's id rather than clicking a
// second time: a second click is muscle memory, and this has no undo.
export function DB({ onUnauthorized }) {
  const [open, setOpen] = useState(null);
  const load = useCallback(() => api.getDBTables(), []);
  const { reload, ...state } = useSection(load, { onUnauthorized });

  if (open) {
    return (
      <TableView
        card={open}
        onBack={() => {
          setOpen(null);
          reload();
        }}
        onUnauthorized={onUnauthorized}
      />
    );
  }

  const tables = state.data?.tables || [];
  const groups = [
    { class: 'operational', label: '运维数据', note: '会话、日志、任务、配置。不含任何人写下的内容。' },
    {
      class: 'user_content',
      label: '用户写下的内容',
      note: '对话、心情、记住的事、计划与愿望。这一组要单独的权限才能打开 —— 谁能看这些，是那个部署的负责人要做的决定。',
    },
  ];

  return (
    <Screen
      title="数据库"
      sub="这个部署里有什么，各有多少。没有搜索也没有排序 —— 那都意味着把一个用户给的东西拼进查询里，而这一屏的整个设计就是不让那件事发生。"
      state={state}
      actions={
        <a className="linkish" href={api.exportURL()}>
          导出 JSON
        </a>
      }
    >
      {state.data && (
        <>
          {groups.map((g) => {
            const rows = tables.filter((t) => t.class === g.class);
            if (rows.length === 0) return null;
            return (
              <div className="block" key={g.class}>
                <div className="block-head">
                  <h2>{g.label}</h2>
                  <span className="muted">{rows.length} 张表</span>
                </div>
                <p className="sub">{g.note}</p>
                <div className="table-grid">
                  {rows.map((t) => (
                    <button
                      key={t.name}
                      className={`table-card ${t.readable ? '' : 'locked'}`}
                      disabled={!t.readable}
                      onClick={() => setOpen(t)}
                      title={t.readable ? '' : '需要「浏览用户内容」这一项权限'}
                    >
                      <span className="table-name">{t.name}</span>
                      <span className="table-rows">
                        {t.rows < 0 ? '数不出来' : `${t.rows.toLocaleString()} 行`}
                      </span>
                      {!t.readable && <span className="pill mute">看不了</span>}
                    </button>
                  ))}
                </div>
              </div>
            );
          })}
          <Notice kind="info" title="导出和备份不是一回事">
            导出的是 JSON 清单，而且<strong>不是一致快照</strong> —— 存储层没有事务，表是一张一张读的，
            中间写进去的东西会出现在一张表里而不在另一张。真要备份，用引擎自己的工具
            （<code>sqlite3 .backup</code> / <code>pg_dump</code> / <code>mysqldump</code> / <code>mongodump</code>）。
            没有导入功能，原因同上：半个导入留下的库既不是旧数据也不是新数据。
          </Notice>
        </>
      )}
    </Screen>
  );
}

const PAGE = 50;

function TableView({ card, onBack, onUnauthorized }) {
  const [offset, setOffset] = useState(0);
  const [data, setData] = useState(null);
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(true);

  const fetch = useCallback(() => {
    setBusy(true);
    setErr('');
    api
      .getDBTable(card.name, PAGE, offset)
      .then(setData)
      .catch((e) => {
        if (e instanceof api.Unauthorized) onUnauthorized?.();
        else setErr(e.message);
      })
      .finally(() => setBusy(false));
  }, [card.name, offset, onUnauthorized]);

  useEffect(fetch, [fetch]);

  async function del(id) {
    try {
      await api.deleteDBRow(card.name, id);
      fetch();
    } catch (e) {
      setErr(e.message);
    }
  }

  const cols = data?.columns || [];
  const idAt = cols.findIndex((c) => c === 'id' || c === '_id');
  const redacted = new Set(data?.redacted || []);
  const total = data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / PAGE));
  const page = Math.floor(offset / PAGE) + 1;

  return (
    <section className="screen">
      <header className="screen-head">
        <div>
          <button className="linkish" onClick={onBack}>
            ← 数据表
          </button>
          <h1 className="mono">{card.name}</h1>
          <p className="sub">
            {total.toLocaleString()} 行 ·{' '}
            {card.class === 'user_content' ? '用户写下的内容' : '运维数据'}
            {data?.whyNot && ` · 不能从这里删行：${data.whyNot}`}
          </p>
        </div>
      </header>

      {err && <Notice kind="error">{err}</Notice>}
      {redacted.size > 0 && (
        <Notice kind="info" title="有列不给看">
          <code>{[...redacted].join(', ')}</code> 是凭据，任何情况下都不从这里发出去 ——
          显示的是一个固定占位符，所以你仍然看得到这一列存在。
        </Notice>
      )}

      {busy && !data ? (
        <div className="placeholder">读取中…</div>
      ) : !data || data.rows.length === 0 ? (
        <Empty>这一页没有行。</Empty>
      ) : (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                {cols.map((c) => (
                  <th key={c} className={redacted.has(c) ? 'redacted' : ''}>
                    {c}
                  </th>
                ))}
                {data.deletable && <th />}
              </tr>
            </thead>
            <tbody>
              {data.rows.map((row, i) => (
                <tr key={idAt >= 0 ? String(row[idAt]) : i}>
                  {row.map((v, j) => (
                    <td key={j} className="mono" title={cellFull(v)}>
                      {cell(v)}
                    </td>
                  ))}
                  {data.deletable && (
                    <td>
                      {idAt >= 0 && (
                        <Confirm
                          word={String(row[idAt])}
                          label="删除"
                          danger={`${card.name} 里的这一行会被永久删掉。不走撤销日志、不触发任何业务规则 —— 删了就没了。`}
                          onConfirm={() => del(String(row[idAt]))}
                        />
                      )}
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="pager">
        <span className="muted">
          第 {page} / {pages} 页
        </span>
        <button disabled={offset === 0 || busy} onClick={() => setOffset(Math.max(0, offset - PAGE))}>
          上一页
        </button>
        <button disabled={offset + PAGE >= total || busy} onClick={() => setOffset(offset + PAGE)}>
          下一页
        </button>
      </div>
    </section>
  );
}

// Cells are truncated to keep the grid readable; the full value is in the title
// attribute. A JSON blob column would otherwise make one row taller than the
// screen and push every other row out of view.
const MAX_CELL = 48;

function cellFull(v) {
  if (v === null || v === undefined) return 'NULL';
  return typeof v === 'string' ? v : JSON.stringify(v);
}

function cell(v) {
  if (v === null || v === undefined) return <span className="dim">NULL</span>;
  const s = cellFull(v);
  return s.length > MAX_CELL ? s.slice(0, MAX_CELL) + '…' : s;
}
