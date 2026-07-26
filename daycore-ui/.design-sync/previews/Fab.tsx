import { Fab } from "@daycore/ui";
import { Sparkles, MessageCircle } from "lucide-react";

export function Default() {
  return <Fab aria-label="添加安排" />;
}

export function CustomIcons() {
  return (
    <div className="flex items-center gap-4">
      <Fab aria-label="生成" icon={<Sparkles size={20} />} />
      <Fab aria-label="聊天" icon={<MessageCircle size={20} />} />
    </div>
  );
}
