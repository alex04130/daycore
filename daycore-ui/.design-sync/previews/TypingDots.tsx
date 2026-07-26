import { TypingDots, GlassCard } from "@daycore/ui";

export function InBubble() {
  return (
    <div
      className="inline-flex max-w-[80%] px-4 py-3"
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border-custom)",
        borderRadius: 16,
        borderBottomLeftRadius: 6,
      }}
    >
      <TypingDots />
    </div>
  );
}

export function OnCard() {
  return (
    <GlassCard className="flex w-48 items-center justify-center py-6">
      <TypingDots size={8} color="var(--color-primary)" />
    </GlassCard>
  );
}

export function Sizes() {
  return (
    <div className="flex items-center gap-6">
      <TypingDots size={5} />
      <TypingDots size={8} />
      <TypingDots size={11} color="var(--color-primary)" />
    </div>
  );
}
