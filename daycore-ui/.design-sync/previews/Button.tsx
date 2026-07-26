import { Button } from "@daycore/ui";
import { Sparkles, Plus, Trash2 } from "lucide-react";

export function Primary() {
  return <Button leadingIcon={<Sparkles size={16} />}>生成日程</Button>;
}

export function Variants() {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button variant="primary">主要</Button>
      <Button variant="secondary">次要</Button>
      <Button variant="outline">描边</Button>
      <Button variant="ghost">幽灵</Button>
      <Button variant="destructive" leadingIcon={<Trash2 size={15} />}>
        删除
      </Button>
    </div>
  );
}

export function Sizes() {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <Button size="sm">小</Button>
      <Button size="md">中</Button>
      <Button size="lg" leadingIcon={<Plus size={18} />}>
        添加安排
      </Button>
      <Button size="icon" aria-label="add">
        <Plus size={18} />
      </Button>
    </div>
  );
}

export function States() {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button>可用</Button>
      <Button disabled>不可用</Button>
      <Button variant="secondary" disabled>
        加载中…
      </Button>
    </div>
  );
}

export function FullWidth() {
  return (
    <div className="w-72">
      <Button fullWidth size="lg">
        开始今天
      </Button>
    </div>
  );
}
