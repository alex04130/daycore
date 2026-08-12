import { useState } from 'react';
import * as api from '@daycore/core';
import { backendBase, setBackendBase } from '@daycore/core';
import { chooseLocale, type Catalog } from '@daycore/core';

// The first-install screen every frontend is required to ship.
//
// docs/specs/frontend-manifest.md: "强制要求每个前端实现 /setting 首次安装配置
// 界面（至少能配连哪个后端）。第三方前端写死后端地址就只能对着一个部署用，而
// 自部署是常态。"
//
// ⚠️ It PROBES before it commits. Typing an address and being told "saved" only
// to hit a blank screen is the worst version of this interaction, because the
// person cannot tell a typo from an outage from a backend that is fine but not
// a daycore. One request answers all three, and the answer is on the same
// screen as the field they would fix.
//
// ⚠️ Its copy comes from the BOOTSTRAP catalogue — 初版's own shipped packs.
// This screen runs before any backend has been reached, so there is no
// deployment language list to read yet; the real one replaces it the moment the
// handshake answers. See packages/core/src/i18n.ts bootstrapCatalog.
export function Setting({
  cat,
  onDone,
  onLocale,
}: {
  cat: Catalog;
  onDone: () => void;
  onLocale: (l: string) => void;
}) {
  const [value, setValue] = useState(backendBase());
  const [state, setState] = useState<'idle' | 'checking' | 'ok'>('idle');
  const [err, setErr] = useState('');
  const t = cat.t;

  async function check() {
    setState('checking');
    setErr('');
    const previous = backendBase();
    setBackendBase(value);
    try {
      await api.probe();
      setState('ok');
      setErr('');
    } catch (e) {
      // Put the old address back. A field that has silently changed what the
      // app talks to, while showing an error about the new one, is how somebody
      // ends up debugging the wrong deployment.
      setBackendBase(previous);
      setState('idle');
      setErr(
        api.isUnreachable(e)
          ? t('setting.unreachable')
          : t('setting.notDaycore') + (e instanceof Error ? e.message : String(e)),
      );
    }
  }

  return (
    <div className="lc-app">
      <main className="lc-main">
        <div className="lc-page">
          <header className="lc-head">
            <h1 className="lc-title">{t('setting.title')}</h1>
          </header>
          <p className="lc-sub">{t('setting.body')}</p>
          <input
            className="lc-input"
            value={value}
            onChange={(e) => {
              setValue(e.target.value);
              setState('idle');
            }}
            placeholder={t('setting.placeholder')}
            autoFocus
            spellCheck={false}
            autoCapitalize="off"
            aria-label={t('setting.title')}
          />
          {err && <p className="lc-err">{err}</p>}
          {state === 'ok' && <p className="lc-sub">{t('setting.ok')}</p>}
          <div className="lc-actrow">
            {state === 'ok' ? (
              <button
                className="lc-btn pri"
                onClick={() => {
                  setBackendBase(value);
                  onDone();
                }}
              >
                {t('setting.use')}
              </button>
            ) : (
              <button className="lc-btn pri" disabled={state === 'checking'} onClick={check}>
                {state === 'checking' ? t('setting.trying') : t('setting.try')}
              </button>
            )}
          </div>

          <div className="lc-row">
            <span>{t('setting.language')}</span>
            <select
              className="lc-input"
              value={cat.locale}
              aria-label={t('setting.language')}
              onChange={(e) => {
                chooseLocale(e.target.value);
                onLocale(e.target.value);
              }}
            >
              {/* ⚠️ 初版's OWN packs only, on this screen. The deployment's list
                  is not knowable yet — see the note above. */}
              <option value="zh-CN">简体中文</option>
              <option value="en-US">English</option>
            </select>
          </div>
          <p className="lc-sub">{t('setting.langHint')}</p>
        </div>
      </main>
    </div>
  );
}
