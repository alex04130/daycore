import { forwardRef, type SelectHTMLAttributes } from "react";
import { ChevronDown } from "lucide-react";
import { cn } from "../lib/cn";

export interface SelectOption {
  value: string;
  label: string;
  disabled?: boolean;
}

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  /** Convenience: render these as `<option>`s. You may also pass `children`. */
  options?: SelectOption[];
  /** Placeholder shown as a disabled first option. */
  placeholder?: string;
}

/**
 * A styled dropdown built on the native `<select>` for reliable behavior on
 * every device, wrapped in the design system's tinted field with a chevron.
 */
export const Select = forwardRef<HTMLSelectElement, SelectProps>(
  ({ options, placeholder, className, children, value, defaultValue, ...rest }, ref) => {
    return (
      <div className="relative w-full">
        <select
          ref={ref}
          value={value}
          defaultValue={defaultValue ?? (placeholder && value === undefined ? "" : undefined)}
          className={cn("dc-field dc-input appearance-none pr-9", className)}
          style={{ color: "var(--color-text-primary)" }}
          {...rest}
        >
          {placeholder && (
            <option value="" disabled hidden>
              {placeholder}
            </option>
          )}
          {options?.map((o) => (
            <option key={o.value} value={o.value} disabled={o.disabled}>
              {o.label}
            </option>
          ))}
          {children}
        </select>
        <ChevronDown
          size={16}
          className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2"
          style={{ color: "var(--color-text-muted)" }}
        />
      </div>
    );
  },
);

Select.displayName = "Select";
