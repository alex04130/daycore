import { useEffect, useRef } from 'react';
import type { Boot } from '@daycore/core';
import { hourLabel, positionOf, toHM, toMin } from './stream';
import { useStore } from './store';

// 一天是一篇日志。向上滚是过去，向下滚是预告 —— 滚动本身就是时间旅行。
//
// ⚠️ 账本渲染流，不是聊天记录。每条是一个事件卡而不是气泡，足迹与今天是同一
// 条流的不同滚动位置 —— 没有「历史页」。导航退化成一个「回到现在」按钮。

export function App({ boot }: { boot: Boot }) {
  const s = useStore(boot);
  const t = boot.catalog.t;
  const nowRef = useRef<HTMLDivElement | null>(null);
  const scrollRef = useRef<HTMLDivElement | null>(null);

  // ⚠️ scrollTop, not scrollIntoView (HANDOFF 03 §7). scrollIntoView moves the
  // nearest scrollable ancestor and can drag the whole page on mobile; this
  // paradigm has exactly one scroll container and it must be the one that moves.
  const backToNow = () => {
    const box = scrollRef.current;
    const line = nowRef.current;
    if (!box || !line) return;
    box.scrollTo({ top: line.offsetTop - box.clientHeight / 3, behavior: 'smooth' });
  };

  // Land on 「现在」 rather than at the top: the ledger opens where you are, and
  // the past is something you choose to scroll back into.
  useEffect(() => {
    const id = window.setTimeout(backToNow, 60);
    return () => clearTimeout(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [s.stream.entries.length]);

  return (
    <div className="dc4-bg fl-app">
      <header className="fl-top">
        <span className="fl-date">{t('top.today')}</span>
        <span className="fl-count">
          {t('top.done', { done: s.stream.doneCount, total: s.stream.total })}
        </span>
        <button className="fl-back" onClick={backToNow}>
          {t('top.backToNow')}
        </button>
      </header>

      {s.undo && (
        <div className="fl-undo">
          <span>{s.undo.label}</span>
          <button onClick={() => void s.takeBack()}>{t('undo.take')}</button>
        </div>
      )}
      {s.error && <p className="fl-err">{s.error}</p>}

      <div className="fl-scroll" ref={scrollRef}>
        {s.stream.entries.length === 0 && (
          <div className="fl-blank">
            <h1>{t('stream.empty')}</h1>
            <p>{t('stream.emptyBody')}</p>
          </div>
        )}

        {s.stream.entries.map((e, i) => {
          const label = hourLabel(s.stream, i);
          const where = positionOf(s.stream, i);
          return (
            <div key={e.kind + ':' + e.id}>
              {i === s.stream.nowIndex && (
                <div className="fl-nowline" ref={nowRef}>
                  <i />
                  <span>{t('stream.now')}</span>
                  <i />
                </div>
              )}
              {label && (
                <div className="fl-hour">
                  {label.kind === 'untimed' ? t('stream.untimed') : label.text}
                </div>
              )}

              {e.kind === 'block' && e.block && (
                <article className={'fl-card ' + where + (e.block.lockLevel === 'hard' ? ' lk-hard' : '')}>
                  {e.block.lockLevel === 'hard' && <i className="fl-pin" title={t('entry.pinned')} />}
                  <div className="fl-row">
                    <span className="fl-time">{e.block.time ?? ''}</span>
                    <span className={'fl-title' + (e.block.completed ? ' is-done' : '')}>
                      {e.block.title}
                    </span>
                  </div>
                  <div className="fl-meta">
                    {e.block.duration_min ? (
                      <span>{t('entry.minutes', { n: e.block.duration_min })}</span>
                    ) : null}
                    {e.block.completed && <span>{t('entry.completed')}</span>}
                  </div>
                  {!e.block.completed && where === 'future' && (
                    <div className="fl-acts">
                      <button
                        className="fl-btn"
                        disabled={s.busy}
                        onClick={() => e.block && void s.complete(e.block)}
                      >
                        {t('entry.done')}
                      </button>
                    </div>
                  )}
                </article>
              )}

              {e.kind === 'proposal' && e.proposal && (
                <article className="fl-prop">
                  <div className="fl-prop-eye">{t('prop.eyebrow')}</div>
                  <div className="fl-prop-title">{e.proposal.title}</div>
                  {e.proposal.summary && <p className="fl-prop-sum">{e.proposal.summary}</p>}
                  {e.proposal.start && (
                    <div className="fl-meta">
                      <span>
                        {e.proposal.start}
                        {e.proposal.dur ? '–' + toHM(toMin(e.proposal.start) + e.proposal.dur) : ''}
                      </span>
                    </div>
                  )}
                  <div className="fl-acts">
                    <button
                      className="fl-btn pri"
                      disabled={s.busy}
                      onClick={() => e.proposal && void s.answer(e.proposal, true)}
                    >
                      {t('prop.accept')}
                    </button>
                    <button
                      className="fl-btn"
                      disabled={s.busy}
                      onClick={() => e.proposal && void s.answer(e.proposal, false)}
                    >
                      {t('prop.reject')}
                    </button>
                  </div>
                </article>
              )}
            </div>
          );
        })}

        {s.stream.nowIndex >= s.stream.entries.length && s.stream.entries.length > 0 && (
          <div className="fl-nowline" ref={nowRef}>
            <i />
            <span>{t('stream.now')}</span>
            <i />
          </div>
        )}
      </div>
    </div>
  );
}
