import { useCallback, useEffect, useRef, useState } from 'react';
import * as api from './api';
import { flowAt, nowMin, type Flow } from './flow';
import type { Boot } from './session';

// 汀's state: the day, the pending proposals, and the thing you can still undo.
//
// ⚠️ No optimistic updates on writes. Every action re-reads, and that is the
// point rather than laziness: PATCH /api/plan can be REFUSED with 409 by the
// plan gate (a locked or petrified block), and a proposal can be answered by a
// second tab between the render and the tap. 汀's entire premise is that the
// screen answers "what now" truthfully — a screen showing a completed task the
// server still considers open is the one failure it cannot afford.
//
// The cost is a round trip per action, on a screen that shows one thing at a
// time. That is the cheap direction.

export interface UndoOffer {
  opId: string;
  label: string;
  until: number;
}

export interface Store {
  flow: Flow;
  proposals: api.Proposal[];
  undo: UndoOffer | null;
  busy: boolean;
  error: string;
  date: string;
  complete: (b: api.TimeBlock) => Promise<void>;
  answer: (p: api.Proposal, accept: boolean) => Promise<void>;
  takeBack: () => Promise<void>;
  dismissUndo: () => void;
  refresh: () => Promise<void>;
}

/** How long an undo stays offered. Matches the prototype's 3s bar. */
const UNDO_MS = 3000;

export function useStore(boot: Boot): Store {
  const [plan, setPlan] = useState<api.DayPlan | null>(null);
  const [proposals, setProposals] = useState<api.Proposal[]>([]);
  const [undo, setUndo] = useState<UndoOffer | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [tick, setTick] = useState(() => nowMin());
  const date = api.todayIso();
  // ⚠️ The undo bar's label is user-visible copy and goes through the
  // catalogue like everything else. It was the last hardcoded string in 汀, and
  // it hid here rather than in a component — which is exactly where this kind
  // of thing survives a copy pass.
  const t = boot.catalog.t;

  // A minute hand. 30s so the "还剩 N 分钟" line is never more than half a
  // minute stale — 汀 shows one number and it is the one people check.
  useEffect(() => {
    const t = setInterval(() => setTick(nowMin()), 30_000);
    return () => clearInterval(t);
  }, []);

  const refresh = useCallback(async () => {
    try {
      const [p, ps] = await Promise.all([api.planForDate(date), api.proposals()]);
      setPlan(p);
      // Only pending ones. The list can carry settled rows, and a card that
      // comes back after being answered is the single most alarming thing this
      // screen could do.
      setProposals((ps.proposals ?? []).filter((x) => x.state === 'pending'));
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [date]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  // The undo offer expires on its own.
  const timer = useRef<number | null>(null);
  const offer = useCallback((opId: string, label: string) => {
    setUndo({ opId, label, until: Date.now() + UNDO_MS });
    if (timer.current) clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setUndo(null), UNDO_MS);
  }, []);
  useEffect(() => () => { if (timer.current) clearTimeout(timer.current); }, []);

  /** The op this action produced, so it can be offered back.
   *
   * ⚠️ Read from GET /api/ops AFTER the write, rather than from the write's own
   * response — the write endpoints return the new state, not the operation id.
   * That is one extra request per action and it is what makes undo real; an
   * undo bar that cannot name what it would undo is decoration. */
  const latestOp = useCallback(async (): Promise<string | null> => {
    try {
      const { ops } = await api.ops(1);
      const top = ops?.[0];
      return top && !top.reverted ? top.id : null;
    } catch {
      return null;
    }
  }, []);

  const act = useCallback(
    async (run: () => Promise<void>, label: string) => {
      setBusy(true);
      setError('');
      try {
        await run();
        const opId = await latestOp();
        await refresh();
        if (opId) offer(opId, label);
      } catch (e) {
        if (e instanceof api.ApiError && e.status === 409) {
          // The plan gate refused. Its message is written for a person and is
          // the whole reason this is not optimistic — see docs/specs.
          setError(e.message);
        } else {
          setError(e instanceof Error ? e.message : String(e));
        }
        await refresh();
      } finally {
        setBusy(false);
      }
    },
    [latestOp, offer, refresh],
  );

  const complete = useCallback(
    (b: api.TimeBlock) =>
      act(
        () =>
          api
            .patchPlan(date, { action: 'update', match: { id: b.id }, changes: { completed: true } })
            .then(() => undefined),
        t('undo.completed', { title: b.title }),
      ),
    [act, date, t],
  );

  const answer = useCallback(
    (p: api.Proposal, accept: boolean) =>
      act(
        () => api.respondToProposal(p.id, accept).then(() => undefined),
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

  return {
    flow: flowAt(plan, tick),
    proposals,
    undo,
    busy,
    error,
    date,
    complete,
    answer,
    takeBack,
    dismissUndo: () => setUndo(null),
    refresh,
  };
}
