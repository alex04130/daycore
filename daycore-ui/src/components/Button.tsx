import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from "react";
import { cn } from "../lib/cn";

export type ButtonVariant =
  | "primary"
  | "secondary"
  | "outline"
  | "ghost"
  | "destructive";
export type ButtonSize = "sm" | "md" | "lg" | "icon";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** Visual emphasis. Default `"primary"`. */
  variant?: ButtonVariant;
  /** Control height & padding. `"icon"` is a square tap target. Default `"md"`. */
  size?: ButtonSize;
  /** Icon rendered before the label. */
  leadingIcon?: ReactNode;
  /** Icon rendered after the label. */
  trailingIcon?: ReactNode;
  /** Stretch to the full width of the container. */
  fullWidth?: boolean;
}

const VARIANT: Record<ButtonVariant, string> = {
  primary: "dc-btn--primary",
  secondary: "dc-btn--secondary",
  outline: "dc-btn--outline",
  ghost: "dc-btn--ghost",
  destructive: "dc-btn--destructive",
};

const SIZE: Record<ButtonSize, string> = {
  sm: "dc-btn--sm",
  md: "dc-btn--md",
  lg: "dc-btn--lg",
  icon: "dc-btn--icon",
};

/**
 * The primary action element. Token-driven so it adopts the active theme's
 * brand color, with five emphasis levels and a square icon size.
 */
export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  (
    {
      variant = "primary",
      size = "md",
      leadingIcon,
      trailingIcon,
      fullWidth,
      className,
      children,
      type = "button",
      ...rest
    },
    ref,
  ) => {
    return (
      <button
        ref={ref}
        type={type}
        className={cn(
          "dc-btn",
          VARIANT[variant],
          SIZE[size],
          fullWidth && "w-full",
          className,
        )}
        {...rest}
      >
        {leadingIcon}
        {children}
        {trailingIcon}
      </button>
    );
  },
);

Button.displayName = "Button";
