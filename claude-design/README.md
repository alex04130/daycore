# Handoff: Daycore v2 前端（自主规划版）

## Overview
Daycore 是一个面向学生的「AI 自主规划 + 温和陪伴」App。v2 主线：系统汇总用户资料（Canvas 作业成绩、课程表、重复规则、长期记忆），一键生成当日/多日计划，用户再通过聊天微调。本包是完整的**视觉与交互设计参考**，供开发者在真实代码库中实现。

## 在 Claude Code 中使用（推荐流程）

1. 把本文件夹解压到后端仓库根目录，例如 `design/`（保持文件夹内部结构不变）。
2. 在仓库根目录运行 `claude` 启动 Claude Code（可选：安装 frontend-design 插件）。
3. 粘贴类似下面的指令：

   > 阅读 design/design_handoff_daycore_v2/README.md、FRONTEND_HANDOFF.md 和 api/openapi.yaml。
   > 按 FRONTEND_HANDOFF.md 的技术选型实现整套前端，逐屏 1:1 复刻 design_handoff_daycore_v2 里的 HTML 原型（原型源码在 app/*.jsx，是精确参考）。
   > 硬性要求：zh-CN/en-US 双语（§0）；每用户长期记忆（§7，需要同时扩展 Go 侧端点）。
   > 所有 fetch 带 credentials:"include"，聊天用 SSE 流式渲染并解析 <plan_update>/<rule_update>/<memory_update> 标签。
   > 每完成一屏，启动 dev server 与原型对照后再继续。

4. 想先看原型效果：在本文件夹内起静态服务（`npx serve .` 或 `python3 -m http.server`），浏览器打开 `Daycore App.html`。**不要直接双击用 file:// 打开**（JSX 是运行时加载的，file:// 会被浏览器拦截）。

## About the Design Files
本包中的 HTML/JSX 是**用 HTML 制作的设计参考**（可运行的高保真原型，数据为本地模拟），不是可直接上线的生产代码。任务是：**在目标代码库的技术栈中重新实现这些设计**。`FRONTEND_HANDOFF.md` §1 建议 Vue 3 + Vite + vue-i18n；若选 React 技术栈，可更直接地移植原型组件与设计系统。原型源码（`app/*.jsx`、`app/app.css`）中的布局、间距、颜色、文案、状态逻辑都是精确的实现参考——**遇到本文档未写明的细节，以原型源码为准**。

## Fidelity
**High-fidelity（高保真）**：颜色、字体、间距、圆角、动效、文案均为最终意图，应按原型像素级复刻；数据交互为本地模拟（`app/store.js` + localStorage），真实实现按 `api/openapi.yaml` 对接。

## 硬性产品需求（实现时不可省略）
1. **双语 i18n**（`FRONTEND_HANDOFF.md` §0）：所有 UI 字符串外部化 zh-CN/en-US；错误按 error 码翻译；日期用 `Intl.DateTimeFormat(locale)`。原型的全部文案（两种语言）在 `app/i18n.js`，可直接作为 locale 文件的起点。
2. **每用户长期记忆**（`FRONTEND_HANDOFF.md` §7，新增）：用户上传过的所有内容 append-only 全部入库（Canvas/ICS/截图解析/聊天事实/心情）；prompt 注入记忆摘要；聊天协议加 `<memory_update>` 标签；设置页记忆管理（删单条/清空）；资料页导入历史。需要 Go 侧新增 `GET/POST/DELETE /api/memory`、`GET /api/import/history`。

## Screens / Views

布局骨架（所有页面共用）：
- **移动端（<900px）**：单列，内容 `max-width: 560px` 居中，左右 padding 20px；顶部 sticky appbar（问候语 + 页面标题 + 右侧头像按钮）；底部 fixed 玻璃 TabBar（5 个 tab，`width:100%; max-width:560px` 居中，`padding-bottom: env(safe-area-inset-bottom)`）。
- **桌面端（≥900px）**：TabBar 变为左侧 232px 侧栏（logo + 纵向导航），内容区 `max-width: 640px`；BottomSheet 变为居中对话框（`top:50%; translateY(-50%)`）。

### 1. 引导 Onboarding（`app/page-onboarding.jsx`）
全屏 3 步，底部步骤条（3 段小横条）。①欢迎：呼吸动画光球（88px 圆，primary→accent 渐变，3.2s breathe）+ 标题 + 开始按钮；②导入资料：三张 GlassCard（Canvas / ICS / 截图），点击模拟导入，完成显示 success Badge，可跳过；③一键生成：点击后全屏 GenOverlay（呼吸光球 + 逐条状态文案），完成进入主界面。

### 2. 今日 Today（`app/page-today.jsx`，主界面）
- **WeekStrip**：7 日横条（周一起始），选中日 primary 实底白字，today 数字高亮，日期下有计划状态小点；左右翻周 + 日历跳转（月历 Sheet）。
- **自主规划 CTA**：页面最显眼元素，primary→accent 渐变大按钮（图标 44px 圆角块 + 标题/副标题 + chevron）。
- **AutoPlanSheet**：范围（今天/明天/未来 N 天/自定义区间，上限 7 天，自定义用月历点选 from/to）→ 摘要 chips（📚 规则数 ⏰ 临期作业 💡 keyFacts，`variant="brief"` 时显示，**默认带摘要**）→ 附加要求文本框 → 模式单选（保留手动 `keep_manual` / 全部重来 `replace_all`）→ 提交出 GenOverlay，结束回填计划并弹 note 反馈。
- **时间块流**：TimeBlockCard 列表（时间列 + 彩色类型点 + 标题 + 时长 + 完成勾选）；当前时刻插红色 now-line；无时间块归入「未定时间」分组；规则块带 ↻ 角标（不能仅靠颜色区分）；点块弹 BlockDetailSheet（改标题/类型/时间/时长、完成、删除；规则块删除=当日隐藏，另有「编辑规则」入口跳资料页）。
- **手动添加**（次级入口）：文字粘贴 / 截图两个 tab → 模拟解析 → 候选块勾选确认 → 选择加到今天/明天。
- **空状态**：EmptyState + 两个引导按钮（去导入资料 / 直接自主规划）。

### 3. 资料库 Materials（`app/page-materials.jsx`）
- 三张导入卡：**Canvas**（上传导出 JSON + 「直连推送设置」打开 Token Sheet：显示/生成/复制 Import Token）、**ICS**、**课表截图**；后两者走 CandidatesSheet（候选规则勾选确认 → 批量保存）。
- **重复规则子页 RulesView**：列表（标题 + 频率自然语言描述 + 来源 Badge user/chat/ics/image/canvas + 启用开关），新建/编辑 RuleEditor（一次性/重复、每天/每周几/每 N 天、时间、时长、起止日期；`by_weekday` 0=周日…6=周六），删除需确认。
- **作业列表**：按 due 分组（已过期红 / 3 天内橙 / 之后），行内完成/忽略按钮。
- **课程与成绩**：course code 胶囊 + 名称 + 分数/等级。

### 4. 陪伴聊天 Companion（`app/page-companion.jsx`）
气泡流（用户右 primary 底，AI 左玻璃底）+ TypingDots + 流式打字；解析 `<plan_update>` / `<rule_update>` 标签→执行动作→流内插入绿色 action card（「已更新 x 日计划 · 去看看」可跳转）；空态显示引导 chips（「帮我规划明天」等）；底部圆角 composer（textarea 自增高 + 圆形发送键）。**真实实现**：SSE `POST /api/ai/companion`，标签处理见 FRONTEND_HANDOFF.md §2，记忆标签 §7。

### 5. 心情 Mood（`app/page-mood.jsx`）
12 宫格 MoodTile（emoji + label，4 列，≤380px 3 列）→ 点选后 AI 回应卡（TypingDots→文字）；负向情绪时附练习邀请按钮 → 三种全屏练习：**呼吸 4-7-8**（3 轮，圆核缩放跟随相位）、**伸展**（5 步 × 20s，进度条 + 跳过）、**着地 5-4-3-2-1**（按顺序点亮圆灯）；下方历史列表（emoji + 标签 + AI 回应 + 时间 + 完成练习 Badge）。

### 6. 设置 Settings（`app/page-settings.jsx`）
- 助手名称 Input（失焦保存 toast）。
- **主题工作室 ThemeStudio**：3 列主题卡网格（渐变 swatch + primary 胶囊 + 选中钩），4 内置主题 + 自定义卡（右键/长按菜单：编辑/删除）+ 「AI 生成主题」卡：输入描述 → 模拟生成 → 顶部深色预览条（应用中… 保留/取消）。
- 语言切换（zh-CN/en-US Segmented，立即生效）。
- Import Token 区块（同资料页）。
- 账户：未登录显示登录/注册 AuthSheet（OAuth 按钮 + 邮箱表单 + 校验错误文案）；已登录 AccountSheet（头像 + 名字 + 退出）。
- **实现时新增（§7）**：「记忆」区块——facts 列表、删单条、确认后清空。

## Interactions & Behavior
- 页面切换：`dc-page-enter`（y:8→0 + fade，320ms `--ease-out-soft`）；卡片入场同款 `dc-rise`。
- 点按反馈：可点卡片 `active` 缩放 .985；CTA hover 上浮 + 阴影加深。
- GenOverlay（自主规划/引导共用）：全屏毛玻璃 + primary 光球 2.4s 呼吸 + 状态文案逐条切换 + 完成变绿钩，然后回调。
- Sheet：移动端 BottomSheet（拖拽把手），桌面居中对话框；危险操作走 Confirm（红色确认键 `--color-states-error`）。
- Toast：底部悬浮黑胶囊（tabbar 上方 96px），可带 action 按钮。
- 主题切换：背景渐变 400ms 过渡；自定义主题变量运行时写到 `:root`（映射逻辑见 `app/ui.jsx` 的 `applyThemeVars`）。
- `prefers-reduced-motion: reduce` 时全部动画近似关闭。
- 崩溃红线：AI 相关请求失败按 error 码翻译提示并可重试；`crisis_detected` 全屏温和提示（见 FRONTEND_HANDOFF.md §0）。

## State Management
原型用 `app/store.js`（单例 + subscribe + localStorage `daycore_v2_state`）模拟后端，真实实现替换为 API 调用（契约 `api/openapi.yaml`）：
- `S.autoPlan({from,to,mode,instructions})` → `POST /api/ai/auto-plan`
- `S.patchPlan(date, action)` → `PATCH /api/plan`（**单块操作一律 PATCH**，勿全量覆盖）
- 规则 CRUD / batch → `/api/rules*`；导入 → `/api/import/*`；聊天 → SSE `/api/ai/companion`；心情 → `/api/ai/mood`
- 会话初始化 `POST /api/session/init` → `assistantName`、`currentTheme`、`language`（用它初始化 locale）
- 记忆（新增）→ `/api/memory`、`/api/import/history`

## Design Tokens
来源：`_ds/daycore-design-system-*/styles.css`（以 CSS 变量为准，勿硬编码）。
- **四主题**（`data-theme`）：sky `#3b82f6`、sunset `#f97316`、night `#a78bfa`（深色，文字 `#f0edff`）、nature `#16a34a`；背景 135°/160° 渐变（各主题 bg-start→bg-end）。
- 圆角：card 20px / chip 999px / button 14px / sheet 24px。动效：120/280/400ms。
- 状态色：success `#22c55e`、warning `#f59e0b`、error `#ef4444`。
- Surface：玻璃拟态 `rgba(255,255,255,.72)` + `backdrop-filter: blur(16px)` + 1px 细边；night 主题用半透明深紫。
- 字体：Inter / SF Pro 栈；等宽 SFMono。
- 组件库：原型使用 `window.DaycoreUI` 的 26 个组件（Button、GlassCard、BottomSheet、TabBar、TimeBlockCard、MoodTile、ChatBubble、TypingDots、Chip、Badge、EmptyState、Skeleton、SectionHeader、Avatar、IconButton、Input、Select、Textarea、ProgressRing…），实现时按同名语义在目标栈中重建。

## Assets
- 图标：全部为内联 SVG 线性图标（lucide 风格，24 viewBox / stroke 2），定义在 `app/icons.js`——实现时可直接换用 lucide-react / lucide-vue。
- 无外部图片；emoji 仅用于心情瓷砖与主题名。
- 字体走系统栈，无需字体文件。

## Files
- `Daycore App.html` — 原型入口（静态服务下打开）
- `app/main.jsx` — 壳：appbar / TabBar / 路由 / 主题启动
- `app/page-today.jsx`、`page-materials.jsx`、`page-companion.jsx`、`page-mood.jsx`、`page-settings.jsx`、`page-onboarding.jsx` — 六屏
- `app/ui.jsx` — Sheet/Confirm/Segmented/Calendar/WeekStrip/GenOverlay/Toast + 主题应用逻辑
- `app/store.js` — 模拟数据层（含演示数据、聊天脚本、导入候选）
- `app/i18n.js` — zh-CN/en-US 全量文案；`app/icons.js` — 图标；`app/app.css` — 全部自定义样式
- `_ds/daycore-design-system-*/` — 设计系统（tokens + 组件样式 + React bundle）
- `FRONTEND_HANDOFF.md` — 产品/对接总纲（§0 i18n、§2 协议、§7 长期记忆）
- `api/openapi.yaml` — 后端 API 契约
- `app/tweaks-panel.jsx` — 仅设计评审用的调节面板，**实现时忽略**
