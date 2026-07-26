import { motion } from "framer-motion";
import { cn } from "../lib/cn";
import { THEMES, type ThemeId } from "../theme/themes";

export interface ThemeSwitcherProps {
  /** Active theme id. */
  value: ThemeId;
  /** Fired with the chosen theme id. */
  onChange?: (theme: ThemeId) => void;
  /**
   * `"pill"` shows a horizontal segmented control of all themes; `"icon"`
   * shows one round button that cycles to the next theme. Default `"pill"`.
   */
  variant?: "pill" | "icon";
  className?: string;
}

/**
 * Switch between the four Daycore themes. The pill variant is a segmented
 * control with a sliding brand-colored indicator; the icon variant is a single
 * round button that cycles. Pairs naturally with {@link ThemeProvider}.
 */
export function ThemeSwitcher({ value, onChange, variant = "pill", className }: ThemeSwitcherProps) {
  if (variant === "icon") {
    const current = THEMES.find((t) => t.id === value) ?? THEMES[0];
    const next = THEMES[(THEMES.findIndex((t) => t.id === value) + 1) % THEMES.length];
    return (
      <button
        type="button"
        aria-label={`切换主题（当前 ${current.label}）`}
        onClick={() => onChange?.(next.id)}
        className={cn("dc-glass flex h-10 w-10 items-center justify-center rounded-full", className)}
      >
        <span className="text-lg">{current.emoji}</span>
      </button>
    );
  }

  return (
    <div className={cn("dc-glass flex gap-1 p-1", className)} style={{ borderRadius: 16 }}>
      {THEMES.map((t) => {
        const isActive = t.id === value;
        return (
          <button
            key={t.id}
            type="button"
            onClick={() => onChange?.(t.id)}
            className="relative z-10 flex-1 rounded-xl px-2 py-1.5 text-xs font-medium transition-colors"
            style={{ color: isActive ? "#fff" : "var(--color-text-muted)" }}
          >
            <span className="relative z-10">{t.emoji}</span>
            {isActive && (
              <motion.span
                layoutId="dc-theme-pill"
                className="absolute inset-0 rounded-xl"
                style={{ background: "var(--color-primary)" }}
                transition={{ type: "spring", stiffness: 400, damping: 30 }}
              />
            )}
          </button>
        );
      })}
    </div>
  );
}

ThemeSwitcher.displayName = "ThemeSwitcher";
