import { StrictMode, useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './theme.css';
import { App } from './App';
import { Setting } from './Setting';
import { boot as bootUp, type Boot } from './session';
import { isFirstRun } from './backend';
import * as api from './api';

// ⚠️ The setting screen comes BEFORE the boot attempt on a fresh install, and
// after a failed one otherwise. Both directions matter: a first-run install has
// no address to try, and a broken address must lead back to the field that
// fixes it rather than to a dead screen with a reload button.
function Root() {
  const [phase, setPhase] = useState<'setting' | 'booting' | 'up' | 'failed'>(
    isFirstRun() ? 'setting' : 'booting',
  );
  const [boot, setBoot] = useState<Boot | null>(null);
  const [err, setErr] = useState('');

  useEffect(() => {
    if (phase !== 'booting') return;
    let live = true;
    bootUp().then(
      (b) => {
        if (!live) return;
        setBoot(b);
        setPhase('up');
        // The theme the session is on. Falls back to the build's default rather
        // than to nothing — an unthemed first paint reads as a broken install.
        document.documentElement.setAttribute('data-tg', b.session.currentTheme || 'night');
        if (b.deferred.length) {
          // Not an error and not silent. An operator has to approve 汀's shadow
          // kind before that one token can be themed; until then the
          // stylesheet's own value applies and everything else works.
          console.info('等运维批准后才能主题化的变量：', b.deferred.join(', '));
        }
      },
      (e) => {
        if (!live) return;
        setErr(api.isUnreachable(e) ? '连不上后端。' : e instanceof Error ? e.message : String(e));
        setPhase('failed');
      },
    );
    return () => {
      live = false;
    };
  }, [phase]);

  if (phase === 'setting') return <Setting onDone={() => setPhase('booting')} />;
  if (phase === 'up' && boot) return <App boot={boot} />;
  if (phase === 'failed') {
    return (
      <div className="tg-app">
        <div className="tg-frame">
          <div className="tg-main">
            <h1 className="tg-title md">连不上。</h1>
            <p className="tg-sub">{err}</p>
            <div className="tg-actrow">
              <button className="tg-btn pri" onClick={() => setPhase('setting')}>
                改一下地址
              </button>
              <button className="tg-btn sec" onClick={() => setPhase('booting')}>
                再试一次
              </button>
            </div>
          </div>
        </div>
      </div>
    );
  }
  return (
    <div className="tg-app">
      <div className="tg-frame">
        <div className="tg-main">
          <p className="tg-sub">连着…</p>
        </div>
      </div>
    </div>
  );
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Root />
  </StrictMode>,
);
