# Daycore 功能全矩阵 · 重新论证（2.4 批次）

> 依据：三路审计（配置死代码 / 路由桩 / 原型→后端覆盖）+ 用户裁决。
> 用户三条定调：①「没做（没关系）」删除，不要明说没做，过时重捞可以但要给体验；② river/周信/晨卡/愿望 dropped 要做；③ 刻意取舍重新论证。

## 一、总论

后端不是「大量死代码」：43 个配置字段全活、149 条路由里 142 条真实现、领域骨架（石化/锁/提案统一资源/反向补偿撤销/节律/心情/主题/通道/agent 15 工具）都比原型更完整。真实问题是**一份明确清单**：1 个误导性桩 + 4 个该做的功能 + 若干「原型有、后端无」的取舍待重新论证。

## 二、已实现（防误判为缺口）

- 石化四态读时计算（5h 阈值）；锁三层（hard/soft/none + 触觉反馈）；提案统一资源（L1/L2/L3 × timed/card/decision × TTL 不对称 × 逐行 respond）。
- 撤销 = 反向补偿（append-only 账本 + registerRevert/registerIrreversible）。
- proposeReschedule → 提案虚影（POST /api/plan/reschedule，accept 才落 refish 链）。
- 节律 GET + pin；心情 12 格目录（moodKinds，唯一真源）；主题 CRUD/AI 主题；材料类别 note 锁定；通道绑定；作业四态。
- weather 入简报；主动关怀 7 开关；助手名/persona(2000)/语言；agent 15 工具（含 plan_add 的 rescheduled_from）。
- 登录合并：幂等守卫 + assistantName/language/timezone/themeByFamily/locale 并入 + mood/oplog 时间戳保留 + proposals/ops 并入。

## 三、未实现 / 本轮要做（用户裁决「做」）

| 项 | 状态 | 说明 |
|---|---|---|
| 愿望 dropped 态 | ✅ 已做（域常量已加，端点本就接受任意 status） | 只差前端接 |
| river 河 15 天色带 | 🚧 子代理实现中 | GET /api/river：每天 ops 计数 + 当天心情 emoji |
| 周信 | 🚧 子代理实现中 | 周日晚 AI 散文信，GET /api/weekly-letter |
| App 内晨卡/导语 | 🚧 子代理实现中 | GET /api/brief：从当天计划推导，读时不调 AI |
| admin 删用户级联 | 🚧 子代理实现中 | 真级联 + 诚实返回（不再是误导 200） |

## 四、已修掉的 bug（本轮）

- 删死代码 weather.OptionsFromEnv()（零调用者，绕 Settings 门）。
- travel + auto-plan 补 requireAI 守卫（AI 未配时 503 而非 panic 500）。
- 修 DefaultChatConfigured 对「无 api_key_env 的本地/测试模型」误判成未配置。
- 补 .env.example 三行（PROVIDERS_CONFIG / THEME_KINDS_DIR / MAX_UPLOAD_BYTES）。
- （待）admin 删用户误导 200 → 子代理做真级联。

## 五、刻意取舍 · 重新论证

### 1. missed / reconcile 三态（「没做（没关系）」）→ 【砍，用户已定】
用户明确定调：不要明说「没做」，提醒没做体验不好，不如提醒「要做什么」。结论：块保持 `Completed bool`（无 missed 态），前端**删掉「没做（没关系）」标签**；过时块走「它该去哪儿」的过时重捞（proposeReschedule / refish / wishfill 填空档），语气是「现在做什么」不是「你漏了什么」。**后端无需加字段，前端删标签即可。**

### 2. redeem 捞回（过期提案复活）→ 【维持砍】
提案生命周期终态（accept/reject/expire），复活价值低。用户说的「过时重捞」是块不是提案。维持砍。

### 3. followUp 追问 / consumeExchange → 【维持砍，但补一句】
追问 = 打开伙伴面板 + 预填「为什么这样安排」。伙伴对话就是媒介，无需独立端点。维持砍。

### 4. l0Today 静默数 → 【维持 actor=agent 近似】
「悄悄做了 N 件」用 ops(actor=agent) 当日计数会高估（含非静默 agent op）。给 operation_logs 加 silent 字段是 schema 变更，价值低。维持近似，清单标注「含非静默 op，高估」。

### 5. 推送条 markPushRead → 【维持砍】
关怀/截止通知 = 提案栈（L3 care card）。琉璃已把 PushChip 改成「待点头提案入口」。维持砍。

### 6. bulkBlocks（一批=一条可撤销 op）→ 【维持砍】
规则铺开走 Worker，前端 confirmDraft 是 N 次 patchPlan，撤销粒度从 1 条变 N 条可接受。维持砍。

### 7. captureIntent 关键词路由 → 【维持：agent 多工具路由】
mock 的本地正则路由是 demo 设施。真后端 planFromText（AI 候选确认）+ companion agent 多工具。维持，查询浮层走 planFromText 的 notice 分支。

### 8. admin 模型增删 / OAuth provider CRUD → 【维持配置文件驱动，标记为控制台后续能力】
当前 models.yaml/OAUTH_CONFIG 配置文件驱动，端点只读/只 test。真正的「控制台里加模型/加 OAuth」是部署级能力，排进控制台重做（Kimi）的后续，不进 2.4。

## 六、还需要什么（后续批次候选）

1. 前端三块 UI（后端已就绪）：资料页真上传（POST /api/files 原始字节）、四端聊天附件（attachmentIds）、Canvas 插件暴露（import token 入口 + 装插件引导）。
2. 控制台按 design-ui/liuli/admin 重做（Kimi 已接活）。
3. 若以后要「重捞过期提案」，加 POST /api/proposals/{id}/redeem（一个小端点，随时可加）。
4. l0Today 精确化（operation_logs 加 silent 字段）——非必需。

## 七、Mock-only（不进产品，标 demo 专属）

scenarioSet 三种子/重新播种、门厅、演示 token daycore-admin、admin 15 表演示数据、import*Demo 固定候选、呼吸/伸展/着地练习本体、store-bridge 桥接层（echo-guard/managed 规则降级）。

## 八、后端已实现 · 矩阵补漏（写了但前面章节没列的）

> 防误判同 §二：这些都是真实现、有测试盯着，不是桩。

- **Worker 六 cron**（节律派生时刻）：PlanAt（Wake−3h30m，⚠️ 至今无消费者——定时 auto-plan 作业不存在）、BriefAt（Wake）、ReviewAt（Sleep−90m）、deadline 巡检、节律学习（日切）、Protector 20h 关怀。每会话 6 条，job_runs 唯一索引互斥。
- **决策卡**：纯内存 registry，每会话一张，新卡顶旧卡（进程重启即丢，单实例假设）。
- **SSE v2 帧协议**：delta/reasoning/tool_start/tool_result/decision_card/error/done + 心跳；tool_result 带 opId 供撤销。
- **导入三家**：Canvas（token 生成/旋转 + extension MV3 直推）、ICS 文件、截图（视觉管线三分支：自带 vision / 转 vision 模型 / read_image+zoom 工具循环 ≤6 轮）。
- **files + blob 总线**：POST /api/files 原始字节（Content-Type 头，不走 multipart/base64——省膨胀与解析），attachments 表所有权 + blob 字节分离，Ref 永不出服务端。
- **inbox（3 条）/ 长期记忆（5 条）/ temp-context（会话级 KV，2 条）/ feedback（1 条）/ chat threads（7 条）/ companion history（2 条）**：全部真实现。
- **schedule rules 5 条**：规则是真实体（CRUD + 一次铺开由 Worker 做），不是原型的「未来 28 天反推归纳」。
- **admin 面 11 组 42 条**：config/frontends/health/models/pairings/prompts/providers/restart/roles/session/stats-users-DB/theme-kinds。备份/导入 DB 明确 501（有意不做，不是 bug）。
- **默契 rapport + 主动性门控 / 心情窗口（趋势+衰减+新鲜度）/ 节律学习**：三个纯函数域，读时派生零存储。
- **搜索**（Tavily→DDG + 资料 FTS/子串兜底）与**天气**（四 provider 注册表 + 30min 缓存，入简报）。
- **auth 面**：注册/登录/登出 + token_version 全设备撤销 + OAuth 提供商 + 演示会话升级合并（幂等守卫 + preferences 六并入）。
- **凭证三轨**：dc_sid（HMAC cookie）/ X-Session-Token / dc_auth（JWT）/ dcimp_ 导入 token。

## 九、四端已实现功能（前端面 · 落文档）

> 依据：09 运营清单逐条核（file:line 证据）+ 验收轮修复。四端各自语言，**不要抽象成通用组件**（除数据层）。

### 琉璃 · 长卷（web/liuli）
- 日画布：块定位/泳道并列（laneize/boxOf）、最小高度 30px 地板、拖拽跟手（PXM 1.05 像素↔分钟）、snap5、石化四态读时色（live/coming/recon/stone）。
- 引力井 2×2（资料收好/交给明天/想做的事/小禾重排）：命中=可见井矩形±pad（触屏 −10/桌面 +18，误触修复 800e1f3），落井动作全可撤。
- 锁三态：hard 拖出 7px 跟手+震动+理由 toast（lockReason 后端 i18n）+ 岔路（标记冲突/小禾重排）；soft 二次确认层；stone 只注释/重新安排（proposeReschedule 虚影，f9a7fc2）。
- 周透镜（点天进日画布）、键盘（←/→ 翻日、Esc 收层）、提案卡（就这么办/不了）、伙伴对话（SSE 流+工具结果撤销）、资料/足迹/展望/伙伴抽屉、节律钉住（B6）、十五天河（自聚合，待换 /api/river）、周信卡（静态，待换 /api/weekly-letter）、登录/账户区（头像菜单）。

### 纸屿 · 顺流（web/zhiyu）
- 叙事流：日分隔（昨天/今天）、展开更早 5 天（expandEarlier）、现在线、时间戳全会话时区（fmtHM tz 化 431ba0a）。
- 心情打卡（此刻卡 + 流上心情卡）、便签提案（就这么办/不了/追问→伙伴预填）、凋零卡（过期提案只读，无捞回——redeem 砍）。
- 捕捉候选确认（planFromText → cand 卡逐条点头）、查询浮层（notice 分支）、ops 流上撤销（逐行）、L0 静默组（actor=agent 近似，标注高估）。
- 设置面板（42 键：记录类别开关/作息节律钉住/主题重命名 patchTheme/界面语言/账户卡+内联登录注册）、rail 五件（伙伴/资料/展望/设置/周信）、一句话改期 prefill（软锁块）。

### 汀 · 此刻（web/ting）
- 单件流三脸（当下/空档/今日毕）+ 换一个…（推明天/移除/解锁/标记冲突/重排岔路）+ 再看看提案（跳过栈重开 aa630cb）。
- peek 全天轴（提案虚影并入）、账本（逐行撤销）、展望、为什么这样安排（理由+证据+去琉璃伙伴真链接 7c857be）。
- 撤销条 TTL 5200ms（7491bec）、胶囊捕捉（planFromText 候选）、心情快捷 6 格（moodKinds 子集）、菜单（水色主题/节律/语言/账户）。
- 简报脸：无端点暂砍，/api/brief 到位后复活（见 §十.加.1）。

### 琉璃初版 · 页面制（web/liuli-classic，对照组）
- 五页（今日/资料/陪伴/心情/设置）+ 桌面 rail；今日页：周条/进度环/提案卡/自主规划/手动添加。
- 资料页 72 键（类别 chips/作业四态/附件物化 createMaterial）；陪伴 SSE 对话；心情页（moodKinds 12 格）。
- 规则页=真端点 CRUD（非原型反推归纳）；设置 45 键（主题/语言/账户 AuthSheet+AccountSheet 双 sheet/? 入口+关于链接）。
-  onboarding：原型有、端点存在（session PATCH）、未做——见 §十.加.7。

### 运营台（web/console）
- 按 liuli/admin 原型重做完毕：token 门 + 侧边栏 8 项（总览/Prompt管理/模型/OAuth/服务配置/AI日志/用户/数据库），全真 /api/v2/admin/* 数据，mock-only 缺口 9 条已列清单待裁决（模型 CRUD / OAuth CRUD / .env 分组+类型化控件 / Prompt 人类名 / Prompt 占位符说明 / AI 日志正文（有意不存）+类型中文标签 / 用户表 App 用户列 / 备份导入 501）。有意取舍：token-only 门（丢了 loginAsMyself 与角色权限编辑）、门副文案改 cookie 描述、删除确认用 AdmConfirm、AI 日志分页页码→加载更早（keyset 游标）、模型屏加「测试」钮（后端有原型没有）。

## 十、前端论证（加 / 砍 / 改 · 我的立场）

### 加（有真后端或用户已定调，排下一批）
1. **汀简报脸复活 → ✅ 已落地**（ting 6556dc5）：/api/brief 接线，午前（会话时区）且今日未划掉时是第一张脸，空日也有卡（「今天还是空的」）；上滑/Enter 划掉记 localStorage 按日，过午自散。
2. **十五天河换 /api/river → ✅ 已落地**（liuli b658ad4 / zhiyu 9ff55b4）：两端自聚合已删，河 15 列含空日、emoji 罩顶，形态不变只换数据源。
3. **周信换 /api/weekly-letter → ✅ 已落地**（同两提交）：有信渲染散文原样，无信保留「周日晚上…」等待文案。
4. **愿望 dropped 三态 UI → ✅ 已落地**（琉璃展望三态+恢复 / 纸屿展望 ordered+放下 tag / 汀 outlook 放下走 dropped；core Wish 类型收敛真三态，替掉前端猜的 archived——core 33cc46a）。
5. **文件上传组件**（一份两吃）：资料页真上传（POST /api/files 原始字节）+ 四端聊天附件（companion attachmentIds）。无死按钮铁律下，这是「端点好了 UI 缺位」的最后一环。
6. **Canvas 插件入口**：设置卡「生成导入 token + 装插件引导」（四端任一即可，建议在 classic——它的受众就是课表/作业党）。
7. **classic onboarding**：端点本就存在（session PATCH 字段齐），原型有完整流程——建议做，它是「第一次见面定语气」的时刻。
8. **classic 过时块「重新安排」**：接 proposeReschedule——classic 是四端唯一没有过时重捞出口的（今天页过时块目前只读）。

### 砍（维持，论证已闭环）
汀折叠静默（无 silent 字段，近似即可）/ 汀推送条（提案栈即通知面）/ classic OCR 与截图直传（mock-only；若未来要做，走截图视觉管线而不是 mock OCR）/ createAssignment/inboxProcess 桥接补丁 / 演示场景与重新播种 / 呼吸练习本体。redeem/followUp/bulkBlocks/captureIntent 关键词路由见 §五。

### 改（本轮已改完，记录论证）
1. **「没做」全删**（用户定调）：汀 ActSheet/琉璃 marks+recon 各钮已删（ting ff8f3ac / liuli bed8bed）。过时块只走「它该去哪儿」——语气是「现在做什么」。
2. **重新安排=提案虚影**（B10）：四端用户侧重排不再直接落块，全走 proposeReschedule——提案虚影铁律在「用户侧/AI 侧」分界上对齐（AI 侧仍可直接 refish）。
3. **session timezone 唯一真相**：四端所有「今天/现在/时间戳」走会话时区（demo 跨时区播种不再空一天），浏览器本地时区只作 tz 空串回退。
4. **井碰撞箱贴可见矩形**（800e1f3）：象限式命中的误触面比井大一个数量级；「数值即产品」，±pad 的 ±10/+18 是原型定稿。
5. **引力井位置/大小维持原型**：桌面 38%/32%、距边 22px、宽 96px、光晕 42px——核对过逐值一致，用户观感疑点实为误触+卡 live 两个真 bug 的叠加。