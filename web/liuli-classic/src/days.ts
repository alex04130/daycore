import type { DayPlan, TimeBlock } from '@daycore/core';

// 一天不再是唯一的一天。
//
// # The paradigm, as a second axis
//
// 汀 answers "what now" with ONE thing. 纸屿 answers it with a POSITION in a
// stream. 长卷 answers it with a COORDINATE on a map of today. All three ask
// the same question about the same day — and all three hard-code that day:
// `const date = api.todayIso()` appears verbatim in ting/store.ts,
// zhiyu/store.ts and liuli/store.ts, and none of them ever asks for another.
//
// 页面制 does not offer a fourth answer to "what now". It offers a second AXIS:
// the week strip, the month grid, "jump to a date", "back to today", the
// planning range — every one of those is the same idea wearing different
// clothes, and not one of the other three has anywhere to put it. That is what
// this module is, and it is why none of it belongs in @daycore/core.
//
// ⚠️ It looks like it could be shared with 纸屿, and it cannot. See groupDay.

/** Today in the reader's own zone, as YYYY-MM-DD. */
export function todayIso(now = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${now.getFullYear()}-${p(now.getMonth() + 1)}-${p(now.getDate())}`;
}

/**
 * Parse YYYY-MM-DD into a local Date at NOON.
 *
 * ⚠️ Two deliberate things, each of which has its own bug if dropped.
 *
 * `new Date('2026-08-11')` is parsed as UTC by spec, so east of Greenwich it is
 * the previous day the moment you read `.getDate()`. Hence the manual split.
 *
 * And noon rather than midnight: on a DST spring-forward date, local midnight
 * may not exist, and the runtime resolves it by moving into the previous day.
 * Noon is never within a transition. This matters here more than in the other
 * three because this is the only frontend that does date ARITHMETIC.
 */
export function toDate(iso: string): Date {
  const [y = '1970', m = '1', d = '1'] = iso.split('-');
  return new Date(Number(y), Number(m) - 1, Number(d), 12, 0, 0, 0);
}

export function addDays(iso: string, n: number): string {
  const d = toDate(iso);
  d.setDate(d.getDate() + n);
  return todayIso(d);
}

/**
 * Whole days from `a` to `b`.
 *
 * ⚠️ Math.round, not Math.floor. Both endpoints are at local noon, so a DST
 * boundary between them makes the difference 23 or 25 hours rather than 24 —
 * floor turns "tomorrow" into "today" twice a year, for everyone in that zone,
 * on a day nobody is looking.
 */
export function dayDiff(a: string, b: string): number {
  return Math.round((toDate(b).getTime() - toDate(a).getTime()) / 86_400_000);
}

/** 0 = Sunday … 6 = Saturday, matching ScheduleRule.by_weekday. */
export function weekdayOf(iso: string): number {
  return toDate(iso).getDay();
}

/**
 * The seven dates of `iso`'s week, Monday first.
 *
 * ⚠️ `(getDay() + 6) % 7` exists for exactly one reason: to map Sunday (0) to 6
 * rather than to 0. Written the obvious way — `-getDay() + 1` — Sunday jumps
 * forward instead of back, and the symptom is that opening the app on a Sunday
 * shows you NEXT week. One day in seven, so it survives casual testing.
 *
 * ⚠️ Monday-first here and Sunday-first in monthGrid, on purpose. The grid is
 * indexed the way `by_weekday` is indexed; the strip is read the way a week is
 * read. Unifying them means one of the two is wrong.
 */
export function weekOf(iso: string): string[] {
  const monday = addDays(iso, -((weekdayOf(iso) + 6) % 7));
  return Array.from({ length: 7 }, (_, i) => addDays(monday, i));
}

/**
 * A month's calendar cells, Sunday first, with leading blanks.
 *
 * @param month YYYY-MM
 */
export function monthGrid(month: string): (string | null)[] {
  const [y = '1970', m = '1'] = month.split('-');
  const year = Number(y);
  const mon = Number(m);
  const pad = new Date(year, mon - 1, 1, 12).getDay();
  // ⚠️ `new Date(y, mon, 0)` — mon is already 1-based here, so this is "day 0 of
  // NEXT month", which is the last day of this one. February gets 29 in a leap
  // year for free. A hard-coded length table gets that wrong once every four
  // years, in a way that shifts every subsequent cell.
  const len = new Date(year, mon, 0).getDate();
  const cells: (string | null)[] = Array.from({ length: pad }, () => null);
  for (let d = 1; d <= len; d++) {
    cells.push(`${y}-${String(mon).padStart(2, '0')}-${String(d).padStart(2, '0')}`);
  }
  return cells;
}

/** One day, as much as a strip or a grid cell needs to know about it. */
export interface DayCell {
  date: string;
  total: number;
  done: number;
  /** Nothing is planned. ⚠️ Distinct from `total === 0` on a plan that EXISTS. */
  empty: boolean;
}

/**
 * Fold a range response into one cell per day.
 *
 * ⚠️ Days the backend did not return must still get a cell. GET /api/plan/range
 * omits days with nothing stored — it does not return them as empty plans — so
 * mapping over the response draws a week with columns missing rather than a week
 * with empty columns. Every caller here wants a fixed-width grid.
 *
 * ⚠️ Three states, not two. A day with no plan at all and a day whose plan holds
 * nothing visible look identical in a `total` count and are not the same thing:
 * the first should offer to plan the day, the second already was planned and
 * emptied. `empty` carries that; `total === 0` cannot.
 */
export function foldRange(plans: DayPlan[], from: string, to: string): DayCell[] {
  const byDate = new Map(plans.map((p) => [p.date, p]));
  const out: DayCell[] = [];
  for (let d = from; dayDiff(d, to) >= 0; d = addDays(d, 1)) {
    const plan = byDate.get(d);
    if (!plan) {
      out.push({ date: d, total: 0, done: 0, empty: true });
      continue;
    }
    const visible = plan.blocks.filter((b) => !b.hidden);
    out.push({
      date: d,
      total: visible.length,
      done: visible.filter((b) => b.completed).length,
      empty: false,
    });
  }
  return out;
}

export function toMin(hhmm: string): number {
  const [h = '0', m = '0'] = hhmm.split(':');
  return Number(h) * 60 + Number(m);
}

export function nowMin(now = new Date()): number {
  return now.getHours() * 60 + now.getMinutes();
}

/** A day, split the way a page renders it. */
export interface Grouped {
  /** Blocks with a time, earliest first. */
  timed: TimeBlock[];
  /**
   * Where the 「现在」 divider goes inside `timed`, or null for no divider.
   *
   * ⚠️ null and `timed.length` are different answers. null means "do not draw
   * one" (you are looking at another day, or nothing is timed); `timed.length`
   * means "draw it at the bottom, the whole day is behind you".
   */
  nowIndex: number | null;
  /** Blocks with no time. They sit after the timed ones, never at midnight. */
  floating: TimeBlock[];
}

/**
 * Split one day's blocks for rendering.
 *
 * ⚠️ This is NOT 纸屿's stream.ts, and merging them would break both. Three
 * differences, each load-bearing:
 *
 *   - 纸屿 builds ONE ordered stream with proposals interleaved by time; a page
 *     has two groups and keeps proposals in a separate stack at the top.
 *   - 纸屿 always looks at today, so its now-line always exists. This looks at
 *     any day, so "no line at all" is a state it must have and 纸屿 does not.
 *   - 纸屿's entries carry a proposal; these are blocks.
 *
 * Sharing it would make one paradigm borrow the other's idea of the present,
 * and the borrowed one is always subtly wrong.
 *
 * ⚠️ Returns an INDEX rather than splicing a marker in. The prototype tracked
 * "have I inserted it yet" in a variable mutated from inside a map callback,
 * which under StrictMode's double-invoke inserts twice.
 */
export function groupDay(
  blocks: TimeBlock[],
  at: { date: string; today: string; nowMinutes: number },
): Grouped {
  const visible = blocks.filter((b) => !b.hidden);
  const timed = visible
    .filter((b) => b.time !== null)
    .sort((a, b) => toMin(a.time!) - toMin(b.time!));
  const floating = visible.filter((b) => b.time === null);

  let nowIndex: number | null = null;
  if (at.date === at.today && timed.length > 0) {
    // ⚠️ `>=`, and the first draft of this line had `>`. The index is where the
    // line is DRAWN, so everything from it down is still ahead of you — which
    // means the predicate has to match the block you are about to start, not
    // skip past it. With `>`, a block beginning at exactly this minute lands
    // above the line and renders as already gone: once a day, for one minute,
    // on the one item the reader was looking at.
    //
    // 纸屿 got this right first (zhiyu/src/stream.ts uses `at >= atMin`); this
    // file got it wrong and the test below is what said so.
    const i = timed.findIndex((b) => toMin(b.time!) >= at.nowMinutes);
    nowIndex = i === -1 ? timed.length : i;
  }
  return { timed, nowIndex, floating };
}

/** A closed date range being picked on a calendar. */
export interface Range {
  from: string;
  to: string | null;
}

/**
 * Advance a two-click range selection.
 *
 * ⚠️ The order of the branches is the whole function.
 *
 *   1. A COMPLETE range restarts. Without this the reader can never choose an
 *      earlier start — every later click only moves the end.
 *   2. Then, and only then, swap if they picked backwards.
 *
 * ⚠️ There is NO length cap here, and its absence is deliberate. The prototype
 * silently truncated to 7 days; the real limit is AUTO_PLAN_MAX_DAYS, a runtime
 * config the backend does not publish — so any number here is a guess that is
 * wrong on a deployment configured differently, AND the truncation was silent,
 * so a reader who clicked the tenth day just watched the seventh light up. The
 * honest path is to send it and render the backend's own `range_too_large`,
 * which arrives already carrying that deployment's number.
 */
export function pickRange(cur: Range | null, clicked: string): Range {
  if (!cur || cur.to !== null) return { from: clicked, to: null };
  if (dayDiff(cur.from, clicked) < 0) return { from: clicked, to: cur.from };
  return { from: cur.from, to: clicked };
}

/** Whether a date falls inside a completed range. */
export function inRange(r: Range | null, date: string): boolean {
  if (!r) return false;
  // ⚠️ String comparison, which is only valid because monthGrid zero-pads. An
  // unpadded "2026-8-1" sorts after "2026-08-31" and the highlight scatters.
  return r.to === null ? date === r.from : date >= r.from && date <= r.to;
}
