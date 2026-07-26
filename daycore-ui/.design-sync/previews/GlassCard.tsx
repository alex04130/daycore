import { GlassCard } from "@daycore/ui";

export function Default() {
  return (
    <GlassCard className="w-72">
      <p className="text-sm font-semibold" style={{ color: "var(--color-text-primary)" }}>
        今天安排得有点紧
      </p>
      <p className="mt-1 text-sm" style={{ color: "var(--color-text-secondary)" }}>
        我在下午给你留了个短暂的喘息。
      </p>
    </GlassCard>
  );
}

export function Interactive() {
  return (
    <GlassCard interactive className="w-72">
      <p className="text-sm font-medium" style={{ color: "var(--color-text-primary)" }}>
        可点击卡片 · hover 会上浮
      </p>
    </GlassCard>
  );
}

export function Paddings() {
  return (
    <div className="flex flex-col gap-3">
      <GlassCard padding="sm" className="w-64 text-xs" style={{ color: "var(--color-text-muted)" }}>
        padding sm
      </GlassCard>
      <GlassCard padding="md" className="w-64 text-xs" style={{ color: "var(--color-text-muted)" }}>
        padding md
      </GlassCard>
      <GlassCard padding="lg" className="w-64 text-xs" style={{ color: "var(--color-text-muted)" }}>
        padding lg
      </GlassCard>
    </div>
  );
}
