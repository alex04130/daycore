import { useCallback } from 'react';
import * as api from '../api.js';
import { Notice, useSection } from '../ui.jsx';

// OAuth providers. Read-only: the backend's GET /api/v2/admin/oauth returns
// enabled providers plus each one's callback URL, and there is no PUT/DELETE —
// a provider became a configured flow with its redirect URI registered at the
// vendor, so no write could take effect under a running process.
export function OAuth({ onUnauthorized }) {
  const load = useCallback(() => api.getOAuth(), []);
  const state = useSection(load, { onUnauthorized });
  const d = state.data;
  const providers = d?.providers || [];

  return (
    <div data-screen-label="OAuth">
      <header className="adm-head">
        <div>
          <h1 className="adm-title">OAuth 登录</h1>
          <p className="adm-sub">
            {d?.configPath || 'config/oauth.yaml'} · <span className="mono">GET /api/admin/oauth</span>
          </p>
        </div>
      </header>

      {state.status === 'loading' && <div className="adm-empty">读取中…</div>}
      {state.status === 'error' && <Notice kind="error">{state.error}</Notice>}

      {d && (
        <>
          {providers.length === 0 ? (
            <div className="adm-empty">没有已启用的第三方登录（client_id 为空的 provider 启动时被跳过）</div>
          ) : (
            <div className="adm-model-grid">
              {providers.map((p) => {
                const ok = p.clientId && p.secretSet;
                return (
                  <div key={p.name} className="adm-model-card">
                    <div className="adm-model-top">
                      <span className="adm-model-id mono">{p.name}</span>
                      <span style={{ display: 'inline-flex', gap: 6 }}>
                        <span className="adm-pill anon">{p.preset ? '预设' : '自定义'}</span>
                        <span className={'adm-pill ' + (ok ? 'ok' : 'anon')}>{ok ? '已配置' : '未配置'}</span>
                      </span>
                    </div>
                    <div className="adm-mrow">
                      <span>client_id</span>
                      <span className="mono">
                        {p.clientId ? p.clientId.slice(0, 18) + (p.clientId.length > 18 ? '…' : '') : '—'}
                      </span>
                    </div>
                    <div className="adm-mrow">
                      <span>secret</span>
                      <span className="mono">{p.secretSet ? '••••••••' : '—'}</span>
                    </div>
                    <div className="adm-cb">
                      <span>回调</span>
                      <span className="mono">{p.redirectUri}</span>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
          <p className="adm-note">
            回调 URL 恒为 PUBLIC_BASE_URL + /api/auth/oauth/&lt;name&gt;/callback，请在提供方后台登记。{d.note ? ' ' + d.note : ''}
          </p>
        </>
      )}
    </div>
  );
}
