import { Card, Button, Badge } from "@daycore/ui";
import { Sparkles, Calendar } from "lucide-react";

export function WithHeader() {
  return (
    <Card
      className="w-80"
      icon={
        <div
          className="flex h-10 w-10 items-center justify-center rounded-xl"
          style={{ background: "color-mix(in srgb, var(--color-primary) 15%, transparent)", color: "var(--color-primary)" }}
        >
          <Calendar size={18} />
        </div>
      }
      title="今日计划"
      description="6 月 19 日 · 星期四"
      action={<Badge tone="primary">4 项</Badge>}
    >
      <p className="text-sm" style={{ color: "var(--color-text-secondary)" }}>
        上午开会，下午写报告，傍晚留了一段放松时间。
      </p>
    </Card>
  );
}

export function WithFooter() {
  return (
    <Card
      className="w-80"
      icon={<Sparkles size={18} style={{ color: "var(--color-primary)" }} />}
      title="来自 Leo 的小提醒"
      footer={
        <div className="flex justify-end gap-2">
          <Button variant="ghost" size="sm">
            稍后
          </Button>
          <Button size="sm">好的</Button>
        </div>
      }
    >
      <p className="text-sm" style={{ color: "var(--color-text-secondary)" }}>
        你已经连续 3 天完成了晨间拉伸，要不要今天也来一组？
      </p>
    </Card>
  );
}

export function Plain() {
  return (
    <Card className="w-72">
      <p className="text-sm" style={{ color: "var(--color-text-primary)" }}>
        没有头部时，Card 退化为一个干净的玻璃面板。
      </p>
    </Card>
  );
}
