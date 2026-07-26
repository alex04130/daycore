// Ported 1:1 from app/page-settings.jsx, plus the §7 memory section (facts
// list, delete one, clear all) and the About line showing the backend build
// version + beta channel badge.
import React, { useState, useEffect, useRef } from 'react';
import UI from '../boot/ds.js';
import IC from '../icons.js';
import S from '../store.js';
import I18N, { t } from '../i18n.js';
import { useStore, Sheet, Confirm, Field, Segmented, useToast, applyThemeVars, applyCurrentTheme } from '../ui.jsx';

const BUILTIN = [
  { id: 'sky', label: 'theme_sky', sw: ['#38bdf8', '#e0f2fe', '#bae6fd'] },
  { id: 'sunset', label: 'theme_sunset', sw: ['#fb923c', '#ffedd5', '#fed7aa'] },
  { id: 'night', label: 'theme_night', sw: ['#a78bfa', '#1e1b4b', '#312e81'], dark: true },
  { id: 'nature', label: 'theme_nature', sw: ['#4ade80', '#dcfce7', '#bbf7d0'] },
];

// ---------- Auth sheet ----------
export function AuthSheet({ open, onClose }) {
  const toast = useToast();
  const [mode, setMode] = useState('login');
  const [f, setF] = useState({ email: '', password: '', name: '' });
  const [err, setErr] = useState(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => { if (open) { setMode('login'); setF({ email: '', password: '', name: '' }); setErr(null); } }, [open]);
  async function submit() {
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(f.email)) { setErr('au_err_email'); return; }
    if (f.password.length < 8) { setErr('au_err_password'); return; }
    setErr(null); setBusy(true);
    try {
      const u = mode === 'login' ? await S.login(f) : await S.register(f);
      toast(t('au_welcome_toast', { name: u.name || u.email }));
      onClose();
    } catch (e) {
      setErr(e.code === 'invalid_credentials' ? 'au_err_password' : e.code === 'email_taken' ? 'au_err_email' : null);
      if (!e.code) toast(t('err_generic'));
    } finally {
      setBusy(false);
    }
  }
  function oauth(p) {
    // Full-page redirect: the backend drives the OAuth dance and sets cookies.
    window.location.href = '/api/auth/oauth/' + p;
  }
  return (
    <Sheet open={open} onClose={onClose} title={mode === 'login' ? t('au_signin') : t('au_register')}>
      <div className="dc-oauth-row">
        <UI.Button variant="outline" fullWidth onClick={() => oauth('google')} disabled={busy}>
          <svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true"><path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.27-4.74 3.27-8.1z"/><path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84A11 11 0 0 0 12 23z"/><path fill="#FBBC05" d="M5.84 14.1a6.6 6.6 0 0 1 0-4.2V7.06H2.18a11 11 0 0 0 0 9.88l3.66-2.84z"/><path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15A11 11 0 0 0 2.18 7.06l3.66 2.84c.87-2.6 3.3-4.52 6.16-4.52z"/></svg>
          {t('au_oauth_google')}
        </UI.Button>
        <UI.Button variant="outline" fullWidth onClick={() => oauth('github')} disabled={busy}>
          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 .5C5.65.5.5 5.65.5 12a11.5 11.5 0 0 0 7.86 10.91c.58.1.79-.25.79-.55v-2.17c-3.2.7-3.87-1.36-3.87-1.36-.53-1.33-1.28-1.69-1.28-1.69-1.05-.72.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.55-.29-5.23-1.28-5.23-5.68 0-1.26.45-2.28 1.19-3.09-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.18 1.18a11.05 11.05 0 0 1 5.78 0c2.2-1.49 3.17-1.18 3.17-1.18.63 1.59.24 2.76.12 3.05.74.8 1.18 1.83 1.18 3.09 0 4.41-2.68 5.38-5.24 5.66.41.36.78 1.05.78 2.13v3.16c0 .3.2.66.8.55A11.5 11.5 0 0 0 23.5 12C23.5 5.65 18.35.5 12 .5z"/></svg>
          {t('au_oauth_github')}
        </UI.Button>
      </div>
      <div className="dc-auth-divider">{t('au_or')}</div>
      {mode === 'register' ? (
        <Field label={t('au_name')}>
          <UI.Input placeholder={t('au_name_ph')} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} />
        </Field>
      ) : null}
      <Field label={t('au_email')}>
        <UI.Input type="email" placeholder="you@school.edu" value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} />
      </Field>
      <Field label={t('au_password')}>
        <UI.Input type="password" placeholder={t('au_password_ph')} value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} onKeyDown={(e) => e.key === 'Enter' && submit()} />
      </Field>
      {err ? <p className="dc-auth-err">{t(err)}</p> : null}
      <UI.Button variant="primary" size="lg" fullWidth disabled={busy} onClick={submit}>
        {mode === 'login' ? t('au_submit_login') : t('au_submit_register')}
      </UI.Button>
      <div className="dc-auth-switch">
        <button onClick={() => { setMode(mode === 'login' ? 'register' : 'login'); setErr(null); }}>
          {mode === 'login' ? t('au_switch_to_register') : t('au_switch_to_login')}
        </button>
      </div>
    </Sheet>
  );
}

// ---------- Account sheet (from avatar) ----------
export function AccountSheet({ open, onClose }) {
  const st = useStore();
  const toast = useToast();
  const [authOpen, setAuthOpen] = useState(false);
  const u = st.user;
  return (
    <React.Fragment>
      <Sheet open={open && !authOpen} onClose={onClose} title={t('au_account')}>
        <div className="dc-auth-head">
          <UI.Avatar name={u ? (u.name || u.email) : '?'} size={64} />
          <div>
            <div style={{ fontWeight: 700, fontSize: 17 }}>{u ? (u.name || u.email) : t('au_anon')}</div>
            <div className="dc-set-sub">{u ? u.email + ' · ' + t('au_synced') : t('au_anon_sub')}</div>
          </div>
        </div>
        {u ? (
          <UI.Button variant="outline" fullWidth onClick={async () => { await S.logout(); toast(t('au_signed_out')); onClose(); }}>
            <IC.LogOut size={16} /> {t('au_signout')}
          </UI.Button>
        ) : (
          <div className="dc-sheet-actions" style={{ marginTop: 0 }}>
            <UI.Button variant="primary" fullWidth onClick={() => setAuthOpen(true)}>{t('au_signin')} / {t('au_register')}</UI.Button>
          </div>
        )}
      </Sheet>
      <AuthSheet open={authOpen} onClose={() => { setAuthOpen(false); onClose(); }} />
    </React.Fragment>
  );
}

// ---------- Theme studio ----------
function swatchFor(th) {
  return { primary: th.variables['--primary'] || '#888', start: th.variables['--bg-start'] || '#eee', end: th.variables['--bg-end'] || '#ddd' };
}
function ThemeStudio() {
  const st = useStore();
  const toast = useToast();
  const cur = st.session.currentTheme;
  const [aiDesc, setAiDesc] = useState('');
  const [aiBase, setAiBase] = useState('');
  const [aiBusy, setAiBusy] = useState(false);
  const [aiErr, setAiErr] = useState(null);
  const [preview, setPreview] = useState(null); // {candidate, editingId, desc}
  const [menu, setMenu] = useState(null);       // {theme, x, y}
  const [rename, setRename] = useState(null);   // theme
  const [renameVal, setRenameVal] = useState('');
  const [confirmDel, setConfirmDel] = useState(null);
  const longPress = useRef(null);

  function applyBuiltin(id) {
    S.patchSession({ currentTheme: id });
    applyCurrentTheme(S.state);
    toast(t('st_theme_applied'));
  }
  function applyCustom(th) {
    S.patchSession({ currentTheme: th.id });
    applyCurrentTheme(S.state);
    toast(t('st_theme_applied'));
  }
  async function generate(editingId, descOverride) {
    const desc = descOverride !== undefined ? descOverride : aiDesc;
    if (!desc.trim()) return;
    setAiBusy(true); setAiErr(null);
    const res = await S.aiTheme(desc, aiBase || undefined, editingId);
    setAiBusy(false);
    if (res.error) { setAiErr(res.error); return; }
    setPreview({ candidate: res, editingId: editingId || null, desc });
    applyThemeVars({ dark: res.dark, base: res.base, variables: res.variables });
  }
  function cancelPreview() {
    setPreview(null);
    applyCurrentTheme(S.state);
  }
  async function savePreview() {
    const c = preview.candidate;
    try {
      let th;
      if (preview.editingId) th = await S.patchTheme(preview.editingId, { name: c.name, dark: c.dark, variables: c.variables });
      else th = await S.createTheme({ name: c.name, dark: c.dark, base: c.base, variables: c.variables });
      await S.patchSession({ currentTheme: th.id });
      setPreview(null);
      applyCurrentTheme(S.state);
      toast(t('st_theme_saved', { name: th.name }));
      setAiDesc('');
    } catch (e) {
      toast(t('err_generic'));
    }
  }
  function openMenu(e, th) {
    e.preventDefault();
    const x = Math.min((e.clientX || 100), window.innerWidth - 180);
    const y = Math.min((e.clientY || 200), window.innerHeight - 170);
    setMenu({ theme: th, x, y });
  }
  async function del(th) {
    await S.deleteTheme(th.id);
    applyCurrentTheme(S.state);
    toast(t('st_theme_deleted'));
  }
  useEffect(() => {
    if (!menu) return;
    const close = () => setMenu(null);
    window.addEventListener('click', close);
    window.addEventListener('scroll', close, true);
    return () => { window.removeEventListener('click', close); window.removeEventListener('scroll', close, true); };
  }, [menu]);

  return (
    <div>
      <div className="dc-theme-grid">
        {BUILTIN.map((b) => (
          <button key={b.id} className={'dc-theme-card' + (cur === b.id ? ' is-sel' : '')} onClick={() => applyBuiltin(b.id)}>
            <span className="dc-theme-swatch" style={{ background: 'linear-gradient(135deg, ' + b.sw[1] + ', ' + b.sw[2] + ')' }}>
              <span className="pill" style={{ background: b.sw[0] }} />
            </span>
            <span className="dc-theme-name">{t(b.label)}{b.dark ? <UI.Badge tone="neutral">{t('st_dark_badge')}</UI.Badge> : null}</span>
            {cur === b.id ? <span className="dc-theme-check"><IC.Check size={13} strokeWidth={3} /></span> : null}
          </button>
        ))}
        {st.customThemes.map((th) => {
          const sw = swatchFor(th);
          return (
            <button key={th.id} className={'dc-theme-card' + (cur === th.id ? ' is-sel' : '')}
              onClick={() => applyCustom(th)}
              onContextMenu={(e) => openMenu(e, th)}
              onTouchStart={(e) => { longPress.current = setTimeout(() => openMenu({ preventDefault() {}, clientX: e.touches[0].clientX, clientY: e.touches[0].clientY }, th), 550); }}
              onTouchEnd={() => clearTimeout(longPress.current)}
              onTouchMove={() => clearTimeout(longPress.current)}>
              <span className="dc-theme-swatch" style={{ background: 'linear-gradient(135deg, ' + sw.start + ', ' + sw.end + ')' }}>
                <span className="pill" style={{ background: sw.primary }} />
              </span>
              <span className="dc-theme-name">{th.name}{th.dark ? <UI.Badge tone="neutral">{t('st_dark_badge')}</UI.Badge> : null}</span>
              {cur === th.id ? <span className="dc-theme-check"><IC.Check size={13} strokeWidth={3} /></span> : null}
            </button>
          );
        })}
      </div>
      {st.customThemes.length ? <p className="dc-field-sub" style={{ margin: '10px 2px 0' }}>{t('st_theme_hint_menu')}</p> : null}

      <UI.GlassCard className="dc-mt-20">
        <div className="dc-row-8" style={{ marginBottom: 10 }}>
          <IC.Wand size={17} style={{ color: 'var(--color-primary)' }} />
          <span style={{ fontWeight: 650, fontSize: 14.5 }}>{t('st_ai_theme_title')}</span>
        </div>
        <UI.Input placeholder={t('st_ai_theme_ph')} value={aiDesc} onChange={(e) => setAiDesc(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && generate()} />
        <div className="dc-row-8" style={{ marginTop: 10, flexWrap: 'wrap' }}>
          <span className="dc-field-sub">{t('st_ai_base')}:</span>
          <UI.Chip variant={(aiBase === '') ? 'selected' : 'default'} onClick={() => setAiBase('')}>{t('st_ai_base_none')}</UI.Chip>
          {BUILTIN.map((b) => <UI.Chip key={b.id} variant={(aiBase === b.id) ? 'selected' : 'default'} onClick={() => setAiBase(b.id)}>{t(b.label)}</UI.Chip>)}
        </div>
        {aiErr ? <p className="dc-auth-err" style={{ margin: '10px 0 0' }}>{t(aiErr === 'not_a_theme' ? 'not_a_theme_err' : aiErr === 'rate_limited' ? 'err_rate_limited' : 'err_generic')}</p> : null}
        <div style={{ marginTop: 12 }}>
          <UI.Button variant="primary" fullWidth disabled={aiBusy || !aiDesc.trim()} onClick={() => generate()}>
            <IC.Sparkles size={16} /> {aiBusy ? t('st_generating') : t('st_ai_generate')}
          </UI.Button>
        </div>
      </UI.GlassCard>

      {preview ? (
        <div className="dc-preview-bar">
          <IC.Palette size={18} />
          <span className="dc-preview-name">{t('st_preview_name', { name: preview.candidate.name })}</span>
          <button className="dc-preview-btn" onClick={() => generate(preview.editingId, preview.desc)}>{t('st_preview_retry')}</button>
          <button className="dc-preview-btn" onClick={cancelPreview}>{t('st_preview_cancel')}</button>
          <button className="dc-preview-btn primary" onClick={savePreview}>{t('st_preview_save')}</button>
        </div>
      ) : null}

      {menu ? (
        <div className="dc-ctx-menu" style={{ left: menu.x, top: menu.y }}>
          <button className="dc-ctx-item" onClick={() => { setRename(menu.theme); setRenameVal(menu.theme.name); setMenu(null); }}><IC.Pencil size={15} /> {t('st_theme_menu_rename')}</button>
          <button className="dc-ctx-item" onClick={() => { const id = menu.theme.id; setMenu(null); setTimeout(() => { const d = prompt(t('st_ai_theme_ph')); if (d) { setAiDesc(d); generate(id, d); } }, 50); }}><IC.Wand size={15} /> {t('st_theme_menu_ai')}</button>
          <button className="dc-ctx-item danger" onClick={() => { setConfirmDel(menu.theme); setMenu(null); }}><IC.Trash size={15} /> {t('st_theme_menu_delete')}</button>
        </div>
      ) : null}

      <Sheet open={!!rename} onClose={() => setRename(null)} title={t('st_rename_title')}>
        <UI.Input value={renameVal} onChange={(e) => setRenameVal(e.target.value)} />
        <div className="dc-sheet-actions">
          <UI.Button variant="outline" fullWidth onClick={() => setRename(null)}>{t('cancel')}</UI.Button>
          <UI.Button variant="primary" fullWidth disabled={!renameVal.trim()} onClick={async () => { await S.patchTheme(rename.id, { name: renameVal.trim() }); setRename(null); toast(t('rl_saved')); }}>{t('save')}</UI.Button>
        </div>
      </Sheet>
      <Confirm open={!!confirmDel} onClose={() => setConfirmDel(null)}
        title={confirmDel ? t('st_theme_delete_title', { name: confirmDel.name }) : ''}
        body={confirmDel && cur === confirmDel.id ? t('st_theme_delete_inuse') : null}
        danger confirmLabel={t('del')} onConfirm={() => del(confirmDel)} />
    </div>
  );
}

// ---------- Memory section (§7) ----------
function MemorySection() {
  const st = useStore();
  const toast = useToast();
  const [confirmClear, setConfirmClear] = useState(false);
  const srcLabel = { chat: 'rl_source_chat', user: 'rl_source_user', import: 'mem_source_import' };
  return (
    <div className="dc-set-group">
      <UI.Label>{t('st_group_memory')}</UI.Label>
      <div className="dc-stack-8" style={{ marginTop: 10 }}>
        {!st.memoryFacts.length ? (
          <UI.GlassCard padding="none" className="dc-set-row" style={{ cursor: 'default' }}>
            <span className="dc-set-ic"><IC.Sparkles size={17} /></span>
            <div className="dc-set-main"><div className="dc-set-sub">{t('mem_empty')}</div></div>
          </UI.GlassCard>
        ) : st.memoryFacts.map((f) => (
          <UI.GlassCard key={f.id} padding="none" className="dc-asg-row">
            <span className="dc-set-ic"><IC.Sparkles size={16} /></span>
            <div className="dc-asg-main">
              <div className="dc-asg-title" style={{ fontSize: 13.5 }}>{f.fact}</div>
              <div className="dc-asg-meta"><span>{t(srcLabel[f.source] || 'rl_source_chat')}</span></div>
            </div>
            <div className="dc-asg-actions">
              <button className="dc-asg-icon-btn" title={t('del')} onClick={async () => { await S.deleteMemoryFact(f.id); toast(t('mem_deleted')); }}><IC.Trash size={15} /></button>
            </div>
          </UI.GlassCard>
        ))}
        {st.memoryFacts.length ? (
          <UI.Button variant="outline" fullWidth className="dc-muted" onClick={() => setConfirmClear(true)}>
            <IC.Trash size={15} /> {t('mem_clear')}
          </UI.Button>
        ) : null}
      </div>
      <Confirm open={confirmClear} onClose={() => setConfirmClear(false)} title={t('mem_clear_title')} body={t('mem_clear_body')} danger confirmLabel={t('mem_clear')}
        onConfirm={async () => { await S.clearMemory(); toast(t('mem_cleared')); }} />
    </div>
  );
}

// ---------- Settings page ----------
export default function SettingsPage({ nav }) {
  const st = useStore();
  const toast = useToast();
  const [nameDraft, setNameDraft] = useState(st.session.assistantName);
  useEffect(() => setNameDraft(st.session.assistantName), [st.session.assistantName]);
  return (
    <div className="dc-page-enter" data-screen-label="设置">
      <UI.SectionHeader title={t('st_title')} />

      <div className="dc-set-group dc-mt-20">
        <UI.Label>{t('st_group_assistant')}</UI.Label>
        <UI.GlassCard padding="none" className="dc-set-row" style={{ cursor: 'default' }}>
          <span className="dc-set-ic"><IC.Heart size={18} /></span>
          <div className="dc-set-main">
            <div className="dc-set-title">{t('st_assistant_name')}</div>
            <div className="dc-set-sub">{t('st_assistant_name_sub')}</div>
          </div>
          <UI.Input style={{ maxWidth: 130 }} value={nameDraft}
            onChange={(e) => setNameDraft(e.target.value)}
            onBlur={() => { if (nameDraft.trim() && nameDraft !== st.session.assistantName) { S.patchSession({ assistantName: nameDraft.trim() }); toast(t('rl_saved')); } }} />
        </UI.GlassCard>
      </div>

      <div className="dc-set-group">
        <UI.Label>{t('st_group_prefs')}</UI.Label>
        <UI.GlassCard padding="none" className="dc-set-row" style={{ cursor: 'default' }}>
          <span className="dc-set-ic"><IC.Globe size={18} /></span>
          <div className="dc-set-main">
            <div className="dc-set-title">{t('st_language')}</div>
            <div className="dc-set-sub">{t('st_language_sub')}</div>
          </div>
          <Segmented small value={st.session.language} onChange={(v) => { S.patchSession({ language: v }); I18N.setLang(v); }}
            options={[{ value: 'zh-CN', label: '中文' }, { value: 'en-US', label: 'EN' }]} />
        </UI.GlassCard>
      </div>

      <div className="dc-set-group">
        <UI.Label>{t('st_group_theme')}</UI.Label>
        <div style={{ marginTop: 10 }}><ThemeStudio /></div>
      </div>

      <MemorySection />

      <div className="dc-set-group">
        <UI.Label>{t('st_group_data')}</UI.Label>
        <div className="dc-stack-8" style={{ marginTop: 10 }}>
          <UI.GlassCard padding="none" className="dc-set-row" onClick={() => nav.go('materials')}>
            <span className="dc-set-ic"><IC.Layers size={18} /></span>
            <div className="dc-set-main"><div className="dc-set-title">{t('st_goto_materials')}</div></div>
            <IC.ChevronRight size={18} style={{ color: 'var(--color-text-muted)' }} />
          </UI.GlassCard>
          <UI.GlassCard padding="none" className="dc-set-row" onClick={() => { S.setOnboarded(false); }}>
            <span className="dc-set-ic"><IC.RefreshCw size={17} /></span>
            <div className="dc-set-main"><div className="dc-set-title">{t('st_replay_onboarding')}</div></div>
            <IC.ChevronRight size={18} style={{ color: 'var(--color-text-muted)' }} />
          </UI.GlassCard>
        </div>
      </div>

      <p className="dc-field-sub" style={{ textAlign: 'center', padding: '8px 0 20px' }}>
        {t('st_about_version', { v: st.version || '2.1.0' })}
        {st.channel === 'beta' ? <UI.Badge tone="primary" style={{ marginLeft: 8 }}>{t('st_beta_badge')}</UI.Badge> : null}
      </p>
    </div>
  );
}
