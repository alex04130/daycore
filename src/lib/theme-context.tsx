"use client";

import React, { createContext, useContext, useEffect, useState } from "react";

export type Theme = "sky" | "sunset" | "night" | "nature";

export const THEMES: { id: Theme; label: string; emoji: string; bgFrom: string; bgTo: string }[] = [
  { id: "sky", label: "天空蓝", emoji: "☁️", bgFrom: "#e0f2fe", bgTo: "#f0f9ff" },
  { id: "sunset", label: "暖橙日落", emoji: "🌅", bgFrom: "#fff7ed", bgTo: "#fef3c7" },
  { id: "night", label: "深夜紫", emoji: "🌙", bgFrom: "#1e1b4b", bgTo: "#312e81" },
  { id: "nature", label: "自然绿", emoji: "🌿", bgFrom: "#f0fdf4", bgTo: "#ecfdf5" },
];

interface ThemeContextValue {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  themeLabel: string;
  themeSwitchTime: string | null;
}

const ThemeContext = createContext<ThemeContextValue>({
  theme: "sky",
  setTheme: () => {},
  themeLabel: "天空蓝",
  themeSwitchTime: null,
});

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [theme, setThemeState] = useState<Theme>("sky");
  const [themeSwitchTime, setThemeSwitchTime] = useState<string | null>(null);

  useEffect(() => {
    const saved = localStorage.getItem("daycore-theme") as Theme | null;
    if (saved && THEMES.find((t) => t.id === saved)) {
      setThemeState(saved);
    }
  }, []);

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
    localStorage.setItem("daycore-theme", theme);
  }, [theme]);

  const setTheme = (t: Theme) => {
    setThemeState(t);
    setThemeSwitchTime(new Date().toISOString());
    // Update session in DB
    const sessionId = localStorage.getItem("daycore-session-id");
    if (sessionId) {
      fetch("/api/session/theme", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sessionId, theme: t }),
      }).catch(() => {});
    }
  };

  const themeLabel = THEMES.find((t) => t.id === theme)?.label ?? "天空蓝";

  return (
    <ThemeContext.Provider value={{ theme, setTheme, themeLabel, themeSwitchTime }}>
      {children}
    </ThemeContext.Provider>
  );
}

export const useTheme = () => useContext(ThemeContext);
