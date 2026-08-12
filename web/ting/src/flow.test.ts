import { describe, expect, it } from 'vitest';
import { flowAt, progressPct, toHM, toMin } from './flow';
import type { DayPlan, TimeBlock } from './api';

const b = (o: Partial<TimeBlock> & { id: string; time: string | null }): TimeBlock => ({
  title: o.id,
  type: 'task',
  duration_min: 60,
  ...o,
});
const day = (...blocks: TimeBlock[]): DayPlan => ({ date: '2026-08-11', blocks });

describe('flowAt', () => {
  it('picks the block that covers right now', () => {
    const f = flowAt(day(b({ id: 'a', time: '09:00' }), b({ id: 'c', time: '14:00' })), toMin('09:30'));
    expect(f.current?.id).toBe('a');
    expect(f.next?.id).toBe('c');
    expect(f.gapMin).toBe(270);
  });

  // ⚠️ A block with no duration is a POINT, not an open interval. Treated as
  // open-ended, one undated task swallows the rest of the day and 汀 answers
  // "what now" with the same thing forever — the exact failure this screen
  // cannot have, since it only ever shows one thing.
  it('does not let a zero-duration block swallow the day', () => {
    const f = flowAt(
      day(b({ id: 'point', time: '09:00', duration_min: null }), b({ id: 'later', time: '14:00' })),
      toMin('11:00'),
    );
    expect(f.current?.id).not.toBe('point');
    expect(f.next?.id).toBe('later');
  });

  // ⚠️ An overdue block is still the honest answer to "what now". 汀 must not
  // pretend a thing stopped existing at its end time — that is how a day
  // quietly loses the task somebody was actually in the middle of.
  it('keeps showing an unfinished block after its end time', () => {
    const f = flowAt(day(b({ id: 'over', time: '09:00', duration_min: 30 })), toMin('12:00'));
    expect(f.current?.id).toBe('over');
    expect(f.doneForToday).toBe(false);
  });

  it('reports nothing left once everything is finished', () => {
    const f = flowAt(day(b({ id: 'x', time: '09:00', completed: true })), toMin('12:00'));
    expect(f.current).toBeNull();
    expect(f.doneForToday).toBe(true);
    expect(f.doneCount).toBe(1);
    expect(f.total).toBe(1);
  });

  // Tombstones and unscheduled blocks are not "now" for anybody.
  it('ignores hidden and unscheduled blocks', () => {
    const f = flowAt(
      day(b({ id: 'ghost', time: '09:00', hidden: true }), b({ id: 'someday', time: null })),
      toMin('09:30'),
    );
    expect(f.current).toBeNull();
    expect(f.total).toBe(0);
  });

  it('survives an empty day', () => {
    const f = flowAt(null, toMin('09:30'));
    expect(f.current).toBeNull();
    expect(f.doneForToday).toBe(true);
  });

  // Blocks arrive in whatever order the API returns them.
  it('does not depend on the order the API returned', () => {
    const f = flowAt(day(b({ id: 'late', time: '14:00' }), b({ id: 'early', time: '09:00' })), toMin('09:30'));
    expect(f.current?.id).toBe('early');
    expect(f.next?.id).toBe('late');
  });
});

describe('progressPct', () => {
  it('never reads as 0% or over 100%', () => {
    const x = b({ id: 'x', time: '09:00', duration_min: 60 });
    expect(progressPct(x, toMin('09:00'))).toBe(2); // just started, still visible
    expect(progressPct(x, toMin('09:30'))).toBe(50);
    expect(progressPct(x, toMin('11:00'))).toBe(100); // overdue, not 200
  });
});

describe('toHM', () => {
  it('wraps past midnight instead of printing 25:00', () => {
    expect(toHM(toMin('23:30') + 60)).toBe('00:30');
    expect(toHM(-30)).toBe('23:30');
  });
});
