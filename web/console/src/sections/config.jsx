import { useCallback, useState } from 'react';
import * as api from '../api.js';
import * as I from '../icons.jsx';
import { Button, Input, Notice, useSection, useToast } from '../ui.jsx';

// 服务配置. The real backend is a FLAT settings list with a per-key layer
// (boot / runtime), not the prototype's .env domain groups — so the screen is
// grouped by that real axis. Secrets never return a value, only a `set` flag.
export function Config({ onUnauthorized }) {
  const toast = useToast();
  const fetcher = useCallback(() => api.getConfig(), []);
  const state = useSection(fetcher, { onUnauthorized });

  // draft: key -> string | null (null = reset to the environment seed).
  const [draft, setDraft] = useState({});
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  const settings = state.data ? state.data.settings : [];
  const boot = settings.filter(function (s) { return s.layer !== 'runtime'; });
  const runtime = settings.filter(function (s) { return s.layer === 'runtime'; });
  const dirty = Object.keys(draft).length;
  const hasRestart = Object.keys(draft).some(function (k) {
    const s = settings.find(function (x) { return x.key === k; });
    return !!(s && s.requiresRestart);
  });

  async function save() {
    setBusy(true);
    setErr('');
    try {
      const res = await api.putConfig(draft);
      setDraft({});
      const restart = (res && res.requiresRestart) || [];
      toast(restart.length ? '已保存；' + restart.join('、') + ' 需重启服务生效' : '已保存，热加载生效');
      state.reload();
    } catch (e) {
      if (e instanceof api.Unauthorized) return onUnauthorized && onUnauthorized();
      setErr(e.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div data-screen-label="服务配置">
      <header className="adm-head">
        <div>
          <h1 className="adm-title">服务配置</h1>
          <p className="adm-sub">
            每个旋钮分两层，真实环境变量始终优先 · <span className="mono">GET / PUT /api/admin/config</span>
          </p>
        </div>
      </header>

      {state.status === 'loading' && <div className="adm-empty">读取中…</div>}
      {state.status === 'error' && <Notice kind="error">{state.error}</Notice>}

      {dirty > 0 && (
        <div className="adm-savebar">
          <I.AlertTriangle size={15} />
          <span>
            {dirty} 项未保存{hasRestart ? ' · 含需重启项' : ''}
          </span>
          <span className="sp"></span>
          <Button variant="ghost" onClick={() => setDraft({})}>
            放弃
          </Button>
          <Button variant="primary" disabled={busy} onClick={save}>
            {busy ? '保存中…' : '保存修改'}
          </Button>
        </div>
      )}

      {err && <Notice kind="error">{err}</Notice>}

      {state.data && (
        <div className="adm-env-stack">
          <div className="adm-card">
            <div className="adm-card-head">
              <I.Sliders size={15} /> 运行时（可改，下次读就生效）
            </div>
            {runtime.map(function (s) { return <RuntimeRow key={s.key} setting={s} draft={draft} onChange={function (v) { return setDraft(function (d) { const n = Object.assign({}, d); n[s.key] = v; return n; }); }} />; })}
            {runtime.length === 0 && <div className="adm-empty">没有可运行时修改的旋钮</div>}
          </div>

          <div className="adm-card">
            <div className="adm-card-head">
              <I.Sliders size={15} /> 启动期（只读）
            </div>
            {boot.map(function (s) { return <BootRow key={s.key} setting={s} />; })}
          </div>
        </div>
      )}

      <p className="adm-note">密钥类值不回显（仅返回 set 标记）。</p>
    </div>
  );
}

function Pills({ setting }) {
  return (
    <>
      {setting.requiresRestart ? (
        <span className="adm-pill anon" title="修改后需重启服务">↻ 重启</span>
      ) : null}
      {setting.source === 'override' ? (
        <span className="adm-pill reg" title="这个值来自控制台的覆盖，不是环境变量">已覆盖</span>
      ) : null}
      {setting.secret ? (
        <span className={'adm-pill ' + (setting.set ? 'ok' : 'anon')}>{setting.set ? '已设置' : '未设置'}</span>
      ) : null}
    </>
  );
}

function RuntimeRow({ setting, draft, onChange }) {
  const staged = Object.prototype.hasOwnProperty.call(draft, setting.key);
  const value = staged ? draft[setting.key] : (setting.value === undefined ? '' : setting.value);
  const isReset = staged && draft[setting.key] === null;
  return (
    <div className="adm-env-row">
      <div className="adm-env-main">
        <div className="adm-env-key mono">
          {setting.env}
          <Pills setting={setting} />
        </div>
        <div className="adm-env-desc">{setting.why || ''}</div>
      </div>
      <div className="adm-env-ctl">
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6, width: '100%' }}>
          {isReset ? (
            <span className="dim" style={{ fontSize: 12.5 }}>将重置为环境变量的值</span>
          ) : (
            <Input value={value} onChange={function (e) { return onChange(e.target.value); }} spellCheck={false} />
          )}
          {setting.source === 'override' && (
            <button type="button" className="adm-back" style={{ alignSelf: 'flex-end', padding: 0 }} onClick={function () { return onChange(isReset ? (setting.value === undefined ? '' : setting.value) : null); }}>
              {isReset ? '取消' : '恢复为环境变量的值'}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

function BootRow({ setting }) {
  return (
    <div className="adm-env-row">
      <div className="adm-env-main">
        <div className="adm-env-key mono">
          {setting.env}
          <Pills setting={setting} />
        </div>
        <div className="adm-env-desc">{setting.why || (setting.secret ? '密钥，不回显' : '')}</div>
      </div>
      <div className="adm-env-ctl">
        {setting.secret ? null : (
          <span className="mono dim" style={{ fontSize: 13 }}>
            {setting.value || <span className="dim">（未设置）</span>}
          </span>
        )}
      </div>
    </div>
  );
}
