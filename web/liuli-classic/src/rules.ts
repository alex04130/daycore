import type { ScheduleRule } from '@daycore/core';

// 一条规则用人话怎么说。
//
// ⚠️ This module names nothing a reader sees. It returns catalogue KEYS and the
// values to interpolate, exactly like 纸屿's hourLabel — whose first draft
// returned the literal 「未定时」 and made the view compare against a Chinese
// string to decide whether to translate it. A data module that produces display
// text is a data module that has to be edited to add a language.
//
// ⚠️ And it describes what the BACKEND actually does, not what the fields look
// like they mean. internal/schedule/expand.go is the authority; every surprising
// clause below is marked with the line of it that forced the clause. A
// description that disagrees with the expansion is worse than no description —
// it is a confident wrong answer about when something will happen.

/** One clause. The view renders the parts; nothing here concatenates them,
 *  so no punctuation decision leaks out of the catalogue. */
export interface Part {
  key: string;
  vars?: Record<string, string | number>;
}

/**
 * Weekday names for a locale, indexed 0 = Sunday … 6 = Saturday.
 *
 * ⚠️ 2023-01-01 really is a Sunday — the whole index depends on it, so it is
 * asserted in the tests rather than trusted. Via Intl so that a new language is
 * still just a JSON file.
 *
 * ⚠️ `short`, not `narrow`. English narrow weekdays are S/M/T/W/T/F/S — two
 * pairs of them identical — so "every T and T" is what a reader would get.
 */
export function weekdayNames(locale: string): string[] {
  const fmt = new Intl.DateTimeFormat(locale, { weekday: 'short' });
  return Array.from({ length: 7 }, (_, i) => fmt.format(new Date(2023, 0, 1 + i, 12)));
}

/**
 * Describe when a rule fires.
 *
 * The first part is the cadence; a second part appears when the rule has an end
 * date. An inactive rule is not described as never firing — "paused" is a state
 * the reader chose, and the screen already shows it.
 */
export function describeRule(rule: ScheduleRule, locale = 'zh-CN'): Part[] {
  const parts: Part[] = [cadence(rule, locale)];
  if (rule.until) {
    // ⚠️ Inclusive — expand.go rejects only `d.After(u)`.
    parts.push({ key: 'rules.until', vars: { date: rule.until } });
  }
  return parts;
}

function cadence(rule: ScheduleRule, locale: string): Part {
  if (rule.kind === 'once') {
    // ⚠️ A once rule with no date can never fire: expand.go returns
    // `r.Date != nil && *r.Date == date`. Saying "on (nothing)" would describe a
    // schedule that does not exist.
    return rule.date ? { key: 'rules.once', vars: { date: rule.date } } : { key: 'rules.never' };
  }

  // ⚠️ interval < 1 becomes 1 server-side, so 0 and undefined both mean "every".
  // Describing `interval: 0` as "every 0 days" would be arithmetic nobody can
  // act on for a rule that in fact fires daily.
  const n = rule.interval && rule.interval >= 1 ? rule.interval : 1;
  const anchored = Boolean(rule.start_date);

  switch (rule.freq) {
    case 'daily':
      return { key: 'rules.daily' };

    case 'every_n_days':
      // ⚠️ No anchor, no occurrences — ever. expand.go: "needs an anchor;
      // validation enforces StartDate", and it returns false rather than
      // guessing one. A rule in that state is silently dead, and the only place
      // that can be visible is here.
      if (!anchored) return { key: 'rules.never' };
      return n === 1 ? { key: 'rules.daily' } : { key: 'rules.everyNDays', vars: { n } };

    case 'weekly': {
      const names = weekdayNames(locale);
      // ⚠️ An empty by_weekday falls back to the START DATE's weekday — it does
      // NOT mean "every day". Getting this backwards would tell a reader their
      // one weekly class happens seven times a week.
      let days = rule.by_weekday ?? [];
      if (days.length === 0) {
        if (!anchored) return { key: 'rules.never' };
        days = [new Date(rule.start_date! + 'T12:00:00').getDay()];
      }
      // ⚠️ Copy before sorting, because `sort` is in place and by_weekday is a
      // field of the caller's object.
      //
      // The honest footnote: TODAY this spread is not what protects it — the
      // `.filter()` below already returns a new array, so removing the spread
      // changes nothing and a mutation test proved exactly that. It stays
      // because the protection would then rest on a link in the chain that
      // exists for an unrelated reason: fold the range check into the `.map()`
      // for tidiness and the caller's array starts getting reordered, with the
      // test still green and this comment still claiming otherwise.
      const list = [...days]
        .filter((d) => d >= 0 && d <= 6)
        .sort((a, b) => a - b)
        .map((d) => names[d]!);
      if (list.length === 0) return { key: 'rules.never' };
      const vars = { days: list.join(listSeparator(locale)) };
      // Interval > 1 needs an anchor too — it counts weeks from it.
      if (n > 1) {
        return anchored
          ? { key: 'rules.weeklyEveryN', vars: { ...vars, n } }
          : { key: 'rules.never' };
      }
      return { key: 'rules.weekly', vars };
    }

    case 'monthly': {
      if (!anchored) return { key: 'rules.never' };
      const day = new Date(rule.start_date! + 'T12:00:00').getDate();
      const vars: Record<string, string | number> = { day };
      if (n > 1) return { key: 'rules.monthlyEveryN', vars: { ...vars, n } };
      // ⚠️ Months without that day are SKIPPED, not clamped to the last day
      // (expand.go: "months lacking the day (e.g. the 31st) are skipped"). For
      // the 29th–31st that is a materially different promise, so it gets its own
      // sentence rather than a footnote nobody reads.
      return day >= 29 ? { key: 'rules.monthlySkips', vars } : { key: 'rules.monthly', vars };
    }
  }
  return { key: 'rules.never' };
}

/**
 * How this language joins a short list.
 *
 * ⚠️ Not a catalogue key, because it is punctuation between values rather than
 * a sentence — and because getting it from the catalogue would mean a language
 * pack that forgot it renders 「一二三」 with no separator at all. The default is
 * the Western one; CJK gets its own ideographic comma.
 */
function listSeparator(locale: string): string {
  const base = (locale.split('-')[0] ?? locale).toLowerCase();
  return base === 'zh' || base === 'ja' ? '、' : ', ';
}
