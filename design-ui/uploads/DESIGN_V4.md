# Daycore v4 — 设计语言规范（「纸屿 / Paper Isle」）

> 本文件是 v4 视觉与交互重做的**唯一权威**。目标：好看、好用、温暖、克制，以用户体验为第一原则。
> 不追求与旧版（v3 / claude-design / _ds bundle）视觉一致；组件类名 API（`dc4-*`、`DcUI.*`）保持稳定，视觉全面重做。
> 硬约束：低饱和、暖色优先、充足留白、清晰层级；**禁止蓝紫渐变、禁止高饱和大色块背景、禁止 Google Material 风**。

---

## 1. 设计原则

1. **纸感为底，色彩为屿**：界面像一张温润的纸，颜色只出现在该出现的地方（CTA、状态、点缀）。大面积区域永远低饱和。
2. **真实层级靠阴影与留白，不靠边框堆叠**：卡片以柔和投影浮于纸上；分割线极细极淡，能不用就不用。
3. **一次只有一个主角**：每屏一个主行动点（CTA），其余都是配角。
4. **动效是反馈不是装饰**：按压缩回、悬浮抬升、页面淡入上滑；克制、快速、可关闭。
5. **内容密度分区**：用户端松弛（14.5px 正文、大留白）；管理台紧致（13.5px、表格化、mono 数据）。

## 2. 色彩系统

### 2.1 结构（CSS 变量，全部组件只引用变量，不硬编码色值）

```
--dc-bg            页面底色（纸）
--dc-surface       卡片/浮层表面（不透明）
--dc-surface-2     次级表面（输入填充、轨道）
--dc-ink           主文字（近黑，非纯黑）
--dc-ink-2         次文字    --dc-ink-3  弱文字/占位
--dc-line          发丝分割线（极淡）
--dc-accent        主题强调色（实心 CTA）
--dc-accent-ink    accent 上的文字色（通常白）
--dc-accent-soft   accent 淡底（10-14% 透明度，chip/激活态）
--dc-accent-2      accent 渐变搭档（仅小面积渐变点缀）
--dc-ok / --dc-warn / --dc-err   状态色（低饱和版本）
--dc-orb-1 / --dc-orb-2 / --dc-orb-3   背景光斑色（≤18% 透明度使用）
```

### 2.2 四主题（`data-theme` 挂在 `<html>`）

| token | sky 晴空 | sunset 暖阳 | night 深夜（暗） | nature 森绿 |
|---|---|---|---|---|
| --dc-bg | #f5f7fa | #faf6f1 | #14161d | #f4f8f4 |
| --dc-surface | #ffffff | #fffdfb | #1e2029 | #ffffff |
| --dc-surface-2 | #eef1f6 | #f3ede4 | #262935 | #ebf2ea |
| --dc-ink | #1a2333 | #26190e | #eceef4 | #152419 |
| --dc-ink-2 | #4d5a70 | #6d5c49 | #a3a8ba | #49604f |
| --dc-ink-3 | #8a96a9 | #a3937f | #6b7186 | #829a88 |
| --dc-line | rgba(26,35,51,.08) | rgba(38,25,14,.09) | rgba(236,238,244,.09) | rgba(21,36,25,.08) |
| --dc-accent | #2f6bff | #e0632a | #4fc2ae | #2e9e63 |
| --dc-accent-ink | #ffffff | #ffffff | #0e2a25 | #ffffff |
| --dc-accent-2 | #63b3ff | #f2a541 | #7fe0cd | #6fc88f |
| --dc-ok | #2e9e63 | --dc-warn #d98a1f | --dc-err #d9544d | （三主题共用，night 自动提亮） |
| orbs | #bcd6ff / #d8e9ff / #cfe4f7 | #ffd9b8 / #ffe9c9 / #ffc9a8 | #2b4a55 / #35506b / #3d5a50 | #c8e9c9 / #d9f0dc / #bfe3d2 |

night 是深色主题：表面用 `--dc-surface`（不透明深灰蓝），阴影更深，边框更亮一点。accent 选青玉色刻意避开蓝紫。
accent 使用规则：**实心填充只给主 CTA / 激活态**；淡底（soft）给 chip、选中、图标底；`--dc-accent-2` 只允许小面积渐变（hero 标题字、logo 点、进度环）；任何 >40% 屏面的区域不得用饱和色。

### 2.3 自定义主题兼容

`DcUI.applyThemeVars` 把旧 12 变量白名单映射到新 token：`--primary→--dc-accent`、`--accent→--dc-accent-2`、`--bg-start→--dc-bg`、`--text-primary→--dc-ink`、`--text-secondary→--dc-ink-2`、`--text-muted→--dc-ink-3`、`--surface→--dc-surface`、`--surface-hover→--dc-surface-2`、`--success→--dc-ok`、`--warning→--dc-warn`、`--error→--dc-err`。底座仍是最近内置主题（dark→night）。

## 3. 背景系统

`.dc4-bg`（fixed inset-0, z-index -1, pointer-events none），三层叠加：
1. 底色 `--dc-bg`；
2. 2-3 个大光斑（radial-gradient，直径 50-70vw，blur 60-80px，透明度 sky/sunset/nature 14-18%、night 20-26%），位置固定（右上、左下、可选中上），可用 40s 缓慢漂移动画（位移 ≤40px）；
3. 细颗粒噪点（内联 SVG feTurbulence，opacity .025，mix-blend-mode: multiply(light)/overlay(night)）。

`prefers-reduced-motion`：光斑静止。AppShell / AdminShell / onboarding 都挂这一层。

## 4. 层级（elevation）

| 级 | 用途 | 阴影（light；night 加深 1.6 倍 alpha） |
|---|---|---|
| e1 | 卡片 | `0 1px 2px rgba(20,28,44,.05), 0 2px 8px rgba(20,28,44,.05)` + 1px `--dc-line` 描边 |
| e2 | 弹层/sheet/drawer/下拉 | `0 4px 12px rgba(20,28,44,.08), 0 12px 32px rgba(20,28,44,.10)` |
| e3 | 悬浮导航/dirty bar/toast | `0 8px 24px rgba(20,28,44,.14), 0 2px 6px rgba(20,28,44,.08)` |

hover 抬升（可交互卡）：translateY(-1px) + 阴影升到 e2。玻璃只用于「悬浮 chrome」（移动端 tabbar、桌面 rail、管理台顶栏、sheet）：`backdrop-filter: blur(20px)` + surface 84% 透明。**内容卡一律不透明**。

## 5. 排版

字体栈不变（Inter/SF Pro + 系统中文字体 + mono）。数字一律 `font-variant-numeric: tabular-nums`（时间、统计、表格）。

| 级 | 尺寸/行高/字重/字距 | 用途 |
|---|---|---|
| Display | 30/36 · 760 · -0.02em | 页面大标题（今日问候、onboarding、管理台页头） |
| H1 | 22/30 · 700 · -0.01em | 卡片组标题、sheet 标题 |
| H2 | 16.5/24 · 650 | 卡标题、区块标题 |
| Body | 14.5/23 · 450 | 正文（管理台 13.5/21） |
| Caption | 12.5/18 · 500 | 辅助说明、表头（表头加 .06em 字距大写） |
| Eyebrow | 11.5/16 · 650 · .08em 大写 | 页眉眉头（accent 或 ink-3） |

## 6. 圆角与间距

圆角：xs 8 / sm 12 / md 16 / lg 20 / xl 26 / pill 999。卡 18、hero 26、按钮 12、输入 12、sheet 24、表格容器 14、tag 8。
间距：4 基网；页边距 20（移动）/ 28（桌面）；区块间隔 24；卡内边距 16-18；表单项间隔 14。

## 7. 核心组件规格

- **主按钮**：`--dc-accent` 实心 + accent-ink 字，高 40（sm 34），圆角 12，字重 600；hover 亮度 +4% 并带 accent 25% 柔光影；active scale .98；focus-visible 2px accent 环（offset 2）。次按钮=表面+发丝边；幽灵=无边 hover 淡底；危险=`--dc-err` 系。
- **输入/选择/文本域**：`--dc-surface-2` 填充、无边、圆角 12、高 40；focus 时背景转 surface + 1.5px accent 环；错误态 `--dc-err` 环 + 下方红字。
- **Toggle**：44×26 pill，轨道 surface-2/ink-3，开时 accent；圆钮 20px 白色 + 阴影，spring 位移。
- **SegmentedControl**：轨道 surface-2 圆角 12 padding 3；指示块 surface + e1 阴影圆角 9，spring 滑动。
- **卡片**：e1 + 圆角 18 + surface 不透明；卡头可选 icon（18px，accent-soft 圆角 10 底衬）。
- **Tag/徽标**：圆角 8、12px 字、淡底深字；状态点 8px + 文字，不只靠颜色。
- **移动 TabBar**：悬浮 pill——左右各 12px、底部 `calc(10px + safe-area)`、圆角 24、玻璃 + e3；激活 tab 图标+文字外包 accent-soft 圆角 14 小 pill；图标 22px。
- **桌面 Rail**：宽 224px，透明底（纸色透出）+ 右侧 1px `--dc-line`；logo 区 28px 圆点（accent→accent-2 渐变）；导航项圆角 12，激活 = surface + e1 + 左侧 3px accent 条；底部用户卡。
- **管理台侧栏**：232px，`#171a22`（浅色主题下也为深色，「控制台」身份），文字 #c9cede，激活项 accent-soft 底；内容区 max-width 1080，顶栏 52px 玻璃。
- **Sheet/Drawer**：移动底部升起圆角 24 + 把手 36×4；桌面居中对话框（max-width 520，宽 sheet 680）；背板 ink 32% + blur(4px)；进入 280ms 上滑+淡入。
- **Toast**：底部居中悬浮 pill（surface + e3 + 状态色图标），320ms 上滑入，最多叠 3 条。
- **ConfirmDialog**：居中卡（圆角 20，e2），危险按钮 `--dc-err` 实心。
- **表格**：容器卡圆角 14 + e1；表头 Caption 大写 + ink-3 + 底部 1px 线；行高 40（管理台 38）、行分隔发丝线、行 hover accent 4% 底；数字右对齐 tabular-nums。
- **Skeleton**：surface-2 底 + 扫光（1.4s 线性斜向高光）。
- **空态**：居中、图标 40px 于 accent-soft 圆角 20 底衬内、H2 标题 + caption 描述 + 可选按钮。
- **DirtyBar**：底部居中悬浮 pill（surface 玻璃 + e3）：「N 项未保存」+ 放弃（幽灵）/ 保存（主按钮 sm）。
- **秘密字段**：mono •••••••• + 状态徽标 + 「重设」ghost 小按钮。

## 8. 动效

`--dc-ease: cubic-bezier(.2,.8,.2,1)`；`--dc-ease-spring: cubic-bezier(.34,1.56,.64,1)`（toggle、segmented、pill）；时长 160（微交互）/240（组件）/360（页面、sheet）。
页面进入：opacity 0→1 + translateY(8px)→0，240ms。列表项可 30ms 级联（仅首屏）。`prefers-reduced-motion`：全部动画与过渡归零（光斑静止）。

## 9. 无障碍

focus-visible 统一 2px accent 环；正文对比 ≥4.5:1（night 主题文字已调亮）；交互目标 ≥40px；图标按钮必有 aria-label；状态不只靠颜色（点+文字/形状）。

## 10. 页面气质指引（页面 worker 各自发挥的空间）

- **今日**：问候区即 hero——Display 标题 + 当日进度细环/细条；自主规划 CTA 是**全页唯一实心大按钮**（可带 accent→accent-2 微渐变）；时间块卡片化，now-line 用 accent 细线 + 圆点。
- **陪伴**：沉浸对话——气泡圆角 20（user 实心 accent、assistant surface），动作卡做成细边「过程条」，决策卡 accent 描边 1.5px；composer 悬浮 pill。
- **资料**：模块清晰分区，导入三卡等宽网格；确认卡是主角（e2 浮层感）。
- **心情**：最柔和的一页——12 宫格大 emoji tile（圆角 18、选中 accent 描边+淡底），练习全屏配大圆环动画。
- **设置**：分组卡列表，行高舒适，危险操作独立红边卡。
- **Onboarding**：全屏光斑 + 超大 Display 字 + 呼吸光球（accent 径向渐变球，scale 1↔1.06 呼吸 4s）。
- **管理台**：紧致专业——stat 大数字（Display 30px tabular-nums）、mono 数据、状态点、发丝表格。

## 11. 不变量（页面 worker 不得破坏）

- 组件类 API：`dc4-*` 类名、`DcUI.*` 组件名与 props、`DcStore`/`DcI18n`/`DcIcons` 接口全部保持不变。
- `data-theme` 切换与自定义主题映射继续工作。
- 页面注册、i18n key（复用现有，新增走 `DcI18n.add`）、路由契约不变。
- 功能一个不丢：所有交互、流程、状态（Skeleton/Empty/Error/Confirm）保留。
