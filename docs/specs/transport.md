# 传输规范（HTTP 与子进程）

> Daycore 所有外部适配层共用这一份：存储、天气、搜索、消息通道。
>
> 每份协议文档只写**它自己的握手载荷与操作**，传输本身的规则在这里 —— 一字不差的东西抄三遍必然漂移。

## 为什么是两种

同一套操作，传输两种，因为它们的**失败模式**不同。

| | `format: http` | `format: exec` |
|---|---|---|
| 形态 | 适配层是个服务 | 后端启动并监管的子进程 |
| 位置 | 可在别的机器、可并发扩容 | 与后端同机 |
| 崩溃时你看到什么 | 连接被拒 —— 只知道它没了 | **退出码 + stderr**，能说出为什么 |
| 重启 | 归运维／编排 | 后端自己带策略 |
| 适用 | 已有的远端服务、要横向扩容 | 本机适配器、开发期、以及「崩了要能诊断」 |

**`exec` 不是 `http` 的降级。** 在可观测性上它是更好的那个：一个 HTTP 适配层崩掉只留下 `connection refused`，而被监管的子进程给出退出码和 stderr，能直接说是配置错了、依赖缺了、还是被 OOM 杀了。

> 为什么不用 Go 的 `plugin` 包：只支持 Linux/macOS，要求插件与主程序用**完全相同的工具链与依赖版本**编译，加载后无法卸载。做第三方生态不可用。所以「子进程／服务 + 协议」不是绕开 Go 的弱点，**它就是 Go 的标准答案**（HashiCorp 的 go-plugin 正是这么做的）。

## 配置

所有适配层在 `config/providers.yaml` 里同构声明 —— `format` 绑实现，其余是它的参数：

```yaml
weather:
  - id: my-weather
    format: http
    base_url: https://wx.example.com
    token_env: MY_WEATHER_TOKEN
storage:
  - id: my-store
    format: exec
    command: ["/usr/local/bin/daycore-store-adapter", "--config", "/etc/adapter.toml"]
    ready_timeout: 10s
    restart: on-failure          # never | on-failure | always
    max_restarts: 5
```

## HTTP 规范

### 基本约定

- **HTTP/1.1 或 HTTP/2**，请求与响应都是 `application/json; charset=utf-8`。
- 路径统一带协议版本前缀：`/v0/…`。协议不兼容变更升这个数字，**不复用旧前缀**。
- 认证：`Authorization: Bearer <token>`，token 从 `token_env` 指的环境变量读。适配层**必须**在配了 token 时校验它。
- 后端每个请求带 `X-Daycore-Request-Id`，适配层应把它写进自己的日志 —— 跨进程排查全靠它。

### 状态码的语义是契约，不是风格

| 码 | 含义 | 后端会怎么做 |
|---|---|---|
| `200` / `201` | 成功 | 用响应体 |
| `400` | 请求本身不合法（字段缺失、`op` 不认识） | **不重试**，当成编程错误报出去 |
| `401` / `403` | 认证/授权失败 | **不重试**，标记适配层不可用 |
| `404` | 资源不存在 | 映射成 `domain.ErrNotFound` |
| `409` | **冲突**（唯一键、CAS 失败） | 映射成 `domain.ErrConflict`，走各自的重试逻辑 |
| `429` | 限流，应带 `Retry-After` | 退避重试 |
| `5xx` | 适配层自己坏了 | 退避重试，超次数标记不可用 |

⚠️ **`409` 与 `5xx` 的区分是硬要求。** 存储协议里「插入即占有」这个机制完全依赖它：插入失败要能分辨「别人已经占了这个场次」与「写失败了」。两者混成一个码，后端会把一次故障当成一次正常落选 —— worker 静默变闲，任何地方都没有一句话说明原因。同理，一次超时**不能**被适配层自己转成 409。

### 超时与重试

- 后端给每个调用一个 deadline，并通过 `X-Daycore-Deadline-Ms` 告知剩余毫秒。适配层**应当**据此裁剪自己的上游调用，别做注定超时的工作。
- 重试只针对 `429` 与 `5xx`，退避 1s／2s／4s，最多三次。
- 后端会关掉超时请求的连接。适配层**必须**尊重 `ctx` 取消（Go 之外的实现：监听连接关闭），否则会攒下没人要的在途工作。

### 健康

```
GET /v0/capabilities
```

每个协议定义自己的 capabilities 载荷（见各自文档）。后端在启动与断线重连后各调一次；**它同时充当健康检查**，不需要额外的 `/healthz`。

## 子进程规范（`format: exec`）

### 握手

1. 后端启动 `command`，环境里放入：

   | 变量 | 含义 |
   |---|---|
   | `DAYCORE_ADAPTER_PROTOCOL` | 协议版本，如 `0` |
   | `DAYCORE_ADAPTER_KIND` | `storage` / `weather` / `search` / `channel` |
   | `token_env` 指定的变量 | 如果配了 |

2. 子进程在 **stdout 第一行**输出一行 JSON 并**立刻 flush**：

   ```json
   {"protocol":"0","transport":"unix","address":"/tmp/daycore-store-8f3a.sock","pid":12345}
   ```

   - `transport`：`unix` 或 `tcp`（后者 `address` 形如 `127.0.0.1:54321`）。
   - **优先 unix socket** —— 不占端口、权限跟着文件走、也不会被别的进程连上。
   - 各协议可以在这一行里追加自己的字段（存储追加 `capabilities`，通道追加 `manifest`），见各自文档。

3. 后端连上去，调该协议的 capabilities 端点验证，然后正常使用。

4. **stdout 在握手行之后不再被解析**。日志走 **stderr**。

`ready_timeout` 内没有握手行 → 杀掉进程、按 `restart` 处理、把 stderr 的最后若干行写进启动错误。**这一段正是 `exec` 存在的理由** —— 同样的失败在 HTTP 那边只能表达成「连不上」。

### 生命周期

- **关停**：后端发 `SIGTERM`，等 5 秒；未退出则 `SIGKILL`。子进程收到 `SIGTERM` 后应停止接受新请求、让在途请求做完、然后 **exit 0**。
- **退出码**：`0` = 正常结束。**非 0 一律记录并按 `restart` 策略处理**，退出码与最后 32 KiB 的 stderr 一起进日志和控制台。这是它比 HTTP 强的地方，别浪费。
- **重启退避**：1s、2s、4s…上限 30s。连续失败到 `max_restarts` 就放弃 —— 后端把这个适配层标成不可用，而不是无限重试。
- **不要 daemon 化**，不要 fork 之后让父进程退出。后端要能通过进程树看见你。

### 硬约定

- **stdout 只有那一行握手。** 任何多余的 stdout 输出都是协议噪音；调试信息全走 stderr。
- socket 文件由**子进程**创建、**子进程**在退出时删除；后端不管它。
- 密钥走**环境变量**，**不要写在 `command` 的参数里** —— 参数在 `ps` 里人人可见。
- 不要假设自己是唯一实例：多实例部署下后端可能同时跑多个，`address` 必须每进程唯一。
- 子进程的工作目录、`umask`、资源限制**不由**后端设置。需要什么自己在启动时设。

### 一个最小的握手实现（任何语言都是这个形状）

```
# 1. 建 socket
sock=$(mktemp -u /tmp/daycore-XXXXXX.sock)
start_http_server_on_unix_socket "$sock"

# 2. 打印一行、flush、然后别再碰 stdout
printf '{"protocol":"0","transport":"unix","address":"%s","pid":%d}\n' "$sock" $$
exec 1>&2          # 之后所有输出都当日志

# 3. 收到 SIGTERM 时优雅退出，删掉 socket，exit 0
trap 'graceful_stop; rm -f "$sock"; exit 0' TERM
wait
```

## 通用的失败语义

- **适配层不可用** ≠ **功能不存在**。查询型（天气、搜索）按 `providers.yaml` 的顺序**降级到下一个**；存储型不可用就是不可用，后端拒绝启动或标记为故障。
- 适配层**不应**替后端做重试。后端知道 deadline、知道预算、知道这次调用是否值得重试；适配层的重试只会把一次超时变成三次。
- 适配层**不应**缓存，除非它自己就是缓存。天气的 30 分钟缓存在 Daycore 侧（`internal/weather`），适配层再加一层只会让「为什么数据是旧的」变得说不清。
