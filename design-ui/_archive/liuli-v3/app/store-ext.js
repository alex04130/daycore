// Daycore v2.2 — store extensions: materials/inbox, threads + async agent, channels,
// preferences, memory facts, ops log, version. Loads AFTER store.js, patches window.DcStore.
(function () {
  'use strict';
  const S = window.DcStore;
  const state = S.state;
  const { uid, todayIso, addDays } = S;
  const T = todayIso();
  const nowIso = () => new Date().toISOString();
  const L = () => (state.session.language || 'zh-CN') === 'zh-CN';

  // ---------- category registry ----------
  const CATEGORIES = [
    { id: 'note',     icon: 'NotebookPen',   dot: 'var(--color-text-muted)',     default: true,  locked: true },
    { id: 'diet',     icon: 'Salad',         dot: 'var(--color-states-warning)', default: true },
    { id: 'health',   icon: 'HeartPulse',    dot: 'var(--color-states-error)',   default: true },
    { id: 'academic', icon: 'GraduationCap', dot: 'var(--color-primary)',        default: true },
    { id: 'travel',   icon: 'Plane',         dot: '#14b8a6',                     default: true },
    { id: 'finance',  icon: 'Wallet',        dot: '#8b5cf6',                     default: false },
    { id: 'fitness',  icon: 'Dumbbell',      dot: '#f97316',                     default: false },
    { id: 'idea',     icon: 'Lightbulb',     dot: '#eab308',                     default: false },
    { id: 'shopping', icon: 'ShoppingBag',   dot: '#ec4899',                     default: false },
    { id: 'media',    icon: 'Film',          dot: '#6366f1',                     default: false },
  ];

  // ---------- one-time migration / seed of v2.2 fields ----------
  function seedV3() {
    state.preferences = {
      morningBrief: true, eveningReview: true, deadlineAlerts: true, rollingReplan: true,
      gapSuggestions: true, doNotDisturb: false, autoPlan: true, materialCategories: {},
    };
    const mat = (cat, title, summary, text, extra, daysAgo, time) => Object.assign({
      id: uid('mt'), category: cat, title, summary, text,
      structured: null, advice: null, source: 'inbox',
      created_at: addDays(T, -daysAgo) + 'T' + (time || '12:00') + ':00+08:00',
    }, extra || {});
    state.materials = [
      mat('diet', '午餐 · 麻辣香锅', '约 850 千卡 · 蛋白质 32g', '中午和室友去吃了麻辣香锅，加了份米饭，有点辣',
        { structured: { items: [{ name: '麻辣香锅', kcal: 780 }, { name: '米饭', kcal: 70 }], total_kcal: 850, protein_g: 32 } }, 0, '12:40'),
      mat('health', '轻微头疼', '下午开始 · 可能没睡够', '下午开始有点头疼，昨晚只睡了五个小时',
        { structured: { symptom: '头疼', since: '下午' }, advice: '先补水，找个安静的地方休息 20 分钟；今晚尽量 23:00 前入睡。如果持续超过 24 小时或明显加重，请及时就医。' }, 1, '16:05'),
      mat('academic', 'ENGL 105 论文选题方向', '教授建议写"数字生活"方向', '写作课找教授聊了选题，她建议往数字生活方向走，参考文献先看 3 篇', null, 2, '15:20'),
      mat('travel', '周末去杭州', '2 天行程草案 · 西湖 + 灵隐', '周末想去杭州放松一下',
        { source: 'travel', structured: { destination: '杭州', start_date: addDays(T, 4), end_date: addDays(T, 5), itinerary: [
          { date: addDays(T, 4), theme: '西湖环线', items: ['上午 · 高铁抵达，酒店寄存行李', '中午 · 楼外楼午餐', '下午 · 苏堤—白堤慢走', '晚上 · 湖滨夜景'] },
          { date: addDays(T, 5), theme: '灵隐与山线', items: ['上午 · 灵隐寺 + 飞来峰', '中午 · 素面', '下午 · 九溪烟树', '傍晚 · 返程高铁'] },
        ], tips: ['周末西湖人多，景点门票提前一天订', '带一件薄外套，山里傍晚偏凉', '高铁票开售即抢，返程留 40 分钟余量'] } }, 2, '21:10'),
      mat('note', '宿舍网费该续了', '月底前记得', '宿舍网费好像月底到期，记得续', null, 3, '09:30'),
    ];
    state.drafts = {};
    state.memory = [
      { id: uid('f'), fact: '晚上 11 点后不排任务', source: 'user', createdAt: addDays(T, -6) + 'T20:00:00+08:00' },
      { id: uid('f'), fact: '午饭后想留 30 分钟午休', source: 'chat', createdAt: addDays(T, -4) + 'T13:00:00+08:00' },
    ];
    state.ops = [];
    state.channels = [
      { id: 'qq', bound: true, externalId: '29****310', boundAt: addDays(T, -5) + 'T18:00:00+08:00' },
      { id: 'telegram', bound: false, externalId: null, boundAt: null },
    ];
    state.importHistory = [
      { id: uid('ih'), source: 'canvas', items: 12, createdAt: T + 'T08:12:00+08:00' },
      { id: uid('ih'), source: 'image', items: 2, createdAt: addDays(T, -3) + 'T19:40:00+08:00' },
    ];
    const th = { id: uid('th'), title: L() ? '日常安排' : 'Daily planning', createdAt: addDays(T, -2) + 'T10:00:00+08:00', updatedAt: addDays(T, -1) + 'T21:00:00+08:00' };
    state.threads = [th];
    state.messages = {};
    state.messages[th.id] = [
      { id: uid('msg'), role: 'user', content: L() ? '这周三前把 Problem Set 8 排进日程' : 'Fit Problem Set 8 in before Wednesday', toolEvents: [], status: 'done', createdAt: addDays(T, -1) + 'T20:58:00+08:00' },
      { id: uid('msg'), role: 'assistant', content: L() ? '排好了：明天下午留了 90 分钟给 Problem Set 8，正好在微积分课后，脑子还热着。' : 'Done — 90 minutes tomorrow afternoon right after Calc, while it is still fresh.',
        toolEvents: [{ type: 'tool_result', callId: 'c1', tool: 'plan_update', ok: true, summary: L() ? '更新了明天的计划' : 'Updated tomorrow’s plan' }], status: 'done', createdAt: addDays(T, -1) + 'T20:59:00+08:00' },
    ];
    state.v3seed = 2;
  }
  if (state.v3seed !== 2) seedV3();
  // crash sweep: pending async turns left over from a previous session → error (mirrors backend)
  Object.values(state.messages || {}).forEach((list) => list.forEach((m) => {
    if (m.status === 'pending') { m.status = 'error'; m.content = window.I18N.t('cp_interrupted'); }
  }));
  // keep auto-plan's keyFacts mirror in sync with memory objects
  function syncFacts() { state.keyFacts = state.memory.map((f) => f.fact); }
  syncFacts();

  // ---------- categories & preferences ----------
  function catEnabled(id) {
    const o = state.preferences.materialCategories || {};
    if (id in o) return !!o[id];
    const c = CATEGORIES.find((x) => x.id === id);
    return c ? c.default : false;
  }
  function listCategories() {
    return CATEGORIES.map((c) => Object.assign({}, c, { enabled: catEnabled(c.id) }));
  }
  function setCategory(id, on) {
    if (id === 'note' && !on) return { error: 'bad_category' };
    state.preferences.materialCategories = Object.assign({}, state.preferences.materialCategories, { [id]: !!on });
    S.emit(); return { ok: true };
  }
  function patchPreferences(changes) { Object.assign(state.preferences, changes); S.emit(); return state.preferences; }

  // ---------- ops log + revert ----------
  function logOp(action, summary, revertData) {
    const op = { id: uid('op'), actor: 'agent', action, summary, status: 'ok', reverted: false, revertData, createdAt: nowIso() };
    state.ops.unshift(op); S.emit(); return op.id;
  }
  function revertOp(id) {
    const op = state.ops.find((o) => o.id === id);
    if (!op || op.reverted) return { error: 'conflict' };
    const rd = op.revertData || {};
    if (rd.kind === 'rule_add') state.rules = state.rules.filter((r) => r.id !== rd.ruleId);
    else if (rd.kind === 'block_add') S.patchPlan(rd.date, { action: 'remove', match: { id: rd.blockId } });
    else if (rd.kind === 'memory_add') { state.memory = state.memory.filter((f) => f.id !== rd.factId); syncFacts(); }
    else if (rd.kind === 'material_add') state.materials = state.materials.filter((m) => m.id !== rd.materialId);
    else if (rd.kind === 'assignment_add') state.assignments = state.assignments.filter((a) => a.id !== rd.assignmentId);
    else if (rd.kind === 'plan_days') rd.dates.forEach((d, i) => { if (rd.prev[i]) state.plans[d] = JSON.parse(rd.prev[i]); else delete state.plans[d]; });
    op.reverted = true; S.emit();
    return { ok: true };
  }

  // ---------- memory ----------
  function addMemoryFact(fact, source) {
    const f = { id: uid('f'), fact, source: source || 'user', createdAt: nowIso() };
    state.memory.unshift(f); syncFacts(); S.emit(); return f;
  }
  function deleteMemoryFact(id) { state.memory = state.memory.filter((f) => f.id !== id); syncFacts(); S.emit(); }
  function clearMemory() { state.memory = []; syncFacts(); S.emit(); }

  // ---------- materials ----------
  function addMaterial(m) {
    const it = Object.assign({ id: uid('mt'), category: 'note', title: '', summary: '', text: '', structured: null, advice: null, source: 'inbox', created_at: nowIso() }, m);
    state.materials.unshift(it); S.emit(); return it;
  }
  function deleteMaterial(id) { state.materials = state.materials.filter((m) => m.id !== id); S.emit(); }
  function searchMaterials(q, category) {
    const needle = (q || '').trim().toLowerCase();
    return state.materials.filter((m) => {
      if (category && category !== 'all' && m.category !== category) return false;
      if (!needle) return true;
      return [m.title, m.summary, m.text].some((s) => (s || '').toLowerCase().includes(needle));
    });
  }

  // ---------- assignments (manual quick-capture route) ----------
  function createAssignment({ title, dueAt, courseId }) {
    const a = { id: uid('a'), courseId: courseId || null, canvasId: null, title, dueAt: dueAt || null, pointsPossible: null, submitted: false, graded: false, score: null, htmlUrl: null, source: 'manual', status: 'pending' };
    state.assignments.unshift(a); S.emit(); return a;
  }

  // ---------- inbox: classify → confirm card → commit ----------
  function parseDue(text) {
    const zhWd = { '一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '日': 0, '天': 0 };
    let date = null, time = '23:59';
    let m = text.match(/(\d{1,2})月(\d{1,2})[日号]/);
    if (m) { const d = new Date(); date = d.getFullYear() + '-' + String(m[1]).padStart(2, '0') + '-' + String(m[2]).padStart(2, '0'); if (date < T) date = null; }
    if (!date) { m = text.match(/周([一二三四五六日天])|星期([一二三四五六日天])/); if (m) { const target = zhWd[m[1] || m[2]]; const cur = S.weekdayOf(T); let diff = (target - cur + 7) % 7; if (diff === 0) diff = 7; date = addDays(T, diff); } }
    if (!date && /后天|day after/i.test(text)) date = addDays(T, 2);
    if (!date && /明天|明晚|tomorrow/i.test(text)) date = addDays(T, 1);
    if (!date && /今晚|今天|tonight|today/i.test(text)) date = T;
    m = text.match(/([0-2]?\d)[:点：](\d{0,2})\s*(前|截止)?/);
    if (m && Number(m[1]) <= 23) { let hh = Number(m[1]); if (hh < 12 && /晚|下午|pm/i.test(text)) hh += 12; time = String(hh).padStart(2, '0') + ':' + (m[2] ? String(m[2]).padStart(2, '0') : '00'); }
    return date ? { date, time } : null;
  }
  const FOODS = [
    [/麻辣香锅/, 780], [/米饭|饭/, 70], [/奶茶/, 320], [/咖啡|拿铁/, 120], [/汉堡/, 550], [/薯条/, 310],
    [/面|拉面|米线/, 480], [/饺子/, 420], [/沙拉|salad/i, 180], [/鸡胸|鸡肉/, 165], [/蛋|egg/i, 78], [/苹果|香蕉|水果/, 95], [/披萨|pizza/i, 285],
  ];
  function classify(text) {
    const en = !L();
    const first = (s, n) => s.length > n ? s.slice(0, n) + '…' : s;
    const on = catEnabled;
    // academic (deadline-ish)
    if (on('academic') && /(作业|截止|due|deadline|要交|考试|quiz|essay|report|论文|实验报告|pre\b|presentation)/i.test(text)) {
      const due = parseDue(text);
      const title = first(text.replace(/[，。,.!！?？]\s*$/, ''), 22);
      return { category: 'academic', confidence: due ? 0.93 : 0.8, title,
        summary: due ? (en ? 'Due ' + due.date + ' ' + due.time : '截止 ' + due.date + ' ' + due.time) : (en ? 'No due date detected' : '未识别到明确截止时间'),
        structured: due ? { due_at: due.date + 'T' + due.time, course: (text.match(/[A-Z]{2,5}\s?\d{3}/) || [null])[0] } : { course: (text.match(/[A-Z]{2,5}\s?\d{3}/) || [null])[0] },
        suggested_action: due ? 'create_assignment' : 'keep_note' };
    }
    // diet
    if (on('diet') && /(吃|喝|早餐|午餐|晚餐|夜宵|外卖|奶茶|咖啡|kcal|卡路里|ate|drank|lunch|dinner|breakfast)/i.test(text)) {
      const items = []; let total = 0;
      FOODS.forEach(([re, kcal]) => { const m = text.match(re); if (m) { items.push({ name: m[0], kcal }); total += kcal; } });
      if (!items.length) { items.push({ name: en ? 'One meal' : '一餐', kcal: 550 }); total = 550; }
      return { category: 'diet', confidence: 0.88, title: first(text, 18),
        summary: en ? '≈ ' + total + ' kcal' : '约 ' + total + ' 千卡',
        structured: { items, total_kcal: total, protein_g: Math.round(total * 0.04) }, suggested_action: 'keep' };
    }
    // health
    if (on('health') && /(头疼|头痛|发烧|咳嗽|嗓子|失眠|睡不着|不舒服|胃疼|肚子|拉肚子|过敏|感冒|例假|痛经|sick|headache|fever|insomnia)/i.test(text)) {
      const symptom = (text.match(/头疼|头痛|发烧|咳嗽|嗓子疼|失眠|睡不着|胃疼|拉肚子|过敏|感冒|痛经|headache|fever|cough|insomnia/i) || [en ? 'discomfort' : '不适'])[0];
      return { category: 'health', confidence: 0.9, title: first(symptom, 14), summary: first(text, 26),
        structured: { symptom, since: (text.match(/今天|昨晚|昨天|早上|下午|晚上|this morning|last night/i) || [null])[0] },
        advice: en
          ? 'Hydrate, rest in a quiet place for 20 minutes, and aim for an early night. If it persists beyond 24 hours or gets worse, please see a doctor.'
          : '先补水，找安静的地方休息 20 分钟，今晚早点睡。如果持续超过 24 小时或明显加重，请及时就医。',
        suggested_action: 'keep' };
    }
    // travel
    if (on('travel') && /(去|飞|高铁|机票|旅行|旅游|出差|周末去|trip|travel|fly to)/i.test(text) && !/回宿舍|去图书馆|去食堂|去上课/.test(text)) {
      const dest = (text.match(/(?:去|飞|to\s+)([\u4e00-\u9fa5A-Za-z]{2,8})/) || [null, en ? 'somewhere' : '某地'])[1];
      return { category: 'travel', confidence: 0.84, title: (en ? 'Trip · ' : '出行 · ') + dest, summary: first(text, 26),
        structured: { destination: dest, start_date: parseDue(text) ? parseDue(text).date : null }, suggested_action: 'plan_trip' };
    }
    if (on('finance') && /(花了|块钱|元|支出|报销|工资|退款|spent|paid|¥|\$)/i.test(text)) {
      const amt = (text.match(/(\d+(?:\.\d+)?)\s*(?:元|块|¥|\$)?/) || [null, null])[1];
      return { category: 'finance', confidence: 0.86, title: first(text, 16), summary: amt ? (en ? 'Amount ¥' + amt : '金额 ¥' + amt) : '', structured: { amount: amt ? Number(amt) : null }, suggested_action: 'keep' };
    }
    if (on('fitness') && /(跑了|健身|锻炼|卧推|深蹲|瑜伽|游泳|骑车|ran|workout|gym)/i.test(text)) {
      const km = (text.match(/(\d+(?:\.\d+)?)\s*(?:公里|km)/i) || [null, null])[1];
      return { category: 'fitness', confidence: 0.87, title: first(text, 16), summary: km ? km + ' km' : '', structured: { distance_km: km ? Number(km) : null, kind: (text.match(/跑|健身|瑜伽|游泳|骑车|run|gym|yoga|swim/i) || [null])[0] }, suggested_action: 'keep' };
    }
    if (on('idea') && /(想到|点子|灵感|idea|一个想法)/i.test(text)) {
      return { category: 'idea', confidence: 0.82, title: first(text.replace(/^(突然)?想到(一个)?/, ''), 18), summary: '', structured: null, suggested_action: 'keep' };
    }
    if (on('shopping') && /(买|下单|购物车|种草|想要.*(?:鞋|包|键盘|耳机)|buy|order)/i.test(text)) {
      return { category: 'shopping', confidence: 0.8, title: first(text, 18), summary: '', structured: null, suggested_action: 'keep' };
    }
    if (on('media') && /(看了|追完|电影|纪录片|专辑|听了|读完|一本书|watched|listened|read)/i.test(text)) {
      return { category: 'media', confidence: 0.8, title: first(text, 18), summary: '', structured: null, suggested_action: 'keep' };
    }
    if (text.trim().length < 5) return null; // low confidence → legacy fallback
    return { category: 'note', confidence: 0.66, title: first(text, 18), summary: '', structured: null, suggested_action: 'keep' };
  }
  function inboxProcess({ text, photo }) {
    const en = !L();
    if (photo) {
      // vision demo: food photo → diet card
      const cls = {
        category: 'diet', confidence: 0.9, title: en ? 'Lunch from photo' : '拍照识别 · 午餐',
        summary: en ? '≈ 640 kcal' : '约 640 千卡',
        structured: { items: [{ name: en ? 'Chicken rice bowl' : '鸡肉饭', kcal: 520 }, { name: en ? 'Soup' : '例汤', kcal: 120 }], total_kcal: 640, protein_g: 34 },
        suggested_action: 'keep', fromPhoto: true,
      };
      const draftId = uid('df');
      state.drafts[draftId] = { classification: cls, text: en ? '(photo)' : '（拍照记录）' };
      S.emit();
      return { understanding: cls.title, suggestions: [], classification: cls, draftId, altTimetable: true };
    }
    const cls = classify(text || '');
    if (!cls) {
      return { understanding: en ? 'Not sure what this is yet' : '还没太看懂这句', suggestions: en ? ['Add a bit more detail', 'Save as a plain note'] : ['再补充一点细节', '直接存成随手笔记'] };
    }
    const draftId = uid('df');
    state.drafts[draftId] = { classification: cls, text: text || '' };
    S.emit();
    return { understanding: cls.title, suggestions: [], classification: cls, draftId };
  }
  function inboxCommit({ draftId, overrides, alsoKeepNote }) {
    const d = state.drafts[draftId];
    if (!d) return { error: 'bad_request' };
    const cls = Object.assign({}, d.classification, overrides || {});
    delete state.drafts[draftId];
    if (cls.category === 'academic' && cls.suggested_action === 'create_assignment' && cls.structured && cls.structured.due_at) {
      const a = createAssignment({ title: cls.title, dueAt: cls.structured.due_at + ':00+08:00' });
      let material = null;
      if (alsoKeepNote) material = addMaterial({ category: 'academic', title: cls.title, summary: cls.summary, text: d.text, structured: cls.structured });
      logOp('assignment_add', (L() ? '添加作业 ' : 'Added assignment ') + cls.title, { kind: 'assignment_add', assignmentId: a.id });
      return { type: 'assignment', assignment: a, material };
    }
    const m = addMaterial({ category: cls.category, title: cls.title, summary: cls.summary, text: d.text, structured: cls.structured || null, advice: cls.advice || null, source: cls.fromPhoto ? 'photo' : 'inbox' });
    return { type: 'material', material: m };
  }

  // ---------- travel itinerary ----------
  function aiTravel({ destination, startDate, endDate, notes }) {
    const en = !L();
    const from = startDate || addDays(T, 7);
    const to = endDate && endDate >= from ? endDate : addDays(from, 1);
    const n = Math.min(4, S.dayDiff(to, from) + 1);
    const themes = en ? ['Arrival & old town', 'Nature day', 'Museums & food', 'Slow morning & return']
      : ['抵达 · 老城区', '自然与山水', '博物馆与本地菜', '慢早晨 · 返程'];
    const itinerary = Array.from({ length: n }, (_, i) => ({
      date: addDays(from, i), theme: themes[i % themes.length],
      items: en
        ? ['Morning · ' + (i === 0 ? 'arrive, drop bags' : 'main sight of the day'), 'Noon · local lunch', 'Afternoon · easy walking route', 'Evening · ' + (i === n - 1 ? 'head back' : 'night view / rest')]
        : ['上午 · ' + (i === 0 ? '抵达，酒店寄存行李' : '当日主景点'), '中午 · 本地午餐', '下午 · 轻松步行路线' + (notes ? '（' + notes.slice(0, 10) + '…考虑在内）' : ''), '晚上 · ' + (i === n - 1 ? '返程' : '夜景 / 休息')],
    }));
    const tips = en
      ? ['Book tickets one day ahead on weekends', 'Pack a light jacket for the evening', 'Keep the last afternoon unscheduled as a buffer']
      : ['周末热门景点提前一天订票', '傍晚偏凉，带件薄外套', '最后一个下午留白当缓冲'];
    const cls = { category: 'travel', confidence: 0.95, title: (en ? 'Trip · ' : '出行 · ') + destination,
      summary: from + ' → ' + to + ' · ' + n + (en ? ' days' : ' 天'),
      structured: { destination, start_date: from, end_date: to, itinerary, tips }, suggested_action: 'keep' };
    const draftId = uid('df');
    state.drafts[draftId] = { classification: cls, text: notes || '' };
    S.emit();
    return { title: cls.title, summary: cls.summary, itinerary: { days: itinerary }, tips, draftId };
  }

  // ---------- channels ----------
  function bindChannel(id) {
    const token = Math.random().toString(36).slice(2, 8).toUpperCase() + Math.random().toString(36).slice(2, 8).toUpperCase();
    state.pendingBind = { channel: id, token: token.slice(0, 12), expiresAt: Date.now() + 600e3 };
    S.emit();
    return { token: state.pendingBind.token, channel: id };
  }
  function verifyChannel(id) {
    const ch = state.channels.find((c) => c.id === id);
    if (ch) { ch.bound = true; ch.externalId = id === 'qq' ? '29****310' : '@yu_chen'; ch.boundAt = nowIso(); }
    state.pendingBind = null; S.emit(); return { ok: true };
  }
  function unbindChannel(id) {
    const ch = state.channels.find((c) => c.id === id);
    if (ch) { ch.bound = false; ch.externalId = null; ch.boundAt = null; S.emit(); }
    return { ok: true };
  }

  // ---------- import history logging (wrap base imports) ----------
  function pushHistory(source, items) {
    state.importHistory.unshift({ id: uid('ih'), source, items, createdAt: nowIso() }); S.emit();
  }
  const _importCanvas = S.importCanvasDemo;
  S.importCanvasDemo = function () { const r = _importCanvas(); pushHistory('canvas', (r.courses || 0) + (r.assignments || 0)); return r; };
  const _saveRulesBatch = S.saveRulesBatch;
  S.saveRulesBatch = function (inputs) { const created = _saveRulesBatch(inputs); const src = inputs[0] && inputs[0].source === 'image' ? 'image' : 'ics'; pushHistory(src, created.length); return created; };

  // ---------- chat threads + async agent ----------
  function listThreads() { return state.threads.slice().sort((a, b) => (b.updatedAt || '').localeCompare(a.updatedAt || '')); }
  function createThread(title) {
    const th = { id: uid('th'), title: title || (L() ? '新会话' : 'New thread'), createdAt: nowIso(), updatedAt: nowIso() };
    state.threads.unshift(th); state.messages[th.id] = []; S.emit(); return th;
  }
  function deleteThread(id) { state.threads = state.threads.filter((x) => x.id !== id); delete state.messages[id]; S.emit(); }
  function threadMessages(id) { return state.messages[id] || []; }

  const pendingDecisions = {}; // cardId → {continue(choiceId)}
  function findMsg(threadId, msgId) { return (state.messages[threadId] || []).find((m) => m.id === msgId); }

  // stream text into a message, then mark done
  function streamInto(threadId, msgId, text, onDone) {
    let i = 0;
    const iv = setInterval(() => {
      const m = findMsg(threadId, msgId); if (!m) { clearInterval(iv); return; }
      i = Math.min(text.length, i + 2 + Math.floor(Math.random() * 3));
      m.content = text.slice(0, i); S.emit();
      if (i >= text.length) {
        clearInterval(iv);
        m.status = 'done';
        const th = state.threads.find((x) => x.id === threadId); if (th) th.updatedAt = nowIso();
        S.emit();
        if (onDone) onDone();
      }
    }, 24);
  }
  function pushEvent(threadId, msgId, ev) { const m = findMsg(threadId, msgId); if (m) { m.toolEvents = [...m.toolEvents, ev]; S.emit(); } }
  function markResult(threadId, msgId, callId, patch) {
    const m = findMsg(threadId, msgId); if (!m) return;
    m.toolEvents = m.toolEvents.map((e) => (e.callId === callId && e.type === 'tool_start' ? Object.assign({ type: 'tool_result', callId, tool: e.tool }, patch) : e));
    S.emit();
  }

  // the scripted agent: intents → tool events + optional decision card + final text
  function agentTurn(threadId, msgId, text) {
    const zh = L();
    const say = (s, cb) => streamInto(threadId, msgId, s, cb);
    const tool = (name, args, resultPatch, delay, cb) => {
      const callId = uid('c');
      pushEvent(threadId, msgId, { type: 'tool_start', callId, tool: name, args: args || {} });
      setTimeout(() => { markResult(threadId, msgId, callId, resultPatch); if (cb) cb(); }, delay || 900);
    };

    // 1) weather
    if (/天气|weather|带伞|下雨|umbrella/i.test(text)) {
      tool('get_weather', { city: 'local' }, { ok: true, summary: zh ? '明天 24–31°C，多云转小雨（傍晚起）' : 'Tomorrow 24–31°C, showers from evening' }, 1100, () => {
        say(zh ? '查了下：明天白天多云，傍晚开始有小雨。你 19:00 的社团例会散场正好赶上雨，我建议带伞，或者要不要我把晨跑挪到室内拉伸？' : 'Checked — cloudy tomorrow, light rain from evening. Your 19:00 club meeting ends right in it; bring an umbrella, or shall I swap the morning run for indoor stretching?');
      });
      return;
    }
    // 2) conflict → decision card
    if (/撞|冲突|都在|怎么办|要不要去/.test(text) || /clash|conflict/i.test(text)) {
      tool('list_upcoming', {}, { ok: true, summary: zh ? '明晚 19:00：社团例会 × Lab 6 都想占用' : '19:00 tomorrow: club meeting × Lab 6 overlap' }, 1000, () => {
        const cardId = uid('dc');
        pushEvent(threadId, msgId, {
          type: 'decision_card', id: cardId, state: 'pending',
          title: zh ? '明晚 19:00 有两件事' : 'Two things at 19:00 tomorrow',
          summary: zh ? '社团例会（1 小时）和 Lab 6：哈希表（想留 90 分钟）撞在一起了。' : 'Club meeting (1h) collides with Lab 6 (needs 90 min).',
          options: [
            { id: 'lab', label: zh ? '优先 Lab 6' : 'Lab 6 first' },
            { id: 'club', label: zh ? '优先社团例会' : 'Club first' },
            { id: 'split', label: zh ? '各留一段' : 'Split the evening' },
          ],
        });
        pendingDecisions[cardId] = {
          threadId, msgId,
          continue(choice) {
            const d1 = addDays(T, 1);
            const isPick = choice === 'lab' || choice === 'club' || choice === 'split';
            if (!isPick) {
              const line = choice === 'timeout'
                ? (zh ? '你一直没选，我先保守处理：两件事都保留提醒，没动任何日程。想好了随时喊我。' : 'No pick — I kept both as reminders and touched nothing. Tell me when you decide.')
                : (zh ? '收到，就按你说的来。我先不动日程，把这句记在备注里，晚点规划时会照顾到。' : 'Got it — I will not touch the plan; I noted your words and will respect them when planning.');
              say(line);
              return;
            }
            const snapshot = [state.plans[d1] ? JSON.stringify(state.plans[d1]) : null];
            const apply = () => {
              if (choice === 'club') S.patchPlan(d1, { action: 'add', block: { title: zh ? '社团例会' : 'Club meeting', time: '19:00', type: 'appointment', duration_min: 60, origin: 'auto' } });
              else if (choice === 'split') { S.patchPlan(d1, { action: 'add', block: { title: 'Lab 6 · Part 1', time: '18:00', type: 'task', duration_min: 50, origin: 'auto' } }); S.patchPlan(d1, { action: 'add', block: { title: zh ? '社团例会（前半场）' : 'Club (first half)', time: '19:00', type: 'appointment', duration_min: 40, origin: 'auto' } }); }
              else S.patchPlan(d1, { action: 'add', block: { title: zh ? 'Lab 6：哈希表（专注）' : 'Lab 6: hash tables (focus)', time: '19:00', type: 'task', duration_min: 90, origin: 'auto' } });
            };
            tool('plan_update', { date: d1 }, (() => { apply(); const opId = logOp('plan_update', zh ? '重排了明晚 19:00' : 'Rearranged tomorrow 19:00', { kind: 'plan_days', dates: [d1], prev: snapshot }); return { ok: true, opId, summary: zh ? '已按你的选择重排明晚' : 'Tomorrow evening rearranged' }; })(), 900, () => {
              const lines = {
                lab: zh ? '好，那明晚 19:00 整块给 Lab 6，我给社团那边留了备注建议你请个假。截止前一天做核心部分最稳妥。' : 'Done — 19:00 goes fully to Lab 6. I’d ping the club that you’ll skip this one.',
                club: zh ? '好，明晚照常去社团例会，Lab 6 挪到了后天上午精神最好的时候，仍然赶得上截止。' : 'Okay — club as planned, Lab 6 moved to the following morning. Still comfortably before the deadline.',
                split: zh ? '安排成了两段：18:00 先冲 50 分钟 Lab 6 核心部分，19:00 去社团露个面，回来如果还有精力再补一段。' : 'Split it: 50 min of Lab 6 at 18:00, then show up at the club at 19:00.',
                timeout: zh ? '你一直没选，我先保守处理：两件事都保留提醒，没动任何日程。想好了随时喊我。' : 'No pick — I kept both as reminders and touched nothing. Tell me when you decide.',
              };
              say(lines[choice] || lines.timeout);
            });
          },
        };
        // decision timeout (async budget, shortened for the prototype)
        setTimeout(() => {
          const m = findMsg(threadId, msgId);
          const ev = m && m.toolEvents.find((e) => e.id === cardId);
          if (ev && ev.state === 'pending') { ev.state = 'timeout'; S.emit(); const pd = pendingDecisions[cardId]; delete pendingDecisions[cardId]; if (pd) pd.continue('timeout'); }
        }, 90e3);
      });
      return;
    }
    // 3) recurring rule
    let m = text.match(/每\s*(\d+)\s*天(?:提醒我)?(.+)|every\s*(\d+)\s*days?\s*(?:remind me to)?(.+)/i);
    if (m) {
      const n = Number(m[1] || m[3]);
      const title = (m[2] || m[4] || '').replace(/[。.!！]$/, '').trim() || (zh ? '提醒' : 'Reminder');
      tool('rule_upsert', { title }, (() => { const r = S.createRule({ title, kind: 'recurring', freq: 'every_n_days', interval: n, type: 'task', source: 'chat', start_date: T }); const opId = logOp('rule_upsert', (zh ? '新增规则 ' : 'New rule ') + title, { kind: 'rule_add', ruleId: r.id }); return { ok: true, opId, summary: (zh ? '已保存规则：' : 'Rule saved: ') + title }; })(), 1000, () => {
        say(zh ? '记下了：「' + title + '」每 ' + n + ' 天一次。以后自主规划都会自动带上它，不想要了随时撤销。' : 'Saved: “' + title + '” every ' + n + ' days. Auto-plan will include it from now on.');
      });
      return;
    }
    m = text.match(/每天\s*(?:([0-2]?\d)[:点：](\d{0,2}))?\s*(?:提醒我)?(.+)/);
    if (m && /每天|daily/i.test(text)) {
      const title = (m[3] || '').replace(/[。.!！]$/, '').trim();
      if (title) {
        const time = m[1] ? String(m[1]).padStart(2, '0') + ':' + (m[2] ? String(m[2]).padStart(2, '0') : '00') : null;
        tool('rule_upsert', { title, time }, (() => { const r = S.createRule({ title, kind: 'recurring', freq: 'daily', interval: 1, time, type: 'task', source: 'chat', start_date: T }); const opId = logOp('rule_upsert', (zh ? '新增规则 ' : 'New rule ') + title, { kind: 'rule_add', ruleId: r.id }); return { ok: true, opId, summary: (zh ? '已保存规则：' : 'Rule saved: ') + title }; })(), 1000, () => {
          say(zh ? '好，「' + title + '」记成每天' + (time ? ' ' + time : '') + '的固定项了，之后会自动出现在你的日程里。' : 'Saved “' + title + '” as a daily item' + (time ? ' at ' + time : '') + '.');
        });
        return;
      }
    }
    // 4) remember
    if (/记住|别忘了|remember/i.test(text)) {
      const fact = text.replace(/^(请|帮我)?(记住|别忘了)[，,：:]?\s*/i, '').replace(/^remember( that)?\s*/i, '').trim();
      tool('memory_add', { fact }, (() => { const f = addMemoryFact(fact, 'chat'); const opId = logOp('memory_add', (zh ? '记住了 ' : 'Remembered ') + fact, { kind: 'memory_add', factId: f.id }); return { ok: true, opId, summary: (zh ? '已记住：' : 'Remembered: ') + fact }; })(), 900, () => {
        say(zh ? '记住了。之后帮你排日程、聊天时我都会考虑这一条。想让我忘掉的话，点动作卡上的撤销，或去设置 → 记忆。' : 'Got it — I’ll keep this in mind when planning. Undo on the card or in Settings → Memory anytime.');
      });
      return;
    }
    // 5) plan tomorrow / this week
    if ((/明天|tomorrow/i.test(text) && /规划|安排|计划|plan/i.test(text)) || (/(这周|本周|this week)/i.test(text) && /(作业|排|规划|schedule|plan)/i.test(text))) {
      const week = /(这周|本周|this week)/i.test(text);
      const from = week ? T : addDays(T, 1);
      const to = week ? addDays(T, 6) : addDays(T, 1);
      const dates = []; for (let d = from; d <= to; d = addDays(d, 1)) dates.push(d);
      const snapshot = dates.map((d) => (state.plans[d] ? JSON.stringify(state.plans[d]) : null));
      tool('list_upcoming', {}, { ok: true, summary: zh ? '3 项临期作业 · 6 条课表规则' : '3 deadlines · 6 schedule rules' }, 800, () => {
        tool('plan_update', { from, to }, (() => { S.autoPlan({ from, to, mode: 'keep_manual' }); const opId = logOp('plan_update', zh ? '重排了 ' + dates.length + ' 天' : 'Replanned ' + dates.length + ' day(s)', { kind: 'plan_days', dates, prev: snapshot }); return { ok: true, opId, summary: zh ? '已更新 ' + dates.length + ' 天的计划' : dates.length + ' day(s) updated' }; })(), 1300, () => {
          say(week
            ? (zh ? '这周排好了：越接近截止的作业越早开始，每天不超过 3 个专注块，晚上刻意留白。Lab 5 明天截止，所以今天下午给了它完整的 90 分钟。去「今日」页看看，不合适随时说。' : 'Week planned: nearer deadlines start earlier, max 3 focus blocks a day. Lab 5 got a full 90 minutes today.')
            : (zh ? '明天排好了：白天以 Problem Set 8 为主（后天截止），晚上留了散步和放松。你手动改过的都没动。' : 'Tomorrow is set: Problem Set 8 in daylight, an easy evening. Your manual edits stay untouched.'));
        });
      });
      return;
    }
    // 6) add block at a time
    m = text.match(/(上午|下午|晚上|中午)?\s*([0-2]?\d)[:点：](\d{0,2})\s*(.+)/);
    if (m && /(加|安排|添加|留|add|schedule|book)/i.test(text)) {
      let h = Number(m[2]); const mm = m[3] ? Number(m[3]) : 0;
      if (/下午|晚上/.test(m[1] || '') && h < 12) h += 12;
      const time = String(h).padStart(2, '0') + ':' + String(mm).padStart(2, '0');
      const title = m[4].replace(/[。.!！]$/, '').trim();
      tool('plan_add', { time, title }, (() => { const p = S.patchPlan(T, { action: 'add', block: { title, time, type: 'task', duration_min: 60, origin: 'manual' } }); const b = p.blocks.find((x) => x.title === title && x.time === time); const opId = logOp('plan_add', (zh ? '添加日程 ' : 'Added ') + title, { kind: 'block_add', date: T, blockId: b ? b.id : '' }); return { ok: true, opId, summary: (zh ? '已添加：' : 'Added: ') + time + ' ' + title }; })(), 900, () => {
        say(zh ? '加好了：今天 ' + time + '「' + title + '」。我看了下前后安排，不冲突。' : 'Added: ' + time + ' today — “' + title + '”. No conflicts around it.');
      });
      return;
    }
    // fallback
    const fall = zh ? [
      '嗯，我在。想让我动手的话直接说，比如「明晚 8 点留给图书馆」或「这周的作业帮我排一排」。',
      '收到。今天的重头是 Lab 5，其他都能挪。要我调整什么吗？',
      '好。顺带一提，Essay 2 还有 6 天截止，建议这两天先列提纲——要排进去吗？',
    ] : [
      'I’m here. Tell me what to change — e.g. “keep 8pm for the library” or “schedule this week’s homework”.',
      'Noted. Lab 5 is today’s heavy piece; everything else can flex.',
      'Okay. Essay 2 is due in 6 days — want me to slot in outlining time?',
    ];
    setTimeout(() => say(fall[Math.floor(Math.random() * fall.length)]), 500);
  }

  function sendAsync(threadId, text) {
    const list = state.messages[threadId]; if (!list) return null;
    list.push({ id: uid('msg'), role: 'user', content: text, toolEvents: [], status: 'done', createdAt: nowIso() });
    const msg = { id: uid('msg'), role: 'assistant', content: '', toolEvents: [], status: 'pending', createdAt: nowIso() };
    list.push(msg);
    const th = state.threads.find((x) => x.id === threadId); if (th) th.updatedAt = nowIso();
    S.emit();
    setTimeout(() => agentTurn(threadId, msg.id, text), 600);
    return msg.id;
  }
  function respondDecision(cardId, choice) {
    const pd = pendingDecisions[cardId]; if (!pd) return { error: 'not_found' };
    delete pendingDecisions[cardId];
    const m = findMsg(pd.threadId, pd.msgId);
    const ev = m && m.toolEvents.find((e) => e.id === cardId);
    if (ev) { ev.state = 'answered'; ev.choice = choice; S.emit(); }
    pd.continue(choice);
    return { ok: true };
  }

  // ---------- version contract ----------
  const versionInfo = { apiVersion: 2, apiMinor: 2, build: '2.2.0-beta', channel: 'beta', minClient: '2.0.0' };

  Object.assign(S, {
    CATEGORIES, listCategories, catEnabled, setCategory, patchPreferences,
    addMaterial, deleteMaterial, searchMaterials,
    inboxProcess, inboxCommit, aiTravel, createAssignment,
    addMemoryFact, deleteMemoryFact, clearMemory,
    logOp, revertOp,
    bindChannel, verifyChannel, unbindChannel,
    listThreads, createThread, deleteThread, threadMessages, sendAsync, respondDecision,
    versionInfo,
  });
  S.emit();
})();
