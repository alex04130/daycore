import { useEffect, useState } from 'react';
import { progressPct, toHM, toMin, nowMin } from './flow';
import { useStore } from './store';
import type { Boot } from './session';

// The single-piece flow. 画面永远只回答一个问题：现在做什么。
//
// Three faces, in priority order — proposal, current block, and the several
// kinds of nothing. The prototype (design-ui/ting) also has a brief face and a
// mood frame; those need endpoints 汀 does not call yet and are deliberately
// left out rather than faked, because a face wired to a mock is a face that
// will be rewritten.

function useSwipe(onUp: () => void, onLeft?: () => void) {
  const [d, setD] = useState({ x: 0, y: 0 });
  const down = (e: React.PointerEvent) => {
    if ((e.target as HTMLElement).closest('button,input')) return;
    const x0 = e.clientX;
    const y0 = e.clientY;
    const mv = (ev: PointerEvent) => setD({ x: ev.clientX - x0, y: Math.min(0, ev.clientY - y0) });
    const up = (ev: PointerEvent) => {
      window.removeEventListener('pointermove', mv);
      window.removeEventListener('pointerup', up);
      const dy = ev.clientY - y0;
      const dx = ev.clientX - x0;
      setD({ x: 0, y: 0 });
      if (dy < -90) onUp();
      else if (onLeft && dx < -90) onLeft();
    };
    window.addEventListener('pointermove', mv);
    window.addEventListener('pointerup', up);
  };
  const style: React.CSSProperties = {
    transform: `translate(${d.x}px, ${d.y}px)`,
    opacity: 1 + d.y / 260,
    transition: d.x || d.y ? 'none' : 'transform .25s cubic-bezier(.22,.9,.24,1), opacity .25s',
  };
  return [style, down] as const;
}

export function App({ boot }: { boot: Boot }) {
  const s = useStore(boot);
  const [clock, setClock] = useState(() => nowMin());
  useEffect(() => {
    const t = setInterval(() => setClock(nowMin()), 30_000);
    return () => clearInterval(t);
  }, []);

  const prop = s.proposals[0] ?? null;
  const cur = s.flow.current;

  const primary = () => {
    if (prop) void s.answer(prop, true);
    else if (cur) void s.complete(cur);
  };
  const secondary = () => {
    if (prop) void s.answer(prop, false);
  };

  // Desktop drives on the keyboard, not on swipes.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null;
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
      if (e.key === 'Enter' || e.key === 'ArrowUp') {
        e.preventDefault();
        primary();
      } else if (e.key === 'Escape') {
        e.preventDefault();
        secondary();
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  });

  const [propStyle, propDown] = useSwipe(() => prop && s.answer(prop, true), () => prop && s.answer(prop, false));
  const [curStyle, curDown] = useSwipe(() => cur && s.complete(cur));

  return (
    <div className="tg-app">
      <div className="tg-frame">
        {s.undo && (
          <div className="tg-undo">
            <span>{s.undo.label}</span>
            <button onClick={() => void s.takeBack()}>撤回</button>
          </div>
        )}

        <header className="tg-top">
          <span className="tg-clock">{toHM(clock)}</span>
          <span className="tg-l0">
            今天完成 {s.flow.doneCount}/{s.flow.total}
          </span>
        </header>

        <div className="tg-main">
          {s.error && <p className="tg-note">{s.error}</p>}

          {prop ? (
            <div className="tg-card tg-prop" onPointerDown={propDown} style={propStyle}>
              <div className="tg-eyebrow">
                <span>{prop.level === 'L3' ? '要紧的' : '一个提案'}</span>
                <i className="ln" />
              </div>
              <h1 className="tg-title md">{prop.title}</h1>
              {prop.summary && <p className="tg-sub">{prop.summary}</p>}
              {prop.reason && <p className="why">{prop.reason}</p>}
              {prop.start && (
                <div className="tg-metarow">
                  <span>
                    {prop.start}
                    {prop.dur ? '–' + toHM(toMin(prop.start) + prop.dur) : ''}
                  </span>
                </div>
              )}
              <div className="tg-actrow">
                <button className="tg-btn pri" disabled={s.busy} onClick={() => void s.answer(prop, true)}>
                  好
                </button>
                <button className="tg-btn sec" disabled={s.busy} onClick={() => void s.answer(prop, false)}>
                  不了
                </button>
              </div>
            </div>
          ) : cur ? (
            <div className="tg-card" onPointerDown={curDown} style={curStyle}>
              <div className="tg-eyebrow">
                <span>
                  此刻 · {cur.time}
                  {cur.duration_min ? '–' + toHM(toMin(cur.time!) + cur.duration_min) : ''}
                </span>
                <i className="ln" />
              </div>
              <h1 className="tg-title">{cur.title}</h1>
              {cur.note && <p className="tg-note">“{cur.note}”</p>}
              <div className="tg-metarow">
                <span className={'tg-origin ' + (cur.origin ?? 'manual')}>
                  {cur.origin === 'manual' ? '我放的' : '排的'}
                </span>
                {cur.duration_min ? (
                  <span>还剩 {Math.max(0, toMin(cur.time!) + cur.duration_min - clock)} 分钟</span>
                ) : null}
              </div>
              {cur.duration_min ? (
                <div className="tg-prog">
                  <i style={{ width: progressPct(cur, clock) + '%' }} />
                </div>
              ) : null}
              <div className="tg-actrow">
                <button className="tg-btn pri" disabled={s.busy} onClick={() => void s.complete(cur)}>
                  完成了
                </button>
              </div>
            </div>
          ) : s.flow.next ? (
            <div className="tg-card">
              <div className="tg-eyebrow mute">
                <span>空档</span>
                <i className="ln" />
              </div>
              <h1 className="tg-title md">
                现在空着，
                <br />
                空着不是欠着。
              </h1>
              <p className="tg-sub">{s.flow.gapMin} 分钟后才有下一件。</p>
            </div>
          ) : (
            <div className="tg-card">
              <div className="tg-eyebrow mute">
                <span>此刻</span>
                <i className="ln" />
              </div>
              <h1 className="tg-title md">
                没有下一件了。
                <br />
                没做完的不积债。
              </h1>
              <p className="tg-sub">要紧的明天会自己浮上来。</p>
            </div>
          )}

          {(prop || cur) && (
            <div className="tg-swipehint">
              <span className="up">↑</span>
              <span>上滑{prop ? '接受' : '送走'}</span>
            </div>
          )}
        </div>

        <footer className="tg-foot">
          {s.flow.next && (
            <div className="tg-next">
              接下来 · {s.flow.next.time} {s.flow.next.title}
            </div>
          )}
        </footer>
      </div>
    </div>
  );
}
