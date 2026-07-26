// Daycore v2 — mock backend store (simulates openapi.yaml endpoints, persists to localStorage)
window.DcStore = (function () {
  'use strict';
  const LS_KEY = 'daycore-v3-state';
  const SEED_VERSION = 3;

  // ---------- date helpers ----------
  const pad = (n) => String(n).padStart(2, '0');
  const iso = (d) => d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate());
  const todayIso = () => iso(new Date());
  const addDays = (dateStr, n) => {
    const [y, m, d] = dateStr.split('-').map(Number);
    const dt = new Date(y, m - 1, d + n);
    return iso(dt);
  };
  const dayDiff = (a, b) => Math.round((new Date(a) - new Date(b)) / 864e5);
  const weekdayOf = (dateStr) => new Date(dateStr + 'T12:00:00').getDay(); // 0=Sun
  const toMin = (t) => { const [h, m] = t.split(':').map(Number); return h * 60 + m; };
  const toHM = (min) => pad(Math.floor(min / 60)) + ':' + pad(min % 60);
  let idc = 1000;
  const uid = (p) => (p || 'id') + '_' + (++idc) + '_' + Math.random().toString(36).slice(2, 6);

  // ---------- seed data ----------
  function seed() {
    const T = todayIso();
    const TZ = 'Asia/Shanghai';
    const wd = weekdayOf(T);
    // Ensure some classes land today: build weekly rules around today's weekday
    const rules = [
      { id: uid('r'), title: 'CS 201 数据结构（课）', type: 'appointment', time: '10:10', duration_min: 95, timezone: TZ, time_mode: 'fixed', kind: 'recurring', freq: 'weekly', interval: 1, by_weekday: [(wd + 7) % 7, (wd + 2) % 7], start_date: addDays(T, -30), until: addDays(T, 80), active: true, source: 'ics', note: null },
      { id: uid('r'), title: 'MATH 172 微积分 II（课）', type: 'appointment', time: '14:00', duration_min: 95, timezone: TZ, time_mode: 'fixed', kind: 'recurring', freq: 'weekly', interval: 1, by_weekday: [(wd + 1) % 7, (wd + 3) % 7], start_date: addDays(T, -30), until: addDays(T, 80), active: true, source: 'ics', note: null },
      { id: uid('r'), title: 'PSYC 110 心理学导论（课）', type: 'appointment', time: '08:30', duration_min: 50, timezone: TZ, time_mode: 'fixed', kind: 'recurring', freq: 'weekly', interval: 1, by_weekday: [(wd + 2) % 7, (wd + 4) % 7], start_date: addDays(T, -30), until: addDays(T, 80), active: true, source: 'image', note: null },
      { id: uid('r'), title: '晨跑 25 分钟', type: 'relax', time: '07:30', duration_min: 25, timezone: TZ, time_mode: 'floating', kind: 'recurring', freq: 'daily', interval: 1, by_weekday: [], start_date: addDays(T, -10), until: null, active: true, source: 'user', note: null },
      { id: uid('r'), title: '给绿萝浇水', type: 'task', time: null, duration_min: 5, timezone: TZ, time_mode: 'floating', kind: 'recurring', freq: 'every_n_days', interval: 3, by_weekday: [], start_date: addDays(T, -3), until: null, active: true, source: 'chat', note: '陪伴聊天里创建' },
      { id: uid('r'), title: '社团例会', type: 'appointment', time: '19:00', duration_min: 60, timezone: TZ, time_mode: 'fixed', kind: 'recurring', freq: 'weekly', interval: 1, by_weekday: [(wd + 5) % 7], start_date: addDays(T, -14), until: null, active: false, source: 'user', note: '暑期暂停' },
    ];
    const courses = [
      { id: 'c1', canvasId: '90811', name: '数据结构与算法', courseCode: 'CS 201', currentScore: 88.5, currentGrade: 'A-' },
      { id: 'c2', canvasId: '90514', name: '微积分 II', courseCode: 'MATH 172', currentScore: 91.2, currentGrade: 'A' },
      { id: 'c3', canvasId: '91203', name: '心理学导论', courseCode: 'PSYC 110', currentScore: 82.4, currentGrade: 'B' },
      { id: 'c4', canvasId: '90992', name: '学术英语写作', courseCode: 'ENGL 105', currentScore: 76.8, currentGrade: 'B-' },
    ];
    const asg = (courseId, title, dueDays, dueTime, pts, extra) => Object.assign({
      id: uid('a'), courseId, canvasId: uid('cv'), title,
      dueAt: dueDays === null ? null : addDays(T, dueDays) + 'T' + (dueTime || '23:59') + ':00+08:00',
      pointsPossible: pts, submitted: false, graded: false, score: null,
      htmlUrl: 'https://canvas.school.edu/courses/x/assignments/y',
      source: 'canvas', status: 'pending',
    }, extra || {});
    const assignments = [
      asg('c1', 'Lab 5：AVL 树实现', 1, '23:59', 100),
      asg('c1', 'Quiz 6：图的遍历', 4, '10:00', 20),
      asg('c2', 'Problem Set 8（级数）', 2, '23:59', 50),
      asg('c3', '阅读反思：记忆与遗忘', 3, '23:59', 30),
      asg('c3', '小组课题选题表', 8, '23:59', 10),
      asg('c4', 'Essay 2 终稿', 6, '23:59', 100),
      asg('c4', 'Essay 2 同伴互评', -1, '23:59', 15, { submitted: true, status: 'done' }),
      asg('c2', 'Problem Set 7（积分技巧）', -3, '23:59', 50, { submitted: true, graded: true, score: 46, status: 'done' }),
    ];
    const moods = [
      { id: uid('m'), mood: '😌 平静', aiResponse: '平静是很珍贵的状态，适合做一点点重要的事。', exerciseOffered: null, exerciseCompleted: false, theme: 'sky', createdAt: addDays(T, -1) + 'T21:30:00+08:00' },
      { id: uid('m'), mood: '😣 压力大', aiResponse: 'Lab 快截止的时候大家都会这样。要不要拆成两块，先做最小可运行版本？', exerciseOffered: 'breathing', exerciseCompleted: true, theme: 'sky', createdAt: addDays(T, -2) + 'T16:05:00+08:00' },
    ];
    return {
      seedVersion: SEED_VERSION,
      session: { id: 'sess_demo', userId: null, assistantName: '小禾', currentTheme: 'sky', language: (function(){ try { return localStorage.getItem('daycore-lang') || 'zh-CN'; } catch(e) { return 'zh-CN'; } })() },
      user: null,
      plans: {},            // date -> {id,date,blocks[],sourceType,note}
      rules,
      courses, assignments,
      customThemes: [],
      moods,
      chat: [],             // {role, content, timestamp}
      keyFacts: ['晚上 11 点后不排任务', '午饭后想留 30 分钟午休'],
      importToken: '',
      canvasImportedAt: addDays(T, 0) + 'T08:12:00+08:00',
      icsImportedAt: null,
      onboarded: false,
    };
  }

  // ---------- state + persistence ----------
  let state;
  try {
    const raw = localStorage.getItem(LS_KEY);
    state = raw ? JSON.parse(raw) : null;
    if (!state || state.seedVersion !== SEED_VERSION) state = seed();
  } catch (e) { state = seed(); }

  let saveTimer = null;
  function persist() {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => {
      try { localStorage.setItem(LS_KEY, JSON.stringify(state)); } catch (e) {}
    }, 150);
  }
  const subs = new Set();
  function emit() { persist(); subs.forEach((fn) => fn(state)); }

  // ---------- rule occurrences (mirrors backend merge) ----------
  function ruleOccursOn(rule, dateStr) {
    if (!rule.active) return false;
    if (rule.kind === 'once') return rule.date === dateStr;
    const start = rule.start_date || dateStr;
    if (dateStr < start) return false;
    if (rule.until && dateStr > rule.until) return false;
    const diff = dayDiff(dateStr, start);
    switch (rule.freq) {
      case 'daily': return true;
      case 'every_n_days': return diff % Math.max(1, rule.interval || 1) === 0;
      case 'weekly': {
        const days = (rule.by_weekday && rule.by_weekday.length) ? rule.by_weekday : [weekdayOf(start)];
        return days.includes(weekdayOf(dateStr));
      }
      case 'monthly': {
        const dom = Number(start.split('-')[2]);
        return Number(dateStr.split('-')[2]) === dom;
      }
      default: return false;
    }
  }
  function ruleBlock(rule, dateStr) {
    return {
      id: 'ro_' + rule.id + '_' + dateStr, date: dateStr, time: rule.time || null,
      title: rule.title, type: rule.type, duration_min: rule.duration_min || null,
      time_mode: rule.time_mode, timezone: rule.timezone || 'Asia/Shanghai',
      completed: false, isAchievement: false, rule_id: rule.id, origin: 'rule', hidden: false,
    };
  }
  // merged plan for a date (hidden filtered). Returns null when nothing.
  function getPlan(dateStr) {
    const stored = state.plans[dateStr];
    const blocks = stored ? stored.blocks.map((b) => ({ ...b })) : [];
    const have = new Set(blocks.filter((b) => b.rule_id).map((b) => b.rule_id));
    state.rules.forEach((r) => {
      if (!have.has(r.id) && ruleOccursOn(r, dateStr)) blocks.push(ruleBlock(r, dateStr));
    });
    const visible = blocks.filter((b) => !b.hidden);
    if (!visible.length && !stored) return null;
    sortBlocks(visible);
    return {
      id: stored ? stored.id : '', date: dateStr, blocks: visible,
      sourceType: stored ? stored.sourceType : 'rules', note: stored ? stored.note : null,
    };
  }
  function sortBlocks(blocks) {
    blocks.sort((a, b) => {
      if (a.time && b.time) return toMin(a.time) - toMin(b.time);
      if (a.time) return -1; if (b.time) return 1; return 0;
    });
  }
  // materialize rule occurrences into stored plan (backend does this before PATCH)
  function materialize(dateStr) {
    let p = state.plans[dateStr];
    if (!p) p = state.plans[dateStr] = { id: uid('plan'), date: dateStr, blocks: [], sourceType: 'manual', note: null };
    const have = new Set(p.blocks.filter((b) => b.rule_id).map((b) => b.rule_id));
    state.rules.forEach((r) => { if (!have.has(r.id) && ruleOccursOn(r, dateStr)) p.blocks.push(ruleBlock(r, dateStr)); });
    return p;
  }
  function patchPlan(dateStr, action) {
    const p = materialize(dateStr);
    if (action.action === 'add') {
      const b = Object.assign({ id: uid('b'), date: dateStr, completed: false, origin: 'manual', time_mode: 'floating', timezone: 'Asia/Shanghai', hidden: false }, action.block);
      p.blocks.push(b);
    } else if (action.action === 'update') {
      const b = p.blocks.find((x) => x.id === (action.match && action.match.id) || (action.match && action.match.title && x.title === action.match.title));
      if (b) {
        Object.assign(b, action.changes);
        if (b.origin === 'auto') b.origin = 'manual'; // edited auto blocks survive re-plan
      }
    } else if (action.action === 'remove') {
      const idx = p.blocks.findIndex((x) => x.id === (action.match && action.match.id));
      if (idx >= 0) {
        const b = p.blocks[idx];
        if (b.rule_id) b.hidden = true; // tombstone
        else p.blocks.splice(idx, 1);
      }
    }
    emit();
    return getPlan(dateStr);
  }

  // ---------- auto-plan simulation ----------
  const WORK_TITLES = {
    'zh-CN': { work: (t) => t, review: '预习 + 整理笔记', lunch: '午餐', dinner: '晚餐', lunchBreak: '午休', walk: '傍晚散步 15 分钟', wind: '睡前放松 · 收拾明天', breakfast: '早餐' },
    'en-US': { work: (t) => t, review: 'Preview + tidy notes', lunch: 'Lunch', dinner: 'Dinner', lunchBreak: 'Nap', walk: '15-min evening walk', wind: 'Wind down · prep tomorrow', breakfast: 'Breakfast' },
  };
  function genDayBlocks(dateStr, keepBlocks, instructions) {
    const lang = state.session.language || 'zh-CN';
    const W = WORK_TITLES[lang] || WORK_TITLES['zh-CN'];
    const busy = []; // [startMin, endMin]
    keepBlocks.forEach((b) => { if (b.time && !b.hidden) busy.push([toMin(b.time), toMin(b.time) + (b.duration_min || 45)]); });
    const overlaps = (s, e) => busy.some(([bs, be]) => s < be && e > bs);
    const claim = (s, e) => { busy.push([s, e]); busy.sort((a, b) => a[0] - b[0]); };
    function place(startPref, dur) { // find earliest slot >= startPref
      for (let s = startPref; s + dur <= 23 * 60; s += 15) if (!overlaps(s, s + dur)) { claim(s, s + dur); return s; }
      return null;
    }
    const out = [];
    const mk = (time, title, type, dur, extra) => out.push(Object.assign({
      id: uid('b'), date: dateStr, time: time === null ? null : toHM(time), title, type,
      duration_min: dur || null, time_mode: 'floating', timezone: 'Asia/Shanghai',
      completed: false, isAchievement: false, origin: 'auto', hidden: false,
    }, extra || {}));
    // meals
    const bf = place(8 * 60, 20); if (bf !== null) mk(bf, W.breakfast, 'meal', 20);
    const lu = place(12 * 60, 40); if (lu !== null) mk(lu, W.lunch, 'meal', 40);
    if (/午休|nap/i.test(instructions || '') || state.keyFacts.some((f) => /午休|nap/i.test(f))) {
      const np = place(13 * 60, 30); if (np !== null) mk(np, W.lunchBreak, 'break', 30);
    }
    const di = place(18 * 60, 45); if (di !== null) mk(di, W.dinner, 'meal', 45);
    // assignments due soon → work blocks (nearest due first, ~2 per day)
    const pending = state.assignments
      .filter((a) => a.status === 'pending' && a.dueAt && a.dueAt.slice(0, 10) >= dateStr && dayDiff(a.dueAt.slice(0, 10), dateStr) <= 14)
      .sort((a, b) => a.dueAt.localeCompare(b.dueAt));
    let placed = 0;
    for (const a of pending) {
      if (placed >= 3) break;
      const dueIn = dayDiff(a.dueAt.slice(0, 10), dateStr);
      const dur = dueIn <= 1 ? 90 : 60;
      const s = place(placed === 0 ? 9 * 60 + 30 : 15 * 60 + 30, dur);
      if (s === null) break;
      const course = state.courses.find((c) => c.id === a.courseId);
      const label = (course ? course.courseCode + ' · ' : '') + a.title;
      mk(s, label, 'task', dur, { note_assignment: a.id });
      placed++;
    }
    if (!placed) { const s = place(9 * 60 + 30, 60); if (s !== null) mk(s, W.review, 'task', 60); }
    // evening care
    const wk = place(20 * 60 + 30, 15); if (wk !== null) mk(wk, W.walk, 'relax', 15);
    const wd2 = place(22 * 60, 30); if (wd2 !== null) mk(wd2, W.wind, 'relax', 30);
    return out;
  }
  function hasMaterials() {
    return state.assignments.length > 0 || state.rules.some((r) => r.active) || state.courses.length > 0;
  }
  // returns {plans, note} or {error}
  function autoPlan({ from, to, instructions, mode }) {
    if (!hasMaterials()) return { error: 'no_material' };
    const f = from || todayIso();
    const t = to || f;
    const n = dayDiff(t, f) + 1;
    if (n > 7) return { error: 'range_too_large' };
    const plans = [];
    const lang = state.session.language || 'zh-CN';
    for (let i = 0; i < n; i++) {
      const d = addDays(f, i);
      const p = materialize(d);
      let kept;
      if (mode === 'replace_all') kept = p.blocks.filter((b) => b.origin === 'rule');
      else kept = p.blocks.filter((b) => b.origin !== 'auto' || b.completed);
      const fresh = genDayBlocks(d, kept, instructions || '');
      p.blocks = kept.concat(fresh);
      p.sourceType = 'auto';
      p.note = lang === 'zh-CN'
        ? '课后给 Lab 5 留了完整的 90 分钟——它明天就截止了。晚上留白，别把自己排太满。'
        : 'I kept a full 90 minutes for Lab 5 after class — it is due tomorrow. The evening stays light on purpose.';
      plans.push(getPlan(d));
    }
    // planned status for scheduled assignments
    state.assignments.forEach((a) => { if (a.status === 'pending' && a.dueAt && dayDiff(a.dueAt.slice(0, 10), f) <= 14 && a.dueAt.slice(0, 10) >= f) a.status = 'planned'; });
    emit();
    return { plans, note: plans[0] && plans[0].note };
  }

  // ---------- plan-text (manual add) simulation ----------
  function planFromText(text, targetDate) {
    const lang = state.session.language || 'zh-CN';
    const lines = text.split(/[\n;；。]/).map((s) => s.trim()).filter(Boolean);
    if (!lines.length) return { error: 'no_schedule_info' };
    const blocks = [];
    const timeRe = /(上午|下午|晚上|中午|早上|morning|afternoon|evening|noon)?\s*(\d{1,2})[:点：]?(\d{0,2})/;
    lines.forEach((ln) => {
      let time = null;
      const m = ln.match(timeRe);
      if (m && m[2]) {
        let h = Number(m[2]); const mm = m[3] ? Number(m[3]) : 0;
        const seg = m[1] || '';
        if ((/下午|晚上|afternoon|evening/.test(seg) && h < 12) || (!seg && h <= 7)) h += 12;
        if (/中午|noon/.test(seg) && h < 11) h += 12;
        if (h < 24) time = toHM(h * 60 + mm);
      }
      let type = 'task';
      if (/健身|跑|锻炼|gym|run|walk|瑜伽/i.test(ln)) type = 'relax';
      if (/饭|餐|午饭|晚饭|lunch|dinner|breakfast/i.test(ln)) type = 'meal';
      if (/会|meeting|面谈|约|appointment|医生/i.test(ln)) type = 'appointment';
      if (/休息|break|午休|nap/i.test(ln)) type = 'break';
      const title = ln.replace(timeRe, '').replace(/^[，,\s]+|[，,\s]+$/g, '') || ln;
      blocks.push({ id: uid('b'), date: targetDate, time, title, type, duration_min: /两小时|2 ?小时|two hours/i.test(ln) ? 120 : (/半小时|30 ?分/i.test(ln) ? 30 : 60), time_mode: 'floating', timezone: 'Asia/Shanghai', completed: false, isAchievement: false, origin: 'manual', hidden: false });
    });
    if (!blocks.length) return { error: 'no_schedule_info' };
    return { note: lang === 'zh-CN' ? '帮你整理成了 ' + blocks.length + ' 条日程。' : 'Parsed ' + blocks.length + ' blocks for you.', blocks };
  }

  // ---------- rules CRUD ----------
  function createRule(input) {
    const r = Object.assign({
      id: uid('r'), title: '', type: 'task', time: null, duration_min: null,
      timezone: 'Asia/Shanghai', time_mode: 'floating', kind: 'recurring',
      date: null, freq: 'daily', interval: 1, by_weekday: [], start_date: todayIso(),
      until: null, active: true, source: 'user', note: null,
    }, input);
    state.rules.unshift(r); emit(); return r;
  }
  function patchRule(id, changes) {
    const r = state.rules.find((x) => x.id === id);
    if (!r) return null;
    Object.assign(r, changes); emit(); return r;
  }
  function deleteRule(id) {
    state.rules = state.rules.filter((x) => x.id !== id);
    Object.values(state.plans).forEach((p) => { p.blocks = p.blocks.filter((b) => !(b.rule_id === id && !b.completed && b.origin === 'rule' && b.date >= todayIso() && !b.hidden)); });
    emit();
  }

  // ---------- imports ----------
  function importCanvasDemo() {
    state.canvasImportedAt = new Date().toISOString();
    // refresh one grade + add one new assignment to feel alive
    const c = state.courses.find((x) => x.courseCode === 'CS 201');
    if (c) { c.currentScore = 89.1; c.currentGrade = 'A-'; }
    if (!state.assignments.some((a) => a.title === 'Lab 6：哈希表')) {
      state.assignments.push({ id: uid('a'), courseId: 'c1', canvasId: uid('cv'), title: 'Lab 6：哈希表', dueAt: addDays(todayIso(), 9) + 'T23:59:00+08:00', pointsPossible: 100, submitted: false, graded: false, score: null, htmlUrl: '#', source: 'canvas', status: 'pending' });
    }
    emit();
    return { ok: true, courses: state.courses.length, assignments: state.assignments.length, warnings: ['import_warning_1'] };
  }
  function icsCandidatesDemo() {
    const T = todayIso(); const wd = weekdayOf(T);
    return [
      { title: 'CHEM 101 普通化学（课）', type: 'appointment', time: '11:10', duration_min: 50, time_mode: 'fixed', kind: 'recurring', freq: 'weekly', interval: 1, by_weekday: [(wd + 1) % 7, (wd + 3) % 7, (wd + 5) % 7], start_date: T, until: addDays(T, 90), source: 'ics' },
      { title: 'CHEM 101 实验', type: 'appointment', time: '15:30', duration_min: 170, time_mode: 'fixed', kind: 'recurring', freq: 'weekly', interval: 1, by_weekday: [(wd + 4) % 7], start_date: T, until: addDays(T, 90), source: 'ics' },
      { title: '导师答疑 Office Hours', type: 'appointment', time: '16:00', duration_min: 60, time_mode: 'fixed', kind: 'recurring', freq: 'weekly', interval: 1, by_weekday: [(wd + 2) % 7], start_date: T, until: null, source: 'ics' },
    ];
  }
  function shotCandidatesDemo() {
    const T = todayIso(); const wd = weekdayOf(T);
    return [
      { title: '体育（羽毛球）', type: 'appointment', time: '16:10', duration_min: 90, time_mode: 'fixed', kind: 'recurring', freq: 'weekly', interval: 1, by_weekday: [(wd + 1) % 7], start_date: T, until: addDays(T, 90), source: 'image' },
      { title: '形势与政策（网课）', type: 'task', time: null, duration_min: 45, time_mode: 'floating', kind: 'recurring', freq: 'weekly', interval: 1, by_weekday: [(wd + 6) % 7], start_date: T, until: addDays(T, 60), source: 'image' },
    ];
  }
  function saveRulesBatch(inputs) {
    const created = inputs.map((r) => createRule(r));
    state.icsImportedAt = new Date().toISOString();
    emit();
    return created;
  }

  // ---------- assignments ----------
  function patchAssignment(id, changes) {
    const a = state.assignments.find((x) => x.id === id);
    if (a) { Object.assign(a, changes); emit(); }
    return a;
  }

  // ---------- mood ----------
  function addMood(entry) {
    const m = Object.assign({ id: uid('m'), exerciseCompleted: false, createdAt: new Date().toISOString(), theme: state.session.currentTheme }, entry);
    state.moods.unshift(m); emit(); return m;
  }
  function completeMoodExercise(id) {
    const m = state.moods.find((x) => x.id === id);
    if (m) { m.exerciseCompleted = true; emit(); }
  }

  // ---------- themes ----------
  const THEME_PRESETS = [
    { match: /海|ocean|sea|深夜|midnight|night/i, name_zh: '深夜的海', name_en: 'Midnight Sea', dark: true, base: 'night', variables: { '--primary': '#4fd1c5', '--accent': '#2c7a7b', '--bg-start': '#0a1628', '--bg-end': '#123047', '--text-primary': '#e6fffa', '--text-secondary': 'rgba(230,255,250,0.72)', '--text-muted': 'rgba(230,255,250,0.45)', '--surface': 'rgba(79,209,197,0.08)', '--surface-hover': 'rgba(79,209,197,0.16)', '--success': '#68d391', '--warning': '#f6ad55', '--error': '#fc8181' } },
    { match: /奶茶|milk|tea|beige|温柔|奶油|cream/i, name_zh: '奶茶时光', name_en: 'Milk Tea', dark: false, base: 'sunset', variables: { '--primary': '#b08968', '--accent': '#9c6644', '--bg-start': '#fdf6ec', '--bg-end': '#f3e5d0', '--text-primary': '#4a3728', '--text-secondary': 'rgba(74,55,40,0.72)', '--text-muted': 'rgba(74,55,40,0.45)', '--surface': 'rgba(176,137,104,0.10)', '--surface-hover': 'rgba(176,137,104,0.18)', '--success': '#7f9d6c', '--warning': '#d9915b', '--error': '#c96f5e' } },
    { match: /森|forest|苔|moss|绿|green/i, name_zh: '雨后苔原', name_en: 'Mossy Rain', dark: false, base: 'nature', variables: { '--primary': '#4a7c59', '--accent': '#3a5f48', '--bg-start': '#eef5ee', '--bg-end': '#d9e8d5', '--text-primary': '#22331f', '--text-secondary': 'rgba(34,51,31,0.72)', '--text-muted': 'rgba(34,51,31,0.45)', '--surface': 'rgba(74,124,89,0.10)', '--surface-hover': 'rgba(74,124,89,0.18)', '--success': '#5a9367', '--warning': '#d19a3f', '--error': '#c4574d' } },
    { match: /紫|薰衣草|lavender|purple|梦/i, name_zh: '薰衣草梦', name_en: 'Lavender Dream', dark: false, base: 'sky', variables: { '--primary': '#8b7ec8', '--accent': '#6c5fa8', '--bg-start': '#f5f2fd', '--bg-end': '#e4ddf6', '--text-primary': '#332d4d', '--text-secondary': 'rgba(51,45,77,0.72)', '--text-muted': 'rgba(51,45,77,0.45)', '--surface': 'rgba(139,126,200,0.10)', '--surface-hover': 'rgba(139,126,200,0.18)', '--success': '#6faf7e', '--warning': '#d9a23f', '--error': '#cd6a60' } },
  ];
  function aiTheme(description, base, themeId) {
    if (!description || description.trim().length < 2 || /^(哈哈|hello|hi|你好)$/i.test(description.trim())) return { error: 'not_a_theme' };
    const hit = THEME_PRESETS.find((p) => p.match.test(description));
    const lang = state.session.language || 'zh-CN';
    if (hit) return { name: lang === 'zh-CN' ? hit.name_zh : hit.name_en, dark: hit.dark, base: hit.base, variables: { ...hit.variables }, warnings: [] };
    // derive a palette deterministically from the text
    let h = 0; for (let i = 0; i < description.length; i++) h = (h * 31 + description.charCodeAt(i)) % 360;
    const dark = /深|暗|dark|夜/i.test(description);
    const P = (l, c) => 'oklch(' + l + ' ' + c + ' ' + h + ')';
    return {
      name: description.slice(0, 8), dark, base: base || (dark ? 'night' : 'sky'),
      variables: dark ? {
        '--primary': P(0.75, 0.12), '--accent': P(0.6, 0.1), '--bg-start': P(0.2, 0.04), '--bg-end': P(0.28, 0.05),
        '--text-primary': P(0.96, 0.01), '--text-secondary': 'rgba(255,255,255,0.72)', '--text-muted': 'rgba(255,255,255,0.45)',
        '--surface': 'rgba(255,255,255,0.07)', '--surface-hover': 'rgba(255,255,255,0.13)',
        '--success': '#68d391', '--warning': '#f6ad55', '--error': '#fc8181',
      } : {
        '--primary': P(0.55, 0.13), '--accent': P(0.45, 0.11), '--bg-start': P(0.97, 0.01), '--bg-end': P(0.92, 0.03),
        '--text-primary': P(0.25, 0.04), '--text-secondary': 'rgba(40,40,50,0.72)', '--text-muted': 'rgba(40,40,50,0.45)',
        '--surface': P(0.55, 0.13).replace(')', ' / 0.10)'), '--surface-hover': P(0.55, 0.13).replace(')', ' / 0.18)'),
        '--success': '#48a56a', '--warning': '#d9a23f', '--error': '#cd6a60',
      },
      warnings: [],
    };
  }
  function createTheme(input) {
    const t = Object.assign({ id: uid('th'), createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() }, input);
    state.customThemes.push(t); emit(); return t;
  }
  function patchTheme(id, changes) {
    const t = state.customThemes.find((x) => x.id === id);
    if (t) { Object.assign(t, changes, { updatedAt: new Date().toISOString() }); emit(); }
    return t;
  }
  function deleteTheme(id) {
    state.customThemes = state.customThemes.filter((x) => x.id !== id);
    let reset = false;
    if (state.session.currentTheme === id) { state.session.currentTheme = 'sky'; reset = true; }
    emit();
    return { ok: true, currentThemeReset: reset };
  }

  // ---------- session / auth ----------
  function patchSession(changes) { Object.assign(state.session, changes); emit(); return state.session; }
  function register({ email, name }) {
    state.user = { id: uid('u'), email, name: name || email.split('@')[0], avatarUrl: null };
    state.session.userId = state.user.id; emit(); return state.user;
  }
  function login({ email }) {
    state.user = { id: uid('u'), email, name: email.split('@')[0], avatarUrl: null };
    state.session.userId = state.user.id; emit(); return state.user;
  }
  function oauth(provider) {
    state.user = { id: uid('u'), email: provider + '.user@example.com', name: provider === 'google' ? 'Yu Chen' : 'yuchen-dev', avatarUrl: null };
    state.session.userId = state.user.id; emit(); return state.user;
  }
  function logout() { state.user = null; state.session.userId = null; emit(); }

  function genToken() { state.importToken = 'dcimp_' + Math.random().toString(36).slice(2, 10) + Math.random().toString(36).slice(2, 10); emit(); return state.importToken; }

  return {
    get state() { return state; },
    subscribe(fn) { subs.add(fn); return () => subs.delete(fn); },
    emit,
    todayIso, addDays, dayDiff, weekdayOf, toMin, toHM, uid,
    getPlan, patchPlan, autoPlan, planFromText, hasMaterials,
    ruleOccursOn,
    createRule, patchRule, deleteRule, saveRulesBatch,
    importCanvasDemo, icsCandidatesDemo, shotCandidatesDemo,
    patchAssignment,
    addMood, completeMoodExercise,
    aiTheme, createTheme, patchTheme, deleteTheme,
    patchSession, register, login, oauth, logout, genToken,
    setOnboarded(v) { state.onboarded = v; emit(); },
  };
})();
