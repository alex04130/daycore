import { forwardRef, type ButtonHTMLAttributes } from "react";
import { cn } from "../lib/cn";

export interface MoodTileProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** The mood emoji. */
  emoji: string;
  /** Short mood label. */
  label: string;
  /** Highlight as the chosen mood. */
  selected?: boolean;
  /** Dim others while one is being processed. */
  dimmed?: boolean;
}

/**
 * A single tile in the mood-check-in grid: a big emoji over a short label on a
 * glass square. The selected tile gets a brand-colored ring.
 */
export const MoodTile = forwardRef<HTMLButtonElement, MoodTileProps>(
  ({ emoji, label, selected, dimmed, className, style, type = "button", ...rest }, ref) => {
    return (
      <button
        ref={ref}
        type={type}
        className={cn(
          "dc-glass flex w-full flex-col items-center gap-1.5 p-3 transition-transform active:scale-90",
          className,
        )}
        style={{
          borderColor: selected ? "var(--color-primary)" : undefined,
          borderWidth: selected ? 2 : 1,
          opacity: dimmed ? 0.5 : 1,
          ...style,
        }}
        {...rest}
      >
        <span className="text-2xl leading-none">{emoji}</span>
        <span
          className="text-center text-[11px] font-medium leading-tight"
          style={{ color: "var(--color-text-primary)" }}
        >
          {label}
        </span>
      </button>
    );
  },
);

MoodTile.displayName = "MoodTile";
