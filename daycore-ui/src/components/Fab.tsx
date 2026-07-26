import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from "react";
import { Plus } from "lucide-react";
import { cn } from "../lib/cn";

export interface FabProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** Icon inside the button. Defaults to a plus. */
  icon?: ReactNode;
  /** Accessible label — required since the button has no visible text. */
  "aria-label": string;
}

/**
 * The floating action button — the primary "add" affordance, a filled circle
 * with the theme's brand color and a soft drop shadow. Place it top-right of a
 * header or bottom-right of a screen.
 */
export const Fab = forwardRef<HTMLButtonElement, FabProps>(
  ({ icon, className, style, type = "button", ...rest }, ref) => {
    return (
      <button
        ref={ref}
        type={type}
        className={cn(
          "flex h-12 w-12 items-center justify-center rounded-full text-white shadow-lg transition-transform active:scale-90",
          className,
        )}
        style={{
          background: "var(--color-primary)",
          boxShadow: "0 8px 24px color-mix(in srgb, var(--color-primary) 40%, transparent)",
          ...style,
        }}
        {...rest}
      >
        {icon ?? <Plus size={22} />}
      </button>
    );
  },
);

Fab.displayName = "Fab";
