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

- `X-Admin-Token`（handlers_admin.go `adminAuthorized`）：ADMIN_TOKEN 设置时常量时间匹配；未设 → dev 全开 / prod 关闭。**⚠️ 待改造，见下。**
- `X-Import-Token`（handlers_import.go `importSession`）：无 cookie 时按 token 解析会话（扩展直推）。

### ⚠️ 管理面鉴权待改造（2026-07-29 记，落地在批次 F4）

**先纠一个常被以为的问题**：token 走的是**请求头**，不是 URL 查询参数，比较也已经是 `subtle.ConstantTimeCompare`（`handlers_admin.go:23-28`）。所以「URL 会泄露」在现有实现上不成立，管理面也没有 EventSource（那是 token 被塞进 URL 的经典原因，因为 `EventSource` 设不了请求头）。**线上那一段是对的，问题在别处。**

真正的暴露面有三处：

1. **明文密钥长期躺在浏览器里。** 控制台把它存进 `sessionStorage`（`design-ui/liuli/admin/admin-store.js:19-26`），运维手输到密码框。控制台上任何一个 XSS 都能读走它 —— 而它**永不过期**，读走就是永久有效。
2. **只能靠重新部署轮换。** 它是环境变量，没有 TTL、没有吊销、没有身份（谁改了那个模型配置，账本上分不出来）。对照 `dc_auth`：JWT 有 `JWT_TTL`，logout 靠服务端 token version 一次吊销全部 —— 同一套机制管理面一点没用上。
3. **`ADMIN_TOKEN` 未设 = dev 全开。** 一个 `APP_ENV != production` 的部署，没设 token 就是一个**无鉴权的管理 API**。

**方向**：机器路径（curl / CI）保留 `X-Admin-Token`；**人的路径换成一次登录交换** —— `POST /api/admin/session` 拿 admin 凭证换一个 httpOnly + Secure + SameSite=Strict 的短 TTL JWT cookie（`scope: admin`）。基建全都在：`internal/auth/token.go` 的签发与 token version 吊销、`ALLOWED_ORIGINS`、`COOKIE_SAMESITE`。这样明文密钥再也不进 JS 可读的存储；cookie 是 httpOnly，XSS 读不到（但**同源仍然用得上它**，所以 CSRF 要靠 SameSite=Strict + Origin 校验，不能只靠 cookie 本身）。

⚠️ **cookie 方案有个前提：真的有 TLS。** `Secure` cookie 在纯 HTTP 上不会被发送，而 `deploy/nginx.conf` 只 `listen 80`、没有 ssl 段，`SECURE_COOKIES` 默认 `false`。所以：**头那条路不能删** —— 它在纯 HTTP 上照样能用（虽然那时候什么都在明文里），而 cookie 那条路会**静默地登不上**。控制台在这种情形下必须说清「你没有 TLS，所以管理登录不可用」，而不是给一个转圈的登录框。参考部署要不要加 TLS 段是另一件事，但**协议不能假设它已经有了**。

⚠️ **与存储降级启动直接冲突的一条**：降级模式下没有 DB，**token version 吊销就不可用**（签发只要 `JWT_SECRET`，吊销要查库）。所以降级模式里那个 admin JWT 的 TTL 必须显著更短，而且 `ADMIN_TOKEN` 未设时**绝不能**沿用「dev 全开」—— 否则「存储挂了仍然把控制台端上来」就等于把一个无鉴权配置界面挂到网上。降级模式必须要求显式凭证，没有就只给一个说明页。

## 有意公开的信息披露

`GET /api/healthz` 免鉴权地返回 `db`（存储引擎名）、`env`、`version`、`channel`。**这是知情的取舍，不是疏忽**，记在这里免得每次审计都重新「发现」一遍：

- `db` **有真消费者** —— 浏览器插件的「测试连接」按钮把它显示给用户（`extension/options.js:107`），那是部署的拥有者在检查自己的服务器。
- `version`/`channel` 无论如何都是公开的：`GET /api/version` 按设计就是公开的（前端要在建立会话之前协商契约）。所以在 healthz 里藏它们毫无意义。
- 剩下的边际侦察价值只有「引擎名 + 环境名」。handler 已经划过一次线，而且划在更要紧的地方：ping 失败时**不回传驱动的错误原文**，因为那里面有 DSN 与主机片段。

⚠️ 但这条意味着一件事要记住：**healthz 的响应随存储后端变化**。所以它不能被当成「与后端无关的契约面」来引用 —— 第五个存储后端接上来时，这个字段会返回一个新值，而 openapi 里它是 `type: string` 无枚举，这是对的。

## 公开端点（无需 session）

`GET /api/healthz`、`GET /api/version`、`GET /api/models`、`POST /api/session/init`、auth 系列（register/login/logout/providers/oauth/me）。

**这份名单现在由测试强制**（`internal/server/auth_surface_test.go`）：对 `RouteTable()` 里每一条不在名单上的路由发一个不带任何凭证的请求，**必须收到 401**；反向也查 —— 名单里有而路由表里没有的条目是过期豁免，同样报错。

⚠️ **2026-07-30 修掉的一个真洞**：`POST /api/ai/plan-text`、`plan-image`、`extract-schedule-image` 三条**只有 IP 限流、没有 `requireSession`** —— 也就是能连到这台机器的任何人都能烧运维的模型额度，而其中两条还是 vision（最贵的那种）。三方文档都说它们该鉴权：openapi 的全局 `security` 适用于它们（它们没有声明 `security: []`）、本文这张公开表从来没有它们。加 `requireSession` 是安全的：`sid` 在那个文件里零出现，三个 handler 从不碰会话，没有任何东西依赖匿名可用。

顺带把另外四个 AI 端点的**认证提到了解析之前**（原来先 `readJSON` 后查会话，返回 400 而不是 401）。这不是洞，但先解析后认证会让未鉴权的调用方能探测 body 校验，也让「每条非公开路由都回 401」没法成为一条可检查的不变量。

**自带凭证轨**（不走 session，因此也在名单上）：`X-Import-Token` 的两条导入、`X-Admin-Token` 的管理面、以及 `POST /api/channels/{channel}/verify` —— 最后这条是 body 里的绑定 token 作凭证，调用方是通道那一侧的 bot，按构造就没有会话。

## 密码

argon2id，PHC 编码，per-user cost jitter，可选 `PASSWORD_PEPPER` HMAC 混入，并发上限 4（auth/password.go）。
