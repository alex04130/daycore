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

- **管理面双路**（`handlers_admin.go` `adminAuthorized` + `handlers_admin_session.go`）：机器走 `X-Admin-Token`（常量时间比较），人走 `dc_admin` httpOnly cookie。**没有「全开」这一档了** —— 见下。
- `X-Import-Token`（handlers_import.go `importSession`）：无 cookie 时按 token 解析会话（扩展直推）。

### ⚠️ 管理面鉴权待改造（2026-07-29 记，落地在批次 F4）

**先纠一个常被以为的问题**：token 走的是**请求头**，不是 URL 查询参数，比较也已经是 `subtle.ConstantTimeCompare`（`handlers_admin.go:23-28`）。所以「URL 会泄露」在现有实现上不成立，管理面也没有 EventSource（那是 token 被塞进 URL 的经典原因，因为 `EventSource` 设不了请求头）。**线上那一段是对的，问题在别处。**

真正的暴露面有三处：

1. **明文密钥长期躺在浏览器里。** 控制台把它存进 `sessionStorage`（`design-ui/liuli/admin/admin-store.js:19-26`），运维手输到密码框。控制台上任何一个 XSS 都能读走它 —— 而它**永不过期**，读走就是永久有效。
2. **只能靠重新部署轮换。** 它是环境变量，没有 TTL、没有吊销、没有身份（谁改了那个模型配置，账本上分不出来）。对照 `dc_auth`：JWT 有 `JWT_TTL`，logout 靠服务端 token version 一次吊销全部 —— 同一套机制管理面一点没用上。
3. **`ADMIN_TOKEN` 未设 = dev 全开。** 一个 `APP_ENV != production` 的部署，没设 token 就是一个**无鉴权的管理 API**。

**方向**：机器路径（curl / CI）保留 `X-Admin-Token`；**人的路径换成一次登录交换** —— `POST /api/admin/session` 拿 admin 凭证换一个 httpOnly + Secure + SameSite=Strict 的短 TTL JWT cookie（`scope: admin`）。基建全都在：`internal/auth/token.go` 的签发与 token version 吊销、`ALLOWED_ORIGINS`、`COOKIE_SAMESITE`。这样明文密钥再也不进 JS 可读的存储；cookie 是 httpOnly，XSS 读不到（但**同源仍然用得上它**，所以 CSRF 要靠 SameSite=Strict + Origin 校验，不能只靠 cookie 本身）。

⚠️ **cookie 方案有个前提：真的有 TLS。** `Secure` cookie 在纯 HTTP 上不会被发送，而 `deploy/nginx.conf` 只 `listen 80`、没有 ssl 段，`SECURE_COOKIES` 默认 `false`。所以：**头那条路不能删** —— 它在纯 HTTP 上照样能用（虽然那时候什么都在明文里），而 cookie 那条路会**静默地登不上**。控制台在这种情形下必须说清「你没有 TLS，所以管理登录不可用」，而不是给一个转圈的登录框。参考部署要不要加 TLS 段是另一件事，但**协议不能假设它已经有了**。

✅ **F4a 已落地（2026-08-07）。** 三个问题各自的修法：

| 原问题 | 现在 |
|---|---|
| 明文密钥长期躺在 `sessionStorage`，**永不过期**，XSS 读走就是永久有效 | `POST /api/admin/session` 用原始 token 换一次 `dc_admin` cookie：**httpOnly**（控制台上的 XSS 读不到）、`Secure`、`SameSite=Strict`、30 分钟。原始 token 再也不进 JS 可读的存储 |
| 环境变量 → 轮换要重新部署，无 TTL、无吊销、无身份 | TTL 就是吊销（见下）。`DELETE /api/admin/session` 清 cookie |
| **`ADMIN_TOKEN` 未设 = dev 全开** | **这一档没有了**。`config.Load` 未设时**生成一个并在启动日志里打出来**；生产环境未设直接拒绝启动 |

**「未设 = 全开」是三条里最危险的一条**，因为它命中的是每一台开发机、每一个 staging、每一个自部署但没设 `APP_ENV` 的实例 —— 一个**无鉴权的配置 API**。生成一个反而两头都好：本地开发只是从日志里复制一次，而**任何时刻都不存在「没有凭证」这个状态**。生产不给生成 —— 一个没人写下来的 token 是一个没人能轮换的 token。

**scope 是必须的**：user JWT 与 admin JWT 用同一把密钥签，没有 `scp` 声明的话**一个用户会话就能认证管理 API** —— 而用户令牌是任何人注册一下就有的。两个方向都校验（`Parse` 拒绝 admin scope，`ParseAdmin` 拒绝 user scope），因为一道守卫守不住一条边界的两侧。

**CSRF 两道，缺一不可**：cookie 会跟着跨站请求一起发出去，所以 httpOnly 买到的是**保密性不是权限**。`SameSite=Strict`（浏览器不会附带）+ **Origin 校验**（给不认 SameSite 的浏览器或流程）。**请求头那条路两道都不需要** —— 自定义请求头会触发预检，跨源攻击者过不去；所以带 `Origin` 的机器请求不会被误伤。

### ⚠️ Origin 校验最容易的死法：反代

**只拿 `Origin` 比 `r.Host` 会打死每一个反代部署。** 浏览器发的是 `https://daycore.example.com`，而进程看到的 `r.Host` 完全取决于代理配置 —— 而 **nginx 的默认值对我们是错的**：`proxy_pass` 不写 `proxy_set_header Host $host` 时，Host 是上游地址（`app:8080`），于是每一个管理请求都 403，且那个 403 看起来像应用的 bug。

所以可接受来源是一个集合，按权威度排：

| 来源 | 为什么可信 |
|---|---|
| `PUBLIC_BASE_URL` | **最权威**。运维本来就必须填它，否则 OAuth 回调不工作 —— 也就是说它是**另一个功能已经在验证的值** |
| `X-Forwarded-Host` | 另一种常见代理写法，**只在 `TRUST_PROXY_HEADERS` 为真时采信**（客户端能设的头就是客户端能撒谎的头，这里撒的谎是「我是同源」）。代理链是追加的，取第一段 |
| `r.Host` | 直连的情况，以及代理正确转发 Host 的情况 |
| `ALLOWED_ORIGINS` | 运维的显式决定 |

**scheme 有意不比**：TLS 在代理那里终结，https 的请求到这个进程时是明文 http。比 scheme 会**恰好拒绝那些 TLS 做对了的部署**。host + port 才是站点身份；攻击者要做 scheme 降级需要的网络位置，那时 CSRF 是最小的问题。

### 部署要点（与上面这条同源，写在一起）

- 反代后面通常不需要特意做什么：真实部署里那个上游端口本来就不对外（compose 不 publish、裸机绑 loopback）。`HOST=127.0.0.1` 只是把这件事写死一层，**不是要求**。真要注意的是别在同一台机器上既开着反代又把应用端口暴露出去 —— 那时绕开的是 nginx 的一切，包括限流与 TLS。
- **内网明文 http 必须留 `SECURE_COOKIES=false`**：`Secure` cookie 在 http 上根本不会被发送，在那里打开它 = 运维被自己的控制台锁在外面，而且**任何地方都不会报错**。生产默认为真是对的，前提是那里有 TLS。
- `deploy/nginx.conf` 里三行是「不写就静默出事」的：`Host $host`（不写 → 管理面 403）、`proxy_buffering off`（不写 → SSE 变成转圈到答案写完）、`Upgrade`/`Connection`（不写 → WebSocket 握手拿到 200 而 socket 永远不开）。已在该文件顶部逐条写明，并跑过 `nginx -t`。

⚠️ **admin token 没有吊销机制**，因为 token version 吊销要查库，而降级启动的整个前提就是**在存储挂掉时把控制台端上来**。所以 **TTL 就是吊销**，30 分钟，**调大它是安全改动不是便利改动**。登出前被复制走的 token 会一直有效到过期 —— 这是「必须能在没有数据库时工作」的诚实代价。

**这条设计的每一处都不碰数据库**，那正是 F4a 与降级启动必须同批设计的原因：一个需要查行的管理登录，会在最需要它的时刻停止工作。

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


## 权限与管理员：作者裁决（2026-08-09）

> 状态：**裁决已定，实现未开始。** 落地条件见文末。

### `ADMIN_TOKEN` 就是 root 密码

不是「旧机制」，是**这套设计里最高的一层**：

- **它通过一切**，包括忘了标权限的路由。
- **改不了** —— 它是环境变量，没有任何界面能改它。
- **不传输**：任何端点都不返回它、不回显它、不以任何形式（前缀、长度、掩码）暴露它。前端在换到 cookie 之后就不再持有它。
- **它是唯一的恢复路径**：最后一个管理员被删、被降级、忘了密码、角色表写坏了、数据库根本没起来 —— 全部走它。

⚠️ 「不要传输」有一处不可避免的例外，说清楚：`POST /api/admin/session` 的请求体里必须带它一次，因为**它就是那次交换的凭据**。除此之外它不在网络上出现。

### 忘标 = 拒绝

一条新的 `/api/admin/*` 路由如果没声明权限，**默认拒绝**（root 仍然通过）。反过来做会让「加了个端点忘了标」变成一个静默的洞，而那正是这一批实测到的那种洞。

### 新增权限，已有的角色默认没有它

正常逻辑，也是安全的那个方向：新权限落地时没有人自动获得它。**超级管理员是一个标记而不是「拥有全部权限」的角色**，正是为了让这条不会把 owner 一起锁在外面 —— 标记在检查函数里早退，新权限一被定义他当场就有，零数据变更。

### 第一个管理员由后台设置

**不是「第一个注册的用户自动成为管理员」** —— 在一个开放注册的部署上，那意味着第一个路过的人拿到了整个控制台。第一个管理员由持有 `ADMIN_TOKEN` 的人在控制台里提拔。

### 权限从已经取到的那一行读，不放 JWT

这条本来准备当成「省一次查询 vs 撤销窗口」的取舍来权衡，**但那个取舍不存在**：

`internal/server/middleware.go:154` —— 每一个带 `dc_auth` 的请求**已经在查库了**（`GetByID` 核对 `TokenVersion`），并且把用户行塞进 ctx 给下游复用。

所以：
- 「放 JWT 省一次查询」的前提是空的 —— 那次读无条件发生。
- 「改权限就把 JWT 判超时」也不需要 —— `TokenVersion` 存在的理由是「JWT 里的东西改不了」；权限不在 JWT 里就没有要作废的东西。
- **撤销窗口是 0**，不是「一个 TTL」。

### 一切都是权限，包括授予本身（作者裁决，2026-08-09）

审查提的三条分歧，作者的答案合起来是同一个立场：**没有「口令轨独占」这种硬编码的禁区。root 通过一切，其余一切靠授予。**

**① 授予权限本身是一个权限。**

不是一个特殊分支、不是「只有 owner 能授予」。它就是清单里的一条，可以授予、可以收回。

⚠️ 但要写清它的性质，因为它读起来像普通一条而实际不是：**持有它的人可以给自己任何东西**，所以授予它等价于把对方变成 owner。这是所有 RBAC 都有的性质、不是这里的缺陷 —— 但控制台在那个开关旁边必须说出这句话，否则运维会以为自己只是发了「一个管理权限」。

**② 破窗轨可授可撤。** 它本来就是 root。

此前的顾虑是「偷到 `ADMIN_TOKEN` 的人清空管理员表、把合法所有者锁在外面」。那不成立：`ADMIN_TOKEN` 在生产必然存在（未设直接拒绝启动），所以合法所有者手上永远有同一把钥匙，被清空后重新提拔即可 —— 最坏代价是一次拉锯，不是不可恢复。

而反过来「谁都不能撤销」**才是真正不可恢复的**：若「破窗只授不撤」加「角色轨不能授予」同时成立，这套设计里**没有任何人能移除一个管理员**，只能手工改库。

**③ `chat_messages` / `mood_checkins` / `memory_facts` 也是可分配的权限。**

不做成硬编码的禁区 —— **这是运维的活**。谁能看用户的对话与情绪记录，是那个部署的负责人要做的决定，不是代码替他做的决定。

⚠️ 由这条推出一个粒度要求：**数据库浏览器不能只有一个 `db.browse`**。可浏览表里既有 `sessions`/`operation_logs` 这类运维数据，也有 `chat_messages`/`mood_checkins`/`memory_facts` 这类与陪伴边界同级的信任数据 —— 两者必须能分开授予，否则「可分配」这句话在界面上没有落点。判据仍是那一条：**是否存在一个现实的人，你会给他 A 而不给他 B** —— 这里显然存在（一个查问题的运维要看 `operation_logs`，不需要看日记）。

### 落地条件

⬜ **控制台要先有 ≥2 个真视图**，且存在一个具体的人「你会给他 A 不给他 B」。在此之前权限清单里被真正差异化检查的条目是 **0** —— 那会是本仓第七次「写完、测过、没人调用」。
