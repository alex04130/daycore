import type { Boot } from '@daycore/core';
import { boxOf, canvasHeight, DAY0, DAY1, toHM, yOf } from './canvas';
import { useStore } from './store';

// 时间是一块连续的空间画布 —— 你在地图上看自己的日子。
//
// ⚠️ Absolute positioning, not a list. That is the paradigm: two blocks that
// overlap in time overlap on screen, and you can SEE the collision instead of
// reading two adjacent rows and working it out.

const HOUR_STEP = 60;

export function App({ boot }: { boot: Boot }) {
  const s = useStore(boot);
  const t = boot.catalog.t;

  const hours: number[] = [];
  for (let m = DAY0; m <= DAY1; m += HOUR_STEP) hours.push(m);

  const nowVisible = s.now >= DAY0 && s.now <= DAY1;

  return (
    <div className="cj-app">
      <header className="cj-top">
        <span className="cj-title">{t('top.title')}</span>
        <span className="cj-count">
          {t('top.done', { done: s.doneCount, total: s.total })}
        </span>
        <span className="cj-clock">{t('top.now', { time: toHM(s.now) })}</span>
      </header>

      {s.undo && (
        <div className="cj-undo">
          <span>{s.undo.label}</span>
          <button onClick={() => void s.takeBack()}>{t('undo.take')}</button>
        </div>
      )}
      {s.error && <p className="cj-err">{s.error}</p>}

      <div className="cj-scroll">
        <div className="cj-day" style={{ height: canvasHeight() }}>
          {hours.map((m) => (
            <div key={m} className="cj-hour" style={{ top: yOf(m) }}>
              <span>{toHM(m)}</span>
              <i />
            </div>
          ))}

          {/* ⚠️ A uniform 1.5px line. An early version used a horizontal
              gradient, which made it thick on the left and vanish on the right —
              a line that means "now" must not look like it means "now, mostly". */}
          {nowVisible && (
            <div className="cj-now" style={{ top: yOf(s.now) }}>
              <span>{toHM(s.now)}</span>
            </div>
          )}

          {s.pieces.map((p) => {
            const box = boxOf(p);
            const past = p.e <= s.now;
            const style = {
              top: box.top,
              height: box.height,
              left: box.left,
              zIndex: p.kind === 'ghost' ? 14 + p.lane : 4 + p.lane,
            };
            if (p.kind === 'ghost' && p.proposal) {
              return (
                <div key={'g:' + p.id} className="cj-blk cj-ghost" style={style}>
                  <div className="cj-blk-eye">{t('ghost.eyebrow')}</div>
                  <div className="cj-blk-title">{p.title}</div>
                  {!box.compact && (
                    <div className="cj-blk-acts">
                      <button
                        className="cj-btn pri"
                        disabled={s.busy}
                        onClick={() => p.proposal && void s.answer(p.proposal, true)}
                      >
                        {t('ghost.accept')}
                      </button>
                      <button
                        className="cj-btn"
                        disabled={s.busy}
                        onClick={() => p.proposal && void s.answer(p.proposal, false)}
                      >
                        {t('ghost.reject')}
                      </button>
                    </div>
                  )}
                </div>
              );
            }
            const blk = p.block!;
            return (
              <div
                key={'b:' + p.id}
                className={
                  'cj-blk' +
                  (box.stacked ? ' stacked' : '') +
                  (box.compact ? ' compact' : '') +
                  (past ? ' past' : '') +
                  (blk.lockLevel === 'hard' ? ' locked' : '')
                }
                style={style}
              >
                <div className="cj-blk-row">
                  <span className="cj-blk-time">{blk.time}</span>
                  <span className={'cj-blk-title' + (blk.completed ? ' is-done' : '')}>
                    {blk.title}
                  </span>
                </div>
                {!box.compact && (
                  <>
                    <div className="cj-blk-meta">
                      {blk.duration_min ? (
                        <span>{t('block.minutes', { n: blk.duration_min })}</span>
                      ) : null}
                      {blk.lockLevel === 'hard' && <span>{t('block.locked')}</span>}
                      {blk.completed && <span>{t('block.completed')}</span>}
                    </div>
                    {!blk.completed && (
                      <div className="cj-blk-acts">
                        <button className="cj-btn" disabled={s.busy} onClick={() => void s.complete(blk)}>
                          {t('block.done')}
                        </button>
                      </div>
                    )}
                  </>
                )}
              </div>
            );
          })}

          {s.pieces.length === 0 && (
            <div className="cj-blank">
              <h1>{t('canvas.empty')}</h1>
              <p>{t('canvas.emptyBody')}</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
