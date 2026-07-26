import { forwardRef, useState, type HTMLAttributes } from "react";
import { cn } from "../lib/cn";

export interface AvatarProps extends Omit<HTMLAttributes<HTMLDivElement>, "children"> {
  /** Image URL. Falls back to initials on load error or when absent. */
  src?: string | null;
  /** Display name — drives the initial and the alt text. */
  name?: string | null;
  /** Fallback when no name (e.g. an email). */
  fallback?: string | null;
  /** Diameter in px. Default `40`. */
  size?: number;
}

/**
 * A user avatar that shows the profile image when available and gracefully
 * falls back to a brand-colored initial. Used in nav, account menus, and chat.
 */
export const Avatar = forwardRef<HTMLDivElement, AvatarProps>(
  ({ src, name, fallback, size = 40, className, style, ...rest }, ref) => {
    const [errored, setErrored] = useState(false);
    const display = name || fallback || "?";
    const initial = display.trim()[0]?.toUpperCase() ?? "?";

    if (src && !errored) {
      return (
        <div
          ref={ref}
          className={cn("shrink-0 overflow-hidden rounded-full", className)}
          style={{ width: size, height: size, ...style }}
          {...rest}
        >
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={src}
            alt={display}
            referrerPolicy="no-referrer"
            onError={() => setErrored(true)}
            className="h-full w-full object-cover"
          />
        </div>
      );
    }

    return (
      <div
        ref={ref}
        className={cn("flex shrink-0 items-center justify-center rounded-full font-semibold text-white", className)}
        style={{ width: size, height: size, fontSize: size * 0.4, background: "var(--color-primary)", ...style }}
        {...rest}
      >
        {initial}
      </div>
    );
  },
);

Avatar.displayName = "Avatar";
