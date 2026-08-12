import { StrictMode, useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './theme.css';
import * as api from '@daycore/core';
import { boot as bootUp, bootstrapCatalog, isFirstRun, type Boot, type Catalog } from '@daycore/core';
import { App } from './App';
import { Setting } from './Setting';
import { manifest } from './manifest';

// ⚠️ The packs 初版 SHIPS, in public/locales/. Passed in rather than read from
// @daycore/core, because each of the four frontends ships a different set — a
// constant in the shared package would be one frontend's answer imposed on the
// other three.
const SHIPPED = ['zh-CN', 'en-US'];

// ⚠️ The setting screen comes BEFORE the boot attempt on a fresh install, and
// after a failed one otherwise. Both directions matter: a first-run install has
// no address to try, and a broken address must lead back to the field that
// fixes it rather than to a dead screen with a reload button.
//
// ⚠️ Two catalogues, and the split is not incidental. `bootCat` is built from
// 初版's own shipped packs and covers the screens that run before any backend
// has been reached; `boot.catalog` is built from what the DEPLOYMENT reports it
// can render and covers everything after. A single catalogue would have to be
// one or the other — either the setting screen is untranslatable, or the
// language list is hardcoded.
function Root() {
  const [phase, setPhase] = useState<'setting' | 'booting' | 'up' | 'failed'>(
    isFirstRun() ? 'setting' : 'booting',
  );
  const [boot, setBoot] = useState<Boot | null>(null);
  const [bootCat, setBootCat] = useState<Catalog | null>(null);
  const [err, setErr] = useState('');

  useEffect(() => {
    void bootstrapCatalog(SHIPPED).then(setBootCat);
  }, []);

  useEffect(() => {
    if (phase !== 'booting') return;
    let live = true;
    bootUp(manifest).then(
      (b) => {
        if (!live) return;
        setBoot(b);
        setPhase('up');
        // The theme the session is on. Falls back to the build's default rather
        // than to nothing — an unthemed first paint reads as a broken install.
        document.documentElement.setAttribute('data-theme', b.session.currentTheme || 'sky');
      },
      (e) => {
        if (!live) return;
        const t = bootCat?.t ?? ((k: string) => k);
        setErr(
          api.isUnreachable(e)
            ? t('boot.unreachable')
            : e && typeof e === 'object' && 'kind' in e && (e as { kind: string }).kind === 'too-old'
              ? t('boot.tooOld', { version: String((e as { message: string }).message) })
              : e instanceof Error
                ? e.message
                : String(e),
        );
        setPhase('failed');
      },
    );
    return () => {
      live = false;
    };
  }, [phase, bootCat]);

  // Nothing renders before the bootstrap pack lands. It is a same-origin fetch
  // of a small file, and a flash of untranslated keys is worse than a beat of
  // nothing.
  if (!bootCat) return <div className="lc-app" />;
  const t = bootCat.t;

  if (phase === 'setting') {
    return (
      <Setting
        cat={bootCat}
        onDone={() => setPhase('booting')}
        onLocale={() => void bootstrapCatalog(SHIPPED).then(setBootCat)}
      />
    );
  }
  if (phase === 'up' && boot) return <App boot={boot} />;
  if (phase === 'failed') {
    return (
      <div className="lc-app">
        <main className="lc-main">
          <div className="lc-page">
            <h1 className="lc-title">{t('boot.failed.title')}</h1>
            <p className="lc-sub">{err}</p>
            <div className="lc-actrow">
              <button className="lc-btn pri" onClick={() => setPhase('setting')}>
                {t('boot.failed.editAddress')}
              </button>
              <button className="lc-btn sec" onClick={() => setPhase('booting')}>
                {t('boot.failed.retry')}
              </button>
            </div>
          </div>
        </main>
      </div>
    );
  }
  return (
    <div className="lc-app">
      <main className="lc-main">
        <div className="lc-page">
          <p className="lc-sub">{t('boot.connecting')}</p>
        </div>
      </main>
    </div>
  );
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Root />
  </StrictMode>,
);
