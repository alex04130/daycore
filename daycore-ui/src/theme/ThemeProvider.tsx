import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { DEFAULT_THEME, THEMES, type ThemeId, type ThemeMeta } from "./themes";

interface ThemeContextValue {
  theme: ThemeId;
  setTheme: (theme: ThemeId) => void;
  meta: ThemeMeta;
  themes: ThemeMeta[];
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

export interface ThemeProviderProps {
  children: ReactNode;
  /** Theme to start on. Default `"sky"`. */
  defaultTheme?: ThemeId;
  /** Controlled theme — when provided, the provider stops owning the value. */
  theme?: ThemeId;
  /** Fires whenever the theme changes (controlled or not). */
  onThemeChange?: (theme: ThemeId) => void;
  /**
   * Where to write the `data-theme` attribute. `"self"` (default) wraps a
   * `<div>` so nesting works; `"root"` writes to `<html>` for a global skin.
   */
  target?: "self" | "root";
  /** Persist the chosen theme to `localStorage` under this key. */
  storageKey?: string;
  className?: string;
}

/**
 * Provides the active Daycore theme to descendants and writes the
 * `data-theme` attribute that drives every design token. Works controlled or
 * uncontrolled, scoped to a wrapper or applied to `<html>`.
 */
export function ThemeProvider({
  children,
  defaultTheme = DEFAULT_THEME,
  theme: controlled,
  onThemeChange,
  target = "self",
  storageKey,
  className,
}: ThemeProviderProps) {
  const [internal, setInternal] = useState<ThemeId>(defaultTheme);
  const theme = controlled ?? internal;

  useEffect(() => {
    if (controlled || !storageKey) return;
    const saved = localStorage.getItem(storageKey) as ThemeId | null;
    if (saved && THEMES.some((t) => t.id === saved)) setInternal(saved);
  }, [controlled, storageKey]);

  const setTheme = useCallback(
    (next: ThemeId) => {
      if (!controlled) setInternal(next);
      if (storageKey) localStorage.setItem(storageKey, next);
      onThemeChange?.(next);
    },
    [controlled, storageKey, onThemeChange],
  );

  useEffect(() => {
    if (target !== "root") return;
    const el = document.documentElement;
    const prev = el.getAttribute("data-theme");
    el.setAttribute("data-theme", theme);
    return () => {
      if (prev) el.setAttribute("data-theme", prev);
    };
  }, [theme, target]);

  const meta = useMemo(
    () => THEMES.find((t) => t.id === theme) ?? THEMES[0],
    [theme],
  );

  const value = useMemo<ThemeContextValue>(
    () => ({ theme, setTheme, meta, themes: THEMES }),
    [theme, setTheme, meta],
  );

  return (
    <ThemeContext.Provider value={value}>
      {target === "self" ? (
        <div data-theme={theme} className={className}>
          {children}
        </div>
      ) : (
        children
      )}
    </ThemeContext.Provider>
  );
}

/** Read the active theme and switch it. Throws outside a `ThemeProvider`. */
export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error("useTheme must be used within a <ThemeProvider>");
  return ctx;
}
