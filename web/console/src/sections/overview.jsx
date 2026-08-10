import { useCallback } from 'react';
import * as api from '../api.js';
import { Empty, Field, Notice, Screen, useSection } from '../ui.jsx';

// The screen somebody opens when they do not yet know what they are looking for.
//
// # Everything here answers a question an operator actually has
//
// The prototype's five stat cards are kept. "AI 调用" and "Token 消耗" ARE
// totals — they read the spend rollup, which is folded server-side per closed
// day and kept forever, plus today's live ledger rows.
//
// They did not used to be. They were COUNT and SUM over a table pruned at
// ninety days, so they were window figures wearing the label of a total and
// they went DOWN as the window slid. The card now carries `since`, because on a
// deployment that upgraded into the rollup the count begins where the ledger
// began — and a total rendered without that date is the same lie one level up.
//
// # What is deliberately not a card
//
// Uptime. The health endpoint reports startedAt and the console could render a
// duration — but on a load-balanced deployment health is per process, so two
// consoles legitimately disagree and "uptime 3 minutes" reads as a crash loop
// when it is a rolling restart. The instance id in the footer is the honest
// version of that fact.
export function Overview({ onUnauthorized, principal }) {
  const load = useCallback(
    () =>
      Promise.all([
        api.getStats().catch((e) => ({ __err: e })),
        api.getAILogs({ limit: 6 }).catch((e) => ({ __err: e })),
        api.getMeta(),
      ]).then(([stats, logs, meta]) => ({ stats, logs, meta })),
    [],
  );
  const state = useSection(load, { onUnauthorized });
  const d = state.data;

  return (
    <Screen
      title="总览"
      sub="这个部署现在是什么状态，以及它最近在做什么。"
      state={state}
    >
      {d && (
        <>
          <StatGrid stats={d.stats} />
          <ServiceCard meta={d.meta} principal={principal} />
          <RecentCalls logs={d.logs} />
        </>
      )}
    </Screen>
  );
}

const fmt = (n) =>
  typeof n === 'number' ? (n >= 10000 ? (n / 1000).toFixed(1) + 'k' : String(n)) : '—';

function StatGrid({ stats }) {
  if (stats?.__err) {
    return (
      <Notice kind="warn" title="统计读不到">
        {stats.__err.message}
      </Notice>
    );
  }
  // The AI figures are totals, not a window — they come from the rollup, which
  // is kept forever, plus today's live rows. `since` is what makes them honest:
  // on a deployment that upgraded into the rollup the count begins where the
  // ledger began, and a total rendered without that date is the same lie the
  // rollup was built to fix.
  const since = stats?.since ? `自 ${stats.since}` : '';
  const tokens = (stats?.promptTokens ?? 0) + (stats?.tokenUsed ?? 0);
  const cards = [
    { lbl: '注册用户', num: fmt(stats?.users) },
    { lbl: '会话总数', num: fmt(stats?.sessions) },
    {
      lbl: 'AI 调用',
      num: fmt(stats?.aiCalls),
      sub: stats?.aiErrors ? `${since} · ${stats.aiErrors} 次失败` : since,
    },
    {
      lbl: 'Token 消耗',
      num: fmt(tokens),
      sub: `${fmt(stats?.promptTokens)} 入 / ${fmt(stats?.tokenUsed)} 出`,
    },
    {
      lbl: '反馈有用率',
      num: stats?.feedbackTotal
        ? Math.round((stats.feedbackUseful / stats.feedbackTotal) * 100) + '%'
        : '—',
      sub: stats?.feedbackTotal ? `${stats.feedbackUseful} / ${stats.feedbackTotal}` : '还没有人评过',
    },
  ];
  return (
    <div className="stat-grid">
      {cards.map((c) => (
        <div key={c.lbl} className="stat">
          <div className="stat-top">{c.lbl}</div>
          <div className="stat-num">{c.num}</div>
          {c.sub && <div className="stat-sub">{c.sub}</div>}
        </div>
      ))}
    </div>
  );
}

function ServiceCard({ meta, principal }) {
  const h = meta?.health;
  const v = meta?.version;
  const degraded = h && h.status === 503;
  return (
    <div className="block">
      <div className="block-head">
        <h2>服务状态</h2>
      </div>
      {degraded && (
        <Notice kind="warn" title="降级运行">
          存储不可用，所以只有管理面在服务，其它端点一律 503。
          <strong>降级是启动时决定的，不会自己恢复</strong> —— 修好配置后要重启进程。
        </Notice>
      )}
      <div className="kv">
        <Field label="数据库">{h?.db || (h?.ok ? '正常' : '读不到')}</Field>
        <Field label="环境">{h?.env || '—'}</Field>
        <Field label="构建">
          {v ? `${v.build}${v.channel ? '-' + v.channel : ''}` : '—'}
        </Field>
        <Field
          label="API 契约"
          why="前端靠这两个数字决定自己能不能连这个后端。minClient 是这个后端要求的最低前端版本。"
        >
          {v ? `v${v.apiVersion}.${v.apiMinor}${v.minClient ? ` · minClient ${v.minClient}` : ''}` : '—'}
        </Field>
        <Field label="你的身份" why="控制台按这个决定显示哪些分区；每个端点仍然自己再查一次。">
          {principal?.root ? 'ADMIN_TOKEN（root）' : principal?.owner ? 'owner' : `${principal?.permissions?.length ?? 0} 项权限`}
        </Field>
      </div>
    </div>
  );
}

function RecentCalls({ logs }) {
  if (logs?.__err) {
    return (
      <div className="block">
        <div className="block-head">
          <h2>最近 AI 调用</h2>
        </div>
        <Notice kind="warn" title="读不到">
          {logs.__err.message}
        </Notice>
      </div>
    );
  }
  const rows = logs?.logs || [];
  return (
    <div className="block">
      <div className="block-head">
        <h2>最近 AI 调用</h2>
        <a href="#ai-logs" className="linkish">
          全部 →
        </a>
      </div>
      {rows.length === 0 ? (
        <Empty>还没有调用记录。</Empty>
      ) : (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th>时间</th>
                <th>用途</th>
                <th>模型</th>
                <th>Tokens</th>
                <th>耗时</th>
                <th>结果</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((l) => (
                <tr key={l.id}>
                  <td className="mono">{shortTime(l.createdAt)}</td>
                  <td>{l.endpoint}</td>
                  <td className="mono">{l.model}</td>
                  <td className="mono">
                    {l.promptTokens} → {l.compTokens}
                  </td>
                  <td className="mono">{(l.durationMs / 1000).toFixed(1)}s</td>
                  <td>
                    <span className={`pill ${l.status === 'ok' ? 'ok' : 'err'}`}>
                      {l.status === 'ok' ? '成功' : '失败'}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// Local time, seconds included: the question asked of this column is almost
// always "was that the one I just triggered", and minutes cannot answer it.
export function shortTime(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const p = (n) => String(n).padStart(2, '0');
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}
