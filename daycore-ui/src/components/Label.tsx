import { forwardRef, type LabelHTMLAttributes } from "react";
import { cn } from "../lib/cn";

export interface LabelProps extends LabelHTMLAttributes<HTMLLabelElement> {
  /** Append a red required asterisk. */
  required?: boolean;
}

/** A form field label in the design system's text-primary weight. */
export const Label = forwardRef<HTMLLabelElement, LabelProps>(
  ({ required, className, children, ...rest }, ref) => {
    return (
      <label ref={ref} className={cn("dc-label", className)} {...rest}>
        {children}
        {required && (
          <span style={{ color: "var(--color-states-error)" }} aria-hidden>
            {" *"}
          </span>
        )}
      </label>
    );
  },
);

Label.displayName = "Label";
