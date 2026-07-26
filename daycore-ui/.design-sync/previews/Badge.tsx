import { Badge } from "@daycore/ui";
import { Check, Clock, AlertTriangle } from "lucide-react";

export function Tones() {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Badge tone="primary">固定</Badge>
      <Badge tone="success">已完成</Badge>
      <Badge tone="warning">待定</Badge>
      <Badge tone="error">冲突</Badge>
      <Badge tone="neutral">草稿</Badge>
    </div>
  );
}

export function WithIcon() {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Badge tone="success" icon={<Check size={11} />}>
        已完成
      </Badge>
      <Badge tone="primary" icon={<Clock size={11} />}>
        进行中
      </Badge>
      <Badge tone="warning" icon={<AlertTriangle size={11} />}>
        即将开始
      </Badge>
    </div>
  );
}
