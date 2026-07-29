# Daycore · 给 AI 助手的入口

这个文件曾经是 845 行的仓库指南。**2026-07-29 收缩成一份指路**，因为它烂掉了：上次实质更新是 2026-07-25，之后路由改分散注册、openapi 改生成、批次 C 六张表、存储行为一致性套件、三层语言目录五次大改，一次都没落到它身上。

烂掉的文档比没有文档更贵。它是给 AI 助手看的入口，所以每一条过期都会**被照着执行**：

- §10.1 教「在 `server.go` 的 `Handler()` 里 `mux.HandleFunc`」—— 那里已经没有集中清单了，而且照做还会漏掉契约同步，`go test` 直接红。
- §4 的目录树把 `api/openapi.yaml` 当手写文件 —— 它是生成物，手改会被 `make api-bundle` 覆盖。
- §6 说 `Store` 有 16 个子 repository（实为 27）、`mongostore` 是一个 1419 行单文件且无测试（实为 26 个文件 + 真机行为套件）。
- §9.4 只说「提示词双语 + 前端 `i18n.js` 平行 key」，漏掉了现在最要紧的那条：**Go 里的用户可见文案必须走 `i18n.Register` + `i18n.T`**。照它写会写出不可翻译的字符串。

它独有的那几节已经搬走，没有内容丢失：代码约定与分步骨架 → [`docs/DEVELOPING.md`](docs/DEVELOPING.md)，加 AI wire-format 的骨架 → [`docs/AI.md`](docs/AI.md)。

## 从哪读起

1. **[`CLAUDE.md`](CLAUDE.md)** —— 仓库铁律、布局、常用事实。**先读这个。**
2. **[`docs/DEVELOPING.md`](docs/DEVELOPING.md)** —— 项目结构、代码约定、「加一条路由 / 一个工具 / 一个模型 / 一门语言」的分步骨架、开发命令。

然后按问题去：

| 想知道 | 看 |
|---|---|
| 架构、包结构、中间件、启动关停、配置分层 | [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) |
| 实体、四个存储后端、加表加列、迁移事故史 | [`docs/DATA.md`](docs/DATA.md) |
| 认证三轨、CORS、鉴权旁路 | [`docs/AUTH.md`](docs/AUTH.md) |
| agent loop、工具带、决策卡 | [`docs/AGENT.md`](docs/AGENT.md) |
| provider、提示词分层、加 wire-format | [`docs/AI.md`](docs/AI.md) |
| 有哪些 HTTP 路由、各在哪个文件 | [`docs/API_SURFACE.md`](docs/API_SURFACE.md)（生成的） |
| 端无关的产品语义（时间三层、提案、注意力阶梯、默契） | [`docs/EXPERIENCE_CORE.md`](docs/EXPERIENCE_CORE.md) |
| **别人照着实现什么**（存储 / provider / 前端适配层） | [`docs/specs/`](docs/specs/README.md) |
| API 契约 | [`api/openapi.yaml`](api/openapi.yaml)（**生成物**，源在 [`api/spec/`](api/spec/README.md)）+ [`api/FRONTEND_HANDOFF.md`](api/FRONTEND_HANDOFF.md) |

## 三条最容易踩的

1. **`api/openapi.yaml` 是生成物。** 改 `api/spec/paths/<tag>.yaml` 然后 `make api-bundle`。文件名必须等于 operation 的 tag，合并器会强制。
2. **路由在自己的 handler 文件里注册**（`init()` 里 `registerRoutes`），不在 `server.go`。加一条路由要连着改 openapi 分片、升 `APIMinor`、跑 `make api-surface` —— 三样都有测试盯着，漏一样就红。
3. **`docs/` 与代码同批更新。** 这不是风格要求：这个文件本身就是不守它的后果。

## 验证

```bash
gofmt -l .                      # 必须空，CI 有门禁
go build ./... && go vet ./... && go test ./...
make test-mongo                 # 真机 Mongo 那半（本机要有 mongod）
node web/frontend/scripts/check-i18n.mjs
```
