import { describe, expect, it } from 'vitest';
import type { DayPlan, TimeBlock } from '@daycore/core';
import {
  addDays,
  dayDiff,
  foldRange,
  groupDay,
  inRange,
  monthGrid,
  pickRange,
  toDate,
  todayIso,
  weekOf,
  weekdayOf,
} from './days';

// Every assertion here was red-verified: the implementation was broken in the
// specific way the comment names, the test was watched to fail, and only then
// was it repaired. An assertion nobody has seen fail is a guess about what the
// code does.

const block = (o: Partial<TimeBlock> & { id: string }): TimeBlock => ({
  time: null,
  title: o.id,
  type: 'task',
  duration_min: 30,
  ...o,
});

describe('date arithmetic', () => {
  it('parses YYYY-MM-DD in the LOCAL zone, not UTC', () => {
    // ⚠️ Broke with `new Date(iso)`: east of Greenwich that is the previous day.
    const d = toDate('2026-08-11');
    expect(d.getFullYear()).toBe(2026);
    expect(d.getMonth()).toBe(7);
    expect(d.getDate()).toBe(11);
  });

  it('parses at noon, so a DST transition cannot move the day', () => {
    expect(toDate('2026-03-08').getHours()).toBe(12);
  });

  it('rolls over a month boundary', () => {
    expect(addDays('2026-08-31', 1)).toBe('2026-09-01');
    expect(addDays('2026-09-01', -1)).toBe('2026-08-31');
  });

  it('rolls over a leap day', () => {
    expect(addDays('2028-02-28', 1)).toBe('2028-02-29');
    expect(addDays('2028-02-29', 1)).toBe('2028-03-01');
    // ⚠️ 2100 is NOT a leap year. Broke with a naive "divisible by 4".
    expect(addDays('2100-02-28', 1)).toBe('2100-03-01');
  });

  it('counts days in both directions and gives 0 for the same day', () => {
    expect(dayDiff('2026-08-11', '2026-08-14')).toBe(3);
    expect(dayDiff('2026-08-14', '2026-08-11')).toBe(-3);
    expect(dayDiff('2026-08-11', '2026-08-11')).toBe(0);
  });

  it('counts across a DST boundary as whole days', () => {
    // ⚠️ Broke with Math.floor. In a zone that springs forward on 2026-03-08 the
    // noon-to-noon gap is 23 hours, so floor gives 0 and "tomorrow" reads as
    // "today". Passes in UTC either way — which is exactly why this is asserted
    // through dayDiff's rounding rather than by choosing a friendly zone.
    expect(dayDiff('2026-03-07', '2026-03-08')).toBe(1);
    expect(dayDiff('2026-03-08', '2026-03-09')).toBe(1);
    expect(dayDiff('2026-11-01', '2026-11-02')).toBe(1);
  });

  it('derives today from a Date in the local zone', () => {
    expect(todayIso(new Date(2026, 7, 3, 23, 30))).toBe('2026-08-03');
    // Zero-padding on both fields, which inRange's string compare depends on.
    expect(todayIso(new Date(2026, 0, 9, 0, 5))).toBe('2026-01-09');
  });
});

describe('weeks', () => {
  it('indexes weekdays 0=Sunday, matching by_weekday', () => {
    expect(weekdayOf('2026-08-09')).toBe(0); // a Sunday
    expect(weekdayOf('2026-08-10')).toBe(1);
    expect(weekdayOf('2026-08-15')).toBe(6);
  });

  it('starts the strip on Monday', () => {
    const w = weekOf('2026-08-12'); // a Wednesday
    expect(w).toHaveLength(7);
    expect(w[0]).toBe('2026-08-10');
    expect(w[6]).toBe('2026-08-16');
  });

  it('puts SUNDAY at the END of its own week, not the start of the next', () => {
    // ⚠️ THE test for `(getDay() + 6) % 7`. Broke with `-getDay() + 1`, which
    // sends Sunday forward: the reader opens the app on Sunday and is shown
    // next week, one day in seven.
    const w = weekOf('2026-08-09'); // Sunday
    expect(w[0]).toBe('2026-08-03');
    expect(w[6]).toBe('2026-08-09');
  });

  it('agrees with itself: every day of a week yields the same seven', () => {
    const reference = weekOf('2026-08-10');
    for (const d of reference) expect(weekOf(d)).toEqual(reference);
  });
});

describe('month grid', () => {
  it('pads to the first Sunday-led cell', () => {
    // 2026-08-01 is a Saturday → six leading blanks.
    const g = monthGrid('2026-08');
    expect(g.slice(0, 6)).toEqual([null, null, null, null, null, null]);
    expect(g[6]).toBe('2026-08-01');
    expect(g).toHaveLength(6 + 31);
  });

  it('has no padding when the month starts on a Sunday', () => {
    expect(monthGrid('2026-03')[0]).toBe('2026-03-01'); // 2026-03-01 is a Sunday
  });

  it('gets February right in a leap year and out of one', () => {
    // ⚠️ Broke with a hard-coded [31,28,31,…] table.
    expect(monthGrid('2028-02').filter(Boolean)).toHaveLength(29);
    expect(monthGrid('2026-02').filter(Boolean)).toHaveLength(28);
    expect(monthGrid('2100-02').filter(Boolean)).toHaveLength(28);
  });

  it('zero-pads every cell, which the range highlight compares as strings', () => {
    // ⚠️ Broke with `${mon}-${d}`: "2026-8-1" sorts after "2026-08-31", so the
    // highlight lands on scattered days rather than a run.
    const g = monthGrid('2026-01').filter((c): c is string => c !== null);
    for (const c of g) expect(c).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    expect(g[0]).toBe('2026-01-01');
  });
});

describe('foldRange', () => {
  const plan = (date: string, blocks: TimeBlock[]): DayPlan => ({ date, blocks });

  it('fills a cell for every day in the range, including days the backend omitted', () => {
    // ⚠️ THE assertion for this function. GET /api/plan/range returns only the
    // days it has; mapping over the response draws a week with missing columns.
    const cells = foldRange([plan('2026-08-12', [block({ id: 'a' })])], '2026-08-10', '2026-08-16');
    expect(cells.map((c) => c.date)).toEqual([
      '2026-08-10',
      '2026-08-11',
      '2026-08-12',
      '2026-08-13',
      '2026-08-14',
      '2026-08-15',
      '2026-08-16',
    ]);
  });

  it('tells "no plan" apart from "a plan with nothing in it"', () => {
    // ⚠️ Both have total 0 and they are NOT the same day. One should offer to
    // plan; the other already was planned and emptied.
    const cells = foldRange([plan('2026-08-11', [])], '2026-08-10', '2026-08-11');
    expect(cells[0]).toEqual({ date: '2026-08-10', total: 0, done: 0, empty: true });
    expect(cells[1]).toEqual({ date: '2026-08-11', total: 0, done: 0, empty: false });
  });

  it('counts done against VISIBLE blocks only', () => {
    // A hidden block is a rule occurrence the reader dismissed for that day. It
    // must not make the day look busier, nor sit forever uncompleted.
    const cells = foldRange(
      [
        plan('2026-08-11', [
          block({ id: 'a', completed: true }),
          block({ id: 'b' }),
          block({ id: 'c', hidden: true }),
        ]),
      ],
      '2026-08-11',
      '2026-08-11',
    );
    expect(cells[0]).toEqual({ date: '2026-08-11', total: 2, done: 1, empty: false });
  });

  it('handles a single-day range', () => {
    expect(foldRange([], '2026-08-11', '2026-08-11')).toHaveLength(1);
  });

  it('returns nothing when the range runs backwards', () => {
    // ⚠️ Broke with a `while (d <= to)` written as an unconditional do/while:
    // an inverted range emitted one bogus cell instead of none. Callers get
    // their ranges from pickRange, which never inverts — but this is the guard
    // that makes that a property rather than a hope.
    expect(foldRange([], '2026-08-16', '2026-08-10')).toEqual([]);
  });
});

describe('groupDay', () => {
  const at = (date: string, today: string, nowMinutes: number) => ({ date, today, nowMinutes });

  it('sorts timed blocks and sinks untimed ones into their own group', () => {
    const g = groupDay(
      [
        block({ id: 'noon', time: '12:00' }),
        block({ id: 'someday' }),
        block({ id: 'dawn', time: '07:30' }),
      ],
      at('2026-08-11', '2026-08-11', 0),
    );
    expect(g.timed.map((b) => b.id)).toEqual(['dawn', 'noon']);
    expect(g.floating.map((b) => b.id)).toEqual(['someday']);
  });

  it('drops hidden blocks from both groups', () => {
    const g = groupDay(
      [block({ id: 'gone', time: '09:00', hidden: true }), block({ id: 'ghost', hidden: true })],
      at('2026-08-11', '2026-08-11', 0),
    );
    expect(g.timed).toEqual([]);
    expect(g.floating).toEqual([]);
  });

  it('puts the now-line above the block starting this very minute', () => {
    // ⚠️ THE `>=` vs `>` assertion, and it earned its keep on the first run:
    // days.ts shipped with `>`, this failed, and the block starting right now
    // was rendering as already past — once a day, for one minute.
    const g = groupDay(
      [block({ id: 'a', time: '09:00' }), block({ id: 'b', time: '10:00' })],
      at('2026-08-11', '2026-08-11', 10 * 60),
    );
    expect(g.nowIndex).toBe(1);
    expect(g.timed[g.nowIndex!]!.id).toBe('b');
  });

  it('puts the line at the top when the whole day is still ahead', () => {
    const g = groupDay([block({ id: 'a', time: '09:00' })], at('2026-08-11', '2026-08-11', 6 * 60));
    expect(g.nowIndex).toBe(0);
  });

  it('puts the line at the end when the whole day is behind', () => {
    const g = groupDay([block({ id: 'a', time: '09:00' })], at('2026-08-11', '2026-08-11', 23 * 60));
    expect(g.nowIndex).toBe(1);
  });

  it('draws NO line on another day — null, not an index', () => {
    // ⚠️ The state 纸屿 does not have, and the reason this is not shared code.
    expect(groupDay([block({ id: 'a', time: '09:00' })], at('2026-08-12', '2026-08-11', 600)).nowIndex).toBeNull();
    expect(groupDay([block({ id: 'a', time: '09:00' })], at('2026-08-10', '2026-08-11', 600)).nowIndex).toBeNull();
  });

  it('draws no line when nothing is timed, even today', () => {
    expect(groupDay([block({ id: 'a' })], at('2026-08-11', '2026-08-11', 600)).nowIndex).toBeNull();
  });

  it('is pure — two calls on the same input agree, and the input is untouched', () => {
    // ⚠️ Guards the mutable "have I inserted the line yet" flag the prototype
    // used, which double-inserts under StrictMode.
    const input = [block({ id: 'b', time: '10:00' }), block({ id: 'a', time: '09:00' })];
    const snapshot = input.map((b) => b.id);
    const first = groupDay(input, at('2026-08-11', '2026-08-11', 570));
    const second = groupDay(input, at('2026-08-11', '2026-08-11', 570));
    expect(second.timed.map((b) => b.id)).toEqual(first.timed.map((b) => b.id));
    expect(second.nowIndex).toBe(first.nowIndex);
    expect(input.map((b) => b.id)).toEqual(snapshot);
  });
});

describe('pickRange', () => {
  it('opens a range on the first click', () => {
    expect(pickRange(null, '2026-08-11')).toEqual({ from: '2026-08-11', to: null });
  });

  it('closes it on the second', () => {
    expect(pickRange({ from: '2026-08-11', to: null }, '2026-08-14')).toEqual({
      from: '2026-08-11',
      to: '2026-08-14',
    });
  });

  it('swaps when the second click is earlier', () => {
    expect(pickRange({ from: '2026-08-14', to: null }, '2026-08-11')).toEqual({
      from: '2026-08-11',
      to: '2026-08-14',
    });
  });

  it('restarts once the range is complete, so an earlier start is reachable', () => {
    // ⚠️ THE ordering assertion. With the swap branch first, a complete range
    // only ever grows backwards from its end and the reader can never pick a
    // new start.
    expect(pickRange({ from: '2026-08-11', to: '2026-08-14' }, '2026-08-09')).toEqual({
      from: '2026-08-09',
      to: null,
    });
  });

  it('allows a single-day range', () => {
    expect(pickRange({ from: '2026-08-11', to: null }, '2026-08-11')).toEqual({
      from: '2026-08-11',
      to: '2026-08-11',
    });
  });

  it('never caps the length — the backend owns that number', () => {
    // ⚠️ Asserting the ABSENCE of the prototype's silent 7-day truncation. The
    // real limit is AUTO_PLAN_MAX_DAYS, runtime config the backend does not
    // publish, so any constant here is wrong on some deployment — and silently.
    expect(pickRange({ from: '2026-08-01', to: null }, '2026-08-30')).toEqual({
      from: '2026-08-01',
      to: '2026-08-30',
    });
  });
});

describe('inRange', () => {
  it('is false with nothing selected', () => {
    expect(inRange(null, '2026-08-11')).toBe(false);
  });

  it('matches only the anchor while the range is open', () => {
    const open = { from: '2026-08-11', to: null };
    expect(inRange(open, '2026-08-11')).toBe(true);
    expect(inRange(open, '2026-08-12')).toBe(false);
  });

  it('includes both endpoints', () => {
    const r = { from: '2026-08-11', to: '2026-08-14' };
    expect(inRange(r, '2026-08-11')).toBe(true);
    expect(inRange(r, '2026-08-14')).toBe(true);
    expect(inRange(r, '2026-08-13')).toBe(true);
    expect(inRange(r, '2026-08-10')).toBe(false);
    expect(inRange(r, '2026-08-15')).toBe(false);
  });

  it('compares across a month boundary', () => {
    // ⚠️ The string compare only works because both sides are zero-padded, and
    // a month boundary is where an unpadded value would first show up.
    const r = { from: '2026-08-28', to: '2026-09-03' };
    expect(inRange(r, '2026-09-01')).toBe(true);
    expect(inRange(r, '2026-08-27')).toBe(false);
  });
});
