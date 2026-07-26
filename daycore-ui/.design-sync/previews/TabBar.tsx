import { TabBar } from "@daycore/ui";
import { Calendar, MessageCircle, Heart, Grid } from "lucide-react";

const ITEMS = [
  { key: "today", label: "今日", icon: <Calendar size={22} /> },
  { key: "companion", label: "Leo", icon: <MessageCircle size={22} /> },
  { key: "mood", label: "心情", icon: <Heart size={22} /> },
  { key: "life", label: "生活", icon: <Grid size={22} /> },
];

export function Default() {
  return (
    <div className="w-96 overflow-hidden rounded-2xl">
      <TabBar items={ITEMS} active="today" />
    </div>
  );
}

export function MoodActive() {
  return (
    <div className="w-96 overflow-hidden rounded-2xl">
      <TabBar items={ITEMS} active="mood" />
    </div>
  );
}
