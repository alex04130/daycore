import { BottomSheet, Tabs, Textarea, Button } from "@daycore/ui";
import { Type, Image } from "lucide-react";

// BottomSheet is a viewport-fixed overlay. A wrapper with `transform` becomes
// the containing block for `position: fixed` descendants, so the sheet docks to
// the bottom of this framed phone-sized box instead of escaping the card.
function Phone({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="dc-app-bg relative overflow-hidden"
      style={{ width: 360, height: 540, transform: "translateZ(0)", borderRadius: 28 }}
    >
      {children}
    </div>
  );
}

export function DayInput() {
  return (
    <Phone>
      <BottomSheet open title="安排你的日程">
        <Tabs
          value="text"
          className="mb-4"
          items={[
            { value: "text", label: "文字描述", icon: <Type size={14} /> },
            { value: "image", label: "上传截图", icon: <Image size={14} /> },
          ]}
        />
        <Textarea placeholder="例如：后天下午买菜，明天早上做作业…" rows={3} />
        <div className="mt-4">
          <Button fullWidth size="lg">
            生成日程
          </Button>
        </div>
      </BottomSheet>
    </Phone>
  );
}

export function AccountMenu() {
  return (
    <Phone>
      <BottomSheet open title="我的账号">
        <div className="flex items-center gap-3">
          <div
            className="flex h-12 w-12 items-center justify-center rounded-full font-semibold text-white"
            style={{ background: "var(--color-primary)" }}
          >
            林
          </div>
          <div>
            <p className="text-sm font-semibold" style={{ color: "var(--color-text-primary)" }}>
              林晓
            </p>
            <p className="text-xs" style={{ color: "var(--color-text-muted)" }}>
              leo@daycore.app
            </p>
          </div>
        </div>
        <div className="mt-4">
          <Button variant="ghost" fullWidth>
            退出登录
          </Button>
        </div>
      </BottomSheet>
    </Phone>
  );
}
