# 汀 · 此刻

单件流：**画面永远只回答一个问题 —— 现在做什么。**

四个前端里最小的一个，也是第一个真做的。路线图把它排在第一，理由是它
「最小却跑通完整写+撤销闭环，是暴露共享层设计错误最便宜的地方」
（`docs/ROADMAP.md` 阶段 ι）。

## 跑起来

```bash
npm install
npm run dev          # http://localhost:5175，/api 代理到 localhost:8080
DAYCORE_API=https://your-backend npm run dev
npm run build        # tsc + vitest + vite build → dist/
```

`npm run build` 里的 `vitest run` 不是可选的：`src/flow.ts` 是纯逻辑，而它算错
「现在该做什么」不会崩、只会显示错的东西 —— 那是这个界面唯一不能出的错。

## 它和后端的关系

**不内嵌进 Go 二进制**，跟运维控制台相反。控制台是版本锁死的、坏的时候正需要它，
所以内嵌；汀 是运行时协商的、将来要变成自己的子仓，内嵌等于把分离要剪的那根线
重新系上。

启动三步，顺序是承重的：

1. `POST /api/version` **握手** —— 自我介绍，拿到被分到哪个 family、哪些 token
   这个部署还校验不了。它必须最先，因为这是唯一不需要会话就能做的一步。
2. 设 `X-Frontend-Build` 请求头 —— 之后每次主题读写都按**我们的** token 空间判定。
3. `POST /api/session/init` 拿会话。

## ⚠️ 它需要第三档

`--tg-shadow` 是 `0 24px 70px rgba(0,0,0,.55)`，内置的六种 kind 一种都表达不了它
（`list-of<length>` 卡在第四位的颜色上）。所以汀 在 manifest 里**提议**一个
`shadow` kind，等运维在控制台点头。

没批准之前汀 照常跑 —— 样式表自己的值生效，少掉的只是「这一条能不能被主题改」。
这个降级就是整个设计：提议、继续工作、等人。

`internal/theme/frontend_manifest_test.go` 把这条 pattern **从这个文件里读出来**，
喂给后端真正的注册表，再拿 `theme.css` 里真正的 `--tg-shadow` 值去验。第一版
pattern 要求每一段都带单位，于是**拒绝了汀 自己出厂的那个值** —— 而 CSS 里
`0` 不带单位是合法的。两边语言不同，那个测试是它们唯一见面的地方。

## ⚠️ 没有乐观更新

每个动作都重读。`PATCH /api/plan` 会被计划闸门以 409 拒掉（锁住的、已石化的块），
提案也可能被另一个标签页抢先答掉。汀 的全部前提是「这一屏诚实地回答现在做什么」——
屏幕上显示一件已完成、而服务器认为还开着的事，是它唯一付不起的错。

代价是每个动作一次往返，在一个一次只显示一件事的界面上。这是便宜的那个方向。

## ⚠️ 会话用 token 不用 cookie

分开部署就是跨源，`dc_sid` cookie 需要 `SameSite=None` 加正确的
`ALLOWED_ORIGINS`，而且第三方 cookie 一关就整个丢掉。签名 token 放请求头到哪都
一样，而且自定义头会强制 CORS 预检 —— 那正是它免疫 CSRF 的原因。

代价说清楚：token 在 localStorage 里，汀 里的一个 XSS 就能把它拿走，cookie 不会。
接受这个取舍是因为汀 的前提就是被部署到别处，而一个静默持久化失败的会话比一个
已知暴露更糟。

## 还没做

- 导语面、心情帧、全天抽屉、账本 —— 原型里有，需要汀 还没调的端点。**不做假的**：
  接在 mock 形状上的界面是注定要重写的界面。
- 多语言。`docs/specs/frontend-manifest.md`「前端的多语言形态」有四条要求，现在
  一条都没实现，文案是写死的中文。这是已知欠账，不是没看见。
- 主题切换 UI（后端 `POST /api/session/theme` 已通，界面没给入口）。
