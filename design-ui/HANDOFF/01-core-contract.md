# 01 · 数据核心与契约

> 源文件：`core/daycore-core.js`（358 行，纯 JS IIFE，挂 `window.DcCore`）。
> **这是四个前端唯一共享的东西。重建时先做它，且逐条对齐语义。**

## 1. 存储与版本

- `localStorage` key：`daycore-v4-core`。四端共读共写同一份。
- 载入时若 key 不存在或版本不符 → 播种（默认 `healthy` 场景）。
- 载入时做一次迁移：给缺 `lockLevel` 的旧块补算锁级别（见 §5）。
- 每次写操作后 `emit()` → 持久化 + 通知所有订阅者。前端用 `subscribe(fn)` 拿变更。

## 2. 领域实体

```ts
type Phase = 'future' | 'now' | 'recon' | 'stone';   // 由 phaseOf(block) 计算，不存储

interface Block {                      // 时间块：日程的原子
  id: string; date: string;            // 'YYYY-MM-DD'
  start: string;                       // 'HH:mm'
  dur: number;                         // 分钟
  title: string;
  type: 'task' | 'appointment' | 'relax' | 'meal' | 'break';
  origin: 'manual' | 'auto';           // auto = 小禾排的
  status: 'planned' | 'done' | 'missed';
  reconciled: boolean;                 // 是否已回顾过
  note: string;
  lockLevel: 'hard' | 'soft' | null;   // 见 §5
  lockReason: string | null;
}

interface Proposal {                   // 提案：AI 的一切建议都是提案
  id: string; state: 'pending' | 'accepted' | 'rejected';
  level: 'L1' | 'L2' | 'L3';           // 打扰等级
  kind: 'timed' | 'card' | 'decision'; // timed = 画在时间轴上的虚影
  title: string; summary: string; reason: string; evidence: string;
  date: string | null; start: string | null; dur: number | null;
  btype: Block['type'];
  lockLevel?: Block['lockLevel'];      // 接受后传给生成的块
  lockReason?: string | null;
  rows: Array<{ id: string; label: string; state: 'pending'|'accepted'|'rejected' }> | null;
}

interface Op {                         // 账本条目：append-only
  id: string; at: number; label: string;
  domain: 'schedule' | 'care' | 'archive' | 'system';
  agent: boolean;                      // 是否 AI 发起
  silent: boolean;                     // 静默动作（不打扰）
  undoable: boolean; undone: boolean;
  payload: object;                     // 反向补偿所需的全部信息
}

interface Wish       { id; text; effort: number|null; at; status: 'active'|'done'|'dropped' }
interface Material   { id; title; body; kind: 'academic'|'life'; at; source }
interface Memory     { id; text; type: 'preference'|'fact'; at }
interface Assignment { id; course; title; dueAt: string /* 'YYYY-MM-DDTHH:mm' */; done: boolean }
interface Mood       { id; e: string /* emoji */; l: string /* 标签 */; note; source: 'user'|'agent'; at }
```

State 根上还有：`scenario`、`userName`、`assistantName`（默认「小禾」）、`l2Prompt`、`language`、
`themes: {liuli, zhiyu, ting}`、`customThemes[]`、`rhythm: {wake, sleep, pinned, learning}`、
`boldness: {schedule, habit, archive}`（胆量系数，§7）、`prefs`、`pushes[]`、`exchange`、`day`。

## 3. 读时石化（petrify）

**阈值：`min(now − 5h, 今天 00:00)`。** 早于阈值的块 `phaseOf` 返回 `'stone'`。

- `future` — 还没开始
- `now` — 正在进行（`start ≤ now < start+dur`）
- `recon` — 已过去但还在阈值内，可以标记完成/没做
- `stone` — 已石化，**只读**。不能拖动、不能改时间、不能改状态；只能加注释或「重新安排」（生成一个新的未来块 + 提案）。

石化是**读时计算**，不是定时任务——不需要后台 job。

## 4. 撤销 = 反向补偿

不做快照回滚，每种操作各自定义逆操作。`undo(opId)` 按 `payload.type` 分派：

| payload.type | 逆操作 |
|---|---|
| `add_block` | 按 blockId 删除 |
| `remove_block` | 把 `payload.block` 推回去 |
| `move_block` | `Object.assign(block, payload.prev)` |
| `status` / `set_mark` / `set_fields` / `reconcile` / `note` | 恢复 `payload.prev` |
| `bulk_blocks` | 删掉 `added[]`、推回 `removed[]`（一批块 = 一条可撤销操作） |
| `assignment_done` | 恢复 `prev` |
| `add_wish` / `add_mood` / `add_material` / `add_memory` | 按 id 删除 |
| `remove_material` / `remove_memory` | `unshift` 回去 |

**规则**：任何新增写操作都必须同时写好逆操作，否则不许标 `undoable`。

## 5. 锁定级别（课程硬锁 / 会议软锁）

真实生活里有些时间不由用户定。`deriveLock(block)` 在建块时自动推断：

```
type !== 'appointment'              → lockLevel = null（自由）
type === 'appointment' 且标题像课程  → 'hard'，默认「课程时间由课表决定」
   匹配 /（课）|课$|课程|讲座|实验课|导论|Lecture|Lab\s*课/i
type === 'appointment' 其余          → 'soft'，默认「和别人约好的时间」
```

`moveBlock(id, to, agent)` 的返回：

| 情况 | 返回 |
|---|---|
| 石化块往后挪 | `{ stone: true }` |
| `hard` 锁 + 非 AI | `{ locked: 'hard', reason }` — **拒绝** |
| `soft` 锁 + 非 AI + 未置 `to._confirmed` | `{ locked: 'soft', reason, pending: to }` — **要二次确认** |
| 通过 | `{ ok: true, opId }` |

AI（`agent = true`）可以越过锁——排期时它需要知道这些是约束，而不是被挡住。提案带 `lockLevel/lockReason`，接受后传给新块。

**UI 表达（三层，缺一不可）**：
1. **静态可见** — 锁定块有实线细边框 + 小图标（琉璃：灰色小锁；纸屿：图钉），样式走 CSS 变量以便 AI 主题覆盖。
2. **触觉反馈** — 拖 `hard` 锁块只跟手 7px 然后阻尼回弹 + 轻微摆动（440ms），移动端伴 12ms 震动。**完全不动会被当成 bug**。
3. **事后解释** — 回弹后浮出原因 **+ 替代动作**（请假 / 标记冲突）。绝不只说"不行"。

## 6. 完整 API 面（`window.DcCore`）

```
// 查询
state / subscribe(fn) / emit() / sweep()
blocksOf(date) ghostsOf(date) stack() phaseOf(b) blockRange(b) petrifyLine()
gapsOf(date, minMin) flowNow() l0Today() brief() briefVisible() dismissBrief()
ledgerByDay(days) river() radar() moodToday() anyMoodToday() unreadPush()
listCategories() boldness() pushBudget()

// 写：日程
addBlock(o, agent) moveBlock(id, to, agent) setStatus(id, s) setMark(id,'none'|'done'|'missed')
setBlockFields(id, {title,type,dur}) setNote(id, note) reconcile(id, verdict, note)
removeBlock(id) bulkBlocks({add[], removeIds[], label}) proposeReschedule(id)

// 写：提案
accept(pid, rowId) reject(pid, rowId) redeem(...) followUp(pid, q) consumeExchange()

// 写：其它域
wishAdd(text, effort, agent) wishSet(id, status) moodRecord(o)
materialAdd(o, agent) materialRemove(id) memoryAdd(o, agent) memoryRemove(id)
setAssignmentDone(id, done) markPushRead(id) undo(opId)

// 意图与对话
captureIntent(text, opts)   // 单句 → 直接执行 / 提案 / 记忆 / 愿望 / 资料
chatSend(text)              // 带工具调用轨迹的对话

// 系统
scenarioSet('healthy'|'crisis'|'cold') theme(front) setTheme(front, id)
aiTheme(desc, base) createTheme(i) patchTheme(id, ch) deleteTheme(id)
patch(partial)   // 单参：Object.assign(state, partial)
setPref(k, v) setCategory(...) bindChannel/verifyChannel/unbindChannel
```

> **`patch` 是单参数的**（`patch({assistantName:'x'})`）。历史上按两参调用过，结果把字符串 `Object.assign` 进 state 根、污染了共享账本。重建时给它明确类型签名。
> `assistantName` / `language` 在 **state 根**上，不在 `prefs` 里——用 `patch()` 改，不要用 `setPref()`。

## 7. 胆量系数（boldness）

`{schedule, habit, archive}`，每个 `{score: 0..1, evid: number}`。用户接受 AI 提案 → 升；拒绝 → 降。分数决定 AI 主动打扰门槛（`pushBudget()`）。冷启动分数低 → AI 几乎不主动说话；被接受多了才敢多提。

## 8. 演示场景（三颗种子）

| 场景 | 内容 |
|---|---|
| `healthy` 健康日 | 十几件静默动作、两三处显影、两张提案卡；今天 6 个块、3 个已完成 |
| `crisis` 危机日 | 两条死线打架 + 一条「20 小时未合眼」的关怀；社团例会显式软锁 |
| `cold` 冷启动 | 空账本。**无欢迎页、无强制导入**——空状态本身即引导 |

`scenarioSet()` 保留 `themes` 与 `customThemes`，其余全部重播种。

## 9. 接真后端

mock 方法名与 `API_CONTRACT.md` 端点基本一一对应。迁移路径：写方法换 `fetch`，读方法换本地缓存 + SWR。**领域语义（石化 / 反向补偿 / 锁级别 / 提案）在服务端也必须成立**，否则多端一致性会破。
