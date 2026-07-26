import { Textarea } from "@daycore/ui";

export function Default() {
  return (
    <div className="w-80">
      <Textarea placeholder="例如：后天下午买菜，明天早上做作业…" />
    </div>
  );
}

export function Filled() {
  return (
    <div className="w-80">
      <Textarea defaultValue={"上午 10 点开周会\n下午写报告\n晚上给妈妈打电话"} rows={4} />
    </div>
  );
}

export function Invalid() {
  return (
    <div className="w-80">
      <Textarea invalid placeholder="说点什么吧" rows={3} />
    </div>
  );
}
