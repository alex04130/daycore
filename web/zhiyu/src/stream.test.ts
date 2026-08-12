import { describe, expect, it } from 'vitest';
import type { DayPlan, Proposal, TimeBlock } from '@daycore/core';
import { buildStream, hourLabel, positionOf, toHM, toMin } from './stream';

const b = (o: Partial<TimeBlock> & { id: string; time: string | null }): TimeBlock => ({
  title: o.id,
  type: 'task',
  duration_min: 30,
  ...o,
});
const p = (o: Partial<Proposal> & { id: string }): Proposal => ({
  state: 'pending',
  level: 'L2',
  kind: 'card',
  title: o.id,
  ...o,
});
const day = (...blocks: TimeBlock[]): DayPlan => ({ date: '2026-08-11', blocks });

describe('buildStream', () => {
  it('orders the day by time', () => {
    const s = buildStream(day(b({ id: 'c', time: '14:00' }), b({ id: 'a', time: '09:00' })), [], toMin('10:00'));
    expect(s.entries.map((e) => e.id)).toEqual(['a', 'c']);
  });

  // ⚠️ A suggestion belongs AT the time it is about — that is what lets this
  // paradigm show one without interrupting. Appending them rebuilds the
  // notification list the design exists to replace.
  it('interleaves proposals by time rather than appending them', () => {
    const s = buildStream(
      day(b({ id: 'morning', time: '09:00' }), b({ id: 'evening', time: '21:00' })),
      [p({ id: 'suggest', start: '20:00' })],
      toMin('10:00'),
    );
    expect(s.entries.map((e) => e.id)).toEqual(['morning', 'suggest', 'evening']);
  });

  // ⚠️ Untimed entries go LAST. Sorting null as 0 puts every "someday" task
  // above the 07:00 alarm — the top of the ledger means "earliest today", not
  // "unscheduled".
  it('puts untimed entries at the end, not at midnight', () => {
    const s = buildStream(
      day(b({ id: 'someday', time: null }), b({ id: 'alarm', time: '07:00' })),
      [],
      toMin('10:00'),
    );
    expect(s.entries.map((e) => e.id)).toEqual(['alarm', 'someday']);
  });

  // ⚠️ A proposal about a block sorts after it, or the note reads as being
  // about the entry above it.
  // ⚠️ The ids are chosen so ALPHABETICAL order is wrong: 'a-move' would sort
  // above 'z-class' on the id tiebreak alone. The first version of this test
  // used 'class' and 'move-it', which sort correctly by accident — deleting the
  // kind rule left it green. A tiebreak test whose inputs already tie-break
  // themselves asserts nothing.
  it('sorts a proposal after the block it shares a minute with', () => {
    const s = buildStream(
      day(b({ id: 'z-class', time: '14:00' })),
      [p({ id: 'a-move', start: '14:00' })],
      toMin('10:00'),
    );
    expect(s.entries.map((e) => e.id)).toEqual(['z-class', 'a-move']);
  });

  it('never shows a settled proposal', () => {
    const s = buildStream(day(), [p({ id: 'answered', start: '10:00', state: 'accepted' })], toMin('09:00'));
    expect(s.entries).toHaveLength(0);
  });

  it('drops tombstoned blocks', () => {
    const s = buildStream(day(b({ id: 'gone', time: '09:00', hidden: true })), [], toMin('10:00'));
    expect(s.entries).toHaveLength(0);
  });
});

describe('the 现在 line', () => {
  it('sits before the first entry still ahead', () => {
    const s = buildStream(
      day(b({ id: 'past', time: '08:00' }), b({ id: 'soon', time: '11:00' })),
      [],
      toMin('10:00'),
    );
    expect(s.nowIndex).toBe(1);
    expect(positionOf(s, 0)).toBe('past');
    expect(positionOf(s, 1)).toBe('future');
  });

  it('sits at the very end when the whole day is behind you', () => {
    const s = buildStream(day(b({ id: 'x', time: '08:00' })), [], toMin('23:00'));
    expect(s.nowIndex).toBe(s.entries.length);
    expect(positionOf(s, 0)).toBe('past');
  });

  // ⚠️ An entry starting EXACTLY now has not happened yet. Off by that one
  // minute and the thing you are about to do renders as already behind you —
  // once a day, for one minute, which is the hardest kind of bug to be told
  // about.
  it('puts an entry starting exactly now on the future side', () => {
    const s = buildStream(day(b({ id: 'starting', time: '10:00' })), [], toMin('10:00'));
    expect(s.nowIndex).toBe(0);
    expect(positionOf(s, 0)).toBe('future');
  });

  it('sits at the very top before the day starts', () => {
    const s = buildStream(day(b({ id: 'x', time: '08:00' })), [], toMin('06:00'));
    expect(s.nowIndex).toBe(0);
    expect(positionOf(s, 0)).toBe('future');
  });

  // ⚠️ Untimed entries must not pull the line to the end. They have no time, so
  // they are neither past nor future — and treating them as "now" would put the
  // 现在 line above the unscheduled pile every single day.
  it('is not dragged by untimed entries', () => {
    const s = buildStream(
      day(b({ id: 'past', time: '08:00' }), b({ id: 'someday', time: null })),
      [],
      toMin('10:00'),
    );
    // Everything timed is behind us, so the line goes after the timed ones —
    // which here is index 1, the position of `someday`.
    expect(s.nowIndex).toBe(2);
  });
});

describe('counts', () => {
  it('counts only timed blocks, and only completed ones as done', () => {
    const s = buildStream(
      day(
        b({ id: 'a', time: '09:00', completed: true }),
        b({ id: 'b', time: '10:00' }),
        b({ id: 'c', time: null }),
      ),
      [p({ id: 'prop', start: '11:00' })],
      toMin('12:00'),
    );
    expect(s.doneCount).toBe(1);
    // ⚠️ Not 3 and not 4: an untimed task is not part of "today's plan" you can
    // be behind on, and a proposal is not something you have failed to do.
    expect(s.total).toBe(2);
  });
});

describe('hourLabel', () => {
  it('labels the first entry of each hour and nothing else', () => {
    const s = buildStream(
      day(b({ id: 'a', time: '09:00' }), b({ id: 'b', time: '09:40' }), b({ id: 'c', time: '11:00' })),
      [],
      toMin('08:00'),
    );
    expect(hourLabel(s, 0)).toEqual({ kind: 'hour', text: '09:00' });
    expect(hourLabel(s, 1)).toBeNull();
    expect(hourLabel(s, 2)).toEqual({ kind: 'hour', text: '11:00' });
  });

  it('labels the untimed pile once', () => {
    const s = buildStream(day(b({ id: 'x', time: null }), b({ id: 'y', time: null })), [], toMin('08:00'));
    // ⚠️ A STRUCTURE, not a display string. Returning the Chinese literal here
    // put a display concern in a data module and made the view compare against
    // it — a comparison that stops matching the day somebody edits the wording.
    expect(hourLabel(s, 0)).toEqual({ kind: 'untimed' });
    expect(hourLabel(s, 1)).toBeNull();
  });
});

describe('toHM', () => {
  it('wraps rather than printing 25:00', () => {
    expect(toHM(toMin('23:30') + 60)).toBe('00:30');
  });
});
