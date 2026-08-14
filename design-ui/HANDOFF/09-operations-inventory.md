# Daycore design-ui · 用户操作与逻辑清单（供与 Kimi 前后端对齐）

> 用途：把 design-ui 四个前端（+ 共享数据核心）的**全部用户操作与逻辑**逐条列清，供 Kimi 的前端子仓实现逐一核对覆盖。
> 说明：共享核心语义（石化/撤销/锁级别/提案/胆量/意图路由）已落在 Go 后端 + packages/core；本清单聚焦「用户能做什么 + 背后的逻辑」，前端实现应对齐。

## 0. 共享核心语义（所有前端都必须成立的「骨头」）

### 石化（读时计算，无后台 job）
- 阈值 = min(now − 5h, 今天 00:00)；早于阈值的块 phaseOf 返回 'stone'。
- 四态：future / now（正在）/ recon（已过去但阈值内，可标记）/ stone（只读）。
- stone 块：不能拖、不能改时间、不能改状态；只能「注释」或「重新安排」（在未来放新的 + 提案）。

### 撤销 = 反向补偿（不是快照回滚）
- 每种写操作都有逆操作；undo(opId) 按 payload.type 分派（add_block/remove_block/move_block/status/set_mark/set_fields/reconcile/note/bulk_blocks/assignment_done/add_wish/add_mood/add_material/add_memory/remove_material/remove_memory/proposal_accept/proposal_reject）。
- bulk_blocks 一批块 = 一条可撤销 op。
- 撤销本身也记一笔 op（undoable:false）。agent 动作被撤销 → 胆量分降。

### 锁级别（deriveLock 建块时自动推断）
- appointment 且标题匹配 /（课）|课$|课程|讲座|实验课|导论|Lecture|Labs*课/ → hard（理由「课程时间由课表决定」）。
- 其余 appointment → soft（「和别人约好的时间」）；task 类 → null。
- moveBlock：stone 往后挪 → {stone:true}；hard+非AI → 拒绝；soft+非AI+未 _confirmed → 二次确认；agent=true 可越锁。
- UI 三层表达：静态可见（锁/图钉）、触觉反馈（拖 hard 只跟手 7px→回弹 440ms+12ms 震动）、事后解释+替代动作。

### 提案（AI 的一切建议都是提案，虚影）
- state: pending/accepted/rejected/expired；level L1/L2/L3；kind timed（画轴虚影）/card/decision（rows 逐条）。
- accept/reject（可整张或逐行）；ttlRule silent_accept（没理就当接受）/silent_reject（没理就作罢）。
- 过期可 redeem 捞回；followUp 生成 10 分钟 exchange 上下文（追问）；consumeExchange 在伙伴里消费。
- 接受/拒绝 → 胆量分（boldness）升降，决定 pushBudget 主动打扰门槛。

### 意图路由 captureIntent(text)（一句话 → 直接执行/提案/记忆/愿望/资料/查询）
- 查询（?/查/截止/ddl/什么时候…）→ 浮层回答（不落库）。
- 带时间（parseWhen）→ 直接 addBlock（guessType/guessDur）。
- 心情词 → moodRecord；压力源顺带记 open_loop。
- 想/改天/想学… → wishAdd（guessEffort，去重合并）。
- 我总是/效率/点后… → memoryAdd(preference)。
- 老师/教室/重点/图书馆… 或 ≥26 字 → materialAdd。
- 兜底 → wishAdd。

### 语气铁律
- 不变红/不积债；「没做（没关系）」；空状态「不是坏事——说明还没人替你着急」；「给死路一条岔路」（每处不能做都配替代动作）；任何 AI 动作 3 秒内可撤。

## 1. 汀 · 此刻（ting）—— 单件流

### 状态机（faceKey）
brief（导语）→ prop（提案整屏）→ cur（当前块）→ gap（空档）→ done（收尾）→ empty（空白）。

### 操作
1. **上滑**（当前块）：setStatus done + 撤销条；若当天无心情 → 弹心情帧。阈值 -90px。
2. **上滑**（导语）：dismissBrief。
3. **提案右滑**（dx>90）：accept；**左滑**（dx<-90）：reject。
4. **下拉把手**：打开全天 peek（今日迷你轴：块+虚影+现在线）。
5. **当前块按钮**：「完成了」（complete）；「没做/换一个…」→ act sheet。
6. **act sheet**：「没做，先送走」（setStatus missed）；「换一个——推给明天」（moveBlock +1d）；「移走」（removeBlock）。
7. **提案按钮**：「就这么办/全部接受」（accept）、「推开」（reject）、「追问」（followUp→companion hint）、「先不看」（skipAll，可「再看看提案」恢复）；decision rows 逐行 ✓/✗。
8. **心情帧**：6 个快捷心情（😊开心/😌平静/🤩兴奋/😐一般/😪疲惫/😣压力大）→ moodRecord；「跳过」。
9. **Peek 底部**：足迹·账本 / 展望·雷达。
10. **账本 sheet**：ledgerByDay(4)；每条 op 可撤销；过期提案「捞回」。
11. **展望 sheet**：radar（截止清单，urgency 分级）；愿望「达成」/「放下」。
12. **菜单 sheet**：主题 night/dawn；演示场景 healthy/crisis/cold；节律展示；跳转其它前端。
13. **收集胶囊**：文本 → captureIntent（answer 走浮层）；附件 → materialAdd；快捷提示 4 条。
14. **推送条**：markPushRead。
15. **顶栏**：时钟、「悄悄做了 N 件」（l0Today→账本）、心情、菜单。
16. **键盘（桌面）**：Enter/↑=主动作、Esc=推开、↓=全天；输入框内不拦截；keyhint 仅桌面。

### 逻辑要点
- queue = stack() + ghostsOf(今天)，过滤 pending 且非 skipped。
- 撤销条 TTL 5200ms。
- 汀刻意不带聊天史（追问 sheet 说明：切到姊妹端「伙伴」）。

## 2. 纸屿 · 顺流（zhiyu）—— 叙事流

### 操作
1. **滚动 = 时间旅行**；「回到现在」按钮（scrollTop 计算，禁 scrollIntoView）；「展开更早的 N 天」。
2. **块条目**：点 ⋯ → EntryMenu。
3. **EntryMenu**：
   - 完成了（setStatus done）
   - recon/stone 且未 reconciled：记完成了 / 记没做（没关系）/ 其实做了（reconcile）
   - hard 锁：说明行 +「请假/不去」（prefill）+「标记冲突」（setStatus planned）
   - soft 锁：挪到明天（含确认层，原地换 confirm 非系统 confirm）+「一句话改期」（prefill）
   - 注释（note）；stone → 重新安排（proposeReschedule）；非 stone → 移走（removeBlock）
4. **块内联**：now 阶段「完成了」；recon 阶段「完成了/没做」chips；note 输入（回车保存）。
5. **提案便签**：accept/reject（整张+逐行）、「追问」（followUp→伙伴）。
6. **凋零卡**：「捞回」（redeem）。
7. **心情检查**：快捷心情 → moodRecord。
8. **输入条**：文本 → captureIntent（executed 滚动到新块）；附件 → materialAdd。
9. **Rail 边栏**：l0Today 静默数；心情检查。
10. **侧 tab**：伙伴 / 资料 / 展望 / 设置（面板）。
11. **资料面板**：资料列表（学业/生活 tag、移出）；记忆列表（偏好/惦记、忘掉）；上传附件。
12. **展望面板**：radar 截止；愿望 add（去重）/达成/放下。
13. **伙伴面板**：chatSend；附件；consumeExchange（追问上下文 10 分钟）。
14. **设置面板**：
    - 纸色（4 内置：格线纸/行线本/牛皮纸/炭黑纸）；AI 生成纸色（aiTheme）；自定义纸色 应用/重命名/删除（二次确认）
    - 助手名字（patch assistantName）；自定义提示词（patch l2Prompt，2000 字）
    - 主动关怀 7 开关（morningBrief/eveningReview/deadlineAlerts/rollingReplan/gapSuggestions/autoPlan/doNotDisturb）
    - 通知渠道（QQ/Telegram bind/verify/unbind）
    - 记录类别（listCategories/setCategory，note 锁定）
    - 作息节律（rhythm 钉住/取消钉住）
    - 演示场景（scenarioSet）；数据（重新播种）

### 逻辑要点
- compose() 把账本渲染成流：块 + 虚影 + 提案 + 心情 + ops；块级 op（add/move/status/reconcile/note/remove/add_mood）HIDE；静默 op 折叠成 L0 组（150 分钟窗口/跨日断开）。
- 未来卡更淡（opacity 递减），过去卡饱和度更低。
- 撤销条 TTL 5600ms。

## 3. 琉璃 · 长卷（liuli）—— 时间画布

### 结构
五席书签（资料/足迹/今天/展望/伙伴）+ 设置；恒定底部输入框（语境注入 agent）；镜头切换（日/周）。

### 操作（画布 DayCanvas）
1. **拖拽块**：起拖 >5px；松手落点 snap5 到 5 分钟；上下移动即 moveBlock（改 start）。
2. **长按 460ms**（未移动）→ 块浮层 + 12ms 震动；期间移动 >5px 取消长按转拖拽。
3. **桌面右键** → 同一浮层（preventDefault + 取消拖拽）。
4. **hard 锁块拖拽**：只跟手 max(±7px)，>10px 触发一次 .tug 回弹 440ms + toast 说明原因+替代动作。
5. **stone 块拖拽**：>14px 触发 toast「过去的记录不改写——用重新安排」。
6. **引力井 2×2**（拖动时显形）：
   - 左上 archive「资料收好」→ materialAdd + removeBlock
   - 左下 park「想做的事/改天再说」→ wishAdd + removeBlock
   - 右上 next「交给明天」→ moveBlock(+1d)
   - 右下 rearrange「小禾重排」→ proposeReschedule
   - 命中判定：手机内缩 -10px，桌面放宽 +18px。
7. **打勾圈 .cj-tick**（26px 二态，onPointerDown stopPropagation+preventDefault）：空→✓完成→空（setMark none/done）。
8. **块浮层 BlockPop**：
   - 虚影（ghost）：accept/reject/追问
   - hard 锁：完成了/请假/标记冲突
   - 非 hard：完成了/放到明天（soft 二次确认）/＋30 分钟/移走
   - 标记区：完成了/没做（没关系）二选一（与打勾圈同一套状态）
   - 过期未标记（recon+planned）：「它该去哪儿？」三选（放回想做的事/明天再排/就这样过去）
   - stone：重新安排
   - 注释输入
9. **周透镜**：7 列色条（块+虚影）；点某天 → 进入日画布。
10. **顶栏**：日期导航（←/→ 天/周）、回到今天、周/日切换、心情胶囊、时钟、头像菜单。
11. **头像菜单**：主题（4 内置 + 自定义）、演示场景、进入设置、回门厅。
12. **提案堆叠 Stack**（L2 · ≤3，右下露出下一张）：接受/拒绝/追问/逐行 ✓✗。
13. **心情胶囊 MoodCapsule**：12 心情格 + 备注 + 回声文案；已打卡可更新。
14. **推送 chip**：markPushRead；预算提示。
15. **查询浮层 AnswerSheet**：question → 浮层 + 「去看看」跳转。
16. **书签视图（抽屉 Drawers）**：
    - 资料：上传文件、资料列表（移出）、记忆列表（忘掉）
    - 展望：radar 截止、明天一瞥（块+虚影，点击跳转）、愿望（add 去重/达成/放下）
    - 足迹：远景河（15 天 river）、节律（钉住）、周信说明；近景账本（ledgerByDay(7)，折叠静默，撤销/捞回）
    - 伙伴：聊天史（consumeExchange、工具卡撤销、提案 rows 逐行）
17. **设置 SettingsPage**：
    - 主题工作室（4 内置 + AI 生成 + 自定义右键/长按 重命名/AI 改一版/删除）
    - 助手名字（patch）、自定义提示词（patch，2000 字）
    - 语言（演示版仅中文）
    - 主动关怀 7 开关
    - 通知渠道（bind/verify/unbind）
    - 记录类别（note 锁定）
    - 记忆（补一条/忘掉/清空全部——逐条进账本）
    - 演示与数据（场景切换/重新播种/管理台入口/回门厅）
18. **键盘**：today 视图 ←/→ 翻日；Esc 关菜单/浮层。

### 逻辑要点
- laneize：块+虚影一起算重叠层叠（左 46+lane*18）；虚影 zIndex 14+lane、不透明底+描边。
- 「现在」线均匀 1.5px 实线。
- h<48px 进 compact（隐藏时长/标签）；标签 ≤3，nowrap+overflow。
- 空状态「今天还是空的——不是坏事」。
- ≥1100px dock 变左侧 228px 竖栏。

## 4. 琉璃初版（liuli-classic）—— 页面制（对照组）

### 壳（main.jsx）
- 5 tab（今日/资料/陪伴/心情/设置）：<900px 底部 TabBar，≥900px 左侧 rail。
- 顶栏问候（按时段 greet_morning/noon/…）；头像 → AccountSheet（登录/账户）。
- TweaksPanel（开发调参）：规划面板 brief/plain、显示 AI 备注、显示 AI 置信度。

### 引导（page-onboarding.jsx，未 onboarded 时拦截）
- 3 步：欢迎 → 导入（Canvas/ICS/课表截图，可跳过，importCanvasDemo/saveRulesBatch）→ 首次自动规划（autoPlan keep_manual，可跳过）→ setOnboarded。

### 桥接层（store-bridge.js —— 核心为唯一真相，旧模型投影）
- 投影：blocks→plans（今天±21 天，字段 time/duration_min/completed/missed/locked/lockReason/time_mode=hard?fixed:floating）；作业+课程（课程从作业 course 字段归纳）；心情→{mood:e+' '+l, aiResponse:note}；记忆→keyFacts；主题/助手名/语言 ← 核心 state 根。
- 写操作改道：patchPlan→addBlock/setMark/setNote/setBlockFields/moveBlock/removeBlock；autoPlan→gapsOf+addBlock(agent)；addMood→moodRecord；patchAssignment→setAssignmentDone；patchSession→setTheme/patch；主题四件套→核心；addMaterial/deleteMaterial/memory 系列→核心。
- 时间未变不产生 move op：patchPlan update 里 ch.time !== cur.start 才 moveBlock。
- 锁：moveBlock 返回 locked 时透传 {locked,reason}。
- 回声防护：echo 标志 + guard() 包裹，写时避免自触发，finally syncAll。
- 规则（核心无 rule 实体）：列表 = 归纳 ∪ 自存。
  - 归纳：扫未来 28 天按标题聚合，≥2 次或带锁 → 一条，managed:true（不可写，patchRule/deleteRule 返回 {error:'managed'}，UI 显示「由共享账本管理」）；≥20 天 → daily。
  - 自存：localStorage 'daycore-liuli0-rules'，managed:false；createRule/saveRulesBatch/patchRule/deleteRule；spawn（铺未来 28 天，跳过同名）与 despawn（撤掉含今天的 planned 同名块）都走 bulkBlocks = 一条 op；停用只清占位、保留记录（29→0→29 幂等）。

### 页面（详情由子代理补全）
- 今日：周条 + 自主规划大按钮 + 手动添加 + 块卡；≥900px 右侧日历 + 临期作业。
- 资料：随手记收集箱 + 三入口导入 + 记录/学业/数据源 tab + 规则列表。
- 陪伴：对话 + 工具调用卡 + 决策卡。
- 心情：12 情绪格 + 最近打卡。
- 设置：助手名 / L2 提示词 / 主题工作室（含 AI 生成）/ 管理台入口。

## 5. 运营台（liuli/admin）

### Gate
- X-Admin-Token 登录（演示 token daycore-admin）；所有管理请求仅该头鉴权。

### 导航（8 项）
总览 / Prompt 管理 / 模型 / OAuth / 服务配置 / AI 日志 / 用户 / 数据库。

### 总览 Overview
- 统计卡：注册用户 / 会话总数 / AI 调用 / Token 消耗 / 反馈有用率。
- 服务状态（healthz：版本/channel/契约/环境/uptime/DB）。
- 快捷操作：下载数据库备份、导出全库 JSON、导入 JSON（合并写入）。
- 最近 AI 调用表（时间/类型/模型/Tokens/耗时/状态）。

（页面级详情：模型/OAuth/config/logs/users/db 由子代理补全）
