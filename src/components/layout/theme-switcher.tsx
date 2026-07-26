"use client";

import { motion } from "framer-motion";
import { useTheme, THEMES, type Theme } from "@/lib/theme-context";
import { cn } from "@/utils/utils";

export function ThemeSwitcher({ variant }: { variant: "topbar" | "sidebar" }) {
  const { theme, setTheme } = useTheme();

  if (variant === "topbar") {
    // Mobile top-right icon button
    const current = THEMES.find((t) => t.id === theme);
    return (
      <button
        className="w-10 h-10 flex items-center justify-center rounded-full glass-card"
        onClick={() => {
          const next = THEMES[(THEMES.findIndex((t) => t.id === theme) + 1) % THEMES.length];
          setTheme(next.id);
        }}>
        <span className="text-lg">{current?.emoji}</span>
      </button>
    );
  }

  // Desktop sidebar horizontal pill selector
  return (
    <div className="relative flex gap-1 p-1 rounded-2xl glass-card">
      {THEMES.map((t) => (
        <button
          key={t.id}
          onClick={() => setTheme(t.id)}
          className={cn(
            "relative z-10 flex-1 px-2 py-1.5 text-xs font-medium rounded-xl transition-all",
            theme === t.id ? "text-white" : "text-[var(--color-text-muted)] hover:opacity-70"
          )}>
          <span>{t.emoji}</span>
          {theme === t.id && (
            <motion.div
              layoutId="theme-pill"
              className="absolute inset-0 rounded-xl"
              style={{ background: "var(--color-primary)" }}
              transition={{ type: "spring", stiffness: 400, damping: 30 }}
            />
          )}
        </button>
      ))}
    </div>
  );
}
