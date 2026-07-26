# 把这个交接包接到 Claude Code

## 最短路径（3 步）

```bash
# 1. 解压到一个目录
unzip Daycore.zip -d daycore && cd daycore

# 2. （推荐）建 git 仓库，之后好回滚
git init && git add -A && git commit -m "design handoff baseline"

# 3. 在这个目录里启动
claude
```

仓库根的 `CLAUDE.md` 会在**每次会话开始时自动读入**，并被当作权威指令 —— 所以你不需要每次重复交代项目背景。

## 第一句话该说什么

**照抄现成的：`PROMPTS_FOR_CLAUDE_CODE.md`** —— 六个步骤的提示词，一段一次，每段带验证条件，另附纠偏提示词。

下面是浓缩版，不要一上来就说"照着重建"：

**第 1 步 · 只读不写**
> 读 HANDOFF/README.md 和 HANDOFF/01-core-contract.md。先不要写代码，跟我确认一遍你理解的领域模型：石化、撤销、锁定级别分别是什么语义。

**第 2 步 · 先做骨头**
> 把 core/daycore-core.js 用 TypeScript 重写成一个独立包（不依赖 DOM、不依赖 React），保持全部语义不变，并给每种 undo payload 写单测。不要动 UI。

**第 3 步 · 一次只做一个端**
> 读 HANDOFF/02-liuli-changjuan.md 和 06-design-tokens.md，用 <你的技术栈> 重建琉璃。像素级对齐，动效数值照抄。做完用 07-acceptance.md 的 B 节自查。

**第 4 步 · 收尾**
> 按 HANDOFF/07-acceptance.md 逐条自查，把没过的列出来。

## 关于原型的运行

想边看边写，把仓库根跑个静态服务器：

```bash
python3 -m http.server 8080
# 打开 http://localhost:8080/index.html
```

四端共用 `localStorage`，所以在一个端里操作、切到另一个端刷新就能看到同步效果。门厅底部可以切三个演示场景。

## 几个有用的做法

- **别让它一次做完四个端。** 一个端一个会话，做完 commit。四端共 1 万多行原型，一次性重建必然失真。
- **数值有争议时让它问你**，不要让它"就近取整"。`CLAUDE.md` 里已经写了这条，但值得在会话里再强调一次。
- **path-scoped 规则**：如果你的目标工程结构复杂，可以在 `.claude/rules/` 下按路径放更细的规则文件，避免把所有约束都堆进 `CLAUDE.md`（后者建议控制在 200 行内）。
- **`--add-dir`**：如果你把设计包和目标工程放在两个目录，用 `claude --add-dir ../daycore` 让它同时读到这边的文档。
- **`/init` 不要在这个包里跑** —— 根目录已经有手写的 `CLAUDE.md` 了，`/init` 会覆盖成自动生成的版本。在你的**目标工程**里跑 `/init`，然后把这份 `CLAUDE.md` 的「硬规矩 / 语气铁律 / 已知坑」三节拷过去。

## 目录导航

| 想找 | 去哪 |
|---|---|
| 产品是什么、语气规则 | `HANDOFF/README.md` |
| 数据模型与全部语义 | `HANDOFF/01-core-contract.md` |
| 四端逐屏规格 | `HANDOFF/02`–`05` |
| 所有颜色/字号/动效值 | `HANDOFF/06-design-tokens.md` |
| 验收清单 | `HANDOFF/07-acceptance.md` |
| 真后端 HTTP 契约 | `API_CONTRACT.md` |
| 原型源码 | `core/` `liuli/` `zhiyu/` `ting/` `liuli-classic/` |
