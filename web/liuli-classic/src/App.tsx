import { useEffect, useState } from 'react';
import type { Boot } from '@daycore/core';
import { useStore } from './store';
import { PageToday } from './PageToday';
import { PageMaterials } from './PageMaterials';
import { PageCompanion } from './PageCompanion';
import { PageMood } from './PageMood';
import { PageSettings } from './PageSettings';

// 五个页面，一条一直在屏幕上的导航栏。
//
// # ⚠️ The badge is the paradigm's tax, and it is the reason this shell exists
//
// The other three frontends have exactly one surface, so "something needs you"
// is answered by the surface itself: 汀 shows the thing, 纸屿 puts it in the
// stream, 长卷 draws it on the canvas. None of them owns a notion of "elsewhere".
//
// A five-page app does. The moment there is more than one place, a reader
// standing on one page cannot see the other four, and the ONLY honest way to
// say "there is something on 陪伴" is a mark on the tab. That is what this file
// is: not decoration, but the price of the paradigm.
//
// ⚠️ Which is why the badge is derived from state that is already loaded rather
// than polled per page. A badge that lies is worse than no badge — it teaches
// the reader to stop looking.

export type PageId = 'today' | 'materials' | 'companion' | 'mood' | 'settings';

export const PAGES: PageId[] = ['today', 'materials', 'companion', 'mood', 'settings'];

/** The rail appears here. Below it, the tab bar. */
const RAIL_MIN_WIDTH = 900;

function useWide(): boolean {
  const [wide, setWide] = useState(
    () => typeof window !== 'undefined' && window.innerWidth >= RAIL_MIN_WIDTH,
  );
  useEffect(() => {
    // ⚠️ matchMedia, not a resize listener. A resize listener fires on every
    // pixel and on the mobile URL bar sliding away; this fires once, when the
    // answer actually changes.
    const mq = window.matchMedia(`(min-width: ${RAIL_MIN_WIDTH}px)`);
    const on = () => setWide(mq.matches);
    mq.addEventListener('change', on);
    on();
    return () => mq.removeEventListener('change', on);
  }, []);
  return wide;
}

export function App({ boot }: { boot: Boot }) {
  const t = boot.catalog.t;
  const store = useStore(boot);
  const [page, setPage] = useState<PageId>('today');
  const wide = useWide();

  // ⚠️ Only 今日 carries a count today, and the honesty of that is the point.
  // 陪伴 would need "messages since you last looked", which nothing stores;
  // inventing it from `updatedAt` would put a number on the tab that is right
  // about as often as it is wrong. A tab with no badge says "nothing to report";
  // a tab with a guessed one says something false in a place people trust.
  const badges: Partial<Record<PageId, number>> = { today: store.proposals.length };

  const nav = (
    <nav className={wide ? 'lc-rail' : 'lc-tabs'} aria-label={t('nav.label')}>
      {PAGES.map((id) => {
        const n = badges[id] ?? 0;
        return (
          <button
            key={id}
            className={'lc-navitem' + (page === id ? ' on' : '')}
            aria-current={page === id ? 'page' : undefined}
            onClick={() => {
              setPage(id);
              // The repo's own rule: scrollTop, never scrollIntoView.
              window.scrollTo(0, 0);
            }}
          >
            <span className="lc-navdot" aria-hidden="true" />
            <span className="lc-navlabel">{t(`nav.${id}`)}</span>
            {n > 0 && (
              <span className="lc-badge" aria-label={t('nav.waiting', { n })}>
                {n > 9 ? '9+' : n}
              </span>
            )}
          </button>
        );
      })}
    </nav>
  );

  return (
    <div className={'lc-app' + (wide ? ' wide' : '')}>
      {nav}
      <main className="lc-main">
        {page === 'today' && <PageToday boot={boot} store={store} />}
        {page === 'materials' && <PageMaterials boot={boot} />}
        {page === 'companion' && <PageCompanion boot={boot} />}
        {page === 'mood' && <PageMood boot={boot} />}
        {page === 'settings' && <PageSettings boot={boot} />}
      </main>

      {/* ⚠️ pointer-events:none on the wrapper, auto on the bar itself. A
          full-width fixed overlay without that is an invisible sheet of glass
          across the bottom of every page — a mistake this project has already
          made once and written down. */}
      <div className="lc-undowrap">
        {store.undo && (
          <div className="lc-undo" role="status">
            <span>{store.undo.label}</span>
            <button className="lc-btn sec" onClick={() => void store.takeBack()}>
              {t('undo.takeBack')}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
