import { forwardRef, type HTMLAttributes } from "react";
import { cn } from "../lib/cn";

export interface SkeletonProps extends HTMLAttributes<HTMLDivElement> {
  /** Convenience width (CSS value or number→px). */
  width?: number | string;
  /** Convenience height (CSS value or number→px). */
  height?: number | string;
  /** Use a fully round shape (avatars, dots). */
  circle?: boolean;
}

const dim = (v?: number | string) => (typeof v === "number" ? `${v}px` : v);

/**
 * A shimmering placeholder block for loading states. Compose several to sketch
 * the shape of incoming content (a card, a list row, an avatar + two lines).
 */
export const Skeleton = forwardRef<HTMLDivElement, SkeletonProps>(
  ({ width, height = 16, circle, className, style, ...rest }, ref) => {
    return (
      <div
        ref={ref}
        className={cn("dc-skeleton", className)}
        style={{
          width: dim(width),
          height: dim(height),
          borderRadius: circle ? "9999px" : undefined,
          ...style,
        }}
        {...rest}
      />
    );
  },
);

Skeleton.displayName = "Skeleton";
