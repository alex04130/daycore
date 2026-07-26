# 认证与 CORS

> 实时文档：改认证/中间件必须同批更新本文件。最后全面核对：2026-07-14（含双轨认证落地）。

## 凭证体系（双轨：cookie + header，header 优先）

| 凭证 | Cookie 轨 | Header 轨 | 内容与校验 |
|---|---|---|---|
| 匿名 session | httpOnly `dc_sid`（MaxAge 400 天） | `X-Session-Token: <同一签名串>`（session/init 带 `tokenInBody:true` 返回 `sessionToken`） | `sid.<hmac>`：`auth/session.go` CookieSigner（HMAC-SHA256 keyed by COOKIE_SECRET，常量时间比对）；sid = 32 字节 crypto/rand base64 |
| 登录用户 | httpOnly `dc_auth`（MaxAge=JWT_TTL） | `Authorization: Bearer <同一 JWT>`（login/register 带 `tokenInBody:true` 返回 `token`） | `auth/token.go` TokenIssuer：HS256，claims = Subject(userID) + `tv` + iat/exp；Parse 拒绝非 HMAC 签名法 |

- cookie 属性统一在 `server/server.go` 的 `setSessionCookie`/`setAuthCookie`/`clearAuthCookie`（HttpOnly / Secure=cfg.SecureCookies / SameSite=`sameSiteMode()`，由 `COOKIE_SAMESITE` 配置，none 需 Secure，config.Load 校验）。
- 纯 Bearer 客户端（无匿名 sid）注册/登录时，`issueAndLink` 会在 `sid=="" && DataSessionID==""` 分支自动铸造 canonical session（`handlers_auth.go`），此后经 userMW→dataSessionMW 解析 sid，无需 X-Session-Token。
- 版本契约：`GET /api/version`（公开）返回 `{apiVersion, apiMinor, build, channel, minClient}`；常量在 `internal/version/version.go`（APIVersion 破坏性才 +1）。

## token_version 撤销机制

- 权威存储：DB `users.token_version` 列（`domain/user.go` TokenVersion，json:"-"）。
- JWT 携带 `tv`；`TokenIssuer.Parse` 只回传 (subject, tv) 不比对。
- 比对点在 `server/middleware.go` userMW：`user.TokenVersion == ver` 才认。
- 撤销 = 登出时 `Users().IncrementTokenVersion`（UPDATE ... +1），全设备 JWT 即失效。

## 中间件（middleware.go，解析不强制）

- `sessionMW`：先读 `X-Session-Token` header，空则读 `dc_sid` cookie → CookieSigner.Verify → ctx `ctxSessionID`。
- `userMW`：先 `bearerToken(r)`（Authorization: Bearer），空则读 `dc_auth` cookie → Parse → 查库比对 tv → ctx `ctxUserID`+`ctxUser`。
- `dataSessionMW`：把 user.DataSessionID 放 ctx `ctxDataSessionID`（登录用户的 canonical data session）。
- handler 内 `requireSession`：优先 ctxDataSessionID，否则 ctxSessionID，都无 → 401 `no_session`。

## 登录流程（handlers_auth.go / handlers_auth_oauth.go）

- 注册/登录成功 → `finishLogin(w, r, user, tokenInBody)` → `issueAndLink`（签 JWT → setAuthCookie → 把匿名 sid 合并/关联到 canonical session；无 sid 且无 canonical 时铸造新 session）→ body `{ok, user}`，`tokenInBody:true` 时另含 `token`。
- 登出：`IncrementTokenVersion` + clearAuthCookie。
- OAuth：`dc_oauth_state` cookie（10min）→ callback 校验 → upsertOAuthUser → issueAndLink → 302 `/`。
- 全代码库唯一 `Authorization: Bearer` 出现在 OAuth **出站** userinfo 请求（auth/oauth.go），当前无入站 Bearer 路径。

## CORS（corsMW 四分支）

1. `/api/import/*` 前缀：`ACAO:*`，允许 `X-Import-Token`，**无 credentials**（浏览器扩展直推）。
2. wildcard（ALLOWED_ORIGINS 含 `*`）且有 Origin：反射 `*`，**明确不与 credentials 组合**（防 CSRF）。
3. 显式 allowlist 命中：回显 origin + `Vary: Origin` + `Access-Control-Allow-Credentials: true`。
4. 其余不设 CORS 头。OPTIONS 一律 204 短路。

分支 2/3 的 Allow-Headers 含 `Authorization, X-Session-Token`（header 轨凭证，必触发 preflight → 天然免 CSRF）。

## 鉴权旁路

- `X-Admin-Token`（handlers_admin.go `adminAuthorized`）：ADMIN_TOKEN 设置时常量时间匹配；未设 → dev 全开 / prod 关闭。
- `X-Import-Token`（handlers_import.go `importSession`）：无 cookie 时按 token 解析会话（扩展直推）。

## 公开端点（无需 session）

`GET /api/healthz`、`GET /api/version`、`GET /api/models`、`POST /api/session/init`、auth 系列（register/login/logout/providers/oauth/me）。

## 密码

argon2id，PHC 编码，per-user cost jitter，可选 `PASSWORD_PEPPER` HMAC 混入，并发上限 4（auth/password.go）。
