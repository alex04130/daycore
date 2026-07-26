# Daycore — 项目记忆（Claude Code 自动读取）

> ## ⚠️ 契约权威在仓库根，不在本目录
>
> 本目录是**设计原型**，已并入 Daycore 主仓库（后端在仓库根）。三份文档的版本号历史上互相矛盾，已裁决：
>
> - **HTTP 契约的唯一权威是仓库根的 `api/openapi.yaml`**（93 操作 / 53 路径）。写代码请求后端时以它为准。
> - 本目录的 `API_CONTRACT.md` 抬头「v4 · beta」**是当时前端自己的编号，不是 API 版本**。它的路径命名（`/api/companion/threads/{id}/messages`、`PUT /api/plan/blocks`、`/api/moods`、`/api/profile`）与真实后端**对不上，一律不采纳** —— 它只说明「设计期望了哪些能力」。
> - 端无关语义总纲是仓库根的 `docs/EXPERIENCE_CORE.md`（与本目录 `uploads/daycore-体验内核.md` 同一份，md5 一致）。其**第十五节的增减项**才是要落到后端的。
> - 版本号分三层：后端构建版本（`internal/version/version.go` 的 `Version`+`Channel`）／API 契约版本（同文件 `APIVersion`+`APIMinor`，**四个前端子仓握手用的是这个**）／各前端自己的版本号（独立迭代）。
>
> 落地方案见仓库根 `docs/` 六篇实时文档。

## 这是什么

「替不擅长规划的人省心的 AI 日程管家」的**设计原型仓库**。同一套数据契约上并存 **四个范式级不同的前端**（琉璃 / 纸屿 / 汀 / 琉璃初版）。
所有 `.html` / `.jsx` / `.css` 是**用 HTML 写的设计参考**，不是生产代码 —— 目标是在真实工程里**重建出一样的效果**。

> 第五端「径 · 远路」是**试过之后放弃的方向** —— 效果不好，没有做下去。`HANDOFF/08-jing-yuanlu.md` 留着当记录，`jing/` 目录从未存在过。**不要把它当待办**：想重拾得先重新验证那个范式本身站不站得住，而不是照文档实现一遍。

## 动手之前必读

0. `PROMPTS_FOR_CLAUDE_CODE.md` — 分步提示词与验证条件（用户会照这个推进）
1. `HANDOFF/README.md` — 总纲、保真度、语气铁律
2. `HANDOFF/01-core-contract.md` — 领域模型与语义（**唯一必须逐条照搬的部分**）
3. 要做哪个端就读对应那份：`02-liuli-changjuan.md` / `03-zhiyu-flow.md` / `04-ting-now.md` / `05-liuli-classic.md` / `08-jing-yuanlu.md`
4. `HANDOFF/06-design-tokens.md` — 全部颜色/圆角/阴影/动效值
5. `HANDOFF/07-acceptance.md` — 收尾时逐条对照

## 目录

```
index.html              门厅：四端入口 + 演示场景切换
core/daycore-core.js    共享数据核心（mock API，四端唯一共享物）
liuli/                  琉璃 · 长卷（时间画布）+ admin.html 运营台
zhiyu/                  纸屿 · 顺流（叙事流）
ting/                   汀 · 此刻（单件流）
liuli-classic/          琉璃初版 · 页面制（对照组，含 store-bridge.js 桥接层）
API_CONTRACT.md         真后端 HTTP 契约（路径命名非权威，见顶部裁决）
HANDOFF/                交接文档（先读这里；08 是无实现的第五端）
_archive/               历史版本，别动
uploads/                设计简报与素材
```

**已删除**：`_ds/`（Claude Design 的 design system 编译产物）。仓库内的权威副本是 `web/frontend/src/vendor/ds-bundle.js` + `src/ds/`（含本地补丁）。连带 `liuli/admin.html`、`liuli-classic/index.html`、`scraps/old-liuli.html`、`_archive/liuli-v3/index.html` 直接打开会没样式 —— 重建照 `.jsx` 源码写，不需要这些页面能跑。

## 技术现状

- 纯静态，无构建：`<script type="text/babel">` 在浏览器里现场转译 JSX。
- React 18.3.1 / ReactDOM / Babel standalone 从 unpkg 加载（带 SRI 完整性校验）。
- 状态全在 `localStorage`（key `daycore-v4-core`），无后端。
- 任一目录整体静态托管即可运行。

## 重建时的硬规矩

- **先做数据核心，别先做 UI。** 领域语义（读时石化 / 撤销=反向补偿 / 锁定级别 / 提案）是产品的骨头。
- **数值即产品。** 长按 460ms、阻尼回弹 7px、石化阈值 5 小时、动效 160/240/360ms —— 改数值等于改产品，要改先问。
- **文案是定稿。** 中文语气经过刻意设计，**不要改写成更客气/更"专业"的说法**。
- **四个前端不是四套皮肤**，是四种根本不同的回答。不要试图抽象出"通用组件"把它们统一掉 —— 除了数据层。
- 新增写操作必须同时实现它的逆操作，否则不许标 `undoable`。
- `DcCore.patch()` 是**单参数**的：`patch({assistantName:'x'})`。
- 不要用 `scrollIntoView`。用 `scrollTop` 计算。

## 语气铁律

1. 不变红、不积债 —— 没做完的事不变红色警告、不累积成欠账
2. 任何 AI 动作 3 秒内可撤
3. 过去不改写 —— 石化的块只能补注释或「重新安排」
4. AI 说人话 —— 「没做，没关系」而不是「任务未完成，请及时处理」
5. 提案永远是"想放这儿?"的虚影，用户点头才落地
6. **给死路一条岔路** —— 每处"不能做"都要同时给出替代动作

## 已知踩过的坑（别再踩）

- 全宽底部/顶部浮层必须 `pointer-events:none` + 子元素 `auto`，否则形成隐形挡板。
- 纸屿的"纸质感"**不要加噪声**：噪声堆上去的结果是完全不像纸。纯色 + 极淡规则线条才像。
- 媒体查询块要排在样式表最后，否则桌面样式只部分生效。
- 标题字号用 class 不用内联 `style`，否则压过媒体查询。
- 拖拽区域里的点击目标必须 `stopPropagation + preventDefault`。
- 从"现有数据反推的周期"不是规则，不能拿去展开生成数据。
- 「现在」线用均匀实线，不要 `linear-gradient(90deg, accent, transparent)`（左粗右消、视觉偏心）。
