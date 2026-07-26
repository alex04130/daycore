import { type ReactNode } from "react";
import { motion } from "framer-motion";
import { cn } from "../lib/cn";

export interface TabBarItem {
  /** Stable key / route. */
  key: string;
  /** Visible label. */
  label: string;
  /** Icon element (e.g. a lucide icon). */
  icon: ReactNode;
}

export interface TabBarProps {
  /** The tabs to render. */
  items: TabBarItem[];
  /** Currently active item key. */
  active: string;
  /** Fired with the key of a tapped tab. */
  onSelect?: (key: string) => void;
  className?: string;
}

/**
 * The frosted bottom navigation bar — evenly spaced icon+label tabs with the
 * active one springing up in the brand color. The primary mobile nav surface.
 */
export function TabBar({ items, active, onSelect, className }: TabBarProps) {
  return (
    <nav className={cn("dc-tab-bar", className)}>
      {items.map((item) => {
        const isActive = item.key === active;
        return (
          <button
            key={item.key}
            type="button"
            onClick={() => onSelect?.(item.key)}
            className="dc-tab-item"
            style={{ color: isActive ? "var(--color-primary)" : "var(--color-text-muted)" }}
          >
            <motion.span animate={{ scale: isActive ? 1.1 : 1 }} transition={{ type: "spring", stiffness: 400, damping: 25 }}>
              {item.icon}
            </motion.span>
            <span className="text-[10px] font-medium">{item.label}</span>
          </button>
        );
      })}
    </nav>
  );
}

TabBar.displayName = "TabBar";
