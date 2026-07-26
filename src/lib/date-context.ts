/**
 * Build a fully-resolved date context to pass to AI prompts.
 * All relative terms (明天, 后天, 大后天, 这周X, 下周X) are pre-computed
 * server-side so the AI never has to do arithmetic.
 */
export function buildDateContext(
  clientDate: string,    // YYYY-MM-DD — the date the user is *viewing/editing* (may differ from today)
  clientWeekday: string, // 星期X from client
  clientTime: string,    // HH:MM from client
  clientTimezone: string // IANA tz from client
) {
  // 用客户端传来的真实今天日期作为"相对时间"的基准，而不是服务端 new Date()
  // 这样"明天"/"后天"永远相对于用户本地的今天，不受服务器时区影响
  const [cy, cm, cd] = clientDate.split("-").map(Number);
  const base = new Date(cy, cm - 1, cd);

  const WEEKDAYS = ["星期日","星期一","星期二","星期三","星期四","星期五","星期六"];

  function fmtDate(dt: Date) {
    const yy = dt.getFullYear();
    const mm = String(dt.getMonth() + 1).padStart(2, "0");
    const dd = String(dt.getDate()).padStart(2, "0");
    return `${yy}-${mm}-${dd}`;
  }

  function addDays(dt: Date, n: number) {
    const r = new Date(dt); r.setDate(r.getDate() + n); return r;
  }

  const tomorrow      = addDays(base, 1);
  const dayAfter      = addDays(base, 2);
  const dayAfter3     = addDays(base, 3);

  // Next weekday dates (本周 = this week's remaining days, 下周 = next week)
  const currentDow = base.getDay(); // 0=Sun...6=Sat
  function nextWeekdayDate(targetDow: number, nextWeek = false): string {
    let diff = targetDow - currentDow;
    if (nextWeek) {
      diff = diff <= 0 ? diff + 7 : diff + 7;
    } else {
      if (diff <= 0) diff += 7;
    }
    return fmtDate(addDays(base, diff));
  }

  const weekMap: Record<string, string> = {
    "这周一": nextWeekdayDate(1), "本周一": nextWeekdayDate(1),
    "这周二": nextWeekdayDate(2), "本周二": nextWeekdayDate(2),
    "这周三": nextWeekdayDate(3), "本周三": nextWeekdayDate(3),
    "这周四": nextWeekdayDate(4), "本周四": nextWeekdayDate(4),
    "这周五": nextWeekdayDate(5), "本周五": nextWeekdayDate(5),
    "这周六": nextWeekdayDate(6), "本周六": nextWeekdayDate(6),
    "这周日": nextWeekdayDate(0), "本周日": nextWeekdayDate(0),
    "下周一": nextWeekdayDate(1, true), "下周二": nextWeekdayDate(2, true),
    "下周三": nextWeekdayDate(3, true), "下周四": nextWeekdayDate(4, true),
    "下周五": nextWeekdayDate(5, true), "下周六": nextWeekdayDate(6, true),
    "下周日": nextWeekdayDate(0, true),
  };

  const weekMapLines = Object.entries(weekMap)
    .map(([k, v]) => `  - "${k}" → ${v}（${WEEKDAYS[new Date(v + "T12:00:00").getDay()]}）`)
    .join("\n");

  // Build the relative date map TABLE that gets injected into the prompt
  const relativeDateMap = [
    `| 明天 | ${fmtDate(tomorrow)}（${WEEKDAYS[tomorrow.getDay()]}） |`,
    `| 后天 | ${fmtDate(dayAfter)}（${WEEKDAYS[dayAfter.getDay()]}） |`,
    `| 大后天 | ${fmtDate(dayAfter3)}（${WEEKDAYS[dayAfter3.getDay()]}） |`,
    ...Array.from({ length: 7 }, (_, i) => {
      const dow = i; // 0=Sun
      const thisWeek = nextWeekdayDate(dow, false);
      const nextWeek = nextWeekdayDate(dow, true);
      const label = WEEKDAYS[dow];
      const thisLabel = `本周${label.replace("星期", "")}`;
      const nextLabel = `下周${label.replace("星期", "")}`;
      return [
        `| ${thisLabel} | ${fmtDate(thisWeek)}（${label}） |`,
        `| ${nextLabel} | ${fmtDate(nextWeek)}（${label}） |`,
      ].join("\n");
    }),
  ].join("\n");

  return {
    date:               clientDate,
    weekday:            clientWeekday,
    time:               clientTime,
    timezone:           clientTimezone,
    tomorrow:           fmtDate(tomorrow),
    tomorrowWeekday:    WEEKDAYS[tomorrow.getDay()],
    dayAfterTomorrow:   fmtDate(dayAfter),
    dayAfterTomorrowWD: WEEKDAYS[dayAfter.getDay()],
    dayAfter3:          fmtDate(dayAfter3),
    dayAfter3WD:        WEEKDAYS[dayAfter3.getDay()],
    weekMapLines,
    relativeDateMap,   // ← 新增：完整对照表字符串，直接注入 prompt
  };
}

export type DateContext = ReturnType<typeof buildDateContext>;
