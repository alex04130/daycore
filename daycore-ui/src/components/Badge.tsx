import { forwardRef, type HTMLAttributes, type ReactNode } from "react";
import { cn } from "../lib/cn";

export type BadgeTone = "primary" | "success" | "warning" | "error" | "neutral";

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  /** Color tone, mapped to design tokens. Default `"primary"`. */
  tone?: BadgeTone;
  /** Leading icon. */
  icon?: ReactNode;
}

const TONE: Record<BadgeTone, string> = {
  primary: "var(--color-primary)",
  success: "var(--color-states-success)",
  warning: "var(--color-states-warning)",
  error: "var(--color-states-error)",
  neutral: "var(--color-text-muted)",
};

/**
 * A small non-interactive status pill — "fixed", "done", "coming soon", counts.
 * Tinted background + matching text drawn from the active theme's tokens.
 */
export const Badge = forwardRef<HTMLSpanElement, BadgeProps>(
  ({ tone = "primary", icon, className, style, children, ...rest }, ref) => {
    const color = TONE[tone];
    return (
      <span
        ref={ref}
        className={cn("dc-badge", className)}
        style={{
          background: `color-mix(in srgb, ${color} 14%, transparent)`,
          color,
          ...style,
        }}
        {...rest}
      >
        {icon}
        {children}
      </span>
    );
  },
);

Badge.displayName = "Badge";
