import { SectionHeader, Fab, Chip } from "@daycore/ui";
import { Plus } from "lucide-react";

export function WithFab() {
  return (
    <div className="w-96">
      <SectionHeader title="日程" eyebrow="6 月 19 日 · 星期四" action={<Fab aria-label="添加" icon={<Plus size={20} />} />} />
    </div>
  );
}

export function WithSubtitle() {
  return (
    <div className="w-96">
      <SectionHeader
        title="生活"
        subtitle="即将上线的新功能，让 Daycore 成为你完整的生活陪伴"
      />
    </div>
  );
}

export function WithChip() {
  return (
    <div className="w-96">
      <SectionHeader title="心情签到" action={<Chip variant="selected">本周</Chip>} />
    </div>
  );
}
