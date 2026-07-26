// Daycore v4 — 共享数据核心（模拟 API v2.3 语义：账本 append-only、读时石化、提案统一资源、撤销即学习）
// 三个前端（琉璃/纸屿/汀）共用同一份 localStorage 状态 —— 多端并存，同一契约。
window.DcCore = (function () {
  'use strict';
  const LS = 'daycore-v4-core', VER = 9, PETRIFY_H = 5;
  const pad = (n) => String(n).padStart(2, '0');
  const iso = (d) => d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate());
  const todayIso = () => iso(new Date());
  const addDays = (ds, n) => { const [y, m, d] = ds.split('-').map(Number); return iso(new Date(y, m - 1, d + n)); };
  const toMin = (hm) => { const [h, m] = hm.split(':').map(Number); return h * 60 + m; };
  const toHM = (min) => pad(Math.floor(min / 60) % 24) + ':' + pad(Math.round(min % 60));
  const WD = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'];
  const weekdayShort = (ds) => WD[new Date(ds + 'T12:00:00').getDay()];
  const fmtDate = (ds, wd) => { const [, m, d] = ds.split('-').map(Number); return m + '月' + d + '日' + (wd ? ' ' + weekdayShort(ds) : ''); };
  const relDay = (ds) => { const T = todayIso(); if (ds === T) return '今天'; if (ds === addDays(T, 1)) return '明天'; if (ds === addDays(T, -1)) return '昨天'; if (ds === addDays(T, 2)) return '后天'; return fmtDate(ds, true); };
  const nowMs = () => Date.now();
  const nowMin = () => { const d = new Date(); return d.getHours() * 60 + d.getMinutes(); };
  const dayStartMs = (ds) => new Date(ds + 'T00:00:00').getTime();
  const fmtHM = (ms) => { const d = new Date(ms); return pad(d.getHours()) + ':' + pad(d.getMinutes()); };
  let idc = 100; const uid = (p) => p + '_' + (++idc) + '_' + Math.random().toString(36).slice(2, 6);
  const MOODS = [{ e: '😊', l: '开心' }, { e: '😌', l: '平静' }, { e: '🥰', l: '被爱' }, { e: '🤩', l: '兴奋' }, { e: '😐', l: '一般' }, { e: '😪', l: '疲惫' }, { e: '😣', l: '压力大' }, { e: '😟', l: '焦虑' }, { e: '😢', l: '低落' }, { e: '😠', l: '烦躁' }, { e: '🤒', l: '不舒服' }, { e: '🌙', l: '失眠' }];

  // ---------- 石化：纯读时函数，无表 ----------
  const petrifyLine = () => Math.min(nowMs() - PETRIFY_H * 3600e3, new Date().setHours(0, 0, 0, 0));
  const blockRange = (b) => { const s = dayStartMs(b.date) + toMin(b.start) * 60e3; return { s, e: s + b.dur * 60e3 }; };
  function phaseOf(b) { const { s, e } = blockRange(b), n = nowMs(); if (e < petrifyLine()) return 'stone'; if (e <= n) return 'recon'; if (s <= n) return 'now'; return 'future'; }

  // ---------- 状态 ----------
  let state = null;
  function save() { try { localStorage.setItem(LS, JSON.stringify(state)); } catch (e) { } }
  const subs = new Set();
  function emit() { save(); subs.forEach((f) => { try { f(state); } catch (e) { } }); }
  function subscribe(f) { subs.add(f); return () => subs.delete(f); }

  // ---------- 账本（append-only）----------
  function op(o) { const row = Object.assign({ id: uid('op'), at: nowMs(), silent: false, agent: false, undoable: true, undone: false, domain: 'schedule', payload: {} }, o); state.ops.unshift(row); return row.id; }
  const findOp = (id) => state.ops.find((o) => o.id === id);

  // ---------- 实体助手 ----------
  function deriveLock(b) {
    if (b.lockLevel !== undefined) return b;
    if (b.type === 'appointment') {
      const isClass = /（课）|课$|课程|讲座|实验课|导论|Lecture|Lab\s*课/i.test(b.title || '');
      if (isClass) { b.lockLevel = 'hard'; b.lockReason = b.lockReason || '课程时间由课表决定'; }
      else { b.lockLevel = 'soft'; b.lockReason = b.lockReason || '和别人约好的时间'; }
    } else { b.lockLevel = null; b.lockReason = null; }
    return b;
  }
  function mkBlock(o) { return deriveLock(Object.assign({ id: uid('b'), date: todayIso(), start: '12:00', dur: 60, title: '', type: 'task', origin: 'manual', status: 'planned', reconciled: false, note: '', lockLevel: undefined, lockReason: null }, o)); }
  function mkProposal(o) { return Object.assign({ id: uid('p'), level: 'L2', kind: 'card', title: '', summary: '', reason: '', evidence: '', date: null, start: null, dur: null, btype: 'task', lockLevel: undefined, lockReason: null, rows: null, state: 'pending', ttlRule: 'silent_reject', expiresAt: nowMs() + 12 * 3600e3, createdAt: nowMs(), domain: 'schedule' }, o); }

  // ---------- 种子 ----------
  function seedCommon(s) { s.rhythm = { wake: '07:30', sleep: '23:40', pinned: false, learning: true }; s.boldness = { schedule: { score: 0.52, evid: 5 }, habit: { score: 0.34, evid: 2 }, archive: { score: 0.72, evid: 9 }, care: { score: 0.5, evid: 3 } }; s.prefs = { morningBrief: true, eveningReview: true, deadlineAlerts: true, rollingReplan: true, gapSuggestions: true, autoPlan: true, doNotDisturb: false }; s.channels = [{ id: 'qq', bound: true, externalId: 'QQ 3324 8871' }, { id: 'telegram', bound: false, externalId: null }]; s.customThemes = [{ id: 'th_milktea', name: '奶茶时光', dark: false, base: 'sunset', createdAt: nowMs() - 8 * 864e5, variables: { '--primary': '#b08968', '--accent': '#9c6644', '--bg-start': '#fdf6ec', '--bg-end': '#f3e5d0', '--text-primary': '#4a3728', '--text-secondary': 'rgba(74,55,40,0.72)', '--text-muted': 'rgba(74,55,40,0.45)', '--surface': 'rgba(255,252,246,0.72)', '--surface-hover': 'rgba(255,252,246,0.9)', '--success': '#7f9d6c', '--warning': '#d9915b', '--error': '#c96f5e' } }]; }
  function base() { return { ver: VER, scenario: 'healthy', userName: '林可', assistantName: '小禾', l2Prompt: '', language: 'zh-CN', themes: { liuli: 'sky', zhiyu: 'sunset', ting: 'night' }, customThemes: [], prefs: {}, channels: [], blocks: [], proposals: [], wishes: [], moods: [], materials: [], memories: [], assignments: [], ops: [], pushes: [], chat: [], typing: false, exchange: null, briefDismissed: null, riverSeed: [], boldness: {}, rhythm: {} }; }

  function clampHM(min) { return toHM(Math.max(6 * 60, Math.min(23 * 60 + 30, Math.round(min / 5) * 5))); }
  function seedHealthy() {
    const s = base(); seedCommon(s); const T = todayIso(), Y = addDays(T, -1), N = nowMin(), V = Math.min(Math.max(N, 13 * 60 + 30), 20 * 60);
    const B = (o) => s.blocks.push(mkBlock(o));
    const R = (startMin, dur, o, keepPlanned) => { const start = clampHM(startMin); const isPast = dayStartMs(T) + (toMin(start) + dur) * 60e3 < nowMs(); B(Object.assign({ date: T, start, dur, origin: 'auto', status: !keepPlanned && isPast ? 'done' : 'planned', reconciled: !keepPlanned && isPast }, o)); };
    // 昨天：石化样本（含一条未对账）
    B({ date: Y, start: '08:30', dur: 50, title: 'PSYC 110 心理学导论', type: 'appointment', origin: 'auto', status: 'done', reconciled: true });
    B({ date: Y, start: '10:10', dur: 95, title: 'CS 201 数据结构（课）', type: 'appointment', origin: 'auto', status: 'done', reconciled: true });
    B({ date: Y, start: '15:00', dur: 90, title: 'Essay 2 修改', type: 'task', origin: 'auto', status: 'missed', reconciled: true, note: '换成了休息，明天补' });
    B({ date: Y, start: '21:30', dur: 30, title: '给妈妈打电话', type: 'relax', origin: 'manual', status: 'planned', reconciled: false });
    // 今天：围绕"现在"布块
    B({ date: T, start: '08:30', dur: 50, title: 'PSYC 110 心理学导论', type: 'appointment', origin: 'auto', status: N > 9.5 * 60 ? 'done' : 'planned', reconciled: N > 9.5 * 60 });
    R(V - 175, 95, { title: 'MATH 172 微积分 II（课）', type: 'appointment' });
    R(V - 35, 80, { title: 'Lab 5：AVL 树 · 最小可运行版', type: 'task' }, true);
    R(V + 75, 95, { title: 'CS 201 数据结构（课）', type: 'appointment' });
    R(V + 200, 30, { title: '慢跑 · 河边', type: 'relax' });
    B({ date: T, start: '12:10', dur: 40, title: '午饭', type: 'meal', origin: 'auto', status: N > 12.9 * 60 ? 'done' : 'planned', reconciled: N > 12.9 * 60 });
    // 明天
    B({ date: addDays(T, 1), start: '08:30', dur: 50, title: 'PSYC 110 心理学导论', type: 'appointment', origin: 'auto' });
    B({ date: addDays(T, 1), start: '14:00', dur: 95, title: 'MATH 172 微积分 II（课）', type: 'appointment', origin: 'auto' });
    B({ date: addDays(T, 1), start: '16:00', dur: 60, title: 'Lab 5 · 测试并提交', type: 'task', origin: 'manual' });
    // 提案：L1 虚影（今晚空档 × 许愿池）+ L2 两张
    const ghostStart = N < 19.5 * 60 ? { date: T, start: '20:00' } : { date: addDays(T, 1), start: '20:00' };
    s.proposals.push(mkProposal({ level: 'L1', kind: 'timed', title: '《悉达多》读 30 页', summary: '这段空着——从你想做的事里挑的，不想看划走就行', reason: '你说过"改天想把《悉达多》看完"；这个空档 45 分钟，正合适。', evidence: '你想做的事之一 · 约 45 分钟 · 正好这段空着', date: ghostStart.date, start: ghostStart.start, dur: 45, btype: 'relax', ttlRule: 'silent_reject', expiresAt: dayStartMs(ghostStart.date) + toMin(ghostStart.start) * 60e3, domain: 'schedule' }));
    s.proposals.push(mkProposal({ level: 'L2', kind: 'decision', title: '把 Lab 5 拆成两段？', summary: '截止在明晚 23:59，一口气做完太狠了', reason: '今晚有 2.5h 空档，明天 16:00 还有 1h——拆开更稳。', evidence: '证据：明晚 23:59 截止 · 今晚有 2.5 小时空着', rows: [{ id: 'r1', label: '今晚 20:00 先做最小可运行版（90 分钟）', state: 'pending', date: T, start: '20:00', dur: 90, btype: 'task' }, { id: 'r2', label: '明天 16:00 补测试 + 提交（60 分钟）', state: 'pending', date: addDays(T, 1), start: '16:00', dur: 60, btype: 'task' }], ttlRule: 'silent_reject', expiresAt: nowMs() + 10 * 3600e3, domain: 'schedule' }));
    s.proposals.push(mkProposal({ level: 'L2', kind: 'card', title: '「给绿萝浇水」改成每 3 天早上？', summary: '最近 3 次你都是早上浇的', reason: '我先按新节奏排了，不同意就拒绝，随时可改回。', evidence: '证据：连续 3 次都是早上浇的', ttlRule: 'silent_accept', expiresAt: nowMs() + 24 * 3600e3, domain: 'habit' }));
    // 愿望池 / 资料 / 记忆 / 作业
    s.wishes = [{ id: uid('w'), text: '把《悉达多》看完', effort: 45, at: nowMs() - 3 * 864e5, status: 'active' }, { id: uid('w'), text: '学一点 Rust', effort: 90, at: nowMs() - 6 * 864e5, status: 'active' }, { id: uid('w'), text: '去看美术馆的版画展', effort: 120, at: nowMs() - 2 * 864e5, status: 'active' }, { id: uid('w'), text: '买牛奶', effort: 15, at: nowMs() - 864e5, status: 'active' }];
    s.materials = [{ id: uid('mt'), title: '数据结构考试重点在第三章', body: '老师课上强调：AVL 旋转 + 图遍历必考。', kind: 'academic', at: nowMs() - 2 * 864e5, source: 'chat' }, { id: uid('mt'), title: '图书馆三楼靠窗最安静', body: '下午 2-5 点人最少。', kind: 'life', at: nowMs() - 5 * 864e5, source: 'chat' }, { id: uid('mt'), title: '实验课在 B 楼 201', body: '每周三，记得带学生卡。', kind: 'academic', at: nowMs() - 7 * 864e5, source: 'chat' }, { id: uid('mt'), title: 'Essay 2 评分 rubric', body: '论点 40% / 结构 30% / 引用 20% / 语言 10%', kind: 'academic', at: nowMs() - 4 * 864e5, source: 'upload' }];
    s.memories = [{ id: uid('me'), text: '上午效率高，难的事尽量排上午', type: 'preference', at: nowMs() - 6 * 864e5 }, { id: uid('me'), text: '晚上 11 点后不排任务', type: 'preference', at: nowMs() - 9 * 864e5 }, { id: uid('me'), text: '要去问 CS 助教 office hour 时间', type: 'open_loop', at: nowMs() - 864e5 }];
    const due = (dd, hm) => addDays(T, dd) + 'T' + hm;
    s.assignments = [{ id: uid('a'), course: 'CS 201', title: 'Lab 5：AVL 树实现', dueAt: due(1, '23:59'), done: false }, { id: uid('a'), course: 'MATH 172', title: 'Problem Set 8（级数）', dueAt: due(2, '23:59'), done: false }, { id: uid('a'), course: 'PSYC 110', title: '阅读反思：记忆与遗忘', dueAt: due(3, '23:59'), done: false }, { id: uid('a'), course: 'CS 201', title: 'Quiz 6：图的遍历', dueAt: due(4, '10:00'), done: false }, { id: uid('a'), course: 'ENGL 105', title: 'Essay 2 终稿', dueAt: due(6, '23:59'), done: false }, { id: uid('a'), course: 'PSYC 110', title: '小组课题选题表', dueAt: due(8, '23:59'), done: false }];
    // 心情（今天留空 → 胶囊邀请）
    s.moods = [{ id: uid('m'), e: '😌', l: '平静', note: '', source: 'user', at: dayStartMs(Y) + 21.5 * 3600e3 }, { id: uid('m'), e: '😣', l: '压力大', note: 'Lab 快截止', source: 'agent', at: dayStartMs(addDays(T, -2)) + 16 * 3600e3 }];
    // 今日 L0（静默小数字的来源）
    const t0 = dayStartMs(T);
    [['Canvas 同步：Lab 5 分数已更新', 'archive', 8.2], ['归档：课程通知《期末周安排》', 'archive', 8.2], ['悄悄排上：慢跑 30 分钟', 'schedule', 7.6], ['「买牛奶」已经在清单里，跳过', 'archive', 9.1], ['记住偏好：上午效率高', 'archive', 10.4], ['早上的问候已备好并推送', 'care', 7.5], ['重排：把「慢跑」让位给课程', 'schedule', 11.2], ['悄悄续期：绿萝浇水 +3 天', 'habit', 9.0], ['合并同类：两条 Lab 提醒并成一张卡', 'schedule', 10.9], ['归档：讲座海报（图片）', 'archive', 13.1]].forEach(([label, domain, h]) => { if (t0 + h * 3600e3 < nowMs()) state = s, op({ label, domain, silent: true, agent: true, undoable: false, at: t0 + h * 3600e3, payload: { type: 'l0' } }); });
    s.ops.sort((a, b) => b.at - a.at);
    // 河流种子（近 14 天远景）
    s.riverSeed = Array.from({ length: 14 }, (_, i) => { const d = addDays(T, -(i + 1)); const md = [['😌', 2], ['😊', 1], ['😐', 2], ['😣', 3], ['😌', 2], ['🤩', 1], ['😪', 3]][i % 7]; return { date: d, count: 6 + ((i * 7) % 9), mood: md[0], tone: md[1] }; }).reverse();
    s.pushes = [];
    s.chat = [{ role: 'user', content: '下周三的离散数学考试提前到周五了', at: nowMs() - 2 * 3600e3 }, { role: 'assistant', content: '收到，已经把考试改到周五，复习块也往前挪了一天。不用你再动手。', at: nowMs() - 2 * 3600e3 + 8e3, tools: [{ label: '作业 · 改了截止' }, { label: '改了日程 · 复习块前移' }] }];
    return s;
  }
  function seedCrisis() {
    const s = seedHealthy(); s.scenario = 'crisis'; const T = todayIso(), N = nowMin(), V = Math.min(Math.max(N, 13 * 60 + 30), 20 * 60);
    s.blocks = s.blocks.filter((b) => b.date !== T || phaseOf(b) === 'stone' || toMin(b.start) < V - 200);
    const B = (o) => s.blocks.push(mkBlock(o));
    B({ date: T, start: clampHM(V - 90), dur: 210, title: '赶 Lab 5（截止今晚 23:59）', type: 'task', origin: 'auto' });
    B({ date: T, start: '19:00', dur: 60, title: '社团例会', type: 'appointment', origin: 'manual', lockLevel: 'soft', lockReason: '例会时间是大家一起定的' });
    B({ date: T, start: clampHM(V + 150), dur: 120, title: 'PS8 至少开个头', type: 'task', origin: 'auto' });
    s.assignments[0].dueAt = T + 'T23:59'; s.assignments[1].dueAt = addDays(T, 1) + 'T10:00';
    s.boldness.schedule = { score: 0.22, evid: 2 };
    s.proposals = s.proposals.filter((p) => p.kind === 'timed');
    s.proposals.push(mkProposal({ level: 'L2', kind: 'decision', title: '今晚挤不下了：Lab 5 和 PS8 撞在一起', summary: '两个截止互相打架，我拟了一个让路方案', reason: '例会可以请假，PS8 的死线在明早 10 点——睡前只护住 Lab。', evidence: '证据：两个截止都在 24 小时内 · 今晚还剩 4.5 小时', rows: [{ id: 'r1', label: '社团例会请假，把 19:00-20:00 让给 Lab', state: 'pending', move: true }, { id: 'r2', label: 'PS8 挪到明早 07:30-09:30（你上午效率高）', state: 'pending', date: addDays(T, 1), start: '07:30', dur: 120, btype: 'task' }], ttlRule: 'silent_reject', expiresAt: nowMs() + 4 * 3600e3, domain: 'schedule' }));
    s.proposals.push(mkProposal({ level: 'L2', kind: 'card', title: 'Essay 2 同伴互评今晚也截止', summary: '只要 15 分钟，300 字以内就够', reason: '占分 15 分，性价比最高的一件事——要不要现在见缝插针？', evidence: '证据：Canvas 同步 · 23:59 截止 · 预计 15 分钟', ttlRule: 'silent_reject', expiresAt: nowMs() + 5 * 3600e3, domain: 'schedule' }));
    s.proposals.push(mkProposal({ level: 'L2', kind: 'card', title: '你已经连着熬了两晚了', summary: '今晚 0:30 之后的部分，我建议直接放掉', reason: '睡眠欠着会在周五的考试上讨回来。放掉的部分我明天帮你捡。', evidence: '证据：你已经醒了 19.5 小时 · 平时差不多 23:40 睡', ttlRule: 'silent_reject', expiresAt: nowMs() + 6 * 3600e3, domain: 'care' }));
    s.pushes.push({ id: uid('ps'), at: nowMs() - 20 * 60e3, title: '要不要眯一会？', body: '你快 20 小时没合眼了。上午的安排我先帮你顺延了——去睡一会，醒来再说。', read: false, kind: 'care' });
    s.pushes.push({ id: uid('ps'), at: nowMs() - 3 * 3600e3, title: 'Lab 5 今晚 23:59 截止', body: '还剩最后一段。拆好的两步在卡片堆里。', read: true, kind: 'ddl' });
    s.moods.unshift({ id: uid('m'), e: '😣', l: '压力大', note: '"两个都要交，来不及了" —— 我听到了', source: 'agent', at: nowMs() - 40 * 60e3 });
    state = s; op({ label: '心情打卡（我代记）：😣 压力大', domain: 'care', agent: true, silent: false, payload: { type: 'add_mood', moodId: s.moods[0].id } });
    return s;
  }
  function seedCold() { const s = base(); seedCommon(s); s.scenario = 'cold'; s.rhythm = { wake: '07:30', sleep: '23:00', pinned: false, learning: false }; s.boldness = { schedule: { score: 0.1, evid: 0 }, habit: { score: 0.1, evid: 0 }, archive: { score: 0.3, evid: 0 }, care: { score: 0.3, evid: 0 } }; return s; }
  const SEEDS = { healthy: seedHealthy, crisis: seedCrisis, cold: seedCold };

  // ---------- 载入 ----------
  try { const raw = localStorage.getItem(LS); const parsed = raw ? JSON.parse(raw) : null; state = parsed && parsed.ver === VER && parsed.day === todayIso() ? parsed : null; } catch (e) { state = null; }
  if (!state) { state = seedHealthy(); state.day = todayIso(); save(); } else { state.day = todayIso(); (state.blocks || []).forEach((b) => { if (b.lockLevel === undefined) { delete b.lockLevel; deriveLock(b); } }); if (state.pushes && state.pushes.some((p) => p.kind === 'brief')) { state.pushes = state.pushes.filter((p) => p.kind !== 'brief'); save(); } }

  // ---------- 提案生命周期 ----------
  function sweep() {
    let changed = false;
    state.proposals.forEach((p) => {
      if (p.state === 'pending' && p.expiresAt < nowMs()) {
        if (p.ttlRule === 'silent_accept') { applyProposal(p, null, true); p.state = 'accepted'; op({ label: '没反对，照做了：' + p.title, domain: p.domain, agent: true, silent: true, undoable: true, payload: { type: 'proposal_accept', pid: p.id, blockIds: p._created || [] } }); }
        else { p.state = 'expired'; op({ label: '这条提议过了：' + p.title, sub: '想要还能捞回', domain: p.domain, agent: true, silent: true, undoable: false, payload: { type: 'proposal_expire', pid: p.id } }); }
        changed = true;
      }
    });
    if (changed) emit();
  }
  setInterval(sweep, 30e3); setTimeout(sweep, 800);
  function applyProposal(p, rowId, silent) {
    p._created = p._created || [];
    const rows = p.rows ? p.rows.filter((r) => (rowId ? r.id === rowId : r.state === 'pending')) : [];
    if (p.kind === 'timed' && !rowId) { const b = mkBlock({ date: p.date, start: p.start, dur: p.dur, title: p.title, type: p.btype, origin: 'auto', lockLevel: p.lockLevel, lockReason: p.lockReason }); state.blocks.push(b); p._created.push(b.id); }
    rows.forEach((r) => { r.state = 'accepted'; if (r.date && r.start) { const b = mkBlock({ date: r.date, start: r.start, dur: r.dur || 60, title: p.title.includes('拆') ? r.label.replace(/（.*/, '') : r.label, type: r.btype || 'task', origin: 'auto' }); state.blocks.push(b); p._created.push(b.id); } });
    return p._created;
  }
  function proposalDone(p) { return !p.rows || p.rows.every((r) => r.state !== 'pending'); }

  // ---------- 查询 ----------
  const blocksOf = (date) => state.blocks.filter((b) => b.date === date).sort((a, b) => toMin(a.start) - toMin(b.start));
  const ghostsOf = (date) => state.proposals.filter((p) => p.state === 'pending' && p.kind === 'timed' && p.date === date);
  const stack = () => state.proposals.filter((p) => p.state === 'pending' && p.kind !== 'timed').slice(0, 3);
  const l0Today = () => state.ops.filter((o) => o.silent && !o.undone && o.at >= dayStartMs(todayIso())).length;
  function ledgerByDay(days) { const from = dayStartMs(addDays(todayIso(), -(days - 1))); const rows = state.ops.filter((o) => o.at >= from); const g = {}; rows.forEach((o) => { const d = iso(new Date(o.at)); (g[d] = g[d] || []).push(o); }); return Object.keys(g).sort().reverse().map((d) => ({ date: d, rows: g[d] })); }
  function river() { const real = {}; state.ops.forEach((o) => { const d = iso(new Date(o.at)); real[d] = (real[d] || 0) + 1; }); const mood = {}; state.moods.forEach((m) => { const d = iso(new Date(m.at)); if (!mood[d]) mood[d] = m.e; }); const T = todayIso(); const days = state.riverSeed.map((r) => ({ ...r })); days.push({ date: T, count: real[T] || 0, mood: mood[T] || null, tone: 2 }); days.forEach((r) => { if (real[r.date]) r.count = Math.max(r.count, real[r.date]); if (mood[r.date]) r.mood = mood[r.date]; }); return days; }
  function radar() { return state.assignments.filter((a) => !a.done).map((a) => { const ms = new Date(a.dueAt.replace('T', 'T') + ':00').getTime(); const dh = (ms - nowMs()) / 3600e3; const [d, hm] = a.dueAt.split('T'); const label = (d === todayIso() ? '今天' : d === addDays(todayIso(), 1) ? '明天' : weekdayShort(d) + ' ' + fmtDate(d)) + ' ' + hm; return { ...a, ms, dueLabel: label, urgency: dh < 26 ? 2 : dh < 76 ? 1 : 0 }; }).sort((a, b) => a.ms - b.ms); }
  function gapsOf(date, minMin) { minMin = minMin || 25; const T = todayIso(); if (date < T) return []; const wake = toMin(state.rhythm.wake || '07:30'); let sleep = toMin(state.rhythm.sleep || '23:30'); if (sleep <= wake) sleep = 24 * 60; let cur = date === T ? Math.max(nowMin(), wake) : wake; const out = []; blocksOf(date).forEach((b) => { const s = toMin(b.start), e = s + b.dur; if (s - cur >= minMin) out.push({ start: toHM(cur), end: toHM(s), min: s - cur }); if (e > cur) cur = e; }); if (sleep - cur >= minMin) out.push({ start: toHM(cur), end: toHM(sleep), min: sleep - cur }); return out; }
  function flowNow() { const T = todayIso(), n = nowMin(); let current = null, next = null, prev = null, nextTomorrow = false; blocksOf(T).forEach((b) => { const s = toMin(b.start), e = s + b.dur; if (s <= n && n < e) current = b; else if (e <= n) prev = b; else if (!next) next = b; }); if (!next) { const tm = blocksOf(addDays(T, 1))[0]; if (tm) { next = tm; nextTomorrow = true; } } const gapMin = !current && next && !nextTomorrow ? toMin(next.start) - n : current ? 0 : null; return { current, next, prev, nextTomorrow, gapMin }; }
  const moodToday = () => state.moods.find((m) => m.at >= dayStartMs(todayIso()) && m.source === 'user');
  const anyMoodToday = () => state.moods.find((m) => m.at >= dayStartMs(todayIso()));

  // ---------- 导语 ----------
  function brief() {
    const T = todayIso(), bs = blocksOf(T);
    if (!bs.length) return { honest: true, title: '今天还是空的', lines: ['不是坏事——说明还没人替你着急。', '跟我说一句今天有什么，我来搭起来；', '或者什么都不说，今天就这样也很好。'], hint: '上滑即走 · 不反对就是接受' };
    const fixed = bs.filter((b) => b.type === 'appointment').length; const gaps = '下午刻意留了空档';
    return { honest: false, title: relDay(T) + ' · ' + fmtDate(T, true), lines: [bs.length + ' 件事，其中 ' + fixed + ' 件是定点的课。', gaps + '——Lab 5 想动工的话正好。', '慢跑放在傍晚，不想去就划掉，没关系。'], hint: '上滑即走 · 过午自动消散' };
  }
  const briefVisible = () => new Date().getHours() < 12 && state.briefDismissed !== todayIso();
  function dismissBrief() { state.briefDismissed = todayIso(); emit(); }

  // ---------- 写操作 ----------
  function addBlock(o, agent) { const b = mkBlock(o); state.blocks.push(b); const opId = op({ label: (agent ? '排上：' : '添加：') + b.title + '（' + relDay(b.date) + ' ' + b.start + '）', domain: 'schedule', agent: !!agent, payload: { type: 'add_block', blockId: b.id } }); emit(); return { blockId: b.id, opId }; }
  function moveBlock(id, to, agent) {
    const b = state.blocks.find((x) => x.id === id); if (!b) return { ok: false };
    if (phaseOf(b) === 'stone' && (to.date > b.date || (to.date === b.date && toMin(to.start) > toMin(b.start)))) return { stone: true };
    if (b.lockLevel === 'hard' && !agent) return { locked: 'hard', reason: b.lockReason || '这一项的时间不由你定' };
    if (b.lockLevel === 'soft' && !agent && !to._confirmed) return { locked: 'soft', reason: b.lockReason || '和别人约好的时间', pending: to };
    const prev = { date: b.date, start: b.start }; Object.assign(b, to); if (b.origin === 'auto' && !agent) b.origin = 'manual';
    const opId = op({ label: '挪动：' + b.title + ' → ' + relDay(b.date) + ' ' + b.start, domain: 'schedule', agent: !!agent, payload: { type: 'move_block', blockId: id, prev } }); emit(); return { ok: true, opId };
  }
  function proposeReschedule(blockId) { const b = state.blocks.find((x) => x.id === blockId); if (!b) return null; const p = mkProposal({ level: 'L2', kind: 'timed', title: '重新安排：' + b.title, summary: '过去的保持原样，在明天放一个新的', reason: '过去的记录不改写——原来那件留着，新的放到明天同一时段。', date: addDays(todayIso(), 1), start: b.start, dur: b.dur, btype: b.type, ttlRule: 'silent_reject', expiresAt: nowMs() + 6 * 3600e3, domain: 'schedule' }); state.proposals.push(p); op({ label: '发起重新安排：' + b.title, domain: 'schedule', undoable: false, payload: { type: 'l0' } }); emit(); return p.id; }
  function setStatus(id, status) { const b = state.blocks.find((x) => x.id === id); if (!b) return null; const prev = b.status; b.status = status; const opId = op({ label: (status === 'done' ? '完成：' : status === 'missed' ? '标记没做：' : '恢复：') + b.title, domain: 'schedule', payload: { type: 'status', blockId: id, prev } }); emit(); return opId; }
  function reconcile(id, verdict, note) { const b = state.blocks.find((x) => x.id === id); if (!b) return null; const prev = { status: b.status, reconciled: b.reconciled, note: b.note }; b.status = verdict === 'did' ? 'done' : verdict; b.reconciled = true; if (note) b.note = note; const vl = { done: '完成了', missed: '没做（没关系）', did: '其实做了' }[verdict] || verdict; const opId = op({ label: '回顾：' + b.title + ' · ' + vl, domain: 'schedule', payload: { type: 'reconcile', blockId: id, prev } }); emit(); return opId; }
  function setNote(id, note) { const b = state.blocks.find((x) => x.id === id); if (!b) return; const prev = b.note; b.note = note; op({ label: '注释：' + b.title, domain: 'schedule', payload: { type: 'note', blockId: id, prev } }); emit(); }
  function removeBlock(id) { const b = state.blocks.find((x) => x.id === id); if (!b) return; const opId = op({ label: '移走：' + b.title, domain: 'schedule', payload: { type: 'remove_block', block: { ...b } } }); state.blocks = state.blocks.filter((x) => x.id !== id); emit(); return opId; }
  function accept(pid, rowId) { const p = state.proposals.find((x) => x.id === pid); if (!p || p.state !== 'pending') return null; const created = applyProposal(p, rowId); if (proposalDone(p)) p.state = 'accepted'; const bd = state.boldness[p.domain]; if (bd) { bd.score = Math.min(0.95, bd.score + 0.03); bd.evid++; } const opId = op({ label: '采纳：' + p.title + (rowId ? '（其中一行）' : ''), domain: p.domain, agent: false, payload: { type: 'proposal_accept', pid, rowId: rowId || null, blockIds: created } }); emit(); return { opId }; }
  function reject(pid, rowId) { const p = state.proposals.find((x) => x.id === pid); if (!p || p.state !== 'pending') return null; if (rowId && p.rows) { const r = p.rows.find((x) => x.id === rowId); if (r) r.state = 'rejected'; if (proposalDone(p)) p.state = 'rejected'; } else p.state = 'rejected'; const bd = state.boldness[p.domain]; if (bd) bd.score = Math.max(0.05, bd.score - 0.05); const opId = op({ label: '谢绝：' + p.title + (rowId ? '（其中一行）' : ''), domain: p.domain, payload: { type: 'proposal_reject', pid, rowId: rowId || null } }); emit(); return { opId }; }
  function redeem(pid) { const p = state.proposals.find((x) => x.id === pid); if (!p || p.state !== 'expired') return; p.state = 'pending'; p.expiresAt = nowMs() + 6 * 3600e3; op({ label: '捞回提议：' + p.title, domain: p.domain, undoable: false, payload: { type: 'l0' } }); emit(); }
  function followUp(pid, q) { const p = pid ? state.proposals.find((x) => x.id === pid) : null; state.exchange = { id: uid('ex'), q: q || (p ? '关于提案「' + p.title + '」' : ''), snapshot: p ? { title: p.title, reason: p.reason, evidence: p.evidence } : null, expiresAt: nowMs() + 10 * 60e3 }; emit(); return state.exchange.id; }
  function consumeExchange() { const ex = state.exchange; if (!ex) return null; if (ex.expiresAt < nowMs()) { state.exchange = null; return null; } state.exchange = null; return ex; }
  function wishAdd(text, effort, agent) { const w = { id: uid('w'), text, effort: effort || null, at: nowMs(), status: 'active' }; const dup = state.wishes.find((x) => x.status === 'active' && x.text.replace(/\s/g, '') === text.replace(/\s/g, '')); if (dup) { const opId = op({ label: '「' + text + '」已经在想做清单里', domain: 'archive', agent: !!agent, silent: true, undoable: false, payload: { type: 'l0' } }); emit(); return { wishId: dup.id, opId, dup: true }; } state.wishes.unshift(w); const opId = op({ label: '记进想做清单：「' + text + '」' + (effort ? '（约 ' + effort + ' 分钟）' : ''), domain: 'archive', agent: !!agent, payload: { type: 'add_wish', wishId: w.id } }); emit(); return { wishId: w.id, opId }; }
  function wishSet(id, status) { const w = state.wishes.find((x) => x.id === id); if (!w) return; w.status = status; op({ label: (status === 'done' ? '愿望达成：' : '放下愿望：') + w.text, domain: 'archive', undoable: false, payload: { type: 'l0' } }); emit(); }
  function moodRecord(o) { const exist = moodToday(); if (exist && o.source === 'agent') { const opId = op({ label: '你今天记过了——只补了一句', domain: 'care', agent: true, silent: true, undoable: false, payload: { type: 'l0' } }); emit(); return { blocked: true, opId }; } const m = { id: uid('m'), e: o.e, l: o.l, note: o.note || '', source: o.source || 'user', at: nowMs() }; state.moods.unshift(m); const opId = op({ label: '心情打卡' + (o.source === 'agent' ? '（我代记）' : '') + '：' + m.e + ' ' + m.l, domain: 'care', agent: o.source === 'agent', payload: { type: 'add_mood', moodId: m.id } }); emit(); return { moodId: m.id, opId }; }
  function materialAdd(o, agent) { const m = { id: uid('mt'), title: o.title, body: o.body || '', kind: o.kind || 'life', at: nowMs(), source: o.source || 'chat' }; state.materials.unshift(m); const opId = op({ label: '收进资料：「' + m.title + '」', sub: '随时能改', domain: 'archive', agent: !!agent, payload: { type: 'add_material', materialId: m.id } }); emit(); return { materialId: m.id, opId }; }
  function materialRemove(id) { const m = state.materials.find((x) => x.id === id); if (!m) return; const opId = op({ label: '移出资料：' + m.title, domain: 'archive', payload: { type: 'remove_material', material: { ...m } } }); state.materials = state.materials.filter((x) => x.id !== id); emit(); return opId; }
  function memoryAdd(o, agent) { const m = { id: uid('me'), text: o.text, type: o.type || 'preference', at: nowMs() }; state.memories.unshift(m); const opId = op({ label: '记住：' + m.text, domain: 'archive', agent: !!agent, payload: { type: 'add_memory', memoryId: m.id } }); emit(); return { memoryId: m.id, opId }; }
  function memoryRemove(id) { const m = state.memories.find((x) => x.id === id); if (!m) return; const opId = op({ label: '忘掉：' + m.text, domain: 'archive', payload: { type: 'remove_memory', memory: { ...m } } }); state.memories = state.memories.filter((x) => x.id !== id); emit(); return opId; }
  function markPushRead(id) { const p = state.pushes.find((x) => x.id === id); if (p) { p.read = true; emit(); } }

  // ---------- 撤销 = 反向补偿 ----------
  function undo(opId) {
    const o = findOp(opId); if (!o || !o.undoable || o.undone) return false;
    const P = o.payload;
    if (P.type === 'add_block') state.blocks = state.blocks.filter((b) => b.id !== P.blockId);
    else if (P.type === 'remove_block') state.blocks.push(P.block);
    else if (P.type === 'move_block') { const b = state.blocks.find((x) => x.id === P.blockId); if (b) Object.assign(b, P.prev); }
    else if (P.type === 'status') { const b = state.blocks.find((x) => x.id === P.blockId); if (b) b.status = P.prev; }
    else if (P.type === 'bulk_blocks') { state.blocks = state.blocks.filter((b) => P.added.indexOf(b.id) < 0); P.removed.forEach((b) => state.blocks.push(b)); }
    else if (P.type === 'set_mark' || P.type === 'set_fields') { const b = state.blocks.find((x) => x.id === P.blockId); if (b) Object.assign(b, P.prev); }
    else if (P.type === 'assignment_done') { const a = state.assignments.find((x) => x.id === P.assignmentId); if (a) a.done = P.prev; }
    else if (P.type === 'reconcile' || P.type === 'note') { const b = state.blocks.find((x) => x.id === P.blockId); if (b) Object.assign(b, P.prev); }
    else if (P.type === 'add_wish') state.wishes = state.wishes.filter((w) => w.id !== P.wishId);
    else if (P.type === 'add_mood') state.moods = state.moods.filter((m) => m.id !== P.moodId);
    else if (P.type === 'add_material') state.materials = state.materials.filter((m) => m.id !== P.materialId);
    else if (P.type === 'remove_material') state.materials.unshift(P.material);
    else if (P.type === 'add_memory') state.memories = state.memories.filter((m) => m.id !== P.memoryId);
    else if (P.type === 'remove_memory') state.memories.unshift(P.memory);
    else if (P.type === 'proposal_accept') { const p = state.proposals.find((x) => x.id === P.pid); (P.blockIds || []).forEach((bid) => { state.blocks = state.blocks.filter((b) => b.id !== bid); }); if (p) { p.state = 'pending'; p.expiresAt = Math.max(p.expiresAt, nowMs() + 3600e3); if (P.rowId && p.rows) { const r = p.rows.find((x) => x.id === P.rowId); if (r) r.state = 'pending'; } else if (p.rows) p.rows.forEach((r) => { if (r.state === 'accepted') r.state = 'pending'; }); } }
    else if (P.type === 'proposal_reject') { const p = state.proposals.find((x) => x.id === P.pid); if (p) { p.state = 'pending'; p.expiresAt = Math.max(p.expiresAt, nowMs() + 3600e3); if (P.rowId && p.rows) { const r = p.rows.find((x) => x.id === P.rowId); if (r) r.state = 'pending'; } } }
    else return false;
    o.undone = true;
    if (o.agent) { const bd = state.boldness[o.domain]; if (bd) bd.score = Math.max(0.05, bd.score - 0.07); }
    op({ label: '撤销 · ' + o.label, sub: '原来那笔也留着——这是新记的一笔「撤销」', domain: o.domain, undoable: false, payload: { type: 'revert', of: o.id } });
    emit(); return true;
  }

  // ---------- 意图路由（模拟 agent 全权判断）----------
  const RE_TIME = /(前天|昨天|今晚|今天|明晚|明天|后天|周[一二三四五六日天])?\s*(早上|上午|中午|下午|晚上)?\s*(\d{1,2})\s*[点:：时]\s*(半|\d{1,2})?/;
  function parseWhen(t) {
    const m = t.match(RE_TIME); if (!m || m[3] === undefined) return null;
    let h = +m[3]; const mi = m[4] === '半' ? 30 : (+m[4] || 0);
    const seg = m[2] ? m[2] : ((m[1] || '').includes('晚') ? '晚上' : null);
    if ((seg === '下午' || seg === '晚上') && h < 12) h += 12; if (seg === '中午' && h < 11) h += 12;
    if (!m[2] && !(m[1] || '').includes('晚') && h <= 7) h += 12; // 裸"3点"偏向下午
    let date = todayIso(); const d1 = m[1] || '';
    if (d1.includes('明')) date = addDays(date, 1); else if (d1 === '后天') date = addDays(date, 2); else if (d1 === '昨天') date = addDays(date, -1); else if (d1 === '前天') date = addDays(date, -2);
    else if (/^周/.test(d1)) { const idx = '日一二三四五六'.indexOf(d1[1] === '天' ? '日' : d1[1]); const cur = new Date().getDay(); let diff = (idx - cur + 7) % 7 || 7; date = addDays(todayIso(), diff); }
    return { date, start: toHM(h * 60 + mi), matched: m[0] };
  }
  function guessType(t) { return /课|会|例会|讲座|约|面试/.test(t) ? 'appointment' : /跑|健身|散步|运动|球/.test(t) ? 'relax' : /饭|餐|吃/.test(t) ? 'meal' : /休息|午休|眯/.test(t) ? 'break' : 'task'; }
  function guessDur(t) { return /课|会/.test(t) ? 90 : /跑|散步/.test(t) ? 30 : /饭|吃/.test(t) ? 45 : 60; }
  function guessEffort(t) { return /买|打电话|发/.test(t) ? 15 : /展|电影|逛/.test(t) ? 120 : /学|读|看完/.test(t) ? 90 : 45; }
  const MOOD_WORDS = [['累', '😪', '疲惫'], ['疲惫', '😪', '疲惫'], ['压力', '😣', '压力大'], ['焦虑', '😟', '焦虑'], ['紧张', '😟', '焦虑'], ['烦', '😠', '烦躁'], ['难受', '😢', '低落'], ['低落', '😢', '低落'], ['emo', '😢', '低落'], ['开心', '😊', '开心'], ['高兴', '😊', '开心'], ['平静', '😌', '平静'], ['轻松', '😌', '平静'], ['兴奋', '🤩', '兴奋'], ['睡不着', '🌙', '失眠']];
  function captureIntent(text, opts) {
    const t = (text || '').trim(); if (!t) return null; opts = opts || {};
    // 查到 → 浮层，不落库（共识 3/16）
    if (/[?？]$|^(查|问)|什么时候|啥时候|还有几|还剩|多少分|截止|ddl|deadline/i.test(t) && !parseWhen(t)) {
      if (/作业|截止|ddl|deadline|考/i.test(t)) { const top = radar().slice(0, 3); return { kind: 'answer', title: '最近的截止', lines: top.map((a) => a.dueLabel + ' · ' + a.course + '《' + a.title + '》'), view: 'outlook' }; }
      if (/今天|安排|日程/.test(t)) { const bs = blocksOf(todayIso()); return { kind: 'answer', title: '今天 ' + bs.length + ' 件事', lines: bs.slice(0, 4).map((b) => b.start + ' ' + b.title), view: 'today' }; }
      return { kind: 'answer', title: '我查了一下', lines: ['（演示环境）真实版本会带着来源回答这句。'], view: null };
    }
    const when = parseWhen(t);
    if (when) {
      const title = t.replace(when.matched, '').replace(/^[，,。 ]+|[，,。 ]+$/g, '') || '未命名安排';
      const r = addBlock({ date: when.date, start: when.start, dur: guessDur(t), title, type: guessType(t), origin: 'auto' }, true);
      return { kind: 'executed', label: '放进 ' + relDay(when.date) + ' ' + when.start + ' 了', sub: '小禾排的——不合适随手拖走', opId: r.opId, blockId: r.blockId, view: 'today', date: when.date };
    }
    for (const [w, e, l] of MOOD_WORDS) if (t.includes(w)) { const reason = /考|作业|ddl|截止|明天/i.test(t); const r = moodRecord({ e, l, note: '「' + t + '」', source: 'agent' }); let memOpId = null; if (reason && !r.blocked) memOpId = memoryAdd({ text: '压力源：' + t.slice(0, 24), type: 'open_loop' }, true).opId; return { kind: 'mood', label: r.blocked ? '你今天打过卡了，帮你补了一条 note' : '心情记下了：' + e + ' ' + l, sub: reason ? '还顺手记了压力源，撤销随时' : '原话进了 note', opId: r.opId, view: 'mood' }; }
    if (/^(想|改天|有机会|回头|哪天)|想学|想去|想看|想买|想试/.test(t)) { const eff = guessEffort(t); const r = wishAdd(t.replace(/^(想要?|改天|有机会|回头|哪天)/, '').trim() || t, eff, true); return { kind: 'wish', label: r.dup ? '这条已经在想做清单里，帮你合并' : '记进想做清单了（约 ' + eff + ' 分钟）', sub: '哪天出现空档，我会试着提出来问你', opId: r.opId, view: 'outlook' }; }
    if (/我(总是|一般|习惯|通常|不)|效率|点后|点前/.test(t)) { const r = memoryAdd({ text: t.slice(0, 40), type: 'preference' }, true); return { kind: 'memory', label: '记住了（偏好）', sub: '在资料·记忆里，可见可改可撤', opId: r.opId, view: 'materials' }; }
    if (/老师|教室|重点|图书馆|实验|楼|教材|考试|rubric/i.test(t) || t.length >= 26 || opts.focus === 'archive') { const r = materialAdd({ title: t.slice(0, 22) + (t.length > 22 ? '…' : ''), body: t, kind: /课|考|老师|实验/.test(t) ? 'academic' : 'life' }, true); return { kind: 'material', label: '收进资料库了', sub: '翻资料随时能改', opId: r.opId, view: 'materials' }; }
    const r = wishAdd(t, null, true); return { kind: 'wish', label: '先记进「想做的事」', sub: '还没定时间——想做的时候再排', opId: r.opId, view: 'outlook' };
  }

  // ---------- Companion（唯一的对话流）----------
  function chatSend(text) {
    const t = (text || '').trim(); if (!t) return;
    state.chat.push({ role: 'user', content: t, at: nowMs() }); state.typing = true; emit();
    setTimeout(() => {
      let reply = '', tools = [], proposalId = null;
      const when = parseWhen(t);
      const moodHit = MOOD_WORDS.find(([w]) => t.includes(w));
      if (moodHit) { const r = moodRecord({ e: moodHit[1], l: moodHit[2], note: '「' + t + '」', source: 'agent' }); if (!r.blocked) tools.push({ label: '心情打卡 · ' + moodHit[1] + ' ' + moodHit[2], opId: r.opId }); if (/考|作业|截止/.test(t)) { const m = memoryAdd({ text: '压力源：' + t.slice(0, 24), type: 'open_loop' }, true); tools.push({ label: '记了一笔 · 压力源', opId: m.opId }); } reply = moodHit[2] === '压力大' || moodHit[2] === '焦虑' ? '听起来真的不轻松。' + (radar()[0] ? '最近的截止是' + radar()[0].dueLabel + '的《' + radar()[0].title + '》——要不要我把它拆小一点？' : '先深呼吸一下，我在。') : moodHit[2] === '开心' || moodHit[2] === '兴奋' ? '真好，这样的日子值得记一笔。已经帮你打了卡。' : '嗯，我记下了。不用回我，休息要紧。'; }
      else if (when) { const title = t.replace(when.matched, '').replace(/^[，,。 ]+|[，,。 ]+$/g, '') || '新安排'; const r = addBlock({ date: when.date, start: when.start, dur: guessDur(t), title, type: guessType(t), origin: 'auto' }, true); tools.push({ label: '排进日程 · ' + relDay(when.date) + ' ' + when.start, opId: r.opId }); reply = '放进' + relDay(when.date) + ' ' + when.start + '了，小禾排的。位置不合适直接拖，或者跟我说一句。'; }
      else if (/怎么安排|帮我(排|安排|规划)|来不及|挤/.test(t)) { const p = mkProposal({ level: 'L2', kind: 'decision', title: '一个安排建议', summary: '按你上午效率高的习惯拟的', reason: '难的放上午、碎的填空档；不同意就整行拒绝。', evidence: '证据：你说过上午效率高 · 又看了眼几个截止', rows: [{ id: 'r1', label: '明早 08:00 先做最要紧的 90 分钟', state: 'pending', date: addDays(todayIso(), 1), start: '08:00', dur: 90, btype: 'task' }, { id: 'r2', label: '下午留白，晚上 20:00 收尾 45 分钟', state: 'pending', date: addDays(todayIso(), 1), start: '20:00', dur: 45, btype: 'task' }], expiresAt: nowMs() + 8 * 3600e3 }); state.proposals.push(p); proposalId = p.id; tools.push({ label: '拟了个提议 · 2 行' }); reply = '我拟了一个，两行可以分开点头：'; }
      else if (/^(想|改天|有机会)|想学|想去|想看/.test(t)) { const eff = guessEffort(t); const r = wishAdd(t.replace(/^(想要?|改天|有机会)/, '').trim() || t, eff, true); tools.push({ label: '想做清单 · 约 ' + eff + ' 分钟', opId: r.opId }); reply = '记进想做清单了。哪天你日历上出现一块' + eff + '分钟的空档，我会想起它。'; }
      else if (/截止|ddl|作业|考/i.test(t)) { const top = radar().slice(0, 3); reply = top.length ? '眼下排在前面的：' + top.map((a) => a.dueLabel + '《' + a.title + '》').join('；') + '。要我把哪个拆进日程？' : '雷达上暂时是空的。'; }
      else if (/老师|教室|重点|图书馆|实验/.test(t)) { const r = materialAdd({ title: t.slice(0, 22), body: t, kind: 'academic' }, true); tools.push({ label: '收进资料 · 长期参考', opId: r.opId }); reply = '这个以后可能用得上，收进资料库了。'; }
      else reply = '嗯，我在听。要紧的我会记下来——你只管说。';
      state.chat.push({ role: 'assistant', content: reply, at: nowMs(), tools: tools.length ? tools : null, proposalId }); state.typing = false; emit();
    }, 900 + Math.random() * 500);
  }

  function scenarioSet(name) { if (!SEEDS[name]) return; const themes = state.themes, custom = state.customThemes; state = SEEDS[name](); state.day = todayIso(); state.themes = themes; if (custom && custom.length) state.customThemes = custom; emit(); }
  // ---------- 主题工作室（内置 4 + AI 生成，12 变量白名单） ----------
  const THEME_PRESETS = [
    { match: /海|ocean|sea|深夜|midnight|night/i, name: '深夜的海', dark: true, base: 'night', variables: { '--primary': '#4fd1c5', '--accent': '#2c7a7b', '--bg-start': '#0a1628', '--bg-end': '#123047', '--text-primary': '#e6fffa', '--text-secondary': 'rgba(230,255,250,0.72)', '--text-muted': 'rgba(230,255,250,0.45)', '--surface': 'rgba(79,209,197,0.08)', '--surface-hover': 'rgba(79,209,197,0.16)', '--success': '#68d391', '--warning': '#f6ad55', '--error': '#fc8181' } },
    { match: /奶茶|milk|tea|beige|温柔|奶油|cream/i, name: '奶茶时光', dark: false, base: 'sunset', variables: { '--primary': '#b08968', '--accent': '#9c6644', '--bg-start': '#fdf6ec', '--bg-end': '#f3e5d0', '--text-primary': '#4a3728', '--text-secondary': 'rgba(74,55,40,0.72)', '--text-muted': 'rgba(74,55,40,0.45)', '--surface': 'rgba(255,252,246,0.72)', '--surface-hover': 'rgba(255,252,246,0.9)', '--success': '#7f9d6c', '--warning': '#d9915b', '--error': '#c96f5e' } },
    { match: /森|forest|苔|moss|绿|green/i, name: '雨后苔原', dark: false, base: 'nature', variables: { '--primary': '#4a7c59', '--accent': '#3a5f48', '--bg-start': '#eef5ee', '--bg-end': '#d9e8d5', '--text-primary': '#22331f', '--text-secondary': 'rgba(34,51,31,0.72)', '--text-muted': 'rgba(34,51,31,0.45)', '--surface': 'rgba(255,255,255,0.66)', '--surface-hover': 'rgba(255,255,255,0.85)', '--success': '#5a9367', '--warning': '#d19a3f', '--error': '#c4574d' } },
    { match: /紫|薰衣草|lavender|purple|梦/i, name: '薰衣草梦', dark: false, base: 'sky', variables: { '--primary': '#8b7ec8', '--accent': '#6c5fa8', '--bg-start': '#f5f2fd', '--bg-end': '#e4ddf6', '--text-primary': '#332d4d', '--text-secondary': 'rgba(51,45,77,0.72)', '--text-muted': 'rgba(51,45,77,0.45)', '--surface': 'rgba(255,255,255,0.66)', '--surface-hover': 'rgba(255,255,255,0.85)', '--success': '#6faf7e', '--warning': '#d9a23f', '--error': '#cd6a60' } },
  ];
  function aiTheme(description, baseId) {
    const d = (description || '').trim();
    if (d.length < 2 || /^(哈哈|hello|hi|你好)$/i.test(d)) return { error: 'not_a_theme' };
    const hit = THEME_PRESETS.find((p) => p.match.test(d));
    if (hit) return { name: hit.name, dark: hit.dark, base: hit.base, variables: { ...hit.variables } };
    let h = 0; for (let i = 0; i < d.length; i++) h = (h * 31 + d.charCodeAt(i)) % 360;
    const dark = /深|暗|dark|夜/i.test(d);
    const P = (l, c) => 'oklch(' + l + ' ' + c + ' ' + h + ')';
    return {
      name: d.slice(0, 8), dark, base: baseId || (dark ? 'night' : 'sky'),
      variables: dark ? { '--primary': P(0.75, 0.12), '--accent': P(0.6, 0.1), '--bg-start': P(0.2, 0.04), '--bg-end': P(0.28, 0.05), '--text-primary': P(0.96, 0.01), '--text-secondary': 'rgba(255,255,255,0.72)', '--text-muted': 'rgba(255,255,255,0.45)', '--surface': 'rgba(255,255,255,0.07)', '--surface-hover': 'rgba(255,255,255,0.13)', '--success': '#68d391', '--warning': '#f6ad55', '--error': '#fc8181' }
        : { '--primary': P(0.55, 0.13), '--accent': P(0.45, 0.11), '--bg-start': P(0.97, 0.01), '--bg-end': P(0.92, 0.03), '--text-primary': P(0.25, 0.04), '--text-secondary': 'rgba(40,40,50,0.72)', '--text-muted': 'rgba(40,40,50,0.45)', '--surface': 'rgba(255,255,255,0.6)', '--surface-hover': 'rgba(255,255,255,0.8)', '--success': '#48a56a', '--warning': '#d9a23f', '--error': '#cd6a60' },
    };
  }
  function createTheme(input) { const t2 = Object.assign({ id: uid('th'), createdAt: nowMs() }, input); state.customThemes.push(t2); op({ label: '存了新主题：「' + t2.name + '」', domain: 'archive', agent: true, silent: true, undoable: false, payload: { type: 'l0' } }); emit(); return t2; }
  function patchTheme(id, changes) { const t2 = state.customThemes.find((x) => x.id === id); if (t2) { Object.assign(t2, changes); emit(); } return t2; }
  function deleteTheme(id) { state.customThemes = state.customThemes.filter((x) => x.id !== id); Object.keys(state.themes).forEach((app) => { if (state.themes[app] === id) state.themes[app] = 'sky'; }); emit(); }
  function patch(partial) { Object.assign(state, partial); emit(); }
  function setPref(k, v) { state.prefs[k] = v; emit(); }
  function bindChannel(id) { return { token: 'DC-' + Math.random().toString(36).slice(2, 8).toUpperCase() }; }
  function verifyChannel(id) { const c = state.channels.find((x) => x.id === id); if (c) { c.bound = true; c.externalId = id === 'qq' ? 'QQ 5521 0093' : '@linke_bot'; op({ label: '绑定通道：' + id.toUpperCase(), domain: 'archive', undoable: false, payload: { type: 'l0' } }); emit(); } }
  function unbindChannel(id) { const c = state.channels.find((x) => x.id === id); if (c) { c.bound = false; c.externalId = null; emit(); } }
  const DEFAULT_CATS = [{ id: 'note', label: '随手笔记', locked: true }, { id: 'food', label: '饮食' }, { id: 'health', label: '健康' }, { id: 'academic', label: '学业' }, { id: 'travel', label: '出行' }, { id: 'finance', label: '财务', off: true }, { id: 'fitness', label: '健身', off: true }];
  function listCategories() { if (!state.categories) { state.categories = DEFAULT_CATS.map((c) => ({ id: c.id, label: c.label, locked: !!c.locked, enabled: !c.off })); save(); } return state.categories; }
  function setCategory(id, on) { const c = listCategories().find((x) => x.id === id); if (c && !c.locked) { c.enabled = !!on; emit(); } }
  function theme(app) { return (state.themes || {})[app] || 'sky'; }
  function setTheme(app, v) { state.themes[app] = v; emit(); }
  const boldness = (d) => { const b = state.boldness[d] || { score: 0.5, evid: 0 }; return { ...b, phase: b.score < 0.45 ? '学徒期' : '管家期' }; };
  const pushBudget = () => ({ used: state.pushes.filter((p) => p.at >= dayStartMs(todayIso())).length, max: 3 });
  // 统一标记：'none' | 'done' | 'missed'，任何时候都能改回去（撤销=反向补偿）
  function setMark(id, mark) {
    const b = state.blocks.find((x) => x.id === id); if (!b) return null;
    const prev = { status: b.status, reconciled: b.reconciled };
    if (mark === 'none') { b.status = 'planned'; b.reconciled = false; }
    else { b.status = mark; b.reconciled = phaseOf(b) !== 'future'; }
    const lb = mark === 'none' ? '取消标记：' : mark === 'done' ? '标为完成：' : '标为没做：';
    const opId = op({ label: lb + b.title, domain: 'schedule', payload: { type: 'set_mark', blockId: id, prev } });
    emit(); return { opId, mark };
  }
  function bulkBlocks(o) {
    const added = [], removed = [];
    (o.add || []).forEach((x) => { const b = mkBlock(x); state.blocks.push(b); added.push(b.id); });
    (o.removeIds || []).forEach((id) => { const i = state.blocks.findIndex((b) => b.id === id); if (i >= 0) removed.push(state.blocks.splice(i, 1)[0]); });
    if (!added.length && !removed.length) return null;
    const opId = op({ label: o.label || '批量调整', domain: 'schedule', payload: { type: 'bulk_blocks', added, removed } });
    emit(); return { opId, added: added.length, removed: removed.length };
  }
  function setBlockFields(id, f) {
    const b = state.blocks.find((x) => x.id === id); if (!b) return null;
    const prev = { title: b.title, type: b.type, dur: b.dur };
    if (f.title != null && f.title !== '') b.title = f.title;
    if (f.type) b.type = f.type;
    if (f.dur) b.dur = Number(f.dur) || b.dur;
    const opId = op({ label: '改了：' + b.title, domain: 'schedule', payload: { type: 'set_fields', blockId: id, prev } });
    emit(); return { opId };
  }
  function setAssignmentDone(id, done) {
    const a = state.assignments.find((x) => x.id === id); if (!a) return null;
    const prev = a.done; a.done = !!done;
    const opId = op({ label: (done ? '交了：' : '取消提交：') + a.title, domain: 'archive', payload: { type: 'assignment_done', assignmentId: id, prev } });
    emit(); return { opId };
  }
  const unreadPush = () => state.pushes.find((p) => !p.read);

  return { setMark, setBlockFields, setAssignmentDone, bulkBlocks, get state() { return state; }, subscribe, emit, sweep, MOODS, uid, todayIso, addDays, toMin, toHM, fmtHM, fmtDate, relDay, weekdayShort, nowMs, nowMin, dayStartMs, petrifyLine, blockRange, phaseOf, blocksOf, ghostsOf, stack, l0Today, gapsOf, flowNow, ledgerByDay, river, radar, moodToday, anyMoodToday, brief, briefVisible, dismissBrief, addBlock, moveBlock, proposeReschedule, setStatus, reconcile, setNote, removeBlock, accept, reject, redeem, followUp, consumeExchange, wishAdd, wishSet, moodRecord, materialAdd, materialRemove, memoryAdd, memoryRemove, markPushRead, undo, captureIntent, chatSend, scenarioSet, theme, setTheme, boldness, pushBudget, unreadPush, aiTheme, createTheme, patchTheme, deleteTheme, listCategories, setCategory, patch, setPref, bindChannel, verifyChannel, unbindChannel };
})();
