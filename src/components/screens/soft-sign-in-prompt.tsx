"use client";

import { motion } from "framer-motion";
import { X } from "lucide-react";

interface Props {
  onDismiss: () => void;
}

export function SoftSignInPrompt({ onDismiss }: Props) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 20 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: 20 }}
      className="glass-card p-4 mb-4 relative"
      style={{ borderColor: "var(--color-primary)", borderWidth: "1.5px" }}>
      <button
        onClick={onDismiss}
        className="absolute top-3 right-3 w-6 h-6 flex items-center justify-center rounded-full"
        style={{ background: "var(--color-surface)" }}>
        <X size={12} style={{ color: "var(--color-text-muted)" }} />
      </button>
      <p className="text-sm font-medium mb-1 pr-6" style={{ color: "var(--color-text-primary)" }}>
        我一直在保存你的每一个小时刻
      </p>
      <p className="text-xs mb-3" style={{ color: "var(--color-text-secondary)" }}>
        如果你想让我永远不遗忘，可以登录保存你的记忆。
      </p>
      <div className="flex gap-2">
        <button
          className="flex-1 py-2 rounded-xl text-sm font-medium text-white"
          style={{ background: "var(--color-primary)" }}>
          登录
        </button>
        <button
          onClick={onDismiss}
          className="flex-1 py-2 rounded-xl text-sm"
          style={{ color: "var(--color-text-muted)", background: "color-mix(in srgb, var(--color-primary) 8%, transparent)" }}>
          以后再说
        </button>
      </div>
    </motion.div>
  );
}
