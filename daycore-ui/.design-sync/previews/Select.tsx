import { Select } from "@daycore/ui";

const THEMES = [
  { value: "sky", label: "☁️ 天空蓝" },
  { value: "sunset", label: "🌅 暖橙日落" },
  { value: "night", label: "🌙 深夜紫" },
  { value: "nature", label: "🌿 自然绿" },
];

export function Default() {
  return (
    <div className="w-64">
      <Select options={THEMES} defaultValue="sky" />
    </div>
  );
}

export function Placeholder() {
  return (
    <div className="w-64">
      <Select options={THEMES} placeholder="选择一个主题…" />
    </div>
  );
}

export function Disabled() {
  return (
    <div className="w-64">
      <Select options={THEMES} defaultValue="night" disabled />
    </div>
  );
}
