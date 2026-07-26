"use client";

import { motion } from "framer-motion";
import { Heart, BookOpen, Utensils, Plane } from "lucide-react";

const MODULES = [
  { id: "health", label: "健康", icon: Heart, desc: "身体数据与运动记录" },
  { id: "academics", label: "学业", icon: BookOpen, desc: "课程与作业管理" },
  { id: "food", label: "饮食", icon: Utensils, desc: "餐饮记录与建议" },
  { id: "travel", label: "出行", icon: Plane, desc: "行程与交通规划" },
];

export function LifeScreen() {
  return (
    <div className="px-4 py-4 max-w-2xl mx-auto">
      <h1 className="text-2xl font-bold mb-2" style={{ color: "var(--color-text-primary)" }}>生活</h1>
      <p className="text-sm mb-6" style={{ color: "var(--color-text-muted)" }}>
        即将上线的新功能，让 Daycore 成为你完整的生活陪伴
      </p>

      <div className="grid grid-cols-2 gap-3">
        {MODULES.map((m, i) => {
          const Icon = m.icon;
          return (
            <motion.div
              key={m.id}
              initial={{ opacity: 0, y: 16 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: i * 0.1 }}
              className="glass-card p-5 flex flex-col items-center text-center gap-3 opacity-60 cursor-not-allowed">
              <div className="w-12 h-12 rounded-full flex items-center justify-center"
                style={{ background: "color-mix(in srgb, var(--color-primary) 15%, transparent)" }}>
                <Icon size={22} style={{ color: "var(--color-primary)" }} />
              </div>
              <div>
                <p className="text-sm font-semibold mb-1" style={{ color: "var(--color-text-primary)" }}>{m.label}</p>
                <p className="text-xs leading-relaxed" style={{ color: "var(--color-text-muted)" }}>{m.desc}</p>
              </div>
              <div className="px-3 py-1 rounded-full text-[10px] font-medium"
                style={{ background: "color-mix(in srgb, var(--color-primary) 10%, transparent)", color: "var(--color-primary)" }}>
                即将上线
              </div>
            </motion.div>
          );
        })}
      </div>
    </div>
  );
}
