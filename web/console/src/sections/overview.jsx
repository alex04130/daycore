import { useCallback, useRef } from 'react';
import * as api from '../api.js';
import * as I from '../icons.jsx';
import { Badge, Button, Notice, useSection, useToast } from '../ui.jsx';

const fmtNum = (n) =>
  typeof n === 'number' ? (n >= 1e6 ? (n / 1e6).toFixed(1) + 'M' : n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n)) : '—';

function fmtUptime(sec) {
  if (sec == null || Number.isNaN(sec)) return '—';
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d > 0) return d + ' 天 ' + h + ' 小时';
  if (h > 0) return h + ' 小时 ' + m + ' 分';
  return m + ' 分 ' + (sec % 60) + ' 秒';
}

function fmtT(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const p = (n) => String(n).padStart(2, '0');
  return p(d.getMonth() + 1) + '-' + p(d.getDate()) + ' ' + p(d.getHours()) + ':' + p(d.getMinutes());
}

export function Overview({ onUnauthorized, health, version, go }) {
  const toast = useToast();
  const fileRef = useRef(null);

  const load = useCallback(
    () =>
      Promise.all([
        api.getStats().catch((e) => ({ __err: e })),
        api.getAILogs({ limit: 6 }).catch((e) => ({ __err: e })),
      ]).then(([stats, logs]) => ({ stats, logs })),
    [],
  );
  const state = useSection(load, { onUnauthorized });
  const d = state.data;

  async function doBackup() {
    try {
      await api.backup();
      toast('备份已开始下载');
    } catch (e) {
      toast('备份不可用：' + e.message);
    }
  }

  async function doImport(file) {
    if (!file) return;
    const reader = new FileReader();
    reader.onload = async () => {
      try {
        const json = JSON.parse(reader.result);
        await api.importDB(json);
        toast('导入成功');
      } catch (e) {
        toast('导入不可用：' + e.message);
      }
    };
    reader.onerror = () => toast('读取文件失败');
    reader.readAsText(file);
  }

  return (
    <div data-screen-label="管理总览">
      <header className="adm-head">
        <div>
          <h1 className="adm-title">总览</h1>
          <p className="adm-sub mono">GET /api/admin/stats</p>
        </div>
      </header>

      {state.status === 'loading' && <div className="adm-empty">读取中…</div>}
      {state.status === 'error' && <Notice kind="error">{state.error}</Notice>}
      {d && <StatGrid stats={d.stats} />}

      <div className="adm-two">
        <ServiceCard health={health} version={version} />
        <div className="adm-card">
          <div className="adm-card-head">
            <I.Database size={16} /> 快捷操作
          </div>
          <div className="adm-act-row">
            <div className="adm-act-main">
              <div className="adm-act-title">下载数据库备份</div>
              <div className="adm-act-sub mono">GET /api/admin/db/backup</div>
            </div>
            <Button variant="outline" onClick={doBackup}>
              <I.Download size={15} /> 备份
            </Button>
          </div>
          <div className="adm-act-row">
            <div className="adm-act-main">
              <div className="adm-act-title">导出全库 JSON</div>
              <div className="adm-act-sub mono">GET /api/admin/db/export</div>
            </div>
            <a href={api.exportURL()} className="dc-btn dc-btn--outline dc-btn--md" style={{ textDecoration: 'none' }}>
              <I.FileJson size={15} /> 导出
            </a>
          </div>
          <div className="adm-act-row">
            <div className="adm-act-main">
              <div className="adm-act-title">导入 JSON</div>
              <div className="adm-act-sub mono">POST /api/admin/db/import</div>
            </div>
            <Button variant="outline" onClick={() => fileRef.current && fileRef.current.click()}>
              <I.Upload size={15} /> 导入
            </Button>
            <input
              ref={fileRef}
              type="file"
              accept="application/json"
              hidden
              onChange={(e) => {
                const f = e.target.files[0];
                if (f) doImport(f);
                e.target.value = '';
              }}
            />
          </div>
        </div>
      </div>

      <div className="adm-card">
        <div className="adm-card-head">
          <I.ScrollText size={16} /> 最近 AI 调用
          <button type="button" className="lnk" onClick={() => go('ailogs')}>
            查看全部 →
          </button>
        </div>
        <RecentCalls logs={d?.logs} />
      </div>
    </div>
  );
}

function StatGrid({ stats }) {
  if (stats?.__err) {
    return <Notice kind="warn">统计读不到：{stats.__err.message}</Notice>;
  }
  const s = stats || {};
  const useful = s.feedbackUseful ?? 0;
  const total = s.feedbackTotal ?? 0;
  const cards = [
    { lbl: '注册用户', num: s.users, icon: I.Users },
    { lbl: '会话总数', num: s.sessions, icon: I.User },
    { lbl: 'AI 调用', num: fmtNum(s.aiCalls), icon: I.Sparkles, sub: s.since ? '自 ' + s.since : undefined },
    {
      lbl: 'Token 消耗',
      num: fmtNum(s.tokenUsed),
      icon: I.Zap,
      sub: fmtNum(s.promptTokens) + ' 入 / ' + fmtNum(s.tokenUsed) + ' 出',
    },
    {
      lbl: '反馈有用率',
      num: total ? Math.round((useful / total) * 100) + '%' : '—',
      sub: total ? useful + ' / ' + total : '还没有人评过',
      icon: I.Heart,
    },
  ];
  return (
    <div className="adm-stat-grid">
      {cards.map((c) => {
        const Icon = c.icon;
        return (
          <div key={c.lbl} className="adm-stat">
            <div className="top">
              {c.lbl}
              <Icon size={16} />
            </div>
            <div className="num">{c.num ?? '—'}</div>
            {c.sub ? <div className="sub">{c.sub}</div> : null}
          </div>
        );
      })}
    </div>
  );
}

function ServiceCard({ health, version }) {
  const degraded = health?.degraded;
  const dbOk = health?.dbReachable;
  return (
    <div className="adm-card">
      <div className="adm-card-head">
        <I.Server size={16} /> 服务状态
      </div>
      <div className="adm-kv">
        <span className="k">服务</span>
        <span className="v">
          <span className={'dot' + (degraded ? ' bad' : '')}></span>
          {degraded ? '降级' : '正常'}
        </span>
        <span className="k">数据库</span>
        <span className="v">
          <span className={'dot' + (dbOk === false ? ' bad' : '')}></span>
          {dbOk === undefined ? '—' : dbOk ? 'ok' : '不可达'}
        </span>
        <span className="k">构建</span>
        <span className="v mono">
          {health?.build || version?.build || '—'}
          {health?.channel || version?.channel ? <Badge tone="warning">{health?.channel || version?.channel}</Badge> : null}
        </span>
        <span className="k">API 契约</span>
        <span className="v mono">
          {version ? 'v' + version.apiVersion + '.' + version.apiMinor + ' · minClient ' + version.minClient : '—'}
        </span>
        <span className="k">环境</span>
        <span className="v mono">{health?.env || '—'}</span>
        <span className="k">已运行</span>
        <span className="v">{fmtUptime(health?.uptimeSec)}</span>
      </div>
    </div>
  );
}

function RecentCalls({ logs }) {
  if (logs?.__err) {
    return <Notice kind="warn">日志读不到：{logs.__err.message}</Notice>;
  }
  const rows = logs?.logs || [];
  if (rows.length === 0) {
    return <div className="adm-empty">还没有调用记录</div>;
  }
  return (
    <div className="adm-table-wrap">
      <table className="adm-table">
        <thead>
          <tr>
            <th>时间</th>
            <th>类型</th>
            <th>模型</th>
            <th>Tokens</th>
            <th>耗时</th>
            <th>状态</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((l) => (
            <tr key={l.id}>
              <td className="mono">{fmtT(l.createdAt)}</td>
              <td>{l.endpoint}</td>
              <td className="mono">{l.model}</td>
              <td className="mono">
                {l.promptTokens} → {l.compTokens}
              </td>
              <td className="mono">{(l.durationMs / 1000).toFixed(1)}s</td>
              <td>
                <span className={'adm-pill ' + (l.status === 'ok' ? 'ok' : 'fail')}>
                  {l.status === 'ok' ? '成功' : l.error || '失败'}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
