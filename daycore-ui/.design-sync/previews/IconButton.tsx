import { IconButton } from "@daycore/ui";
import { Settings, Bell, X, Plus } from "lucide-react";

export function Variants() {
  return (
    <div className="flex items-center gap-3">
      <IconButton aria-label="设置" variant="glass" icon={<Settings size={18} style={{ color: "var(--color-text-secondary)" }} />} />
      <IconButton aria-label="添加" variant="primary" icon={<Plus size={18} />} />
      <IconButton aria-label="通知" variant="ghost" icon={<Bell size={18} />} />
    </div>
  );
}

export function Sizes() {
  return (
    <div className="flex items-center gap-3">
      <IconButton aria-label="关闭" size="sm" icon={<X size={14} style={{ color: "var(--color-text-muted)" }} />} />
      <IconButton aria-label="关闭" size="md" icon={<X size={16} style={{ color: "var(--color-text-muted)" }} />} />
      <IconButton aria-label="关闭" size="lg" icon={<X size={18} style={{ color: "var(--color-text-muted)" }} />} />
    </div>
  );
}
