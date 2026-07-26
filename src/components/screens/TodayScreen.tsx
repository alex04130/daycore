"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { Plus, Sparkles, AlertCircle, ChevronUp } from "lucide-react";
import { TimeBlockCard } from "./time-block-card";
import { DayInputModal, type BlocksByDate } from "./day-input-modal";
import { useSession } from "@/lib/use-session";
import { useTheme } from "@/lib/theme-context";
import type { TimeBlock } from "@/lib/db/schema/daycore";

function fmt(d: Date) {
  return d.toLocaleDateString("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit" }).replace(/\//g, "-");
}
function wday(d: Date) {
  return ["星期日","星期一","星期二","星期三","星期四","星期五","星期六"][d.getDay()];
}
function addDays(d: Date, n: number) {
  const r = new Date(d); r.setDate(r.getDate() + n); return r;
}
function parseDate(s: string) {
  const [y, m, d] = s.split("-").map(Number);
  return new Date(y, m - 1, d);
}

type DayGroup = { date: string; blocks: TimeBlock[]; note: string | null };

// How many future days to initially load
const INITIAL_FUTURE_DAYS = 6;
const MORE_DAYS_STEP = 7;

export function TodayScreen() {
  const { sessionId } = useSession();
  const { themeLabel, themeSwitchTime } = useTheme();

  const today = fmt(new Date());
  const [days, setDays] = useState<DayGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [generating, setGenerating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [inputOpen, setInputOpen] = useState(false);
  const [inputDate, setInputDate] = useState<string>(today);

  // Past-days loading
  const [showLoadPast, setShowLoadPast] = useState(false);
  const [loadingPast, setLoadingPast] = useState(false);
  const [pastDaysLoaded, setPastDaysLoaded] = useState(0);

  // Sentinel for "scroll to top → show load-past button"
  const topRef = useRef<HTMLDivElement>(null);
  const todayRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  /* ── Load a range of dates from the DB ── */
  const loadDates = useCallback(async (dates: string[]): Promise<DayGroup[]> => {
    if (!sessionId) return [];
    const results = await Promise.all(
      dates.map(date =>
        fetch(`/api/plan?sessionId=${sessionId}&date=${date}`)
          .then(r => r.json())
          .then(data => ({ date, blocks: (data?.blocks as TimeBlock[]) ?? [], note: data?.note ?? null }))
          .catch(() => ({ date, blocks: [], note: null }))
      )
    );
    // Only keep dates that actually have content
    return results.filter(g => g.blocks.length > 0);
  }, [sessionId]);

  /* ── Initial load: today + next N days ── */
  useEffect(() => {
    if (!sessionId) return;
    setLoading(true);
    const dates = Array.from({ length: INITIAL_FUTURE_DAYS + 1 }, (_, i) => fmt(addDays(new Date(), i)));
    loadDates(dates).then(groups => {
      setDays(groups);
      setLoading(false);
    });
  }, [sessionId, loadDates]);

  /* ── Detect scroll-to-top to reveal "load past" ── */
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const handler = () => setShowLoadPast(el.scrollTop < 40);
    el.addEventListener("scroll", handler, { passive: true });
    return () => el.removeEventListener("scroll", handler);
  }, [loading]);

  /* ── Load past days on demand ── */
  const loadPastDays = async () => {
    if (!sessionId || loadingPast) return;
    setLoadingPast(true);
    const start = pastDaysLoaded + 1;
    const dates = Array.from({ length: MORE_DAYS_STEP }, (_, i) => fmt(addDays(new Date(), -(start + i))));
    const groups = await loadDates(dates);
    setDays(prev => [...groups.reverse(), ...prev]);
    setPastDaysLoaded(p => p + MORE_DAYS_STEP);
    setLoadingPast(false);
    if (groups.length === 0) setShowLoadPast(false);
  };

  /* ── After plan generated, merge into days by date ── */
  const handlePlanGenerated = useCallback(async (
    groups: Record<string, TimeBlock[]>,
    newNote: string | null,
    mode: "append" | "replace",
  ) => {
    setGenerating(false);
    setError(null);

    setDays(prev => {
      let next = [...prev];
      for (const [date, newBlocks] of Object.entries(groups)) {
        const existing = next.find(d => d.date === date);
        let finalBlocks: TimeBlock[];
        if (mode === "append" && existing && date === fmt(new Date())) {
          const merged = [...existing.blocks, ...newBlocks];
          const seen = new Set<string>();
          finalBlocks = merged.filter(b => {
            const key = `${b.time ?? "null"}-${b.title}`;
            if (seen.has(key)) return false;
            seen.add(key); return true;
          }).sort((a, b) => {
            if (!a.time) return 1; if (!b.time) return -1;
            return a.time.localeCompare(b.time);
          });
        } else {
          finalBlocks = newBlocks;
        }
        const finalNote = newNote || existing?.note || null;
        const group: DayGroup = { date, blocks: finalBlocks, note: finalNote };
        next = [...next.filter(d => d.date !== date), group]
          .sort((a, b) => a.date.localeCompare(b.date));
      }
      return next;
    });

    // Persist each date to DB
    for (const [date, newBlocks] of Object.entries(groups)) {
      if (sessionId) {
        await fetch("/api/plan", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ sessionId, date, blocks: newBlocks, note: newNote, sourceType: "text" }),
        }).catch(() => {});
      }
    }
  }, [sessionId]);

  const handleBlockUpdate = useCallback((updatedBlock: TimeBlock, date: string) => {
    setDays(prev => prev.map(d => {
      if (d.date !== date) return d;
      const updated = d.blocks.map(b => b.id === updatedBlock.id ? updatedBlock : b);
      if (sessionId) {
        fetch("/api/plan", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ sessionId, date, blocks: updated, sourceType: "text" }),
        }).catch(() => {});
      }
      return { ...d, blocks: updated };
    }));
  }, [sessionId]);

  const handleExerciseDone = (exerciseName: string) => {
    const achievement: TimeBlock = {
      id: `achievement-${Date.now()}`,
      time: new Date().toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false }),
      title: `完成：${exerciseName}`,
      type: "relax", duration_min: 2,
      time_mode: "floating",
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      completed: true, isAchievement: true,
    };
    setDays(prev => {
      const todayGroup = prev.find(d => d.date === today);
      if (todayGroup) {
        const updated = [...todayGroup.blocks, achievement].sort((a, b) => {
          if (!a.time) return 1; if (!b.time) return -1;
          return a.time.localeCompare(b.time);
        });
        return prev.map(d => d.date === today ? { ...d, blocks: updated } : d);
      }
      return [...prev, { date: today, blocks: [achievement], note: null }]
        .sort((a, b) => a.date.localeCompare(b.date));
    });
  };

  /* ── Scroll to today section ── */
  const scrollToToday = () => {
    todayRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
  };

  return (
    <div className="flex flex-col h-full max-w-2xl mx-auto">

      {/* Sticky header */}
      <div className="shrink-0 px-4 pt-4 pb-3 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold" style={{ color: "var(--color-text-primary)" }}>日程</h1>
          <p className="text-xs" style={{ color: "var(--color-text-muted)" }}>
            {new Date().toLocaleDateString("zh-CN", { month: "long", day: "numeric" })}，{wday(new Date())}
          </p>
        </div>
        <motion.button
          whileTap={{ scale: 0.92 }}
          onClick={() => { setInputDate(today); setInputOpen(true); }}
          className="w-10 h-10 rounded-full flex items-center justify-center text-white shadow-lg"
          style={{ background: "var(--color-primary)" }}>
          <Plus size={20} />
        </motion.button>
      </div>

      {/* Error banner */}
      <AnimatePresence>
        {error && (
          <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            className="mx-4 mb-3 glass-card px-4 py-3 flex gap-3 items-center"
            style={{ borderColor: "var(--color-states-error)" }}>
            <AlertCircle size={16} style={{ color: "var(--color-states-error)" }} />
            <p className="text-sm flex-1" style={{ color: "var(--color-states-error)" }}>{error}</p>
            <button className="text-sm underline" style={{ color: "var(--color-primary)" }}
              onClick={() => { setError(null); setInputOpen(true); }}>重试</button>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Scrollable stream */}
      <div
        ref={scrollRef}
        className="flex-1 min-h-0 overflow-y-auto"
        style={{ scrollbarWidth: "none", msOverflowStyle: "none" }}>
        {/* webkit scrollbar hide */}
        <style>{`div::-webkit-scrollbar{display:none}`}</style>

        {/* Top sentinel */}
        <div ref={topRef} className="h-1" />

        {/* Load past button */}
        <AnimatePresence>
          {showLoadPast && !loading && (
            <motion.div
              initial={{ opacity: 0, y: -8 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0 }}
              className="px-4 pb-4 flex justify-center">
              <button
                onClick={loadPastDays}
                disabled={loadingPast}
                className="flex items-center gap-2 px-4 py-2 rounded-full text-sm font-medium"
                style={{
                  background: "color-mix(in srgb, var(--color-primary) 12%, transparent)",
                  color: "var(--color-primary)",
                }}>
                <ChevronUp size={14} />
                {loadingPast ? "加载中…" : "查看过去的日程"}
              </button>
            </motion.div>
          )}
        </AnimatePresence>

        {/* Loading skeleton */}
        {loading && (
          <div className="px-4 space-y-6">
            {[0,1].map(g => (
              <div key={g}>
                <div className="skeleton h-6 w-24 mb-3 rounded-xl" />
                {[0,1,2].map(i => <div key={i} className="skeleton h-16 mb-3" style={{ animationDelay: `${(g*3+i)*80}ms` }} />)}
              </div>
            ))}
          </div>
        )}

        {/* Generating overlay */}
        {generating && (
          <div className="mx-4 mb-4 glass-card p-8 text-center">
            <motion.div animate={{ scale: [1,1.1,1] }} transition={{ repeat: Infinity, duration: 1.4 }}
              className="text-4xl mb-3">✨</motion.div>
            <p className="text-sm" style={{ color: "var(--color-text-secondary)" }}>正在规划…</p>
          </div>
        )}

        {/* Day groups */}
        {!loading && (
          <div className="px-4 pb-24">
            {days.length === 0 && !generating && (
              <motion.div initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }}
                className="flex flex-col items-center justify-center py-20 text-center">
                <div className="w-20 h-20 rounded-full glass-card flex items-center justify-center mb-5">
                  <CalendarIcon color="var(--color-primary)" />
                </div>
                <h2 className="text-lg font-semibold mb-2" style={{ color: "var(--color-text-primary)" }}>
                  还没有安排
                </h2>
                <p className="text-sm mb-6 max-w-xs" style={{ color: "var(--color-text-muted)" }}>
                  点击右上角「+」描述你的一天，或上传课程表截图
                </p>
              </motion.div>
            )}

            {days.map((day) => {
              const d = parseDate(day.date);
              const isTodaySection = day.date === today;
              return (
                <div
                  key={day.date}
                  ref={isTodaySection ? todayRef : undefined}
                  className="mb-10">

                  {/* Date header */}
                  <div className="flex items-center justify-between mb-3 sticky top-0 py-2 z-10"
                    style={{ background: "var(--color-bg-start)" }}>
                    <div>
                      <h2 className="text-lg font-bold leading-tight" style={{ color: "var(--color-text-primary)" }}>
                        {isTodaySection
                          ? <span>今天 <span className="text-base font-normal" style={{ color: "var(--color-text-muted)" }}>· {d.getMonth()+1}月{d.getDate()}日</span></span>
                          : `${d.getMonth()+1}月${d.getDate()}日`
                        }
                      </h2>
                      <p className="text-xs" style={{ color: "var(--color-text-muted)" }}>{wday(d)}</p>
                    </div>
                    <button
                      onClick={() => { setInputDate(day.date); setInputOpen(true); }}
                      className="px-3 py-1.5 rounded-xl text-xs font-medium flex items-center gap-1"
                      style={{ background: "color-mix(in srgb, var(--color-primary) 12%, transparent)", color: "var(--color-primary)" }}>
                      <Plus size={12} />
                      添加
                    </button>
                  </div>

                  {/* AI note */}
                  {day.note && (
                    <div className="glass-card px-4 py-3 mb-3 flex gap-3 items-start">
                      <Sparkles size={14} style={{ color: "var(--color-primary)", marginTop: 2, flexShrink: 0 }} />
                      <p className="text-sm" style={{ color: "var(--color-text-secondary)" }}>{day.note}</p>
                    </div>
                  )}

                  {/* Time blocks */}
                  <AnimatePresence mode="popLayout">
                    {day.blocks.map((block, i) => (
                      <motion.div key={block.id}
                        initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }}
                        exit={{ opacity: 0, scale: 0.95 }}
                        transition={{ delay: i * 0.04, duration: 0.24, ease: [0.16,1,0.3,1] }}>
                        <TimeBlockCard
                          block={block}
                          onUpdate={(b) => handleBlockUpdate(b, day.date)}
                          onExerciseDone={handleExerciseDone}
                        />
                      </motion.div>
                    ))}
                  </AnimatePresence>
                </div>
              );
            })}

            {/* Load more future */}
            {!loading && days.length > 0 && (
              <div className="flex justify-center pb-4">
                <button
                  onClick={async () => {
                    const last = days[days.length - 1];
                    const lastDate = parseDate(last.date);
                    const dates = Array.from({ length: MORE_DAYS_STEP }, (_, i) =>
                      fmt(addDays(lastDate, i + 1))
                    );
                    const groups = await loadDates(dates);
                    if (groups.length > 0) setDays(prev => [...prev, ...groups]);
                  }}
                  className="text-xs px-4 py-2 rounded-full"
                  style={{ color: "var(--color-text-muted)", background: "color-mix(in srgb, var(--color-primary) 8%, transparent)" }}>
                  继续往后看
                </button>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Day input modal */}
      <DayInputModal
        open={inputOpen}
        onClose={() => setInputOpen(false)}
        onGenerating={() => { setGenerating(true); setInputOpen(false); }}
        onDone={handlePlanGenerated}
        onError={(msg) => { setGenerating(false); setError(msg); }}
        sessionId={sessionId}
        theme={themeLabel}
        themeSwitchTime={themeSwitchTime}
        existingBlocks={days.find(d => d.date === inputDate)?.blocks || []}
      />
    </div>
  );
}

function CalendarIcon({ color }: { color: string }) {
  return (
    <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect width="18" height="18" x="3" y="4" rx="2" ry="2"/>
      <line x1="16" x2="16" y1="2" y2="6"/>
      <line x1="8" x2="8" y1="2" y2="6"/>
      <line x1="3" x2="21" y1="10" y2="10"/>
    </svg>
  );
}
