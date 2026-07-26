import { TimeBlockCard } from "@daycore/ui";

export function ADayTimeline() {
  return (
    <div className="flex w-80 flex-col gap-3">
      <TimeBlockCard time="08:30" title="晨间拉伸 + 早餐" type="meal" durationMin={30} onComplete={() => {}} />
      <TimeBlockCard time="10:00" title="团队周会" type="appointment" timeMode="fixed" durationMin={60} onComplete={() => {}} />
      <TimeBlockCard time="12:30" title="午休 · 散步" type="break" durationMin={45} onComplete={() => {}} />
      <TimeBlockCard time="14:30" title="完成作业草稿" type="task" durationMin={90} onComplete={() => {}} />
    </div>
  );
}

export function States() {
  return (
    <div className="flex w-80 flex-col gap-3">
      <TimeBlockCard time="09:00" title="晨会（已结束）" type="appointment" past durationMin={30} />
      <TimeBlockCard time="15:00" title="写周报" type="task" completed durationMin={60} />
      <TimeBlockCard title="给妈妈打电话" type="task" durationMin={20} onComplete={() => {}} />
    </div>
  );
}

export function Achievement() {
  return (
    <div className="w-80">
      <TimeBlockCard time="20:15" title="完成：4-7-8 呼吸" type="relax" durationMin={2} isAchievement />
    </div>
  );
}

export function Types() {
  return (
    <div className="flex w-80 flex-col gap-3">
      <TimeBlockCard time="11:00" title="任务块" type="task" onComplete={() => {}} />
      <TimeBlockCard time="13:00" title="约定块" type="appointment" timeMode="fixed" onComplete={() => {}} />
      <TimeBlockCard time="16:00" title="放松块" type="relax" onComplete={() => {}} />
    </div>
  );
}
