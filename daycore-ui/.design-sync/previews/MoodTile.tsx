import { MoodTile } from "@daycore/ui";

export function Grid() {
  return (
    <div className="grid w-72 grid-cols-3 gap-2.5">
      <MoodTile emoji="😰" label="压力山大" />
      <MoodTile emoji="😟" label="焦虑" />
      <MoodTile emoji="😪" label="疲惫" selected />
      <MoodTile emoji="😐" label="还行" />
      <MoodTile emoji="😊" label="很好" />
      <MoodTile emoji="😌" label="平静" />
    </div>
  );
}

export function Selected() {
  return (
    <div className="w-24">
      <MoodTile emoji="🤩" label="兴奋" selected />
    </div>
  );
}

export function Dimmed() {
  return (
    <div className="grid w-48 grid-cols-2 gap-2.5">
      <MoodTile emoji="🙏" label="感恩" selected />
      <MoodTile emoji="😵‍💫" label="困惑" dimmed />
    </div>
  );
}
