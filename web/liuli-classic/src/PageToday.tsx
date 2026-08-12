import { useState } from 'react';
import * as api from '@daycore/core';
import type { Boot, TimeBlock } from '@daycore/core';
import type { Store } from './store';
import { addDays, dayDiff, inRange, monthGrid, pickRange, toDate, weekOf, type Range } from './days';

// 今日：这一天，以及它前后的那些天。
//
// ⚠️ The one page where the date cursor is visible, and therefore the one that
// makes 初版 a different product from the other three rather than a different
// skin. 汀/纸屿/长卷 each answer "what now"; this answers "what about that day".

function fmtWeekday(iso: string, locale: string): string {
  // ⚠️ Intl, never a hand-rolled table. Adding a language is supposed to be
  // dropping a JSON file — a weekday list in the source makes that false for
  // the one string readers see most.
  return new Intl.DateTimeFormat(locale, { weekday: 'short' }).format(toDate(iso));
}

function fmtDay(iso: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { month: 'long', day: 'numeric' }).format(toDate(iso));
}

/** 今天 / 明天 / a real date. ⚠️ No 「昨天」 — see below. */
function dayLabel(iso: string, today: string, locale: string, t: Boot['catalog']['t']): string {
  const d = dayDiff(today, iso);
  if (d === 0) return t('today.label.today');
  if (d === 1) return t('today.label.tomorrow');
  // ⚠️ The prototype had no 「昨天」 either, and it is a product decision rather
  // than an omission: a friendly name for yesterday invites treating it as
  // still-editable, and the past is petrified. A plain date reads as a record.
  return fmtDay(iso, locale);
}

export function PageToday({ boot, store }: { boot: Boot; store: Store }) {
  const t = boot.catalog.t;
  const locale = boot.catalog.locale;
  const [adding, setAdding] = useState(false);
  const [title, setTitle] = useState('');
  const [time, setTime] = useState('');
  const [planning, setPlanning] = useState(false);
  const [range, setRange] = useState<Range | null>(null);
  const [month, setMonth] = useState(() => store.date.slice(0, 7));
  const [instructions, setInstructions] = useState('');
  const [planNote, setPlanNote] = useState('');
  const [planBusy, setPlanBusy] = useState(false);

  const days = weekOf(store.date);
  const { timed, nowIndex, floating } = store.grouped;
  const done = timed.filter((b) => b.completed).length + floating.filter((b) => b.completed).length;
  const total = timed.length + floating.length;

  async function runAutoPlan() {
    if (!range?.to) return;
    setPlanBusy(true);
    setPlanNote('');
    try {
      const res = await api.autoPlan({
        from: range.from,
        to: range.to,
        date: store.date,
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
        instructions: instructions.trim(),
        mode: 'keep_manual',
      });
      // ⚠️ A 200 that is not a plan. `no_material` means the model had nothing
      // to work from — an answer, not a fault — so it arrives with res.ok true
      // and must be read off the body. A client that only checks the HTTP status
      // renders an empty successful day and the reader never learns why.
      if (res.error) {
        setPlanNote(
          res.error === 'no_material'
            ? t('today.plan.noMaterial')
            : String(res.message ?? res.error),
        );
      } else {
        setPlanNote(String(res.note ?? t('today.plan.done')));
        setPlanning(false);
        await store.refresh();
      }
    } catch (e) {
      // ⚠️ `range_too_large` arrives as a 400 whose message already carries THIS
      // deployment's limit (AUTO_PLAN_MAX_DAYS, runtime config that is never
      // published to clients). Showing the backend's sentence is the only way
      // to be right on every install — any number written here is a guess.
      setPlanNote(e instanceof Error ? e.message : String(e));
    } finally {
      setPlanBusy(false);
    }
  }

  return (
    <div className="lc-page">
      <header className="lc-head">
        <h1 className="lc-title">{dayLabel(store.date, store.today, locale, t)}</h1>
        {store.date !== store.today && (
          <button className="lc-btn sec" onClick={() => store.setDate(store.today)}>
            {t('today.backToToday')}
          </button>
        )}
      </header>

      {/* ── 周条 ── */}
      <div className="lc-strip" role="group" aria-label={t('today.week')}>
        <button className="lc-step" aria-label={t('today.prevWeek')} onClick={() => store.setDate(addDays(store.date, -7))}>
          ‹
        </button>
        {days.map((d, i) => {
          const cell = store.week[i];
          return (
            <button
              key={d}
              className={
                'lc-day' +
                (d === store.date ? ' on' : '') +
                (d === store.today ? ' today' : '')
              }
              onClick={() => store.setDate(d)}
              aria-current={d === store.date ? 'date' : undefined}
            >
              <span className="lc-dayname">{fmtWeekday(d, locale)}</span>
              <span className="lc-daynum">{Number(d.slice(8))}</span>
              {/* ⚠️ Three states, not two: nothing planned / planned and open /
                  planned and finished. `empty` is what tells the first two
                  apart, and they mean opposite things to a reader. */}
              <span
                className={
                  'lc-daydot' +
                  (cell && !cell.empty && cell.total > 0
                    ? cell.done === cell.total
                      ? ' full'
                      : ' some'
                    : '')
                }
              />
            </button>
          );
        })}
        <button className="lc-step" aria-label={t('today.nextWeek')} onClick={() => store.setDate(addDays(store.date, 7))}>
          ›
        </button>
      </div>

      {total > 0 && (
        <p className="lc-sub">{t('today.progress', { done, total })}</p>
      )}
      {store.error && <p className="lc-err">{store.error}</p>}

      {/* ── 提案 ── */}
      {store.proposals.map((p) => (
        <article key={p.id} className="lc-card ghost">
          <h2 className="lc-cardtitle">{p.title}</h2>
          {p.summary && <p className="lc-sub">{p.summary}</p>}
          <div className="lc-actrow">
            <button className="lc-btn pri" disabled={store.busy} onClick={() => void store.answer(p, true)}>
              {t('proposal.accept')}
            </button>
            <button className="lc-btn sec" disabled={store.busy} onClick={() => void store.answer(p, false)}>
              {t('proposal.reject')}
            </button>
          </div>
        </article>
      ))}

      {/* ── 这一天 ── */}
      {total === 0 ? (
        <p className="lc-empty">{t('today.empty')}</p>
      ) : (
        <ul className="lc-list">
          {timed.map((b, i) => (
            <li key={b.id}>
              {nowIndex === i && <div className="lc-now">{t('today.now')}</div>}
              <BlockRow b={b} store={store} t={t} />
            </li>
          ))}
          {nowIndex === timed.length && <li><div className="lc-now">{t('today.now')}</div></li>}
          {floating.length > 0 && <li className="lc-groupsep">{t('today.untimed')}</li>}
          {floating.map((b) => (
            <li key={b.id}>
              <BlockRow b={b} store={store} t={t} />
            </li>
          ))}
        </ul>
      )}

      <div className="lc-actrow">
        <button className="lc-btn pri" onClick={() => setPlanning((v) => !v)}>
          {t('today.autoPlan')}
        </button>
        <button className="lc-btn sec" onClick={() => setAdding((v) => !v)}>
          {t('today.add')}
        </button>
      </div>

      {adding && (
        <form
          className="lc-form"
          onSubmit={(e) => {
            e.preventDefault();
            if (!title.trim()) return;
            void store.add(title.trim(), time || null);
            setTitle('');
            setTime('');
            setAdding(false);
          }}
        >
          <input
            className="lc-input"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder={t('today.addTitle')}
            aria-label={t('today.addTitle')}
          />
          <input
            className="lc-input time"
            type="time"
            value={time}
            onChange={(e) => setTime(e.target.value)}
            aria-label={t('today.addTime')}
          />
          <button className="lc-btn pri" type="submit" disabled={store.busy}>
            {t('common.save')}
          </button>
        </form>
      )}

      {planning && (
        <section className="lc-card">
          <h2 className="lc-cardtitle">{t('today.plan.title')}</h2>
          <p className="lc-sub">{t('today.plan.pickRange')}</p>
          <div className="lc-monthhead">
            <button className="lc-step" onClick={() => setMonth(prevMonth(month))} aria-label={t('today.prevMonth')}>‹</button>
            <span>{new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'long' }).format(toDate(month + '-01'))}</span>
            <button className="lc-step" onClick={() => setMonth(nextMonth(month))} aria-label={t('today.nextMonth')}>›</button>
          </div>
          <div className="lc-grid">
            {monthGrid(month).map((cell, i) =>
              cell === null ? (
                <span key={`pad${i}`} className="lc-cell pad" />
              ) : (
                <button
                  key={cell}
                  className={'lc-cell' + (inRange(range, cell) ? ' on' : '')}
                  onClick={() => setRange((r) => pickRange(r, cell))}
                >
                  {Number(cell.slice(8))}
                </button>
              ),
            )}
          </div>
          <textarea
            className="lc-input"
            value={instructions}
            onChange={(e) => setInstructions(e.target.value)}
            placeholder={t('today.plan.instructions')}
            aria-label={t('today.plan.instructions')}
          />
          {planNote && <p className="lc-sub">{planNote}</p>}
          <div className="lc-actrow">
            <button className="lc-btn pri" disabled={!range?.to || planBusy} onClick={() => void runAutoPlan()}>
              {planBusy ? t('today.plan.running') : t('today.plan.start')}
            </button>
            <button className="lc-btn sec" onClick={() => setPlanning(false)}>
              {t('common.cancel')}
            </button>
          </div>
        </section>
      )}
    </div>
  );
}

function prevMonth(m: string): string {
  const [y = '1970', mo = '1'] = m.split('-');
  const d = new Date(Number(y), Number(mo) - 2, 1, 12);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
}

function nextMonth(m: string): string {
  const [y = '1970', mo = '1'] = m.split('-');
  const d = new Date(Number(y), Number(mo), 1, 12);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
}

function BlockRow({ b, store, t }: { b: TimeBlock; store: Store; t: Boot['catalog']['t'] }) {
  // ⚠️ A locked block still renders its actions. The gate answers 409 with a
  // sentence written for a person, and store.act shows it — which is the
  // product rule "给死路一条岔路" working as designed. Hiding the button here
  // would replace an explanation with silence.
  return (
    <article className={'lc-card' + (b.completed ? ' done' : '')}>
      <div className="lc-cardmain">
        {b.time && <span className="lc-time">{b.time}</span>}
        <span className="lc-cardtitle">{b.title}</span>
        {b.lockLevel === 'hard' && <span className="lc-tag">{t('block.locked')}</span>}
      </div>
      <div className="lc-actrow">
        {!b.completed && (
          <button className="lc-btn pri" disabled={store.busy} onClick={() => void store.complete(b)}>
            {t('block.complete')}
          </button>
        )}
        <button className="lc-btn sec" disabled={store.busy} onClick={() => void store.remove(b)}>
          {/* ⚠️ A rule-sourced block is not deleted, it is tombstoned for that
              day — the rule survives. Two different words because they are two
              different promises, and the backend keeps them apart whether or
              not this screen does. */}
          {b.origin === 'rule' ? t('block.hideToday') : t('block.remove')}
        </button>
      </div>
    </article>
  );
}
