// Daycore Admin — 服务配置 (.env) / 模型目录 (models.yaml) / OAuth (oauth.yaml)
(function () {
  'use strict';
  const { useState, useContext } = React;
  const UI = window.DaycoreUI;
  const IC = window.DcIcons;
  const A = window.AdminStore;
  const { useAdmin, ToastCtx, AdmConfirm, Seg } = window.AdminShell;

  const Sw = ({ on, onClick, label }) => <button type="button" className={'dc-switch' + (on ? ' is-on' : '')} role="switch" aria-checked={on} aria-label={label} onClick={onClick} />;
  const F = ({ label, hint, children }) => (
    <div className="adm-f">
      <label>{label}</label>
      {children}
      {hint ? <p className="hint">{hint}</p> : null}
    </div>
  );
  function Drawer({ title, onClose, children }) {
    return (
      <React.Fragment>
        <div className="adm-scrim" onClick={onClose}></div>
        <div className="adm-drawer">
          <div className="adm-drawer-head">
            <h3>{title}</h3>
            <button type="button" className="adm-x" onClick={onClose}><IC.X size={17} /></button>
          </div>
          {children}
        </div>
      </React.Fragment>
    );
  }
  const kTok = (n) => (n >= 1000 ? Math.round(n / 1024) + 'k' : String(n));

  // ---------- 服务配置 ----------
  function ConfigView() {
    useAdmin();
    const toast = useContext(ToastCtx);
    const [ovr, setOvr] = useState({});
    const dirty = Object.keys(ovr).length;
    const val = (k) => (k in ovr ? ovr[k] : A.envValue(k));
    const set = (k, v) => setOvr((o) => {
      const n = { ...o };
      const d = A.envDef(k);
      if (d.type !== 'secret' && v === A.envValue(k)) delete n[k]; else n[k] = v;
      return n;
    });
    function save() {
      const r = A.setEnvMany(ovr);
      setOvr({});
      toast(r.needsRestart.length ? '已保存；' + r.needsRestart.join('、') + ' 需重启服务生效' : '已保存，热加载生效');
    }
    return (
      <div data-screen-label="服务配置">
        <header className="adm-head">
          <div>
            <h1 className="adm-title">服务配置</h1>
            <p className="adm-sub">.env 覆盖层，真实环境变量始终优先 · <span className="mono">GET / PUT /api/admin/config</span></p>
          </div>
        </header>
        {dirty ? (
          <div className="adm-savebar">
            <IC.AlertTriangle size={15} />
            <span>{dirty} 项未保存{Object.keys(ovr).some((k) => A.envDef(k).restart) ? ' · 含需重启项' : ''}</span>
            <span className="sp"></span>
            <UI.Button variant="ghost" onClick={() => setOvr({})}>放弃</UI.Button>
            <UI.Button variant="primary" onClick={save}>保存修改</UI.Button>
          </div>
        ) : null}
        <div className="adm-env-stack">
          {A.envGroups().map((g) => (
            <div key={g.id} className="adm-card">
              <div className="adm-card-head"><IC.Sliders size={15} /> {g.label}</div>
              {g.items.map((it) => {
                const isSet = A.envIsSet(it.key) || (it.key in ovr && ovr[it.key] !== '');
                return (
                  <div key={it.key} className="adm-env-row">
                    <div className="adm-env-main">
                      <div className="adm-env-key mono">{it.key}
                        {it.restart ? <span className="adm-pill anon" title="修改后需重启服务">↻ 重启</span> : null}
                        {it.type === 'secret' ? <span className={'adm-pill ' + (isSet ? 'ok' : 'anon')}>{isSet ? '已设置' : '未设置'}</span> : null}
                      </div>
                      <div className="adm-env-desc">{it.d}</div>
                    </div>
                    <div className="adm-env-ctl">
                      {it.type === 'select' ? (
                        <select className="adm-sel" value={val(it.key)} onChange={(e) => set(it.key, e.target.value)}>
                          {it.opts.map((o) => <option key={o} value={o}>{o}</option>)}
                        </select>
                      ) : it.type === 'bool' ? (
                        <Sw on={val(it.key) === true || val(it.key) === 'true'} label={it.key} onClick={() => set(it.key, !(val(it.key) === true || val(it.key) === 'true'))} />
                      ) : it.type === 'secret' ? (
                        <UI.Input type="password" placeholder={isSet ? '••••••••（输入以替换）' : '未设置'} value={it.key in ovr ? ovr[it.key] : ''} onChange={(e) => set(it.key, e.target.value)} />
                      ) : (
                        <UI.Input value={val(it.key)} inputMode={it.type === 'number' ? 'numeric' : undefined} onChange={(e) => set(it.key, e.target.value)} />
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          ))}
        </div>
        <p className="adm-note">密钥类值不回显（仅返回 set 标记）；演示环境改动只存在本机。</p>
      </div>
    );
  }

  // ---------- 模型目录 ----------
  const FORMATS = [{ value: 'openai', label: 'openai' }, { value: 'anthropic', label: 'anthropic' }, { value: 'ollama', label: 'ollama' }];
  const EMPTY_MODEL = { id: '', format: 'openai', base_url: '', model: '', api_key_env: '', vision: false, tools: false, stream: true, context_window: 65536 };
  function ModelsView() {
    useAdmin();
    const toast = useContext(ToastCtx);
    const [edit, setEdit] = useState(null);      // {m, isNew}
    const [del, setDel] = useState(null);
    const defs = A.modelDefaults();
    const badge = (id) => {
      const b = [];
      if (id === defs.chat) b.push('默认对话');
      if (id === defs.vision) b.push(defs.visionExplicit ? '默认视觉' : '默认视觉（自动）');
      if (id === defs.planner && defs.plannerExplicit) b.push('默认规划');
      return b;
    };
    function saveDraft() {
      const m = { ...edit.m, context_window: parseInt(edit.m.context_window, 10) || 0 };
      const r = A.saveModel(m, edit.isNew);
      if (r.error === 'id_required') return toast('id 不能为空');
      if (r.error === 'id_exists') return toast('id 已存在');
      setEdit(null); toast('已保存 models.yaml');
    }
    const setM = (patch) => setEdit((e) => ({ ...e, m: { ...e.m, ...patch } }));
    return (
      <div data-screen-label="模型目录">
        <header className="adm-head">
          <div>
            <h1 className="adm-title">模型</h1>
            <p className="adm-sub">config/models.yaml · 默认模型在「服务配置」中指定 · <span className="mono">GET / PUT /api/admin/models</span></p>
          </div>
          <UI.Button variant="primary" onClick={() => setEdit({ m: { ...EMPTY_MODEL }, isNew: true })}><IC.Plus size={15} /> 新增模型</UI.Button>
        </header>
        <div className="adm-model-grid">
          {A.modelsList().map((m) => (
            <button key={m.id} type="button" className="adm-model-card" onClick={() => setEdit({ m: { ...m }, isNew: false })}>
              <div className="adm-model-top">
                <span className="adm-model-id mono">{m.id}</span>
                <span className="adm-pill anon mono">{m.format}</span>
              </div>
              {badge(m.id).length ? <div className="adm-model-badges">{badge(m.id).map((b) => <span key={b} className="adm-pill reg">{b}</span>)}</div> : null}
              <div className="adm-mrow"><span>base_url</span><span className="mono">{m.base_url}</span></div>
              <div className="adm-mrow"><span>model</span><span className="mono">{m.model || m.id}</span></div>
              <div className="adm-mrow"><span>api_key</span><span className="mono">{m.api_key_env ? m.api_key_env + (A.envIsSet(m.api_key_env) ? ' ✓' : ' ✗ 未设置') : '—'}</span></div>
              <div className="adm-mrow"><span>context</span><span className="mono">{kTok(m.context_window)}</span></div>
              <div className="adm-flags">
                {[['vision', m.vision], ['tools', m.tools], ['stream', m.stream]].map(([f, on]) => (
                  <span key={f} className={'adm-flag' + (on ? ' is-on' : '')}>{f}</span>
                ))}
              </div>
            </button>
          ))}
        </div>
        {edit ? (
          <Drawer title={edit.isNew ? '新增模型' : '编辑 · ' + edit.m.id} onClose={() => setEdit(null)}>
            <F label="id" hint="在 DEFAULT_*_MODEL 中引用的名字">
              <UI.Input value={edit.m.id} disabled={!edit.isNew} onChange={(e) => setM({ id: e.target.value.trim() })} />
            </F>
            <F label="format" hint="wire 格式；新增格式需在 internal/ai/formats 注册后重编译">
              <Seg value={edit.m.format} onChange={(v) => setM({ format: v })} options={FORMATS} />
            </F>
            <F label="base_url"><UI.Input value={edit.m.base_url} onChange={(e) => setM({ base_url: e.target.value })} /></F>
            <F label="model" hint="上游模型名；留空 = 同 id"><UI.Input value={edit.m.model} onChange={(e) => setM({ model: e.target.value })} /></F>
            <F label="api_key_env" hint="引用「服务配置 → API 密钥」中的变量名"><UI.Input value={edit.m.api_key_env} onChange={(e) => setM({ api_key_env: e.target.value })} /></F>
            <F label="context_window"><UI.Input inputMode="numeric" value={String(edit.m.context_window)} onChange={(e) => setM({ context_window: e.target.value })} /></F>
            <div className="adm-caprow"><span>vision — 可读图片</span><Sw on={edit.m.vision} label="vision" onClick={() => setM({ vision: !edit.m.vision })} /></div>
            <div className="adm-caprow"><span>tools — 支持工具调用</span><Sw on={edit.m.tools} label="tools" onClick={() => setM({ tools: !edit.m.tools })} /></div>
            <div className="adm-caprow"><span>stream — 流式输出</span><Sw on={edit.m.stream} label="stream" onClick={() => setM({ stream: !edit.m.stream })} /></div>
            <div className="adm-drawer-actions">
              {!edit.isNew ? <UI.Button variant="outline" onClick={() => { setDel(edit.m); setEdit(null); }}><IC.Trash size={15} /> 删除</UI.Button> : null}
              <span className="sp"></span>
              <UI.Button variant="primary" onClick={saveDraft}>保存</UI.Button>
            </div>
          </Drawer>
        ) : null}
        <AdmConfirm open={!!del} onClose={() => setDel(null)} title={'删除模型 ' + (del ? del.id : '')}
          desc="从 models.yaml 中移除；引用它的 DEFAULT_* 配置会失效。"
          confirmLabel="删除" onConfirm={() => { const r = A.deleteModel(del.id); toast(r.error === 'is_default' ? '该模型是当前默认，请先在服务配置中更换' : '已删除'); }} />
      </div>
    );
  }

  // ---------- OAuth ----------
  const EMPTY_PROVIDER = { name: '', preset: false, client_id: '', client_secret: '', auth_url: '', token_url: '', userinfo_url: '', scopes: '', id_field: 'id', email_field: 'email', name_field: 'name', avatar_field: 'avatar_url' };
  function OAuthView() {
    useAdmin();
    const toast = useContext(ToastCtx);
    const [edit, setEdit] = useState(null);
    const [del, setDel] = useState(null);
    const base = A.envValue('PUBLIC_BASE_URL') || 'https://your.domain';
    const cb = (name) => base + '/api/auth/oauth/' + (name || '<name>') + '/callback';
    const setP = (patch) => setEdit((e) => ({ ...e, p: { ...e.p, ...patch } }));
    function saveDraft() {
      const r = A.saveProvider(edit.p, edit.isNew);
      if (r.error === 'bad_name') return toast('name 需为小写字母开头的 slug');
      if (r.error === 'name_exists') return toast('该 provider 已存在');
      setEdit(null); toast('已保存 oauth.yaml');
    }
    return (
      <div data-screen-label="OAuth">
        <header className="adm-head">
          <div>
            <h1 className="adm-title">OAuth 登录</h1>
            <p className="adm-sub">config/oauth.yaml · google / github 为预设，其余需填全字段 · <span className="mono">GET / PUT /api/admin/oauth</span></p>
          </div>
          <UI.Button variant="primary" onClick={() => setEdit({ p: { ...EMPTY_PROVIDER }, isNew: true })}><IC.Plus size={15} /> 自定义 Provider</UI.Button>
        </header>
        <div className="adm-model-grid">
          {A.oauthList().map((p) => {
            const ok = p.client_id && p.client_secret;
            return (
              <button key={p.name} type="button" className="adm-model-card" onClick={() => setEdit({ p: { scopes: '', ...p }, isNew: false })}>
                <div className="adm-model-top">
                  <span className="adm-model-id mono">{p.name}</span>
                  <span style={{ display: 'inline-flex', gap: 6 }}>
                    {p.preset ? <span className="adm-pill anon">预设</span> : <span className="adm-pill anon">自定义</span>}
                    <span className={'adm-pill ' + (ok ? 'ok' : 'anon')}>{ok ? '已配置' : '未配置'}</span>
                  </span>
                </div>
                <div className="adm-mrow"><span>client_id</span><span className="mono">{p.client_id ? p.client_id.slice(0, 18) + (p.client_id.length > 18 ? '…' : '') : '—'}</span></div>
                <div className="adm-mrow"><span>secret</span><span className="mono">{p.client_secret ? '••••••••' : '—'}</span></div>
                <div className="adm-cb"><span>回调</span><span className="mono">{cb(p.name)}</span></div>
              </button>
            );
          })}
        </div>
        <p className="adm-note">回调 URL 恒为 PUBLIC_BASE_URL + /api/auth/oauth/&lt;name&gt;/callback，请在提供方后台登记。</p>
        {edit ? (
          <Drawer title={edit.isNew ? '自定义 Provider' : '配置 · ' + edit.p.name} onClose={() => setEdit(null)}>
            <F label="name" hint="小写 slug；决定回调路径"><UI.Input value={edit.p.name} disabled={!edit.isNew} onChange={(e) => setP({ name: e.target.value.trim() })} /></F>
            <F label="client_id"><UI.Input value={edit.p.client_id} onChange={(e) => setP({ client_id: e.target.value })} /></F>
            <F label="client_secret"><UI.Input type="password" placeholder={edit.p.client_secret ? '••••••••（输入以替换）' : ''} value={edit.p.client_secret} onChange={(e) => setP({ client_secret: e.target.value })} /></F>
            <div className="adm-cbbox">
              <span className="mono">{cb(edit.p.name)}</span>
              <UI.IconButton label="复制" onClick={() => { try { navigator.clipboard.writeText(cb(edit.p.name)); } catch (e) {} toast('已复制回调 URL'); }}><IC.Copy size={14} /></UI.IconButton>
            </div>
            {edit.p.preset ? (
              <p className="adm-note" style={{ marginTop: 14 }}>预设 provider：URL 与字段映射自动填充，只需 client_id / client_secret。</p>
            ) : (
              <React.Fragment>
                <F label="auth_url"><UI.Input value={edit.p.auth_url} onChange={(e) => setP({ auth_url: e.target.value })} /></F>
                <F label="token_url"><UI.Input value={edit.p.token_url} onChange={(e) => setP({ token_url: e.target.value })} /></F>
                <F label="userinfo_url"><UI.Input value={edit.p.userinfo_url} onChange={(e) => setP({ userinfo_url: e.target.value })} /></F>
                <F label="scopes" hint="逗号分隔"><UI.Input value={edit.p.scopes} onChange={(e) => setP({ scopes: e.target.value })} /></F>
                <div className="adm-two" style={{ marginBottom: 0 }}>
                  <F label="id_field"><UI.Input value={edit.p.id_field} onChange={(e) => setP({ id_field: e.target.value })} /></F>
                  <F label="email_field"><UI.Input value={edit.p.email_field} onChange={(e) => setP({ email_field: e.target.value })} /></F>
                  <F label="name_field"><UI.Input value={edit.p.name_field} onChange={(e) => setP({ name_field: e.target.value })} /></F>
                  <F label="avatar_field"><UI.Input value={edit.p.avatar_field} onChange={(e) => setP({ avatar_field: e.target.value })} /></F>
                </div>
              </React.Fragment>
            )}
            <div className="adm-drawer-actions">
              {!edit.isNew && !edit.p.preset ? <UI.Button variant="outline" onClick={() => { setDel(edit.p); setEdit(null); }}><IC.Trash size={15} /> 删除</UI.Button> : null}
              <span className="sp"></span>
              <UI.Button variant="primary" onClick={saveDraft}>保存</UI.Button>
            </div>
          </Drawer>
        ) : null}
        <AdmConfirm open={!!del} onClose={() => setDel(null)} title={'删除 provider ' + (del ? del.name : '')}
          desc="从 oauth.yaml 中移除，已用它登录的账号将无法再以此方式登录。"
          confirmLabel="删除" onConfirm={() => { A.deleteProvider(del.name); toast('已删除'); }} />
      </div>
    );
  }

  Object.assign(window.AdminViews, { ConfigView, ModelsView, OAuthView });
})();
