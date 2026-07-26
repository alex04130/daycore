# Daycore 仓库指南

## 布局

- `v2/` — 当前产品（Go 后端 + `v2/web/frontend/` Vite+React 前端）。**根目录的 `src/`、`package.json`、`plan.md` 等是 v1（Next.js）遗留，不要动。**
- `claude-design/` — Claude Design 的设计交付原件（设计系统 `_ds/`、原型 `app/`）。只读参考，不参与构建。
- `v2/api/` — API 契约唯一权威：`openapi.yaml` + `FRONTEND_HANDOFF.md`。
- `v2/docs/` — **实时项目文档**（架构/认证/Agent/数据/AI/路由总表）。

## 实时文档铁律

1. **任何代码改动，必须在同一批修改中更新 `v2/docs/` 对应文件**（改了认证就改 `AUTH.md`，加了路由就改 `API_SURFACE.md` + `openapi.yaml`，依此类推）。文档与代码不同步视为改动未完成。
2. **做规划/探索时先读 `v2/docs/`，不要派 agent 全量扫代码**；只对将要改动的文件做点状核实。发现文档与代码脱节时，先修文档再继续。

## 常用事实

- 版本唯一来源：`v2/internal/version/version.go`（同步 `v2/web/frontend/package.json`）。规则：2.<minor>.<patch>-beta。
- 构建验证：`cd v2 && go build ./... && go vet ./... && go test ./...`。
- 新增 domain 实体的完整路径：domain struct → repository.go 接口 → sqlstore（三方言 DDL）→ mongostore → 详见 `v2/docs/DATA.md`。
- 提示词模板必须 zh-CN / en-US 双 locale 同时存在，缺一启动报错（`v2/internal/ai/prompts.go`）。
