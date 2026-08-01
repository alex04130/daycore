# 存储适配协议（v0，草案）

> 状态：**协议已定案、未实现**。落地是批次 F8b，前置条件是 F8a 的行为一致性套件（已完成，见 `internal/storage/storagetest`）。
>
> 这份文档是给**写适配器的人**看的 —— 包括不是本项目成员的人。

## 这是什么，不是什么

Daycore 的存储抽象是 `domain.Store`：**27 个 repository 接口、115 个方法**（2026-07-30 用 AST 数过；此前抬头写的「26 / 179」两个数字都是错的）、四个原生实现（sqlite / postgres / mysql / mongodb）。这个协议让**第五种存储不必用 Go 写在仓库里** —— 写一个适配器，说这套协议，就能接。

**它是兼容层，不是性能层。** 每次读都过一趟进程边界，而 companion 组一次提示词要读八次库。原生适配器是快路径，这条是「原生适配器写不出来」时的那条路。

**协议底线一句话**：CRUD，其中 **insert 在 id 冲突时必须失败而不是覆盖** —— 有了这一个原语，条件写要么原生支持、要么用锁补出来（见下）。连这个都没有的存储（`PUT` 是覆盖语义的 KV）接不上，因为它连一把正确的锁都建不出来。

## ⚠️ 条件写是必需的 —— 原生或模拟，但不能没有

这是整份协议唯一不能妥协的地方。

`sqlstore` **全包没有事务**（`grep BeginTx` 零命中），而 Daycore 的多实例正确性全部压在**单条条件语句**上。20 处，其中五处是骨架：

| 靠它的东西 | 语句形状 | 退成读-改-写的后果 |
|---|---|---|
| 选主 | `UPDATE leases SET … WHERE name=? AND expires_at <= ?` | 两个实例同时持锁 |
| 任务场次占有 | `INSERT`，靠唯一键冲突分辨「别人占了」 | 早报发两遍 |
| 提案行级接受 | `UPDATE … WHERE rev = ?` | 后写的擦掉先写的 |
| 节律日边界 | `SET first_min = CASE WHEN ? < first_min THEN ? ELSE first_min END` | 两个标签页开着就丢信号 |
| 清醒标记 | `UPDATE … WHERE last_signal_at < ?` | 迟到的信号把清醒段截短 |

一个只有 `get`/`put`/`delete` 的协议表达不了任何一条。**少了条件写，这些保证会全部静默降级成「通常能用」** —— 不报错，只在并发下偶尔出错，而那种错在测试里几乎抓不到（批次 C 的对抗式审查抓到的多数问题正是这一类）。

所以：**实现 `?ifMatch=` 和 `PATCH … where=` 才算实现了这个协议。** 缺了它们的适配器不是「功能少一点」，是不正确。

存储本身没有条件写不是死路 —— 可以用一把锁模拟出来。**谁来补见下面「补救归谁」那一节**（2026-07-30 修正：先前这里写着「必须由适配层补，不能由 Daycore 降级」，那条太死，已经改了）。

## 基础集 + 可选扩展集（RISC-V 那个形状）

**协议小，但可扩展。** 一个实现只需要做基础集；高阶能力做成**具名可选集**，由 `capabilities` 如实声明，Daycore 按声明决定走原生还是走补救。

### 基础集（`base`）—— 不做就接不上

| | |
|---|---|
| `get` / `put` / `delete` / `insert` | 其中 **insert 在 id 冲突时必须失败而不是覆盖** |
| `query`，谓词 `eq` `in` `lt` `lte` `gt` `gte` `exists` `notExists` | 单字段、AND 组合 |
| `?ifMatch=` 与 `PATCH … where=` | 条件写，见上 |
| `count` | |
| `capabilities` | 自述，也是健康检查 |

### 可选扩展集

| 扩展 | 内容 | 缺了怎么办 |
|---|---|---|
| `or` | 谓词的 OR 组合 | Daycore 拆成多次 `query` 再合并去重 —— **正确性等价**，代价是往返次数 |
| `like` | 子串匹配 | 同上，或退回全量扫 + 本地过滤（有上限） |
| `offset` | `query` 的跳过 N | 拉 `limit+offset` 条再本地丢弃；分页很深时代价显著 |
| `fulltext` | 原生全文索引 | 退回 `like`／子串扫描，**召回下限保证**（现有 `MaterialSearcher` 就是这么分层的） |
| `conditionalWrite` | 原生条件写 | **不一样，见下** |
| `batch` | 一次多个操作 | 逐条发 |

声明形状：

```
GET /v0/capabilities
  200 {
    "protocol": "0",
    "base": true,
    "extensions": ["or", "offset", "fulltext"],   // 有哪些高阶集
    "conditionalWrite": true,                      // false 时必须同时声明 emulatedVia
    "maxBatch": 100
  }
```

**未声明的扩展 = 没有**，Daycore 走补救路径，并在控制台标出「这个能力靠模拟」。**不允许「声明了但其实做不对」** —— 那比不声明糟得多，因为补救路径不会被启用。

## 补救归谁：两类，不能混为一谈

先前这份文档一刀切写「必须由适配层补」。那条不对，也不是协议该管的 —— **补救在转换层里做，只要它正确，就是合法的**。真正要区分的是补救**能不能保住原来的保证**：

### 甲类 · 补救保正确，只花性能（`or` / `like` / `offset` / `fulltext`）

这些是**纯查询重写**。OR 拆成几次 AND 查询再合并去重，结果集与原生一模一样；差别只有往返次数。`fulltext` 退回子串扫描会**少召回**，但不会返回错的东西 —— 这是仓库里已经在用的分层（`search/material.go` 先探原生索引、失败退子串，`Score=1` 作召回下限）。

**这一类 Daycore 侧补，理直气壮。** 它甚至该由 Daycore 补而不是适配层：Daycore 知道自己的查询形状与预算，适配层只看得见一个孤立请求，让它去猜怎么拆 OR 只会拆得更差。

### 乙类 · 补救会动摇保证（`conditionalWrite`）

条件写不一样。用一把锁模拟出来的 CAS **在锁正确时**等价，锁不正确时是**静默的错误**（不是慢，是错）。而分布式锁的正确性依赖的东西（租约、fence、时钟）不在这份协议的可见范围内。

所以乙类的规则保持严格，但理由说清楚了：

- 能原生做就原生做。三大 SQL 引擎都能，Mongo 本来就能。
- 做不到就**在适配层里补**，因为锁必须贴着数据待的地方 —— 隔着一次网络的锁，持锁者与被保护的写之间多一个可能失败的跃点，那正是丢正确性的地方。
- 必须声明 `emulatedVia`，Daycore 在启动日志与控制台标出来。**运维有权知道自己的正确性是靠一层模拟撑着的。**

一句话：**甲类补在 Daycore 侧（它知道得更多），乙类补在适配层侧（它离数据更近）。**

⚠️ 与之相关的一条**代码现状**：`domain/proposal.go:308` 有一句「禁止 Go 侧补救」的注释。那句针对的是**乙类**（提案行级接受的 `rev` CAS），不是通用规则 —— 按本节重新读它。

## 操作（8 个）

所有请求与响应都是 JSON。`{coll}` 是集合名（对应 SQL 的表名）。

### 读

```
GET /v0/doc/{coll}/{id}
  200 {"doc": {...}, "rev": 7}
  404 {"error":"not_found"}
```

```
POST /v0/query/{coll}
  {
    "where":  [{"field":"session_id","op":"eq","value":"s1"},
               {"field":"expires_at","op":"lte","value":1785000000000}],
    "order":  [{"field":"created_at","dir":"desc"},{"field":"id","dir":"desc"}],
    "after":  {"created_at":1784999000000,"id":"op-42"},   // 游标，可选
    "limit":  100
  }
  200 {"docs":[{"doc":{...},"rev":3}, …]}
```

**基础集的 `op`**：`eq` `ne` `lt` `lte` `gt` `gte` `in` `exists` `notExists`。单字段、AND 组合。

`or` 与 `like` 是**可选扩展集**，不在基础集里（见上）。

⚠️ 这里先前写着「没有 `or`、没有 `like` —— 现有 179 个方法都不需要」。**那句话是假的**：`sqlstore` 里 6 个文件用了 `OR`（`derived.go` `coordination.go` `proposals.go` `oplog.go` `users.go` `material_repo.go`），`material_repo.go` 用了 `LIKE`；其中 `derived.go` 与 `coordination.go` 的 OR **正是上面那张「靠条件写的东西」表里第 5 行和第 2 行的机制**，而 `proposals.go` 的两处正是批次 D 要扩的那个 filter。协议自述与代码不符，是这份文档自己没跟上，不是代码越界了。

`offset` 同理是扩展集：基础集只有键集游标 `after`，但 `material_repo.go` 的分页要 offset。

`after` 是**键集游标**（不是 offset）：按 `order` 的字段逐个比较，严格大于/小于。`OpLogs().Scan` 的重放路径依赖它 —— 见「游标语义」。

### 写

```
POST /v0/doc/{coll}
  {"id":"…","doc":{…}}
  201 {"rev":1}
  409 {"error":"conflict"}      ← 必须与 500 区分！见下
```

```
PUT /v0/doc/{coll}/{id}?ifMatch=7
  {"doc":{…}}
  200 {"rev":8}
  409 {"error":"conflict","rev":9}   ← rev 已经动过
  404 {"error":"not_found"}
```

```
PATCH /v0/patch/{coll}
  {
    "where": [{"field":"lease_name","op":"eq","value":"worker"},
              {"field":"expires_at","op":"lte","value":1785000000000}],
    "set":   {"holder":"inst-7","expires_at":1785000060000},
    "inc":   {"fence":1},
    "min":   {"first_min":400},        // 只在更小时才写
    "max":   {"last_min":900},         // 只在更大时才写
    "unset": ["ended_at","error_text"]
  }
  200 {"matched": 1}
```

```
DELETE /v0/doc/{coll}?where=<urlencoded JSON array>
  200 {"deleted": 12}
```

### 元

```
POST /v0/count/{coll}
  {"where":[…], "sum":["useful"]}
  200 {"count": 128, "sum": {"useful": 91}}
```

形状见上面「基础集 + 可选扩展集」。要点：`extensions` 列出高阶集，**没列 = 没有**，Daycore 走补救；`conditionalWrite: false` 时必须同时声明 `emulatedVia`。

⚠️ `features` 这个旧字段名已被 `extensions` 取代（2026-07-30）。旧名字里塞的 `"fulltext"` 语义没变，只是它现在是一个具名扩展集而不是一个自由标签。

## 三条容易做错的语义

### 1. `matched`，不是 `changed`

`PATCH` 的 `matched` 是**匹配到的行数**，不是**值发生变化的行数**。

这不是吹毛求疵：MySQL 的 `RowsAffected` 默认数「改变的行」，而这个仓库有 23 处依赖它数「匹配的行」，其中三处把 0 变成 `ErrNotFound` —— 结果是**在 MySQL 上原样保存一个主题会返回 404**。`mysqlDialect.NormalizeDSN` 为此强制 `clientFoundRows=true`。协议必须把语义写死，否则第五个后端会重演同一个 bug。

把一行的每个字段都设成它已有的值，`matched` 也必须是 1。

### 2. 冲突必须与故障区分

`POST /doc` 撞唯一键要回 **409**，别的失败回 **5xx**。

因为「插入即占有」这个机制**完全依赖**这个区分：`jobRunRepo.Claim` 靠插入失败判断「别人占了这个场次」。如果一次瞬时写失败也长成 409，后端会把一次故障当成一次正常落选 —— worker 静默变闲，任何地方都没有一句话说明原因。SQL 侧为此专门加了「失败后查一次行是否存在」的兜底，协议这边可以直接把它变成契约。

### 3. 游标语义

`after` 是**严格**在游标之后，按 `order` 的字段序做词典比较。`OpLogs().Scan` 是 rapport 的重放路径 —— rapport 被定义为「账本的读数」，缓存必须能靠**按发生顺序重放每一条操作**重建，所以游标续读**不能重、不能漏**。一致性套件里 `OpLog/ScanIsOldestFirstFromACursor` 测的就是这条。

## 如果你的存储没有条件写

`conditionalWrite: false` 是允许的。它属于**乙类补救**（见「补救归谁」）：由适配层补，因为锁必须贴着数据待的地方。甲类（`or`/`like`/`offset`/`fulltext`）不受本节约束。

### 不可再少的那一个原语

「用一把数据库锁模拟条件写」是对的思路，但它有一个前提，必须点明：

> **能用来建锁的最小原语是「按给定 id 插入，若该 id 已存在则失败」。**

纯粹的 `get`/`put`/`delete` 建不出锁 —— 抢锁本身就是「不存在才创建」，那正是一次条件写。所以循环依赖只有一个出口：**插入必须在主键重复时失败，而不是静默覆盖。** 这个原语几乎每个存储都有（它就是主键唯一性），但有些 KV 的 `PUT` 是覆盖语义 —— **那样的存储接不上这个协议**，因为它连一把正确的锁都建不出来。

所以协议的真实底线是：**CRUD，其中 insert 在 id 冲突时失败。** 有了它，其余都能补。

### 用锁模拟的配方

以 `PUT ?ifMatch=7` 为例，适配层内部：

```
1. INSERT lock/{coll}/{id}  值 = {owner, expiresAt}      ← 冲突就是没抢到
   没抢到 → 退避重试；重试若干次仍未抢到 → 500（不是 409！这不是冲突，是忙）
2. 读 doc/{coll}/{id}，比较 rev
   不等 → 释放锁，回 409
3. 写新值，rev+1
4. DELETE lock/{coll}/{id}
```

四条硬要求，少一条这把锁就是错的：

1. **锁必须有过期时间。** 持有者崩在第 3 步，锁就永远留着 —— 那张表从此写不进去。`expiresAt` 加一个清扫（自己扫，或者靠存储的 TTL 索引）。
2. **过期不等于可抢。** 抢一把过期锁要用 `INSERT`-删除-`INSERT` 或带 owner 比较的删除，不能读一眼「过期了」然后直接覆盖 —— 那又是一次读-改-写。
3. **`min`/`max`/`inc` 也要走锁。** 它们和 CAS 一样是读-改-写，`rhythmRepo.Observe` 的扩边界正是靠 `min`/`max`。漏了它们，两个标签页开着就丢信号。
4. **`PATCH` 影响多行时，要么整批一把锁，要么逐行加锁。** 逐行不是原子的 —— 对 `Proposals().Expire` 这种「把过期的都推进终态」可以接受（每行独立），对需要「要么全改要么全不改」的**不行**。这个协议里没有后者，但别在自己的实现里制造出来。

### 代价，写清楚以免以后当成默认路径

- **一次写从 1 个往返变成 4 个**（抢、读、写、放）。存储型适配层已经在进程边界之外了，四倍往返意味着 companion 组一次提示词的八次读写会变成三十几次。
- **一把错的锁比没有锁更糟**：没有锁时问题是「偶尔丢更新」，锁写错时问题是「整张表卡死」，而后者只在生产的并发下出现。
- 所以：**能原生做条件写就原生做。** 三大 SQL 引擎都能（`json_set`/`jsonb_set`/`JSON_SET` 配 `WHERE json_extract(...)`，已在 SQLite 上实测一条语句成立），Mongo 的 `$set`/`$min`/`$max` 本来就是。锁是给真的做不到的存储留的门，不是省事的路。
- 这是**乙类补救**（见「补救归谁」）：它动摇的是保证本身，不是性能。甲类（`or`/`like`/`offset`/`fulltext`）不受这一节约束，那些由 Daycore 侧补。

适配层在 capabilities 里如实声明：

```json
{"conditionalWrite": false, "emulatedVia": "lock", "lockCollection": "daycore_locks"}
```

后端会照常使用它，但会在启动日志和控制台里标出来 —— **运维有权知道自己的正确性是靠一层模拟撑着的。**

## 两种传输

传输本身的规则（HTTP 状态码语义、超时重试、子进程握手与生命周期、`providers.yaml` 形状）在 **[transport.md](transport.md)**，四种适配层共用一份。这里只写存储协议特有的部分。

### `format: exec` 的握手行

存储适配层在握手那一行里**追加 `capabilities`**，这样后端在连上 socket 之前就能判断它能不能用 —— 一个 `conditionalWrite: false` 又没声明 `emulatedVia` 的适配层，后端不会先建连接再发现：

```json
{"protocol":"0","transport":"unix","address":"/tmp/daycore-store-8f3a.sock","pid":12345,
 "capabilities":{"protocol":"0","conditionalWrite":true,"features":["fulltext"]}}
```

`DAYCORE_ADAPTER_KIND=storage`。

⚠️ **「不会用它」不等于「拒绝启动」**（2026-07-29 更正）。这一段先前写的是「后端会直接拒绝启动」。不对 —— 一个说不清自己能力的存储适配层是个**配置问题**，而拒绝启动恰好拿掉了唯一的补救入口。正确的表现是 [transport.md「存储型」](transport.md#存储型cli-说清楚--web-ui-仍然能上)那条：**CLI 把话说明白 + HTTP 照常起来只服务控制台 + 需要库的端点给统一的不可用信封**。本文档自己在 `emulatedVia` 那一节已经用的就是这个范式（照常起来 + 在启动日志和控制台里标出来）。

### `format: http` 的额外要求

- 存储是**热路径**，连接必须可复用 —— 支持 keep-alive，不要每个请求关连接。
- `GET /v0/capabilities` 必须便宜（后端会用它做健康检查）。
- **不要在适配层里重试写操作。** 一次「超时但其实成功了」的写，重试会变成两次插入 —— 而「插入即占有」的语义下，那意味着占有判定失真。超时就把超时报回来。

## 一致性套件是验收标准

`internal/storage/storagetest` 是一套按 `domain.Store` 写的行为套件，四个原生后端跑的就是它（`make test` 跑 SQLite、`make test-mongo` 跑真机 Mongo）。

**一个适配器算不算写完，标准就是这套套件通过。** 它测的不是「能存能取」，是那些真出过错的地方 —— 已知的分歧里包括：

- `ProposalOp.Args` 里的数字在每个后端都必须回来是 `float64`（JSON 与 BSON 对这个不一致，而 agent 工具会对它做类型断言）
- 接管必须轮换占有令牌，让僵尸实例迟到的 `Finish` 落在空处而不是把健康的运行标成失败
- nil 切片回来必须是空切片；指向零值时间的指针算「不存在」
- 同毫秒并列时 `Supersede` 必须恰好留一张
- 学习作业不能擦掉活的清醒标记

这些不是从规范推出来的，是从**已经发生过的分歧**里来的。这也是为什么共享套件是加第五个后端唯一负担得起的方式：两两分歧数随后端数平方增长。

## 未定的口子

- **认证**：`token_env` 目前只设想 bearer。mTLS 要不要进协议未定。
- **批量**：`maxBatch` 已在 capabilities 里预留，但还没有批量端点。`Proposals().Expire` 这种跨会话扫描目前是一条 `PATCH`，量大时可能需要分页。
- **可观测性**：适配器要不要暴露 `/metrics` 未定。
- **`features` 的取值表**：现在只有 `fulltext`（对应 `domain.MaterialFTS`，不声明就自动退回子串扫描）。
