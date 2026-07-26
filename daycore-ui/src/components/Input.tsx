import { forwardRef, type InputHTMLAttributes, type ReactNode } from "react";
import { cn } from "../lib/cn";

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  /** Icon rendered inside the field, left-aligned. */
  leadingIcon?: ReactNode;
  /** Mark the field invalid (red ring). */
  invalid?: boolean;
}

/**
 * A single-line text field with the soft tinted fill and focus ring of the
 * Daycore language. Pass any native `<input>` prop; add `leadingIcon` for a
 * search/label affordance.
 */
export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ leadingIcon, invalid, className, style, ...rest }, ref) => {
    if (leadingIcon) {
      return (
        <div className="relative w-full">
          <span
            className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2"
            style={{ color: "var(--color-text-muted)" }}
          >
            {leadingIcon}
          </span>
          <input
            ref={ref}
            className={cn("dc-field dc-input pl-10", className)}
            style={{ ...(invalid ? { borderColor: "var(--color-states-error)" } : null), ...style }}
            {...rest}
          />
        </div>
      );
    }
    return (
      <input
        ref={ref}
        className={cn("dc-field dc-input", className)}
        style={{ ...(invalid ? { borderColor: "var(--color-states-error)" } : null), ...style }}
        {...rest}
      />
    );
  },
);

Input.displayName = "Input";
