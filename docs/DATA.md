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
| rules.go / memory.go / mood.go / session.go / user.go / theme.go / prompts.go | 同名实体（**MoodCheckin 加 Source/Note**） | user.TokenVersion json:"-"；MoodCheckin 见下「心情窗口」 |
| operations.go | OperationLog（**加 Domain 列**）, AICallLog, AdminStats, **OpDomain 常量 + OpDomainOf()** | 见下「操作域」 |
| repository.go | 全部 Repository 接口 + Store 组合接口 | 新方法先在这里定义 |
| searcher.go | Searcher/SearchQuery/SearchResult（+ 阶段5 MaterialFTS 能力接口） | |
| wish.go | Wish{EffortMin} —— 许愿池 | 5 条 HTTP 路由 + `wishes` 表 |
| proposal.go | Proposal + ProposalOp/ProposalRow + ProposalFilter | 批次 C 六表之首；「提案是统一资源」（虚影/确认卡/决策卡/审批同一个对象） |
| coordination.go | Lease{Fence} + JobRun{Attempts,Status} | 选主与任务场次占有 —— 全仓唯一的互斥手段 |
| derived.go | RapportState/RapportScore、RhythmProfile/RhythmDay、LocaleOverride | 读时派生的缓存 + 语言包 DB 层 |
| phase.go | Phase（future/now/recon/stone） | 读时石化，无表 |
| lock.go | LockLevel 常量 + 派生规则 | 配 `api/lock-rules.json` 契约夹具 |
| mood_kind.go | 心情注册表（12 种 + valence） | 存 id 不存标签 —— 多语言扩展的前提 |
| tempcontext.go / channel.go / feedback.go / errors.go | TempContext / ChannelBinding / FeedbackLog / 哨兵错误 | |

## 存储后端（4 个：sqlite/postgres/mysql/mongo）

- `sqlstore/`：每实体一文件；方言拆分 dialect_sqlite/postgres/mysql.go；测试三份 —— `store_test.go`（round-trip）、`conformance_test.go`（与 mongostore 同一份行为套件）、`dialect_parity_test.go`（三方言 DDL 静态比对）。
- `mongostore/`：每实体一 repo 文件，store.go 有 index specs；`bson_test.go` 覆盖序列化往返（不需要真机），`conformance_test.go` **跑真机 Mongo 的行为一致性套件**（`MONGO_TEST_DSN` 未设才 skip，`make test-mongo`；CI 的 backend job 带 mongo:8 service）。详见 ARCHITECTURE.md「行为一致性套件」。

## 新增实体的完整路径

domain 加 struct → repository.go 加接口 + Store 组合 → sqlstore 加文件 + 三方言 DDL → mongostore 加 repo 文件 + accessor → （如需）index specs。

⚠️ `domain.Store` 组合接口是唯一强制两个 store 同步的东西：漏一个 accessor 是编译错误，不是运行时错误。加接口时先加进 `Store`，让编译器把两边都逼出来。三方言 DDL 写完跑 `go test ./internal/storage/sqlstore/` —— `dialect_parity_test.go` 会静态比对三份 schema，见下。

## 给已有表加列（ColumnMigration 机制）

1. `sqlstore/dialect.go`：`ColumnMigration{Table, Column, DDL}`；共享清单函数 `sessionColumnMigrations(textType)`——名字带 session 但**实际覆盖六张表：sessions / memory_facts / users / chat_messages / operation_logs（`domain`）/ mood_checkins（`source`+`note`）**，直接往里 append 即可，三方言自动获得（只有类型 token 不同：sqlite/pg "TEXT"，mysql "VARCHAR(64)"）。
2. 执行：`store.go` Migrate 先跑建表 `Migrations()`，再对每个 ColumnMigration 用 `ColumnExistsQuery()` 查列（sqlite pragma_table_info / pg+mysql information_schema），`sql.ErrNoRows` 才执行 DDL（ALTER ADD COLUMN 非幂等）。
3. 同时改：三方言**建表 DDL** 也要加同列（新库直接建全）；sqlstore 实体文件的 SELECT/INSERT/Scan；mongostore doc struct（bson tag，mongo 无需迁移）。
4. domain struct 加字段。

## 三方言对等测试（`dialect_parity_test.go`，2026-07-26）

这个测试直接读三个方言 `Migrations()` 的返回值做**静态比对** —— 它检查的是 DDL 字符串自身的性质，不需要跑引擎。它**不能替代真机**：静态比对只能看出三份 DDL 互相不一致，看不出其中任何一份是否合法。

✅ 它诞生时「只有 sqlite 被真机测过」，这个洞放跑过三次真事故（见下）。**2026-07-29 起 pg 与 MySQL 都已真机验证**（`conformance_real_test.go`，本机 PostgreSQL 16.14 + MySQL 8）：两边各 32 张表 DDL 全部合法（三方言表名集合完全一致）、31 例行为套件全过、原生全文索引（tsvector+GIN / FULLTEXT ngram）建得起来且能查。**四个后端至此全部真机过套件。**

| 检查 | 挡住什么 |
|---|---|
| 三方言表集合相同、每表列集合相同 | 某张表只在一个引擎存在 = 另外两个引擎线上炸 |
| 三方言索引集合相同（MySQL 内联 KEY 与 `CREATE INDEX` 归一比较） | 一个引擎少一个索引 = 那个引擎线上全表扫，本地永远看不出来 |
| MySQL 的 TEXT/BLOB 列不带字面 DEFAULT | MySQL 直接拒绝 → 启动失败 |
| `ColumnMigration` 不出现「NOT NULL 且无 DEFAULT」 | SQLite 直接拒绝，MySQL 严格模式也拒绝 |
| `ColumnMigration` 的 Table/Column 全小写 | Postgres 的 `information_schema` 大小写折叠，大写会让守卫永不命中 → ALTER 每次启动重跑 → 第二次启动死于「列已存在」 |
| 每个 `ColumnMigration` 的列必须**也在建表里**，且**类型逐字一致** | 类型不一致时守卫会在新库上跳过 ALTER —— 新库永远保留建表的类型，升级库拿到 ALTER 的类型，同一份代码跑出两种库 |
| 索引名 ≤ 63 字节 | Postgres 静默截断，两个长名字撞成一个，第二条 `CREATE INDEX IF NOT EXISTS` 变成空操作 |
| 仓库 SQL 里 `FROM/INSERT INTO/UPDATE/DELETE` 提到的每张表都有建表语句 | 就是 `feedback_logs` 那一类 |

**失败时改 schema，不要放宽检查** —— 每一条都是因为它禁止的事已经发生过了。

### 它一上来就抓到的既有事故（均已核实、已修）

| 事故 | 后果 |
|---|---|
| `feedback_logs` 有完整 repository，**三方言都没有建表** | 三个引擎上每次写反馈都失败 |
| `sessions.preferences` 的 ALTER 是 `TEXT NOT NULL DEFAULT '{}'`，而 `sessions` 建表里**根本没这列** | MySQL 拒绝 TEXT 的字面 DEFAULT，且因为列不在建表里，这条 ALTER 连全新库都要跑 → **MySQL 永远起不来** |
| `temp_contexts.key` 在 MySQL 建表里加了反引号，**五条 DML 一条都没加** | `KEY` 是 MySQL 保留字，`Rebind` 只改占位符不改标识符 → 每次 temp context 读写都是语法错误（inbox 上传、Exchange 快照全废） |
| `persona_prompt` 走 `textType` = MySQL `VARCHAR(64)`，而 handler 允许 2000 字 | MySQL 上任何像样的人设提示词都是 data-too-long |
| MySQL 建表宽度与 ALTER 宽度不一致三处（`language` 16/64、`operation_logs.domain` 16/64、`mood_checkins.source` 32/64） | 新库与升级库永久分叉 |
| `assignments.html_url` 在 MySQL 可空、另两个方言 NOT NULL，读侧却扫进 plain string | NULL 一旦出现就是 `converting NULL to string is unsupported` |

修法确立了两条规则，新表一律照办：

1. **要 `NOT NULL DEFAULT ''` 的列在 MySQL 上必须是 `VARCHAR(n)`**；凡是 `TEXT`/`LONGTEXT`，**三方言一律可空**、读侧 `COALESCE` —— 不允许「一个方言可空另一个不可空」。
2. **新表不要用需要引号的列名**。`Dialect.Quote(ident)` 是为已经犯了这个错的那一张表加的，不是给新表用的许可。

## ConditionalMigration

见上方「Material 搜索」一节——机制与 FTS 一起落地（2026-07-15）。新的可选特性 DDL 一律走它，不要放 Migrations()（那里失败会阻断启动）。

## Material 搜索（原生 FTS + 子串兜底，2026-07-15 落地）

- `search/material.go` MaterialSearcher：先探 `store.(domain.MaterialFTS)` → 原生索引带真实打分；ok=false/err（索引缺失、sqlite 查询词<3 rune、mongo 无 CJK 分词命中为空）→ 回退 `Materials().List` 子串扫描（Score=1，召回下限保证）。Index/Deindex 维持 no-op（四引擎全自动维护）。
- 索引实现：SQLite FTS5 external-content 虚表 + 三触发器 + 建表时 rebuild 回填（`dialect_sqlite.go`，modernc.org/sqlite v1.34.4 验证带 FTS5+trigram）；PG tsvector 生成列('simple') + GIN（需 PG≥12）；MySQL FULLTEXT ngram；Mongo text index（store.go specs）。查询在 `sqlstore/fts.go` / `mongostore/fts.go`。
- **ConditionalMigration 机制**（`dialect.go` + `store.go`）：`{Name, CheckQuery, DDLs}`，CheckQuery 无行才执行；失败只进 `Store.warnings`（main.go 启动打日志）不阻断启动；成败记录在 `Store.condApplied[name]`，FTS 查询据此短路。✅ **sqlite / PostgreSQL / MySQL 三个 SQL 引擎都已真机验证**（`TestNativeFTSBuildsOnRealEngines`：不只断言 `condApplied` 为真，还真发一次查询 —— 索引建起来不等于查询语法对，而查询语法错只在有人搜索时才报）。

⚠️ 这个「失败只警告」的设计有个代价：索引没建起来 → `SearchMaterialsFTS` 按 `condApplied` 短路 → **永远静默跑子串兜底，而且没人会发现**（子串搜索也返回结果）。所以那个测试盯的正是这条。

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

## 多语言机制（`i18n.Catalog` 三层 + 用户自选一主一副，2026-07-26）

### 语言包三层：数据库 → 本地文件 → 内嵌

```
db        控制台改，多实例共享         ← 优先
files     LOCALES_DIR/<locale>.json
embedded  Go 字面量，只有 zh-CN / en-US  ← 兜底地板
```

**内嵌的两种是地板不是全集**。它们只需覆盖「没有数据库也没有文件时仍能把页面画出来并说清为什么」。加一门语言是丢一个 `ja-JP.json` 进 `LOCALES_DIR`（或从控制台粘一份），**不是一次发版**。形状照抄 `prompt_overrides` —— 文件作种子、DB 覆盖、立即生效，这个仓库唯一把配置做对的地方。

⚠️ **DB 那层 2026-07-29 才真正接上。** `domain.LocaleRepository` 在两个存储都实现了、`storagetest` 也测了，`i18n.Catalog` 也一直留着 `db` 那一层 —— 但**没有任何代码把两者连起来**，所以在那之前跑着的服务器其实只有文件层与内嵌层，上面这段描述有一层是假的。接线是 `server.ReloadLocaleOverrides`（启动时一次，控制台改完再一次），`internal/server/locales_test.go` 锁住它：覆盖生效、只盖被指定的那个 locale、DB 里加一门新语言会出现在 `Available()` 里、卸载又消失。

**仓库测过 repository 却没测接线，这正是它能无声断掉的原因** —— repository 从来不是坏掉的那一半。同一天在提示词那边发现同型问题：`NewPromptServiceDisk` 无人调用、`config.Load` 不读 `PROMPTS_DIR`，而 `daycore install` 却把模板解出来并把这个键写进 `.env`（见 AI.md）。

```go
type Text map[string]string                        // 一条消息的各语言版本
func Register(key string, t Text)                  // 包 init 里注册内嵌兜底
func Reg(key string, t Text) string                // 注册并返回 key，一行声明
func T(key, locale string) string                  // 解析
func Tf(key, locale string, args ...any) string    // 带 %verb 的
func (c *Catalog) LoadDir(dir string) error        // 文件层，可重载
func (c *Catalog) SetOverrides(map[string]Text)    // DB 层，store 喂进来
func Available() []string                          // 装了哪些语言
func (c *Catalog) Coverage(locale) (have, total int)
func (c *Catalog) Export(locale) map[string]string // 翻译起点：导出→翻→存成 <locale>.json
```

⚠️ **解析按 locale 逐层问，不是按层逐 locale**。回退链里每一个候选语言都先问 db、再问 files、最后问 embedded。反过来做（先挑「哪一层认识这个 key」，再在那层里跑回退链）会让 DB 里一条半成品的法语覆盖，盖住下面那条完整的英文。

**回退链四步**（`Pick`）：精确 tag → 同语言其他地区（`zh-TW` 先找到 `zh-CN`，不掉英文）→ `Default`（en-US）→ 表里任意一条。最后一步是**半翻译的表宁可显示看不懂的语言也不显示空白**：缺翻译是内容缺口，不是渲染失败。取值按 `Embedded` 顺序，不吃 map 迭代随机性。

- **未注册的 key 原样返回 key 本身**，不返回空串 —— 空按钮看起来像布局 bug，会把人引到错的地方找；编出来的文案更糟。
- **同一 key 注册两次 panic** —— 两个包抢一个 key，文案会随链接顺序变，从「文字不对」这种 bug 报告几乎不可能查回来。
- `Missing(t)` 只检查 `Embedded`：文件装进来的语言**允许不全**（`Coverage` 报告缺多少），回退链兜着；内嵌那两种是回退链要落到的地方，必须齐。
- **不要直接 `i18n.Pick(someText, locale)`** —— 那会绕开 db/files 两层，让这条字符串悄悄变成不可翻译的。一律走 `T`/`Tf`。（`PickFrom` 泛型版已删除，正是因为它鼓励这种绕行。）

### 用户自选一主一副（不是部署定死）

三层状态别混：

| 层 | 存哪 | 是什么 | DDL |
|---|---|---|---|
| 装了哪些语言 | `i18n.Available()` | 这个安装能渲染什么 | — |
| 我的一主一副 | `SessionPrefs.PrimaryLocale` / `SecondaryLocale` | 我的开关在哪两种之间切 | **零**（`preferences` JSON） |
| 我现在读的是哪一种 | `sessions.language` 列（既有） | 首页开关翻的就是它 | — |

第三层是**状态**不是偏好，所以留在原来的列上。部署只给默认值（`DEFAULT_PRIMARY_LOCALE` / `DEFAULT_SECONDARY_LOCALE`，`config.Load()` 里 `i18n.NewPair` 校验，值不对启动失败），**不限制用户能选什么**。

- `PairOr(userP, userS, deploymentDefault)` 是读路径：大多数人从没打开过语言设置，只设了主语言的人不该因此丢掉开关。
- **读时钳制、写时拒绝**：换配对后 `sessions.language` 可能落在配对外 —— 读（`Pair.Resolve`）静默落回主语言，设置页照常打得开；写返回 `400 unsupported_locale`。改配对时后端顺手把 `language` 拉回配对内。
- 副语言为空 = 单语言用户，`List()` 只有一项，**前端隐藏开关而不是禁用**。
- `NewPair` 拒绝主副相同：在自己和自己之间切换的按钮是个什么都不做的控件。

### 已进目录的 key（全部 `json:"-"`，**API 形状不变**）

| 前缀 | 内容 | 位置 |
|---|---|---|
| `mood.<id>` | 12 个心情标签 | `domain/mood_kind.go` |
| `category.<id>` / `.hint` | 10 个资料类别名 + 给 inbox 分类器的提示行 | `domain/material_category.go` |
| `lock.reason.<level>` | 派生锁原因，与 `api/lock-rules.json` 对齐 | `domain/lock.go` |
| `weather.precip` | `"降水%d%%"` / `"precip %d%%"`，整段含空格进表 | `domain/weather.go` |
| `weather.wmo.<code>` / `.unknown` | 28 个 WMO 天气码 | `weather/openmeteo` |
| `date.*` | 星期全名/短名各 7 条、明天后天大后天、本周下周、表格行格式、相对短语连接格式 | `ai/datectx.go` |
| `worker.deadline.*` | 8 条，逾期提醒逐句拆开 | `server/worker.go` |
| `worker.gapSuggestion` / `morningUser` / `eveningUser` / `replanUser` | 四条定时任务文案 | `server/worker.go` |
| `agent.wrapUpNudge` · `companion.personaHeading` / `wishPoolHeading` / `asyncError` · `inbox.categoryLine` | 单句 | 各自文件 |

⚠️ 带 `%` 动词的条目（`weather.precip`、`inbox.categoryLine`、`date.row`/`date.rel`、`worker.replanUser`、`worker.deadline.*`）：新语言必须保留同样的动词、同样的顺序。`TestPrecipFormatKeepsItsVerb` 挡住漏掉 `%d` 的翻译。

**逾期提醒为什么拆成 8 条而不是一条格式串**：两种语言组句方式不同 —— 中文用「，」和「：」把子句连起来，英文要 `and` 和冒号。一种语言一条格式串表达不了两者，总有一边读着像翻译腔。拆开也让译者改 JSON 而不是改 Go。⚠️ 两种语言都没做复数：中文没有复数，英文写的是 `assignment(s)`。这是既有文案原样保留；真有复数规则的语言需要的不止一条格式串，这张表就是那时候要改的地方。

**日期短星期为什么存成 key 而不是一个 `shortFn`**：中文要去掉「星期」前缀（下周三），英文没什么可去。存成 `date.weekdayShort.<0-6>` 让译者加语言时不必写 Go。

**顺带修掉的既有缺陷**：`datectx` 的相对日期表在 en-US 下渲染成 `| ThisMonday | … |`（中文 `本周`+`三` 连写，英文照抄丢了空格）。现在 `date.rel` 分 locale 给，`"%s%s"` / `"%s %s"`。

**没有改、也不该改成 key 的**：`ai/prompts.go` 的 `DefaultPersona`/`HardBoundaryReminder`、`worker.go` 的 `buildBriefSystemPrompt`/`buildReplanSystemPrompt`。这四段是多段落提示词，正确去处是 `prompts/<locale>/*.tmpl` —— 已有机制、已带启动期双 locale 硬校验、落 `prompt_overrides` 后控制台可改。属批次 5 的「提示词一次性收口」。

**外部 provider 的语言参数是另一回事**（`internal/weather/lang.go` 的 `Lang(locale, codes, fallback)`）：每个上游有自己的代码空间（QWeather `zh`/`zh-hant`、OWM `zh_cn`/`zh_tw`、wttr.in 只有一种中文），表跟着 provider 走，只共享查表逻辑。上游没有的语言退回它自己的默认值 —— 语言不对的天气预报仍然告诉你会下雨。

### 还没做的

- ~~**DB 覆盖层的表**~~ **已完成**：`locale_overrides` 与批次 C 的五张表同批建好，接线（`server.ReloadLocaleOverrides`，启动时调一次）也在 2026-07-29 补上了。**三层今天是真的三层。**
- **控制台的语言包分区**（列出已装语言 + 覆盖率、导出、粘贴导入、重载 `LOCALES_DIR`）：批次 F5，`Coverage`/`Export`/`Keys` 都已备好。

## 多实例与派生缓存的六张表（批次 C，2026-07-27）

一次性给三方言加完，因为分两次改 `dialect_*.go` 是计划明令避免的事，而作者不在本机跑 pg/mysql/mongo。

| 表 | 作用 | 现在接了吗 |
|---|---|---|
| `proposals` | 提案统一资源持久化 —— 取代 `agent.go` 的进程内 `map`，那是水平扩展的直接阻碍 | **未接线**，批次 D |
| `leases` | 选主，只让一个实例跑后台任务 | **未接线**，批次 5 |
| `job_runs` | 每个任务「场次」的占有与审计 | **未接线**，批次 5 |
| `rapport_states` | 默契评分缓存 + 账本游标 | **未接线**，批次 D |
| `rhythm_profiles` / `rhythm_days` | 节律画像 + 每日首尾 | **未接线**，批次 5 |
| `locale_overrides` | 消息目录的 DB 层 | **已接线**（`server.ReloadLocaleOverrides`，2026-07-29）；控制台的编辑端点还没有 → 批次 F |

**六张表今天全部是死重量** —— 建了、能 round-trip、有测试，但没有任何调用方。别以为提案已经在落库了。

### 两条贯穿性设计

**`job_runs` 是正确性机制，`leases` 只是节流。** 唯一索引 `(session_id, job_name, run_key)` 是四个后端唯一共有的互斥手段（sqlstore 全包无事务），所以**先写行再干活**：INSERT 成功即占有，撞唯一键即别人已占。反过来「干完再记」会留下这张表本来要关掉的窗口。lease 只是省掉「N 个实例各自醒来、建上下文、然后 N−1 个白干」。**正确性不能压在 lease 上，因为 lease 压在时钟上，而不同机器的时钟不一致。**

- lease 的 `fence` 只在**交接**时 +1，续期不动。停顿过久的持有者靠比对 fence 就能发现自己已经不是 leader —— 这是时间戳给不了的，因为它自己的时钟正是不能信的那个东西。
- **接管时轮换行的 id**。原持有者要是终于醒过来调 `Finish`，它手上的 id 已经匹配不到任何行，那次迟到的写入变成无害空操作，而不是对新持有者那次运行的判决。Mongo 侧因为 `_id` 不可变，用「删旧 + 插新」复现同一语义。
- `failed` 可重试，上限 `JobMaxAttempts=3`：不重试则一次早报失败就赔掉一整天；无上限则一个必然失败的任务（密钥被吊销）每 tick 刷一条。
- **被抑制的任务（关了开关 / 免打扰）不写行** —— 每半小时记一条「什么都没做」会把真正有信息的行埋掉。
- **停滞接管也有上限**（`JobMaxCrashAttempts=6`，比 `JobMaxAttempts=3` 大）：返回错误的任务走前者，**杀死自己实例的任务**（大上下文 OOM）什么都不返回、行停在 running、十分钟后被下一个实例接管、再 OOM 一次 —— 没有上限这个循环永不终止，而 attempts 列就在那里记着它已经发生七次了却没人读。
- **接管成功后要把行里的 attempts 读回调用方的结构体**。留在 1 会让「这是不是最后一次尝试」永远答错，而 Mongo 侧返回的是真值 —— 两个后端对同一次调用给出不同答案比任何一个答案都糟。
- ⚠️ **`started_at` 由占有方自己的时钟写，超时判定由读方的时钟做。** 跨机时钟偏差必须小于 `JobStaleAfter`（10 分钟），否则慢钟实例能偷走一个几毫秒前的占有、把同一场次跑两遍。**`Claim` 只应由 lease 持有者调用** —— lease 是「两个实例不会同时走到这里」的保证，但主机之间仍必须 NTP 同步在 `JobStaleAfter` 之内。这是部署要求，不是代码能修的。
- **`Acquire` 的后像不能无条件返回。** 它用第二条语句读回行，而在这个间隙里，一个跑快的时钟能把 lease 抢走 —— 不加校验就会把「写着别人是持有者、带着别人 fence」的那一行交给我们，于是我们把别人的 fence 记成自己的、断定什么都没变。**那正是 fence 存在要抓的唯一那种失败**，不能在这里被打败。`confirm()` 校验行是否仍然指着我们。
- **`Acquire` 的首次 INSERT 失败不能一律吞掉。** 「别人先插进去了」与「数据库连不上」在这里长得一样，全吞成落选会让一次故障看起来像一次普通交接：worker 静默变闲，任何地方都没有一句话说明原因。改成失败后读回行 —— 行在了就是真落选，读也失败就把错误抛出去。
- **Mongo 侧 `_id` 是「场次」而不是「占有」**（`session:job:runKey`），另有一个轮换的 `claim_id`，`Finish` 匹配后者。SQL 在一条 UPDATE 里轮换主键，Mongo 的 `_id` 不可变 —— 而「删了再插」不是同一回事：两条语句之间那个场次**根本不存在**，此时 ctx 被取消就永久毁掉了此前的尝试记录，包括 `Prune` 特意保留的那条「崩溃后再没回来」的 running 行。现在是一条原子 `FindOneAndUpdate`。

**UPDATE-then-INSERT 的 upsert 必须处理 INSERT 输掉。** 并发首写时两边 UPDATE 都影响 0 行、都 INSERT、一个撞唯一键。此时**重试 UPDATE**：赢家的行已经在了，我们的值照样写进去，那个约束错误本来就不该给调用方看见。`leaseRepo.Acquire` 与 `jobRunRepo.Claim` 一直是这么做的，另外三处（rapport / rhythm_days / locale_overrides）漏了。

### 一次对抗式审查抓到的（全部已修，每条都留了回归测试）

| 缺陷 | 后果 |
|---|---|
| MySQL 三张新表内联 `KEY` 前缺逗号、末尾多逗号 | 每张两个 1064 语法错误，**MySQL 起不来**。对等测试当时看不见 —— `reInlineKey` 逐行匹配，不看邻行标点。已补 `TestCreateTableCommasAreWellFormed` |
| `Supersede` 退休「不是 keepID 的全部」 | 两个 daemon 各建一张同 mergeKey 的卡、各自 supersede 对方 → **存活 0 张**，用户什么都看不到。改成按 `created_at` 比较，谁先调用结果都一样 |
| `Validate()` 不要求 `ExpiresAt` | 无死线的卡**出生即过期**：第一次清扫看到零值时间，判定已过期、按沉默结案 |
| `rapportRepo.Get` 丢弃坏缓存后又把游标写回 | 之后**永久跳过**游标之前的全部账本，重建出的读数一直偏低且不会自愈 |
| `Proposal.Update` 两侧字段清单不一致 | SQL 不写 level/kind/origin/threadID，Mongo 的 `ReplaceOne` 会写。改成同一份清单的 `$set` —— 那五个字段是身份，要改就是新卡 |
| Mongo 接管不轮换 id | 僵尸实例的 `Finish` 会把新持有者那次运行标成 failed |
| MySQL 的 `RowsAffected` 是 CHANGED 行 | 23 处调用点依赖它，其中三处把 0 变成 `ErrNotFound` —— **原样保存主题/规则/愿望在 MySQL 上返回 404**；upsert 写同样内容会掉进 INSERT 然后撞自己的唯一键。`mysqlDialect.NormalizeDSN` 强制 `clientFoundRows=true` |
| SQLite 缺 `busy_timeout` 就立刻 `SQLITE_BUSY` | 两个请求一重叠，`Observe`（每个 awake 信号都跑）就开始失败。`sqliteDialect.NormalizeDSN` 强制 `busy_timeout(5000)` + `journal_mode(WAL)` |
| Postgres 的 `IF NOT EXISTS` **不是并发原语** | 两个实例同时启动会互相打断迁移（`pg_type` 唯一键冲突）—— 而「两个实例」正是这六张表存在的理由。`postgresDialect.MigrationLock()` 给出 `pg_advisory_lock`，`Migrate` 在**单独一条连接**上持有它（在池化的 `*sql.DB` 上取会话级锁，等于锁在一条没人用的连接上） |

**`NormalizeDSN` 这条模式值得单说**：代码依赖的连接参数不要写进文档等运维抄全 —— DSN 是运维给的，从旧 README 复制一份就静默失去了。方言自己补。

### 其余设计要点

- `proposals` 的 `rows_json`/`ops_json`/`applied_op_ids`/`accept_op_ids` 都是 JSON 列不建子表，`rev` 做乐观锁。行级接受要同时改「这一行的状态」与「父卡是否全部落定」，无事务下 JSON 列 + CAS 是一次单行 UPDATE。
- **`Supersede` 的顺序必须是全序**。按 `created_at` 严格比较不够：时间戳是毫秒精度，两个 daemon 响应同一个触发落在同一毫秒里是常态，此时谁都不比谁旧，于是**一张都不退休、两张都投递** —— 与互相消灭同一个失败的另一面。并列时按 id 破 —— 任意，但从任一调用方看都一样。
- **keeper 不存在时两侧都返回 `(0, nil)`**：没有 keeper 就是没有东西可合并，与 mergeKey 为空是同一个非事件。一侧返回 `ErrNotFound` 会让「遇到 ErrNotFound 就中止本轮投递」的调用方在一个后端中止、在另一个继续。
- `ProposalFilter.DeliverableAt` 让「现在可投递」进 WHERE。**`Undelivered` 单独用不是待发池**：它会带上池子里已过期的、和压后再投的，两者用户都不该看见；取回来再在 Go 里筛会让 `LIMIT` 先于筛选生效，一池过期卡能让本该有内容的一页返回空。
- `rhythm_days` **一人一天一行，不是一个信号一行**。心跳每分钟一次就是每人每天约 1400 行，而 `Learn` 只读每天的首尾；`CurrentRun` 只要「这段连续清醒从何时开始」，那是 `rhythm_profiles` 上两列的 O(1) 维护。`internal/rhythm/incremental.go` 的 `Day`/`Live` 是这一对，与原始信号版本共用同一个核（`LearnDays`），有测试逐步比对两者。
- `rapport_states` 的 `scores_json` 用 JSON 不用四对具名列：域列表今天是闭的，但一域一列会让加一个域变成三方言迁移，而且没有任何跨会话查询能从「可查询」里得到好处。
- `rapport.NewFolderFrom(scores, resolve)` 的 `resolve` 不是可选的 —— 没有它，增量追赶会跳过「原 op 在游标之前」的 revert，缓存与重放分叉，而缓存唯一的存在理由就是它能靠重放重建。存储层用 `OperationLogRepository.Get` 填它。

## 心情窗口（`internal/mood/`，体验内核 §12.6，2026-07-26）

**一次打卡不是心情，一段走向才是。** 「今天焦虑」和「连着四天越来越焦虑」是两件事，只有后者该改变系统行为；三周前的一次低落今天不该有任何权重。所以**任何要用心情做判断的地方读的都不是最近一条 `MoodCheckin`，而是 `mood.Read()` 派生出的窗口**。

与石化、默契同属**读时派生**：不建趋势表、不跑定时任务，参数错了下次读就自愈。

```go
type Window struct {
    Known     bool          // 有没有足够近的信号说得出话
    Score     float64       // 衰减加权均值 valence，−2..+2
    Trend     Trend         // improving / worsening / flat / unknown
    Staleness time.Duration // 距上次打卡多久 —— 这本身就是信息
    Stale     bool          // 超过阈值：知道，但不能当今天的底色
    LastKind  string; LastAt time.Time
    Samples   int; Weight float64
}
func (w Window) Speakable() bool   // Known && !Stale —— 提示词分支就看它
func (w Window) Tone() Tone        // neutral / gentle / bright
func (w Window) Restrained() bool  // auto-plan 该不该排少一点
```

**衰减 vs 新鲜度是两件事，别合并**：

| | 回答什么 | 默认 |
|---|---|---|
| 衰减（`HalfLife`） | 旧数据算几分 | 半衰期 72h，**指数衰减、永不归零** |
| 新鲜度（`StaleAfter`） | 还能不能开口谈心情 | 72h |

**不设硬性截断**是刻意的：「只看 7 天」会让持续的低落在第 8 天凭空消失 —— 而那正是它最要紧的时候。三周前的一条低落衰减到 7 个半衰期后 ≈0.008，实际归零但没有断崖。

`Known` 与 `Stale` **可以同时为真**：有真实历史，但太旧、不能替今天说话。此时 agent 知道「上周不太好」，但不该表现得那就是今天早上 —— 那是可以问的事，不是可以假设的事。

- **未注册的 mood id 跳过而不是当中性**：不在注册表里的心情没有 valence，记成 0 会把每个均值都往中间拖。
- **趋势在两段之内取无权均值**：问题是「那时候比现在差吗」，在旧的那一段里套衰减曲线回答的是另一个问题。两段各至少一条才给方向，否则 `unknown`（「说不准」和「没在动」是不同的答案）。
- **代打卡权重更低**（`AgentWeight` 0.6）：`source=agent` 是从用户说的话里推断出来的，`source=user` 是他自己按的按钮。都算数，不等重（§12.1）。
- **`POST /api/mood` 不接受 body 里的 source**，一律记 `user`。让客户端自己挑，就等于让它写出服务端会悄悄打折的打卡 —— 或者更糟，让一个前端 bug 把真实打卡重标成推断。agent 代打卡时写 `agent`。

⚠️ **2026-07-30 修掉的一条数据正确性 bug**：现役前端存的是 `emoji + " " + 本地化名`（`"😊 开心"`），而 `mood.Read` 用 `MoodKindByID` 解析、解不出就跳过 —— 于是**每一条打卡对心情窗口都不存在**，`moodHistoryContext` 永远回 `{"known":false,"why":"no check-ins"}` 注进 companion 提示词。看起来一切正常：行存下了、心情页的 AI 也回应了，唯一的症状是助手从不提起你最近怎么样。

而且存的是本地化字符串，所以同一个心情中英用户存的是不同的值 —— 正是「存 id 不存标签」这条规则要防的。

两边的词表当时已经是**两套不同的集合**（`great` vs `happy`、`grateful` vs `loved`、`sad` vs `down`、`angry` vs `irritable`、`sick` vs `unwell`；前端独有 `bored`/`lonely`，后端独有 `neutral`/`sleepless`）—— 这是计划里待决问题 ⑦ 一直没拍板的后果。**以后端注册表为准**（它有 `Valence` 与双 locale `Names`），前端改成从 `GET /api/mood/kinds` 取。

写侧现在**拒绝**未知 id（400 `unknown_mood`）：解析不出的值没有 valence，而没有 valence 的打卡不是弱信号，是没有信号。三条回归测试端到端锁住这个接缝 —— 单测两侧都会通过（仓库存了给它的东西，窗口正确地跳过了它读不懂的东西），错的只有中间那一道缝。

**agent 代打卡已落地**（β0+）：`mood_record` 工具写 `Source=agent`，且当天已有用户手动打卡时不新建（EXPERIENCE_CORE §12.1 三边界）。撤销走 `mood_record` 的注册 revert（`MoodRepository.Delete`）。

**设计上用得到的地方都要走同一个窗口**（`s.moodWindow(ctx, sid)`）：companion 上下文注入 · 默契的语气档位与主动性门槛 · Protector 的 20h 关怀措辞 · 晨卡与晚复盘 · 提案卡语气 · auto-plan 强度。六处各算各的迟早会分叉，用户会遇到一个「同一周里这里温柔那里干脆」的系统。

⚠️ **六处里现在只接了一处**（2026-07-29 核实）：`s.moodWindow` 全仓唯一的生产调用点在 `moodHistoryContext` 里 —— 也就是 companion 上下文注入。`internal/mood` 包在 `internal/server` 里除那一个文件之外零引用。上面那句是**设计意图**，其中默契与提案卡连表都还没接线。别照它去找已经存在的代码。

**注入的是窗口不是最近五条**。原来的 `moodHistoryContext` 吐 `[{mood,at}×5]`，这会诱导模型把最上面那条当成今天的心情 —— 三周前一个糟糕的周二就这样染上了一个周四。现在吐的是 `{known, trend, tone, speakable, lastKind, daysSince, samples}`，`Stale` 时额外带一句 `"too old to assume; ask rather than presume"`：这条规则要用话说出来，不能指望模型从一个日期数字里推出来。

⚠️ **一条边界**：心情窗口是**语气与节奏**的输入，**不是内容与拒绝**的输入。锁定块的 409 不查心情 —— 拒绝要即时、确定、可测；一个拖不动的块，用户需要立刻知道为什么，而不是等系统先想想他今天心情如何。

`Restrained()` 比 `Tone()==gentle` **更窄**（要求低分**且**方向不向上）：说话轻一点便宜且低风险，悄悄给人少排一点事却是在替他决定他的时间。

### `MoodCheckin` 新增两列（`source` / `note`）

走 `ColumnMigration`（`sessionColumnMigrations` 里追加两条），三方言建表 DDL 同步加。

- `source`：`user` / `agent`。存量行 `''`，**读作 user** —— 这列存在之前 agent 根本记不了打卡，所以那些就是用户自己记的。
- `note`：**MySQL 的 TEXT 不允许字面 DEFAULT**，三方言共用一份迁移清单，所以 ALTER 与建表都不带 `NOT NULL DEFAULT ''`，读时把 NULL 归一成 `""`。
- 回归测试：`sqlstore/store_test.go` 的 `TestMoodSourceAndNoteRoundTrip`（含「没写 source 的行读回来是空」）。

## 主题与前端握手（路线已定，落地在批次 F）

**决定与理由在 `docs/EXPERIENCE_CORE.md` §1.2**，这里只记数据层要动什么。

现状写死给单个前端：`themeVarWhitelist`（`handlers_themes.go:16`，12 条硬编码）、`themeColorRe`（强制值必须是颜色字面量）、`builtinThemePresets`、`theme_gen.tmpl` 里那套设计规则。琉璃要模糊半径、纸屿要纸纹颗粒、汀要留白刻度 —— 相当一部分不是颜色，现有正则会直接拒掉。

**存储形状不用改**：`custom_themes.variables` 已经是 JSON 列，四引擎都存得下自由文档。Mongo 更自然但**不能成为要求**。要搬走的是校验规则。

**两层身份**：`build_hash`（一次构建的指纹，派生不可改）+ `family_id`（主题兼容组，build 声明、**运维可在控制台改**）。主题与按端偏好按 **family** 存 —— 那才是一套主题有意义的单位。

**family 的 token 空间是并集，不做子集拦截**：新 build 带来新 token 就扩一条，已存主题因此缺那一条 → 运维触发一次**补算**（每个缺 token 的主题一次 AI 调用），补完若两套变量完全一致就**合并**（保留一条，会话偏好改指）。build 用不到的 token 由**前端自己丢弃**，后端不裁剪。

| 改动 | DDL |
|---|---|
| `frontend_builds`（`build_hash` 主键 + `family_id` + `manifest_json` + `rules_approved` + 首见/末见） | 新表 |
| `frontend_families`（`family_id` 主键 + `tokens_json` 并集 + `rules` 运维版本 + `display_name`） | 新表 |
| `custom_themes.family_id` · `theme_switch_log.family_id` | 三方言加列 |
| 按端当前主题 → `SessionPrefs` 的 `{familyID: themeID}` | **零** |
| 主题补算 | 走 `job_runs`，运维触发 |

⚠️ **加列与两张新表一起排在批次 F**，不要单独去动方言文件 —— 这一天已经碰过三次了。

**两条安全边界**（前端是开放的，第三方在场，上报内容是第三方数据）：

1. **`kind` 是服务端已知的封闭集合**（color / length / number / ratio / duration / enum），校验器归后端，**前端不能自带正则、没有 `raw` 档**。加一种 kind 是改后端 —— 这个不便利是把值层面注入面钉在后端的那颗钉子。AI 生成的主题落地前逐 token 校验 key 与 kind。
2. **`theme.rules` 未经运维批准不进 LLM**。前端可以主张、运维可以改并决定是否采用；没主张或没批准 → 后端按 token 清单机械生成。第三方前端一上来功能完整，代价只是提示词平淡；注入面默认为零。

## 批次 C 的第二轮审查（七路跑完，2026-07-28）

七路对抗式审查全部完成（前两次被 529 与会话额度打断），共 59 条去重后的 finding。**两条 fatal 都是批次 C 自己写出来的**：

| 缺陷 | 后果 |
|---|---|
| `Supersede` 用 `(SELECT … FROM (SELECT …) AS k1)` 引用目标表 | 那是 **MySQL 5.7 前的旧绕法**；MySQL 8 的 `derived_merge` 默认开启会把派生表合并回去、重新暴露目标表 → **每次调用 error 1093**。改成先单独读一次 keeper 的 `created_at`（它不可变，没有丢失更新的窗口） |
| `NormalizeDSN` 在整个 DSN 里找第一个 `?` | MySQL DSN 的参数段从**最后一个 `/` 之后**的第一个 `?` 开始，而密码里可以有 `?`（驱动 README 明说不用转义）→ 参数被拼到**数据库名**上 → `Unknown database 'daycore&clientfoundrows=true'`，**起不来**。改用驱动自己的 `ParseDSN`/`FormatDSN`，按构造正确 |

其余已修的（每条都有回归测试）：

- **`itoa` 的 8 字节缓冲从高位静默截断**：`itoa(100000000)` = `"00000000"` → `LIMIT 0` → **一行不返回**。既有 bug（import history 的 limit），批次 C 又复制到三个新调用点。换成 `limitClause(limit, def, max)`：`strconv.Itoa` + 上限（无上限的 LIMIT 是让服务端物化整张表的办法）。

  ⚠️ **上限当时只补了一半**（2026-08-02 补齐）。`limitClause` 用在四个 List 上，而那四个**没有一个是 query 参数够得到的**；真正由 `?limit=` 直通的三个（`/api/ops`、`/api/mood`、chat 历史）只有默认值、没有天花板 —— 覆盖面正好是反的，任何登录用户都能让服务端物化整张表。同时 `ListImports` 的默认值 sqlstore 是 20、mongostore 是 50，**同一个调用在两种部署上返回不同长度的历史**。

  现在这对数字定义在 `internal/domain/listlimit.go`（`ListLimit(limit, def, max)` + 每个 List 的 `XxxListDefault`/`XxxListMax`），两个 store 都读它，漂移在编译期就不可能。行为套件 `List/LimitDefaultAndCeilingAgree` 播超过天花板的行数再断言 —— 播不够的话「不超过 500」这个断言无论有没有天花板都成立，抓不到任何东西。
- **`proposalCols` 只 COALESCE 了 JSON 列**：title/summary/reason/evidence/lock_reason 是 TEXT（本 schema 要求三方言一律可空），却直接扫进 `string` —— 一个 NULL 不止毁那张卡，**整个会话的 List 都炸**。
- **`Update` 不调 `Validate`**：一次字段不全的 Update 就能把 `ttl_policy` 清空，而 `ttl_policy=""` 两个清扫分支都不匹配 → **永远 pending**，投递查询也永远返回不了它。两侧都补上。
- **`marshalJSON` 失败写字面 `"null"`**：ops 里有 NaN 就够 —— 卡入库时声称有活要干、实际一件都没有，用户点接受什么也不会发生。新增 `marshalStrict`，proposals 的 rows/ops 走它，失败就拒绝写入。
- **`Claim` 的 ID 是占有令牌不是调用方身份**：复用同一个 struct 会撞主键而不是撞场次索引，于是 `Claim` 对一个没人占的场次报「别人占了」。改成总是新生成。
- **`Claim` 把一切 INSERT 失败都当「别人占了」**：瞬时写失败也是。改成失败后查一次场次行 —— 有行是真落选，没行说明那个错误是真的。
- **`Acquire` 接受空 holder**：两个实例共用 holder 字符串会**都**匹配续期语句、**都**一直返回 true 而 fence 永不动 —— fence 存在要抓的唯一那种失败变成不可检测。唯一性是调用方的契约（每进程一个值、永不可配置），空值是这一层唯一看得见的退化情形，拒掉。
- **`rhythmProfiles` 一行两个写者、节奏差几个数量级**：夜间学习作业 `Get→算→Save` 会把开始前的清醒标记快照写回去，**擦掉中途开始的清醒段** —— 而零值 `RunSince` 读作「睡了」，Protector 就忘了这人已经醒着九小时。拆成 `Save`（只写学到的一半）+ `Touch`（只写两个标记，且只向前）。
- **Mongo 侧五处与 SQL 不一致**：lease 吞掉一切 `InsertOne` 错误且返回 nil、`Update` 把「指向零值时间的非 nil 指针」当真值 `$set`、`localeRepo.Set` 的 upsert 过滤条件不是唯一键（自己和自己竞争）、`Supersede` 的 keeper 缺失语义、以及最要紧的一条：
- **`ProposalOp.Args` 在两库回来的 Go 类型不同** —— SQL 走 JSON 得 `float64`/`[]interface{}`，Mongo 走 BSON 得 `int32`/`int64`/`primitive.A`。执行工具的 `args["minutes"].(float64)` 在 SQL 上成立、在 Mongo 上断言失败。**两库让执行器拿到不同类型比任何一个选择都糟**，所以 Mongo 侧的 `rows_json`/`ops_json` 也改成 JSON 字符串（字段名本来就叫 json），两边同样有损（>2^53 的整数掉精度 —— 工具参数不该带那种数）。
- **对等测试自己的盲点**：它跳过了 `PRIMARY KEY` 行，于是 `rhythm_days` 与 `locale_overrides` 那两个「upsert 唯一依赖的复合主键」在三方言之间**没有任何东西在比**。已补。

`mongostore` 因此有了第一批测试（`bson_test.go`，7 个，不需要真机）—— 随后 `conformance_test.go` 让它第一次被真机测过。

**仍未做的**（审查列出、判断为后续批次）：`ProposalFilter` 表达不了注意力阶梯的两个预算（没有 `level` 谓词、没有 `pushed_at`、没有计数）→ 批次 D 用得到时再加；proposals 没有保留策略（唯一一张没有 `Prune` 的新表）；`RapportState.FoldVersion` 存了但没有任何地方定义「当前版本是几」；`Proposal.OwnerInstance` 写了但不可查（崩溃清扫写不出来）。

## 节律信号记录（`internal/server/awake.go`，2026-07-29）

**学习作业与 Protector 还没写，但记录先接上了**，因为 `MinDays=5` / `WindowDays=21` 意味着学习器落地时需要好几周历史，而**信号不能追认**。记录是 append-only、forward-only、目前无人读，所以提前打开零成本，买到的是唯一必须实时收集的那个输入。

**「醒着」由调用点定义，不由路径名单定义**：挂在 `requireSession` 上，而两个机器直推端点（Canvas、ICS）走的是 `importSession`，入站通道消息根本不经过 HTTP handler。所以「插件在凌晨四点推了一批数据」不能冒充用户醒着。路径白名单能表达同一条规则，然后慢慢漂离它。

- **节流 5 分钟／会话**（进程内）。读请求是常态，每次读都写一行等于把行更新放到热路径上，而日行只记当天第一分钟与最后一分钟 —— 相隔一分钟的两个信号与一个信号无法区分。5 分钟这个分辨率对 `IdleBreak`(3h)/`ProtectAfter`(20h) 有三个数量级余量。
- 节流表有上限（20000），**满了整表丢弃**而不是扫最旧的：它是节流不是状态，冷表的代价是每个活跃会话多一次写；而一个永不生效的上限才是真隐患。
- 写在 `GoTracked` 的 goroutine 上、用 `context.WithoutCancel`：写要活过响应，而中途断开的客户端当时确实是醒着的。
- ⚠️ **时区暂用 `WORKER_DEFAULT_TZ`**，是部署级默认，所以别的时区的用户日界线画错了地方。记在错的时区仍好过不记（日键可从分钟重算，缺的那天不能），但学习器上线前必须修 —— 批次 ζ。

## 撤销注册表（`handlers_ops.go`，批次 0，2026-07-28）

`switch orig.Action` 改成 `revertHandlers` 映射表 + `registerRevert(action, h)`。⚠️ **机制在，搬迁没做**：12 条 `registerRevert` 目前全挤在 `handlers_ops.go` 自己的一个 `init()` 里，没有任何别的文件调用它 —— 照下面那句「住在写这条 op 的代码旁边」去 `handlers_plan.go` / `handlers_rules.go` / `handlers_memory.go` 找逆操作会一无所获。搬迁是批次 D 的事。

**为什么**：switch 是错的形状 —— 每加一个写操作都要回到同一个文件改同一个函数，于是一批本来不相干的并行工作全撞在这里（计划里 `handlers_ops.go` 被 8 个工作项争）。映射表让逆操作住在**写这条 op 的代码旁边**，那也是别人会去找它的地方。

- 注册同一个 action 两次 **panic** —— 两个逆操作意味着有一个是死代码，而谁赢取决于链接顺序。
- 没注册不是静默的缺口：`handleOpRevert` 明确返回 `irreversible` 并说出是哪个 action。铁律 3 说一切可撤，这里就是这句话被守住或被打破的地方。
- `revert` 自己永不可注册为可撤销：撤销一次撤销是一笔新的正向操作，不是回滚（共识 23，账本 append-only，后悔是新的一笔而不是橡皮擦）。
- 测试 `handlers_ops_test.go`：注册表非空、12 个已知写操作都有逆、重复注册 panic。

**同批修掉一个真 bug**：防重复撤销原来是扫最近 200 条 op —— 会话忙到把 revert 挤出第 200 行之后，同一操作可以被撤销两次，而**补偿不是幂等的**（「把块加回去」执行两次就加了两个块）。改成 `OperationLogRepository.RevertedBy(sid, targetID)` 精确查询。

## 操作域（OperationLog.Domain）

`schedule` / `habit` / `archive` / `care` / `system` 五个值，`OpDomainOf(action)` 按 action 前缀派生。两个消费者：河流按域上色、**默契（rapport）按域计分**（前四个域计分，system 不计）。

- 走 `ColumnMigration` 加列（`dialect.go` 的 `sessionColumnMigrations`），三方言建表 DDL 同步加。
- 存量行默认 `''`，**读时用 `OpDomainOf` 派生而不回填** —— 账本 append-only，且派生结果可重放重现。写入时 `Add()` 若未设则自动派生并回写进 struct。
- 未知 action 一律落 `system`，避免新 action 悄悄影响默契评分。
- 测试：`domain/operations_test.go` + `sqlstore/store_test.go` 的 `TestOpLogDomain`（含模拟存量空值行）。

## 其他要点

- TempContext：session 级 KV，TTL 1h（inbox upload 的图片 base64 <1MB 存这里；阶段 3 起归类草稿 `inbox:draft:<id>` 也存这里）。
- Assignment 手动创建（阶段 4）：`CanvasID = "manual:"+uuid` 走现有 `UpsertByCanvasID` INSERT 分支，零新 repo 方法；Canvas 再导入不碰 manual 行。**合成 id 不是可选的**：`courses` 与 `assignments` 都有 `UNIQUE (session_id, canvas_id)`，所以空 canvas id 不是「没有键」，而是所有无键行共用的**同一个键** —— upsert 会找到上一条无键行原地覆盖、连 id 都留着，第一条就此消失且无处报错（2026-08-02 实测确认）。四个后端现在一律返回 `domain.ErrMissingUpsertKey`，行为套件 `Upsert/EmptyCanvasIDIsRefused` 盯着。
- sessions.preferences 列：SessionPrefs JSON 序列化（worker.go），合并端点 GET/PATCH /api/session/preferences——加用户级开关优先用它，零迁移。⚠️ 2026-07-15 修复：此前 sqlstore/mongostore 的 session Get/Update 都没接 preferences 列，偏好从未持久化过（含 worker 的主动性开关）；同类 bug 也出现在 users 的 data_session_id（同日修复）。**新加列后必须同时检查 SELECT/Scan/Update 三处**。
- inbox 草稿：`inbox:draft:<id>`（TempContext，1h TTL）存 AI 归类结果，`POST /api/inbox/commit` 消费；上传文件是 `inbox:<tempID>`。
