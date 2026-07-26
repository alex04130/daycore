import { Skeleton, GlassCard } from "@daycore/ui";

export function LoadingCard() {
  return (
    <GlassCard className="w-80">
      <div className="flex items-center gap-3">
        <Skeleton circle width={44} height={44} />
        <div className="flex-1">
          <Skeleton width="60%" height={14} />
          <div className="mt-2">
            <Skeleton width="90%" height={12} />
          </div>
        </div>
      </div>
    </GlassCard>
  );
}

export function Lines() {
  return (
    <div className="flex w-72 flex-col gap-2.5">
      <Skeleton width="100%" height={16} />
      <Skeleton width="85%" height={16} />
      <Skeleton width="70%" height={16} />
    </div>
  );
}

export function Blocks() {
  return (
    <div className="flex w-80 flex-col gap-3">
      <Skeleton height={64} />
      <Skeleton height={64} />
    </div>
  );
}
