# 架构总览

> 实时文档：改代码必须同批更新本目录对应文件。最后全面核对：2026-07-14。

## 包结构（仓库根，Go module `daycore`，go 1.23）

| 包 | 职责 |
|---|---|
| `cmd/daycore/` | 入口 main.go（装配 config/store/catalog/prompts/server/worker/channels）+ install.go（prompt 模板导出器，walk embed FS，新模板自动导出） |
| `internal/domain/` | 纯数据结构 + Repository 接口，零外部依赖 |
| `internal/server/` | HTTP 路由、中间件、全部 handler、agent loop、cron Worker |
| `internal/storage/sqlstore/` | SQL 三方言（SQLite/PostgreSQL/MySQL），每实体一文件 |
| `internal/storage/mongostore/` | MongoDB 实现，每实体一 repo 文件；`bson_test.go` + 真机行为一致性套件（`make test-mongo`，CI 带 mongo:8） |
| `internal/ai/` | AIProvider 抽象、Catalog、PromptService（embed+DB override）、流式协议、vision 管线 |
| `internal/auth/` | 密码(argon2id)/OAuth/JWT(token.go)/签名 cookie(session.go) |
| `internal/channels/` | 通道插件框架（Registry + OneBot 11 适配器） |
| `internal/config/` | 环境变量配置（godotenv，见下） |
| `internal/search/` | web 搜索（Tavily→DDG）+ MaterialSearcher（原生 FTS 优先 + 子串兜底，见 DATA.md） |
| `internal/weather/` | WeatherProvider registry（open-meteo/qweather/owm/wttr.in，30min 缓存） |
| `internal/version/` | 版本唯一真源：Version="2.2.0" Channel="beta"；APIVersion 契约常量（阶段 1 加） |
| `internal/rapport/` `internal/rhythm/` `internal/mood/` | 默契评分与主动性门控 / 节律学习 + 20h 关怀 / 心情窗口（趋势+衰减+新鲜度）—— 三个都是纯函数、零存储、读时派生，见 DATA.md |
| `internal/schedule/` `internal/ics/` `internal/i18n/` | 规则展开引擎 / ICS 解析 / locale 协商 + **三层消息目录**（DB → 文件 → 内嵌 zh-CN·en-US）+ 用户级一主一副 `Pair`，见 DATA.md |

## 推荐 MongoDB，但四个后端都要能跑（2026-07-29）

**MongoDB 是推荐部署**，理由是这个数据模型确实更贴它：主题变量、提案的 rows/ops、rapport 分数、节律日 —— 一半的新实体本来就是自由文档，Mongo 存它们不需要「塞进一个 JSON 列再整体重写」。

**但兼容不是可选的。** 推荐与要求之间那条线要划清楚，否则「支持四个后端」会悄悄变成「只有 Mongo 真的测过」—— 而今天之前 `mongostore` 恰好是那个**零测试**的，方向还是反的。现在两边都跑同一套行为套件（见下）。

### SQL 侧的取舍规则：条件写用列，其余用 JSON

「大键值是个 JSON、每次整体重写」在**大部分**字段上是对的取舍 —— 省掉三方言 DDL、加字段零迁移、与 Mongo 的形状对齐。但它不能一刀切，因为整体重写**放弃了条件写**：

> **凡是出现在 `WHERE` 里、或被算术/`CASE` 更新的字段，必须是列。其余一律可以进 JSON blob。**

批次 C 已经是这么分的，把规则写下来是为了后面不走偏：

| 必须是列（参与条件写） | 可以是 JSON blob |
|---|---|
| `rev`（CAS）、`state`、`expires_at`、`ttl_policy`、`delivered_at`、`deliver_after`、`merge_key`、`created_at` | `rows_json`、`ops_json`、`applied_op_ids`、`accept_op_ids` |
| `first_min`/`last_min`/`signals`（`CASE WHEN` 扩边界） | — |
| `last_signal_at`/`run_since`（`WHERE last_signal_at < ?`） | — |
| `attempts`、`status`、`started_at`（占有与接管判定） | `error_text` |
| `fence`、`holder`、`expires_at`（选主） | — |
| `cursor_created_at`/`cursor_id`（游标续读） | `scores_json` |

把 `first_min` 塞进 blob，扩边界就退成读-改-写 —— 两个标签页开着就丢信号。这不是性能取舍，是正确性取舍。

### 行为一致性套件（`internal/storage/storagetest`，已落地）

一套按 `domain.Store` 写的行为套件，**四个后端都跑同一份**：sqlstore 覆盖 SQLite / PostgreSQL / MySQL（后两个靠 `PG_TEST_DSN`/`MYSQL_TEST_DSN`，`make test-sql`），mongostore 用真机 Mongo（`MONGO_TEST_DSN` 未设则跳过，`make test-mongo`）。42 个用例，全部来自审查与对抗验证抓到的真实分歧 —— 不是「能存能取」，而是：

lease 只有一个持有者且 fence 只在交接时动 / `Acquire` 永不返回别人的行 / 空 holder 被拒 / 场次占有互斥 / 完成的场次不再被占 / 失败重试到上限 / 崩溃接管有界且回报真实 attempts / **接管轮换占有令牌使僵尸的 `Finish` 落空** / `Prune` 保留 running / nil 切片回来是空切片而非 nil / 指向零值时间的指针算「不存在」 / **`ProposalOp.Args` 的数字在每个后端都回来是 `float64`** / `Validate` 在 `Create` 与 `Update` 两侧都生效 / `rev` CAS 拒绝陈旧写 / TTL 不对称 / **同毫秒并列时 Supersede 恰好留一张** / 已投递的卡不被退休 / keeper 缺失是非事件 / 可投递集合排除过期与压后 / 序列化失败拒绝写入 / rapport 游标往返 / **学习作业不擦掉活的清醒标记** / `Touch` 只向前 / 分钟 0 是有意义的值 / 并发首写不丢信号 / 语言包往返与整语言卸载 / `RevertedBy` 精确且不跨会话 / `Scan` 最旧优先且游标续读无重无漏 / **空 canvas id 的 upsert 被拒而不是覆盖上一条无键行** / **每个 List 的默认页大小与天花板四后端一致** / **`Delete` 限定在本会话内、删不存在的行报 `ErrNotFound`** / 提案的 level 谓词与三态戳谓词 / **`Count` 忽略 `Limit`**（否则预算数到一页就饱和）/ **「堆叠里几张卡」是合取不是问戳**（`delivered_at` 永不清除）/ **`Prune` 放过 pending**（老的 pending 是 `Expire` 还没扫到，删它是抹掉用户还欠着的卡）/ `OwnerInstance` 可查。

**加后端的验收标准就是这套套件通过**，包括计划中的 HTTP 转换层。这也是让第五个后端负担得起的唯一办法：两两分歧数随后端数平方增长，共享套件把它压平。

✅ **四个后端全部真机验证完毕**（首次 2026-07-29，最近一次 2026-08-06 —— ε 的 `attachments` 六例在四个后端上逐条跑过）：

| 后端 | 行为套件 | 建表 | 原生全文索引 |
|---|---|---|---|
| SQLite | 42/42 | ✅ | ✅ FTS5 external-content + 三触发器 |
| PostgreSQL 16 | **42/42** | ✅ 33 张表 | ✅ tsvector 生成列 + GIN |
| MySQL 8 | **42/42** | ✅ 33 张表 | ✅ FULLTEXT ngram |
| MongoDB 8 | 42/42 | ✅ | ✅ text index |

**pg 与 MySQL 是第一次真机执行**（此前只有 `dialect_parity_test.go` 的静态比对，而静态比对只能看出三份 DDL 互相不一致、看不出其中任何一份是否合法 —— 这个洞放跑过三次真事故）。原生索引那一列也是第一次验：不只断言 `condApplied` 为真，还真发一次查询，因为索引建起来不等于查询语法对，而查询语法错只在有人搜索时才报。

CI 的 backend job 起 mongo:8 + postgres:16 + mysql:8 三个 service，并有一步**断言它们没有静默 skip**（跳过的套件读起来和通过的一样，而 DSN 环境变量正是那种会悄悄不再被设置的东西）。本机跑：`make test-mongo` + `make test-sql`，各用例自建自删 schema/数据库。

## 存储的第五种后端：HTTP / 子进程转换层

**完整协议在 [`docs/specs/storage-protocol.md`](specs/storage-protocol.md)**，传输规则在 [`specs/transport.md`](specs/transport.md)。这里只记为什么与顺序。

「转换层 + 内部高效适配」这个模式扩到存储层是对的，天气/搜索/通道（F2）已经这么设计。但**最小接口不是基础 CRUD**：

`domain.Store` 有 179 个方法、**20 处条件写**。它们不是优化，是这个仓库唯一的互斥手段（**全包无事务**，`grep BeginTx` 零命中）—— 选主、任务场次占有、提案行级接受、节律日边界、清醒标记。纯 CRUD 表达不了任何一条，**少了条件写这些保证会全部静默降级成「通常能用」**，且降级不报错，只在并发下偶尔出错。

协议底线是 **CRUD，其中 insert 在 id 冲突时失败而不是覆盖**。有了这一个原语，条件写要么原生支持、要么适配层用一把锁补出来（代价：一次写从 1 个往返变成 4 个；而一把**错**的锁比没有锁更糟 —— 没锁是偶尔丢更新，锁错是整张表卡死。配方与四条硬要求见协议文档）。

**传输两种**：`http`（服务，可远端、可扩容）与 `exec`（后端监管的子进程）。`exec` 不是降级 —— 崩溃给**退出码 + stderr**，而 HTTP 适配层崩了只剩 `connection refused`。顺带记下：Go 的 `plugin` 包不适合第三方生态（仅 Linux/macOS、要求完全相同的工具链与依赖版本、无法卸载），所以「子进程 + 协议」不是绕开 Go 的弱点，**它就是 Go 的标准答案**。

⚠️ **顺序：先套件，后后端。** 批次 C 的审查在现有两个后端之间抓到约十五处行为分歧，而当时没有任何测试断言两者行为相同。再加一个后端只会把分歧从 O(1) 对变成 O(n²) 对。

### SQL 侧存自由数据的三种写法

「一个大 JSON 每次整体重写」是最省事的一种，但不是唯一的，也不总是最好的：

| 写法 | 适用 | 代价 |
|---|---|---|
| **JSON 路径写**：`json_set(doc,'$.k',?) WHERE json_extract(doc,'$.k') > ?` | 自由结构**且**需要条件写 | 三方言语法不同（`json_set`/`jsonb_set`/`JSON_SET`），要走 `Dialect` 方法；索引靠生成列或表达式索引 |
| **侧表** `(entity_id, key, value)` | 键集开放**且**要按键单独查 | 一次 join、行数放大 |
| **热字段提列 + 其余 blob** | 大多数情况 | 加字段要动 DDL |
| **整包 blob 重写** | 从不被查询、从不被条件写 | 放弃条件写 |

已实测：SQLite（modernc，带 JSON1）**一条语句就能做条件式部分更新**，`json_set` 配 `WHERE json_extract(...)` 影响行数正确。Postgres 是 `jsonb_set`/`->>`，MySQL 8 是 `JSON_SET`/`->>`。

**Mongo 不是「不需要」，是同一个问题的另一种写法** —— `rhythmRepo.Observe` 用的 `$min`/`$max` 就是路径写，与 SQL 的 `json_set(...) WHERE json_extract(...)` 同构。所以「统包 blob」在两边都不是必需的。

**主题变量是侧表更合适的那一类**：F7 的补算要问「哪些主题缺 token X」，侧表是一个 `WHERE`，blob 是全表扫加逐个解析；「给所有主题加一个 token」也变成每主题一次 INSERT 而不是整体重写。

## 文件总线的 HTTP 面：取舍与边界（ε，2026-08-06）

存储侧在 [DATA.md「附件与文件总线」](DATA.md)。这里是端点这一层为什么长这样。

### 上传体是原始字节，不是 multipart，也不是 base64 JSON

| 方案 | 为什么不 |
|---|---|
| **base64 进 JSON** | 每个字节涨三分之一，两端都要把整份文件拿在内存里。文件总线的存在理由第一句就是「装不进 JSON 体的字节」，用 JSON 送它是自相矛盾 |
| **multipart/form-data** | 单文件场景下什么都没买到：这边多一个解析器，四个前端各多一段 FormData 代码 |
| **原始字节 + `Content-Type`**（选中） | `fetch(url, {method:'POST', headers:{'Content-Type': file.type}, body: file})` 就说完了；`blob.Store.Put` 本来就收 `io.Reader`，可以直接流进去不落内存 |

**文件名走 query 而不是 header**：非 ASCII 文件名放 header 需要客户端做 RFC 5987 编码，放 query 只需要 `encodeURIComponent`。

### 下载一律代理，永远不给签名 URL

`blob.Store` 有可选的 `URLSigner`（S3 这类能签，本机目录不能）。**端点仍然只代理**：让客户端处理两种形状，实际结果是它只会被测到作者部署的那一种，另一种在别人的部署上第一天就坏。签名是以后的优化，`GET /api/files/{id}` 是契约。

### 三个必须存在的响应头

| 头 | 少了它会怎样 |
|---|---|
| `X-Content-Type-Options: nosniff` | 浏览器嗅探内容类型，一个上传的 `.html` 变成同源页面 |
| `Content-Security-Policy: default-src 'none'; sandbox` | 上传的 `.svg` 打开就是**同源脚本**，能读走本站 cookie |
| `Content-Disposition`（经 `domain.SafeFilename` + `mime.FormatMediaType`） | 客户端送的文件名是唯一能从请求走进**响应头**的自由文本 —— CRLF 就是响应头注入 |

`ETag` 用内容 SHA-256，所以 `If-None-Match` 是精确的，可以配 `immutable` 长缓存。

### 状态码分工（客户端据此决定要不要重试）

| 码 | 含义 |
|---|---|
| `400` | 没有 `Content-Type`，或**零字节** —— 零字节永远是客户端 bug（空 File、读了一半），不是谁想留的文件。这时已落盘的字节会被删掉，不留一条解析成空洞的行 |
| `404` | 这个会话没有这个附件（含「是别人的」） |
| `409` | 它已经跟着消息发出去了 —— 删消息才能删它 |
| `410` | 行还在、字节没了（清扫做了一半、或者目录被还原时漏了文件）。与 404 分开是因为**客户端该不该继续问**不一样 |
| `413` | 超过 `MAX_UPLOAD_BYTES` |
| `503` | 这个部署没有 `BLOB_STORE`。**是受支持的配置，不是故障** |

### 两个上限是两个问题，不要合并

- `MAX_UPLOAD_BYTES`（默认 32 MiB）—— **放得下吗**。
- `MAX_IMAGE_BYTES`（默认 8 MiB）—— **塞得进一次模型请求吗**。

一份 30 MiB 的 PDF 是个正常上传、是个糟糕的提示词。合并成一个数，就只能取两者里小的那个，于是「能存但当前模型读不了」这个完全正常的状态变成了「传不上去」。超过内联上限的附件走「模型读不了」那条路：把文件名念给模型听。

### `features.files`

`GET /api/version` 的 `features` 里多一位，由**有没有配 `BLOB_STORE`** 决定（其余四位由模型目录派生）。前端据此决定画不画回形针 —— 否则用户点了得到 503，读起来像 bug。

## 四个后端的已知行为差异（调用方必须绕开的）

行为一致性套件把绝大多数差异变成了「不许不一样」。下面这些是**留在调用方这一侧**的，套件管不到，每一条都真的咬过人：

| 差异 | 后果 | 绕法 |
|---|---|---|
| **`ChatRepository.AppendMessages` 不把生成的 id 写回调用方切片**（mongostore 侧） | 依赖回读 id 的代码只在四个后端里的一个上失败，而且只在特定路径（ε：只在用户发了附件时绑不上） | 调用方**预生成 id**（`uuid.NewString()` 后放进 `ChatMessage.ID`）。async companion 早就这么做，注释里写着原因 |
| **空字符串 vs 字段缺失**（BSON `omitempty`） | `{"field": ""}` 在 Mongo 上匹配不到省略了该字段的文档，同一个谓词在 SQL 上匹配所有默认行 —— 清扫/筛选静默变成空操作 | 参与查询的字段**不加 `omitempty`**，显式存 `""`。见 `mongostore/attachment_repo.go` 的 doc |
| **JSON 数字回来的 Go 类型**（BSON int32 vs encoding/json float64） | `args["minutes"].(float64)` 在一个后端上断言失败 | proposals 的 rows/ops 两边都过 `encoding/json`，代价是两边同样有精度上限（>2^53 的整数不能进工具参数） |
| **nil 切片 vs 空切片** | 一边回 `nil` 一边回 `[]`，JSON 序列化出 `null` 与 `[]` 两种 | 套件已断言统一（`RoundTrip` 类用例），新 repository 照做 |

**加一条差异时**：先问它能不能变成套件里的一条用例（那样它就消失了）；只有在 `domain.Store` 接口表达不了的时候，才记到这张表上。

## 中间件链（server.go 底部，全局单链，无分组）

```
recoverMW → requestIDMW → loggingMW → corsMW → sessionMW → userMW → dataSessionMW → mux
```

三个身份中间件是**「解析不强制」**：cookie/header 有效才往 ctx 注入，从不拦截。真正的鉴权在 handler 内 `requireSession`（无 sid → 401 no_session）或 `adminAuthorized`。详见 AUTH.md。

## 路由注册模式

**分散注册**：每个 handler 文件在自己的 `init()` 里 `registerRoutes("<组名>", func(s *Server, mux Mux){ … })`，`Handler()` 遍历注册表。注册表在 `internal/server/routes.go`。当前 **109 条路由 / 24 个组**，`server.go` 只剩静态 `/` 那一条（它有条件，只在 `STATIC_DIR` 存在时挂）。

原先是 `Handler()` 里 100 行集中注册，让 `server.go` 成了全仓最抢手的文件（12 个工作项都要改同一份清单）。**顺序无关紧要** —— Go 1.22 的 ServeMux 按 pattern 具体度而非注册顺序裁决，所以打散不会改变谁胜出，`/` 兜底也永远输给任何真路由。

`Mux` 是个只有 `HandleFunc` 的接口，不是 `*http.ServeMux`：注册打散之后就没有任何一处能读到完整 HTTP 面了，而 `*http.ServeMux` 无法枚举自己收了什么。换成接口，`RouteTable(*Server)` 就能拿一个记录器把注册重放一遍，把那份清单还回来 —— 传 `&Server{}` 即可，闭包只取方法值不调用，所以**读路由表不需要数据库**。

于此之上三条测试（`routes_test.go`）：pattern 不重复（ServeMux 撞了是 panic，但要等到起服务才炸）、每条都带方法、以及**与 `api/openapi.yaml` 双向核对** —— 服务了没写进契约、写进契约没人服务，两个方向都报错。这是实时文档铁律里「加了路由就改 openapi」那一条第一次真正由 CI 兜住。

路由→handler 文件映射见 API_SURFACE.md，REST 细节以 `api/openapi.yaml` 为准。

## main.go 启动/关停

- 启动顺序：config.Load → store.Open+Migrate → catalog/prompts → server.New → locale 覆盖层 → 两个 cleanup ticker → **Worker（无条件启动）** → channels（仅当配了通道）→ inbound 消费循环。
- **Worker 不再绑在 OneBot 上**（2026-07-29）。它曾经只在 `ONEBOT_WS_URL` 非空时启动，于是没绑 QQ 的用户拿不到早报、晚复盘、定时 auto-plan、deadline 巡检 —— 产品「主动」那一半被一个无关设置整体关掉。Worker 做的事没有一件需要通道：产出全部落库、由 App 读，推到通道只是可选的最后一步（`sendToChannels` 在 registry 为 nil 时只记日志，那个分支本来就有）。
- **按用户排程改成首次请求时懒排**（`Server.SetScheduleOnUse`，由 `markAwake` 在节流放行时调一次，`ScheduleUser` 本身幂等）。原来的种子是「每个已验证的通道绑定」——同一个耦合的第二处。改用枚举会话则需要给 `SessionRepository` 加方法、四个后端各实现一遍，而且会给每个曾经调过一次 API 的人都挂上 cron 项。⚠️ 代价是**重启当天早上有个缺口**：那天还没发过请求的人没有 cron 项，07:00 重启后 07:30 的早报对他不触发。补它需要正是这里在回避的会话枚举，且必须与 Lease 选主同批（否则每个实例都会给每个用户排程）—— 批次 ζ。
- inbound 消费：每消息经 `srv.GoTracked` 起 goroutine（背景 ctx，不绑请求），纳入 `Server.asyncWG`。
- 启动清扫：Migrate 后 `store.Chats().FailPendingMessages` 把崩溃遗留的 pending 占位消息标为 error。
- 关停：SIGINT/SIGTERM → `httpSrv.Shutdown(15s)` 等 in-flight HTTP → `srv.WaitBackground(shutdownCtx)` 等后台 agent（异步聊天/通道回复 + 节律信号写入）→ `worker.Stop()`（等 `cron.Stop().Done()`）→ `cancelRoot()`。TempContext/ChannelBinding 两个 cleanup ticker 仍是自由 goroutine（无状态，进程退出即弃，无碍）。
- **两个 cleanup ticker 现在逐 tick 收 panic**（`Server.everyTick`，2026-07-29）。它们原本是裸 `go func(){ for range ticker.C {…} }`，而 `recoverMW` 只包 HTTP handler —— 自由 goroutine 上的 panic 会带走整个进程。一次 tick panic 只杀那一次：清理失败是几行陈旧数据，清理循环停掉是无界增长。这也是降级启动的前置（store 为桩时不会 panic，但 nil store 会，症状是「启动成功、一分钟后无声死掉」）。

## 文件总线（`internal/blob`，2026-08-01 落地本机磁盘）

Daycore 此前**没有地方放一个字节**：上传 ≤1 MiB 的被 base64 塞进 `temp_contexts` 一小时 TTL，更大的字节直接丢；图片只能以 base64 内联在请求里到达模型；`Material.StorageRef` 是三方言建了列、Mongo 也持久化、openapi 也发布，却**没有任何代码生产或解析过一个 ref** 的空壳。四个功能在等同一样东西 —— 图片上传、PDF、文生图、给适配层子进程递产物。

**注册表形状照抄 `internal/storage`**：驱动名选实现，驱动 `init()` 自注册，`main.go` blank import。加一个后端是一个包加一次 `Register` —— 与数据库层同一个交易，理由也一样（没人该为了指向自己的对象存储而 fork 这个仓库）。计划中的驱动：本机 / 从机 / S3 / OSS / COS / 七牛 / 又拍 / OBS / KS3 / OneDrive / DB blob 列，外加 `http` 与 `exec` 两个通用兜底。

**`Store` 不是鉴权**。它把 id 映射到字节，对会话一无所知 —— 与参考 MCP 那套「文件总线是哑的、编排器决定谁能读」同构。发 ref 的人负责决定谁能用它。**也不是数据库**：没有列举、没有查询、没有事务，一个只会 PUT/GET 的后端是合法实现，这正是「指向 S3、或一个目录、或一个列」能成为真选项而不是口号的原因。

`URLSigner` 是**可选**接口：对象存储都能签，而签不了的两个（本地目录、数据库列）恰好是代理转发很便宜的那两个。所以调用方永远要有代理路径，把签名当优化。

**`DATA_DIR` 是仓库的第一个可写路径。** 此前每个目录配置（`STATIC_DIR`/`LOCALES_DIR`/`PROMPTS_DIR`/`MODELS_CONFIG`）都是只读输入，`internal/` 里没有一处 `os.WriteFile`。`BLOB_STORE` 为空 = 没有文件总线，这是**受支持的配置**：需要它的功能各自检查并说明，而不是让进程为一个多数部署第一天用不到的能力拒绝启动。

**ε 批次给了它第一个真正的使用者**：`POST /api/files` 存字节、`attachments` 表存所有权、`GET /api/files/{id}` 按会话解析。在那之前 `blob.Store` 是写完、跑过 11 例行为套件、接进 `Server` 却零调用方的一层。分工没变 —— 总线仍然只认 ref 不认会话，鉴权全在 `attachments` 行上，见 DATA.md。

本机驱动的三个要点：**先写临时文件再 rename**（同目录内 rename 是原子的，读者永不会看到半个 blob，崩溃留下的是游离临时文件而不是看起来像数据的截断文件）；**rename 前 sync**（否则崩溃可能留下一个名字对、内容空的文件，而那正是这套动作要防的）；**ref 必须是本 store 铸的 64 位十六进制**，因为 ref 来自外部（数据库列、工具参数），把攻击者选的字符串拼到根目录上正是文件总线变成任意文件读的方式。

**行为套件先于第二个驱动存在**（`internal/blob/blobtest`，11 例）。存储层是用昂贵的方式学到这一课的：两个后端上线、没有任何测试断言它们行为相同，事后审查找出约十五处分歧。两两分歧数随后端数平方增长，而上面那张驱动清单很长。

其中一条值得单独说：**相同内容必须产生不同的 ref**。内容寻址会让两个用户的相同上传共享存储，然后一个人的删除会拿走另一个人的文件 —— 或者更糟，用户能探测某个文件是否已存在。去重不值这个代价。

## 存储不可用时的降级启动（2026-07-29 定案，未实现）

**决定**：存储不可用**不拒绝启动**。CLI 把话说明白，HTTP 照常起来只服务控制台与健康面，需要库的端点给统一的不可用信封。协议侧的措辞与理由在 [`docs/specs/transport.md`](specs/transport.md#存储型cli-说清楚--web-ui-仍然能上)，这里记落地事实。

**现状是一条全或无的直线**：`main()` 只做 `run(logger)`，失败就一行 JSON 日志 + `os.Exit(1)`（`main.go:51-54`）；`run` 里每一步都是 `return fmt.Errorf(...)`。`storage.Open` 内部真的 `Ping`（`sqlstore/store.go:52`、`mongostore/store.go:41`），所以「库没起来」在 `main.go:75` 就退出了，连 `Migrate` 都到不了。

**HTTP 层其实早就写好了答案，只是到不了**：`GET /api/healthz` 在 Ping 失败时返 503 `{"ok":false,"db":…,"error":"database unavailable"}`（`handlers_misc.go:21-27`）—— 进程既然拒绝启动，这段分支**永远执行不到**。所以这条改动是让代码自洽。

### 启动路径上真正的硬依赖只有五处

| `main.go` | 是什么 | 降级时怎么办 |
|---|---|---|
| `75-78` | `storage.Open`（含 Ping） | 不 return，换成 null store |
| `81-83` | `store.Migrate` | 不 return，但**要更响** —— 表结构半成品比连不上更危险 |
| `93-95` | `FailPendingMessages` | 已经是「失败也继续」（`err != nil` 时既不 return 也不 log —— 顺带记：这个静默吞错本身是缺陷） |
| `107` | `ai.NewPromptService(store.Prompts())` | `prompts.go:144-146` **已有 nil-repo 降级语义**（读走内嵌、写报错），现成 |
| `141-180` | channels 块（含 `ListAllVerified`） | 整块跳过 |

`server.New` / `weather.New` / `search.NewMaterialSearcher` / `ai.LoadCatalog` / `auth.LoadOAuthProviders` **全都不碰库**；路由注册也不需要 store（`routes.go` 的 `RouteTable` 用零值 `Server` 就能跑）。所以 HTTP 面本身没有障碍。

### ⚠️ 两个会让「起来一分钟后自己死」的 goroutine

`main.go:135-136` 无条件启动的两个清理 ticker（`StartTempContextCleanup` / `StartChannelBindingCleanup`）**内部没有 recover**。store 为 nil 时它们在第一次 tick（1 分钟 / 10 分钟）就 nil-panic，**把整个进程带走** —— 而 `recoverMW` 只包 HTTP handler，管不到自由 goroutine。表现是「启动成功、一分钟后无声无息地死」，是这次改动里最容易踩且最难查的一个坑。

### null store 胜过 nil store

中间件链里只有 `userMW` 碰库（`middleware.go:138`），而且只在请求带**签名有效的 JWT** 时才碰。听起来无害 —— 但后果是：**浏览器只要还带着一个有效的 `dc_auth` cookie，连 `GET /` 的 index.html 都会 panic**，被 `recoverMW` 变成 JSON 500，用户看到白屏。「控制台能上」当场落空。

所以传一个所有方法都返回 `domain.ErrUnavailable` 的桩，而不是 nil：

- **不漏**。nil 方案要逐个 handler 检查（100 条路由），漏一个就是一次 panic；桩方案是「默认安全」。
- 用 `RouteTable()` 在中间件层按路由分类挡掉也不行 —— 那张分类表得手工维护、必然漂移，而且 `userMW` 跑在路由裁决**之前**，它挡不住。
- 代价是约 115 个方法的样板。生成它；`storagetest` 可以顺手断言它从不返回 `nil, nil`。

### CLI 提示与 i18n

运行期现在只有 slog JSON（`main.go:50`），失败就是 `{"level":"ERROR","msg":"fatal","err":"open db (sqlite): …"}` —— **仓库里没有任何「给人看的启动错误」先例**。唯一漂亮的 CLI 是 `daycore install` 子命令（`install.go:28-31` 的 heading/done/hint/prompt + ANSI 色），直接复用。

**这会是第一条需要本地化的启动期文案，而它恰好可行**：`config.Load` 在打开存储**之前**就已经 `i18n.Std().LoadDir(LOCALES_DIR)`（`config.go:193-205`），所以那一刻文件层与内嵌层都在，只有 DB 覆盖层没有 —— 这正是「内嵌是地板」当初要覆盖的场景。

### 安全：`ADMIN_TOKEN` 未设 = dev 全开，绝不能带进降级模式

`adminAuthorized` 在 `ADMIN_TOKEN` 为空时返回 `!IsProduction()`（`handlers_admin.go:24-26`）。降级模式下 DB 支撑的会话全没了，管理面就是**唯一**的门 —— 一个 `APP_ENV != production` 的部署，存储一挂就变成挂在网上的**无鉴权配置界面**。降级模式必须要求显式凭证，没有就只给一个说明页。与 `docs/AUTH.md`「管理面鉴权待改造」同批设计。

### ⚠️ 今天其实还没有控制台可上

这一条必须写在前面，否则会以为改完 `main.go` 就完事了：

- 控制台 web UI 目前只是 `design-ui/liuli/admin/` 的**纯 mock 原型（零 fetch）**；`web/frontend/src` 里 grep `admin` 零命中 —— **`STATIC_DIR` 里没有可上的控制台**。
- 8 个分区里无 DB 仍有意义的三个（服务配置 / 模型 / OAuth，都是改文件不是改库）**后端端点一个都不存在**。
- `DB_DSN` 只能从环境变量／`.env` 来，所以「无库时上 web UI 改配置」还需要写 `.env` + 重启确认（`restart-ack`）这套同样不存在的东西。

**所以依赖链是 F1（配置分层）+ F4（admin 端点）+ F5（控制台前端）**。在那之前，降级启动能交付的是诚实的一半：**CLI 说清哪一项错了、该改什么**，并让 `/api/healthz` 的 503 第一次真正可达。

## 静态托管（static.go）

- `STATIC_DIR`（默认 `web/frontend/dist`）非空且有 index.html 时：`/api/` 前缀永不被静态遮蔽；`/assets/` immutable 长缓存；其余 SPA fallback 回 index.html（no-cache）。
- `STATIC_DIR=""` 或无构建产物 → **纯 API 模式**（不挂 `/` 路由）。`deploy/Dockerfile` 不 COPY 前端产物，本就是纯 API 镜像。

## 配置（internal/config/config.go，环境变量）

关键项：`APP_ENV`/`HOST`/`PORT`；`STATIC_DIR`；`ALLOWED_ORIGINS`（CSV，空=同源）；`DB_TYPE`(sqlite)/`DB_DSN`；`JWT_SECRET`/`COOKIE_SECRET`（prod 缺失报错，dev 回退不安全默认）；`JWT_TTL`(168h)；`SECURE_COOKIES`（prod 自动 true）；`AI_REQUEST_TIMEOUT`(120s)；`AI_RATE_LIMIT_PER_MIN`(30)/`AUTH_RATE_LIMIT_PER_MIN`(10)；`TRUST_PROXY_HEADERS`（只在可信反代后设 true，否则 XFF 可伪造绕过按 IP 限流）；`AGENT_MAX_ROUNDS`(6)；`MAX_IMAGE_BYTES`(8MiB，模型请求里内联附件的上限)/**`MAX_UPLOAD_BYTES`**(32MiB，`POST /api/files` 的上限 —— 与前者是两个问题：一个是「塞得进一次模型请求吗」，一个是「放得下吗」)；`ADMIN_TOKEN`；`ONEBOT_WS_URL`/`ONEBOT_TOKEN`；`MODELS_CONFIG`/`OAUTH_CONFIG`；**`BLOB_STORE`**（文件总线驱动名，空=没有）/**`DATA_DIR`**（唯一的可写路径，`local` 驱动用）；**`PROMPTS_DIR`**（空=只用内嵌；设了则 `<dir>/<locale>/<key>.tmpl` 逐文件覆盖内嵌模板，**且 `<dir>/boundaries.json` 覆盖 L1 硬边界 —— 那一份没有 DB 层，是控制台唯一够不着的提示词**，见 AI.md）；天气三项；`COOKIE_SAMESITE`（lax|strict|none，none 需 Secure）；**`DEFAULT_PRIMARY_LOCALE`/`DEFAULT_SECONDARY_LOCALE`/`LOCALES_DIR`**（前两个是**新用户的默认**一主一副，不限制用户能选什么，`Load()` 里 `i18n.NewPair` 校验、值不对启动失败；`LOCALES_DIR` 放 `<locale>.json` 语言包 —— 加语言不用重新编译，见 DATA.md「多语言机制」）。

## 仓库级布局

- `api/` = 契约唯一权威（openapi.yaml + FRONTEND_HANDOFF.md）。**`openapi.yaml` 是生成物**，源在 `api/spec/`（`head.yaml` + `paths/<tag>.yaml` 一个 tag 一个文件 + `components.yaml`），`make api-bundle` 拼回同一个路径 —— 下游生成器不受影响。理由与三条强制规则见 `api/spec/README.md`；`go test ./...` 会因产物过期而红。
- `docs/` = 实时项目文档，随代码同批更新；`EXPERIENCE_CORE.md` 是端无关语义总纲（v2.3）。
- `extension/` = Chrome MV3 插件（Canvas 抓取 → 导入）。
- `design-ui/` = 设计原型，只读参考不参与构建。四端源码（liuli / zhiyu / ting / liuli-classic）+ `core/daycore-core.js` 共享 mock + `HANDOFF/` 交接文档 + `_ds/` 设计系统原件。其 `API_CONTRACT.md` 的路径命名非权威（见该目录 CLAUDE.md 顶部裁决）。
- `web/frontend/` = 现役前端，将来被四端替换。
- 设计系统已 vendor 进 `web/frontend/src/ds/` 与 `src/vendor/ds-bundle.js`；原件在 `design-ui/_ds/`。

## 目标架构：前后端分离（进行中）

后端收敛为纯 API 服务，四个前端各自独立部署、做成 git 子仓库，只靠 **API 契约 + 版本协商**耦合：

- **后端报告**：`GET /api/version` → `{apiVersion, apiMinor, minClient, build, channel}`
- **前端声明**：各子仓 `package.json` 声明最低支持的 `apiVersion`/`apiMinor`，启动握手不满足则降级提示（不白屏 —— 语气铁律「给死路一条岔路」）
- **升版规则**：breaking 升 `APIVersion`，additive（新端点/新字段）升 `APIMinor`。整批工作统一升一次，不要每个改动各升各的。

版本三层的区分见根 `CLAUDE.md`「目标架构」一节。

## 配置分层（现状与待改造，2026-07-26 核实）

配置目前分三处，**问题是「能不能热改」没有被设计过**：

| 层 | 载体 | 生效方式 | 现状 |
|---|---|---|---|
| 启动期 | 25 个环境变量（`internal/config/config.go`） | 改了要重启 | 合理的部分：密钥、`DB_DSN`、`HOST`/`PORT` |
| 同上 | 但也塞了本该热改的：`WEATHER_PROVIDER`、`TAVILY_API_KEY`、`ONEBOT_WS_URL`、各阈值 | 改了要重启 | **错位** —— 运维控制台永远配不到 |
| 数据驱动 | `config/models.yaml`、`config/oauth.yaml` | 重启加载 | 方向对，但无端点、多实例不同步 |
| 运行时覆盖 | `prompt_overrides` 表 + `PUT /api/admin/prompts/{key}` | 立即生效 | **唯一做对的范式**，模型/OAuth/服务配置应照抄 |

两处 registry 不一致，也在待改造之列：

- `internal/weather` —— `Register(name, Factory)` + 四个 provider 子包 init 自注册 + `WEATHER_PROVIDER` 选择。**这是正确形状**。
- `internal/search` —— 硬编码 `if TavilyKey != "" { Tavily } else { DuckDuckGo }`，**无注册表**，物理上无法增删搜索通道。
- `internal/channels` —— 有 `Registry`，但 `cmd/daycore/main.go:141` 是 OneBot 硬编码单例，且**拿 `ONEBOT_WS_URL` 是否为空来决定要不要启动 Worker** —— 后果是没绑通道的用户，节律学习、定时 auto-plan、20h 关怀全都不跑。这是既有缺陷，落地节律后会非常显眼。

运维控制台（`design-ui/liuli/admin/`，8 个分区，**留在主仓库不做独立子仓库**）依赖的 `PUT /api/admin/{config,models,oauth}`、`POST /api/admin/models/test`、`restart-ack` 目前**全部不存在**；已有的只有 prompts / stats / ailogs / users / db 五组。

## 目标：外部能力走 HTTP 适配器，不再改代码

**接一个新的搜索源／天气源／消息通道，不应该需要改 Go 代码或写插件** —— 只应该需要写一个 HTTP 适配层，它可以独立部署、放在别的服务器、独立扩缩容。

设计沿用 `config/models.yaml` 已验证的模式（`format` 绑定 registry 里注册的实现，`base_url` 指外部服务），推广到 weather / search / channels：内置 provider 保持 Go 实现走各自 format，**外部适配器统一走 `format: http`**。对上层透明 —— `WeatherProvider` / `Searcher` / `Channel` 三个接口一行不用改。

两类协议形状不同，**载体也不同**：

**查询型（天气、搜索）= HTTP** —— 无状态请求／响应，天然可并发，WS 反而累赘：
```
GET  /manifest   → {name, displayName, logo, type:"query", capabilities:["weather"|"search"]}
POST /weather    {lat, lon, date, tz}   → {temp, condition, …}
POST /search     {query, limit, locale} → {results:[{title, url, snippet}]}
```

**通道型（QQ/napcat 等）= WebSocket** —— 双向、有状态、需要身份。只在真的收发消息时才有流量。帧协议：
```
后端 →  {"t":"hello", "protocol":1}
适配器 → {"t":"manifest", "name","displayName","logo","features":{attachments,markdown}}
适配器 → {"t":"inbound", "externalUserId","externalName","avatar","messageId","text","attachments","ts"}
后端 →  {"t":"send", "to":"<externalUserId>","text","replyTo":"<messageId>"}
后端 →  {"t":"ping"}   // 断线重连沿用 onebot 现有逻辑
```

**连接方向必须两种都支持** —— 取决于谁在 NAT 后面，协议不该替部署做决定：

| `mode` | 谁连谁 | 适用 |
|---|---|---|
| `dial` | 后端作 client 连出去（适配器当 server） | 适配器有可达地址。**现有 onebot 就是这种**（`websocket.DefaultDialer.DialContext`） |
| `listen` | 后端开 WS 端点，等适配器连进来 | 适配器在 NAT／内网后面，后端有公网地址 |

OneBot 11 本身就规定了三种接入方式，现有实现只覆盖第一种，都要补：

| `providers.yaml` 的 mode | OneBot 官方术语 | 现状 |
|---|---|---|
| `dial` | 正向 WebSocket（napcat 当 server） | ✅ 已实现 |
| `listen` | 反向 WebSocket（napcat 主动连后端） | ⬜ 待补 |
| `http` | HTTP 上报 + HTTP API 调用 | ⬜ 待补 |

`internal/channels` 的 `Channel` 接口（`Send(externalID, msg)` + `Start(inbound chan<-)` + `Stop()`）对三种模式**都适用** —— `Start` 里是拨出去还是挂个 handler 等连接，是实现细节。**零接口变更**。

`manifest` 必须自带 `displayName` 与 `logo` —— 控制台与前端据此渲染，**接一个新通道零前端改动**。

### 内置 vs 外部：写了的直接配置，没写的走转换层

| 能力 | 内置（Go 实现，直接配置） | 外部（写适配层） |
|---|---|---|
| 天气 | openmeteo / qweather / openweathermap / wttrin（已有四个） | `format: http` |
| 搜索 | tavily（有 key）+ DuckDuckGo（免费兜底） | `format: http` |
| 通道 | napcat/OneBot（已有）+ 后续常见可接 bot 的平台 | `format: ws` |

### 搜索有三层来源，第一层目前是死代码

1. **模型厂商原生搜索** —— 走 **Anthropic 的 web search server tool 规范**（DeepSeek 的 Anthropic 兼容端点即此；kimi 等同类可比照）。**结果不是「融进回答的黑盒」** —— 规范返回的是 `web_search_tool_result` 块，其 `.content` 是一个 **`web_search_result` 结构化列表**（带 citations），后端拿得到、可落库、可展示来源。

   ⚠️ **现状：完全未接线（2026-07-26 核实）**。`config/models.yaml` 的 `deepseek_search: true` 是死配置：
   - `Capabilities.DeepseekSearch` 只被写入（`internal/ai/models.go:86`），**没有任何消费方**
   - `ToolDef.ServerSide`（`internal/ai/provider.go:44`）**全仓零赋值**
   - `internal/ai/formats/anthropic/anthropic.go:106` 序列化 tools 时只输出 `{Name, Description, InputSchema}`，**没有 type-based server tool 分支**
   - 同文件只解析 `text` / `tool_use` / `content_block_delta` / `message_stop` 四种块，**不认识 `server_tool_use` 与 `web_search_tool_result`**

   要接通需要两件事：发请求时按 `{"type": "web_search_20250305", "name": "web_search", "max_uses": N}` 序列化，收响应时解析 `web_search_tool_result` 并把 `.content` 映射成 `SearchResult`。

2. **内置工具搜索** —— agent 调 `web_search` 工具 → `Searcher` 接口 → tavily / DuckDuckGo。已实现。
3. **外部适配器** —— 协议转换层，`format: http`。

三层的**执行位置不同，结果模型应当统一**：第 1 层由模型服务端执行，第 2、3 层由后端执行，但都产出「标题 + URL + 摘要」的列表，都该落到同一个 `SearchResult`，这样账本、引用展示、前端渲染只有一套。

### 厂商原生搜索有两种形状，配置入口不同

| 形状 | 机制 | 配置放哪 |
|---|---|---|
| **返回独立结果** | server tool 规范，响应里带结构化列表（Anthropic `web_search_tool_result` 即此，DeepSeek 走它） | **搜索配置** —— 要注册工具、要解析结果、要映射成 `SearchResult` |
| **直接嵌入提示词** | 没有结构化返回，靠提示词引导模型自己去搜并在回答里带上 | **模型配置** —— 本质是一段随模型走的提示词片段 |

模型有没有原生搜索、是哪种形状，由 `models.yaml` 的能力声明表达（把现有的 `deepseek_search` 布尔泛化成 provider 无关的 `server_search`）。

### 多个搜索源并存：把选择权交给模型

不做「后端挑一个搜索源」的硬路由，而是**把多个搜索源各自注册成工具**，每个带自己的描述，模型按当前问题自己选（这与体验内核共识 5「不设关键词硬规则、错了再改」同构）。描述通过提示词模板动态注入：

```gotemplate
{{if .Searches}}
## 可用的搜索来源
{{range .Searches}}- `{{.ToolName}}`：{{.Description}}
{{end}}{{end}}
```

## 配置驱动的提示词片段：哪里用得到，哪里就能改

**提示词片段应该跟着配置走，就近编辑，而不是全堆在 Prompt 管理页。** 一个搜索源该怎么描述给模型、一个消息通道有什么特性（QQ 不支持 markdown、回复要短），都是那个 provider 自己的属性 —— 编辑入口就应该在它自己的配置卡片旁边，而不是让人去 Prompt 页翻一个巨大的模板。

现有基建已经够用，不需要新机制：

- `internal/ai/prompts.go` 用 `text/template`，`Render(ctx, key, locale, data any)` 接任意数据 —— `{{if}}` / `{{range}}` 天然可用，且现有模板里已有 5 处 `{{if}}` 先例。
- `PromptService` 有 `Validate`（parse 校验），控制台保存模板时能挡住语法错误。
- `prompt_overrides` 表 + `PUT /api/admin/prompts/{key}` 已是「文件作种子 + DB 存覆盖 + 立即生效」的正确范式，provider 描述照搬。

落地形状：每个 provider 在 `providers.yaml` 里带一段 `description`（**zh-CN / en-US 双份 —— 提示词双 locale 是启动期硬校验**），控制台在该 provider 的配置卡片旁给「编辑描述」入口，改动落 DB 覆盖层，`companion_agent.tmpl` 用 `{{range}}` 消费。同一套机制覆盖搜索源、消息通道、天气源。

⚠️ `companion_agent.tmpl` 目前是**零插值**的纯规则清单（有意为之，见 `AGENTS.md`）。引入 `{{range}}` 是对它的第一次结构性改动，两个 locale 必须同批改。

落地计划见批次 F。
