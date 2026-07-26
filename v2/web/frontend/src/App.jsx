// Ported 1:1 from the design prototype (app/main.jsx). The TweaksPanel is a
// design-review tool and is intentionally omitted; its defaults are inlined
// (auto-plan sheet: brief variant with summary chips; AI note shown).
import React, { useState, useEffect, useMemo } from 'react';
import UI from './boot/ds.js';
import IC from './icons.js';
import S from './store.js';
import I18N, { t } from './i18n.js';
import { useStore, useLang, applyCurrentTheme } from './ui.jsx';
import Onboarding from './pages/Onboarding.jsx';
import TodayPage from './pages/Today.jsx';
import MaterialsPage from './pages/Materials.jsx';
import CompanionPage from './pages/Companion.jsx';
import MoodPage from './pages/Mood.jsx';
import SettingsPage, { AccountSheet } from './pages/Settings.jsx';

const TWEAKS = { apVariant: 'brief', showAiNote: true };

function greetKey() {
  const h = new Date().getHours();
  if (h < 5) return 'greet_night';
  if (h < 11) return 'greet_morning';
  if (h < 13) return 'greet_noon';
  if (h < 18) return 'greet_afternoon';
  if (h < 23) return 'greet_evening';
  return 'greet_night';
}

export default function App() {
  const st = useStore();
  const lang = useLang();
  const [tab, setTab] = useState('today');
  const [params, setParams] = useState(null);
  const [accountOpen, setAccountOpen] = useState(false);

  useEffect(() => { applyCurrentTheme(S.state); }, [st.session.currentTheme, st.customThemes.length]);
  useEffect(() => { document.documentElement.lang = lang; }, [lang]);

  const nav = useMemo(() => ({
    params,
    go(next, p) { setParams(p || null); setTab(next); window.scrollTo(0, 0); },
    clearParams() { setParams(null); },
  }), [params]);

  if (!st.onboarded) return <Onboarding onDone={() => setTab('today')} />;

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
        {tab === 'today' ? <TodayPage nav={nav} tweaks={TWEAKS} /> : null}
        {tab === 'materials' ? <MaterialsPage nav={nav} /> : null}
        {tab === 'companion' ? <CompanionPage nav={nav} /> : null}
        {tab === 'mood' ? <MoodPage /> : null}
        {tab === 'settings' ? <SettingsPage nav={nav} /> : null}
      </main>

      <div className="dc-tabbar-wrap">
        <div className="dc-rail-logo"><span className="dot"></span> {t('app_name')}</div>
        <UI.TabBar items={TABS} active={tab} onSelect={(k) => nav.go(k)} />
      </div>

      <AccountSheet open={accountOpen} onClose={() => setAccountOpen(false)} />
    </div>
  );
}
