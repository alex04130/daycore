"use client";

import { motion } from "framer-motion";
import { Clock, Coffee, Sunset, Utensils, CheckCircle2 } from "lucide-react";
import type { TimeBlock } from "@/lib/db/schema/daycore";
import { cn } from "@/utils/utils";

const TYPE_CONFIG: Record<TimeBlock["type"], { label: string; icon: typeof Clock; dotColor: string }> = {
  task:        { label: "任务", icon: Clock,      dotColor: "var(--color-primary)" },
  appointment: { label: "约定", icon: Clock,      dotColor: "#f97316" },
  break:       { label: "休息", icon: Coffee,     dotColor: "#22c55e" },
  relax:       { label: "放松", icon: Sunset,     dotColor: "#a78bfa" },
  meal:        { label: "饮食", icon: Utensils,   dotColor: "#f59e0b" },
};

interface Props {
  block: TimeBlock;
  onUpdate: (block: TimeBlock) => void;
  onExerciseDone?: (name: string) => void;
}

export function TimeBlockCard({ block, onUpdate, onExerciseDone }: Props) {
  const config = TYPE_CONFIG[block.type];
  const Icon = config.icon;
  const isAchievement = block.isAchievement;
  const isPast = block.time ? isTimePast(block.time) : false;

  return (
    <motion.div
      whileHover={{ y: -2 }}
      whileTap={{ scale: 0.97 }}
      className={cn(
        "glass-card px-4 py-3.5 mb-3 flex items-center gap-3 cursor-pointer",
        block.completed && "opacity-60",
        isAchievement && "border-2"
      )}
      style={{
        borderColor: isAchievement ? "var(--color-states-success)" : undefined,
      }}>

      {/* Time column */}
      <div className="flex flex-col items-center gap-1 shrink-0 w-12">
        <span
          className="text-xs font-semibold tabular-nums"
          style={{ color: isPast ? "var(--color-text-muted)" : "var(--color-text-primary)" }}>
          {block.time ?? "待定"}
        </span>
        <div
          className="w-2 h-2 rounded-full"
          style={{ background: isAchievement ? "var(--color-states-success)" : config.dotColor }}
        />
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          {isAchievement
            ? <CheckCircle2 size={14} style={{ color: "var(--color-states-success)", flexShrink: 0 }} />
            : <Icon size={14} style={{ color: config.dotColor, flexShrink: 0 }} />
          }
          <p className={cn(
            "text-sm font-medium truncate",
            block.completed && "line-through"
          )} style={{ color: "var(--color-text-primary)" }}>
            {block.title}
          </p>
        </div>
        <div className="flex items-center gap-2 mt-0.5 flex-wrap">
          {block.duration_min != null && (
            <span className="text-xs" style={{ color: "var(--color-text-muted)" }}>
              {block.duration_min} 分钟
            </span>
          )}
          {!block.time && (
            <span
              className="text-xs px-1.5 py-0.5 rounded-full"
              style={{
                background: "color-mix(in srgb, var(--color-text-muted) 12%, transparent)",
                color: "var(--color-text-muted)",
              }}>
              待定时间
            </span>
          )}
          {block.time_mode === "fixed" && block.time && (
            <span
              className="text-xs px-1.5 py-0.5 rounded-full"
              style={{
                background: "color-mix(in srgb, var(--color-primary) 10%, transparent)",
                color: "var(--color-primary)",
              }}>
              固定
            </span>
          )}
          {isAchievement && (
            <span
              className="text-xs px-1.5 py-0.5 rounded-full font-medium"
              style={{
                background: "color-mix(in srgb, var(--color-states-success) 12%, transparent)",
                color: "var(--color-states-success)",
              }}>
              已完成
            </span>
          )}
        </div>
      </div>

      {/* Complete button */}
      {!isAchievement && !block.completed && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            onUpdate({ ...block, completed: true });
          }}
          className="w-8 h-8 rounded-full flex items-center justify-center transition-all hover:opacity-70 shrink-0"
          style={{ background: "color-mix(in srgb, var(--color-primary) 10%, transparent)" }}>
          <CheckCircle2 size={16} style={{ color: "var(--color-primary)" }} />
        </button>
      )}
      {!isAchievement && block.completed && (
        <CheckCircle2 size={18} style={{ color: "var(--color-states-success)" }} />
      )}
    </motion.div>
  );
}

function isTimePast(time: string): boolean {
  const [h, m] = time.split(":").map(Number);
  const now = new Date();
  return h < now.getHours() || (h === now.getHours() && m < now.getMinutes());
}
