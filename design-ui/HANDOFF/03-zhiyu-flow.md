# 03 · 纸屿 · 顺流（叙事流）

> 源码：`zhiyu/index.html` + `zhiyu/app/{flow-main,flow-parts,flow-panels,flow-settings}.jsx` + `dc.css`（设计系统层）+ `flow.css`（页面层）
> 用户端与运营台同一个 SPA，运营台在 `#/admin`。

## 范式一句话

**一天是一篇由你和小禾共同书写的日志。** 向上滚是过去，向下滚是预告，**滚动本身就是时间旅行**。导航退化为一个「回到现在」按钮。

关键区别：这是**账本渲染流**，不是聊天记录。每条都是一个事件卡，不是气泡。足迹与今天是同一条流的不同滚动位置——没有"历史页"。

## 1. 骨架

```
┌─ 顶部细条：日期 + 「回到现在」───────────────┐
│                                              │
│   ── 7月23日 周三 ─────────────  ← 日分割线   │
│   ┌ .fl-card ────────────────┐               │
│   │ 08:30  PSYC 110（课）  📌 │  ← 图钉=硬锁  │
│   │ 50 分钟                  │               │
│   └──────────────────────────┘               │
│   ┌ .fl-prop 便签条 ─────────┐               │
│   │ 小禾想在 20:00 放 45 分钟 │  ← 提案       │
│   │ [就这么办] [不了] [追问]  │               │
│   └──────────────────────────┘               │
│   ── 现在 ────────────────────                │
│   （下面是预告，视觉更淡）                     │
│                                              │
├─ 底部：一句话输入 + 心情胶囊 ─────────────────┤
└──────────────────────────────────────────────┘
```

## 2. 纸张主题体系（四种纸，不是四种颜色）

主题名与底纹**必须对应**——名字叫「格线纸」就得看见格子。定义在 `dc.css`，主题列表在 `flow-settings.jsx` 的 `BUILTIN`。

| id | 名称 | 底纹 | 主色 | 纸色 |
|---|---|---|---|---|
| `sky` | **格线纸** | 26px 方格（横竖各一层 1px 线，`--dc-ink` 4%） | `#2f6bff` | `#f5f7fa` |
| `nature` | **行线本** | 27px 横行线（`--dc-ink` 9%）+ 左侧 76px 处一道红边线（`#c2564a` 34%） | `#2e9e63` | `#f4f8f4` |
| `sunset` | **牛皮纸** | 纯色（早期的斜向纤维纹已去掉，见 §3） | `#e0632a` | `#efe3ce`（偏黄褐） |
| `night` | **炭黑纸** | 纯色 | `#4fc2ae` | `#14161d` |

## 3. 纸就是纸：噪声必须全部去掉

这是踩过两次的坑，重建时**不要"加质感"**：

- `.dc4-bg::after`（噪声层）与 `.dc4-bg::before`（彩色光斑）**都是 `display:none`**。
- `.fl-card` 背景是**纯 `var(--dc-surface)`**，没有 `--dc-grain`、没有 `background-blend-mode: multiply`。
- 便签 `.fl-prop` 背景 = 行线 + 一层柔和斜向渐变（`linear-gradient(166deg, var(--note-hi), var(--note))`），**没有随机噪声图**。

> 追求"纸质感"堆噪声的结果是**完全不像纸**。真实的纸在屏幕上就是干净的纯色 + 极淡的规则线条。

## 4. 图钉（硬锁标记）

纸屿用**图钉**而不是小锁——因为纸屿的视觉语法是"贴在纸上的东西"。

纯 CSS，**不画 SVG**：

```css
:root{--pin-c:var(--dc-accent-2,var(--dc-accent));--pin-sz:9px;--pin-shape:50%}
.fl-pin{
  position:absolute;left:7px;top:9px;width:var(--pin-sz);height:var(--pin-sz);
  border-radius:var(--pin-shape);
  background:radial-gradient(circle at 34% 30%, #fff 0 12%, var(--pin-c) 46%,
             color-mix(in srgb, var(--pin-c) 62%, #000) 100%);
  box-shadow:0 1px 0 rgba(0,0,0,.28),          /* 短硬投影 = 钉尖压进纸里 */
             0 2px 2px -1px rgba(0,0,0,.32),
             0 0 0 2.5px color-mix(in srgb, var(--dc-ink) 5%, transparent); /* 纸面压凹 */
  z-index:2;
}
```

**关键**：投影要**短而硬**。长柔投影会立刻变成"浮在上面的贴纸"，而不是"被压凹的钉子"。三个变量 `--pin-c/--pin-sz/--pin-shape` 在 `:root`，AI 生成主题时可覆盖。

## 5. 锁定块的卡片语法

```
.fl-card.lk-hard  实线细边框（--dc-ink 22%）+ 左侧 2px 直角色条（40%）+ padding-left:20px
.fl-card.lk-soft  实线边框（--dc-ink 14%），无图钉
```

锁定块**换掉便签那套"可撕可挪"的语法**，变成"钉在纸上的印刷条目"。

拖不动的反馈：`.fl-card.lk-hard.tug` → `@keyframes fl-tug` 420ms，位移 7px → −3px → 1.5px → 0，伴 0.5deg 摆动。

## 6. 条目菜单 `EntryMenu`

- `hard` 锁：`.fl-menu-lock` 说明行（小图钉 + 原因 +「时间不由你定」）+「请假 / 不去」+「标记冲突」
- `soft` 锁：「挪到明天」右侧有个 `.fl-lk-dot` 小灰点提示；点击后菜单**原地**换成 `.fl-menu-confirm`（问句 + `[仍要改][算了]`），不是弹系统 `confirm`
- 自由块：挪到明天 / 一句话改期…
- 石化块：只有注释与「重新安排」

## 7. 滚动与时间旅行

- 单一滚动容器，日分割线 `sticky`。
- 「现在」位置是流里的一条线；`flowNow()` 提供 current / next / prev。
- 未来的卡视觉更淡（`opacity` 递减），过去的卡饱和度更低。
- 「回到现在」按钮平滑滚回该线（**不要用 `scrollIntoView`**，用 `scrollTop` 计算）。
