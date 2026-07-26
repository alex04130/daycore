import { forwardRef, type HTMLAttributes } from "react";
import { cn } from "../lib/cn";

export type GlassPadding = "none" | "sm" | "md" | "lg";

export interface GlassCardProps extends HTMLAttributes<HTMLDivElement> {
  /** Inner padding. Default `"md"`. */
  padding?: GlassPadding;
  /** Add hover lift + brighten — use for clickable cards. */
  interactive?: boolean;
}

const PAD: Record<GlassPadding, string> = {
  none: "",
  sm: "p-3",
  md: "p-4",
  lg: "p-6",
};

/**
 * The foundational surface of the Daycore language: a frosted-glass panel with
 * a soft blue shadow and rounded corners. Everything else stacks on top of it.
 */
export const GlassCard = forwardRef<HTMLDivElement, GlassCardProps>(
  ({ padding = "md", interactive, className, children, ...rest }, ref) => {
    return (
      <div
        ref={ref}
        className={cn(
          "dc-glass",
          interactive && "dc-glass--interactive",
          PAD[padding],
          className,
        )}
        {...rest}
      >
        {children}
      </div>
    );
  },
);

GlassCard.displayName = "GlassCard";
