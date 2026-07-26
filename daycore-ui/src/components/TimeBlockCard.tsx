import { Clock, Coffee, Sunset, Utensils, CheckCircle2 } from "lucide-react";
import { type ComponentType } from "react";
import { cn } from "../lib/cn";

export type TimeBlockType = "task" | "appointment" | "break" | "relax" | "meal";

export interface TimeBlockCardProps {
  /** Time label, e.g. `"14:30"`. Pass `null`/omit for an unscheduled block. */
  time?: string | null;
  /** What the block is. */
  title: string;
  /** Category — drives the icon and accent dot. Default `"task"`. */
  type?: TimeBlockType;
  /** Duration in minutes, shown as a small caption. */
  durationMin?: number;
  /** `"fixed"` shows a "固定" badge; `"floating"` follows the local clock. */
  timeMode?: "floating" | "fixed";
  /** Render in the done state (dimmed, struck through). */
  completed?: boolean;
  /** Render as a celebratory achievement (green ring + check). */
  isAchievement?: boolean;
  /** Show a past-time block in muted time color. */
  past?: boolean;
  /** Called when the complete button is pressed. Omit to hide the button. */
  onComplete?: () => void;
  className?: string;
}

const TYPE: Record<TimeBlockType, { label: string; Icon: ComponentType<{ size?: number; style?: React.CSSProperties }>; dot: string }> = {
  task: { label: "任务", Icon: Clock, dot: "var(--color-primary)" },
  appointment: { label: "约定", Icon: Clock, dot: "#f97316" },
  break: { label: "休息", Icon: Coffee, dot: "#22c55e" },
  relax: { label: "放松", Icon: Sunset, dot: "#a78bfa" },
  meal: { label: "饮食", Icon: Utensils, dot: "#f59e0b" },
};

/**
 * A single entry in the day timeline: a time column with an accent dot, the
 * title with a category icon, duration/mode metadata, and a complete toggle.
 * Supports the `completed`, `fixed`-time, past, and achievement states.
 */
export function TimeBlockCard({
  time,
  title,
  type = "task",
  durationMin,
  timeMode,
  completed,
  isAchievement,
  past,
  onComplete,
  className,
}: TimeBlockCardProps) {
  const config = TYPE[type];
  const Icon = config.Icon;
  const dot = isAchievement ? "var(--color-states-success)" : config.dot;

  return (
    <div
      className={cn(
        "dc-glass flex items-center gap-3 px-4 py-3.5",
        completed && "opacity-60",
        className,
      )}
      style={{
        borderColor: isAchievement ? "var(--color-states-success)" : undefined,
        borderWidth: isAchievement ? 2 : undefined,
      }}
    >
      {/* Time column */}
      <div className="flex w-12 shrink-0 flex-col items-center gap-1">
        <span
          className="text-xs font-semibold tabular-nums"
          style={{ color: past ? "var(--color-text-muted)" : "var(--color-text-primary)" }}
        >
          {time ?? "待定"}
        </span>
        <span className="h-2 w-2 rounded-full" style={{ background: dot }} />
      </div>

      {/* Content */}
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          {isAchievement ? (
            <CheckCircle2 size={14} style={{ color: "var(--color-states-success)", flexShrink: 0 }} />
          ) : (
            <Icon size={14} style={{ color: config.dot, flexShrink: 0 } as React.CSSProperties} />
          )}
          <p
            className={cn("truncate text-sm font-medium", completed && "line-through")}
            style={{ color: "var(--color-text-primary)" }}
          >
            {title}
          </p>
        </div>
        <div className="mt-0.5 flex flex-wrap items-center gap-2">
          {durationMin != null && (
            <span className="text-xs" style={{ color: "var(--color-text-muted)" }}>
              {durationMin} 分钟
            </span>
          )}
          {!time && (
            <span
              className="rounded-full px-1.5 py-0.5 text-xs"
              style={{ background: "color-mix(in srgb, var(--color-text-muted) 12%, transparent)", color: "var(--color-text-muted)" }}
            >
              待定时间
            </span>
          )}
          {timeMode === "fixed" && time && (
            <span
              className="rounded-full px-1.5 py-0.5 text-xs"
              style={{ background: "color-mix(in srgb, var(--color-primary) 10%, transparent)", color: "var(--color-primary)" }}
            >
              固定
            </span>
          )}
          {isAchievement && (
            <span
              className="rounded-full px-1.5 py-0.5 text-xs font-medium"
              style={{ background: "color-mix(in srgb, var(--color-states-success) 12%, transparent)", color: "var(--color-states-success)" }}
            >
              已完成
            </span>
          )}
        </div>
      </div>

      {/* Complete toggle */}
      {!isAchievement && onComplete && !completed && (
        <button
          onClick={onComplete}
          aria-label="完成"
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full transition-opacity hover:opacity-70"
          style={{ background: "color-mix(in srgb, var(--color-primary) 10%, transparent)" }}
        >
          <CheckCircle2 size={16} style={{ color: "var(--color-primary)" }} />
        </button>
      )}
      {!isAchievement && completed && <CheckCircle2 size={18} style={{ color: "var(--color-states-success)" }} />}
    </div>
  );
}

TimeBlockCard.displayName = "TimeBlockCard";
