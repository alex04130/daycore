import { type ReactNode } from "react";
import { cn } from "../lib/cn";
import { Badge } from "./Badge";

export interface FeatureCardProps {
  /** Icon centered in a tinted circle. */
  icon: ReactNode;
  /** Feature name. */
  label: ReactNode;
  /** One-line description. */
  description?: ReactNode;
  /** Pill text under the description (e.g. "即将上线"). */
  badge?: ReactNode;
  /** Dim + disable, for not-yet-available modules. */
  comingSoon?: boolean;
  onClick?: () => void;
  className?: string;
}

/**
 * A centered feature tile — icon in a halo, label, description, and an optional
 * status badge. Used for the "life modules" grid and feature directories.
 */
export function FeatureCard({ icon, label, description, badge, comingSoon, onClick, className }: FeatureCardProps) {
  return (
    <div
      onClick={comingSoon ? undefined : onClick}
      className={cn(
        "dc-glass flex flex-col items-center gap-3 p-5 text-center",
        comingSoon ? "cursor-not-allowed opacity-60" : onClick && "dc-glass--interactive",
        className,
      )}
    >
      <div
        className="flex h-12 w-12 items-center justify-center rounded-full"
        style={{ background: "color-mix(in srgb, var(--color-primary) 15%, transparent)", color: "var(--color-primary)" }}
      >
        {icon}
      </div>
      <div>
        <p className="mb-1 text-sm font-semibold" style={{ color: "var(--color-text-primary)" }}>
          {label}
        </p>
        {description && (
          <p className="text-xs leading-relaxed" style={{ color: "var(--color-text-muted)" }}>
            {description}
          </p>
        )}
      </div>
      {badge && <Badge tone="primary">{badge}</Badge>}
    </div>
  );
}

FeatureCard.displayName = "FeatureCard";
