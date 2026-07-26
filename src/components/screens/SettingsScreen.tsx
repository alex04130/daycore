"use client";

import { useState, useEffect } from "react";
import { motion } from "framer-motion";
import { Edit2, Check } from "lucide-react";
import { useSession, broadcastAssistantName } from "@/lib/use-session";

export function SettingsScreen() {
  const { sessionId, assistantName: initialName } = useSession();
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(initialName || "Leo");
  const [saving, setSaving] = useState(false);

  // Sync with prop changes
  useEffect(() => {
    setName(initialName || "Leo");
  }, [initialName]);

  const handleSave = async () => {
    if (!sessionId || !name.trim() || saving) return;
    setSaving(true);

    try {
      await fetch("/api/session/settings", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sessionId, assistantName: name.trim() }),
      });

      // 立即广播，同页面所有用 useSession 的组件实时更新
      broadcastAssistantName(name.trim());

      setEditing(false);
    } catch {
      // silently fail
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="px-4 py-4 max-w-2xl mx-auto">
      <h1 className="text-2xl font-bold mb-6" style={{ color: "var(--color-text-primary)" }}>设置</h1>

      <div className="glass-card p-5 mb-3">
        <div className="flex items-center justify-between mb-2">
          <p className="text-sm font-semibold" style={{ color: "var(--color-text-primary)" }}>AI 助手名称</p>
          {!editing ? (
            <button onClick={() => setEditing(true)} className="flex items-center gap-1 text-sm"
              style={{ color: "var(--color-primary)" }}>
              <Edit2 size={12} />
              修改
            </button>
          ) : (
            <button onClick={handleSave} disabled={saving}
              className="flex items-center gap-1 text-sm disabled:opacity-50"
              style={{ color: "var(--color-states-success)" }}>
              <Check size={14} />
              {saving ? "保存中…" : "保存"}
            </button>
          )}
        </div>
        {!editing ? (
          <p className="text-base font-medium" style={{ color: "var(--color-text-primary)" }}>
            {name}
          </p>
        ) : (
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleSave()}
            className="w-full px-3 py-2 rounded-xl text-sm outline-none"
            style={{
              background: "color-mix(in srgb, var(--color-primary) 8%, transparent)",
              border: "1px solid var(--color-border-custom)",
              color: "var(--color-text-primary)",
            }}
            autoFocus
          />
        )}
        <p className="text-xs mt-2" style={{ color: "var(--color-text-muted)" }}>
          可以叫它 Leo、小艾、Ambi……你喜欢的名字
        </p>
      </div>

      <div className="glass-card p-5">
        <p className="text-xs leading-relaxed" style={{ color: "var(--color-text-muted)" }}>
          Daycore v1.0 · 所有数据存储在你的设备和云端，随时可导出
        </p>
      </div>
    </div>
  );
}
