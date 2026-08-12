import { useCallback, useEffect, useRef, useState } from 'react';
import * as api from '@daycore/core';
import type { Boot } from '@daycore/core';
import { nowMin, piecesFor, type Piece } from './canvas';

// 长卷's state. Same discipline as the other two — no optimistic updates —
// because PATCH /api/plan can be REFUSED with 409 by the plan gate.
//
// ⚠️ Worst here of the three. 汀 shows one thing; 纸屿 shows a ledger you scroll.
// 长卷 shows the whole day AT ONCE, positioned — a stale block does not just say
// the wrong thing, it occupies the wrong coordinates, and the user's next drag
// is aimed at a picture that is no longer true.

export interface UndoOffer {
  opId: string;
  label: string;
}

const UNDO_MS = 4000;

export function useStore(boot: Boot) {
  const t = boot.catalog.t;
  const [plan, setPlan] = useState<api.DayPlan | null>(null);
  const [proposals, setProposals] = useState<api.Proposal[]>([]);
  const [undo, setUndo] = useState<UndoOffer | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [tick, setTick] = useState(() => nowMin());
  const date = api.todayIso();

  useEffect(() => {
    const h = setInterval(() => setTick(nowMin()), 30_000);
    return () => clearInterval(h);
  }, []);

  const refresh = useCallback(async () => {
    try {
      const [pl, ps] = await Promise.all([api.planForDate(date), api.proposals()]);
      setPlan(pl);
      setProposals(ps.proposals ?? []);
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

  const act = useCallback(
    async (run: () => Promise<unknown>, label: string) => {
      setBusy(true);
      setError('');
      try {
        await run();
        // The op id comes from GET /api/ops after the write — the write
        // endpoints return the new state, not the operation. An undo bar that
        // cannot name what it would undo is decoration.
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

  const pieces: Piece[] = piecesFor(plan, proposals);
  const timed = (plan?.blocks ?? []).filter((b) => !b.hidden && b.time !== null);
  return {
    pieces,
    doneCount: timed.filter((b) => b.completed).length,
    total: timed.length,
    undo,
    busy,
    error,
    now: tick,
    complete,
    answer,
    takeBack,
    refresh,
  };
}
