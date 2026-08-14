import { useCallback, useState } from 'react';
import * as api from '../api.js';
import { Notice, useSection, useToast } from '../ui.jsx';

const kTok = (n) => (n >= 1000 ? Math.round(n / 1024) + 'k' : String(n));

const ROLE_LABEL = { chat: '默认对话', vision: '默认视觉', planner: '默认规划' };

// The model catalogue. Read-only by design: the backend's GET /api/v2/admin/models
// returns the loaded Catalog, and there is no PUT/DELETE — an entry became a
// constructed provider at startup, so no write could take effect under a running
// process. The one action the backend DOES offer is a real connectivity probe
// (POST /models/{id}/test), which this screen exposes.
export function Models({ onUnauthorized }) {
  const toast = useToast();
  const load = useCallback(() => api.getModels(), []);
  const state = useSection(load, { onUnauthorized });
  const [testing, setTesting] = useState(null);
  const d = state.data;

  async function test(id) {
    setTesting(id);
    try {
      const r = await api.testModel(id);
      toast(r.message);
    } catch (e) {
      if (e instanceof api.Unauthorized) return onUnauthorized?.();
      toast(e.message);
    } finally {
      setTesting(null);
    }
  }

  const models = d?.models || [];

  return (
    <div data-screen-label="模型目录">
      <header className="adm-head">
        <div>
          <h1 className="adm-title">模型</h1>
          <p className="adm-sub">
            {d?.configPath || 'config/models.yaml'} · 默认模型在「服务配置」中指定 · <span className="mono">GET /api/admin/models</span>
          </p>
        </div>
      </header>

      {state.status === 'loading' && <div className="adm-empty">读取中…</div>}
      {state.status === 'error' && <Notice kind="error">{state.error}</Notice>}

      {d && (
        <>
          <div className="adm-model-grid">
            {models.map((m) => {
              const badges = (m.roles || []).map((r) => ROLE_LABEL[r]).filter(Boolean);
              return (
                <div key={m.id} className="adm-model-card">
                  <div className="adm-model-top">
                    <span className="adm-model-id mono">{m.id}</span>
                    <span style={{ display: 'inline-flex', gap: 6, alignItems: 'center' }}>
                      <span className="adm-pill anon mono">{m.format}</span>
                      <button
                        type="button"
                        className="adm-pill reg"
                        style={{ border: 'none', cursor: 'pointer', font: 'inherit' }}
                        onClick={() => test(m.id)}
                        disabled={testing === m.id}
                      >
                        {testing === m.id ? '测试中…' : '测试'}
                      </button>
                    </span>
                  </div>
                  {badges.length ? (
                    <div className="adm-model-badges">
                      {badges.map((b) => (
                        <span key={b} className="adm-pill reg">
                          {b}
                        </span>
                      ))}
                    </div>
                  ) : null}
                  <div className="adm-mrow">
                    <span>base_url</span>
                    <span className="mono">{m.baseUrl}</span>
                  </div>
                  <div className="adm-mrow">
                    <span>model</span>
                    <span className="mono">{m.model || m.id}</span>
                  </div>
                  <div className="adm-mrow">
                    <span>api_key</span>
                    <span className="mono">
                      {m.apiKeyEnv ? m.apiKeyEnv + (m.keySet ? ' ✓' : ' ✗ 未设置') : '—'}
                    </span>
                  </div>
                  <div className="adm-mrow">
                    <span>context</span>
                    <span className="mono">{kTok(m.caps?.ContextWindow || 0)}</span>
                  </div>
                  <div className="adm-flags">
                    {[
                      ['vision', m.caps?.Vision],
                      ['tools', m.caps?.Tools],
                    ].map(([f, on]) => (
                      <span key={f} className={'adm-flag' + (on ? ' is-on' : '')}>
                        {f}
                      </span>
                    ))}
                  </div>
                </div>
              );
            })}
          </div>
          <p className="adm-note">
            这一屏只读：模型条目在启动时已经建成了 provider，运行时改不了。要改就在 {d?.configPath || 'config/models.yaml'}{' '}
            里编辑再重启；「测试」发一次真调用（max_tokens=1）验证 key 与连通性。
          </p>
        </>
      )}
    </div>
  );
}
