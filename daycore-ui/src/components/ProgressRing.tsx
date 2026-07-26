import { type ReactNode } from "react";
import { cn } from "../lib/cn";

export interface ProgressRingProps {
  /** Progress 0–1. */
  value: number;
  /** Outer diameter in px. Default `120`. */
  size?: number;
  /** Stroke width in px. Default `8`. */
  strokeWidth?: number;
  /** Ring color (CSS value). Defaults to the theme's brand token. */
  color?: string;
  /** Track color (CSS value). Defaults to a faint tint of the ring color. */
  trackColor?: string;
  /** Centered content (a count, a label). */
  children?: ReactNode;
  className?: string;
}

/**
 * A circular progress indicator — the breathing-exercise timer and any
 * round metric. Pass `value` 0–1; drop a number or label into the center.
 */
export function ProgressRing({
  value,
  size = 120,
  strokeWidth = 8,
  color = "var(--color-primary)",
  trackColor = "color-mix(in srgb, var(--color-primary) 14%, transparent)",
  children,
  className,
}: ProgressRingProps) {
  const clamped = Math.max(0, Math.min(1, value));
  const r = (size - strokeWidth) / 2;
  const c = 2 * Math.PI * r;
  return (
    <div className={cn("relative inline-flex items-center justify-center", className)} style={{ width: size, height: size }}>
      <svg width={size} height={size} className="-rotate-90">
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke={trackColor} strokeWidth={strokeWidth} />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke={color}
          strokeWidth={strokeWidth}
          strokeLinecap="round"
          strokeDasharray={c}
          strokeDashoffset={c * (1 - clamped)}
          style={{ transition: "stroke-dashoffset 0.5s var(--ease-out-soft, ease)" }}
        />
      </svg>
      {children != null && <div className="absolute inset-0 flex flex-col items-center justify-center">{children}</div>}
    </div>
  );
}

ProgressRing.displayName = "ProgressRing";
