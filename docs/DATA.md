# 数据层与迁移手册

> 实时文档：改 domain/存储必须同批更新本文件。最后全面核对：2026-07-14。

## Domain 实体（internal/domain/，纯 struct + 接口）

| 文件 | 实体 | 备注 |
|---|---|---|
| chat.go | ChatThread{Summary 滑窗摘要, Archived}, ChatMessage{ToolEvents, 阶段2 加 Status} | |
| companion.go | Message, Role 常量(user/assistant/system/tool), CompanionMemory | |
| courses.go | Course, Assignment{Source: canvas/manual; Status: pending/planned/done/dismissed} | Assignment 与 dayplan **无外键**，只作 auto-plan LLM 上下文 |
| material.go | Material{Category, Title/Summary/Body/Source/MimeType/StorageRef/Tags} | ⚠️ MaterialRepository 的 ctx 参数是 `interface{}` |
| material_category.go | MaterialCategory 注册表（10 类，note/diet/health/academic/travel 默认开）+ MaterialCategoryByID | 加类别 = 此文件加一条目（含 PromptHint）；写侧枚举校验在 handlers_materials_full.go `normalizeCategory`（空→note，未知→400；读侧不拦 legacy 自由文本）；会话级开关在 SessionPrefs.MaterialCategories |
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
