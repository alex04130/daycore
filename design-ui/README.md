# design-ui —— 设计原型（只读参考，不参与构建）

四套**范式级不同**的前端原型，同一套数据语义的四种回答。它们是重建生产前端时**逐行对照的依据**，不是生产代码本身（`.html`/`.jsx`/`.css` 靠 babel standalone 在浏览器里现场转译）。

## 先读哪份

| 顺序 | 文件 | 是什么 |
|---|---|---|
| 0 | [`CLAUDE.md`](CLAUDE.md) | **项目记忆 + 契约裁决**（顶部那段决定你该信哪份契约，先看） |
| 1 | [`README-CLAUDE-DESIGN.md`](README-CLAUDE-DESIGN.md) | 设计仓自带的原始说明 |
| 2 | [`HANDOFF/README.md`](HANDOFF/README.md) | 交接总纲、保真度要求、语气铁律 |
| 3 | [`HANDOFF/01-core-contract.md`](HANDOFF/01-core-contract.md) | 领域模型与语义 —— **唯一必须逐条照搬的部分** |
| 4 | [`PROMPTS_FOR_CLAUDE_CODE.md`](PROMPTS_FOR_CLAUDE_CODE.md) | 分步重建提示词与验证条件 |

## 四端

| 目录 | 范式 | 一句话 |
|---|---|---|
| [`liuli/`](liuli/) | 长卷 · 时间画布 | 时间是一块连续的空间画布 |
| [`zhiyu/`](zhiyu/) | 顺流 · 叙事流 | 一天是一篇共同书写的日志 |
| [`ting/`](ting/) | 此刻 · 单件流 | 画面永远只回答一个问题：现在做什么 |
| [`liuli-classic/`](liuli-classic/) | 页面制 | Tab + 卡片列表，作为对照组 |

第五端「径 · 远路」只有文档（[`HANDOFF/08-jing-yuanlu.md`](HANDOFF/08-jing-yuanlu.md)），无实现。

其余：`core/daycore-core.js` 四端共享的 mock 数据核心 · `_ds/` DaycoreUI 设计系统原件 · `app/` 与 `_archive/` 历史版本 · `uploads/` 设计简报与素材 · `scraps/` 零碎截图。

## 与主仓库的关系

- **契约权威在仓库根** [`api/openapi.yaml`](../api/openapi.yaml)，不是本目录的 `API_CONTRACT.md` —— 详见 [`CLAUDE.md`](CLAUDE.md) 顶部裁决。
- 端无关语义总纲：[`docs/EXPERIENCE_CORE.md`](../docs/EXPERIENCE_CORE.md)（与 `uploads/daycore-体验内核.md` 同一份）。
- 现役生产前端是 [`web/frontend/`](../web/frontend/)，四端重建完成前不动它。
- 重建落地计划见仓库根 [`docs/`](../docs/) 六篇实时文档。

## 重建时的硬规矩

摘自 [`CLAUDE.md`](CLAUDE.md)，违反了等于白做：

1. **先做数据核心，别先做 UI** —— 领域语义（读时石化 / 撤销=反向补偿 / 锁定级别 / 提案）是骨头。
2. **数值即产品** —— 长按 460ms、阻尼回弹 7px、石化阈值 5 小时。改数值等于改产品。
3. **文案是定稿** —— 中文语气经过刻意设计，不要改写成更"专业"的说法。
4. **四端不是四套皮肤** —— 除数据层外不要抽象出"通用组件"把它们统一掉。
5. 新增写操作必须同时实现逆操作，否则不许标 `undoable`。
