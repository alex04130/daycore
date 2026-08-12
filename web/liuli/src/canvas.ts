import type { DayPlan, Proposal, TimeBlock } from '@daycore/core';

// 时间是一块连续的空间画布。不是列表、不是聊天记录 —— 是一张可以拖、可以捏合
// 的图。
//
// # The paradigm, as geometry
//
// 汀 answers "what now" with one thing. 纸屿 answers it with a position in a
// stream. 琉璃 answers it with a COORDINATE: every block has a y, a height, and
// a lane, and the day is a map you look at.
//
// ⚠️ Third frontend, third idea of "now", and none of them borrowed. That is
// the evidence the shared layer stopped at the right line: what these three
// have in common is how they TALK to the backend, not what they think a day is.

/** The visible window, in minutes from midnight. 06:00–24:00 by default. */
export const DAY0 = 6 * 60;
export const DAY1 = 24 * 60;

/** Pixels per minute at zoom 1. */
export const PXM_BASE = 1.1;

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

/** Where a minute sits on the canvas. */
export function yOf(min: number, pxm = PXM_BASE): number {
  return (min - DAY0) * pxm;
}

/** The canvas height for the visible range. */
export function canvasHeight(pxm = PXM_BASE): number {
  return (DAY1 - DAY0) * pxm;
}

export type PieceKind = 'block' | 'ghost';

/** One thing on the canvas: a real block, or a proposal's ghost. */
export interface Piece {
  kind: PieceKind;
  id: string;
  title: string;
  /** Start, in minutes from midnight. */
  s: number;
  /** End. ⚠️ Never equal to s — see DEFAULT_DUR. */
  e: number;
  lane: number;
  block?: TimeBlock;
  proposal?: Proposal;
}

/**
 * The duration a piece gets when the backend does not give one.
 *
 * ⚠️ Not zero. A zero-height block is invisible and unclickable — the thing is
 * on the canvas, occupies no pixels, and the user's only evidence it exists is
 * that something they created never appeared. 45 minutes matches the
 * prototype's `o.dur || 45`.
 */
export const DEFAULT_DUR = 45;

/**
 * Assign overlap lanes.
 *
 * Blocks that overlap in time stack rather than sitting side by side — the
 * paradigm's own choice (see HANDOFF 02 §2): a stacked card reads as "these
 * collide", two narrow columns read as "these are both fine".
 *
 * ⚠️ Sorted by start, then by LONGER FIRST. The tie-break is not cosmetic: with
 * shorter-first, a long block starting at the same minute as a short one lands
 * in a higher lane and is drawn on top of the thing it contains — so the short
 * block disappears behind the long one, which is the opposite of what stacking
 * is for.
 *
 * ⚠️ Ghosts are laned WITH the real blocks, not separately. Laning them apart
 * would let a ghost land in lane 0 under a real block and be unreadable, which
 * is the one thing a proposal must never be.
 */
export function laneize(pieces: Omit<Piece, 'lane'>[]): Piece[] {
  const arr = pieces
    .map((p) => ({ ...p, lane: 0 }))
    .sort((a, b) => a.s - b.s || b.e - a.e || (a.id < b.id ? -1 : 1));
  const lanes: Piece[][] = [];
  for (const it of arr) {
    let i = 0;
    while ((lanes[i] ?? []).some((o) => o.s < it.e && it.s < o.e)) i++;
    (lanes[i] ??= []).push(it);
    it.lane = i;
  }
  return arr;
}

/** Fold a day and its pending proposals into laned canvas pieces. */
export function piecesFor(plan: DayPlan | null, proposals: Proposal[]): Piece[] {
  const out: Omit<Piece, 'lane'>[] = [];
  for (const b of plan?.blocks ?? []) {
    // ⚠️ Untimed blocks are NOT on the canvas. There is no coordinate for
    // "someday" on a map of today, and putting them at midnight would be a
    // claim about when they happen. They belong in a drawer this module does
    // not own.
    if (b.hidden || b.time === null) continue;
    const s = toMin(b.time);
    out.push({
      kind: 'block',
      id: b.id,
      title: b.title,
      s,
      e: s + (b.duration_min || DEFAULT_DUR),
      block: b,
    });
  }
  for (const p of proposals) {
    if (p.state !== 'pending' || !p.start) continue;
    const s = toMin(p.start);
    out.push({
      kind: 'ghost',
      id: p.id,
      title: p.title,
      s,
      e: s + (p.dur || DEFAULT_DUR),
      proposal: p,
    });
  }
  return laneize(out);
}

export interface Box {
  top: number;
  height: number;
  left: number;
  /** Compact mode hides the duration line and the tags. */
  compact: boolean;
  /** Stacked pieces get a shadow so the collision reads. */
  stacked: boolean;
}

/** Geometry for one piece. */
export function boxOf(p: Piece, pxm = PXM_BASE): Box {
  const height = Math.max(MIN_HEIGHT, (p.e - p.s) * pxm - 2);
  return {
    top: yOf(p.s, pxm),
    height,
    left: GUTTER + p.lane * LANE_STEP,
    compact: height < COMPACT_UNDER,
    stacked: p.lane > 0,
  };
}

/** The time gutter's width, and how far each lane steps in. */
export const GUTTER = 46;
export const LANE_STEP = 18;

/**
 * ⚠️ A floor, not a suggestion. A 5-minute block at any sane zoom is a few
 * pixels tall — too small to hit with a finger, and the user's experience is
 * "it is not there". Below this the block stops being shorter and starts being
 * absent.
 */
export const MIN_HEIGHT = 30;
export const COMPACT_UNDER = 48;

/** The gravity wells, in reading order. 2×2 — an odd number reads as lopsided. */
export const WELLS = ['keep', 'tomorrow', 'someday', 'replan'] as const;
export type Well = (typeof WELLS)[number];

/**
 * Which well a drop landed in, or null.
 *
 * ⚠️ NEAREST corner within tolerance, not a quadrant test. Quadrants plus a
 * widening pad create an OVERLAP zone near the centre lines where two wells
 * both claim the point — and the first version resolved that by the order the
 * `if`s happened to be written, which is an accident rather than a decision.
 * Nearest-corner has no ambiguous region: widening makes a target easier to
 * hit, it does not make it win ties.
 *
 * ⚠️ `pad` differs by input, and the corrections point opposite ways. A finger
 * is imprecise but its target is what it COVERS, so the reach is cut — otherwise
 * a drag that merely passed near a corner gets captured on the way past. A
 * pointer is precise but small, so the reach is extended. Same geometry,
 * opposite corrections — see HANDOFF 02 §3.
 */
export function wellAt(
  x: number,
  y: number,
  w: number,
  h: number,
  touch: boolean,
): Well | null {
  const pad = touch ? -10 : 18;
  // A corner's pull reaches a quarter of each axis, adjusted by the input.
  const reachX = w / 4 + pad;
  const reachY = h / 4 + pad;
  if (reachX <= 0 || reachY <= 0) return null;

  const corners: { well: Well; cx: number; cy: number }[] = [
    { well: 'keep', cx: 0, cy: 0 },
    { well: 'tomorrow', cx: w, cy: 0 },
    { well: 'someday', cx: 0, cy: h },
    { well: 'replan', cx: w, cy: h },
  ];

  let best: Well | null = null;
  let bestD = Infinity;
  for (const c of corners) {
    const dx = Math.abs(x - c.cx);
    const dy = Math.abs(y - c.cy);
    if (dx > reachX || dy > reachY) continue;
    // Normalised, so a tall screen does not make the vertical axis dominate the
    // choice between two corners that are equally "in reach".
    const d = (dx / reachX) ** 2 + (dy / reachY) ** 2;
    if (d < bestD) {
      bestD = d;
      best = c.well;
    }
  }
  return best;
}
