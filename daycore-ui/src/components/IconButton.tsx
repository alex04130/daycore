import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from "react";
import { cn } from "../lib/cn";

export type IconButtonVariant = "glass" | "primary" | "ghost";
export type IconButtonSize = "sm" | "md" | "lg";

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** The icon to render (e.g. a lucide icon element). */
  icon: ReactNode;
  /** Visual style. Default `"glass"`. */
  variant?: IconButtonVariant;
  /** Diameter. Default `"md"`. */
  size?: IconButtonSize;
  /** Accessible label — required since the button has no text. */
  "aria-label": string;
}

const SIZE: Record<IconButtonSize, number> = { sm: 32, md: 40, lg: 48 };

/**
 * A circular icon-only button — used for top-bar actions, dismiss buttons, and
 * theme toggles. Glass variant floats on any surface; primary draws focus.
 */
export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(
  ({ icon, variant = "glass", size = "md", className, style, type = "button", ...rest }, ref) => {
    const d = SIZE[size];
    const base: React.CSSProperties = { width: d, height: d };
    const byVariant: Record<IconButtonVariant, React.CSSProperties> = {
      glass: {},
      primary: { background: "var(--color-primary)", color: "#fff" },
      ghost: { color: "var(--color-text-secondary)" },
    };
    return (
      <button
        ref={ref}
        type={type}
        className={cn(
          "inline-flex items-center justify-center rounded-full transition-transform active:scale-90",
          variant === "glass" && "dc-glass",
          className,
        )}
        style={{ ...base, ...byVariant[variant], ...style }}
        {...rest}
      >
        {icon}
      </button>
    );
  },
);

IconButton.displayName = "IconButton";
