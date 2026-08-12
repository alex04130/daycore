import { useCallback, useEffect, useRef, useState } from 'react';
import * as api from '@daycore/core';
import type { Boot } from '@daycore/core';
import { buildStream, nowMin, type Stream } from './stream';

// 纸屿's state. Same discipline as 汀 — no optimistic updates — for the same
// reason: PATCH /api/plan can be REFUSED with 409 by the plan gate, and a
// proposal can be answered by another tab between render and tap.
//
// ⚠️ It matters MORE here than in 汀. 汀 shows one thing, so a stale entry is
// one wrong screen; 纸屿 shows the whole day at once, so a stale entry sits in
// a ledger that otherwise looks authoritative, and "the ledger is what happened"
// is the entire premise of this paradigm.

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

  const stream: Stream = buildStream(plan, proposals, tick);
  return { stream, undo, busy, error, now: tick, complete, answer, takeBack, refresh };
}
