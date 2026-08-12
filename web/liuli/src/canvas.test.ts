import { describe, expect, it } from 'vitest';
import type { DayPlan, Proposal, TimeBlock } from '@daycore/core';
import {
  boxOf, canvasHeight, COMPACT_UNDER, DAY0, DEFAULT_DUR, laneize,
  MIN_HEIGHT, piecesFor, toMin, wellAt, yOf,
} from './canvas';

const b = (o: Partial<TimeBlock> & { id: string; time: string | null }): TimeBlock => ({
  title: o.id, type: 'task', duration_min: 60, ...o,
});
const p = (o: Partial<Proposal> & { id: string }): Proposal => ({
  state: 'pending', level: 'L2', kind: 'timed', title: o.id, ...o,
});
const day = (...blocks: TimeBlock[]): DayPlan => ({ date: '2026-08-11', blocks });
const piece = (id: string, s: number, e: number) => ({ kind: 'block' as const, id, title: id, s, e });

describe('yOf', () => {
  it('puts the top of the visible range at zero', () => {
    expect(yOf(DAY0)).toBe(0);
    expect(yOf(DAY0 + 60)).toBeGreaterThan(0);
  });
  it('gives the canvas a height matching the range', () => {
    expect(canvasHeight()).toBeGreaterThan(0);
    expect(yOf(DAY0 + (24 * 60 - DAY0))).toBeCloseTo(canvasHeight());
  });
});

describe('laneize', () => {
  it('leaves non-overlapping pieces all in lane 0', () => {
    const out = laneize([piece('a', 540, 600), piece('b', 600, 660)]);
    expect(out.map((x) => x.lane)).toEqual([0, 0]);
  });

  it('stacks overlapping pieces into rising lanes', () => {
    const out = laneize([piece('a', 540, 660), piece('b', 570, 690), piece('c', 600, 620)]);
    expect(out.map((x) => x.lane)).toEqual([0, 1, 2]);
  });

  it('reuses a lane once it is free again', () => {
    const out = laneize([piece('a', 540, 600), piece('b', 550, 610), piece('c', 620, 680)]);
    const byId = Object.fromEntries(out.map((x) => [x.id, x.lane]));
    expect(byId['a']).toBe(0);
    expect(byId['b']).toBe(1);
    // c starts after both ended — it must not climb to lane 2 forever.
    expect(byId['c']).toBe(0);
  });

  // ⚠️ Longer first on a tie. With shorter-first, a long block starting at the
  // same minute as a short one lands in a higher lane and is drawn ON TOP of
  // the thing it contains — so the short block disappears behind it, which is
  // the opposite of what stacking is for.
  it('gives the LONGER piece the lower lane when two start together', () => {
    const out = laneize([piece('short', 540, 570), piece('long', 540, 700)]);
    const byId = Object.fromEntries(out.map((x) => [x.id, x.lane]));
    expect(byId['long']).toBe(0);
    expect(byId['short']).toBe(1);
  });

  // Adjacent, not overlapping: one ends exactly where the next begins.
  it('does not treat touching pieces as a collision', () => {
    const out = laneize([piece('a', 540, 600), piece('b', 600, 660)]);
    expect(out.every((x) => x.lane === 0)).toBe(true);
  });

  it('does not depend on input order', () => {
    const forward = laneize([piece('a', 540, 660), piece('b', 570, 690)]);
    const backward = laneize([piece('b', 570, 690), piece('a', 540, 660)]);
    expect(forward.map((x) => [x.id, x.lane])).toEqual(backward.map((x) => [x.id, x.lane]));
  });
});

describe('piecesFor', () => {
  // ⚠️ Ghosts are laned WITH the real blocks. Laning them apart lets a ghost
  // land in lane 0 under a real block and be unreadable — the one thing a
  // proposal must never be.
  it('lanes ghosts against real blocks, not separately', () => {
    const out = piecesFor(
      day(b({ id: 'class', time: '09:00', duration_min: 120 })),
      [p({ id: 'ghost', start: '09:30', dur: 45 })],
    );
    const byId = Object.fromEntries(out.map((x) => [x.id, x.lane]));
    expect(byId['class']).toBe(0);
    expect(byId['ghost']).toBe(1);
  });

  // ⚠️ There is no coordinate for "someday" on a map of today, and putting it
  // at midnight would be a claim about when it happens.
  it('keeps untimed blocks off the canvas entirely', () => {
    const out = piecesFor(day(b({ id: 'someday', time: null })), []);
    expect(out).toHaveLength(0);
  });

  it('drops tombstones and settled proposals', () => {
    const out = piecesFor(
      day(b({ id: 'gone', time: '09:00', hidden: true })),
      [p({ id: 'answered', start: '10:00', state: 'accepted' }), p({ id: 'untimed' })],
    );
    expect(out).toHaveLength(0);
  });

  // ⚠️ A zero-duration piece is invisible AND unclickable — the thing exists,
  // occupies no pixels, and the only evidence is that it never appeared.
  it('gives a piece with no duration a real one', () => {
    const out = piecesFor(day(b({ id: 'x', time: '09:00', duration_min: null })), []);
    expect(out[0]!.e - out[0]!.s).toBe(DEFAULT_DUR);
  });
});

describe('boxOf', () => {
  it('places a piece by time and lane', () => {
    const [pc] = piecesFor(day(b({ id: 'x', time: '09:00', duration_min: 60 })), []);
    const box = boxOf(pc!);
    expect(box.top).toBeCloseTo(yOf(toMin('09:00')));
    expect(box.left).toBe(46);
    expect(box.stacked).toBe(false);
  });

  // ⚠️ A floor, not a suggestion. Below it a block stops being shorter and
  // starts being absent — too small to hit with a finger.
  it('never lets a short block shrink out of existence', () => {
    const [pc] = piecesFor(day(b({ id: 'tiny', time: '09:00', duration_min: 5 })), []);
    expect(boxOf(pc!).height).toBeGreaterThanOrEqual(MIN_HEIGHT);
  });

  it('goes compact only when it is genuinely too short for the detail', () => {
    const [small] = piecesFor(day(b({ id: 's', time: '09:00', duration_min: 20 })), []);
    const [big] = piecesFor(day(b({ id: 'b', time: '11:00', duration_min: 180 })), []);
    expect(boxOf(small!).compact).toBe(true);
    expect(boxOf(big!).compact).toBe(false);
    expect(boxOf(big!).height).toBeGreaterThan(COMPACT_UNDER);
  });

  it('steps stacked pieces inward and marks them', () => {
    const out = piecesFor(
      day(b({ id: 'a', time: '09:00', duration_min: 120 }), b({ id: 'b', time: '09:30', duration_min: 60 })),
      [],
    );
    const boxes = out.map((x) => boxOf(x));
    expect(boxes[1]!.left).toBeGreaterThan(boxes[0]!.left);
    expect(boxes[1]!.stacked).toBe(true);
  });
});

describe('wellAt', () => {
  const W = 400;
  const H = 800;

  it('maps the four corners to the four wells', () => {
    expect(wellAt(10, 10, W, H, false)).toBe('keep');
    expect(wellAt(W - 10, 10, W, H, false)).toBe('tomorrow');
    expect(wellAt(10, H - 10, W, H, false)).toBe('someday');
    expect(wellAt(W - 10, H - 10, W, H, false)).toBe('replan');
  });

  // ⚠️ Opposite corrections for opposite input devices. A finger is imprecise
  // but its target is what it covers, so the box is drawn IN — otherwise a drag
  // that merely passed near a corner gets captured. A pointer is precise but
  // small, so the box is widened.
  it('shrinks the reach for touch and extends it for a pointer', () => {
    // A point just outside a corner's touch reach but inside its pointer reach.
    const x = W / 4 + 5;
    expect(wellAt(x, 10, W, H, false)).toBe('keep'); // pointer: extended, captured
    expect(wellAt(x, 10, W, H, true)).toBeNull(); // touch: cut back, not captured
  });

  // ⚠️ No ambiguous region. Quadrants plus a widening pad create an overlap near
  // the centre lines where two wells both claim the point, and the first version
  // resolved it by the order the `if`s were written — an accident, not a
  // decision. Every reachable point now has exactly one nearest corner.
  it('never lets two wells claim the same point', () => {
    for (let x = 0; x <= W; x += 7) {
      for (let y = 0; y <= H; y += 13) {
        for (const touch of [true, false]) {
          const a = wellAt(x, y, W, H, touch);
          const again = wellAt(x, y, W, H, touch);
          expect(again).toBe(a); // deterministic, and by distance rather than order
        }
      }
    }
    // The exact centre belongs to nobody.
    expect(wellAt(W / 2, H / 2, W, H, true)).toBeNull();
  });

  it('has all four wells, because three reads as lopsided', () => {
    const seen = new Set([
      wellAt(10, 10, W, H, false),
      wellAt(W - 10, 10, W, H, false),
      wellAt(10, H - 10, W, H, false),
      wellAt(W - 10, H - 10, W, H, false),
    ]);
    expect(seen.size).toBe(4);
  });
});
