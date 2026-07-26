# Daycore — 四前端单体仓库

同一套 API v2.3 语义（`API_CONTRACT.md`），**四个范式级不同的前端**，可并存部署、随时切换。门厅：`index.html`。

> **要在别处重建这套设计？** 从 `CLAUDE_CODE_SETUP.md` 开始，文档在 `HANDOFF/`。

| 前端 | 范式 | 一句话 | 入口 |
|------|------|--------|------|
| **琉璃 liuli/** | 长卷 · 时间画布 | 时间是一块连续的空间画布，在地图上看自己的日子 | `liuli/index.html` |
| **纸屿 zhiyu/** | 顺流 · 叙事流 | 一天是一篇共同书写的日志，滚动即时间旅行 | `zhiyu/index.html` |
| **汀 ting/** | 此刻 · 单件流 | 画面永远只回答一个问题：现在做什么 | `ting/index.html` |
| **琉璃初版 liuli-classic/** | 常规范式 · 页面制 | 底部/侧边 Tab + 卡片列表，作为对照组 | `liuli-classic/index.html` |

## 结构

- `core/daycore-core.js` — 共享数据核心（mock API v2.3）：账本 append-only、读时石化（`min(now−5h, 今天00:00)`）、提案统一资源（虚影/卡/决策行级接受）、撤销=反向补偿、胆量系数、许愿池、Exchange 快照、意图路由。三端共用同一份 `localStorage`（key `daycore-v4-core`）——在任一端的动作，其余两端可见。
- 四端各自自包含视觉层：琉璃玻璃拟态（`liuli/app/changjuan.css`）、纸屿纸张体系（`zhiyu/app/dc.css` + `flow.css`）、汀水色夜屏（`ting/app/ting.css`）、初版走 DaycoreUI 设计系统（副本在 `web/frontend/src/{vendor,ds}/` + `liuli-classic/app/app.css`；`_ds/` 原件已删）。
- 初版通过 `liuli-classic/app/store-bridge.js` 把旧数据模型投影到共享账本，因此四端数据互见。
- 演示场景（健康日/危机日/冷启动）在门厅或各端设置里切换，写进共享账本。
- 运营台：`liuli/admin.html`（独立页，沿用旧版）。
- 设计依据：`uploads/daycore-设计简报.md` + `uploads/daycore-体验内核.md`。旧版前端在 `_archive/`。

演示均为本地 mock，无需后端；把仓库任一目录整体静态托管即可跑。

## 交接

`CLAUDE.md`（Claude Code 自动读取的项目记忆）+ `CLAUDE_CODE_SETUP.md`（接入步骤）+ `PROMPTS_FOR_CLAUDE_CODE.md`（分步提示词）+ `HANDOFF/`（总纲、数据契约、四端逐屏规格、设计令牌、验收清单）。
