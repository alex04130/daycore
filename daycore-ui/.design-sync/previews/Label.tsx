import { Label, Input } from "@daycore/ui";

export function WithInput() {
  return (
    <div className="flex w-72 flex-col gap-1.5">
      <Label htmlFor="name">AI 助手名称</Label>
      <Input id="name" defaultValue="Leo" />
    </div>
  );
}

export function Required() {
  return (
    <div className="flex w-72 flex-col gap-1.5">
      <Label htmlFor="mood" required>
        今天的心情
      </Label>
      <Input id="mood" placeholder="必填" />
    </div>
  );
}
