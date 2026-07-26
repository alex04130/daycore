import { Tabs } from "@daycore/ui";
import { Type, Image } from "lucide-react";

export function TextVsImage() {
  return (
    <div className="w-80">
      <Tabs
        value="text"
        items={[
          { value: "text", label: "文字描述", icon: <Type size={14} /> },
          { value: "image", label: "上传截图", icon: <Image size={14} /> },
        ]}
      />
    </div>
  );
}

export function ThreeWay() {
  return (
    <div className="w-80">
      <Tabs
        value="week"
        items={[
          { value: "day", label: "日" },
          { value: "week", label: "周" },
          { value: "month", label: "月" },
        ]}
      />
    </div>
  );
}
