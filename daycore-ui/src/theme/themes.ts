/**
 * The four shipped Daycore themes. Each maps to a `[data-theme="<id>"]` block
 * in `styles.css`; setting that attribute on any ancestor re-skins the whole
 * subtree at runtime.
 */
export type ThemeId = "sky" | "sunset" | "night" | "nature";

export interface ThemeMeta {
  id: ThemeId;
  /** Human label (Chinese, matching Daycore's product voice). */
  label: string;
  /** Representative emoji used in compact theme switchers. */
  emoji: string;
  /** Gradient stops, handy for previews and pickers. */
  bgFrom: string;
  bgTo: string;
}

export const THEMES: ThemeMeta[] = [
  { id: "sky", label: "天空蓝", emoji: "☁️", bgFrom: "#e0f2fe", bgTo: "#f0f9ff" },
  { id: "sunset", label: "暖橙日落", emoji: "🌅", bgFrom: "#fff7ed", bgTo: "#fef3c7" },
  { id: "night", label: "深夜紫", emoji: "🌙", bgFrom: "#1e1b4b", bgTo: "#312e81" },
  { id: "nature", label: "自然绿", emoji: "🌿", bgFrom: "#f0fdf4", bgTo: "#ecfdf5" },
];

export const DEFAULT_THEME: ThemeId = "sky";
