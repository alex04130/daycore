# design-ui —— 前后端分离式前端（待导入）

本目录是 **Claude Design 导出前端**的落地点，目前为空占位。

## 定位

Daycore 当前有两个前端形态，本目录是后者：

| 目录 | 状态 | 说明 |
|---|---|---|
| [`web/frontend/`](../web/frontend/) | **现役** | Vite + React，与 Go 后端同仓同构建，生产由单二进制托管 `STATIC_DIR` |
| `design-ui/`（本目录） | **待导入** | 前后端分离架构下的独立前端；后端尚未跟进对应改造 |

## 来源

设计交付原件在仓库根的 [`claude-design/`](../claude-design/)：

- `claude-design/_ds/` —— 设计系统（design tokens、组件样式）
- `claude-design/app/` —— 页面原型

现役前端已把设计系统 vendor 进 `web/frontend/src/vendor/ds-bundle.js` 与
`web/frontend/src/ds/`（含本地补丁），并非直接读取 `claude-design/`。

## 接入前需要确认的事

分离式前端要独立部署，就不能再依赖「Go 二进制托管静态文件」这条路径，
以下几项需要先在后端落地：

1. **CORS** —— `ALLOWED_ORIGINS` 需加入新前端的域名（见 `internal/config/config.go`）
2. **鉴权** —— 现有 `dc_sid` 是 httpOnly cookie，跨域场景需确认
   `SECURE_COOKIES` + `SameSite` 策略（见 `docs/AUTH.md`）
3. **API 契约** —— 以 [`api/openapi.yaml`](../api/openapi.yaml) 与
   [`api/FRONTEND_HANDOFF.md`](../api/FRONTEND_HANDOFF.md) 为唯一权威
4. **i18n** —— zh-CN / en-US 双 locale 必须对齐，参照
   `web/frontend/scripts/check-i18n.mjs` 的校验规则

## 约定

导入代码后，请同步更新 [`docs/ARCHITECTURE.md`](../docs/ARCHITECTURE.md)
与根 [`README.md`](../README.md) 的项目结构章节 —— 本仓库要求文档与代码
同批更新（见 `CLAUDE.md` 的「实时文档铁律」）。
