"use client";

import { useState, useRef } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { X, Upload, Type, Image, ChevronLeft, ChevronRight, Plus, Trash2 } from "lucide-react";
import type { TimeBlock } from "@/lib/db/schema/daycore";

// Block groups keyed by date (YYYY-MM-DD)
export type BlocksByDate = Record<string, TimeBlock[]>;

interface Props {
  open: boolean;
  onClose: () => void;
  onGenerating: () => void;
  // grouped by date; mode tells parent how to merge
  onDone: (groups: BlocksByDate, note: string | null, mode: "append" | "replace") => void;
  onError: (msg: string) => void;
  sessionId: string | null;
  theme: string;
  themeSwitchTime: string | null;
  existingBlocks: TimeBlock[]; // for today, to check if append/replace prompt needed
}

function fmt(d: Date) {
  return d.toLocaleDateString("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit" }).replace(/\//g, "-");
}
function wday(d: Date) {
  return ["星期日","星期一","星期二","星期三","星期四","星期五","星期六"][d.getDay()];
}
function addDays(d: Date, n: number) {
  const r = new Date(d); r.setDate(r.getDate() + n); return r;
}

export function DayInputModal({
  open, onClose, onGenerating, onDone, onError,
  existingBlocks,
}: Props) {
  const [tab, setTab] = useState<"text" | "image">("text");
  const [text, setText] = useState("");
  const [imageFile, setImageFile] = useState<File | null>(null);
  const [imagePreview, setImagePreview] = useState<string | null>(null);
  const [targetDate, setTargetDate] = useState<Date>(new Date());
  const [step, setStep] = useState<"input" | "mode">("input");
  const [pendingGroups, setPendingGroups] = useState<BlocksByDate | null>(null);
  const [pendingNote, setPendingNote] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const isToday = fmt(targetDate) === fmt(new Date());
  // Append/replace prompt needed only when today has existing blocks AND the AI produced blocks for today
  const hasExistingToday = existingBlocks.length > 0 && isToday;

  const getContext = () => {
    const now = new Date();
    const todayStr = fmt(now);
    const todayWday = wday(now);
    const currentTime = now.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false });
    const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;

    // 给 AI 完整的相对日期对照表，让它不用自己推算
    const relativeDates: Record<string, string> = {};
    const relativeNames = [
      ["明天", 1], ["后天", 2], ["大后天", 3],
      ["昨天", -1], ["前天", -2],
    ] as [string, number][];
    for (const [name, offset] of relativeNames) {
      relativeDates[name] = fmt(addDays(now, offset as number));
    }
    // 本周每天
    const weekdays = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"];
    const todayDow = now.getDay(); // 0=Sunday
    for (let i = 0; i < 7; i++) {
      const diff = i - todayDow;
      relativeDates[weekdays[i]] = fmt(addDays(now, diff));
    }
    const relativeDateLines = Object.entries(relativeDates)
      .map(([k, v]) => `${k} = ${v}`)
      .join("，");

    return {
      // 今天的真实日期 — 永远是锚点
      today: todayStr,
      todayWeekday: todayWday,
      date: todayStr,      // 向后兼容提示词里的 {date} 占位符
      weekday: todayWday,
      time: currentTime,
      timezone: tz,
      // 用户选择要规划的目标日期（可能和 today 不同）
      targetDate: fmt(targetDate),
      targetWeekday: wday(targetDate),
      // 相对日期对照表
      relativeDateMap: relativeDateLines,
    };
  };

  const handleSubmit = async () => {
    if (tab === "text" && !text.trim()) return;
    if (tab === "image" && !imageFile) return;

    onGenerating();

    try {
      const ctx = getContext();
      let res: Response;

      if (tab === "text") {
        res = await fetch("/api/ai/plan-text", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ description: text, ...ctx }),
        });
      } else {
        const base64 = await fileToBase64(imageFile!);
        res = await fetch("/api/ai/plan-image", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ imageBase64: base64, mimeType: imageFile!.type, ...ctx }),
        });
      }

      const data = await res.json();

      if (data.error === "crisis_detected" || data.error === "no_schedule_info" || data.error === "unreadable_image") {
        onError(data.message);
        return;
      }
      if (data.error || !data.blocks) {
        onError(data.message || "日程生成出了点问题，请稍后再试");
        return;
      }

      // Group blocks by their own date field; fall back to targetDate if missing
      const fallbackDate = fmt(targetDate);
      const groups: BlocksByDate = {};
      for (const block of data.blocks as (TimeBlock & { date?: string })[]) {
        const d = block.date || fallbackDate;
        if (!groups[d]) groups[d] = [];
        // Remove the date field from the block itself (it's the group key)
        const { date: _date, ...rest } = block as TimeBlock & { date?: string };
        groups[d].push({ ...rest, id: rest.id || `block-${Date.now()}-${Math.random()}` });
      }

      setText("");
      setImageFile(null);
      setImagePreview(null);

      const todayHasNewBlocks = !!groups[fmt(new Date())];
      if (hasExistingToday && todayHasNewBlocks) {
        setPendingGroups(groups);
        setPendingNote(data.note || null);
        setStep("mode");
      } else {
        onDone(groups, data.note || null, "replace");
      }
    } catch {
      onError("网络好像出了点问题，稍后再试一下？");
    }
  };

  const handleModeSelect = (mode: "append" | "replace") => {
    if (!pendingGroups) return;
    onDone(pendingGroups, pendingNote, mode);
    setPendingGroups(null);
    setPendingNote(null);
    setStep("input");
  };

  const handleClose = () => {
    setStep("input");
    setPendingGroups(null);
    setPendingNote(null);
    setText("");
    setImageFile(null);
    setImagePreview(null);
    onClose();
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setImageFile(file);
    const reader = new FileReader();
    reader.onload = (ev) => setImagePreview(ev.target?.result as string);
    reader.readAsDataURL(file);
  };

  return (
    <AnimatePresence>
      {open && (
        <>
          <motion.div
            initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            className="fixed inset-0 bg-black/25 z-50"
            onClick={handleClose}
          />
          <motion.div
            initial={{ y: "100%" }} animate={{ y: 0 }} exit={{ y: "100%" }}
            transition={{ type: "spring", stiffness: 300, damping: 30 }}
            className="fixed inset-x-0 bottom-0 z-50 rounded-t-[24px] overflow-hidden"
            style={{
              background: "var(--color-surface)",
              backdropFilter: "blur(20px)",
              borderTop: "1px solid var(--color-border-custom)",
              paddingBottom: "env(safe-area-inset-bottom)",
            }}>

            <div className="flex justify-center pt-3 pb-1">
              <div className="w-10 h-1.5 rounded-full opacity-40" style={{ background: "var(--color-text-muted)" }} />
            </div>

            <div className="px-5 pb-6">
              {step === "input" ? (
                <>
                  <div className="flex items-center justify-between mb-4">
                    <h3 className="text-lg font-semibold" style={{ color: "var(--color-text-primary)" }}>
                      安排你的日程
                    </h3>
                    <button onClick={handleClose} className="w-8 h-8 flex items-center justify-center rounded-full"
                      style={{ background: "color-mix(in srgb, var(--color-text-muted) 12%, transparent)" }}>
                      <X size={16} style={{ color: "var(--color-text-muted)" }} />
                    </button>
                  </div>

                  {/* Date selector */}
                  <div className="flex items-center justify-between mb-4 glass-card px-4 py-2.5">
                    <button onClick={() => setTargetDate(d => addDays(d, -1))}
                      className="w-8 h-8 flex items-center justify-center rounded-lg"
                      style={{ color: "var(--color-primary)" }}>
                      <ChevronLeft size={18} />
                    </button>
                    <div className="text-center">
                      <p className="text-sm font-semibold" style={{ color: "var(--color-text-primary)" }}>
                        {isToday ? "今天" : fmt(targetDate)}
                      </p>
                      <p className="text-xs" style={{ color: "var(--color-text-muted)" }}>
                        {wday(targetDate)}
                      </p>
                    </div>
                    <button onClick={() => setTargetDate(d => addDays(d, 1))}
                      className="w-8 h-8 flex items-center justify-center rounded-lg"
                      style={{ color: "var(--color-primary)" }}>
                      <ChevronRight size={18} />
                    </button>
                  </div>

                  {/* Input tabs */}
                  <div className="flex gap-2 mb-4">
                    {([["text", "文字描述", Type], ["image", "上传截图", Image]] as const).map(([id, label, Icon]) => (
                      <button key={id} onClick={() => setTab(id)}
                        className="flex-1 flex items-center justify-center gap-2 py-2 rounded-xl text-sm font-medium"
                        style={{
                          background: tab === id ? "var(--color-primary)" : "color-mix(in srgb, var(--color-primary) 10%, transparent)",
                          color: tab === id ? "white" : "var(--color-primary)",
                        }}>
                        <Icon size={14} />
                        {label}
                      </button>
                    ))}
                  </div>

                  {tab === "text" && (
                    <textarea
                      value={text}
                      onChange={e => setText(e.target.value)}
                      placeholder={isToday
                        ? "例如：后天下午买菜，明天早上做作业…"
                        : "描述这天的安排，例如：上午开会，下午写报告…"}
                      className="w-full h-28 resize-none rounded-xl p-3 text-sm outline-none"
                      style={{
                        background: "color-mix(in srgb, var(--color-primary) 6%, transparent)",
                        border: "1px solid var(--color-border-custom)",
                        color: "var(--color-text-primary)",
                      }}
                      autoFocus
                    />
                  )}

                  {tab === "image" && (
                    <div>
                      <input ref={fileRef} type="file" accept="image/*" className="hidden" onChange={handleFileChange} />
                      {imagePreview ? (
                        <div className="relative rounded-xl overflow-hidden h-36">
                          {/* eslint-disable-next-line @next/next/no-img-element */}
                          <img src={imagePreview} alt="预览" className="w-full h-full object-cover" />
                          <button onClick={() => { setImageFile(null); setImagePreview(null); }}
                            className="absolute top-2 right-2 w-7 h-7 bg-black/50 rounded-full flex items-center justify-center">
                            <X size={14} className="text-white" />
                          </button>
                        </div>
                      ) : (
                        <button onClick={() => fileRef.current?.click()}
                          className="w-full h-28 rounded-xl flex flex-col items-center justify-center gap-2 border-2 border-dashed"
                          style={{ borderColor: "var(--color-border-custom)", color: "var(--color-text-muted)" }}>
                          <Upload size={22} />
                          <span className="text-sm">点击上传课程表、日历或待办截图</span>
                        </button>
                      )}
                    </div>
                  )}

                  <button
                    onClick={handleSubmit}
                    disabled={tab === "text" ? !text.trim() : !imageFile}
                    className="w-full mt-4 py-3 rounded-[var(--radius-button)] font-medium text-white disabled:opacity-40"
                    style={{ background: "var(--color-primary)" }}>
                    生成日程
                  </button>
                </>
              ) : (
                /* Append vs replace */
                <div>
                  <div className="flex items-center gap-2 mb-5">
                    <button onClick={() => setStep("input")}
                      className="w-8 h-8 flex items-center justify-center rounded-full"
                      style={{ background: "color-mix(in srgb, var(--color-primary) 10%, transparent)" }}>
                      <ChevronLeft size={16} style={{ color: "var(--color-primary)" }} />
                    </button>
                    <h3 className="text-base font-semibold" style={{ color: "var(--color-text-primary)" }}>
                      今天已有 {existingBlocks.length} 个安排，如何处理？
                    </h3>
                  </div>
                  <div className="space-y-3">
                    <button onClick={() => handleModeSelect("append")}
                      className="w-full glass-card px-4 py-4 flex items-center gap-4 text-left"
                      style={{ borderColor: "var(--color-primary)", borderWidth: "1.5px" }}>
                      <div className="w-10 h-10 rounded-xl flex items-center justify-center shrink-0"
                        style={{ background: "color-mix(in srgb, var(--color-primary) 15%, transparent)" }}>
                        <Plus size={20} style={{ color: "var(--color-primary)" }} />
                      </div>
                      <div>
                        <p className="text-sm font-semibold" style={{ color: "var(--color-text-primary)" }}>追加到现有安排</p>
                        <p className="text-xs mt-0.5" style={{ color: "var(--color-text-muted)" }}>新内容加入日程，原有安排保留</p>
                      </div>
                    </button>
                    <button onClick={() => handleModeSelect("replace")}
                      className="w-full glass-card px-4 py-4 flex items-center gap-4 text-left">
                      <div className="w-10 h-10 rounded-xl flex items-center justify-center shrink-0"
                        style={{ background: "color-mix(in srgb, var(--color-states-warning) 15%, transparent)" }}>
                        <Trash2 size={20} style={{ color: "var(--color-states-warning)" }} />
                      </div>
                      <div>
                        <p className="text-sm font-semibold" style={{ color: "var(--color-text-primary)" }}>替换今天的安排</p>
                        <p className="text-xs mt-0.5" style={{ color: "var(--color-text-muted)" }}>清空今天日程，换成新版本</p>
                      </div>
                    </button>
                  </div>
                </div>
              )}
            </div>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  );
}

async function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve((reader.result as string).split(",")[1]);
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
}
