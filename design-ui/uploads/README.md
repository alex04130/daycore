# uploads/ —— 设计过程的原始上传件

设计四端时喂给设计工具的素材。**只读、不参与构建、不是任何东西的权威。**

留下的是图片（截图与草图 —— 它们没有更新的版本，就是原件）与两份仍有参考价值的设计文本：

- `DESIGN_V4.md`、`daycore-设计简报.md` —— 界面形态与视觉意图。四端重建时对照它们。

## 2026-07-29 删掉的三份

它们是**仓库内其它文件的旧副本**，留着只会让人读到错的那一份：

| 删掉的 | 权威在 | 为什么删 |
|---|---|---|
| `FRONTEND_HANDOFF-634a586f.md`（228 行） | [`api/FRONTEND_HANDOFF.md`](../../api/FRONTEND_HANDOFF.md)（253 行） | 逐行比对，独有的六行**全是指向已删除的 `claude-design/` 目录的失效路径**；唯一有用的那条（老后端没有 `/api/version` 时回退 `/api/healthz`）新版已覆盖并扩写 |
| `daycore-体验内核.md`（250 行） | [`docs/EXPERIENCE_CORE.md`](../../docs/EXPERIENCE_CORE.md)（338 行） | **独有内容 0 行**，纯子集 |
| `openapi-9c1d7c33.yaml` | [`api/openapi.yaml`](../../api/openapi.yaml) | 既过期，**又连 YAML 都解析不过**（第 122 行语法错）。一份坏掉的契约副本比没有副本危险 |

三份都在 git 历史里，需要时 `git log --diff-filter=D -- <path>` 取回。
