# 数据层与迁移手册

> 实时文档：改 domain/存储必须同批更新本文件。最后全面核对：2026-07-14。

## Domain 实体（internal/domain/，纯 struct + 接口）

| 文件 | 实体 | 备注 |
|---|---|---|
| chat.go | ChatThread{Summary 滑窗摘要, Archived}, ChatMessage{ToolEvents, 阶段2 加 Status} | |
| companion.go | Message, Role 常量(user/assistant/system/tool), CompanionMemory | |
| courses.go | Course, Assignment{Source: canvas/manual; Status: pending/planned/done/dismissed} | Assignment 与 dayplan **无外键**，只作 auto-plan LLM 上下文 |
| material.go | Material{Category, Title/Summary/Body/Source/MimeType/StorageRef/Tags} | ⚠️ MaterialRepository 的 ctx 参数是 `interface{}` |
| material_category.go | MaterialCategory 注册表（10 类，note/diet/health/academic/travel 默认开）+ MaterialCategoryByID | 加类别 = 此文件加一条目（含 `Names` 与 `Hints`，**两者都要每种 Supported locale 齐全**，见下「Go 侧显示名的多语言机制」）；写侧枚举校验在 handlers_materials_full.go `normalizeCategory`（空→note，未知→400；读侧不拦 legacy 自由文本）；会话级开关在 SessionPrefs.MaterialCategories |
| plan.go | DayPlan, TimeBlock, BlockType, TimeMode(floating/fixed/local), Origin(auto/manual/rule), **LockLevel(""/none/soft/hard)** | 见下「块的锁定与重捞字段」 |
| rules.go / memory.go / mood.go / session.go / user.go / theme.go / prompts.go | 同名实体 | user.TokenVersion json:"-" |
| operations.go | OperationLog（**加 Domain 列**）, AICallLog, AdminStats, **OpDomain 常量 + OpDomainOf()** | 见下「操作域」 |
| repository.go | 全部 Repository 接口 + Store 组合接口 | 新方法先在这里定义 |
| searcher.go | Searcher/SearchQuery/SearchResult（+ 阶段5 MaterialFTS 能力接口） | |

## 存储后端（4 个：sqlite/postgres/mysql/mongo）

- `sqlstore/`：每实体一文件；方言拆分 dialect_sqlite/postgres/mysql.go；测试 store_test.go（round-trip）。
- `mongostore/`：每实体一 repo 文件，store.go 有 index specs；**无测试**。

## 新增实体的完整路径

domain 加 struct → repository.go 加接口 + Store 组合 → sqlstore 加文件 + 三方言 DDL → mongostore 加 repo 文件 + accessor → （如需）index specs。

## 给已有表加列（ColumnMigration 机制）

1. `sqlstore/dialect.go`：`ColumnMigration{Table, Column, DDL}`；共享清单函数 `sessionColumnMigrations(textType)`——名字带 session 但**实际覆盖 sessions/memory_facts/users（阶段 2 起含 chat_messages）**，直接往里 append 即可，三方言自动获得（只有类型 token 不同：sqlite/pg "TEXT"，mysql "VARCHAR(64)"）。
2. 执行：`store.go` Migrate 先跑建表 `Migrations()`，再对每个 ColumnMigration 用 `ColumnExistsQuery()` 查列（sqlite pragma_table_info / pg+mysql information_schema），`sql.ErrNoRows` 才执行 DDL（ALTER ADD COLUMN 非幂等）。
3. 同时改：三方言**建表 DDL** 也要加同列（新库直接建全）；sqlstore 实体文件的 SELECT/INSERT/Scan；mongostore doc struct（bson tag，mongo 无需迁移）。
4. domain struct 加字段。

## ConditionalMigration

见上方「Material 搜索」一节——机制与 FTS 一起落地（2026-07-15）。新的可选特性 DDL 一律走它，不要放 Migrations()（那里失败会阻断启动）。

## Material 搜索（原生 FTS + 子串兜底，2026-07-15 落地）

- `search/material.go` MaterialSearcher：先探 `store.(domain.MaterialFTS)` → 原生索引带真实打分；ok=false/err（索引缺失、sqlite 查询词<3 rune、mongo 无 CJK 分词命中为空）→ 回退 `Materials().List` 子串扫描（Score=1，召回下限保证）。Index/Deindex 维持 no-op（四引擎全自动维护）。
- 索引实现：SQLite FTS5 external-content 虚表 + 三触发器 + 建表时 rebuild 回填（`dialect_sqlite.go`，modernc.org/sqlite v1.34.4 验证带 FTS5+trigram）；PG tsvector 生成列('simple') + GIN（需 PG≥12）；MySQL FULLTEXT ngram；Mongo text index（store.go specs）。查询在 `sqlstore/fts.go` / `mongostore/fts.go`。
- **ConditionalMigration 机制**（`dialect.go` + `store.go`）：`{Name, CheckQuery, DDLs}`，CheckQuery 无行才执行；失败只进 `Store.warnings`（main.go 启动打日志）不阻断启动；成败记录在 `Store.condApplied[name]`，FTS 查询据此短路。⚠️ 只测过 sqlite 真机；pg/mysql 的 DDL 语法按文档写就，首次接入时留意启动 warning。

## 块的锁定与重捞字段（2026-07-26 落地，体验内核 v2.3）

`TimeBlock` 新增 6 个字段，**零 DDL** —— blocks 存在 `day_plans.blocks` 一个 TEXT 列里（`sqlstore/dayplans.go` 的 `marshalJSON`），mongo 是内嵌数组（`mongostore/dayplan_repo.go`）。给 TimeBlock 加字段不需要动任何建表 DDL 或 ColumnMigration。

| 字段 | 值域 | 要点 |
|---|---|---|
| `LockLevel` | `""` / `none` / `soft` / `hard` | **三态，不是两态**。`""`=从未派生（设计原型的 `undefined`），`none`=派生过且自由（原型的 `null`）。用命名字符串不用指针：Go 1.23 无 `omitzero`，指针无法区分「键缺失」与「显式 null」 |
| `LockReason` | 自由文本 | hard 默认「课程时间由课表决定」，soft 默认「和别人约好的时间」（设计原件定稿原文） |
| `LockSource` | `""` / `derived` / `user` / `agent` | **区分「用户手动解锁」与「从未派生」的唯一手段**。没有它，读时补算会把用户解锁的课重新锁死，toggle-lock 直接失效 |
| `Note` | 自由文本 | 完成时可附一句（感受／哪里没做完）。永不主动索要，agent 读它了解用户 |
| `RescheduledFrom` | 原块 ID | 重捞链式追踪 |
| `RescheduleCount` | int | 已被重捞几次，达阈值（默认 2）停止并写 Memory(preference) |

三个锁字段与 `Note` 都是 **no omitempty**，理由同 `Completed`/`IsAchievement`：`revertPlanUpdate` 从 before 快照逐键重建 changes，键不在 JSON 里 revert 就恢复不了。

**不做的**：`Status` 三态（planned/done/missed）与 `Reconciled`。产品判断是「没勾和没做是一回事」，不要求用户对账 —— 克制是战略。设计侧 `01-core-contract.md §2` 的 `status`/`reconciled` 两字段在后端**有意不实现**，四端重建时用 `Completed` 映射。

回归测试：`sqlstore/store_test.go` 的 `TestBlockLockAndNoteRoundTrip`，特别断言 `LockNone`（手动解锁）与 `LockUnset`（从未派生）不被混淆。

## 读时石化（Phase，2026-07-26 落地）

`internal/timeutil/petrify.go` + `internal/domain/phase.go`，**纯读时函数，无表、无 job、无迁移**。参数错了下一次读就自愈。

- `PetrifyLine(now, loc, horizon)` = `min(now − horizon, 今天 00:00)`，horizon 默认 5h（`PetrifyHorizonDefault`）。白天 midnight 胜出 → 今天永不石化；凌晨 rolling 胜出 → 夜猫子还能改昨晚；05:00 起 rolling 追上 midnight，昨天关闭。
- `TimeBlock.Span(planDate, loc)` 解析绝对起止：有 `UTCTime` 锚点优先用它（fixed/local），否则按 `loc` 解释墙钟（floating）；无 `Time` 或无日期 → `ok=false`，**未定时的块永不石化**。
- `PhaseAt(start, end, now, line)` → `future` / `now` / `recon` / `stone`。边界照抄原型：`end < line` 才 stone（**恰好等于 line 不算**），`end <= now` 为 recon，`start <= now` 为 now。
- `TimeBlock.Frozen(...)` 是写路径唯一要 gate 的谓词。

**DST 行为是实测的，不是推的**（Go 文档称空洞情形「not guaranteed」，所以观测值即契约）：spring-forward 空洞 Go **向前回退**（`02:30` → `01:30 EST`，不是推到 03:30）；fall-back 歧义取**第一次**（EDT）。两者与 5 个场景一起固化在 `api/testdata/petrify-vectors.json`，Go 侧 `phase_test.go` 读同一份跑表驱动，将来 TS 侧 vendored 同一份 —— 这是防「两颗脑子」的地基。

## Proposal（提案统一资源，2026-07-26 建模）

`internal/domain/proposal.go`。**虚影 / 确认卡 / 决策卡 / 集成端审批是同一个对象**，待发池就是这张表 `state=pending` 的过滤视图 —— 不建 `/api/outbox`，靠 `kind`(timed/card/decision) + `level`(L1/L2/L3) + `delivered_at` 三个字段区分，不靠三张表。

- **前端共享字段的 JSON 名逐字照抄设计契约**（`dur`/`btype`/`lockLevel`），哪怕不合后端命名 —— 那个 interface 正是四端共享的东西，改名等于把它要统一的东西拆开。
- **`rows[]` 用 JSON 列不建子表**。首要理由：`sqlstore` **全包没有事务**（`grep BeginTx` 零命中），行级接受要同时改「这一行的状态」与「父卡是否全部落定」，子表方案得先给四引擎各造一套事务抽象；JSON 列 + `Rev` 乐观锁是一次单行 UPDATE。先例是 `day_plans.blocks`。
- **四态而非三态**：`expired` 不并进 `rejected`。共识 15 要求过期提案就地作废、用户永不可见，共识 3 要求账本留痕 —— 只有区分「显式拒绝」与「静默过期」两者才能同时成立。前端契约仍是三态，因为过期的从不投递。
- 列名用 `rows_json` / `start_time`：`ROWS` 与 `START` 是 MySQL 8 保留字（`temp_contexts.key` 已踩过同类坑）。

**TTL 不对称（共识 14）** —— 沉默的含义取决于事情做没做：

| policy | 到期状态 | 到期时执行 op？ |
|---|---|---|
| `silence_accepts`（先斩后奏） | `accepted` / `silence` | 否 —— op 建卡时已执行，到期只是关闭撤销窗口 |
| `silence_rejects`（先奏后斩） | `expired` / `silence` | 否 —— 从未执行，就地作废 |

**一条硬不变量**：`silence_accepts` 必须 `len(AppliedOpIDs) > 0`，在 `Validate()` 建卡时就拦。「做了但撤不了」正是撤销体系存在的理由，不能留到运行期才发现。

**TTL 默认值**（`ProposalExpiry()`，均可配置）：决策卡 45s / 异步 90s · 先斩后奏撤销窗口 2h · 普通 L2 卡 6h **且不越当地午夜**（今天的事不该明天还挂着）· L1 占位到它所指时段的开始时刻（§4 明写，不可调）· 晨卡到当地 12:00（共识 17，不可调）。

## 默契 rapport（原「胆量系数」，2026-07-26）

`internal/rapport/`，纯函数零依赖。**分数不是事实，是账本的读数** —— 任何缓存都必须能靠重放账本重建，所以 `Replay`（全量）与 `Folder`（增量）走同一份折叠逻辑，两份实现迟早会分叉，那时缓存就凌驾于账本之上了，正好反了。

**增减不对称，别「整理」成对称**（数值抄自 `daycore-core.js:186/187/222`）：接受 `+0.03` · 拒绝 `-0.05` · **撤销 agent 做的事 `-0.07`**。信任慢涨快跌是刻意的。上下限 `[0.05, 0.95]`，`< 0.45` 为学徒期（卡面亮证据）否则管家期（理由沉入卡内，追问可召回）—— 某域被连撤会自己滑回学徒期，不需要谁去翻开关。

- **四个域**：schedule / habit / archive / care，system 不计分。§7 正文写三个，但设计原型三颗种子都带 care，危机日还有 care 域的提案。
- **冷启动不对称**：archive/care 起点 0.30，schedule/habit 0.10 —— 归档错了翻目录就改，课挪错了就是缺课（共识 3 的归档偏置）。
- **撤销的记分归属**：`revert` op 的域是它自己的，但罚分要记到**被撤那条 op 的域**上；被撤的若不是 agent 发起（用户撤自己的编辑），不扣分；原 op 不在重放窗口内则跳过而非猜测。
- **拒绝不涨证据**：否则门控可以靠「被拒绝很多次」解锁。

**门控（`gate.go`，体验内核 §8）** —— 节流的是主动性不是能力：响应式智能 Day 1 全功率，要挣的只是「不请自来地打扰」。默认阈值（可配）：schedule 3 / habit 3 / archive 1 / care 2 次证据解锁该域首张自发 L2；**首刀**额外要求置信度 ≥ 0.80 —— §8 只说「必须是全库最高的那张」，光这条会让一池弱候选靠技术性条件混过去。

## 节律 rhythm（2026-07-26 落地，体验内核 §5 第三层）

`internal/rhythm/`，纯函数零依赖零存储。学用户惯常的起床/入睡时刻，Daemon 据此摆放定时工作 —— 固定 07:00 的导语对不在 07:00 起床的人是每天一次小小的冒犯。

**节律日不按午夜切，按 `DayCutHour`（默认 04:00）切**。凌晨 2 点还在干活的人是在过一个很长的周二，不是一个很早的周三；按午夜切会把这一场记成两个短日，两边都学不到东西。中位数也必须在节律日空间里取：按墙钟分钟排序，02:00 的睡点排在 22:00 前面，夜猫子一周的中位数会算到中午去。

**只有真实交互算「醒着」**：`intent` / `ui` / `heartbeat`。**刻意排除**一切可能在人睡着时发生的信号 —— 插件定时推 Canvas、绑定的 QQ 通道来消息。算进去就会教会 Daemon「手机凌晨 3 点响的人是夜猫子」，然后把导语排到那个点。未知 kind 一律不算：新信号源必须显式加入，因为误加入的失败是静默的，几周后才看得出来。

**三个定时时刻由两个身体时刻 + 两个偏移派生**，不是三个独立数字：

| 派生 | 冷启动值 | 来源 |
|---|---|---|
| `PlanAt` = `Wake − PlanBefore(3h30m)` | 04:00 | auto-plan 跑在静默窗 |
| `BriefAt` = `Wake` | 07:30 | 惯常起床时刻 |
| `ReviewAt` = `Sleep − ReviewBefore(90m)` | 21:00 | 入睡前窗口 |

冷启动 `Wake 07:30 / Sleep 22:30` 配默认偏移**恰好复现设计写死的 04:00 / 07:30 / 21:00**（`TestColdStartMatchesDesignFallbacks` 守着这条）。派生而不是存三个数，好处是学到节律时三个一起动，且不会漂成「复盘排在睡觉之后」这种自相矛盾的组合。⚠️ `PlanAt` 有护栏：睡不到 3h30m 的人，`Wake − PlanBefore` 会落到他上床**之前**，auto-plan 会在他还看着的时候改写当天 —— 此时坍缩到睡眠区间中点。

- `MinDays`（默认 5）之前不采信学习结果，但**照样报告已观测天数** —— 足迹页要显示「已学 3 天」的进度。两天的数据是轶事。
- 只有一条信号的一天不算证据（一次 15:00 的打卡不能说明 15:00 起床）。
- `Pin("11:00","03:00")` = 用户手动钉住（「我就是夜猫子，别管我」），`Learn` 原样返回，检查放在包里而不是每个调用点。
- 中位数取偶数长度时用**较小的中间值**而不是两者平均：23:00 与 02:00 的平均是 00:30，那不是任何人的睡点，而取观测值总是。

**20h 关怀（Protector）**：`CurrentRun` 找出「上一次长间隔之后到现在」这段连续清醒，`NeedsProtector` 判阈值（默认 20h）。

- **连续时长量到 now 而不是量到最后一条信号** —— 累到没力气再点的人正是这个功能要照顾的对象。
- `IdleBreak`（默认 3h）刻意取长：安静的一个下午不是小睡，认错了就会在最需要的时候把计数器清零。取长意味着 Protector 很少响、响就是真的 —— 少触发是这个功能正确的失败方向，它的语气是担心不是训斥。
- 最后一条信号已经超过 `IdleBreak`：返回**空 run**，读作「睡了」而不是「一直醒着」。否则这个功能会为了叫人去睡觉而把人吵醒。
- `NeedsProtector` 只是纯谓词，**不管要不要真发** —— 推送预算、注意力阶梯、这一 run 是不是已经发过，三样都归调用方。这样阈值可以脱离调度器测。

**工作日/周末是否分开学**：设计里明写的口子，暂不做。

## 心情注册表（`domain/mood_kind.go`）

12 种，集合/顺序/emoji 抄自设计原型（四端画的就是这张表）。**存 id 不存标签** —— 中文标签是给人看的，存它会让改名变成数据迁移，也会让英文界面为同一种感受存出不同的行。`Valence`（−2..+2）只为算趋势存在，从不展示，且刻意粗糙：疲惫与压力大是种类之差，细分是假精度。

## Go 侧显示名的多语言机制（`i18n.Text` + `i18n.Pick`）

提示词模板按 `prompts/<locale>/*.tmpl` 分目录，天然可扩展；**但结构体里的显示名此前是 `NameZH`/`NameEN` 两个具名字段** —— 加第三种语言要改每一个带这对字段的结构体和每一处 `if HasPrefix(locale,"en")` 分支，语言扩展变成了 schema 变更。已改为 locale map：

```go
type Text map[string]string          // {"zh-CN": "开心", "en-US": "Happy"}
func Pick(t Text, locale string) string
func Missing(t Text) []string        // 缺哪些 Supported locale
```

**回退链**（`Pick`，四步）：精确 tag → 同语言的其他地区（`zh-TW` 先找到 `zh-CN`，不会掉去英文）→ `Default`（en-US）→ 表里任意一条。

- 第 2 步让「有这门语言但没这个地区」不掉到另一门语言。
- 第 3 步落在 `i18n.Default`，与 `i18n.Resolve` 处理无法识别的 `Accept-Language` 是同一个终点 —— 走正常协商的调用方与直接传裸 tag 的调用方得到同一个答案。
- 第 4 步是**半翻译的表宁可显示看不懂的语言也不显示空白**：缺翻译是内容缺口，不是渲染失败。最后一步按 `Supported` 顺序取，不吃 map 迭代随机性，同一个缺口每次渲染一致。

**加一门语言的完整路径**：`i18n.Supported` 加一项 → 跑 `go test ./internal/domain/`，`TestRegistriesCoverEverySupportedLocale` 会把所有缺的条目一次列全 → 补完即止。这条测试是启动期硬校验（提示词双 locale）的测试期对应物：注册表缺一条只降级到别的语言，不该把服务器带下线，但也不该悄悄发出去。

另有 `PickFrom[T](map[string]T, locale)` —— 同一条回退链，但键值是任意类型（一整套日期词汇、一组格式串）。`Pick` 额外把空串当作缺失，`PickFrom` 不判断值，因为只有调用方知道自己的类型里「空」是什么意思。

**已改造的 16 处**（显示名全部 `json:"-"`，**API 形状不变**，端点仍按请求 locale 吐一个 `name` 字符串）：

| 位置 | 内容 |
|---|---|
| `domain/mood_kind.go` | `MoodKind.Names` → `MoodName(locale)` |
| `domain/material_category.go` | `Names`/`Hints` → `Name(locale)`/`Hint(locale)`。`Hints` 是给 inbox 分类器的提示词行 —— 中文提示塞进英文提示词，本身就是让分类器答错语言的常见原因 |
| `domain/lock.go` | `defaultLockReasons`，与 `api/lock-rules.json` 的 `defaultReason` 对齐 |
| `domain/weather.go` | `precipFormat`（`"降水%d%%"` / `"precip %d%%"`）—— 整段进表含空格：中文贴着数字、拉丁文要空格，这是译者的判断，不是运行时按 script 推的 |
| `weather/openmeteo` | `codeTexts` 28 个 WMO 天气码 `[2]string` → `i18n.Text`，加 `wmoUnknown` |
| `ai/datectx.go` | `dateTables`（星期名 / 明天后天 / 本周下周 / `shortFn` / 表格行格式）走 `PickFrom` |
| `server/worker.go` | `gapSuggestionText` / `morningUserText` / `eveningUserText` / `replanUserText` / `deadlineTables` |
| `server/agent.go` | `wrapUpNudgeText` |
| `server/handlers_ai_companion.go` | `personaHeading` / `wishPoolHeading` |
| `server/handlers_ai_companion_async.go` | `asyncErrorMsg` |
| `server/handlers_inbox.go` | `categoryLine`（`- %s（%s）：%s` / `- %s (%s): %s`，全角标点属于翻译的一部分） |

⚠️ 带 `%` 动词的条目（`precipFormat`、`categoryLine`、`replanUserText`、`deadlineTables`、`dateTables.rowFmt/relFmt`）：新语言必须保留同样的动词、同样的顺序。`TestPrecipFormatKeepsItsVerb` 挡住漏掉 `%d` 的翻译。

**顺带修掉的既有缺陷**：`datectx` 的相对日期表在 en-US 下渲染成 `| ThisMonday | … |`（中文的 `本周` + `三` 是连写的，英文照抄就没了空格）。现在 `relFmt` 分 locale 给，`"%s%s"` / `"%s %s"`。

**没有改、也不该改成 `Text` 的**：`ai/prompts.go` 的 `DefaultPersona`/`HardBoundaryReminder`、`worker.go` 的 `buildBriefSystemPrompt`/`buildReplanSystemPrompt` —— 这四段是多段落提示词，正确去处是 `prompts/<locale>/*.tmpl`（已有机制，已带启动期双 locale 硬校验，且落进 `prompt_overrides` 后控制台可改），塞进 map 只会把它们钉死在 Go 里。属计划里**批次 5 的「提示词一次性收口」**，与定稿语气那一遍合并做。

**契约侧**：`api/lock-rules.json` 新增 `localeFallback`（`chain` + `defaultLocale`），四端实现同一条回退链；`lock_test.go` 从夹具读 `defaultLocale` 而不是写死，两侧改一处即可。

**外部 provider 的语言参数是另一回事**（`internal/weather/lang.go` 的 `Lang(locale, codes, fallback)`）：每个上游有自己的语言代码空间（QWeather `zh`/`zh-hant`、OpenWeatherMap `zh_cn`/`zh_tw`、wttr.in 只有一种中文），表跟着 provider 走，只共享查表逻辑。上游没有的语言退回它自己的默认值 —— 语言不对的天气预报仍然告诉你会下雨。

## 操作域（OperationLog.Domain）

`schedule` / `habit` / `archive` / `care` / `system` 五个值，`OpDomainOf(action)` 按 action 前缀派生。两个消费者：河流按域上色、**默契（rapport）按域计分**（前四个域计分，system 不计）。

- 走 `ColumnMigration` 加列（`dialect.go` 的 `sessionColumnMigrations`），三方言建表 DDL 同步加。
- 存量行默认 `''`，**读时用 `OpDomainOf` 派生而不回填** —— 账本 append-only，且派生结果可重放重现。写入时 `Add()` 若未设则自动派生并回写进 struct。
- 未知 action 一律落 `system`，避免新 action 悄悄影响默契评分。
- 测试：`domain/operations_test.go` + `sqlstore/store_test.go` 的 `TestOpLogDomain`（含模拟存量空值行）。

## 其他要点

- TempContext：session 级 KV，TTL 1h（inbox upload 的图片 base64 <1MB 存这里；阶段 3 起归类草稿 `inbox:draft:<id>` 也存这里）。
- Assignment 手动创建（阶段 4）：`CanvasID = "manual:"+uuid` 走现有 `UpsertByCanvasID` INSERT 分支，零新 repo 方法；Canvas 再导入不碰 manual 行。
- sessions.preferences 列：SessionPrefs JSON 序列化（worker.go），合并端点 GET/PATCH /api/session/preferences——加用户级开关优先用它，零迁移。⚠️ 2026-07-15 修复：此前 sqlstore/mongostore 的 session Get/Update 都没接 preferences 列，偏好从未持久化过（含 worker 的主动性开关）；同类 bug 也出现在 users 的 data_session_id（同日修复）。**新加列后必须同时检查 SELECT/Scan/Update 三处**。
- inbox 草稿：`inbox:draft:<id>`（TempContext，1h TTL）存 AI 归类结果，`POST /api/inbox/commit` 消费；上传文件是 `inbox:<tempID>`。
