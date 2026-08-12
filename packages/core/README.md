# @daycore/core

四个前端共用的那一层：HTTP 客户端、握手、多语言、后端地址配置。

## ⚠️ 抽的时机是有依据的，不是猜的

路线图（阶段 ι）把汀 排在第一，理由写着「最小却跑通完整写+撤销闭环，是暴露
共享层设计错误最便宜的地方」。这个包是**在汀 跑通之后**从它里面抽出来的 ——
每一个模块都有过一个真实调用方，而不是先设计一个接口再去找人用。

规划里曾经有三份提案（daycore-core / contract / client）在造同一个包。合成
一个：三个包意味着三份版本、三次同步、以及「这个类型该放哪个」这个每周都要
回答一次的问题。

## 里面有什么

| 模块 | 是什么 |
|---|---|
| `paths` | `/api/v2` 前缀规则。⚠️ 后端权威在 `internal/apipath`，这里是它的镜像 |
| `http` | fetch 包装：会话 token、`X-Frontend-Build` 头、`ApiError` |
| `types` | 线上类型：TimeBlock / DayPlan / Proposal / Session / OperationLog / Handshake |
| `endpoints` | 每个调用一个函数 |
| `backend` | 后端地址（运行时配置）+ 首次运行判断 + buildHash |
| `i18n` | 四条规矩（见 `docs/specs/frontend-manifest.md`） |
| `boot` | 握手 → 装 build 头 → 开会话，顺序是承重的 |

## 里面**没有**什么，以及为什么

- **manifest 的内容**。每个前端声明自己的 token 空间和 family —— 那是它的身份，
  不是共享的。共享的只有它的**形状**（`TokenSpec` / `KindSpec`）。
- **「现在该做什么」的推导**。汀 的 `flow.ts` 是单件流的语义；长卷和顺流对
  「现在」的定义不一样。把它提上来会逼三个范式共用一个它们并不共享的概念。
- **任何 UI**。四端是**范式级不同**的，共用组件等于把它们变成一个东西的四个皮肤，
  而那正好是这个项目不想要的。

## ⬜ 阶段 κ 的一个未解问题

前端将来要变成 git 子仓（`docs/ROADMAP.md` 阶段 κ）。子仓没法 `file:../../packages/core`
—— 那时候要么发到私有 registry，要么 vendor 一份。**现在不解，因为解法取决于
那时候子仓怎么托管**，而提前选一个就是在给一个还没有的约束写代码。
