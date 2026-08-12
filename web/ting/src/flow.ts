import type { DayPlan, TimeBlock } from './api';

// "What is happening right now" — derived from the day's blocks, in the
// browser's own clock.
//
// ⚠️ Derived on the client, and that is a deliberate limit rather than an
// oversight. The backend has a richer notion of the same thing (petrification,
// locks, rhythm) and 汀 asks it before it WRITES; this is only for deciding
// which of the day's blocks to put on screen. Doing that server-side would
// mean a round trip every minute to answer a question the client can answer
// from data it already has, and a screen that goes stale when the network does.
//
// What that costs, stated: a device whose clock is wrong shows the wrong
// "now". The alternative costs a request per tick, and a wrong clock breaks far
// more than this screen.

export function toMin(hhmm: string): number {
  const [h = '0', m = '0'] = hhmm.split(':');
  return Number(h) * 60 + Number(m);
}

export function toHM(min: number): string {
  const m = ((min % 1440) + 1440) % 1440;
  return String(Math.floor(m / 60)).padStart(2, '0') + ':' + String(m % 60).padStart(2, '0');
}

export function nowMin(now = new Date()): number {
  return now.getHours() * 60 + now.getMinutes();
}

export interface Flow {
  /** The block covering right now, if any. */
  current: TimeBlock | null;
  /** The next one that has not started. */
  next: TimeBlock | null;
  /** Minutes until `next` starts. */
  gapMin: number;
  /** Nothing left today. */
  doneForToday: boolean;
  /** How many of today's blocks are finished. */
  doneCount: number;
  total: number;
}

/** Blocks worth showing: scheduled, not hidden, not finished. */
function live(plan: DayPlan | null): TimeBlock[] {
  if (!plan) return [];
  return plan.blocks
    .filter((b) => !b.hidden && b.time !== null)
    .sort((a, b) => toMin(a.time!) - toMin(b.time!));
}

export function flowAt(plan: DayPlan | null, atMin: number): Flow {
  const all = live(plan);
  const doneCount = all.filter((b) => b.completed).length;
  const open = all.filter((b) => !b.completed);

  let current: TimeBlock | null = null;
  let next: TimeBlock | null = null;
  for (const b of open) {
    const start = toMin(b.time!);
    // ⚠️ A block with no duration is treated as a POINT, not as open-ended.
    // Open-ended would make one undated task swallow the rest of the day and
    // 汀 would answer "what now" with the same thing forever.
    const end = start + (b.duration_min ?? 0);
    if (current === null && atMin >= start && atMin < end) {
      current = b;
      continue;
    }
    // ⚠️ NO early exit once `current` is found. The first draft broke out of
    // the loop here, and since the list is sorted ascending that meant `next`
    // was never found WHILE SOMETHING WAS IN PROGRESS — precisely when the
    // footer's "接下来" line and the gap face need it. Caught by a test, not by
    // reading: every individual case looked right.
    if (start > atMin && next === null) next = b;
  }
  if (current === null && next === null) {
    // Everything is behind us. The most recent unfinished one is still the
    // honest answer to "what now" — 汀 does not pretend an overdue thing
    // stopped existing at its end time.
    const past = open.filter((b) => toMin(b.time!) <= atMin);
    current = past.length ? past[past.length - 1]! : null;
  }

  return {
    current,
    next,
    gapMin: next ? Math.max(0, toMin(next.time!) - atMin) : 0,
    doneForToday: current === null && next === null,
    doneCount,
    total: all.length,
  };
}

/** How far through the current block we are, 2..100. */
export function progressPct(b: TimeBlock, atMin: number): number {
  const start = toMin(b.time!);
  const dur = b.duration_min ?? 0;
  if (dur <= 0) return 100;
  return Math.max(2, Math.min(100, Math.round(((atMin - start) / dur) * 100)));
}
