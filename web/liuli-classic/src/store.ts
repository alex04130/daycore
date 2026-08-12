import { useCallback, useEffect, useRef, useState } from 'react';
import * as api from '@daycore/core';
import type { Boot } from '@daycore/core';
import { foldRange, groupDay, nowMin, todayIso, weekOf, type DayCell, type Grouped } from './days';

// 初版's state.
//
// Same discipline as the other three — NO optimistic updates — because
// PATCH /api/plan can be refused with 409 by the plan gate. What is different
// here is how much a stale write costs: this is the only one of the four that
// derives the SAME fact into three places at once (the week strip's dots, the
// progress line, the list itself), so a local guess that the server then
// rejects leaves three widgets disagreeing with each other on one screen.
//
// ⚠️ And the day is a variable. `refresh` reloads two things, not one — the day
// you are looking at, and the week it sits in — because moving the cursor
// changes both and reloading only the first leaves the strip pointing at a day
// that is no longer selected.

export interface UndoOffer {
  opId: string;
  label: string;
}

const UNDO_MS = 4000;

export function useStore(boot: Boot) {
  const t = boot.catalog.t;
  const [date, setDate] = useState(() => todayIso());
  const [plan, setPlan] = useState<api.DayPlan | null>(null);
  const [week, setWeek] = useState<DayCell[]>([]);
  const [proposals, setProposals] = useState<api.Proposal[]>([]);
  const [undo, setUndo] = useState<UndoOffer | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [tick, setTick] = useState(() => nowMin());
  const today = todayIso();

  useEffect(() => {
    const h = setInterval(() => setTick(nowMin()), 30_000);
    return () => clearInterval(h);
  }, []);

  const refresh = useCallback(async () => {
    const days = weekOf(date);
    const from = days[0]!;
    const to = days[6]!;
    try {
      const [pl, ps, range] = await Promise.all([
        api.planForDate(date),
        api.proposals(),
        api.planRange(from, to),
      ]);
      setPlan(pl);
      setProposals(ps.proposals ?? []);
      setWeek(foldRange(range ?? [], from, to));
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [date]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const timer = useRef<number | null>(null);
  const offer = useCallback((opId: string, label: string) => {
    setUndo({ opId, label });
    if (timer.current) clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setUndo(null), UNDO_MS);
  }, []);
  useEffect(() => () => { if (timer.current) clearTimeout(timer.current); }, []);

  /**
   * Run a write, then reload, then offer to take it back.
   *
   * ⚠️ The op id comes from GET /api/ops AFTER the write — the write endpoints
   * answer with the new state, not with the operation. An undo bar that cannot
   * name what it would undo is decoration.
   */
  const act = useCallback(
    async (run: () => Promise<unknown>, label: string) => {
      setBusy(true);
      setError('');
      try {
        await run();
        let opId: string | null = null;
        try {
          const { ops } = await api.ops(1);
          const top = ops?.[0];
          opId = top && !top.reverted ? top.id : null;
        } catch {
          opId = null;
        }
        await refresh();
        if (opId) offer(opId, label);
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
        // ⚠️ Reload even on failure. A 409 from the plan gate means the server's
        // day differs from the one on screen, and that is exactly when showing
        // the stale one is worst.
        await refresh();
      } finally {
        setBusy(false);
      }
    },
    [offer, refresh],
  );

  const complete = useCallback(
    (b: api.TimeBlock) =>
      act(
        () => api.patchPlan(date, { action: 'update', match: { id: b.id }, changes: { completed: true } }),
        t('undo.completed', { title: b.title }),
      ),
    [act, date, t],
  );

  const remove = useCallback(
    (b: api.TimeBlock) =>
      act(
        () => api.patchPlan(date, { action: 'remove', match: { id: b.id } }),
        t('undo.removed', { title: b.title }),
      ),
    [act, date, t],
  );

  const add = useCallback(
    (title: string, time: string | null) =>
      act(
        () => api.patchPlan(date, { action: 'add', block: { title, time, type: 'task' } }),
        t('undo.added', { title }),
      ),
    [act, date, t],
  );

  const answer = useCallback(
    (p: api.Proposal, accept: boolean) =>
      act(
        () => api.respondToProposal(p.id, accept),
        t(accept ? 'undo.accepted' : 'undo.rejected', { title: p.title }),
      ),
    [act, t],
  );

  const takeBack = useCallback(async () => {
    if (!undo) return;
    const id = undo.opId;
    setUndo(null);
    setBusy(true);
    try {
      await api.revertOp(id);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }, [undo, refresh]);

  const grouped: Grouped = groupDay(plan?.blocks ?? [], { date, today, nowMinutes: tick });

  return {
    date,
    today,
    setDate,
    plan,
    grouped,
    week,
    proposals: proposals.filter((p) => p.state === 'pending'),
    undo,
    busy,
    error,
    setError,
    complete,
    remove,
    add,
    answer,
    takeBack,
    refresh,
  };
}

export type Store = ReturnType<typeof useStore>;
