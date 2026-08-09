# Daycore 仓库指南

## 布局

仓库根即实现面 —— Go 单二进制后端 + Vite/React 前端，无子项目分层。

- `cmd/daycore/` + `internal/` — Go 后端（18 个包，module 名 `daycore`）。
- `web/frontend/` — **现役** Vite+React 前端，生产由 Go 二进制托管 `STATIC_DIR`。**将来会被四端替换**，替换完成前不要动它（任何时刻都要有能跑的东西）。
- `design-ui/` — **设计原型**（只读参考，不参与构建）：四套范式级不同的前端（琉璃·长卷 / 纸屿·顺流 / 汀·此刻 / 琉璃初版）+ `core/daycore-core.js` 共享 mock + `HANDOFF/` 交接文档 + `_ds/` 设计系统。重建时逐行对照它。**注意其 `API_CONTRACT.md` 的路径命名不是权威**，详见该目录 `CLAUDE.md` 顶部裁决。
- `api/` — API 契约唯一权威：`openapi.yaml` + `FRONTEND_HANDOFF.md`。**`openapi.yaml` 是生成物**，改的是 `api/spec/paths/<tag>.yaml`（一个 tag 一个文件），然后 `make api-bundle`；`go test ./...` 会因它过期而红。
- `docs/ROADMAP.md` — **总规划**：做什么、按什么顺序、为什么是这个顺序（排序原则是「成本曲线」，不是并发度）。规划此前只活在仓库外的计划文件里，那是个断点。
- `docs/` — **实时项目文档**（架构/认证/Agent/数据/AI/路由总表/开发者手册）+ `EXPERIENCE_CORE.md`（端无关语义总纲，v2.3）。
- `docs/specs/` — **对外协议**（别人照着实现什么）：`transport.md` 传输层四种适配层共用（HTTP 语义 + 子进程握手与生命周期）、`storage-protocol.md`、`provider-protocol.md`、`frontend-manifest.md`。改协议要同时想「第三方照这个写会不会踩坑」。
- `extension/` — Chrome MV3 插件（抓 Canvas → 推 `POST /api/import/canvas`）。
- 设计系统已 vendor 进 `web/frontend/src/ds/` 与 `src/vendor/ds-bundle.js`；原件在 `design-ui/_ds/`。

## 目标架构（进行中）

前后端分离部署：Go 后端收敛为纯 API 服务，四个前端各自独立构建部署、做成 **git 子仓库**，与后端只靠 **API 契约 + 版本号**同步 —— 前端声明最低 API 支持，后端由 `GET /api/version` 报告。落地方案见 `docs/EXPERIENCE_CORE.md` 与计划文件。

**版本号只有一个**（2026-08-09 合并，此前是三层）：`internal/version/version.go` 的 `Version` + `Channel`，现 `2.3.0-beta`。`APIVersion`/`APIMinor` 仍然存在、仍然由 `GET /api/version` 报，但**是从 `Version` 推导出来的** —— 线上形状没变，只是数字的来源变了。

**它是「第几个修改批次」不是发布号**：`2.2 → 2.3` 的意思是**一整份规划实现完了**（vNext 路线图从头到尾），不是「加了些功能」。

⚠️ **合并的代价说清楚**：前端没法再把「我要的契约至少这么新」和「我要的构建至少这么新」分开讲了，两句话变成同一句。这是真的精度损失，有意接受 —— 今天没有任何前端在协商，所以它不花钱；等它开始花钱（前端在自己的仓库、按自己的节奏发版），再把数字拆回去，比先靠手工对齐两个数字好几个月要便宜。

各前端自己的版本号仍然独立（`design-ui/API_CONTRACT.md` 抬头的「v4」是那一层，与上面无关）。

## 实时文档铁律

1. **任何代码改动，必须在同一批修改中更新 `docs/` 对应文件**（改了认证就改 `AUTH.md`，加了路由就改 `API_SURFACE.md` + `api/spec/paths/<tag>.yaml` 再 `make api-bundle`，依此类推）。文档与代码不同步视为改动未完成。加/删路由不必靠自觉：`internal/server/routes_test.go` 与 openapi 双向核对，漏一边就红。

   **写的不只是「现在是什么样」，还必须包括三样**（2026-08-06 补，此前它们一直只活在代码注释与 commit message 里 —— 那两处都不可检索、也不随代码演进）：
   - **边界**：什么是有意不做的、什么绝对不能加。例：不要给 L1 硬边界加 DB 覆盖或 admin 端点。
   - **取舍**：选了什么、放弃了什么、代价是什么。例：上传走原始字节而不是 multipart / base64。
   - **它防的是哪个具体失败**：不写「为了安全」，写「少了 `nosniff`，一个上传的 `.svg` 打开就是同源脚本」。

   判据很简单：**改这块代码的人不该需要先读一遍代码才知道哪些是有意为之**。这类约束被当成疏漏「顺手修掉」，是本仓最贵的一种回归。
2. **做规划/探索时先读 `docs/`，不要派 agent 全量扫代码**；只对将要改动的文件做点状核实。发现文档与代码脱节时，先修文档再继续。

## 常用事实

- 版本唯一来源：`internal/version/version.go`（同步 `web/frontend/package.json`）。规则：2.<minor>.<patch>-beta，**minor 只在一整份规划完成时 +1**。发布节奏：v2 beta → 小范围内测 → v2 继续改 → 公测 → v3 正式版。
- 构建验证：`go build ./... && go vet ./... && go test ./...`（或 `make test`，会先跑 i18n 校验）。
- **加一个配置项必须同批分类**（`internal/config/layer.go` 的 `Settings`：启动期 / 运行时 / 是不是密钥）—— 不分类 `go test` 直接红。判据与生成的总表见 `docs/CONFIG.md`；`make config-doc` 重生成。
- 新增 domain 实体的完整路径：domain struct → repository.go 接口 → sqlstore（三方言 DDL）→ mongostore → 详见 `docs/DATA.md`。
- **存储层改动的验收标准是 `internal/storage/storagetest` 的行为套件**（43 个用例，**四个后端跑同一份**）。它测的是四个后端必须一致的**行为**，`dialect_parity_test.go` 是三方言 DDL 的**静态**比对，两者互不替代。本机跑真机那几个：`make test-mongo`（Mongo）、`make test-sql`（PG + MySQL）。
- **SQL 里存自由数据不必只会「一个大 JSON 整体重写」**：需要条件写就用 JSON 路径写（`json_set` 配 `WHERE json_extract`，SQLite/PG/MySQL 都支持，已实测），键集开放且要按键查就用侧表。**凡是出现在 `WHERE` 里、或被算术/`CASE` 更新的字段必须是列** —— 塞进 blob 就退成读-改-写，那是正确性取舍不是性能取舍。
- 提示词模板必须 zh-CN / en-US 双 locale 同时存在，缺一启动报错（`internal/ai/prompts.go`）。**L1 硬边界是唯一例外**：`internal/ai/prompts/boundaries.json`，只有磁盘（`PROMPTS_DIR/boundaries.json`）+ 内嵌两层，**不要给它加 DB 覆盖或 admin 端点** —— 能从控制台改的边界等于能被删。
- **Go 里的用户可见文案一律 `i18n.Register` 注册 key + `i18n.T`/`Tf` 取用**，不要写 `if HasPrefix(locale,"en")`，也**不要直接 `i18n.Pick`**（会绕开 DB/文件两层，让这条字符串变成不可翻译的）。语言包三层：DB → `LOCALES_DIR/<locale>.json` → 内嵌 zh-CN/en-US；**加一门语言是丢一个翻译文件，不是改代码也不是发版**。主副语言由用户自己在设置页选，配置只给默认值。详见 `docs/DATA.md`「多语言机制」。
- 许可证：LGPL-3.0-or-later（`COPYING.LESSER` + `COPYING`）。引入新依赖前确认其协议兼容（Apache-2.0 / MIT / BSD / MPL-2.0 可以；GPL-only、SSPL、专有协议不行）。
- **验证全部在本机**（2026-08-09 起：CI 已删除，此前是 `.github/workflows/ci.yml` 四个 job）。一条也不少，而且比 CI 那份**更强** —— 它包含 CI 从来没有的 `-tags lite`、`-race` 与子进程 e2e：

  ```bash
  gofmt -l .                                        # 必须为空
  go build ./... && go vet ./... && go test ./...
  go build -tags lite ./... && go test -tags lite ./internal/resources/ ./internal/setup/
  go test -race ./internal/adapters/ ./internal/weather/
  make test-mongo                                   # 真机 MongoDB
  make test-sql                                     # 真机 PostgreSQL + MySQL
  node web/frontend/scripts/check-i18n.mjs
  ```

  ⚠️ **丢掉的是什么，写清楚免得有人以为还有人在看**：没有任何东西检查这台机器没跑过的分支；前端 `vite build` 与插件 zip 现在只有人跑才跑。别人来提交之前要把它加回去。
- 历史：v1（Next.js + Eazo SDK）已于建库时移除，可从首个 commit 取回。代码中出现的 `v2` 字样如无特别说明均指外部 API 版本号（如 Google OAuth、QWeather），不要当作目录路径改写。
