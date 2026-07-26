import { EmptyState, Button } from "@daycore/ui";
import { Calendar, Plus, Inbox } from "lucide-react";

export function NoSchedule() {
  return (
    <div className="w-96">
      <EmptyState
        icon={<Calendar size={32} />}
        title="还没有安排"
        description="点击右上角「+」描述你的一天，或上传课程表截图"
        action={<Button leadingIcon={<Plus size={16} />}>添加安排</Button>}
      />
    </div>
  );
}

export function Simple() {
  return (
    <div className="w-96">
      <EmptyState icon={<Inbox size={32} />} title="一切都清空了" description="今天没有待办，享受这份轻松。" />
    </div>
  );
}
