# 前端 manifest 与主题协议（v1）

> 状态：**协议已定案、未实现**。落地是批次 F7。
>
> 给**写前端的人**看的 —— 包括第三方前端、以及同一系列的不同平台（琉璃可以有 web、app、嵌入式）。
>
> 设计裁决与理由在 [`docs/EXPERIENCE_CORE.md` §1.2](../EXPERIENCE_CORE.md)，这里是实现要照着做的那部分。

## 三条硬要求

任何前端 —— 包括第三方做的、别的平台的 —— 必须做到：

1. **握手并自我介绍**（`POST /api/version`）。不介绍，后端分不清连上来的是谁，主题与偏好就无处归属。
2. **首次安装配置界面**（`/setting` 或同等结构），至少能配**连哪个后端**。把后端地址写死的前端只能对着一个部署用，而自部署是这个项目的常态。这个界面同时是语言开关与主题选择的落点。
3. **自主丢弃用不到的主题 token**。主题按 family 存、token 空间取并集，所以你会拿到本 build 不认识的变量 —— 忽略它们是你的责任，后端不为此裁剪。

## 两层身份

| | 是什么 | 谁定 |
|---|---|---|
| **`buildHash`** | 一次构建的指纹（manifest 的哈希） | 派生，不可改 |
| **`familyId`** | 主题兼容组（如 `liuli`） | build 自己声明；**运维可在控制台改** |

主题、按端偏好、`ThemeSwitch` 全部按 **family** 存 —— 那才是一套主题有意义的单位。`buildHash` 让控制台看得见「琉璃 有三个 build 在连：web 4.2、app 1.0、嵌入式 0.9」，也让运维把一个新 build 归到已有 family。

## 握手

```
POST /api/version
  {
    "familyId":   "liuli",
    "buildHash":  "sha256:…",
    "displayName":"琉璃 · 长卷",
    "version":    "4.2.0",
    "minApi":     1,
    "theme": {
      "tokens": [
        {"name":"--primary",     "kind":"color",  "description":"主色（按钮/选中态）"},
        {"name":"--glass-blur",  "kind":"length", "description":"玻璃层模糊半径"},
        {"name":"--glass-alpha", "kind":"ratio",  "description":"玻璃层不透明度"}
      ],
      "builtins": [ {"id":"sky","name":"天空蓝","dark":false,"variables":{…}} ],
      "rules":    "琉璃的玻璃质感依赖 …（可选，见「rules 的处理」）"
    }
  }

  200 {
    "apiVersion": 1, "apiMinor": 0, "build":"2.3.0-beta", "channel":"beta",
    "minClient":  "…",
    "locales":    {"available":["en-US","zh-CN"],"defaultPrimary":"zh-CN","defaultSecondary":"en-US"},
    "assignedFamilyId":      "liuli",     // 运维可能改过，用这个，不要用你发的那个
    "rulesAccepted":         false,       // 未批准 → 后端用机械生成的规则
    "newTokens":             ["--glass-alpha"],
    "pendingThemeBackfill":  7            // 有 7 套主题缺新 token，等运维触发补算
  }
```

`buildHash` 一致就是空操作，所以每次启动调一次很便宜。`GET /api/version` 保持原样，给匿名与老客户端读。

## token 空间取并集，不做子集拦截

琉璃-app 用到的和琉璃-web 不必一致。

- **带来 family 没有的新 token** → family 的 token 空间扩一条。此时**已存的主题全都缺这一条**，运维在控制台跑一次**补算**：每个缺 token 的主题一次 AI 调用，按它现有配色补上。要花钱，所以是显式动作。
- **用不到 family 里的某些 token** → 不用管，**前端自己丢弃**。
- 补算后若两套主题变量完全一致 → **合并**，会话偏好改指。

控制台显示的是**差异**而不是通过／不通过：新增了哪几个 token、多少主题待补算、大约多少次调用。

⚠️ **同名 token 在两个 build 里 `kind` 不同是真冲突**（`--accent` 一边是颜色一边是长度，主题就没意义了），控制台必须报出来让人处理。

## `kind` 是服务端已知的封闭集合

`themeColorRe` 现有的注释点明了它守的东西：**收窄的字符集让「通过变量值注入 CSS/HTML」不可能**。放开 token 空间时这条不能丢。

**从后端的封闭集合里挑，每种的校验器归后端。前端不能自带正则，也没有 `raw` 这一档。**

| `kind` | 值形状 | 例 |
|---|---|---|
| `color` | hex / `rgb()` / `rgba()` / `hsl()` / `hsla()` / `transparent` | `#f472b6` · `rgba(255,255,255,0.72)` |
| `length` | 数字 + 白名单单位（`px` `rem` `em` `ch` `vh` `vw`） | `12px` · `1.5rem` |
| `number` | 纯数字 | `1.6` |
| `ratio` | 0..1 | `0.75` |
| `duration` | 数字 + `ms`/`s` | `200ms` |
| `enum` | 前端列出字面允许值，后端只比对 | `blur` / `none` |

新增一种 `kind` 是**改后端**（加一个校验器），不是前端在 manifest 里塞一段正则。这个不便利是刻意的 —— 它是把值层面注入面钉在后端的那颗钉子。

AI 生成的主题落地前**逐 token 校验**：key 必须在 family 的 token 空间里，value 必须过该 token 的 kind 校验器。模型被诱导输出别的东西，进不了库。

## `rules` 的处理

`theme.rules` 是「怎么给这一端设计主题」那段提示词片段。

- 前端**可以**主动发一段。
- 运维在控制台**可以改它、决定是否采用**。
- **没有主张、或运维没同意 → 后端按 token 清单机械生成**（token 名 + kind + 描述），前端照样能用 AI 配主题，只是提示词是后端写的。

这个顺序是有意的：**未经批准的客户端文本不进 LLM，注入面默认为零。** 第三方前端一上来就能用完整功能，代价只是主题提示词平淡一点。响应里的 `rulesAccepted` 告诉你当前用的是哪一种。

⚠️ **前端是开放的** —— 第三方与多平台是一等场景，所以上报内容按第三方数据对待，不按「我们自己的代码」对待。`description` 同样有长度上限并去换行。

## 主题的读写

```
GET   /api/themes                 本 family 的内置 + 自定义主题
POST  /api/themes                 建一套自定义主题（variables 逐 token 校验）
POST  /api/ai/theme               自然语言 → 一套主题（走 theme_gen 提示词）
PATCH /api/session                {"currentTheme": "<themeId>"}  当前主题，按 family 分开
```

**当前主题按 family 分开存**（`SessionPrefs` 的 `{familyID: themeID}`）—— 桌面用琉璃、手机用汀 是常态。

## 没有传输规范，这是有意的

适配层是**后端调出去**，前端是**前端调进来**。所以这里没有子进程、没有 `format`、没有生命周期 —— 硬塞一段传输规范只会让人以为前端也要写成适配器。前端只需要一个 HTTP 客户端。

## 未定的口子

- **主题的暗色处理**：`dark` 现在是一个布尔，四端里有的需要「跟随系统」第三态。
- **`builtins` 的更新语义**：新 build 改了某个内置主题的取值，已经选了它的用户看到的是旧值还是新值，未定。
- **family 的所有权**：第一个声明 `liuli` 的 build 定义它的 token 空间；运维 pin 之后才不可被覆盖。名字抢占的防护未定。
