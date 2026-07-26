import { type ReactNode } from "react";
import { cn } from "../lib/cn";

export interface SectionHeaderProps {
  /** Main heading. */
  title: ReactNode;
  /** Small line above the title (date, category). */
  eyebrow?: ReactNode;
  /** Secondary line below the title. */
  subtitle?: ReactNode;
  /** Trailing control aligned to the title (a Fab, a Button, a Chip). */
  action?: ReactNode;
  className?: string;
}

/**
 * A screen / section heading: an optional eyebrow, a bold title, an optional
 * subtitle, and a trailing action. The standard top-of-screen pattern.
 */
export function SectionHeader({ title, eyebrow, subtitle, action, className }: SectionHeaderProps) {
  return (
    <div className={cn("flex items-start justify-between gap-3", className)}>
      <div className="min-w-0">
        {eyebrow && (
          <p className="mb-0.5 text-xs font-medium" style={{ color: "var(--color-text-muted)" }}>
            {eyebrow}
          </p>
        )}
        <h1 className="text-2xl font-bold leading-tight" style={{ color: "var(--color-text-primary)" }}>
          {title}
        </h1>
        {subtitle && (
          <p className="mt-1 text-sm" style={{ color: "var(--color-text-secondary)" }}>
            {subtitle}
          </p>
        )}
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  );
}

SectionHeader.displayName = "SectionHeader";
