// Daycore v2.2 — Settings: assistant, language, proactive care, capture categories, channels, memory, themes, about
(function () {
  'use strict';
  const { useState, useEffect } = React;
  const UI = window.DaycoreUI;
  const IC = window.DcIcons;
  const S = window.DcStore;
  const { t } = window.I18N;
  const { useStore, Sheet, Confirm, Field, Segmented, useToast } = window.DcUI;
  const SP = window.DcSettingsParts;
  const CF = window.DcCapture;

  const PREFS = [
    { key: 'morningBrief', icon: IC.Sun },
    { key: 'eveningReview', icon: IC.Moon },
    { key: 'deadlineAlerts', icon: IC.Bell },
    { key: 'rollingReplan', icon: IC.RefreshCw },
    { key: 'gapSuggestions', icon: IC.Sparkles },
    { key: 'autoPlan', icon: IC.Zap },
    { key: 'doNotDisturb', icon: IC.EyeOff },
  ];

  function Switch({ on, onToggle, label }) {
    return <button className={'dc-switch' + (on ? ' is-on' : '')} role="switch" aria-checked={on} aria-label={label} onClick={onToggle} />;
  }

  // ---------- channels ----------
  function ChannelBindSheet({ channel, open, onClose }) {
    const st = useStore();
    const toast = useToast();
    const [token, setToken] = useState('');
    useEffect(() => { if (open && channel) setToken(S.bindChannel(channel).token); }, [open, channel]);
    if (!channel) return null;
    const name = t('ch_' + channel);
    return (
      <Sheet open={open} onClose={onClose} title={t('ch_token_title', { name })}>
        <p className="dc-confirm-body">{t('ch_token_desc', { name })}</p>
        <div className="dc-token-box">
          <IC.Key size={16} style={{ color: 'var(--color-primary)', flexShrink: 0 }} />
          <span style={{ flex: 1, letterSpacing: '0.08em', fontWeight: 700 }}>{token}</span>
          <UI.IconButton label={t('copy')} onClick={() => { try { navigator.clipboard.writeText(token); } catch (e) {} toast(t('copied')); }}><IC.Copy size={16} /></UI.IconButton>
        </div>
        <p className="dc-field-sub" style={{ margin: '10px 2px 0' }}>{t('ch_token_expires')}</p>
        <div className="dc-sheet-actions">
          <UI.Button variant="outline" fullWidth onClick={onClose}>{t('cancel')}</UI.Button>
          <UI.Button variant="primary" fullWidth onClick={() => { S.verifyChannel(channel); toast(t('ch_bound_toast', { name })); onClose(); }}>
            <IC.Check size={15} /> {t('ch_simulate')}
          </UI.Button>
        </div>
      </Sheet>
    );
  }

  function ChannelsGroup() {
    const st = useStore();
    const toast = useToast();
    const [bindFor, setBindFor] = useState(null);
    const [confirmUnbind, setConfirmUnbind] = useState(null);
    const icons = { qq: IC.MessageCircle, telegram: IC.Send };
    return (
      <div className="dc-stack-8" style={{ marginTop: 10 }}>
        {st.channels.map((ch) => {
          const Icon = icons[ch.id] || IC.Link;
          const name = t('ch_' + ch.id);
          return (
            <UI.GlassCard key={ch.id} padding="none" className="dc-set-row" style={{ cursor: 'default' }}>
              <span className="dc-set-ic"><Icon size={18} /></span>
              <div className="dc-set-main">
                <div className="dc-set-title dc-row-8">{name} {ch.bound ? <UI.Badge tone="success">{t('ch_bound')}</UI.Badge> : null}</div>
                <div className="dc-set-sub">{ch.bound ? ch.externalId : t('ch_unbound_sub')}</div>
              </div>
              {ch.bound
                ? <UI.Button size="sm" variant="outline" onClick={() => setConfirmUnbind(ch)}>{t('ch_unbind')}</UI.Button>
                : <UI.Button size="sm" variant="secondary" onClick={() => setBindFor(ch.id)}>{t('ch_bind')}</UI.Button>}
            </UI.GlassCard>
          );
        })}
        <p className="dc-field-sub" style={{ margin: '2px 2px 0' }}>{t('st_care_note')}</p>
        <ChannelBindSheet channel={bindFor} open={!!bindFor} onClose={() => setBindFor(null)} />
        <Confirm open={!!confirmUnbind} onClose={() => setConfirmUnbind(null)}
          title={confirmUnbind ? t('ch_unbind') + ' · ' + t('ch_' + confirmUnbind.id) : ''}
          body={confirmUnbind ? t('ch_unbind_confirm', { name: t('ch_' + confirmUnbind.id) }) : ''}
          danger confirmLabel={t('ch_unbind')}
          onConfirm={() => { S.unbindChannel(confirmUnbind.id); toast(t('ch_unbound_toast')); }} />
      </div>
    );
  }

  // ---------- memory ----------
  function MemoryGroup() {
    const st = useStore();
    const toast = useToast();
    const [draft, setDraft] = useState('');
    const [confirmClear, setConfirmClear] = useState(false);
    const srcKey = { chat: 'mem_source_chat', user: 'mem_source_user', import: 'mem_source_import' };
    return (
      <div style={{ marginTop: 10 }}>
        <div className="dc-stack-8">
          {!st.memory.length ? <UI.EmptyState icon={<IC.Brain size={24} />} title={t('mem_empty')} /> :
            st.memory.map((f) => (
              <UI.GlassCard key={f.id} padding="none" className="dc-mem-row">
                <span className="dc-set-ic" style={{ width: 32, height: 32 }}><IC.Brain size={15} /></span>
                <div className="dc-set-main">
                  <div className="dc-mem-fact">{f.fact}</div>
                  <div className="dc-set-sub"><UI.Badge tone="neutral">{t(srcKey[f.source] || 'mem_source_user')}</UI.Badge></div>
                </div>
                <button className="dc-asg-icon-btn" aria-label={t('del')} onClick={() => { S.deleteMemoryFact(f.id); toast(t('mem_deleted')); }}><IC.Trash size={15} /></button>
              </UI.GlassCard>
            ))}
        </div>
        <div className="dc-mem-add">
          <UI.Input placeholder={t('mem_add_ph')} value={draft} onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter' && draft.trim()) { S.addMemoryFact(draft.trim(), 'user'); setDraft(''); toast(t('rl_saved')); } }} />
          <UI.Button variant="secondary" disabled={!draft.trim()} onClick={() => { S.addMemoryFact(draft.trim(), 'user'); setDraft(''); toast(t('rl_saved')); }}>{t('mem_add')}</UI.Button>
        </div>
        {st.memory.length ? (
          <UI.Button variant="outline" fullWidth className="dc-muted" style={{ marginTop: 8 }} onClick={() => setConfirmClear(true)}>
            <IC.Trash size={15} /> {t('mem_clear')}
          </UI.Button>
        ) : null}
        <Confirm open={confirmClear} onClose={() => setConfirmClear(false)} title={t('mem_clear_confirm_title')} body={t('mem_clear_confirm')} danger confirmLabel={t('mem_clear')}
          onConfirm={() => { S.clearMemory(); toast(t('mem_cleared')); }} />
      </div>
    );
  }

  // ---------- page ----------
  function SettingsPage({ nav }) {
    const st = useStore();
    const toast = useToast();
    const [nameDraft, setNameDraft] = useState(st.session.assistantName);
    useEffect(() => setNameDraft(st.session.assistantName), [st.session.assistantName]);
    const [l2Draft, setL2Draft] = useState(st.session.l2Prompt || '');
    useEffect(() => setL2Draft(st.session.l2Prompt || ''), [st.session.l2Prompt]);
    const saveL2 = () => { if (l2Draft !== (st.session.l2Prompt || '')) { S.patchSession({ l2Prompt: l2Draft }); toast(t('rl_saved')); } };
    const V = S.versionInfo;
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
          <UI.GlassCard className="dc-l2-card">
            <div className="dc-set-title dc-row-8"><IC.Sparkles size={15} style={{ color: 'var(--color-primary)' }} /> {t('st_l2_title')}</div>
            <p className="dc-set-sub" style={{ margin: '4px 0 10px' }}>{t('st_l2_sub')}</p>
            <UI.Textarea rows={4} maxLength={2000} placeholder={t('st_l2_ph')} value={l2Draft} onChange={(e) => setL2Draft(e.target.value)} onBlur={saveL2} />
            <p className="dc-field-sub" style={{ margin: '8px 2px 0' }}>{t('st_l2_note', { n: l2Draft.length })}</p>
          </UI.GlassCard>
        </div>

        <div className="dc-set-group">
          <UI.Label>{window.I18N.lang==='zh-CN'?'管理':'Admin'}</UI.Label>
          <a href="admin.html" style={{textDecoration:'none',color:'inherit',display:'block',marginTop:10}}>
            <UI.GlassCard padding="none" className="dc-set-row">
              <span className="dc-set-ic"><IC.Key size={18}/></span>
              <div className="dc-set-main">
                <div className="dc-set-title">{t('st_admin_console')}</div>
                <div className="dc-set-sub">{window.I18N.lang==='zh-CN'?'模型、密钥、数据与用户管理（需管理员账号）':'Models, keys, data & users (admin account required)'}</div>
              </div>
              <IC.ExternalLink size={16} style={{color:'var(--color-text-muted)',marginRight:14}}/>
            </UI.GlassCard>
          </a>
        </div>

        <div className="dc-set-group">
          <UI.Label>{t('st_group_prefs')}</UI.Label>
          <UI.GlassCard padding="none" className="dc-set-row" style={{ cursor: 'default' }}>
            <span className="dc-set-ic"><IC.Globe size={18} /></span>
            <div className="dc-set-main">
              <div className="dc-set-title">{t('st_language')}</div>
              <div className="dc-set-sub">{t('st_language_sub')}</div>
            </div>
            <Segmented small value={st.session.language} onChange={(v) => { S.patchSession({ language: v }); window.I18N.setLang(v); }}
              options={[{ value: 'zh-CN', label: '中文' }, { value: 'en-US', label: 'EN' }]} />
          </UI.GlassCard>
        </div>

        <div className="dc-set-group">
          <UI.Label>{t('st_group_care')}</UI.Label>
          <div className="dc-stack-8" style={{ marginTop: 10 }}>
            {PREFS.map((p) => (
              <UI.GlassCard key={p.key} padding="none" className="dc-set-row" style={{ cursor: 'default' }}>
                <span className="dc-set-ic"><p.icon size={17} /></span>
                <div className="dc-set-main">
                  <div className="dc-set-title">{t('pref_' + p.key)}</div>
                  <div className="dc-set-sub">{t('pref_' + p.key + '_sub')}</div>
                </div>
                <Switch on={!!st.preferences[p.key]} label={t('pref_' + p.key)} onToggle={() => S.patchPreferences({ [p.key]: !st.preferences[p.key] })} />
              </UI.GlassCard>
            ))}
            <p className="dc-field-sub" style={{ margin: '2px 2px 0' }}>{t('st_care_note')}</p>
          </div>
        </div>

        <div className="dc-set-group">
          <UI.Label>{t('st_group_channels')}</UI.Label>
          <ChannelsGroup />
        </div>

        <div className="dc-set-group">
          <UI.Label>{t('st_group_categories')}</UI.Label>
          <p className="dc-field-sub" style={{ margin: '6px 2px 10px' }}>{t('st_cat_sub')}</p>
          <div className="dc-cat-manage">
            {S.listCategories().map((c) => {
              const m = CF.catMeta(c.id);
              return (
                <UI.GlassCard key={c.id} padding="none" className="dc-set-row dc-cat-toggle" style={{ cursor: 'default' }}>
                  <CF.CatIc cat={c.id} size={34} />
                  <div className="dc-set-main">
                    <div className="dc-set-title" style={{ fontSize: 14 }}>{m.label}</div>
                    {c.locked ? <div className="dc-set-sub">{t('st_cat_note_locked')}</div> : null}
                  </div>
                  {c.locked ? <IC.Check size={16} style={{ color: 'var(--color-text-muted)' }} /> :
                    <Switch on={c.enabled} label={m.label} onToggle={() => S.setCategory(c.id, !c.enabled)} />}
                </UI.GlassCard>
              );
            })}
          </div>
        </div>

        <div className="dc-set-group">
          <UI.Label>{t('st_group_memory')}</UI.Label>
          <p className="dc-field-sub" style={{ margin: '6px 2px 0' }}>{t('mem_sub')}</p>
          <MemoryGroup />
        </div>

        <div className="dc-set-group">
          <UI.Label>{t('st_group_theme')}</UI.Label>
          <div style={{ marginTop: 10 }}><SP.ThemeStudio /></div>
        </div>

        <div className="dc-set-group">
          <UI.Label>{t('st_group_data')}</UI.Label>
          <div className="dc-stack-8" style={{ marginTop: 10 }}>
            <UI.GlassCard padding="none" className="dc-set-row" onClick={() => nav.go('materials', { sub: 'sources' })}>
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

        <div className="dc-about-card">
          <div className="dc-row-8" style={{ justifyContent: 'center' }}>
            <span style={{ fontWeight: 700 }}>{t('st_about_version', { v: V.build })}</span>
            <UI.Badge tone="warning">{t('st_beta_badge')}</UI.Badge>
          </div>
          <p className="dc-field-sub" style={{ margin: '6px 0 0', textAlign: 'center' }}>{t('st_api_line', { major: V.apiVersion, minor: V.apiMinor, min: V.minClient })}</p>
          <div style={{ textAlign: 'center', marginTop: 8 }}><a className="dc-admin-link" href="admin.html">{t('st_admin_console')} <IC.ExternalLink size={12} /></a></div>
        </div>
      </div>
    );
  }

  window.DcPages = Object.assign(window.DcPages || {}, { SettingsPage });
})();
