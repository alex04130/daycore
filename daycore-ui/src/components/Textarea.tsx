import { forwardRef, type TextareaHTMLAttributes } from "react";
import { cn } from "../lib/cn";

export interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  /** Mark the field invalid (red ring). */
  invalid?: boolean;
}

/**
 * A multi-line text field matching {@link Input}'s tinted fill and focus ring.
 * Used for day descriptions, journal entries, and the companion composer.
 */
export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ invalid, className, style, rows = 4, ...rest }, ref) => {
    return (
      <textarea
        ref={ref}
        rows={rows}
        className={cn("dc-field dc-textarea", className)}
        style={{ ...(invalid ? { borderColor: "var(--color-states-error)" } : null), ...style }}
        {...rest}
      />
    );
  },
);

Textarea.displayName = "Textarea";
