import { describe, expect, it } from 'vitest';
import type { ScheduleRule } from '@daycore/core';
import { describeRule, weekdayNames } from './rules';

// Red-verified the same way as days.test.ts. The assertions that matter most
// here are the ones about rules that can NEVER fire: the backend answers false
// and says nothing, so a wrong description is the only signal a reader would
// ever get, and it would be a reassuring one.

const rule = (o: Partial<ScheduleRule>): ScheduleRule => ({
  id: 'r',
  title: 'x',
  type: 'task',
  time: '09:00',
  kind: 'recurring',
  active: true,
  ...o,
});

describe('weekdayNames', () => {
  it('is indexed 0 = Sunday, matching by_weekday', () => {
    // ⚠️ The whole index rests on 2023-01-01 being a Sunday. Asserted rather
    // than trusted, because swapping the anchor for a "nicer" date silently
    // rotates every weekday in the app by however many days it is off.
    expect(new Date(2023, 0, 1, 12).getDay()).toBe(0);
    const en = weekdayNames('en-US');
    expect(en[0]).toBe('Sun');
    expect(en[6]).toBe('Sat');
  });

  it('uses short names, not narrow ones', () => {
    // ⚠️ English narrow is S/M/T/W/T/F/S — Tue and Thu identical, Sat and Sun
    // identical. "every T and T" is not a schedule anybody can read.
    const en = weekdayNames('en-US');
    expect(new Set(en).size).toBe(7);
  });

  it('follows the locale', () => {
    expect(weekdayNames('zh-CN')[1]).not.toBe(weekdayNames('en-US')[1]);
  });
});

describe('describeRule — cadence', () => {
  it('describes a one-off by its date', () => {
    expect(describeRule(rule({ kind: 'once', date: '2026-08-14' }))).toEqual([
      { key: 'rules.once', vars: { date: '2026-08-14' } },
    ]);
  });

  it('calls a one-off with no date what it is: never', () => {
    // ⚠️ expand.go returns `r.Date != nil && *r.Date == date`, so this rule can
    // never fire. Describing it as "once, on ()" would be a schedule that does
    // not exist.
    expect(describeRule(rule({ kind: 'once' }))[0]).toEqual({ key: 'rules.never' });
  });

  it('describes daily', () => {
    expect(describeRule(rule({ freq: 'daily' }))[0]).toEqual({ key: 'rules.daily' });
  });

  it('treats interval 0 and interval undefined as every', () => {
    // ⚠️ The server clamps `interval < 1` to 1. "every 0 days" would be
    // arithmetic about a rule that in fact fires daily.
    expect(describeRule(rule({ freq: 'every_n_days', interval: 0, start_date: '2026-08-01' }))[0]).toEqual({
      key: 'rules.daily',
    });
    expect(describeRule(rule({ freq: 'every_n_days', start_date: '2026-08-01' }))[0]).toEqual({
      key: 'rules.daily',
    });
  });

  it('describes every N days', () => {
    expect(describeRule(rule({ freq: 'every_n_days', interval: 3, start_date: '2026-08-01' }))[0]).toEqual({
      key: 'rules.everyNDays',
      vars: { n: 3 },
    });
  });

  it('says NEVER for every_n_days without an anchor', () => {
    // ⚠️ expand.go: "needs an anchor; validation enforces StartDate" — it
    // returns false rather than guessing one. Such a rule is silently dead, and
    // this line is the only place a reader can find that out.
    expect(describeRule(rule({ freq: 'every_n_days', interval: 3 }))[0]).toEqual({ key: 'rules.never' });
  });

  it('lists weekdays in week order, whatever order they arrive in', () => {
    const d = describeRule(rule({ freq: 'weekly', by_weekday: [5, 1, 3] }), 'en-US');
    expect(d[0]).toEqual({ key: 'rules.weekly', vars: { days: 'Mon, Wed, Fri' } });
  });

  it('does not reorder the caller ARRAY while sorting', () => {
    // ⚠️ A PROPERTY guard, not a mutation guard, and the difference is worth
    // recording: deleting the defensive spread in rules.ts leaves this green,
    // because the `.filter()` in the same chain already copies. This asserts the
    // observable behaviour — the caller's array survives — which is what should
    // hold however the chain is later rewritten.
    const by = [5, 1, 3];
    const r = rule({ freq: 'weekly', by_weekday: by });
    describeRule(r, 'en-US');
    expect(by).toEqual([5, 1, 3]);
  });

  it('joins with the ideographic comma in Chinese and a plain one otherwise', () => {
    const zh = describeRule(rule({ freq: 'weekly', by_weekday: [1, 3] }), 'zh-CN')[0];
    expect(String(zh!.vars!.days)).toContain('、');
    const en = describeRule(rule({ freq: 'weekly', by_weekday: [1, 3] }), 'en-US')[0];
    expect(String(en!.vars!.days)).toContain(', ');
  });

  it('falls back to the start date weekday, NOT to every day', () => {
    // ⚠️ THE weekly assertion. expand.go uses `weekdays = []int{start.Weekday()}`
    // when by_weekday is empty. Reading that as "unrestricted" would promise a
    // reader their one weekly class happens seven times a week.
    const d = describeRule(rule({ freq: 'weekly', start_date: '2026-08-12' }), 'en-US'); // a Wednesday
    expect(d[0]).toEqual({ key: 'rules.weekly', vars: { days: 'Wed' } });
  });

  it('says NEVER for weekly with neither weekdays nor an anchor', () => {
    expect(describeRule(rule({ freq: 'weekly' }))[0]).toEqual({ key: 'rules.never' });
  });

  it('says NEVER for weekly with only out-of-range weekdays', () => {
    expect(describeRule(rule({ freq: 'weekly', by_weekday: [7, -1] }))[0]).toEqual({ key: 'rules.never' });
  });

  it('describes a fortnightly rule, and refuses to without an anchor', () => {
    expect(
      describeRule(rule({ freq: 'weekly', by_weekday: [1], interval: 2, start_date: '2026-08-10' }), 'en-US')[0],
    ).toEqual({ key: 'rules.weeklyEveryN', vars: { days: 'Mon', n: 2 } });
    // Interval > 1 counts weeks FROM the anchor, so without one it never fires.
    expect(describeRule(rule({ freq: 'weekly', by_weekday: [1], interval: 2 }))[0]).toEqual({
      key: 'rules.never',
    });
  });

  it('takes the monthly day-of-month from the START DATE, not from a field', () => {
    // There is no day-of-month field; expand.go compares `d.Day() != start.Day()`.
    expect(describeRule(rule({ freq: 'monthly', start_date: '2026-08-09' }))[0]).toEqual({
      key: 'rules.monthly',
      vars: { day: 9 },
    });
  });

  it('warns that a late-month rule SKIPS short months', () => {
    // ⚠️ expand.go: "months lacking the day (e.g. the 31st) are skipped" — it
    // does not clamp to the last day. For the 29th–31st that is a materially
    // different promise, so it gets its own sentence.
    expect(describeRule(rule({ freq: 'monthly', start_date: '2026-08-31' }))[0]).toEqual({
      key: 'rules.monthlySkips',
      vars: { day: 31 },
    });
    expect(describeRule(rule({ freq: 'monthly', start_date: '2026-08-29' }))[0]!.key).toBe('rules.monthlySkips');
    expect(describeRule(rule({ freq: 'monthly', start_date: '2026-08-28' }))[0]!.key).toBe('rules.monthly');
  });

  it('says NEVER for an unknown frequency', () => {
    // A deployment newer than this build could add one. Inventing a description
    // for a cadence we do not understand is the one thing that must not happen.
    expect(describeRule(rule({ freq: 'fortnightly-ish' }))[0]).toEqual({ key: 'rules.never' });
  });
});

describe('describeRule — bounds', () => {
  it('adds an end date when there is one', () => {
    const d = describeRule(rule({ freq: 'daily', until: '2026-12-31' }));
    expect(d).toHaveLength(2);
    expect(d[1]).toEqual({ key: 'rules.until', vars: { date: '2026-12-31' } });
  });

  it('adds nothing when there is not', () => {
    expect(describeRule(rule({ freq: 'daily' }))).toHaveLength(1);
    expect(describeRule(rule({ freq: 'daily', until: null }))).toHaveLength(1);
  });

  it('never returns display text — only keys and values', () => {
    // ⚠️ The property this whole module exists for. A part carrying a sentence
    // is a part that has to be edited to add a language.
    const all = [
      rule({ kind: 'once', date: '2026-08-14' }),
      rule({ freq: 'daily', until: '2026-12-31' }),
      rule({ freq: 'weekly', by_weekday: [1, 3] }),
      rule({ freq: 'monthly', start_date: '2026-08-31' }),
      rule({ freq: 'every_n_days', interval: 3, start_date: '2026-08-01' }),
    ].flatMap((r) => describeRule(r, 'zh-CN'));
    for (const p of all) {
      expect(p.key).toMatch(/^rules\.[a-zA-Z]+$/);
      for (const v of Object.values(p.vars ?? {})) {
        // Values are dates, counts and weekday names — never a sentence.
        expect(String(v)).not.toMatch(/[，。；]/);
      }
    }
  });
});
