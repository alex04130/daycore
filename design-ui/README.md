# design-ui —— 前后端分离式前端（待导入）

本目录是 **Claude Design 导出前端**的落地点，目前为空占位。

## 定位

Daycore 当前有两个前端形态，本目录是后者：

| 目录 | 状态 | 说明 |
|---|---|---|
| [`web/frontend/`](../web/frontend/) | **现役** | Vite + React，与 Go 后端同仓同构建，生产由单二进制托管 `STATIC_DIR` |
| `design-ui/`（本目录） | **待导入** | 前后端分离架构下的独立前端；后端尚未跟进对应改造 |

## 来源

设计交付原件（设计系统 `_ds/` + 页面原型 `app/`）**不在本仓库内** —— 从
Claude Design 网页版获取。仓库内只保留实现副本：

- [`web/frontend/src/vendor/ds-bundle.js`](../web/frontend/src/vendor/ds-bundle.js)
  —— 设计系统 JS bundle（`window.DaycoreUI`，26 组件；含本地补丁）
- [`web/frontend/src/ds/`](../web/frontend/src/ds/) —— `styles.css` +
  `_ds_bundle.css`，design tokens 的权威来源

导入新前端时可直接复用这两处，无需再从网页版取一遍设计系统。

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
