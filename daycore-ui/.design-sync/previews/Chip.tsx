import { Chip } from "@daycore/ui";
import { Plus } from "lucide-react";

export function Variants() {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Chip>默认</Chip>
      <Chip variant="selected">已选中</Chip>
      <Chip variant="muted">不可选</Chip>
    </div>
  );
}

export function Filters() {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Chip variant="selected">全部</Chip>
      <Chip>任务</Chip>
      <Chip>约定</Chip>
      <Chip>休息</Chip>
      <Chip icon={<Plus size={13} />}>自定义</Chip>
    </div>
  );
}

export function AsTags() {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Chip asTag variant="muted">
        固定时间
      </Chip>
      <Chip asTag>90 分钟</Chip>
      <Chip asTag variant="selected">
        今天
      </Chip>
    </div>
  );
}
