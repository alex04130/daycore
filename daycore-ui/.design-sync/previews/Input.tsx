import { Input } from "@daycore/ui";
import { Search } from "lucide-react";

export function Default() {
  return (
    <div className="w-72">
      <Input placeholder="给你的 AI 助手起个名字…" />
    </div>
  );
}

export function WithIcon() {
  return (
    <div className="w-72">
      <Input leadingIcon={<Search size={16} />} placeholder="搜索安排" />
    </div>
  );
}

export function Filled() {
  return (
    <div className="w-72">
      <Input defaultValue="Leo" />
    </div>
  );
}

export function Invalid() {
  return (
    <div className="w-72">
      <Input invalid defaultValue="" placeholder="不能为空" />
    </div>
  );
}

export function Disabled() {
  return (
    <div className="w-72">
      <Input disabled defaultValue="已锁定" />
    </div>
  );
}
