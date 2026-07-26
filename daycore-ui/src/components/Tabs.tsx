import { type ReactNode } from "react";
import { motion } from "framer-motion";
import { cn } from "../lib/cn";

export interface TabItem {
  /** Stable value. */
  value: string;
  /** Visible label. */
  label: ReactNode;
  /** Optional leading icon. */
  icon?: ReactNode;
}

export interface TabsProps {
  /** The tabs to render. */
  items: TabItem[];
  /** Selected tab value (controlled). */
  value: string;
  /** Fired with the newly selected value. */
  onValueChange?: (value: string) => void;
  /** Stretch tabs to fill the width. Default `true`. */
  fullWidth?: boolean;
  className?: string;
}

/**
 * A segmented tab switcher on a glass track with a sliding brand indicator —
 * the in-panel toggle used for "text vs image" inputs and similar two/three-way
 * choices. Controlled via `value` / `onValueChange`.
 */
export function Tabs({ items, value, onValueChange, fullWidth = true, className }: TabsProps) {
  return (
    <div className={cn("dc-glass flex gap-1 p-1", className)} style={{ borderRadius: 14 }}>
      {items.map((t) => {
        const isActive = t.value === value;
        return (
          <button
            key={t.value}
            type="button"
            onClick={() => onValueChange?.(t.value)}
            className={cn(
              "relative flex items-center justify-center gap-1.5 rounded-[10px] px-3 py-2 text-sm font-medium transition-colors",
              fullWidth && "flex-1",
            )}
            style={{ color: isActive ? "#fff" : "var(--color-primary)" }}
          >
            {isActive && (
              <motion.span
                layoutId="dc-tabs-indicator"
                className="absolute inset-0 rounded-[10px]"
                style={{ background: "var(--color-primary)" }}
                transition={{ type: "spring", stiffness: 400, damping: 30 }}
              />
            )}
            <span className="relative z-10 inline-flex items-center gap-1.5">
              {t.icon}
              {t.label}
            </span>
          </button>
        );
      })}
    </div>
  );
}

Tabs.displayName = "Tabs";
