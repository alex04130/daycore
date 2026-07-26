// Daycore v2 — app shell: tab nav, appbar, theme boot, tweaks
(function () {
  'use strict';
  const { useState, useEffect, useMemo } = React;
  const UI = window.DaycoreUI;
  const IC = window.DcIcons;
  const S = window.DcStore;
  const { t } = window.I18N;
  const { useStore, useLang, useDesktop, applyCurrentTheme, ToastProvider, useToast } = window.DcUI;
  const P = window.DcPages;
  const { TweaksPanel, TweakSection, TweakToggle, TweakRadio, useTweaks } = window;

  applyCurrentTheme(S.state); // boot theme before first paint

  function greetKey() {
    const h = new Date().getHours();
    if (h < 5) return 'greet_night';
    if (h < 11) return 'greet_morning';
    if (h < 13) return 'greet_noon';
    if (h < 18) return 'greet_afternoon';
    if (h < 23) return 'greet_evening';
    return 'greet_night';
  }

  function App() {
    const st = useStore();
    const lang = useLang();
    const [tab, setTab] = useState('today');
    const [params, setParams] = useState(null);
    const [accountOpen, setAccountOpen] = useState(false);
    const [tweaks, setTweak] = useTweaks({ apVariant: 'brief', showAiNote: true });

    useEffect(() => { applyCurrentTheme(S.state); }, [st.session.currentTheme, st.customThemes.length]);
    useEffect(() => { document.documentElement.lang = lang; }, [lang]);

    const nav = useMemo(() => ({
      params,
      go(next, p) { setParams(p || null); setTab(next); window.scrollTo(0, 0); },
      clearParams() { setParams(null); },
    }), [params]);

    if (!st.onboarded) return <P.Onboarding onDone={() => setTab('today')} />;

    const TABS = [
      { key: 'today', label: t('tab_today'), icon: <IC.Sun size={22} /> },
      { key: 'materials', label: t('tab_materials'), icon: <IC.Layers size={22} /> },
      { key: 'companion', label: t('tab_companion'), icon: <IC.MessageHeart size={22} /> },
      { key: 'mood', label: t('tab_mood'), icon: <IC.Smile size={22} /> },
      { key: 'settings', label: t('tab_settings'), icon: <IC.Settings size={22} /> },
    ];
    const titles = { today: t('app_name'), materials: t('tab_materials'), companion: st.session.assistantName, mood: t('tab_mood'), settings: t('tab_settings') };

    return (
      <div className="dc-app">
        <header className="dc-appbar">
          <div className="dc-appbar-title">
            {tab === 'today' ? <span className="dc-appbar-greet">{t(greetKey())}{st.user ? '，' + st.user.name : ''}</span> : null}
            <span className="dc-appbar-main">{titles[tab]}</span>
          </div>
          <div className="dc-appbar-side">
            <button className="dc-avatar-btn" aria-label={t('au_account')} onClick={() => setAccountOpen(true)}>
              <UI.Avatar name={st.user ? st.user.name : '?'} size={38} />
            </button>
          </div>
        </header>

        <main className="dc-main">
          {tab === 'today' ? <P.TodayPage nav={nav} tweaks={tweaks} /> : null}
          {tab === 'materials' ? <P.MaterialsPage nav={nav} /> : null}
          {tab === 'companion' ? <P.CompanionPage nav={nav} /> : null}
          {tab === 'mood' ? <P.MoodPage /> : null}
          {tab === 'settings' ? <P.SettingsPage nav={nav} /> : null}
        </main>

        <div className="dc-tabbar-wrap">
          <div className="dc-rail-logo"><span className="dot"></span> {t('app_name')}</div>
          <UI.TabBar items={TABS} active={tab} onSelect={(k) => nav.go(k)} />
        </div>

        <P.AccountSheet open={accountOpen} onClose={() => setAccountOpen(false)} />

        <TweaksPanel title="Tweaks">
          <TweakSection label="今日页" />
          <TweakRadio label="规划面板" value={tweaks.apVariant} onChange={(v) => setTweak('apVariant', v)}
            options={[{ value: 'brief', label: '带摘要' }, { value: 'plain', label: '纯表单' }]} />
          <TweakToggle label="显示 AI 备注" value={tweaks.showAiNote} onChange={(v) => setTweak('showAiNote', v)} />
        </TweaksPanel>
      </div>
    );
  }

  ReactDOM.createRoot(document.getElementById('root')).render(
    <ToastProvider><App /></ToastProvider>
  );
})();
