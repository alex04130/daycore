import { ProgressRing } from "@daycore/ui";

export function BreathingTimer() {
  return (
    <ProgressRing value={0.62} size={140} strokeWidth={8}>
      <span className="text-sm font-semibold" style={{ color: "var(--color-primary)" }}>
        吸气
      </span>
      <span className="text-3xl font-bold tabular-nums" style={{ color: "var(--color-text-primary)" }}>
        4
      </span>
    </ProgressRing>
  );
}

export function Steps() {
  return (
    <div className="flex items-center gap-6">
      <ProgressRing value={0.25} size={72} strokeWidth={6}>
        <span className="text-sm font-bold" style={{ color: "var(--color-text-primary)" }}>
          25%
        </span>
      </ProgressRing>
      <ProgressRing value={0.5} size={72} strokeWidth={6}>
        <span className="text-sm font-bold" style={{ color: "var(--color-text-primary)" }}>
          50%
        </span>
      </ProgressRing>
      <ProgressRing value={1} size={72} strokeWidth={6}>
        <span className="text-sm font-bold" style={{ color: "var(--color-states-success)" }}>
          ✓
        </span>
      </ProgressRing>
    </div>
  );
}
