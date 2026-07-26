# Daycore · 纸屿 UI — 实现说明（HANDOFF）

> 读者：后端 / 全栈 Claude Code。本文描述纸屿 UI（`zhiyu/index.html` + `zhiyu/app/`）的结构与 mock→真实 API 的切换步骤。
> **API 契约以根目录 `API_CONTRACT.md` 为准**（琉璃 / 纸屿两套 UI 共用同一套 API，风格不同、同步演进）。

## 0. 快速开始
- 直接用静态服务器打开 `zhiyu/index.html`（babel-standalone 现场转译 JSX，开发够用；上线请预编译）。
- 全部数据在 `localStorage["dc4.state.v1"]` 的 mock 后端里。演示账号：
  - 用户 `demo@daycore.app / demo1234`；管理员 `admin@daycore.app / admin123`。
- 路由（hash）：`#/onboarding` `#/today` `#/materials` `#/companion` `#/mood` `#/settings`；管理台 `#/admin[/models|/env|/oauth|/users|/data]`（非 admin 无入口且被守卫拦截）。

## 1. 文件结构
```
index.html               入口（pinned React 18.3.1 + Babel；脚本顺序即依赖顺序）
app/
  dc.css                 设计系统「纸屿」：tokens(4 主题)+全部组件类（dc4-*）
  icons.js               window.DcIcons（stroke 图标，DcIcons.svg(name)）
  i18n.js                window.DcI18n（zh/en，t()/pick()/add()/setLocale()）
  store.js               window.DcStore 核心：session/auth/plan/autoPlan/courses/assignments/rules
  store-ext.js           inbox/imports/moods/themes/companion(流式)/memory/数据导入导出
  store-admin.js         admin* 全部管理台方法（含校验与错误码，镜像真实 API）
  ui.jsx                 window.DcUI 组件库（Button/Sheet/Toggle/Seg/Table/Toast/Confirm/DirtyBar…）
  main.jsx               路由壳、Rail/TabBar、AuthSheet、window.dcNav
  page-*.jsx             用户端五页 + onboarding（注册到 DcPages.<Name>）
  admin-*.jsx            管理台 shell + 5 页（DcPages.Admin<Name>）
```
新页面 = 写 `DcPages.X` 并在 `main.jsx` 的 map 中加一行；所有页面只通过 `DcStore.*` 取数。

## 2. 切换到真实后端（唯一需要动的层）
`DcStore` 是**唯一 API 边界**。保持方法签名与返回形状不变，把实现换成
`fetch('/api/…',{credentials:'include'})` 即可，页面零改动。

### 2.1 端点映射与领域形状

见根目录 **`API_CONTRACT.md`**（两套 UI 共用；本文件只保留纸屿实现细节）。

## 4. UI/设计契约（改后端不需要碰，但要知道）
- 主题 = `html[data-theme]` 四套内置 + `#dc4-theme-vars` 内联覆盖（`DcStore.applyThemeVars`）。自定义主题只覆盖 `--dc-*` 变量。
- i18n：所有文案经 `DcI18n.t(key)`；新增文案用 `DcI18n.add({key:{zh,en}})`，不要写死字符串。
- 撤销模型：任何批量改动（自主规划 / 采纳建议）前调 `DcStore.setOp(label,dates)` 存快照，`undoLast()` 恢复 —— 后端等价物：服务端保存 last-op snapshot 或返回逆操作。
- 管理台保存模式统一为「改动累积 → 底部 DirtyBar 一次 PUT 全量文档」；env 例外（增量 PUT map）。
- 陪伴页为独立聊天布局（`.dc4-chatpage`：header / 滚动消息区 / 置底输入条），输入条不随页面滚动。
- 资料页顶部常驻「导入」栏（Canvas / ICS / 截图 + 导入历史），不藏在 tab 里。
- 设置含「自定义提示词」（`profile.l2Prompt`，≤2000 字，onBlur 自动保存）。

## 5. 已知边界 / TODO
- OAuth 为模拟（1.4s 后返回假账号）；接真后端时改成整页跳转 `location.href=/api/auth/oauth/{p}/start`。
- 截图导入返回固定示例课程；接真后端换成 multipart 上传。
- babel-standalone 仅限开发；生产请用任意打包器把 `app/*.jsx` 预编译成一个 bundle（无其它构建依赖）。
- 主题（自定义）与语言存本地，不随账号同步。
