# 04 · 汀 · 此刻（单件流）

> 源码：`ting/index.html` + `ting/app/{ting-main,ting-parts}.jsx` + `ting.css`

## 范式一句话

**画面永远只回答一个问题：现在做什么。** 上滑送走一件事，提案整屏浮上来，一滑接受、一滑推开。概览被刻意折叠成下拉抽屉——**是抽屉，不是家**。

## 1. 骨架

```
┌─ .tg-top：时钟 + 一行状态（.tg-l0）──────────┐
│  .tg-handle（下拉把手 → 全天抽屉）            │
│                                              │
│            ┌ .tg-main ─────────┐             │
│            │  .tg-eyebrow      │  小字标签    │
│            │  .tg-title        │  大标题      │
│            │  .tg-sub          │  一句解释    │
│            │  .tg-metarow      │  元信息      │
│            │  .tg-actrow       │  [主][次]    │
│            └───────────────────┘             │
│         .tg-swipehint / .tg-keyhint          │
├─ .tg-foot：.tg-next（下一件露头）+ .tg-caps ──┤
└──────────────────────────────────────────────┘
```

三种「面」（`faceKey`）：`brief`（导语）/ 当前块 / `prop`（提案，整屏浮上来）。

## 2. 手势（移动端）

| 手势 | 行为 |
|---|---|
| 上滑 | 主动作：当前块 = 完成并送走；提案 = 接受；导语 = 知道了 |
| 下滑 | 打开全天抽屉 |
| 左滑 | 推开（提案 = 不了） |

`.tg-swipehint` 只在移动端显示（`.up` 箭头 + 文案）。

## 3. 桌面端（≥860px）

**不套手机壳。** 整屏就是那一个问题：

```css
.tg-frame{width:100%;height:100%;border:none;border-radius:0;box-shadow:none;background:none}
.tg-desk{display:none}
.tg-main{padding:0 34px;align-items:center}
.tg-main > *{width:100%;max-width:660px}     /* 内容锁在 660px 栏里居中 */
.tg-title{font-size:clamp(38px,4.4vw,58px);letter-spacing:-.025em}
.tg-sub{font-size:16.5px;max-width:52ch}
.tg-btn{height:50px;padding:0 26px;font-size:14.5px}
.tg-caps,.tg-next{width:min(660px,100%)}      /* 底部与内容同栏宽 */
```

标题字号分三档，用 **class 而不是内联 `style`**（`.tg-title` / `.tg-title.md` / `.tg-title.sm`），否则内联样式会压过媒体查询、桌面端放不大。

> ⚠ **整个桌面 `@media` 块必须排在 `ting.css` 最后**，压过手机端基础值。层叠顺序错了会出现"桌面样式部分生效"的诡异现象。

**键盘操作**（桌面主要靠键盘，不靠滑动）：

| 键 | 行为 |
|---|---|
| `Enter` / `↑` | 主动作（`.tg-main .tg-btn.pri`） |
| `Esc` | 推开（`.tg-sheet .tg-dots` 或 `.tg-btn.sec`） |
| `↓` | 看全天（`.tg-handle`） |

监听挂在 `document`，输入框内不拦截。提示条 `.tg-keyhint` 仅桌面显示，替代上滑提示。

## 4. 主题

只有两套：`night`（水边夜色，默认）与 `dawn`（晨雾）。见 `06-design-tokens.md`。
背景是 `radial-gradient(120vw 60vh at 50% 108%, var(--water), transparent 62%)` —— 底部一汪水光。

## 5. 撤销条

`.tg-undo`：动作后从顶部滑入，3 秒内可撤。桌面端 `top:22px`。
