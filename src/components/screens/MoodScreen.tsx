"use client";

import { useState, useEffect, useRef } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { Plus, X, Check } from "lucide-react";
import Link from "next/link";
import { useSession } from "@/lib/use-session";

interface MoodOption {
  id: string;
  label: string;
  emoji: string;
  custom?: boolean;
}

const DEFAULT_MOODS: MoodOption[] = [
  { id: "stressed", label: "压力山大", emoji: "😰" },
  { id: "anxious", label: "焦虑", emoji: "😟" },
  { id: "tired", label: "疲惫", emoji: "😪" },
  { id: "okay", label: "还行", emoji: "😐" },
  { id: "great", label: "很好", emoji: "😊" },
  { id: "sad", label: "难过", emoji: "😢" },
  { id: "angry", label: "生气", emoji: "😤" },
  { id: "calm", label: "平静", emoji: "😌" },
  { id: "excited", label: "兴奋", emoji: "🤩" },
  { id: "lonely", label: "孤独", emoji: "😔" },
  { id: "grateful", label: "感恩", emoji: "🙏" },
  { id: "confused", label: "困惑", emoji: "😵‍💫" },
];

const STORAGE_KEY = "daycore-custom-moods";

function loadCustomMoods(): MoodOption[] {
  if (typeof window === "undefined") return [];
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY) || "[]");
  } catch {
    return [];
  }
}

function saveCustomMoods(moods: MoodOption[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(moods));
}

export function MoodScreen() {
  const { sessionId } = useSession();

  const [customMoods, setCustomMoods] = useState<MoodOption[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [aiResponse, setAiResponse] = useState<string | null>(null);
  const [exerciseOffered, setExerciseOffered] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [history, setHistory] = useState<{ mood: string; createdAt: string }[]>([]);

  // Custom mood editor state
  const [addingCustom, setAddingCustom] = useState(false);
  const [customEmoji, setCustomEmoji] = useState("");
  const [customLabel, setCustomLabel] = useState("");
  const labelRef = useRef<HTMLInputElement>(null);

  const allMoods = [...DEFAULT_MOODS, ...customMoods];

  useEffect(() => {
    setCustomMoods(loadCustomMoods());
  }, []);

  useEffect(() => {
    if (!sessionId) return;
    fetch(`/api/mood?sessionId=${sessionId}&limit=10`)
      .then(r => r.json())
      .then(data => setHistory(Array.isArray(data) ? data : []))
      .catch(() => {});
  }, [sessionId]);

  useEffect(() => {
    if (addingCustom) setTimeout(() => labelRef.current?.focus(), 50);
  }, [addingCustom]);

  const handleMoodSelect = async (moodId: string) => {
    if (loading) return;
    const mood = allMoods.find(m => m.id === moodId);
    if (!mood) return;

    setSelected(moodId);
    setLoading(true);
    setAiResponse(null);
    setExerciseOffered(null);

    try {
      const res = await fetch("/api/ai/mood", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ mood: `${mood.emoji} ${mood.label}` }),
      });
      const data = await res.json();
      setAiResponse(data.response);

      const text = (data.response || "").toLowerCase();
      if (text.includes("4-7-8") || text.includes("呼吸")) setExerciseOffered("breathing");
      else if (text.includes("伸展") || text.includes("桌边")) setExerciseOffered("stretch");
      else if (text.includes("着陆") || text.includes("五感")) setExerciseOffered("grounding");

      if (sessionId) {
        await fetch("/api/mood", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            sessionId,
            mood: `${mood.emoji} ${mood.label}`,
            aiResponse: data.response,
            exerciseOffered: exerciseOffered || null,
            theme: undefined,
          }),
        });
      }
    } catch {
      setAiResponse("遇到了点问题，稍后再试一下？");
    } finally {
      setLoading(false);
    }
  };

  const handleAddCustom = () => {
    const emoji = customEmoji.trim() || "💭";
    const label = customLabel.trim();
    if (!label) return;

    const newMood: MoodOption = {
      id: `custom-${Date.now()}`,
      label,
      emoji,
      custom: true,
    };
    const updated = [...customMoods, newMood];
    setCustomMoods(updated);
    saveCustomMoods(updated);
    setAddingCustom(false);
    setCustomEmoji("");
    setCustomLabel("");
  };

  const handleDeleteCustom = (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    const updated = customMoods.filter(m => m.id !== id);
    setCustomMoods(updated);
    saveCustomMoods(updated);
    if (selected === id) setSelected(null);
  };

  // Find label for history display
  const getMoodDisplay = (moodStr: string) => {
    // history stores "emoji label" format or plain id
    const found = allMoods.find(m => `${m.emoji} ${m.label}` === moodStr || m.id === moodStr);
    if (found) return { emoji: found.emoji, label: found.label };
    // If stored as "emoji label" directly
    const parts = moodStr.match(/^(\S+)\s+(.+)$/);
    if (parts) return { emoji: parts[1], label: parts[2] };
    return { emoji: "💭", label: moodStr };
  };

  return (
    <div className="px-4 py-4 max-w-2xl mx-auto">
      <h1 className="text-2xl font-bold mb-6" style={{ color: "var(--color-text-primary)" }}>
        心情签到
      </h1>

      {/* Mood grid */}
      <div className="grid grid-cols-3 sm:grid-cols-4 lg:grid-cols-6 gap-2.5 mb-6">
        {allMoods.map((m) => (
          <motion.div key={m.id} className="relative group">
            <motion.button
              whileTap={{ scale: 0.9 }}
              onClick={() => handleMoodSelect(m.id)}
              disabled={loading}
              className="w-full glass-card p-3 flex flex-col items-center gap-1.5 transition-all"
              style={{
                borderColor: selected === m.id ? "var(--color-primary)" : undefined,
                borderWidth: selected === m.id ? "2px" : "1px",
                opacity: loading && selected !== m.id ? 0.5 : 1,
              }}>
              <span className="text-2xl leading-none">{m.emoji}</span>
              <span className="text-[11px] font-medium text-center leading-tight"
                style={{ color: "var(--color-text-primary)" }}>
                {m.label}
              </span>
            </motion.button>

            {/* Delete button for custom moods */}
            {m.custom && (
              <button
                onClick={(e) => handleDeleteCustom(m.id, e)}
                className="absolute -top-1.5 -right-1.5 w-5 h-5 rounded-full flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity z-10"
                style={{ background: "var(--color-states-error)" }}>
                <X size={10} className="text-white" />
              </button>
            )}
          </motion.div>
        ))}

        {/* Add custom mood button */}
        <motion.button
          whileTap={{ scale: 0.9 }}
          onClick={() => setAddingCustom(true)}
          className="glass-card p-3 flex flex-col items-center gap-1.5 border-dashed"
          style={{ borderColor: "var(--color-primary)", opacity: 0.7 }}>
          <Plus size={20} style={{ color: "var(--color-primary)" }} />
          <span className="text-[11px] font-medium" style={{ color: "var(--color-primary)" }}>
            自定义
          </span>
        </motion.button>
      </div>

      {/* Custom mood creation modal */}
      <AnimatePresence>
        {addingCustom && (
          <>
            <motion.div
              initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
              className="fixed inset-0 bg-black/25 z-50"
              onClick={() => setAddingCustom(false)}
            />
            <motion.div
              initial={{ opacity: 0, scale: 0.92, y: 16 }}
              animate={{ opacity: 1, scale: 1, y: 0 }}
              exit={{ opacity: 0, scale: 0.92, y: 16 }}
              transition={{ type: "spring", stiffness: 400, damping: 28 }}
              className="fixed inset-x-4 bottom-[calc(80px+env(safe-area-inset-bottom))] lg:inset-x-auto lg:left-1/2 lg:-translate-x-1/2 lg:w-80 z-50 glass-card p-5"
              style={{ border: "1.5px solid var(--color-primary)" }}>

              <h3 className="text-base font-semibold mb-3" style={{ color: "var(--color-text-primary)" }}>
                添加自定义心情
              </h3>

              {/* Emoji quick-pick grid */}
              <p className="text-xs mb-2" style={{ color: "var(--color-text-muted)" }}>选择 emoji</p>
              <div className="grid grid-cols-8 gap-1 mb-3 p-2 rounded-xl"
                style={{ background: "color-mix(in srgb, var(--color-primary) 6%, transparent)" }}>
                {[
                  "😊","😂","🥰","😎","🤩","😜","😇","🥳",
                  "😔","😢","😭","😤","😠","🤬","😰","😟",
                  "😪","😴","🤒","🥴","😵","😶","😶‍🌫️","🤔",
                  "🙄","😑","😐","😮","😲","😳","🥺","😬",
                  "🤗","🫠","🥱","😮‍💨","😌","😏","🫡","🤧",
                  "💪","🙏","❤️","💔","🔥","⭐","✨","🫶",
                ].map(e => (
                  <button key={e} onClick={() => setCustomEmoji(e)}
                    className="w-8 h-8 text-lg flex items-center justify-center rounded-lg transition-all"
                    style={{
                      background: customEmoji === e ? "var(--color-primary)" : "transparent",
                      transform: customEmoji === e ? "scale(1.15)" : "scale(1)",
                    }}>
                    {e}
                  </button>
                ))}
              </div>

              {/* Label input */}
              <input
                ref={labelRef}
                type="text"
                value={customLabel}
                onChange={e => setCustomLabel(e.target.value)}
                onKeyDown={e => e.key === "Enter" && handleAddCustom()}
                placeholder="描述这个心情… (最多8字)"
                maxLength={8}
                className="w-full h-11 px-3 rounded-xl outline-none text-sm mb-3"
                style={{
                  background: "color-mix(in srgb, var(--color-primary) 8%, transparent)",
                  border: "1px solid var(--color-border-custom)",
                  color: "var(--color-text-primary)",
                }}
              />

              {/* Preview */}
              {(customEmoji || customLabel) && (
                <div className="glass-card p-2.5 flex items-center gap-2 mb-3">
                  <span className="text-xl">{customEmoji || "💭"}</span>
                  <span className="text-sm" style={{ color: "var(--color-text-secondary)" }}>
                    {customLabel || "…"}
                  </span>
                </div>
              )}

              <div className="flex gap-2">
                <button
                  onClick={handleAddCustom}
                  disabled={!customLabel.trim()}
                  className="flex-1 py-2.5 rounded-xl text-sm font-medium text-white disabled:opacity-40 flex items-center justify-center gap-1.5"
                  style={{ background: "var(--color-primary)" }}>
                  <Check size={14} />
                  添加
                </button>
                <button
                  onClick={() => { setAddingCustom(false); setCustomEmoji(""); setCustomLabel(""); }}
                  className="flex-1 py-2.5 rounded-xl text-sm"
                  style={{ color: "var(--color-text-muted)", background: "color-mix(in srgb, var(--color-primary) 8%, transparent)" }}>
                  取消
                </button>
              </div>
            </motion.div>
          </>
        )}
      </AnimatePresence>

      {/* AI response */}
      <AnimatePresence>
        {(loading || aiResponse) && (
          <motion.div
            initial={{ opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0 }}
            className="glass-card p-4 mb-6">
            {loading ? (
              <div className="flex gap-1.5 py-1">
                {[0, 1, 2].map(i => (
                  <motion.div key={i} className="w-2 h-2 rounded-full"
                    style={{ background: "var(--color-primary)" }}
                    animate={{ y: [0, -5, 0] }}
                    transition={{ repeat: Infinity, duration: 0.6, delay: i * 0.18 }} />
                ))}
              </div>
            ) : (
              <>
                <p className="text-sm leading-relaxed" style={{ color: "var(--color-text-secondary)" }}>
                  {aiResponse}
                </p>
                {exerciseOffered && (
                  <Link href={`/mood/exercise/${exerciseOffered}`}
                    className="inline-block mt-3 px-4 py-2 rounded-xl text-sm font-medium text-white"
                    style={{ background: "var(--color-primary)" }}>
                    开始练习
                  </Link>
                )}
              </>
            )}
          </motion.div>
        )}
      </AnimatePresence>

      {/* History */}
      {history.length > 0 && (
        <div>
          <h3 className="text-sm font-semibold mb-3" style={{ color: "var(--color-text-muted)" }}>
            最近签到
          </h3>
          <div className="space-y-2">
            {history.slice(0, 6).map((h, i) => {
              const display = getMoodDisplay(h.mood);
              return (
                <motion.div
                  key={i}
                  initial={{ opacity: 0, y: 6 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: i * 0.05 }}
                  className="glass-card px-4 py-3 flex items-center gap-3">
                  <span className="text-xl">{display.emoji}</span>
                  <div className="flex-1">
                    <p className="text-sm font-medium" style={{ color: "var(--color-text-primary)" }}>
                      {display.label}
                    </p>
                    <p className="text-xs" style={{ color: "var(--color-text-muted)" }}>
                      {new Date(h.createdAt).toLocaleString("zh-CN", {
                        month: "numeric", day: "numeric",
                        hour: "numeric", minute: "2-digit",
                      })}
                    </p>
                  </div>
                </motion.div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
