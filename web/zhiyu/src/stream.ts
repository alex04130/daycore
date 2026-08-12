import type { DayPlan, Proposal, TimeBlock } from '@daycore/core';

// 一天是一篇由你和小禾共同书写的日志。向上滚是过去，向下滚是预告。
//
// # The paradigm, as data
//
// 汀 answers "what now" with ONE thing. 纸屿 answers it with a POSITION in a
// single ordered stream — the same day, seen as a ledger you scroll through.
// Same backend, same blocks; a different question asked of them.
//
// ⚠️ This is why "what is now" did NOT go into @daycore/core. Sharing it would
// have meant one of these two paradigms borrowing the other's idea of the
// present, and the borrowed one is always subtly wrong: 汀 needs "the single
// thing to show", 纸屿 needs "where the line goes".

export type EntryKind = 'block' | 'proposal';

export interface Entry {
  kind: EntryKind;
  id: string;
  /** Minutes from midnight. Entries with no time sit at the end — see below. */
  at: number | null;
  block?: TimeBlock;
  proposal?: Proposal;
}

export interface Stream {
  entries: Entry[];
  /**
   * Index of the first entry at or after now — where the 「现在」 line is drawn.
   * Equal to entries.length when the whole day is behind you.
   */
  nowIndex: number;
  doneCount: number;
  total: number;
}

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

/**
 * Fold the day and its pending proposals into one ordered stream.
 *
 * ⚠️ Proposals are interleaved BY TIME, not appended. A suggestion for 20:00
 * belongs at 20:00 in the ledger — that is the whole reason this paradigm can
 * show a suggestion without interrupting: it is a note left at the place it is
 * about. Appending them would rebuild the notification list this design exists
 * to replace.
 */
export function buildStream(
  plan: DayPlan | null,
  proposals: Proposal[],
  atMin: number,
): Stream {
  const blocks = (plan?.blocks ?? []).filter((b) => !b.hidden);
  const entries: Entry[] = [];

  for (const b of blocks) {
    entries.push({
      kind: 'block',
      id: b.id,
      at: b.time === null ? null : toMin(b.time),
      block: b,
    });
  }
  for (const p of proposals) {
    if (p.state !== 'pending') continue;
    entries.push({
      kind: 'proposal',
      id: p.id,
      at: p.start ? toMin(p.start) : null,
      proposal: p,
    });
  }

  entries.sort((x, y) => {
    // ⚠️ Untimed entries go LAST, not at midnight. Sorting null as 0 would put
    // every "someday" task above the 07:00 alarm — the top of the ledger, which
    // is the position that means "earliest today", not "unscheduled".
    if (x.at === null && y.at === null) return x.id < y.id ? -1 : 1;
    if (x.at === null) return 1;
    if (y.at === null) return -1;
    if (x.at !== y.at) return x.at - y.at;
    // ⚠️ At the same minute, a PROPOSAL sorts after the block it is about.
    // Otherwise a suggestion to move something appears above the thing it wants
    // to move, and the note reads as being about the entry before it.
    if (x.kind !== y.kind) return x.kind === 'proposal' ? 1 : -1;
    return x.id < y.id ? -1 : 1;
  });

  let nowIndex = entries.length;
  for (let i = 0; i < entries.length; i++) {
    const at = entries[i]!.at;
    if (at !== null && at >= atMin) {
      nowIndex = i;
      break;
    }
  }

  const timed = blocks.filter((b) => b.time !== null);
  return {
    entries,
    nowIndex,
    doneCount: timed.filter((b) => b.completed).length,
    total: timed.length,
  };
}

/** Past entries are rendered muted; future ones lighter. */
export function positionOf(s: Stream, i: number): 'past' | 'future' {
  return i < s.nowIndex ? 'past' : 'future';
}

/**
 * The divider above an entry, or null when it continues the previous one.
 *
 * ⚠️ Returns a STRUCTURE, not a display string. The first version returned
 * "09:00" or the literal "未定时", and the view then compared against that
 * Chinese literal to decide whether to translate it — a display concern living
 * in a data module, and a comparison that would silently stop matching the day
 * somebody edited the wording. The i18n gate caught it; the fix is that this
 * layer never names anything a reader sees.
 */
export type Divider = { kind: 'hour'; text: string } | { kind: 'untimed' };

export function hourLabel(s: Stream, i: number): Divider | null {
  const at = s.entries[i]!.at;
  if (at === null) {
    return i > 0 && s.entries[i - 1]!.at === null ? null : { kind: 'untimed' };
  }
  const hour = Math.floor(at / 60);
  const prev = s.entries[i - 1];
  if (prev && prev.at !== null && Math.floor(prev.at / 60) === hour) return null;
  return { kind: 'hour', text: String(hour).padStart(2, '0') + ':00' };
}
