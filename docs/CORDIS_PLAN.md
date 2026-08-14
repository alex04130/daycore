# Cordis 化改造方案（草案 · 待与 Kimi 讨论定稿）

> 目标：所有能力都是插件、所有配置都能热改；硬编码是硬伤，只有真改不了的才需要重启。
> 现状盘点（2026-08）：仓库已有一半基础设施，不是从零做。

## 一、已有（别再重做）

1. **存储插件**：`storage.Register(dbType, opener)` —— sqlite/pg/mysql/mongo 四后端都是注册进来的。
2. **文件总线插件**：`blob.Register(name, opener)` —— localfs 是注册进来的。
3. **AI wire-format 插件**：`formats/{openai,anthropic,ollama}` 自注册。
4. **外部能力适配协议**（`docs/specs/provider-protocol.md` v1）：
   - 查询型（天气/搜索）`format: http` ✅ 已实现（F2-A）+ 有 `tools/adapter-example` 参考实现 + e2e 真子进程验证。
   - `format: exec`（子进程）⬜ 未实现（但参考实现已在，只是没接进 providers 装配）。
   - 通道型（`format: ws` / `http` / `exec`）⬜ 未实现（只有内置 OneBot WS）。
5. **通道框架**：`internal/channels` Channel 接口 + OneBot 11 适配器。
6. **配置分层**：`internal/config/layer.go` Settings（启动期/运行时/密钥 三层分类）+ `ReloadSettings` 热重载管线。

## 二、硬伤清单（要修的缺口）

1. **硬编码默认助手名 "Leo"**：4 处（三个方言建表 SQL 的 DEFAULT + sessions INSERT + 2 处 orDefault）。→ 提成 `DEFAULT_ASSISTANT_NAME`。
2. **通道列表硬编码 onebot**：`GET /api/channels` 写死。→ 改成读 providers/通道注册表。
3. **providers.yaml 空**：天气/搜索/通道无声明源，落到内置兜底。→ 填默认源 + 支持运行时注册。
4. **5 个「改了要重启」旋钮**：RateLimitPerMin / AuthRateLimitPerMin / DefaultChatModel / DefaultVisionModel / DefaultPlannerModel —— 启动期烘焙进对象，热改只存不生效。→ 改成热重载（重构建 limiter / catalog 指针）。
5. **通道只有 WS（OneBot）**：无 HTTP 通道兼容层、无 `format: exec` 子进程通道。→ 补齐协议里的通道型。
6. **admin 模型/OAuth 只能改文件**：控制台只读。→ 控制台 CRUD（Kimi 的运营台）。

## 三、分阶段方案（每阶段独立可交付、可验收）

### P1 去硬编码（低风险，1 批）
- `DEFAULT_ASSISTANT_NAME` 配置项，替换 4 处 "Leo"。
- 通道列表从 providers/注册表读，不再写死。
- 填 `providers.yaml` 默认源（open-meteo 天气 + DDG 搜索）。

### P2 热重载（中风险，1 批）
- 5 个 notHotYet 旋钮改成真热：ReloadSettings 里重构建限流器 + 重解析 catalog 默认模型指针。
- 移除 `requiresRestart` 那套标注（或只留真正改不了的）。

### P3 补齐插件协议（较大，2-3 批）
- `format: exec` 子进程：把 `tools/adapter-example` 接进 providers 装配，天气/搜索/通道都能用子进程扩展。
- 通道型 `format: ws`/`http`/`exec`：HTTP 通道兼容层（OneBot HTTP 上报/下发）+ 子进程通道。
- 目标是「内置实现和外部适配层走同一接口，上层无差别」（协议文档里已写明这个目标）。

### P4 控制台 CRUD（Kimi）
- 模型增删改、OAuth provider CRUD、provider 源增删改，从「改文件重启」变成「控制台热改」。

## 四、要和 Kimi 讨论定夺的点

1. **优先级**：P1 去硬编码是明确该做的；P2 热重载的 5 个旋钮里，限流×2 容易热改，3 个默认模型要重解析 catalog 指针（涉及并发安全）。这 5 个要不要都热改，还是「默认模型」这类确实少见热改的可以先标「改完下次会话生效」？
2. **通道 HTTP 兼容层**：是补 OneBot 的 HTTP 上报/下发（同一个 bot 走 HTTP），还是只做 `format: exec` 子进程通道（协议层通用）？前端/运营台对「通道」这一屏的期望是什么？
3. **子进程扩展的安全边界**：`format: exec` 启动子进程，需要定「谁来起、超时/重启、密钥怎么传、崩溃怎么办」——协议文档里 transport.md 有没有覆盖，还是需要补？
4. **控制台 CRUD 的范围**：运营台要暴露到哪一层（模型/oauth/provider 三个都做，还是先做 provider）？这决定 P4 的切分。
5. **顺序**：这四个阶段和「2.4 beta 收尾」怎么排？我倾向 P1 先做（小、明确、无风险），P2/P3/P4 等 2.4 合流后再排。
## 五、与 Kimi 讨论后的定稿（2026-08-14）

1. **P2 热重载**：限流×2 热改；三个默认模型标「下次会话生效」（低频动作，不值得为 catalog 指针加并发热换，写进文档当取舍）。
2. **通道 HTTP 兼容层**：不补 OneBot HTTP；直接 exec 子进程通道（P3，协议层通用）。运营台/前端对通道屏只期望：注册表驱动的适配器列表 + 绑定/解绑 + 状态，不加 verify。
3. **exec 安全边界**：transport.md 已覆盖（握手/ready_timeout/SIGTERM 5s→SIGKILL/重启退避/socket 归属/stderr），密钥传递约定「只经环境变量、永不上 argv」也已在 transport.md:118。无需新增。
4. **控制台 CRUD 三步**：先 provider 源 → 再模型 → 最后 OAuth（client secret 最敏感）。
5. **排序**：P1 随 2.4 收尾一起走（已做，见下）；P2–P4 等 2.4 合流后；P4 前压着「运营台 9 条缺口裁决」。

## 六、P1 已完成（随 2.4 收尾）

- `DEFAULT_ASSISTANT_NAME` 配置项：替换 4 处 "Leo" 硬编码（3 方言建表 SQL + GetOrCreate + companion ×2 + channel_agent + merge + worker），并进了 config 分层（boot 层）+ CONFIG.md 重生成。
- 通道列表从 `channels.Registry.List()` 读（新增 Channel.Label() + Registry.List()），不再是写死的 onebot 切片。
- `config/providers.yaml` 填默认源（open-meteo/wttr 天气 + ddg 搜索），不再是空文件。
- transport.md 的 exec 密钥约定已存在（118 行），无需新增。