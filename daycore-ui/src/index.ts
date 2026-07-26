// ════════════════════════════════════════════════════════════════════════════
// Daycore Design System — public surface
//
// A warm, tactile, glassmorphic UI language for AI day-planning & emotional
// companionship apps. Import the stylesheet once ("@daycore/ui/styles.css") and
// wrap your tree in <ThemeProvider> to activate the design tokens.
// ════════════════════════════════════════════════════════════════════════════

// ── Utilities ────────────────────────────────────────────────────────────────
export { cn } from "./lib/cn";

// ── Theme ────────────────────────────────────────────────────────────────────
export { ThemeProvider, useTheme } from "./theme/ThemeProvider";
export type { ThemeProviderProps } from "./theme/ThemeProvider";
export { THEMES, DEFAULT_THEME } from "./theme/themes";
export type { ThemeId, ThemeMeta } from "./theme/themes";

// ── Primitives ───────────────────────────────────────────────────────────────
export { Button } from "./components/Button";
export type { ButtonProps, ButtonVariant, ButtonSize } from "./components/Button";

export { GlassCard } from "./components/GlassCard";
export type { GlassCardProps, GlassPadding } from "./components/GlassCard";

export { Card } from "./components/Card";
export type { CardProps } from "./components/Card";

export { Input } from "./components/Input";
export type { InputProps } from "./components/Input";

export { Textarea } from "./components/Textarea";
export type { TextareaProps } from "./components/Textarea";

export { Label } from "./components/Label";
export type { LabelProps } from "./components/Label";

export { Select } from "./components/Select";
export type { SelectProps, SelectOption } from "./components/Select";

export { Chip } from "./components/Chip";
export type { ChipProps, ChipVariant } from "./components/Chip";

export { Badge } from "./components/Badge";
export type { BadgeProps, BadgeTone } from "./components/Badge";

export { Skeleton } from "./components/Skeleton";
export type { SkeletonProps } from "./components/Skeleton";

// ── Controls & actions ───────────────────────────────────────────────────────
export { IconButton } from "./components/IconButton";
export type { IconButtonProps, IconButtonVariant, IconButtonSize } from "./components/IconButton";

export { Fab } from "./components/Fab";
export type { FabProps } from "./components/Fab";

export { Tabs } from "./components/Tabs";
export type { TabsProps, TabItem } from "./components/Tabs";

export { ThemeSwitcher } from "./components/ThemeSwitcher";
export type { ThemeSwitcherProps } from "./components/ThemeSwitcher";

// ── Identity & feedback ──────────────────────────────────────────────────────
export { Avatar } from "./components/Avatar";
export type { AvatarProps } from "./components/Avatar";

export { TypingDots } from "./components/TypingDots";
export type { TypingDotsProps } from "./components/TypingDots";

export { ProgressRing } from "./components/ProgressRing";
export type { ProgressRingProps } from "./components/ProgressRing";

// ── Compositions ─────────────────────────────────────────────────────────────
export { EmptyState } from "./components/EmptyState";
export type { EmptyStateProps } from "./components/EmptyState";

export { SectionHeader } from "./components/SectionHeader";
export type { SectionHeaderProps } from "./components/SectionHeader";

export { ChatBubble } from "./components/ChatBubble";
export type { ChatBubbleProps, ChatRole } from "./components/ChatBubble";

export { MoodTile } from "./components/MoodTile";
export type { MoodTileProps } from "./components/MoodTile";

export { TimeBlockCard } from "./components/TimeBlockCard";
export type { TimeBlockCardProps, TimeBlockType } from "./components/TimeBlockCard";

export { TabBar } from "./components/TabBar";
export type { TabBarProps, TabBarItem } from "./components/TabBar";

export { FeatureCard } from "./components/FeatureCard";
export type { FeatureCardProps } from "./components/FeatureCard";

export { BottomSheet } from "./components/BottomSheet";
export type { BottomSheetProps } from "./components/BottomSheet";
