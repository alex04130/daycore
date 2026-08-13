# Daycore 仓库指南（浓缩版）

> **完整指南在 [`AGENTS.md`](AGENTS.md)** —— 仓库布局、技术栈、构建验证、代码约定、测试策略、部署与安全要点全在那里。本文只保留 AGENTS.md 没有的三样：目标架构、版本语义、实时文档铁律的判据，外加一条必须在本机跑的验证清单。两份文档的分工是刻意的：同一主题只在一处详述，避免「一份改了另一份漂了」。

## 目标架构（进行中）

前后端分离部署：Go 后端收敛成纯 API 服务，四个前端（琉璃 / 纸屿 / 汀 / 运营台）各自独立构建部署、做成 **git 子仓库**，与后端只靠 **API 契约 + 版本号**同步 —— 前端声明最低 API 支持，后端由 `GET /api/version` 报告。落地方案见 [`docs/EXPERIENCE_CORE.md`](docs/EXPERIENCE_CORE.md) 与 [`docs/ROADMAP.md`](docs/ROADMAP.md)。

## 版本语义

**版本号只有一个**（2026-08-09 合并，此前是三层）：`internal/version/version.go` 的 `Version` + `Channel`，现 `2.3.0-beta`。`APIVersion`/`APIMinor` 仍然存在、仍然由 `GET /api/version` 报，但**是从 `Version` 推导出来的** —— 线上形状没变，只是数字的来源变了。

**它是「第几个修改批次」不是发布号**：`2.2 → 2.3` 的意思是**一整份规划实现完了**，不是「加了些功能」。发布节奏：v2 beta → 小范围内测 → v2 继续 → 公测 → v3 正式版。v3 之后才切成普通语义化（大版本.小版本.bug修复），且与 `/api/v2` 路径前缀同源——见 [`docs/ROADMAP.md`](docs/ROADMAP.md)。

⚠️ **合并的代价说清楚**：前端没法再把「我要的契约至少这么新」和「我要的构建至少这么新」分开讲，两句话变成同一句。这是真的精度损失，有意接受 —— 今天没有任何前端在协商，所以它不花钱；等它开始花钱（前端在自己的仓库、按自己的节奏发版），再把数字拆回去。

各前端自己的版本号仍然独立（`design-ui/API_CONTRACT.md` 抬头的「v4」是那一层，与上面无关）。

## 实时文档铁律

1. **任何代码改动，必须在同一批修改中更新 `docs/` 对应文件**（改了认证就改 `AUTH.md`，加了路由就改 `API_SURFACE.md` + `api/spec/paths/<tag>.yaml` 再 `make api-bundle`，依此类推）。加/删路由不必靠自觉：`internal/server/routes_test.go` 与 openapi 双向核对，漏一边就红。

   **写的不只是「现在是什么样」，还必须包括三样**：
   - **边界**：什么是有意不做的、什么绝对不能加。例：不要给 L1 硬边界加 DB 覆盖或 admin 端点。
   - **取舍**：选了什么、放弃了什么、代价是什么。例：上传走原始字节而不是 multipart / base64。
   - **它防的是哪个具体失败**：不写「为了安全」，写「少了 `nosniff`，一个上传的 `.svg` 打开就是同源脚本」。

   判据很简单：**改这块代码的人不该需要先读一遍代码才知道哪些是有意为之**。这类约束被当成疏漏「顺手修掉」，是本仓最贵的一种回归。
2. **做规划/探索时先读 `docs/`，不要派 agent 全量扫代码**；只对将要改动的文件做点状核实。发现文档与代码脱节时，先修文档再继续。

## 改动后的必跑项（CI 已删除，验证全在本机）

```bash
gofmt -l .                                        # 必须为空
go build ./... && go vet ./... && go test ./...
go build -tags lite ./... && go test -tags lite ./internal/resources/ ./internal/setup/
go test -race ./internal/adapters/ ./internal/weather/
make cross                                        # windows / darwin / linux-arm64 编译 + vet
make test-mongo                                   # 真机 MongoDB（行为套件 69 例，看用例数别看 ok）
make test-sql                                     # 真机 PostgreSQL + MySQL
node web/frontend/scripts/check-i18n.mjs
```

**`make cross` 不是可选的**：`cmd/daycore/restart_windows.go` 这类 build-tag 文件在本机永远不会被编译，而一个没人编译的 build-tag 文件就是一个会悄悄编不过的文件。

⚠️ **丢掉的是什么，写清楚**：没有任何东西检查这台机器没跑过的分支；前端 `vite build` 与插件 zip 现在只有人跑才跑。

## 文档地图（其余都看 AGENTS.md）

| 想知道 | 看 |
|---|---|
| 完整指路 + 铁律 | [`AGENTS.md`](AGENTS.md) |
| 总规划（做什么、按什么顺序、为什么） | [`docs/ROADMAP.md`](docs/ROADMAP.md) |
| 架构 | [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) |
| 每个配置项能不能热改 | [`docs/CONFIG.md`](docs/CONFIG.md) |
| 数据层与四个存储后端 | [`docs/DATA.md`](docs/DATA.md) |
| 认证与 CORS | [`docs/AUTH.md`](docs/AUTH.md) |
| agent loop 与 SSE | [`docs/AGENT.md`](docs/AGENT.md) |
| AI 子系统 | [`docs/AI.md`](docs/AI.md) |
| 端无关产品语义 | [`docs/EXPERIENCE_CORE.md`](docs/EXPERIENCE_CORE.md) |
| 战略与功能审计 | [`docs/STRATEGY.md`](docs/STRATEGY.md) |
| 开发手册与分步骨架 | [`docs/DEVELOPING.md`](docs/DEVELOPING.md) |
| 路由总表（生成物） | [`docs/API_SURFACE.md`](docs/API_SURFACE.md) |
| 对外协议 | [`docs/specs/`](docs/specs/README.md) |
