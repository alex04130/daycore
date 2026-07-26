import { type ReactNode } from "react";
import { cn } from "../lib/cn";

export interface EmptyStateProps {
  /** Icon centered in a glass circle. */
  icon?: ReactNode;
  /** Headline. */
  title: ReactNode;
  /** Supporting copy below the title. */
  description?: ReactNode;
  /** Call-to-action (a Button, usually). */
  action?: ReactNode;
  className?: string;
}

/**
 * The friendly "nothing here yet" placeholder — a haloed icon, a title, a line
 * of guidance, and an optional action. Used for empty schedules, lists, etc.
 */
export function EmptyState({ icon, title, description, action, className }: EmptyStateProps) {
  return (
    <div className={cn("flex flex-col items-center justify-center px-6 py-16 text-center", className)}>
      {icon && (
        <div
          className="dc-glass mb-5 flex h-20 w-20 items-center justify-center rounded-full"
          style={{ color: "var(--color-primary)" }}
        >
          {icon}
        </div>
      )}
      <h2 className="mb-2 text-lg font-semibold" style={{ color: "var(--color-text-primary)" }}>
        {title}
      </h2>
      {description && (
        <p className="mb-6 max-w-xs text-sm leading-relaxed" style={{ color: "var(--color-text-muted)" }}>
          {description}
        </p>
      )}
      {action}
    </div>
  );
}

EmptyState.displayName = "EmptyState";
