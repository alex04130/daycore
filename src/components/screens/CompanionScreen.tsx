"use client";

import { useState, useRef, useEffect, useCallback } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { Send, Bot, LogIn } from "lucide-react";
import { useSession } from "@/lib/use-session";
import { useTheme } from "@/lib/theme-context";
import { useEazo } from "@eazo/sdk/react";
import { auth } from "@eazo/sdk";

interface Message {
  id: string;
  role: "user" | "assistant";
  content: string;
}

function applyPlanUpdate(blocks: unknown[], action: {
  action: string;
  match?: Record<string, unknown>;
  changes?: Record<string, unknown>;
  block?: Record<string, unknown>;
}) {
  if (action.action === "update") {
    return blocks.map((b) => {
      const block = b as Record<string, unknown>;
      const matched = Object.entries(action.match || {}).every(([k, v]) => block[k] === v);
      return matched ? { ...block, ...action.changes } : block;
    });
  } else if (action.action === "remove") {
    return blocks.filter((b) => {
      const block = b as Record<string, unknown>;
      return !Object.entries(action.match || {}).every(([k, v]) => block[k] === v);
    });
  } else if (action.action === "add") {
    return [...blocks, { ...action.block, id: `block-${Date.now()}` }]
      .sort((a, b) => {
        const ta = String((a as Record<string, unknown>).time ?? "");
        const tb = String((b as Record<string, unknown>).time ?? "");
        return ta > tb ? 1 : -1;
      });
  }
  return blocks;
}

export function CompanionScreen() {
  const { sessionId, assistantName } = useSession();
  const { themeLabel } = useTheme();
  const user = useEazo((s) => s.auth.user);

  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [streaming, setStreaming] = useState(false);
  const [historyLoaded, setHistoryLoaded] = useState(false);
  const [todayPlan, setTodayPlan] = useState<unknown[]>([]);
  const [moodHistory, setMoodHistory] = useState<unknown[]>([]);
  const bottomRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const saveTimerRef = useRef<NodeJS.Timeout | null>(null);

  const name = assistantName || "Leo";

  useEffect(() => {
    if (!sessionId) return;
    const today = new Date().toLocaleDateString("zh-CN", {
      year: "numeric", month: "2-digit", day: "2-digit",
    }).replace(/\//g, "-");

    Promise.all([
      fetch(`/api/plan?sessionId=${sessionId}&date=${today}`).then(r => r.json()).catch(() => null),
      fetch(`/api/mood?sessionId=${sessionId}&limit=5`).then(r => r.json()).catch(() => []),
      fetch(`/api/companion-history?sessionId=${sessionId}`).then(r => r.json()).catch(() => ({ history: [] })),
    ]).then(([plan, moods, mem]) => {
      if (plan?.blocks) setTodayPlan(plan.blocks);
      if (Array.isArray(moods)) setMoodHistory(moods);
      const saved: { role: string; content: string }[] = mem.history ?? [];
      if (saved.length > 0) {
        setMessages(saved.map((m, i) => ({ id: `h-${i}`, role: m.role as "user" | "assistant", content: m.content })));
      } else {
        const hour = new Date().getHours();
        const greeting = hour < 12 ? "早上好" : hour < 18 ? "下午好" : "晚上好";
        setMessages([{ id: "welcome", role: "assistant", content: `${greeting}！我是 ${name}，有什么可以帮你的？` }]);
      }
      setHistoryLoaded(true);
    });
  }, [sessionId, name]);

  const saveHistory = useCallback((msgs: Message[]) => {
    if (!sessionId || !historyLoaded) return;
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    saveTimerRef.current = setTimeout(() => {
      fetch("/api/companion-history", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          sessionId,
          history: msgs.slice(-60).map(m => ({ role: m.role, content: m.content })),
        }),
      }).catch(() => {});
    }, 2000);
  }, [sessionId, historyLoaded]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, streaming]);

  const sendMessage = async () => {
    if (!input.trim() || streaming || !sessionId) return;

    const userMsg: Message = { id: Date.now().toString(), role: "user", content: input };
    const nextMessages = [...messages, userMsg];
    setMessages(nextMessages);
    setInput("");
    setStreaming(true);

    const assistantId = (Date.now() + 1).toString();
    setMessages(prev => [...prev, { id: assistantId, role: "assistant", content: "" }]);

    try {
      const res = await fetch("/api/ai/companion", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          message: input,
          date: new Date().toLocaleDateString("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit" }).replace(/\//g, "-"),
          weekday: ["星期日","星期一","星期二","星期三","星期四","星期五","星期六"][new Date().getDay()],
          time: new Date().toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false }),
          timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
          theme: themeLabel,
          todayPlan,
          moodHistory,
          memoryContext: [],
          assistantName: name,
          conversationHistory: messages.slice(-20).map(m => ({ role: m.role, content: m.content })),
        }),
      });

      const reader = res.body!.getReader();
      const decoder = new TextDecoder();
      let fullContent = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        for (const line of decoder.decode(value).split("\n")) {
          if (!line.startsWith("data: ")) continue;
          const data = line.slice(6);
          if (data === "[DONE]") break;
          try {
            const { delta } = JSON.parse(data);
            fullContent += delta;
            setMessages(prev => prev.map(m => m.id === assistantId ? { ...m, content: fullContent } : m));
          } catch { /* skip */ }
        }
      }

      // Parse plan_update
      const planMatch = fullContent.match(/<plan_update>([\s\S]*?)<\/plan_update>/);
      if (planMatch) {
        try {
          const action = JSON.parse(planMatch[1].trim());
          setTodayPlan(prev => applyPlanUpdate(prev, action));
          const today = new Date().toLocaleDateString("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit" }).replace(/\//g, "-");
          fetch("/api/plan", {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ sessionId, date: today, action }),
          }).catch(() => {});
          const cleaned = fullContent.replace(/<plan_update>[\s\S]*?<\/plan_update>/g, "").trim();
          setMessages(prev => prev.map(m => m.id === assistantId ? { ...m, content: cleaned } : m));
          fullContent = cleaned;
        } catch { /* skip */ }
      }

      const finalMessages = [...nextMessages, { id: assistantId, role: "assistant" as const, content: fullContent }];
      saveHistory(finalMessages);

    } catch {
      setMessages(prev => prev.map(m =>
        m.id === assistantId ? { ...m, content: "遇到了点问题，稍后再试一下？" } : m
      ));
    } finally {
      setStreaming(false);
    }
  };

  return (
    <div className="flex flex-col h-full max-w-2xl mx-auto">
      {/* Header */}
      <div className="shrink-0 px-4 pt-2 pb-3 lg:pt-6 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-full glass-card flex items-center justify-center"
            style={{ background: "color-mix(in srgb, var(--color-primary) 15%, transparent)" }}>
            <Bot size={20} style={{ color: "var(--color-primary)" }} />
          </div>
          <div>
            <h1 className="text-lg font-semibold" style={{ color: "var(--color-text-primary)" }}>{name}</h1>
            <p className="text-xs" style={{ color: "var(--color-text-muted)" }}>
              {user ? `已登录` : "你的日程与情绪陪伴"}
            </p>
          </div>
        </div>
        {!user && (
          <button
            onClick={() => auth.login().catch(() => undefined)}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-medium"
            style={{
              background: "color-mix(in srgb, var(--color-primary) 12%, transparent)",
              color: "var(--color-primary)",
            }}>
            <LogIn size={13} />
            登录 / 注册
          </button>
        )}
      </div>

      {/* Messages */}
      <div className="flex-1 min-h-0 overflow-y-auto no-scrollbar px-4 space-y-3 pb-4">
        {!historyLoaded && (
          <div className="space-y-3 pt-2">
            {[1,2,3].map(i => <div key={i} className="skeleton h-10" style={{ animationDelay: `${i*100}ms` }} />)}
          </div>
        )}
        <AnimatePresence initial={false}>
          {messages.map((msg) => (
            <motion.div key={msg.id}
              initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.22 }}
              className={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}>
              <div className="max-w-[80%] px-4 py-2.5 rounded-2xl text-sm leading-relaxed"
                style={msg.role === "user"
                  ? { background: "var(--color-primary)", color: "white", borderBottomRightRadius: 6 }
                  : { background: "var(--color-surface)", color: "var(--color-text-primary)", border: "1px solid var(--color-border-custom)", borderBottomLeftRadius: 6 }}>
                {msg.content || (streaming && msg.role === "assistant" && <TypingDots />)}
              </div>
            </motion.div>
          ))}
        </AnimatePresence>
        <div ref={bottomRef} />
      </div>

      {/* Input */}
      <div className="shrink-0 px-4 pt-2 pb-4 lg:pb-6">
        <div className="flex items-end gap-2 glass-card p-2">
          <textarea ref={inputRef} value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); sendMessage(); } }}
            placeholder={`和 ${name} 聊聊…`}
            rows={1}
            className="flex-1 resize-none bg-transparent outline-none text-sm py-1.5 px-2 max-h-28"
            style={{ color: "var(--color-text-primary)" }}
          />
          <motion.button whileTap={{ scale: 0.9 }} onClick={sendMessage}
            disabled={!input.trim() || streaming}
            className="w-9 h-9 rounded-xl flex items-center justify-center text-white shrink-0 disabled:opacity-40 transition-opacity"
            style={{ background: "var(--color-primary)" }}>
            <Send size={16} />
          </motion.button>
        </div>
      </div>
    </div>
  );
}

function TypingDots() {
  return (
    <div className="flex gap-1 py-1">
      {[0, 1, 2].map((i) => (
        <motion.div key={i} className="w-1.5 h-1.5 rounded-full"
          style={{ background: "var(--color-text-muted)" }}
          animate={{ y: [0, -4, 0] }}
          transition={{ repeat: Infinity, duration: 0.6, delay: i * 0.18 }} />
      ))}
    </div>
  );
}
