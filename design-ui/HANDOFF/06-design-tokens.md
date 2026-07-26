# 06 · 设计令牌

四端各自自包含视觉层，**不共享 CSS 变量命名**（这是刻意的：它们是四个产品，不是四套皮肤）。

---

## 1. 琉璃 · 长卷（`liuli/app/changjuan.css`）

主题挂在 `[data-cj="..."]`。

```
:root  --r-lg:22px  --r-md:16px  --r-sm:11px
       --ease:cubic-bezier(.22,.9,.24,1)      /* 常规过渡 */
       --spring:cubic-bezier(.34,1.5,.5,1)    /* 弹入 */
       --lock-c:var(--ink3) --lock-sz:9px --lock-op:.72 --lock-radius:1.5px
```

| 变量 | sky（默认） | sunset | night | nature |
|---|---|---|---|---|
| `--bg` | — | `#fff7ed` | `#1e1b4b` | `#f0fdf4` |
| `--bg2` | — | `#fef3c7` | `#312e81` | `#ecfdf5` |
| `--ink` | — | `#1c1917` | `#f0edff` | `#122a1c` |
| `--ink2` | — | `#5d564d` | `#c9c5ec` | `#42604e` |
| `--ink3` | — | `#a59d90` | `#8f8ac2` | `#7e9a89` |
| `--accent` | — | `#f97316` | `#a78bfa` | `#16a34a` |
| `--accent2` | — | `#fb923c` | `#c4b5fd` | `#4ade80` |
| `--glass` | — | `rgba(255,252,246,.78)` | `rgba(255,255,255,.085)` | `rgba(255,255,255,.76)` |
| `--glass-brd` | — | `rgba(255,255,255,.66)` | `rgba(255,255,255,.15)` | — |

night 另有：`--ok:#4ade80`、`--warm:#fbbf24`、`--shadow:0 18px 50px rgba(0,0,0,.5), 0 2px 8px rgba(0,0,0,.32)`、`--shadow-sm:0 6px 22px rgba(0,0,0,.38)`、`color-scheme:dark`。

**玻璃拟态配方** `.glass`：`background: var(--glass)` + `backdrop-filter: blur(...)` + `1px solid var(--glass-brd)` + `::before` 顶部高光 `linear-gradient(168deg, rgba(255,255,255,.07), transparent 40%)`。

块颜色按 type 走 `--c-task / --c-appointment / --c-relax / --c-meal / --c-break`，块内用 `--bc` 引用。

---

## 2. 纸屿 · 顺流（`zhiyu/app/dc.css`）

主题挂在 `html[data-theme="..."]`。

```
:root  --dc-r-xs:8px --dc-r-sm:12px --dc-r-md:16px --dc-r-lg:20px --dc-r-xl:26px
       --dc-r-card:18px --dc-r-sheet:24px
       --dc-ease:cubic-bezier(.2,.8,.2,1)
       --dc-ease-spring:cubic-bezier(.34,1.56,.64,1)
       --dc-t1:160ms --dc-t2:240ms --dc-t3:360ms
       --pin-c:var(--dc-accent-2) --pin-sz:9px --pin-shape:50%
```

字体栈：
```
--dc-font  -apple-system,"SF Pro Text",Inter,"Segoe UI","PingFang SC","Hiragino Sans GB","Microsoft YaHei",sans-serif
--dc-mono  ui-monospace,"SF Mono","JetBrains Mono","Cascadia Code",Consolas,monospace
--dc-serif Georgia,"Times New Roman","Songti SC","Noto Serif SC",serif
```

| 变量 | 格线纸 sky | 牛皮纸 sunset | 炭黑纸 night | 行线本 nature |
|---|---|---|---|---|
| `--dc-bg` | `#f5f7fa` | `#efe3ce` | `#14161d` | `#f4f8f4` |
| `--dc-surface` | `#ffffff` | `#f8f0e2` | `#1e2029` | `#ffffff` |
| `--dc-surface-2` | `#eef1f6` | `#e6d7bd` | `#262935` | `#ebf2ea` |
| `--dc-ink` | `#1a2333` | `#26190e` | `#eceef4` | `#152419` |
| `--dc-ink-2` | `#4d5a70` | `#6d5c49` | `#a3a8ba` | `#49604f` |
| `--dc-ink-3` | `#8a96a9` | `#a3937f` | `#6b7186` | `#829a88` |
| `--dc-line` | `rgba(26,35,51,.08)` | `rgba(38,25,14,.09)` | `rgba(236,238,244,.09)` | `rgba(21,36,25,.08)` |
| `--dc-accent` | `#2f6bff` | `#e0632a` | `#4fc2ae` | `#2e9e63` |
| `--dc-accent-2` | `#63b3ff` | `#f2a541` | `#7fe0cd` | `#6fc88f` |
| `--dc-accent-ink` | `#fff` | `#fff` | `#0e2a25` | `#fff` |
| `--dc-ok` | `#2e9e63` | `#2e9e63` | `#46c583` | `#2e9e63` |
| `--dc-warn` | `#d98a1f` | `#d98a1f` | `#e8a54a` | `#d98a1f` |
| `--dc-err` | `#d9544d` | `#d9544d` | `#e77b74` | `#d9544d` |

阴影（night 版）：`--dc-e1:0 1px 2px rgba(0,0,0,.22),0 2px 8px rgba(0,0,0,.20)`、
`--dc-e2:0 4px 12px rgba(0,0,0,.30),0 12px 32px rgba(0,0,0,.34)`、
`--dc-e3:0 8px 24px rgba(0,0,0,.44),0 2px 6px rgba(0,0,0,.28)`。

便签色（`.fl-prop`）：`--note:#f3e0a1` `--note-hi:#fbf0c4` `--note-ink:#57450f` `--note-ink2:#8a6f2b`。

> `--dc-orb-*` 与 `--dc-grain` / `--dc-noise-blend` 仍在变量表里，但**光斑层与噪声层已 `display:none`**（见 `03-zhiyu-flow.md` §3）。重建时可以直接不实现这些变量。

---

## 3. 汀 · 此刻（`ting/app/ting.css`）

主题挂在 `[data-tg="..."]`。

| 变量 | night（默认） | dawn |
|---|---|---|
| `--bg` | `#0a1014` | `#edf3f1` |
| `--bg2` | `#0d151b` | `#e4edea` |
| `--water` | `#12241f` | `#d7e8e0` |
| `--ink` | `#e9f0ef` | `#152a26` |
| `--ink2` | `#9fb2b0` | `#4e6a63` |
| `--ink3` | `#5f7370` | `#8ba39b` |
| `--line` | `rgba(233,240,239,.09)` | `rgba(21,42,38,.10)` |
| `--card` | `#101a1e` | `#f7fbf9` |
| `--card2` | `#152226` | `#eaf2ee` |
| `--accent` | `#59d0b2` | `#118a6f` |
| `--accent-ink` | `#06231b` | `#f2fbf7` |
| `--warm` | `#e2b565` | `#bd8a2f` |
| `--soft` | `rgba(89,208,178,.13)` | `rgba(17,138,111,.10)` |
| `--shadow` | `0 24px 70px rgba(0,0,0,.55)` | `0 24px 70px rgba(31,64,55,.18)` |

背景：`radial-gradient(120vw 60vh at 50% 108%, var(--water), transparent 62%)` + `var(--bg)`。

桌面端字号：标题 `clamp(38px,4.4vw,58px)`（提案面 `clamp(30px,3vw,40px)`），正文 16.5px（`max-width:52ch`），元信息 13px，按钮 14.5px/高 50px，胶囊 13.5px/高 54px，内容栏宽 `min(660px,100%)`。

---

## 4. 琉璃初版（`liuli-classic/app/app.css` + DaycoreUI 设计系统）

走设计系统的语义变量：`--color-primary` `--color-text-primary/secondary/muted` `--color-surface` `--color-surface-hover` `--color-border-custom` `--color-states-success` `--tab-bar-bg`。主题由 `applyThemeVars()` 注入 12 个变量白名单（`--primary --accent --bg-start --bg-end --text-primary --text-secondary --text-muted --surface --surface-hover --success --warning --error`）。

---

## 5. 全局共同约定

- **字体**：全部系统栈，无 web font 请求。
- **动效时长**：微交互 160ms，常规过渡 240ms，出入场 320–440ms。**不要加长**。
- **触摸目标**：≥44px。移动端引力井是唯一例外（74px 宽但判定内缩 10px，因为它是拖放目标不是点击目标）。
- **最小字号**：正文 ≥13px，元信息 ≥10.5px（仅用于时间戳/标签）。
- **`text-wrap: pretty`** 用在所有多行说明文字上。
- **链接色**：每端都在 `<style>` 里定义了 `a` / `a:hover`，用各自的 `--accent`。重建时不要留浏览器默认蓝。
