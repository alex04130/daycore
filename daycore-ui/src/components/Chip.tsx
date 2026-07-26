import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from "react";
import { cn } from "../lib/cn";

export type ChipVariant = "default" | "selected" | "muted";

export interface ChipProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** Visual state. `"selected"` uses the theme's selected-chip pair. Default `"default"`. */
  variant?: ChipVariant;
  /** Leading icon or emoji. */
  icon?: ReactNode;
  /** Render as static text (no press affordance / not a control). */
  asTag?: boolean;
}

const VARIANT: Record<ChipVariant, string> = {
  default: "",
  selected: "dc-chip--selected",
  muted: "dc-chip--muted",
};

/**
 * A rounded pill for filters, tags, and single-select choices (mood filters,
 * "fixed time" markers, quick suggestions). Toggle `variant="selected"` to show
 * the active state in any theme.
 */
export const Chip = forwardRef<HTMLButtonElement, ChipProps>(
  ({ variant = "default", icon, asTag, className, children, type = "button", ...rest }, ref) => {
    return (
      <button
        ref={ref}
        type={type}
        className={cn("dc-chip", VARIANT[variant], asTag && "dc-chip--static", className)}
        {...rest}
      >
        {icon}
        {children}
      </button>
    );
  },
);

Chip.displayName = "Chip";
