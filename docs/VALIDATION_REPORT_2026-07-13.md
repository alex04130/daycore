# Daycore v2 — 验证补完报告（2026-07-13）

> 范围：对 Go 后端做功能性 + 安全性全面审计，修复全部发现的 bug，将天气重构为适配器模式，重构 plan.md 的 P5/P∞ 规划，产出前端 API 契约。
> 结果：8 个批次全部完成，`go build/vet/test` 全绿，端到端冒烟通过。
>
> *路径说明：本报告成文时代码位于 `v2/` 子目录下；建立 git 仓库时 `v2/` 已上提到仓库根，文中链接均已同步为新路径。*

---

## 一、背景与方法

plan.md 宣称「P0-P4 全部完成 ✅」。但「标记完成」不等于「验证过无 bug」。本次用三个并行审计代理分别扫描**安全面**、**P0-P2 核心链路**、**P3-P4 + 天气 + 契约现状**，每条发现都落到 `file:line`。

审计的核心结论：**「P0-P4 ✅」严重夸大**。很多是「写了代码但没接线」或「写了框架但零实现」：

| 宣称完成 | 实际状态 |
|---------|---------|
| P3 通道 + 主动性 ✅ | **整块死代码**——`main.go` 从不实例化 Worker/Registry/OneBot，config 连 `ONEBOT_*` 键都没有；bind→verify 因查询谓词矛盾**永远无法成功** |
| P4 资料库全文搜索 ✅ | Searcher **四后端零实现**，`/api/materials/search` 永远返回 501 |
| P4 临时上下文 ✅ | TempContext 因空 ID **塌缩成全局单行**，跨 session 互相覆盖 |
| P4 收件箱 ✅ | upload **丢弃文件**，`temp_id` 不关联任何数据 |
| 时区三模式 ✅ | `timeutil` **零导入**，`UTCTime`/`OffsetMin` 从不读写——完全未实装 |
| chat 线程 ✅ | 实时对话**从不服务端持久化**，多轮上下文不累积 |
| 撤销全链路 ✅ | 四类操作（rule_delete/rule_batch/memory_delete/memory_clear）的 revert **全部失效** |
| 登录合并 ✅ | `mergeSessionData` **漏迁移 8 类表** |

`go test ./...` 曾全绿，但 `context.go`/`plan_patch.go`/`handlers_ops.go`/`timeutil`/`mongostore`/`channels` **零测试覆盖**——绿灯是盲区造成的。

---

## 二、修复概览（8 批次）

| 批次 | 内容 | 关键产出 |
|------|------|---------|
| **A** | 安全加固 | High×5 / Medium×5 / Low×4 全修 |
| **B** | P0-P2 功能 bug | 撤销四路 / 登录合并 / chat 持久化 / 压缩 / rule 校验 / panic 防护 |
| **C** | 时区三模式实装 | floating/fixed/local 锚定 UTCTime + DST + 旧数据回填 |
| **D** | P3 接线 + 修复 | main.go 接线 Worker/OneBot + bind→verify 修复 + cron 修复 |
| **E** | P4 补完 | Searcher 实现 / TempContext 修复 / inbox 持久化 / Wish JSON / wish-mood 联动 |
| **F** | WeatherProvider 适配器 | 接口 + 4 provider + registry + fallback + 缓存 |
| **G** | plan.md P5/P∞ 重构 + 记忆修正 | — |
| **H** | 前端契约 | openapi.yaml（93 操作）+ FRONTEND_HANDOFF v2 协议 |

---

## 三、批次 A — 安全加固

| 级别 | 项 | 修复 | 位置 |
|------|----|------|------|
| High | XFF 限速绕过 | `X-Forwarded-For` 仅在 `TRUST_PROXY_HEADERS` 开启时信任，取最后一跳（客户端不可伪造） | [server.go](../internal/server/server.go) `clientIP` |
| High | CORS `*`+credentials | wildcard 分支禁止发 `Allow-Credentials` | [middleware.go](../internal/server/middleware.go) `corsMW` |
| High | OAuth 未验证邮箱接管 | 只有 `email_verified=true` 才按邮箱链接已有账户；未验证邮箱建账户时不写 email | [handlers_auth_oauth.go](../internal/server/handlers_auth_oauth.go) |
| High | dev 弱密钥 + 开放 admin | dev 模式默认只绑 loopback（`127.0.0.1`），弱密钥启动告警 | [config.go](../internal/config/config.go) |
| High | channel token 过期不检查 | verify 强制 10min TTL + 后台 sweep（并入批次 D 重构） | — |
| Medium | admin token 非恒定时间 | `crypto/subtle.ConstantTimeCompare` | [handlers_admin.go](../internal/server/handlers_admin.go) |
| Medium | auth 无爆破防护 + argon2 DoS | 独立 auth 限速（默认 10/min）+ argon2 并发上限 4 | [middleware.go](../internal/server/middleware.go) / [password.go](../internal/auth/password.go) |
| Medium | Mongo `$regex` ReDoS | `regexp.QuoteMeta` 转义用户输入 | [mongostore/material_repo.go](../internal/storage/mongostore/material_repo.go) |
| Medium | JWT 不可撤销 | **token_version 机制**：logout 递增版本，userMW 校验 → 旧 token 立即失效；默认 TTL 30 天→7 天 | [token.go](../internal/auth/token.go) + 4 存储后端 |
| Medium | SECURE_COOKIES 默认 false | production 未显式设置时默认 true | [config.go](../internal/config/config.go) |
| Low | mood IDOR | `MarkExerciseCompleted` 加 `session_id` 谓词（接口 + SQL + Mongo + handler） | — |
| Low | agent memory 无长度上限 | tool 侧 clamp 到 500 字（对齐用户端） | [tool_memory.go](../internal/server/tool_memory.go) |
| Low | client 可注入 system role | companion 历史 role 白名单（仅 user/assistant） | [handlers_ai_companion.go](../internal/server/handlers_ai_companion.go) |
| Low | health 泄漏 DB 错误 | 泛化为 "database unavailable" + 服务端日志 | [handlers_misc.go](../internal/server/handlers_misc.go) |

**审计确认无问题的面**（有证据）：密码哈希（argon2id + pepper + 恒定时间）、JWT 无 alg-confusion、cookie HMAC 签名、匿名→登录 claim 不可劫持、SQL 全参数化、agent 工具严格限定当前 session、决策卡不可跨 session 响应、无 SSRF、静态服务无路径穿越、prompt 模板注入受限（`text/template` 无 FuncMap）。

---

## 四、批次 B — P0-P2 功能修复

### 撤销四路全坏（最高优先）
`handleOpRevert` 把 `Detail` 解码成 `{Before,After}`，但 rule_delete/rule_batch/memory_delete/memory_clear 的生产端存的是**裸快照**，导致 `Before/After` 恒为 nil。根因是生产端漏包裹（revert 端逻辑本身正确）。修复：4 处生产端补 `{before,after}` 包裹。另修 **omitempty 不对称**——`TimeBlock.Completed/IsAchievement/DurationMin` 去掉 omitempty，使「标记完成」「设时长」可被撤销。

### 登录合并丢数据
`mergeSessionData` 只迁移 5 类表。补全 **materials/wishes/themes/courses/assignments/chat 线程+消息/companion/session 设置**；`ruleMergeKey` 从 `Title|Kind|Freq` 补齐到 `(title,kind,freq,interval,time,by_weekday,date)` 全字段；合并 facts 保留 `Type`。feedback（无 List 接口）和 temp_context（TTL 临时）注释说明不迁移。**顺带修了 Mongo `userDoc` 缺 `DataSessionID` 字段**——mongo 后端下 canonical session 此前完全不工作。

### 其他
- **chat 实时对话持久化**：`runCompanionAgent` 改为返回最终答案文本，handler 成功后 `AppendMessages([user, assistant])`。
- **分页 tied-timestamp**：同批消息给递增时间戳（两后端），`before` 加数字校验；auto-import 消费 legacy 源防「删空后复活」。
- **上下文压缩**：`TrimLeft`（cutset bug，会吃英文首字母）→ `TrimPrefix`；开环 facts 去重；summary 回读注入；压缩输入排除 system prompt；post-compaction 首条保证是 user（防孤儿 tool）。
- **rule PATCH 先写后验证**：新增 `applyRuleUpdate` helper，持久化前验证合并结果（HTTP + tool 两处）；`TimeMode:"local"` 加入合法值。
- **matchesAll panic**：`reflect.DeepEqual` 替代 `!=`，防非可比较类型 panic。
- **agent 重复 done 帧**：wrap-up 失败时不再二次发 done。

**回归测试**：`revert_test.go`（rule_delete/memory_delete revert 恢复、rule PATCH 拒绝非法且不损坏）。

---

## 五、批次 C — 时区三模式实装

`timeutil.ToUTC/FromUTC` 本身正确（`ParseInLocation`/`In` 处理 DST），只是从没被调用。新增 [timezone.go](../internal/server/timezone.go) 的 `fillBlockUTC`：
- `floating`：跟墙钟，不锚定（无 `utc_time`）
- `fixed`：绝对时刻，存 UTC 锚点
- `local`：生理节律，跟当前时区，也存锚点

接入保存路径（upsert + patch）和读取路径（GET/range 惰性回填旧数据）。测试覆盖 DST 边界（US Eastern EDT/EST）+ fillBlockUTC 语义。

---

## 六、批次 D — P3 通道 + 主动性接线

根因：整块从不接线。修复：

1. **main.go 接线**：创建 Registry → 配置了 `ONEBOT_WS_URL` 则注册 OneBot → 创建 Worker → `StartAll` + `Start` + **inbound 消费循环** → 对已绑定 session `ScheduleUser`；config 补 `ONEBOT_WS_URL/ONEBOT_TOKEN/WORKER_DEFAULT_TZ`。
2. **inbound → agent 桥**：新增 [channel_agent.go](../internal/server/channel_agent.go)——`HandleInbound` 解析绑定 → `runChannelMessage`（用 `nullResponseWriter` 复用同一 agent loop）→ 回推。
3. **bind→verify 修复**：重构绑定接口——`GetPendingByToken`（查未验证 token 行，此前查询含 `verified_at IS NOT NULL` 永远查不到）+ `Promote`（设真实 external_id + verified_at）+ `ExpirePending`（sweep）。**顺带修了 `ListBySession` 遇 NULL verified_at 崩溃**（`sql.NullInt64`）。
4. **cron 修复**：`cronScheduleAt` 用 `CRON_TZ=` 前缀——**同时**修了时区（按用户 tz 触发）和 6 字段 spec 解析失败（缺 `WithSeconds()` 导致晨/晚简报被静默丢弃）。
5. **OneBot 群 @ 过滤**：`strings.Contains("@")` → 精确匹配 `[CQ:at,qq=self_id]`；频控 map 加驱逐（>60s 清理，防无界增长）。
6. **GapSuggestions** toggle 生效（并入晨简报 prompt）；**feedback** 接入 admin stats（此前只写不读）。

**回归测试**：`channel_test.go`（完整 bind→verify→promote 流程，此前不可能成功）。

---

## 七、批次 E — P4 补完

| 项 | 修复 |
|----|------|
| **Searcher** | `MaterialSearcher`（跨方言子串匹配，接 `Deps.Searcher`）——`/materials/search` 不再 501。原生 FTS5/tsvector/FULLTEXT/text 索引作为文档化的后续优化，Index/Deindex 留作 seam |
| **TempContext 塌缩** | 空 ID 导致 `ON CONFLICT(id)` 塌缩单行 → 改 **UPDATE-then-INSERT keyed on (session_id,key)**（跨方言安全，MySQL 无 ON CONFLICT）；GET 加 TTL 过滤；缺失键返 `{data:null}` 而非 500 |
| **inbox** | upload 补 requireSession + 限速 + 持久化到 temp_context（小文件 base64，1h TTL）；process 支持 `temp_id` 读回；非文本引导走 vision |
| **Wish JSON** | 补 camelCase json tag（此前无 tag → PascalCase，破坏前端） |
| **wish↔mood 联动** | active wishes 注入 companion L3 上下文，AI 可主动建议 |

**回归测试**：`p4_test.go`（TempContext 多 key 隔离 + 覆盖 + 过期；Searcher 命中/未命中）。

---

## 八、批次 F — WeatherProvider 适配器

按 §2.5 适配器铁律（对齐 `ai/registry.go` 模式）重构：

```
domain/weather.go     WeatherProvider 接口 + WeatherQuery + Forecast + Day + Summary
internal/weather/
  weather.go          registry(Register/build) + New(Options) + chain(fallback) + cached(30min)
  openmeteo/          迁移现有 Open-Meteo（默认，免费，含 WMO 码映射）
  qweather/           和风天气（需 key，geo→3d 预报）
  openweathermap/     OWM（需 key，geo→3h 聚合成日）
  wttrin/             wttr.in（免费无 key，兼作 fallback）
```

- **注入**：Server 依赖 `domain.WeatherProvider`（`Deps.Weather`），tool_external + worker 用接口。
- **fallback 链**：主 provider 失败 → wttr.in（照 search 包 Tavily→DuckDuckGo 模式）。
- **配置**：`WEATHER_PROVIDER`（默认 open-meteo）+ `QWEATHER_API_KEY` / `OPENWEATHERMAP_API_KEY`；无 key 的 provider 工厂返回 nil，自动回落免费默认。
- **install 向导**加天气 key 询问 + `.env` 生成。
- **测试**：4 provider 各 httptest 夹具 + chain fallback + cache dedup。

---

## 九、批次 G — plan.md P5/P∞ 重构

第十章「P5+ 头脑风暴」拆成：
- **十、P5 实际计划**：多语言 Phase 1（ja/ko/es/fr/de）/ STT（QQ 语音→agent，走 STTProvider 适配器）/ PWA
- **十一、P∞ 愿景**：文生图 / TTS+语音克隆 / 数据交叉分析 / 社区市场 / 评测 golden CI / 自由插件 / 原生全文搜索 / CalDAV / 每用户时区

§2.5 适配器表更新（WeatherProvider ✅、STTProvider→P5）；环境变量表补齐；第九章加「验证补完批次」小节；记忆 `daycore-vnext-plan.md` + `daycore-codebase-audit.md` 修正「P0-P4 ✅」的过度乐观。

---

## 十、批次 H — 前端契约

产出物在 `api/`：

- **`openapi.yaml`**（OpenAPI 3.1，重写）：全 **93 操作 / 53 路径**，完整 components/schemas（TimeBlock 加 local + utc_time、Material/Wish/ChatThread/Forecast/SSE 帧/AdminStats 等）。此前只覆盖 39 路由（42%）且有缩进 bug + 明确 punt 了 ~40 路由。Python 验证 YAML 合法（redocly 因离线跳过）。
- **`FRONTEND_HANDOFF.md`**（外科式修正）：保留原有设计价值（tokens/六界面/i18n/记忆/主题），顶部插入权威的 **v2 协议章节**——修正审计发现的致命错误：旧文档描述的是 v1 的 `data:{"delta"}`+`[DONE]` SSE 和 `<plan_update>`/`<rule_update>`/`<memory_update>` XML 标签，实际是 **v2 tool-calls 帧协议**（`{"type":...}` 帧 + `{"type":"done"}`，7 种帧类型 + 决策卡阻塞时序）。

> ~~`web/FRONTEND_HANDOFF.md` + `web/api/openapi.yaml` 是更旧的重复副本，未删除（避免破坏未知引用）；建议后续删除。~~
> **已处理**：两份旧副本均已不在树中，`web/` 下只剩 `frontend/`。**权威版本是 `api/`**。

---

## 十一、关键工程决策

| 决策 | 取舍 |
|------|------|
| 撤销修复方式 | 改生产端补 `{before,after}` 而非改 revert 端——revert 端逻辑本就正确，且与其他 op 一致 |
| JWT 撤销 | token_version（每 user 一个整数）而非无状态；userMW 查 user 顺带校验，dataSessionMW 复用（净查询数不变） |
| dev fail-safe | dev 默认绑 loopback 而非拒绝启动——不破坏本地开发，但不无意暴露弱密钥/开放 admin 到网络 |
| Searcher | LIKE 跨方言实现让 search 立即可用（修 501），原生 FTS 索引作为文档化后续优化——诚实标注，非静默降级 |
| TempContext | UPDATE-then-INSERT 而非 ON CONFLICT——跨方言安全（MySQL 无 ON CONFLICT） |
| cron 时区 | `CRON_TZ=` 前缀一举两得（时区 + 字段数），无需 per-user cron 实例 |
| 天气 fallback | wttr.in 免费无 key 天然兜底，无论主 provider 是谁 |
| inbox AI 理解 | 保留关键词理解 + 打通 upload→process 管线，AI 理解标注为后续增强（聚焦审计核心：丢文件 + 无鉴权） |

---

## 十二、测试与验证

- **单元/集成**：新增 `revert_test.go` / `channel_test.go` / `p4_test.go` / `timezone_test.go` / `token_test.go` / `convert_test.go` + 4 个天气 provider 测试。补了此前零覆盖的 `timeutil`、撤销、时区、通道绑定、TempContext。
- **全量**：`go build ./...` + `go vet ./...` + `go test ./...` 全绿。
- **端到端冒烟**（真实 HTTP，sqlite）：
  - 健康检查 ✓ / P3 接线不崩 ✓
  - fixed 块锚定 `utc_time`（09:00 上海 → 01:00 UTC）✓
  - TempContext 多 key 隔离 + 缺失返 null ✓
  - Searcher 命中（此前永远 501）✓
  - Wish JSON camelCase（此前 PascalCase）✓
  - ops 记录 + 撤销正常 ✓

---

## 十三、已知遗留（记入 P∞）

- 原生全文搜索索引（当前跨方言子串匹配）
- inbox AI 理解（当前关键词匹配）
- 每用户时区（session 无 tz 字段，worker 简报用 `WORKER_DEFAULT_TZ` 全局默认）
- worker per-user cron 的规模优化（大量用户时应改批处理扫描）
- `web/` 目录的旧契约副本待删除
- version 常量仍是 `2.1.0-beta`（`internal/version`），plan.md 基线写 2.2.0——升版时同步
