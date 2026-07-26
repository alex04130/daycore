import { FeatureCard } from "@daycore/ui";
import { Heart, BookOpen, Utensils, Plane } from "lucide-react";

export function Grid() {
  return (
    <div className="grid w-96 grid-cols-2 gap-3">
      <FeatureCard icon={<Heart size={22} />} label="健康" description="身体数据与运动记录" badge="即将上线" comingSoon />
      <FeatureCard icon={<BookOpen size={22} />} label="学业" description="课程与作业管理" badge="即将上线" comingSoon />
      <FeatureCard icon={<Utensils size={22} />} label="饮食" description="餐饮记录与建议" badge="即将上线" comingSoon />
      <FeatureCard icon={<Plane size={22} />} label="出行" description="行程与交通规划" badge="即将上线" comingSoon />
    </div>
  );
}

export function Active() {
  return (
    <div className="w-48">
      <FeatureCard icon={<Heart size={22} />} label="健康" description="今天已记录 3 项" onClick={() => {}} />
    </div>
  );
}
