import { forwardRef, type HTMLAttributes, type ReactNode } from "react";
import { cn } from "../lib/cn";
import { GlassCard, type GlassPadding } from "./GlassCard";

export interface CardProps extends Omit<HTMLAttributes<HTMLDivElement>, "title"> {
  /** Heading shown in the card header row. */
  title?: ReactNode;
  /** Secondary line under the title. */
  description?: ReactNode;
  /** Leading visual (icon, avatar) in the header. */
  icon?: ReactNode;
  /** Trailing control in the header (button, menu). */
  action?: ReactNode;
  /** Pinned footer region. */
  footer?: ReactNode;
  /** Inner padding. Default `"md"`. */
  padding?: GlassPadding;
  /** Hover lift — use when the whole card is clickable. */
  interactive?: boolean;
}

/**
 * A structured glass surface with optional header (icon · title · description ·
 * action) and footer slots. Drop content into `children`; omit every slot and
 * it degrades to a plain {@link GlassCard}.
 */
export const Card = forwardRef<HTMLDivElement, CardProps>(
  (
    { title, description, icon, action, footer, padding, interactive, className, children, ...rest },
    ref,
  ) => {
    const hasHeader = title || description || icon || action;
    return (
      <GlassCard ref={ref} padding={padding} interactive={interactive} className={cn(className)} {...rest}>
        {hasHeader && (
          <div className="flex items-start gap-3">
            {icon && <div className="shrink-0">{icon}</div>}
            <div className="min-w-0 flex-1">
              {title && (
                <p className="text-sm font-semibold leading-tight" style={{ color: "var(--color-text-primary)" }}>
                  {title}
                </p>
              )}
              {description && (
                <p className="mt-0.5 text-xs leading-relaxed" style={{ color: "var(--color-text-muted)" }}>
                  {description}
                </p>
              )}
            </div>
            {action && <div className="shrink-0">{action}</div>}
          </div>
        )}
        {children && <div className={cn(hasHeader && "mt-3")}>{children}</div>}
        {footer && (
          <div className="mt-4 border-t pt-3" style={{ borderColor: "var(--color-border-custom)" }}>
            {footer}
          </div>
        )}
      </GlassCard>
    );
  },
);

Card.displayName = "Card";
