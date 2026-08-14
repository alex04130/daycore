
# Daycore 设计原型 — 用户操作与逻辑全量清单

> 范围：`liuli-classic`（对照组 page-based 前端）+ `liuli/admin`（琉璃·运营台）。
> 符号：`S` = `window.DcStore`（旧模型，`store.js` + `store-ext.js`）；`C` = `window.DcCore`（共享账本核心，`core/daycore-core.js`）。
> **桥接**：`store-bridge.js` 覆盖一部分 `S.*`，把写操作改道 `C.*` 并做「投影 + 回写 + 规则降级 + echo-guard」；未覆盖方法仍走旧模型。
> 标签为实际渲染中文（已解析 i18n）；`t('key')` 同时标注 key。

## 桥接边界速查

**被桥接（改道 DcCore）：** `getPlan` `ruleOccursOn` `patchPlan` `autoPlan` `addMood` `completeMoodExercise` `patchAssignment` `patchSession` `aiTheme` `createTheme` `patchTheme` `deleteTheme` `addMaterial` `deleteMaterial` `addMemoryFact` `deleteMemoryFact` `clearMemory` `createRule` `saveRulesBatch` `patchRule` `deleteRule`。

**未被桥接（仍走旧模型）：** `todayIso`/`addDays`/`dayDiff`/`weekdayOf`/`toMin`/`toHM`/`uid`、`importCanvasDemo`、`icsCandidatesDemo`、`shotCandidatesDemo`、`planFromText`、`createAssignment`、`register`/`login`/`oauth`/`logout`/`genToken`、`setOnboarded`、`inboxProcess`/`inboxCommit`/`aiTravel`、`searchMaterials`、`listCategories`/`setCategory`/`catEnabled`、`patchPreferences`、`logOp`/`revertOp`、`bindChannel`/`verifyChannel`/`unbindChannel`、`listThreads`/`createThread`/`deleteThread`/`sendAsync`/`respondDecision`、`versionInfo`。

---

# 一、liuli-classic/app/main.jsx

**用户操作**
- 底部 TabBar 5 标签：**今日**(`tab_today`) / **资料**(`tab_materials`) / **陪伴**(`tab_companion`) / **心情**(`tab_mood`) / **设置**(`tab_settings`) → `nav.go(k)`
- 左侧窄栏 nav 同样 5 标签（`dc-rail-item`）→ `nav.go(it.key)`
- 顶栏头像按钮（aria-label＝**账户** `au_account`）→ `setAccountOpen(true)`
- 窄栏底部用户条（`dc-rail-user`）→ `setAccountOpen(true)`
- `AccountSheet` 关闭 → `setAccountOpen(false)`
- TweaksPanel：今日页 → 规划面板 `TweakRadio`（**带摘要**`brief` / **纯表单**`plain`）→ `setTweak('apVariant',v)`；**显示 AI 备注**开关 → `setTweak('showAiNote',v)`；随手记 → **显示 AI 置信度**开关 → `setTweak('showConfidence',v)`

**逻辑 / 阈值**
- 未 onboarding 时整页被 `<P.Onboarding>` 取代（`if (!st.onboarded)`）。
- 启动即 `applyCurrentTheme(S.state)`（首帧前应用主题）；`useEffect` 在 `st.session.currentTheme` / `st.customThemes.length` 变化时重应用。
- `greetKey()`：<5 夜深了 / <11 早上好 / <13 中午好 / <18 下午好 / <23 晚上好 / 其余 夜深了。
- `nav.go(next,p)`：`setParams(p||null); setTab(next); window.scrollTo(0,0)`；`nav.clearParams()`。
- tweaks 默认 `{apVariant:'brief', showAiNote:true, showConfidence:true}`。
- 标题栏：今日页主标题 `app_name`（Daycore），陪伴页 `assistantName`，其余为标签名；今日页附加问候 + 用户名。

---

# 二、liuli-classic/app/page-onboarding.jsx

**用户操作**
- 第 0 步 **开始**（`ob_start`）→ `setStep(1)`
- 第 1 步三张导入卡 `fakeImport(k)`：**Canvas 作业与成绩**（`ob_import_canvas`）/ **课程表 .ics**（`ob_import_ics`）/ **课表截图**（`ob_import_shot`）
- 第 1 步 **下一步**（`next`）→ `setStep(2)`；**先跳过**（`ob_skip`）→ `setStep(2)`
- 第 2 步 **生成我的第一份计划**（`ob_generate`）→ `generate()`；**先自己看看**（`ob_generate_skip`）→ `S.setOnboarded(true); onDone()`
- 第 1/2 步返回按钮（`back`）→ `setStep(step-1)`
- `GenOverlay` 完成 → `S.setOnboarded(true); onDone()`

**逻辑 / 阈值**
- `fakeImport(kind)` 幂等守卫 `if (imported[kind] || busy) return`。
- canvas → `S.importCanvasDemo()`（未桥接）；其余 → `S.saveRulesBatch(ics/shots candidates)`（桥接 → spawn 铺块）。
- 模拟延迟：shot 1900ms、其余 1300ms；完成显 `ob_imported_badge`「已导入」。
- `generate()` → `S.autoPlan({from:T,to:T,mode:'keep_manual'})`（桥接 → gap 填块），延迟 3400ms。
- `counts`：c=课程、a=pending/planned 作业、r=启用规则；`hasAny=c||a||r`；`nDue`=有 dueAt 的 pending 作业数（用于 `gen_status_2`）。
- `imported` 初始 `canvas=!!st.canvasImportedAt`。

---

# 三、liuli-classic/app/page-today.jsx

## 3.1 AutoPlanSheet
**用户操作**
- 范围 `Segmented`（`ap_range`）：**今天/明天/未来 3 天/未来 7 天/自定义…**
- 自定义日历点选 `pickDay(d)`（`ap_pick_days`「点选起止日（最多 7 天）」）
- 附加要求 `Textarea`（`ap_instructions`）
- 模式（`ap_mode`）：**保留我改过的**(`keep_manual`) / **全部重来**(`replace_all`)
- **开始规划**（`ap_submit`，`disabled={!resolved}`）→ `submit()`
- 错误内联 **去导入**（`err_go_import`）→ `onClose(); goMaterials()`

**逻辑 / 阈值**
- `resolved`：today[ T,T ]、tomorrow[ T+1,T+1 ]、d3[ T,T+2 ]、week[ T,T+6 ]、custom 用 from/to（缺 to 用 from）。
- `pickDay` 若 `to<from` 交换；`dayDiff(to,from)>6` 则 `to=addDays(from,6)`（上限 7 天）。
- `submit()` → `S.autoPlan(...)`（桥接），延迟 3600ms；错误映射 no_material→err_no_material（附去导入）/ range_too_large→err_range_too_large / 其余→err_generic。
- brief 徽标：`ap_day_badge_rules`（{n}节课=启用规则数）、`ap_day_badge_due`（{n}项截止=0..14 天 pending 作业）、前 2 条 `keyFacts`。

## 3.2 BlockDetailSheet
**用户操作**
- 名称 `Input`（`bd_name`）；类型 chips 任务/约定/休息/放松/饮食；时间 `type=time`（`bd_time`，空显「不定时」）；时长 `type=number min=5 step=5`（`bd_duration`）；固定时间开关（`bd_fixed`「自主规划不会挪动它」）
- **标记完成/取消完成**（`bd_completed`/`bd_uncomplete`）→ `toggleComplete()`
- **保存**（`save`，`disabled={!dirty}`）→ `save()`
- **编辑这条规则**（`bd_edit_rule`，仅 rule 且有 onEditRule）→ `onEditRule(rule)`
- 删除：规则源 **今天不再显示**（`bd_hide_today`）/ 非规则 **删除日程**（`bd_delete`）；非规则弹 `Confirm`（确认键 **删除**）；规则源说明 `bd_hide_sub`

**逻辑 / 阈值**
- `isRule=block.origin==='rule'||!!block.rule_id`；`rule=state.rules.find(id===block.rule_id)`；来源徽标 `bd_origin_auto/rule/manual`。
- `dirty`=名称/类型/时间/时长/time_mode 任一变化。
- `save()` → `S.patchPlan update`（桥接）；toast「已保存」。**桥接仅当 `ch.time!==cur.start` 才 `C.moveBlock`（带 `_confirmed:true`）**。
- `toggleComplete()` → `patchPlan update {completed}`（桥接 → `C.setMark`）；完成 toast「完成 +1 ✓」。
- `remove()` → `patchPlan remove`（桥接 → `C.removeBlock`）；规则源 toast「今天已隐藏，规则保留」，否则「已删除」。
- ⚠️ 桥接 `toOld` 恒 `origin:'auto'|'manual'`、`rule_id:null` → isRule 恒 false、rule 恒 null，「编辑规则/今天不再显示」分支不出现。

## 3.3 ManualAddSheet
**用户操作**
- `Segmented`：**文字描述**(`di_tab_text`) / **截图识别**(`di_tab_image`)
- 目标日期 chips（`di_target`「添加到」）：今天/明天/原 date
- 文本 `Textarea`（占位 `di_text_ph`）；**识别并添加**（`di_parse`）→ `parse()`
- 截图 **选择图片**（`di_choose_image`）→ 文件输入 → `parseImageDemo()`
- 候选行点击切换 checked；**返回**（`back`）；**保存 {n} 条**（`confirm_save_n`）→ `confirmAdd()`

**逻辑 / 阈值**
- `parse()` → `S.planFromText(text,target)`（未桥接），延迟 1100ms；no_schedule_info→err_no_schedule_info。
- `parseImageDemo()`：1600ms 产出 2 条固定候选（组会 10:00 60min 约定 / 写实验报告 15:00 120min 任务），预勾选。
- `confirmAdd()`：剥离 checked 后 `S.patchPlan(target,{action:'add',block})`（桥接 → `C.addBlock`）；toast「已添加 {n} 条日程」。

## 3.4 TodayPage
**用户操作**
- `WeekStrip` 换日 / `onJump` 跳转日历；**回到今天**（`back_to_today`）→ `setDate(T)`
- 主 CTA **自主规划**（`autoplan_cta_title`）→ `setApOpen(true)`；**手动添加**（`manual_add`）→ `setAddOpen(true)`
- 空状态 **导入资料，让我来安排**（`empty_import`）→ `nav.go('materials')`；**直接自主规划**（`empty_autoplan`）→ `setApOpen(true)`
- 块卡片点击 → `setDetail(b)`；完成按钮 → `completeBlock(b)`
- 侧栏日历 / 进度 / 临期作业行点击 → `nav.go('materials')`；跳转日历浮层（`month_pick_title`「跳转日期」）+ **回到今天**

**逻辑 / 阈值**
- `completeBlock` → `S.patchPlan update {completed}`（桥接 → `C.setMark`）。
- 过去态 `past=!!b.time&&date===T&&toMin(time)+(duration||45)<nowMin()`。
- 定时/不定时分组；「现在」线 `nowInserted` 只插一次；进度「{done}/{total} 已完成」。
- `hasAuto=sourceType==='auto'`；`rulesOnly=sourceType==='rules'`；AI 备注需 `hasAuto&&plan.note&&tweaks.showAiNote`。
- ⚠️ 桥接 `sourceType` 恒 'auto'、`note` 恒 null → AI 备注与 rules_only_hint 实际不显示。
- `upcoming`：pending+dueAt，`dd` 0..14，升序取前 4；dueLabel 0/1/其余「{n} 天后」；hot(dd≤1)、warm(dd≤3)。
- 日历徽标 `calcBadges`：该日有 blocks 即标点。

---

# 四、liuli-classic/app/page-materials.jsx

## 4.1 AddAssignmentSheet（手动加截止）
**用户操作**
- 名称 `Input`（占位 `ad_name_ph`）；截止日期按钮 → 日历（`min=today`）；时间 `type=time` 默认 23:59
- **取消**(`cancel`) / **保存**(`save`，`disabled={!title.trim()}`) → `save()`

**逻辑**
- `save()` → `S.createAssignment({title,dueAt: date? date+'T'+time+':00+08:00':null})`（未桥接，source='manual'）；toast `ad_created`「已添加作业」。

## 4.2 MaterialsPage
**用户操作**
- `CF.CaptureBox`（onResult → ConfirmSheet）
- 提示 chips：`cap_hint_1`「明晚 8 点前要交 PSYC 阅读反思」/ `cap_hint_2`「午饭吃了牛肉面和酸奶」/ `cap_hint_3`「这两天有点失眠」→ `S.inboxProcess({text})` 直接开确认卡
- 出行 chip **出行规划**(`tv_entry`，仅 `catEnabled('travel')`) → TravelSheet
- 导入按钮：**导入 Canvas 作业** → 文件 → `importCanvas()`；**导入日历 ICS** → `importIcs()`；**识别课表截图** → `importShot()`；**全部数据源 →** → `setSub('sources')`
- 子标签 `Tabs`：**记录**(`feed`) / **学业**(`academic`) / **数据源**(`sources`)
- 记录：搜索框（占位 `feed_search_ph`）+ 清空；类别 chips（`cat_all` + 启用类，带计数）；`MaterialRow` 点击 → 详情
- 学业：过滤 chips **待办/全部/已完成/已忽略**；**手动加一条截止**(`mt_add_deadline`)；`AssignmentRow`；课程列表
- 数据源：Canvas **上传导出文件**(`mt_upload_json`)/**直连推送**(`mt_token_setup`)；ICS **上传 .ics**；截图 **上传截图**；**重复规则**入口 → `setView('rules')`；导入历史

**逻辑 / 阈值**
- `importCanvas()`：1400ms → `S.importCanvasDemo()`（未桥接）；toast `import_success_canvas`；再 700ms 后 `import_warning_1`（4200ms 时长）。
- `importIcs()`：1300ms → `S.icsCandidatesDemo().map(checked:true)` → CandidatesSheet。
- `importShot()`：2000ms → `S.shotCandidatesDemo().map(checked:true)` → CandidatesSheet。
- 作业过滤：pending 含 pending+planned；all 全量；按 dueAt 升序（空视作 '9999'）。
- `onSaved(catId)`：非 academic → feed/all；否则 academic。
- `useEffect` 处理 `nav.params`：`{rules}` 进规则视图（可带 editRule）；`{sub}` 切子标签。

---

# 五、liuli-classic/app/materials-parts.jsx

## 5.1 ruleDesc(r)
**逻辑**：once→「{date} · 一次」；daily→「每天」；every_n_days→「每 {n} 天」；monthly→「每月 {day} 日」（day 取 start_date 日）；weekly→「每周{days}」（缺 by_weekday 回退 weekdayOf(start_date)）；追加 time / duration（{n} 分钟）/ recurring 有 until 追加「 · 至 {date}」。

## 5.2 RuleEditor
**用户操作**
- 名称输入；类型 chips（任务/约定/休息/放松/饮食）；`rl_kind` 类型 **重复**(recurring)/**一次性**(once)
- 一次性：日期按钮（`rl_date`）；重复：频率 `rl_freq` **每天/每周/每月/每 N 天**（weekly 显星期多选、every_n_days 显 interval `min=1`）
- 时间 `type=time`；时长 `type=number min=5 step=5`；重复显 `rl_start` 开始 / `rl_until` 结束（可清空，清空显「留空 = 一直重复」）；固定时间开关
- **删除**(`del`，编辑态) → `Confirm`（`rl_delete_confirm_title`/`rl_delete_confirm`）；**取消**（新建）/ **保存** → `save()`

**逻辑 / 阈值**
- `save()`：标题空 → toast「给规则起个名字吧」中止。
- payload 归一：time 空→null；duration 空→null；once 才带 date；freq 仅 recurring；interval 仅 every_n_days/weekly/monthly 时 Number||1；by_weekday 仅 weekly。
- 新建 `S.createRule({active:true,source:'user',...})`（桥接）；编辑 `S.patchRule(id,payload)`（桥接）；toast「已创建规则/已保存」。
- 删除 `S.deleteRule(id)`（桥接）；toast「已删除规则」。

## 5.3 CandidatesSheet
**用户操作**：**全选**(`select_all`) chip；候选行点击切换 checked；**取消**(`cancel`)；**保存 {n} 条**(`confirm_save_n`，`disabled={!n}`) → `S.saveRulesBatch(checked)`（桥接）。
**逻辑**：`n=checked 数`；标题 `candidates_title`「识别到 {n} 条课程规则」。

## 5.4 TokenSheet
**用户操作**：**复制**(`copy`)→clipboard+toast「已复制」；**生成 Token**(`token_generate`，无 token)→`S.genToken()`（未桥接）；**轮换**(`token_rotate`，有 token)→`Confirm`(`token_rotate_confirm`)→`S.genToken()`；**安装浏览器插件**(`ext_guide`) 链接（preventDefault）。
**逻辑**：`genToken` 生成 `dcimp_<随机>`（旧模型 state.importToken）。

## 5.5 AssignmentRow
**用户操作**
- 完成（title `asg_mark_done`=**完成**）→ `S.patchAssignment(id,{status:'done'})`（桥接 → `C.setAssignmentDone(id,true)`）+ toast「完成 +1 ✓」
- 忽略（`asg_dismiss`=**忽略**）→ `patchAssignment {status:'dismissed'}`
- 恢复（`asg_restore`=**恢复**，done/dismissed 时显）→ `patchAssignment {status:'pending'}`

**逻辑 / 阈值（dueMeta）**：diff<0→urgent「过期 {n} 天」；0→urgent「今天截止」；1→urgent「明天截止」；≤3→soon「{n} 天后截止」；其余空样式。`done=status==='done'||submitted`。元信息：课程码/截止/分值(asg_pts)/已评分(asg_graded)/已提交(asg_submitted)/planned 提示(asg_planned_hint)。
⚠️ 桥接 `patchAssignment` 仅处理 'status' in ch 且 done→setAssignmentDone(true)；dismissed/restore 实际不写核心。

## 5.6 RulesView
**用户操作**
- 返回(`back`)→onBack；**新建规则**(`rl_new`)→`setEditor('new')`
- 规则行点击：仅 `!r.managed` 才 `setEditor(r)`（managed 行 cursor:default 不可点）
- 规则开关（非 managed）→ `S.patchRule(id,{active:!active})`（桥接，managed 拒绝）

**逻辑（managed 规则）**：来源徽标 `rl_source_*`；managed 显示「**由共享账本管理**」徽标 + 右侧锁 span（title「这条是从已有日程归纳出来的，不是可展开的规则」）+ 描述追加「 · 在日程里直接改」；开关替换为「—」；非 active 显「已暂停」；`autoEditId` 自动开编辑器。

---

# 六、liuli-classic/app/capture-flow.jsx

## 6.1 CaptureBox
**用户操作**：文本 `textarea`（占位 `cap_ph`，自动增高上限 110px）；Enter（无 Shift）发送；**拍照**(`cap_camera`)→文件→`run({photo:true})`；发送按钮（`cap_send`=「记一笔」）→ `send()`。
**逻辑**：`run` → `S.inboxProcess(payload)`（未桥接），延迟 1300ms；`cap_busy`「正在理解…」。

## 6.2 StructuredPreview（只读）
**逻辑**：diet items kcal + 合计（`cc_diet_total`「合计」+`cc_diet_protein`「蛋白质约 {n}g」）；due_at→「截止」；course→「课程」；symptom→「症状」；destination→「目的地」；amount→「金额 ¥」；distance_km→「距离 km」；建议块 `cc_health_advice`「自护建议」+`cc_health_disclaimer`。

## 6.3 ConfirmSheet（AI 确认卡）
**用户操作**
- 无分类回退：建议 chips；**不保存**(`cc_discard`)；**存成笔记**(`cc_save_note`)→`S.addMaterial`（桥接）
- 标题输入(`cc_title_field`)；类别 chips（`cc_change_cat`「类别（可改）」，遍历 enabledCats）→`setCat`
- 「同时保留一条学业笔记」开关（`cc_also_note`，仅 asAssignment）
- 「顺便生成行程建议」（`cc_gen_itinerary`，仅 travel+plan_trip）→`onPlanTrip`
- 「这其实是课表截图？转去课表识别」（`cc_alt_timetable`，仅 `res.altTimetable`）
- **不保存**(`cc_discard`) / **存入「{cat}」** 或 **创建作业**(`cc_save_assignment`) → `commit()`

**逻辑 / 阈值**
- `asAssignment = cat==='academic' && suggested_action==='create_assignment' && structured.due_at`。
- `commit()` → `S.inboxCommit({draftId, overrides:{category:cat,title}, alsoKeepNote})`（未桥接）。
- 置信度 `cc_confidence`「AI 置信 {p}%」，`p=Math.round(cls.confidence*100)`（受 showConfidence 控制）。
- ⚠️ inboxCommit 作业分支调未桥接 createAssignment（写旧模型），材料分支调桥接 addMaterial（进核心），不对称。

## 6.4 TravelSheet
**用户操作**：目的地输入(`tv_dest`)；日期按钮→日历范围（`ap_pick_days_travel`）；想法输入(`tv_notes`)；**生成行程建议**(`tv_generate`，`disabled={!dest.trim()||busy}`)→`generate()`；结果态 **换个思路**(`tv_retry`)/**存入出行**(`tv_save`)→`save()`。
**逻辑 / 阈值**：`generate()` → `S.aiTravel(...)`（未桥接）延迟 1700ms；最多 4 天（`Math.min(4,dayDiff+1)`），默认起始 T+7、结束 from+1。`save()` → `S.inboxCommit({draftId})`。

## 6.5 MaterialRow / MaterialDetailSheet
**用户操作**：记录行点击 → 详情；详情 **删除记录**(`md_delete`)→`Confirm`(`md_delete_confirm`，确认键 **删除**)→`S.deleteMaterial(id)`（桥接 → `C.materialRemove`）；**好**(`ok`) 关闭。
**逻辑**：来源 `md_source_photo`「拍照记录」/`md_source_inbox`「随手记」；有 itinerary 用 Itinerary，否则 StructuredPreview。

---

# 七、liuli-classic/app/page-companion.jsx

**工具图标**：get_weather→Sun、web_search→Search、list_upcoming→ListChecks、plan_add→CalendarPlus、plan_update→CalendarDays、plan_remove→Trash、rule_upsert/rule_remove→Repeat、memory_add/memory_remove→Brain。
**toolLink**：rule_*→rules；memory_*→memory；plan*→plan；其余 null。

## 7.1 ToolCard
**用户操作**
- 成功卡 **撤销**（`undo`，仅 `ev.opId`）→ `S.revertOp(ev.opId)`（未桥接，旧模型 ops）；`r.ok` → toast `tool_undone_toast`「已撤销这次改动」
- 跳转链接：**查看规则**(`cp_view_rules`)→`nav.go('materials',{rules:true})`；**查看记忆**(`cp_view_memory`)→`nav.go('settings')`；**查看计划**(`cp_view_plan`)→`nav.go('today')`

**逻辑 / 阈值**
- `tool_start` → 转圈「正在{name}…」(`tool_running`)；`!ev.ok` → 失败卡 `tool_fail`「没成功，稍后再试」。
- `undone = op && op.reverted`（`op=st.ops.find(o=>o.id===ev.opId)`）→ 已撤销态 `tool_undone`「已撤销」，隐藏撤销/跳转按钮。
- ⚠️ 撤销走旧模型 `S.revertOp`（`state.ops` + revertData 反向），**不是**核心 `C.undo()`；opId 也来自 `logOp`。

## 7.2 DecisionCard
**用户操作**：选项按钮 → `S.respondDecision(ev.id, o.id)`；自定义输入 + Enter/发送 → `S.respondDecision(ev.id, text.trim())`。
**逻辑**：pending 显示选项+自定义；已答复 `dc_answered`「已选择「{label}」」，超时 `dc_timeout`「已超时，按保守方案处理」；提示 `dc_timeout_hint`。

## 7.3 ThreadSheet
**用户操作**：**开启新会话**(`th_new`)→`S.createThread()`+toast；会话行点击→`onPick(th.id)`；删除（`threads.length>1`）→`Confirm`(`th_delete_confirm_title`/`th_delete_confirm`)→`S.deleteThread(id)`；删当前则切下一个或新建。

## 7.4 CompanionPage
**用户操作**：会话 pill→`setThOpen(true)`；空态提示 chips（`cp_hint_1/2/3/conflict/weather`）→`send(t(h))`；输入 `textarea`（占位「说点什么…」，自动增高上限 120px，Enter 发送）；发送按钮→`send()`。
**逻辑 / 阈值**
- `busy = 末条 assistant 且 status==='pending'`；`hasPendingDecision` = 任意消息含 pending decision_card。
- `send()` 守卫：空/无 thread/busy/有 pending 决策卡 均 return；否则 `S.sendAsync(thread.id, v)`（未桥接）。
- `sendAsync`→`agentTurn`（脚本化 agent），延迟 600ms 起；SSE 式帧 tool_start→（900ms）tool_result。
- agent 意图（正则）：天气(get_weather,1100ms)；冲突(list_upcoming+decision_card，超时 90s)；每 N 天规则(rule_upsert→`S.createRule` 桥接)；每天规则；记住(memory_add→`addMemoryFact` 桥接)；规划明天/本周(list_upcoming+plan_update→`S.autoPlan` 桥接)；加日程(plan_add→`S.patchPlan add` 桥接)；兜底回复。
- 崩溃清扫：载入时把遗留 pending 消息置 error（`cp_interrupted`「刚才的生成被打断了，再说一遍吧。」），模拟后端 FailPendingMessages。

---

# 八、liuli-classic/app/page-mood.jsx

## 8.1 MoodPage
**用户操作**
- 12 个 MoodTile：😊很好/😌平静/🤩兴奋/🥰感恩/😪累了/😣压力大/😰焦虑/😢难过/😠生气/😑无聊/🫥孤独/🤒不舒服 → `pick(emoji,key)`（再点取消）
- 备注 `Textarea maxLength=200`（「想说点什么吗？（可选）」）
- **记录这一刻** → `submit()`
- 练习邀请卡：**开始练习**(`md_start_exercise`) / **下次吧**(`md_not_now`)
- 历史列表（前 6 条，只读）

**逻辑 / 阈值**
- `pick` 守卫 `if (thinking) return`；`submit` 守卫 `!picked||thinking`。
- `submit()`：NEGATIVE 映射（tired→stretch，stressed/anxious/sad/angry→breathing，lonely→grounding，sick/bored→stretch）；AI 回复随机取 1 条；延迟 1400ms → `S.addMood({mood,note,aiResponse,exerciseOffered})`（桥接 → `C.moodRecord`）。
- ⚠️ 桥接 `addMood` 仅透传 e/l/note=aiResponse，exerciseOffered 与备注被丢弃；source='user'。
- `finishExercise()` → `S.completeMoodExercise(checkinId)`（桥接，no-op）+ toast「练习完成，好样的 ✓」。

## 8.2 BreathingEx（4-7-8）
**操作**：关闭(X)；**提前结束**(`ex_skip`)。**阈值**：吸气 4s→屏住 7s→缓缓呼气 8s；3 轮；起始延迟 2600ms；缩放 1.55(吸/屏) vs 1(呼/就绪)。

## 8.3 StretchEx
**操作**：关闭(X)；**提前结束**(`ex_skip`)；**下一步**(`next`)。**阈值**：5 步，每步 20s；环形进度 `1-left/20`。

## 8.4 GroundingEx（5-4-3-2-1）
**操作**：关闭(X)；**提前结束**(`ex_skip`)；点亮灯按钮（aria-label="light N"）。**阈值**：5→4→3→2→1；只能按序点下一盏（`if (j!==lit) return`）；满盏 450ms 进下一步；文案 `ground_5..1`。

---

# 九、liuli-classic/app/page-settings.jsx

开关组件 `Switch`（role=switch）。

## 9.1 ChannelBindSheet / ChannelsGroup
**用户操作**
- **绑定**(`ch_bind`)→`bindFor=id`→ChannelBindSheet（`S.bindChannel(channel)` 取 token）
- token **复制**(`copy`)；**模拟机器人确认**(`ch_simulate`)→`S.verifyChannel(channel)`（未桥接）+toast「{name} 绑定成功」
- **解绑**(`ch_unbind`)→`Confirm`(`ch_unbind_confirm`)→`S.unbindChannel(id)`（未桥接）

**阈值**：`bindChannel` token 12 位，`expiresAt=Date.now()+600e3`（10 分钟）；提示「口令 10 分钟内有效，仅可使用一次。」。

## 9.2 MemoryGroup
**用户操作**：单条删除(Trash)→`S.deleteMemoryFact(id)`（桥接 → `C.memoryRemove`）+toast「已忘记这一条」；添加输入+**记住**(`mem_add`，Enter/按钮)→`S.addMemoryFact(draft.trim(),'user')`（桥接 → `C.memoryAdd`）；**清空记忆**(`mem_clear`)→`Confirm`(`mem_clear_confirm_title`/`mem_clear_confirm`)→`S.clearMemory()`（桥接 → 逐条 `C.memoryRemove`）。
**逻辑**：来源徽标 `mem_source_chat/user/import`。

## 9.3 SettingsPage
**用户操作**
- 助手名字输入(onBlur)→`S.patchSession({assistantName})`（桥接 → `C.patch`）
- 自定义提示词 `Textarea maxLength=2000`(onBlur=saveL2)→`S.patchSession({l2Prompt})`（桥接）；字数提示「{n} / 2000 字…」
- 管理控制台入口（admin.html 链接）
- 语言 `Segmented` **中文/EN** → `S.patchSession({language})+I18N.setLang(v)`（桥接）
- 主动关怀 7 开关（`pref_*`）：早间简报/晚间回顾/临期提醒/滚动重排/空档建议/自动规划/勿扰模式 → `S.patchPreferences({[key]:!...})`（未桥接）
- 记录类别开关 → `S.setCategory(id,!enabled)`（未桥接；note 锁定，显「基础类别，始终开启」）
- 主题工作室 `SP.ThemeStudio`
- 数据：**管理导入的资料**(`st_goto_materials`)→`nav.go('materials',{sub:'sources'})`；**重看新手引导**(`st_replay_onboarding`)→`S.setOnboarded(false)`（未桥接）
- 关于卡：版本 `st_about_version` / `st_beta_badge` / `st_api_line` / 管理控制台链接

**阈值**：`PREFS` 7 键；`V=S.versionInfo`（apiVersion 2 / apiMinor 2 / build 2.2.0-beta / minClient 2.0.0）。

---

# 十、liuli-classic/app/settings-theme.jsx

## 10.1 AuthSheet
**用户操作**：**用 Google 继续**(`au_oauth_google`)/**用 GitHub 继续**(`au_oauth_github`)→`oauth(p)`（未桥接）；注册态昵称输入(`au_name`)；邮箱/密码输入（Enter 提交）；**登录**(`au_submit_login`)/**创建账户**(`au_submit_register`)→`submit()`；切换 `au_switch_to_register`/`au_switch_to_login`。
**阈值**：邮箱正则 `/^[^@\s]+@[^@\s]+\.[^@\s]+$/` 不通过→「邮箱格式不太对」；密码 <8→「密码至少 8 位」。`submit`→`S.login/S.register`（未桥接）延迟 900ms；`oauth` 延迟 1100ms；成功 toast「欢迎回来，{name}」。

## 10.2 AccountSheet
**用户操作**：**退出登录**(`au_signout`)→`S.logout()`+toast「已退出，回到匿名会话」；**登录 / 注册**→`setAuthOpen(true)`→AuthSheet。

## 10.3 ThemeStudio
**用户操作**
- 内置主题卡（天空蓝/暖橙日落/深夜紫/自然绿）→`applyBuiltin(id)`→`S.patchSession({currentTheme:id})`（桥接）+`applyCurrentTheme`+toast「已切换主题」
- 自定义主题卡→`applyCustom(th)`；右键 `onContextMenu`→菜单；移动端长按 550ms→菜单
- 菜单：**重命名**(`st_theme_menu_rename`)/**AI 微调**(`st_theme_menu_ai`)/**删除**(`st_theme_menu_delete`)
- AI 调色：描述输入(占位 `st_ai_theme_ph`)；基准 chips（`st_ai_base_none`「不指定」+4 内置）；**生成预览**(`st_ai_generate`)→`generate()`
- 预览栏：**再试一次**(`st_preview_retry`)/**取消**(`st_preview_cancel`)/**保存**(`st_preview_save`)
- 重命名浮层→`S.patchTheme(id,{name})`（桥接 → `C.patchTheme`）
- 删除 `Confirm`（`st_theme_delete_title`；使用中追加 `st_theme_delete_inuse`）→`S.deleteTheme`（桥接）

**逻辑 / 阈值**
- `generate()`→`S.aiTheme(desc,aiBase||undefined,editingId)`（桥接 → `C.aiTheme`），延迟 1600ms；not_a_theme→not_a_theme_err；成功 `applyThemeVars`。
- `savePreview()`：编辑→`S.patchTheme`，否则 `S.createTheme`（桥接）；随后 `patchSession({currentTheme:th.id})`+toast「已保存主题「{name}」」。
- `del(th)`→`S.deleteTheme(th.id)`（桥接），当前主题被删核心 reset 到 'sky'。
- 菜单定位 `x=min(clientX,innerWidth-180)`、`y=min(clientY,innerHeight-170)`。

---

# 十一、liuli-classic/app/store-bridge.js（CRITICAL：旧模型 ↔ 核心桥接）

## 11.1 载入与投影
- 加载顺序：`store.js → store-ext.js → store-bridge.js`。`S=window.DcStore`、`C=window.DcCore`，任一缺失即 return。
- 投影窗口 **`SPAN = 21`**（前后各 21 天）。
- `syncPlans()`：`st.plans={}`，遍历 `[-21,21]`，对 `C.blocksOf(d)` 非空天生成 `{id:'plan_'+d, date, blocks:bs.map(toOld), sourceType:'auto', note:null}`。⚠️ **sourceType 硬编码 'auto'、note 硬编码 null** → 前端「规则专属天」与「AI 备注」在桥接模式永不出现。
- `toOld(b)`：id/date 直传；time=`b.start`；duration_min=`b.dur`；time_mode=`b.lockLevel==='hard'?'fixed':'floating'`；timezone='Asia/Shanghai'；completed=`b.status==='done'`；origin=`b.origin==='auto'?'auto':'manual'`；rule_id=null；locked=`b.lockLevel`；lockReason；missed=`b.status==='missed'`。⚠️ rule_id 恒 null、origin 无 'rule' → 「来自规则/编辑规则/今天隐藏」分支全失效。
- `syncMaterials()`：作业从 `C.state.assignments` 映射（done→status，pointsPossible:100，submitted:!!done）；课程从作业 `course` 字段去重归纳（`id='c_'+code`）；心情从 `C.state.moods`；keyFacts 从 `C.state.memories`；主题/助手名/语言从核心读。
  - **规则归纳**：扫描未来 28 天（`i=0..28`）每天 `C.blocksOf(d)`，按标题聚合（first/days[]/wd{}）；筛选 `days.length>=2 || first.lockLevel` → managed 规则；`daily = days.length>=20`；freq=daily?'daily':'weekly'；source=`lockLevel==='hard'?'ics':origin==='auto'?'chat':'user'`；managed:true。
  - **自存定义优先**：`defs`（localStorage `daycore-liuli0-rules`）同名/同 id 覆盖为 managed:false，否则追加。
- `syncInbox()`：资料库=核心 `C.state.materials`（`KIND2CAT` 映射 kind→旧类别）+ 旧模型非 `_core` 条目；`st.memory` 从 `C.state.memories`。

## 11.2 echo-guard（防回环）
- `C.subscribe(() => { if (echo) return; syncAll(); S.emit(); })` —— 核心变化即重投影并通知 UI。
- `guard(fn)` 包装每个桥接写操作：`echo=true` 执行 fn，`finally { echo=false; syncAll(); S.emit(); }` —— 抑制「核心变化→重投影」回环，写后手动重投影一次。

## 11.3 写操作改道明细
- `S.patchPlan(date, action)`（桥接）：
  - add → `C.addBlock({date,start:block.time||'12:00',dur:block.duration_min||45,title:block.title||'未命名',type:block.type||'task',origin:'manual'})`（origin 强制 manual）。
  - update：`id=match.id`；`cur=C.state.blocks.find(id)`；completed→`C.setMark(id,'done'/'none')`；note→`C.setNote`；title/type/duration_min→`C.setBlockFields`；**仅当 `ch.time && ch.time!==cur.start`** 才 `C.moveBlock(id,{date,start:ch.time,_confirmed:true})`（避免每次保存塞假挪动）；moveBlock 返回 locked 时回传 `{locked,reason}`。
  - remove → `C.removeBlock(id)`（**无墓碑**，直接删核心块）。
- `S.autoPlan({from,to,instructions})`（桥接）：`f=from||today`；`n=clamp(1,7,round((to-f)/864e5)+1)`（最多 7 天）；每天 `C.gapsOf(d,45).slice(0,3)` 每空档 `C.addBlock({date,start:g.start,dur:Math.min(90,g.min||60),title: 未完成作业 'course · title' 或 '整理 + 预习', type:'task', origin:'auto'}, true)`（agent=true，每天≤3 块）；`made===0` → `{error:'no_material'}`。⚠️ **忽略 mode**：keep_manual/replace_all 均按「填空档」处理。
- `S.addMood(entry)` → `C.moodRecord({e,l,note:entry.aiResponse,source:'user'})`，返回 `C.state.moods[0]`。
- `S.completeMoodExercise()` → 空函数 no-op。
- `S.patchAssignment(id,ch)` → 仅 `'status' in ch`：`C.setAssignmentDone(id, ch.status==='done')`。
- `S.patchSession(ch)` → currentTheme→`C.setTheme('liuli',...)`；language→`C.patch({language})`；assistantName→`C.patch({assistantName})`；随后 `Object.assign(st.session,ch)`。
- 主题：aiTheme→`C.aiTheme`；createTheme→`C.createTheme`；patchTheme→`C.patchTheme`；deleteTheme→`C.deleteTheme`（返回 `{ok:true,currentThemeReset:!!r.reset}`）。
- 资料/记忆：addMaterial→`C.materialAdd`；deleteMaterial：核心有该 id→`C.materialRemove` 否则回退旧 `_delMaterial`；addMemoryFact→`C.memoryAdd`；deleteMemoryFact→`C.memoryRemove`；clearMemory→逐条 `C.memoryRemove`。

## 11.4 规则降级与 spawn/despawn（幂等性）
- 核心无 rule 实体；规则定义自存 `localStorage['daycore-liuli0-rules']`（`defs`）。
- `rid(title)='rd_'+encodeURIComponent(title).replace(/%/g,'')`。
- `occurs(r,d)`：start_date 前 false；until 后 false；once 只 `date===d`；daily 恒 true；every_n_days 用 `diff % max(1,interval)===0`；weekly 看 by_weekday（缺回退 `wdOf(start_date)`）；monthly 看日号相等。
- `spawn(r)`：遍历 `HORIZON=28` 天，occurs 且 **同标题块已存在则跳过**（`C.blocksOf(d).some(x=>x.title===r.title)` → 按标题去重/幂等）；`add.length` 时 `C.bulkBlocks({add,label:'按规则铺开：…（N 次）'})`。
- `despawn(title)`：遍历 28 天，仅删 `title===title && status==='planned'` 的块（**不删 done/missed**）；`C.bulkBlocks({removeIds,label:'停用规则：…（撤下 N 次）'})`。
- `mkDef(input)` 默认：kind:'recurring', freq:'weekly', interval:1, by_weekday:[], start_date:today, until:null, active:true, type:'task', time:null, duration_min:45, source:'user', time_mode:'floating'。
- `S.createRule(input)`：mkDef→defs.push→saveDefs→spawn。
- `S.saveRulesBatch(inputs)`：逐条 createRule 语义。
- `S.patchRule(id,ch)`：`own(id)=defs.find(id)`；**`!own` → `{error:'managed'}`（managed-rules 拒绝写）**。active===false→despawn(oldTitle)；标题/时间/时长/类型/星期/频率变化→先 despawn(oldTitle) 再 spawn。
- `S.deleteRule(id)`：`!own` → `{error:'managed'}`；否则 despawn(title)+从 defs 移除。

## 11.5 核心边角行为速查
- **石化**：`phaseOf(b)` 依据块结束时间 vs `petrifyLine() = min(now-5h, 今日00:00)`（`PETRIFY_H=5`）判 stone/recon/now/future。
- **moveBlock 锁定**：stone 且向后挪（更晚日期或同日更晚）→`{stone:true}`；hard 且非 agent→`{locked:'hard',reason}`；soft 且非 agent 且未 `_confirmed`→`{locked:'soft',reason,pending}`。桥接恒传 `_confirmed:true`（软锁跳过），硬锁不可绕过（仅 agent），石化被桥接忽略（只查 `r.locked`）。
- **锁派生** `deriveLock`：type==='appointment' 且标题含「课/课程/讲座/实验课/Lecture/Lab课」→hard「课程时间由课表决定」，否则 soft「和别人约好的时间」；非 appointment→null。
- **撤销**：核心 `C.undo(opId)` 反向补偿（undoable/undone 守卫、agent 操作扣 boldness、记 revert 操作）；前端陪伴页「撤销」按钮实际调 `S.revertOp`（旧模型 ops，reverted 守卫→`{error:'conflict'}`，revertData 分 rule_add/block_add/memory_add/material_add/assignment_add/plan_days）。
- **去重**：moodRecord 当日已有 mood 且 source==='agent'→`{blocked:true}`（用户 source 不拦）；wishAdd 按去空白文本去重；importCanvasDemo 按标题去重「Lab 6：哈希表」；spawn 按标题去重。

---

# 十二、liuli/admin/admin-main.jsx
**用户操作**：无（入口）。`useAdmin()`；`A.isAuthed() ? <Shell/> : <Gate/>`。

---

# 十三、liuli/admin/admin-shell.jsx

## 13.1 Gate（管理登录）
**用户操作**：Token 输入 `Input type=password`（占位「X-Admin-Token」，Enter 提交）；**连接**（`busy?'连接中…':'连接'`，`disabled={busy||!tok.trim()}`）→`A.login(tok.trim())`。
**阈值**：错误「Token 无效（401 unauthorized）」；演示 token 提示 `daycore-admin`；登录延迟 450ms；token 存 `sessionStorage['dc-admin-token']`。

## 13.2 ToastHost / AdmConfirm / Seg
- Toast 时长 2200ms。`AdmConfirm`：scrim 点击关闭、面板 stopPropagation；按钮 **取消** / `confirmLabel||'确认'`。`fmtNum`：≥1e6→'M'、≥1000→'k'。

## 13.3 Overview（总览）
**用户操作**：快捷操作 **备份**→`A.backup()`；**导出**→`A.exportAll()`；**导入**→文件→`A.importJson(f,cb)`；**查看全部 →**→`go('logs')`。
**阈值**：统计卡 注册用户 users / 会话总数 sessions / AI 调用 aiCalls / Token 消耗 tokenUsed / 反馈有用率 `round(useful/total*100)%`（231/252）；最近 AI 调用 `ailogs.slice(0,5)`。

## 13.4 Shell
**用户操作**：侧栏 8 项 **总览 / Prompt 管理 / 模型 / OAuth / 服务配置 / AI 日志 / 用户 / 数据库**；底部 **返回应用**（href="Daycore App v2.2.html"）、**退出控制台**→`A.logout()`。

---

# 十四、liuli/admin/admin-config.jsx

## 14.1 ConfigView（服务配置）
**用户操作**：每项控件 select / bool(Sw) / secret(type=password，占位「••••••••（输入以替换）」或「未设置」) / text / number(inputMode=numeric)；保存栏 **放弃**→`setOvr({})`；**保存修改**→`save()`→`A.setEnvMany(ovr)`。
**阈值**：`dirty=Object.keys(ovr).length`；secret 不回显（仅 set 标记）；保存 toast：有 needsRestart 项→「已保存；{项} 需重启服务生效」否则「已保存，热加载生效」。
**分组**：服务(APP_ENV/HOST/PORT/STATIC_DIR/PUBLIC_BASE_URL/ALLOWED_ORIGINS)、数据库(DB_TYPE/DB_DSN)、认证(JWT_SECRET/COOKIE_SECRET/PASSWORD_PEPPER/JWT_TTL/SECURE_COOKIES)、AI 与模型(DEFAULT_CHAT_MODEL/DEFAULT_VISION_MODEL/DEFAULT_PLANNER_MODEL/AI_REQUEST_TIMEOUT/AGENT_MAX_ROUNDS/MODELS_CONFIG/OAUTH_CONFIG)、API 密钥(DEEPSEEK/OPENAI/ANTHROPIC/TAVILY)、限额与管理(AI_RATE_LIMIT_PER_MIN/MAX_IMAGE_BYTES/ADMIN_TOKEN)、自主规划(AUTO_PLAN_MAX_DAYS/ASSIGNMENT_LOOKAHEAD_DAYS)。

## 14.2 ModelsView（模型目录）
**用户操作**：**新增模型**→`setEdit({m:{...EMPTY_MODEL},isNew:true})`；模型卡→编辑 Drawer（id 禁改、format Seg openai/anthropic/ollama、base_url、model、api_key_env、context_window、vision/tools/stream 开关）；**保存**→`A.saveModel(m,isNew)`；**删除**→`AdmConfirm`→`A.deleteModel(id)`。
**阈值**：`EMPTY_MODEL.context_window=65536`；`saveModel` 错 id_required/id_exists；`deleteModel` 当前默认→is_default（「该模型是当前默认，请先在服务配置中更换」）；徽标 默认对话/默认视觉（含「（自动）」）/默认规划。

## 14.3 OAuthView
**用户操作**：**自定义 Provider**→新建 Drawer（name/client_id/client_secret/auth_url/token_url/userinfo_url/scopes/id_field/email_field/name_field/avatar_field）；回调 URL **复制**→clipboard；**保存**→`A.saveProvider(p,isNew)`；**删除**（非预设）→`A.deleteProvider(name)`。
**阈值**：校验 bad_name（`/^[a-z][a-z0-9_-]*$/`）/name_exists；预设 google/github URL 与字段自动填充；回调恒 `PUBLIC_BASE_URL + /api/auth/oauth/<name>/callback`。

---

# 十五、liuli/admin/admin-views.jsx

## 15.1 Prompts
**用户操作**：prompt 列表选择；locale Seg（中文/English）；textarea 编辑；占位符 chip 点击→光标处插入 `{{name}}`；**还原默认**（modified||dirty 时）→`A.resetPrompt(key,loc)`；**保存**（`disabled={!dirty}`）→`A.savePrompt(key,loc,draft)`。
**阈值**：8 个 prompt key（companion_system/auto_plan/mood_reply/inbox_classify/theme_generate/travel_plan/vision_food/vision_timetable）；`savePrompt` 等于默认则删覆盖，存 `localStorage['dc-admin-prompt-ovr']`；按 locale 分开。

## 15.2 Logs
**用户操作**：类型过滤 chips（全部+KINDS）、**仅失败** chip、搜索框（会话/模型）；行点击→详情 Drawer（请求/响应 pre）；分页按钮 ←/→。
**阈值**：`PER=12` 条/页；`pages=Math.max(1,ceil(rows/PER))`；过滤 kind/status/关键字（sessionId+model+id）；「最近 48 小时」；KINDS=companion/auto_plan/mood/inbox_classify/theme/vision_food/travel。

## 15.3 UsersView
**用户操作**：行删除(Trash)→`AdmConfirm`（desc「将删除其全部数据：计划、资料、规则、记忆、主题与聊天记录，不可恢复。」，确认键 **永久删除**）→`A.deleteUser(id)`。
**阈值**：统计「注册 {nReg} · 匿名会话 {len-nReg}」。

## 15.4 DbView
**用户操作**：表卡点击→进表；**备份 .db / 导出 JSON / 导入**；**数据表** 返回；行删除(Trash)→`AdmConfirm`（确认键 **删除**）→`A.deleteRow(table,id)`。
**阈值**：`cut(v)` 超 38 字符截断加「…」；NULL/undefined→「NULL」；15 张表（users/sessions/plans/time_blocks/schedule_rules/assignments/materials/memory_facts/custom_themes/chat_threads/chat_messages/ai_logs/import_history/ops_log/decision_cards）。

---

# 十六、liuli/admin/admin-store.js（mock 服务端状态）

## 16.1 鉴权
`DEMO_TOKEN='daycore-admin'`；`login(tok)`：`tok!==DEMO_TOKEN→{error:'unauthorized'}`，成功存 `sessionStorage['dc-admin-token']`；logout 清除。

## 16.2 元数据 / 阈值
`health` = `{ok:true, db:'ok', version:'2.2.0-beta', channel:'beta', env:'production', uptime:'3 天 14 小时'}`。
`versionInfo` = `{apiVersion:2, apiMinor:2, build:'2.2.0-beta', minClient:'2.0.0'}`。
`stats` = `{users:6, sessions:14, aiCalls:12480, tokenUsed:8412030, feedbackUseful:231, feedbackTotal:252}`。

## 16.3 prompts / placeholders
8 个 prompt（同 15.1）；`promptDefaults` 中英双语；覆盖存 `localStorage['dc-admin-prompt-ovr']`；`getPrompt` 覆盖优先；`savePrompt` 等于默认则删覆盖；`resetPrompt` 删覆盖。

## 16.4 env（.env 覆盖）
`ENV_GROUPS` 7 组；`envValue` secret 恒返 ''；`envIsSet`=覆盖非空或 `d.set`；`setEnvMany(map)`→`localStorage['dc-admin-env']`，返回 `needsRestart`。
关键阈值：`AI_RATE_LIMIT_PER_MIN:'30'`、`MAX_IMAGE_BYTES:'8388608'`（8 MiB）、`AGENT_MAX_ROUNDS:'6'`、`AUTO_PLAN_MAX_DAYS:'7'`、`ASSIGNMENT_LOOKAHEAD_DAYS:'14'`、`JWT_TTL:'720h'`。

## 16.5 models / oauth
`MODEL_DEFAULTS` 5 个：deepseek-chat(65536)/deepseek-reasoner(65536)/gpt-4o(128000)/claude(200000)/llava(8192)。`saveModel` id_required/id_exists；`deleteModel` 当前默认→is_default。`OAUTH_DEFAULTS` google(preset,空密钥)、github(preset,演示密钥)。

## 16.6 演示数据
`ailogs` 36 条（3 失败 rate_limited/parse_error）；`users` 10 条（6 注册+4 匿名）；`tables` 15 张+`rowsByTable` 样例；`deleteRow` 减行数；`download/backup/exportAll/importJson` 走 Blob/FileReader。

---

# 附：跨文件关键数值一览
- 石化 `PETRIFY_H=5h`；投影 `SPAN=21` 天；规则铺开/归纳 `HORIZON=28` 天。
- 规则归纳为 managed：`days.length>=2 || lockLevel`；判 daily：`days.length>=20`。
- 桥接 autoPlan：范围 clamp 1..7 天；`gapsOf(45)` 取 ≤3 空档/天；单块 `min(90,gap.min||60)`；无材料→no_material。
- 作业临期窗口 14 天；今日页 upcoming 取前 4；dd≤1 热、≤3 温。
- 主题 AI：12 变量白名单（--primary/--accent/--bg-start/--bg-end/--text-primary/--text-secondary/--text-muted/--surface/--surface-hover/--success/--warning/--error）；描述 <2 字或「哈哈/hello/hi/你好」→not_a_theme；名称截 8 字；对比度 ≥4.5:1。
- 呼吸 4-7-8（3 轮，起始 2600ms）；伸展 5 步×20s；着地 5-4-3-2-1（步进 450ms）。
- 决策卡超时 90s；绑定口令 10 分钟；管理 toast 2200ms、登录 450ms；日志分页 12 条/页。
- 情绪备注 200 字上限、L2 提示词 2000 字上限；时长/间隔输入 min=5/step=5 与 min=1。




