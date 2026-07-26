# 给 Claude Code 的提示词（照顺序复制粘贴）

> 用法：一段一次，**做完一段再贴下一段**。每段结尾都有验证动作，没过就别往下走。
> 括号里的 `<...>` 需要你替换成实际值。

---

## 步骤 0 · 把项目拉下来

如果你还没有本地文件，先在 Claude Code 里贴这段：

```
用 claude_design MCP（https://api.anthropic.com/v1/design/mcp，需要先 /design-login）
把这个设计项目导入到当前目录：
https://claude.ai/design/p/22da7446-67c2-42b5-b1eb-ea4bc693affa

导入完成后：
1. 用 ls -R 列出目录结构给我看
2. 确认这四个文件存在：CLAUDE.md、CLAUDE_CODE_SETUP.md、HANDOFF/README.md、core/daycore-core.js
3. git init && git add -A && git commit -m "design handoff baseline"
4. 不要跑 /init —— 根目录已经有手写的 CLAUDE.md 了

先不要写任何代码。
```

**验证**：`ls` 输出里有 `liuli/ zhiyu/ ting/ liuli-classic/ core/ HANDOFF/`，git 首次 commit 成功。

---

## 步骤 1 · 让它先读懂，别急着写

```
读这三份文档，然后回答我下面的问题。这一步不要写任何代码、不要创建任何文件。

- HANDOFF/README.md
- HANDOFF/01-core-contract.md
- CLAUDE.md

回答（每条 2-3 句，用你自己的话，不要抄原文）：

1. 「读时石化」的阈值是什么？为什么是"读时"而不是定时任务？石化的块用户还能做什么、不能做什么？
2. 「撤销 = 反向补偿」和快照回滚的区别是什么？如果我新增一个写操作，我必须同时做什么才允许标 undoable？
3. hard 锁和 soft 锁分别用在什么东西上？moveBlock 对两者的返回值有什么不同？为什么 AI 可以越过锁？
4. 锁定块的 UI 表达为什么必须有三层？只做第一层（画个图标）会出什么问题？
5. 「给死路一条岔路」这条铁律，举一个文档里的具体例子。
6. 这四个前端为什么不能抽象成"通用组件 + 四套主题"？

回答完停下等我确认。
```

**验证**：第 1、3、4 题答错就别往下走 —— 这三条是最容易被"优化"掉的语义。特别是第 4 题，正确答案要提到"完全不动会被当成 bug"。

---

## 步骤 2 · 先做骨头：数据核心

```
把 core/daycore-core.js 重写成一个独立的 TypeScript 包，放在 packages/daycore-core/。

硬性要求：
- 不依赖 DOM、不依赖 React、不依赖 localStorage —— 持久化抽象成一个 Storage 接口，
  由调用方注入（浏览器传 localStorage 适配器，测试传内存实现）。
- 所有实体按 HANDOFF/01-core-contract.md §2 定义为 interface，导出。
- Phase 计算、锁级别推断、账本、撤销分派逐条对齐 §3/§4/§5。
- patch() 保持单参数签名，并用类型约束住（Partial<State>），不允许两参调用。
- assistantName / language 在 State 根上，不在 prefs 里。

测试（Vitest）：
- 每一种 undo payload.type 至少一个 case：add_block / remove_block / move_block /
  status / set_mark / set_fields / reconcile / note / bulk_blocks / assignment_done /
  add_wish / add_mood / add_material / add_memory / remove_material / remove_memory
- 石化边界：now-5h 前后各一个块，断言 phase
- hard 锁：moveBlock 返回 {locked:'hard'}，块未移动
- soft 锁：未确认返回 {locked:'soft'}；带 _confirmed 才落下
- agent=true 越过两种锁
- bulkBlocks：29 个块写成 1 条 op，undo 一步全清
- deriveLock：「CS 201 数据结构（课）」→ hard，「社团例会」→ soft，「写作业」→ null
- scenarioSet 保留 themes / customThemes

不要碰任何 UI 文件。做完把测试跑一遍，把结果贴给我。
```

**验证**：测试全绿，且 case 数量 ≥ 25。少于这个数说明 undo 表没覆盖完。

---

## 步骤 3 · 一端一会话（先做琉璃）

> ⚠ **不要让它一次做四个端。** 一个端一个会话，做完 commit 再开下一个。

```
读 HANDOFF/02-liuli-changjuan.md 和 HANDOFF/06-design-tokens.md §1，
用 <你的技术栈，例如 React 19 + TypeScript + Vite + CSS Modules> 重建「琉璃 · 长卷」。

对照源码：liuli/index.html、liuli/app/{cj-main,cj-canvas,cj-parts,cj-drawers,cj-settings}.jsx、
liuli/app/changjuan.css、liuli/app/app.css

要求：
- 像素级对齐。颜色、圆角、阴影、字号从 06-design-tokens.md §1 照抄，不要"就近取整"。
- 动效数值照抄：长按 460ms、阻尼回弹 7px、tug 动画 440ms、--ease / --spring 曲线原样。
- 数据层只用 packages/daycore-core，不要在 UI 里存业务状态。
- 图标用内联 SVG（源码在 liuli/app/icons*.js），不要引图标库。

特别注意这几条（都是踩过坑修回来的）：
1. .cj-top / .cj-bottom / .cj-wells 必须 pointer-events:none + 子元素 auto
2. 虚影和真实块一起 laneize，虚影 z-index 更高 + 不透明底 + 描边
3. 「现在」线是均匀 1.5px 实线，不是渐变
4. 打勾圈 onPointerDown 要 stopPropagation + preventDefault
5. 引力井 2×2 四个，手机判定内缩 10px / 桌面放宽 18px
6. 标签 slice(0,3) 硬上限

做完按 HANDOFF/07-acceptance.md 的 B 节逐条自查，把没过的列出来，不要自己判断"差不多"。
```

**验证**：让它跑一遍 B 节 19 条，逐条给出"过 / 没过 + 证据"。然后 commit。

**其余三端同法，替换文档与源码路径**：

| 端 | 文档 | 令牌 | 验收节 |
|---|---|---|---|
| 纸屿 | `03-zhiyu-flow.md` | §2 | C 节 |
| 汀 | `04-ting-now.md` | §3 | D 节 |
| 琉璃初版 | `05-liuli-classic.md` | §4 | E 节 |

做纸屿时额外强调：

```
纸屿有一条反直觉的规矩：不要加噪声、不要加"纸质感"贴图。
.dc4-bg::before 和 ::after 都是 display:none，卡片背景是纯色。
四个主题名（格线纸/行线本/牛皮纸/炭黑纸）必须和底纹对应 —— 叫格线纸就要看见格子。
图钉的投影要短而硬（0 1px 0 + 0 2px 2px -1px），长柔投影会变成"贴纸"而不是"钉子"。
```

做汀时额外强调：

```
桌面端（≥860px）不要套手机壳，整屏就是那一个问题，内容锁 660px 栏居中。
整个桌面 @media 块必须排在样式表最后。
标题字号用 class 三档，不要内联 fontSize，否则压过媒体查询。
桌面要有键盘操作：Enter/↑ 主动作、Esc 推开、↓ 全天。
```

做初版时额外强调：

```
这一版的重点是 store-bridge.js 那层投影：核心是唯一真相，旧模型只是视图。
规则（rules）核心里没有这个实体 —— 归纳出来的规则一律不可写（managed:true），
因为"从现存块反推的周期"只是猜测，拿去展开会凭空造数据。
一条规则的铺开/撤下必须是 1 条 op，不是 N 条。
```

---

## 步骤 4 · 门厅与跨端联动

```
重建 index.html 门厅：四端入口卡 + 演示场景切换器（健康日/危机日/冷启动）。

然后验证跨端联动，把结果贴给我：
1. 在琉璃里完成一个块 → 切到纸屿刷新，那个块显示为完成
2. 在纸屿接受一个提案 → 切到汀，它出现在流里
3. 门厅切到危机日 → 四个端都变成危机日的数据
4. 在任一端撤销一个 AI 动作 → 其余三端同步回滚
```

---

## 步骤 5 · 收尾验收

```
按 HANDOFF/07-acceptance.md 逐条自查全部六节（A-F），输出一张表：
| 条目 | 过/没过 | 证据或原因 |

规则：
- 没实际验证过的不许写"过"
- "视觉上差不多"不算过，给出实测数值
- F 节（语气与文案）要 grep 一遍代码，确认没有"逾期""未完成""请及时处理"这类字样

最后列出所有没过的项，按修复成本排序。
```

---

## 随时可用的纠偏提示词

**它开始"优化"设计时**：
```
停。CLAUDE.md 里写了「数值即产品」——长按 460ms、回弹 7px、石化 5 小时这些不是随手写的，
改数值等于改产品。你觉得某个值不合理，先告诉我理由，我决定改不改，不要自己动。
```

**它开始改文案时**：
```
文案是定稿。中文语气经过刻意设计：「没做，没关系」不能改成「任务未完成，请及时处理」，
「不是坏事——说明还没人替你着急」不能改成「暂无日程」。
把你改过的文案全部还原，并列出你改了哪些。
```

**它想抽象出通用组件时**：
```
不要抽象。四个前端不是四套皮肤，是四种根本不同的回答——
时间画布 / 叙事流 / 单件流 / 页面制。把它们统一成"通用组件 + 主题"会毁掉这个项目的全部意义。
唯一该共享的是数据层（packages/daycore-core）。
```

**它报告"做完了"但你没看到验证时**：
```
把 HANDOFF/07-acceptance.md 对应那一节逐条走一遍，每条给出"过/没过 + 你怎么验证的"。
没实际点过、没实际量过的不许写"过"。
```

**遇到"部分区域完全没样式"时**：
```
先怀疑样式表配对，不要去调 flex。
琉璃初版踩过这个坑：markup 用了 75 个类只在 v3 版 app.css 里有，配了旧版就整片区域退化成裸文字流。
先 grep 一遍：markup 里用到的 class 有多少在样式表里找不到。
```

**它想加红色警告 / 逾期提示时**：
```
铁律第一条：不变红、不积债。没做完的事不变红色警告、不累积成欠账。
过期未做的块只是安静地问「它该去哪儿？」并给三条出路。
把红色状态和逾期计数全部去掉。
```
