# 05 · 琉璃初版 · 页面制（对照组）

> 源码：`liuli-classic/index.html` + `liuli-classic/app/*`
> 这是项目最早那一版，**作为第四个 UI 保留下来当对照组**——用来看清另外三个「非常规」到底非常规在哪。

## 范式一句话

**最老实的那一版**：底部/侧边 Tab 栏、五个页面（今日 / 资料 / 陪伴 / 心情 / 设置）、纵向卡片列表。没有画布，也没有手势——所有能力都摆在明面上。

## 1. 结构

| 页面 | 文件 | 内容 |
|---|---|---|
| 今日 | `page-today.jsx` | 周条 + 「自主规划」大按钮 + 手动添加 + 块卡列表；≥900px 右侧多一栏（日历 + 临期作业） |
| 资料 | `page-materials.jsx` + `materials-parts.jsx` + `capture-flow.jsx` | 随手记收集箱 + 三入口导入（Canvas / ICS / 课表截图）+ 记录/学业/数据源三个 tab + 规则列表 |
| 陪伴 | `page-companion.jsx` | 对话 + 工具调用卡 + 决策卡 |
| 心情 | `page-mood.jsx` | 12 个情绪格 + 最近打卡 |
| 设置 | `page-settings.jsx` + `settings-theme.jsx` | 助手名 / L2 提示词 / 主题工作室（含 AI 生成）/ 管理台入口 |

- `≥900px`：`.dc-rail` 左侧竖向导航（logo + 5 项 + 底部账户卡）；`<900px`：底部 `TabBar`。
- 依赖 **DaycoreUI 设计系统**（`_ds/` 原件已删；用仓库内副本 `web/frontend/src/vendor/ds-bundle.js` + `src/ds/`）：`GlassCard` `Badge` `Sheet` `Confirm` `Segmented` `Field` `Calendar` `TimeBlockCard` `TabBar` `EmptyState`。
- 页面层样式 `liuli-classic/app/app.css`（**必须是 v3 版那份**，见 §4 陷阱）。

## 2. 桥接层：旧数据模型 ↔ 共享账本

`liuli-classic/app/store-bridge.js`（载入顺序：`store.js` → `store-ext.js` → `core/daycore-core.js` → `store-bridge.js`）

初版有自己的一套模型（`plans[date].blocks[]`，字段是 `time/duration_min/completed`）。桥接层把**核心变成唯一真相**，投影成旧形状，写操作全部改道核心：

```
块        Block → {id,date,time:start,title,type,duration_min:dur,
                   completed:status==='done',time_mode:lockLevel==='hard'?'fixed':'floating',
                   locked:lockLevel,lockReason,missed:status==='missed'}
plans     今天 ±21 天，每天一条 {id,date,blocks[],sourceType:'auto'}
作业      core.assignments → 旧形状（courseId/status/pointsPossible…）
课程      从作业的 course 字段归纳
心情      core.moods → {mood: e+' '+l, aiResponse: note, createdAt}
记忆      core.memories → state.memory + keyFacts
资料      core.materials → state.materials（kind→category：academic→study，其余→note），本地条目排在后面
主题      core.customThemes；session.currentTheme ← core.themes.liuli
助手名/语言 core.state 根（用 C.patch({...}) 改，不是 setPref）
```

写操作改道：`patchPlan`→`addBlock/setMark/setNote/setBlockFields/moveBlock/removeBlock`；
`autoPlan`→`gapsOf` + `addBlock(…, true)`；`addMood`→`moodRecord`；
`patchAssignment`→`setAssignmentDone`；`addMaterial/deleteMaterial`→`materialAdd/materialRemove`；
`addMemoryFact/deleteMemoryFact`→`memoryAdd/memoryRemove`；主题四件套→核心同名方法。

**回声防护**：`C.subscribe` 里重投影 + `S.emit()`；所有写操作包一层 `guard()`，写时置 `echo = true` 避免自己触发自己，`finally` 里 `syncAll()`。

### 规则（rules）：核心没有这个实体

核心只有块，没有"重复规则"。桥接层的处理：

- **列表 = 归纳 ∪ 自存**。归纳：扫未来 28 天，按标题聚合，出现 ≥2 次或带锁的算一条规则，标 `managed: true`。自存：用户通过 `createRule/saveRulesBatch` 建的，存在桥接层自己的 `localStorage['daycore-liuli0-rules']`，标 `managed: false`。
- **归纳规则一律不可写。** `patchRule/deleteRule` 对它们返回 `{error:'managed'}`，UI 上开关换成一个「由共享账本管理」标记、行也不可点进编辑器。
  > 原因：归纳出来的周期只是**从现存块反推的猜测**。把猜测当真规则去展开，2 次出现会被推成 `by_weekday:[0,6]` 然后铺满 28 天 —— 一次开关凭空造出 7 节课。这条路必须直接断掉，而不是去调展开逻辑。
- **自存规则可增删改停用**：`spawn()` 铺未来 28 天（跳过已有同名块），`despawn()` 撤掉 **含今天** 的所有 `planned` 同名块。停用只清占位、**保留记录**，所以能来回切（29 → 0 → 29 幂等）。
- **一条规则 = 一条 op**：`spawn/despawn` 走 `C.bulkBlocks()`，避免一次开关往账本里灌 29 条记录、把撤销栈冲垮。

## 3. 与另外三端的关系

场景切换（健康日 / 危机日 / 冷启动）在初版里也立刻生效——实测健康日 3/6、危机日 2/5、冷启动空白。这是"共用账本"最直观的证据。

## 4. 重建时的两个陷阱

1. **样式表必须配对。** 这一版的 markup 用了 75 个类（`dc-rail-*` `dc-today-side` `dc-matrow-*` `dc-thread-*` `dc-capture` `dc-decision` `dc-itin-*`…）只在 v3 版 `app.css` 里有。配了旧版 `app.css` 的结果是：这些区域退化成裸文字流——图标和文字连成一串、日历摊平、行内挤在一起。**看到"部分区域完全没样式"先怀疑样式表版本，不要去调 flex。**
2. **不要给归纳规则加写能力。** 见 §2。
