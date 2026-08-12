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

## ⚠️ 它有三分之二不是库，是一份手写的契约镜像

| | 行数 | 跟着谁动 |
|---|---|---|
| `paths` / `types` / `endpoints` / `boot` / `manifest` | ~680 | **后端契约** |
| `i18n` / `backend` / `http` / `build` | ~430 | 自己 |

这不是抱怨，是一条能推出结论的事实：**镜像落后比没有镜像更糟** —— 它是一个自信的
错误答案。而唯一让它不落后的机制，是「改契约」和「改镜像」是同一个 commit。

三面镜子此前**一面都没有被检查过**。现在有了，在 Go 这边（`internal/server/core_client_test.go`）：

- `API_PREFIX` 必须等于后端的 `apipath.Prefix`（错了是**每一个请求同时 404**）
- 不加版本的路径两边必须一致（错了轻则一屏坏掉，重则**握手都发不出去**）
- `endpoints.ts` 里每一个 `/api/...` 都必须命中真实路由表

第三条封住的是一个三角形的第三条边：`routes_test.go` 早就在双向核对路由表与
openapi，但**没有任何东西核对过手写的客户端**。这不是假想 —— 早期草稿里的
`/api/plan/today` 就是从一个测试夹具里 grep 来的。

⚠️ 提取器会先剥注释，而且用的是扫描器不是正则。第一版扫原文，把本文件里一句
「`/api/plan/summary` 从来不存在」的说明**当成了调用**报出来。会对散文报警的闸门，
是那种失败会被挥手放过的闸门，而挥手的习惯才是杀死下一个真警报的东西。

## 阶段 κ：它已经是一个真的包了

子仓没法 `file:../../packages/core`，所以无论最后走 npm、走 git 依赖、还是 vendor
一份，**每一种都是装一份拷贝**而不是符号链接。那件事现在可以验证：

```bash
make check-core-pack               # 默认 web/liuli-classic
APP=web/ting make check-core-pack
```

它把这个包 `npm pack` 成 tarball，装进一份临时的前端拷贝，跑完整的
`tsc && vitest && vite build`。四个前端都过，而且产物哈希与工作区版本**逐字节相同**。

所以 κ 不是一次重构，是一次机械搬运 —— **渠道的选择被降级成一个可以以后再改的
决定**，因为包边界本身已经被证明是真的。

四条路都实测过（见 `docs/ROADMAP.md`）。两条要记住的事实：

- **npm 的 git 依赖不支持子目录**（实测 npm 11.13，`#path:` 被当成 commit-ish，
  `&path=` 直接被拒）。所以「git 依赖」和「嵌套 submodule」有同一个前提：**core
  得是自己的仓**。
- **超级仓根加 npm workspaces，会在树内自动覆盖掉 git 依赖**。一份 package.json
  两种世界：树内解析到 `packages/core`，独立 clone 回落到 git 依赖。

⚠️ `"private": true` 还留着，它是那道保险：`npm publish` 会被它挡住。选定渠道之前
不要删这一行。
