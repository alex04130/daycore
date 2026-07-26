// 琉璃初版 · 共享账本桥接
// 载入顺序：store.js → store-ext.js → 本文件。
// 作用：把「日程 / 作业 / 心情 / 主题」四个域的真相source 换成 core/daycore-core.js，
// 使初版与琉璃·纸屿·汀共用同一份账本（含场景切换、撤销、锁定块）。
// 规则(rules) 核心没有对应实体 —— 降级为只读来源标注，不再自行生成块。
(function () {
  'use strict';
  const S = window.DcStore, C = window.DcCore;
  if (!S || !C) return;
  const st = S.state;
  const SPAN = 21;

  const isoOf = (ms) => { const d = new Date(ms); const p = (n) => String(n).padStart(2, '0'); return d.getFullYear() + '-' + p(d.getMonth() + 1) + '-' + p(d.getDate()); };

  // 规则定义：核心无 rule 实体，故在桥接层自存一份（只存定义，块仍在核心）
  const RLS = 'daycore-liuli0-rules';
  let defs = [];
  try { defs = JSON.parse(localStorage.getItem(RLS) || '[]') || []; } catch (e) { defs = []; }
  const saveDefs = () => { try { localStorage.setItem(RLS, JSON.stringify(defs)); } catch (e) {} };
  const rid = (title) => 'rd_' + encodeURIComponent(title).replace(/%/g, '');

  function toOld(b) {
    return {
      id: b.id, date: b.date, time: b.start, title: b.title, type: b.type,
      duration_min: b.dur, time_mode: b.lockLevel === 'hard' ? 'fixed' : 'floating',
      timezone: 'Asia/Shanghai', completed: b.status === 'done',
      isAchievement: false, origin: b.origin === 'auto' ? 'auto' : 'manual',
      hidden: false, rule_id: null,
      locked: b.lockLevel || null, lockReason: b.lockReason || null,
      missed: b.status === 'missed',
    };
  }

  function syncPlans() {
    const T = C.todayIso();
    st.plans = {};
    for (let i = -SPAN; i <= SPAN; i++) {
      const d = C.addDays(T, i);
      const bs = C.blocksOf(d);
      if (!bs.length) continue;
      st.plans[d] = { id: 'plan_' + d, date: d, blocks: bs.map(toOld), sourceType: 'auto', note: null };
    }
  }

  function syncMaterials() {
    // 作业 + 课程（课程从作业的 course 字段归纳）
    const codes = [];
    st.assignments = (C.state.assignments || []).map((a) => {
      if (a.course && codes.indexOf(a.course) < 0) codes.push(a.course);
      return {
        id: a.id, courseId: 'c_' + (a.course || 'x'), canvasId: a.id, title: a.title,
        dueAt: a.dueAt && a.dueAt.length === 16 ? a.dueAt + ':00+08:00' : a.dueAt,
        pointsPossible: 100, submitted: !!a.done, graded: false, score: null,
        htmlUrl: '#', source: 'canvas', status: a.done ? 'done' : 'pending',
      };
    });
    st.courses = codes.map((code) => ({ id: 'c_' + code, canvasId: code, name: code, courseCode: code, currentScore: null, currentGrade: null }));
    // 心情
    st.moods = (C.state.moods || []).slice().sort((a, b) => b.at - a.at).map((m) => ({
      id: m.id, mood: (m.e || '') + ' ' + (m.l || ''), aiResponse: m.note || '',
      exerciseOffered: null, exerciseCompleted: false,
      theme: st.session.currentTheme, createdAt: new Date(m.at).toISOString(),
    }));
    // 记忆 → 关键事实
    st.keyFacts = (C.state.memories || []).map((m) => m.text);
    // 主题
    st.customThemes = C.state.customThemes || [];
    st.session.assistantName = C.state.assistantName || st.session.assistantName;
    st.session.currentTheme = (C.state.themes && C.state.themes.liuli) || st.session.currentTheme;
    st.session.language = C.state.language || st.session.language;
    // 规则由「未来 28 天里重复出现的块」归纳而来（核心没有 rule 实体）
    const g = {};
    for (let i = 0; i <= 28; i++) {
      const d = C.addDays(C.todayIso(), i);
      C.blocksOf(d).forEach((b) => {
        const k = b.title;
        if (!g[k]) g[k] = { first: b, days: [], wd: {} };
        g[k].days.push(d);
        g[k].wd[new Date(d + 'T12:00:00').getDay()] = 1;
      });
    }
    st.rules = Object.keys(g).filter((k) => g[k].days.length >= 2 || g[k].first.lockLevel).map((k) => {
      const e = g[k], b = e.first, wd = Object.keys(e.wd).map(Number);
      const daily = e.days.length >= 20;
      return {
        id: rid(b.title), title: b.title, type: b.type, time: b.start, duration_min: b.dur,
        timezone: 'Asia/Shanghai', time_mode: b.lockLevel === 'hard' ? 'fixed' : 'floating',
        kind: 'recurring', freq: daily ? 'daily' : 'weekly', interval: 1,
        by_weekday: daily ? [] : wd,
        start_date: e.days[0], until: null, active: true,
        source: b.lockLevel === 'hard' ? 'ics' : b.origin === 'auto' ? 'chat' : 'user',
        note: b.lockReason || null, managed: true,
      };
    });
    // 自存的定义优先：同名则覆盖（带着 active），停用的规则也留在列表里
    defs.forEach((d) => {
      const i = st.rules.findIndex((r) => r.id === d.id || r.title === d.title);
      if (i >= 0) st.rules[i] = Object.assign({}, st.rules[i], d, { managed: false });
      else st.rules.push(Object.assign({}, d, { managed: false }));
    });
  }

  const KIND2CAT = { academic: 'study', life: 'note', diet: 'diet', health: 'health', travel: 'travel' };
  const CAT2KIND = { study: 'academic', note: 'life', diet: 'life', health: 'life', travel: 'life' };

  function syncInbox() {
    // 资料库：核心的 materials 是真相source（保留初版本地的结构化演示条目在后面）
    const local = (st.materials || []).filter((m) => !m._core);
    const fromCore = (C.state.materials || []).map((m) => ({
      id: m.id, _core: true, category: KIND2CAT[m.kind] || 'note',
      title: m.title, summary: '', text: m.body || '',
      structured: null, advice: null, source: m.source || 'inbox',
      created_at: new Date(m.at).toISOString(),
    }));
    st.materials = fromCore.concat(local);
    st.memory = (C.state.memories || []).map((m) => ({ id: m.id, text: m.text, kind: m.type || 'preference', created_at: new Date(m.at).toISOString() }));
  }

  function syncAll() { syncPlans(); syncMaterials(); syncInbox(); }
  syncAll();

  // 核心变了 → 重投影 → 通知初版 UI
  let echo = false;
  C.subscribe(() => { if (echo) return; syncAll(); S.emit(); });
  const guard = (fn) => function () { echo = true; try { return fn.apply(null, arguments); } finally { echo = false; syncAll(); S.emit(); } };

  // ---- 写操作改道核心 ----
  S.getPlan = (d) => st.plans[d] || null;
  S.ruleOccursOn = (r, d) => !!(st.plans[d] && st.plans[d].blocks.some((b) => b.title === r.title));

  S.patchPlan = guard(function (date, action) {
    if (action.action === 'add') {
      const b = action.block || {};
      C.addBlock({ date, start: b.time || '12:00', dur: b.duration_min || 45, title: b.title || '未命名', type: b.type || 'task', origin: 'manual' });
    } else if (action.action === 'update') {
      const id = action.match && action.match.id; const ch = action.changes || {};
      if (!id) return null;
      const cur = (C.state.blocks || []).find((x) => x.id === id);
      if ('completed' in ch) C.setMark(id, ch.completed ? 'done' : 'none');
      if ('note' in ch) C.setNote(id, ch.note);
      if (ch.title || ch.type || ch.duration_min) C.setBlockFields(id, { title: ch.title, type: ch.type, dur: ch.duration_min });
      // 时间真的变了才动 —— 否则每次保存都会往共享账本里塞一条假的「挪动」
      if (ch.time && cur && ch.time !== cur.start) {
        const r = C.moveBlock(id, { date, start: ch.time, _confirmed: true });
        if (r && r.locked) return { locked: r.locked, reason: r.reason };
      }
    } else if (action.action === 'remove') {
      const id = action.match && action.match.id; if (id) C.removeBlock(id);
    }
    return null;
  });

  S.autoPlan = guard(function ({ from, to, instructions }) {
    const f = from || C.todayIso();
    const n = Math.max(1, Math.min(7, Math.round((new Date(to || f) - new Date(f)) / 864e5) + 1));
    let made = 0;
    for (let i = 0; i < n; i++) {
      const d = C.addDays(f, i);
      C.gapsOf(d, 45).slice(0, 3).forEach((g, k) => {
        const a = (C.state.assignments || []).filter((x) => !x.done)[k];
        C.addBlock({ date: d, start: g.start, dur: Math.min(90, g.min || 60), title: a ? a.course + ' · ' + a.title : '整理 + 预习', type: 'task', origin: 'auto' }, true);
        made++;
      });
    }
    if (!made) return { error: 'no_material' };
    return { plans: [], note: (instructions ? '按你说的「' + instructions + '」调整过了。' : '') + '排进了空档，晚上留白——不合适随手挪走。' };
  });

  S.addMood = guard((entry) => {
    const raw = (entry && entry.mood) || '';
    const e = raw.trim().slice(0, 2), l = raw.replace(e, '').trim();
    C.moodRecord({ e: e || '🙂', l: l || '记一下', note: entry && entry.aiResponse || '', source: 'user' });
    return C.state.moods[0];
  });
  S.completeMoodExercise = guard(() => {});
  S.patchAssignment = guard((id, ch) => { if ('status' in ch) C.setAssignmentDone(id, ch.status === 'done'); return (C.state.assignments || []).find((x) => x.id === id); });

  S.patchSession = guard((ch) => {
    if (ch.currentTheme) C.setTheme && C.setTheme('liuli', ch.currentTheme);
    if (ch.language) C.patch({ language: ch.language });
    if (ch.assistantName) C.patch({ assistantName: ch.assistantName });
    Object.assign(st.session, ch);
    return st.session;
  });

  S.aiTheme = (d, base) => C.aiTheme(d, base);
  S.createTheme = guard((i) => C.createTheme(i));
  S.patchTheme = guard((id, ch) => C.patchTheme(id, ch));
  S.deleteTheme = guard((id) => { const r = C.deleteTheme(id); return { ok: true, currentThemeReset: !!(r && r.reset) }; });

  // 资料 / 记忆 → 核心
  const _addMaterial = S.addMaterial, _delMaterial = S.deleteMaterial;
  S.addMaterial = guard((m) => {
    const r = C.materialAdd({ title: m.title || (m.text || '').slice(0, 22) || '一条记录', body: m.text || '', kind: CAT2KIND[m.category] || 'life', source: m.source || 'inbox' });
    const it = (C.state.materials || [])[0];
    return it ? { id: it.id, _core: true, category: m.category || 'note', title: it.title, summary: m.summary || '', text: it.body, structured: m.structured || null, advice: m.advice || null, source: it.source, created_at: new Date(it.at).toISOString() } : _addMaterial(m);
  });
  S.deleteMaterial = guard((id) => { if ((C.state.materials || []).some((m) => m.id === id)) C.materialRemove(id); else _delMaterial(id); });
  S.addMemoryFact = guard((f) => C.memoryAdd({ text: typeof f === 'string' ? f : (f && f.text) || '', type: (f && f.kind) || 'preference' }));
  S.deleteMemoryFact = guard((id) => C.memoryRemove(id));
  S.clearMemory = guard(() => (C.state.memories || []).slice().forEach((m) => C.memoryRemove(m.id)));

  // ---- 规则 = 核心里未来 28 天的一批块（核心无 rule 实体，规则由块归纳而来） ----
  const HORIZON = 28;
  const wdOf = (d) => new Date(d + 'T12:00:00').getDay();
  function occurs(r, d) {
    if (r.start_date && d < r.start_date) return false;
    if (r.until && d > r.until) return false;
    if (r.kind === 'once') return r.date === d;
    const diff = Math.round((new Date(d) - new Date(r.start_date || d)) / 864e5);
    if (r.freq === 'daily') return true;
    if (r.freq === 'every_n_days') return diff % Math.max(1, r.interval || 1) === 0;
    if (r.freq === 'weekly') { const ds = (r.by_weekday && r.by_weekday.length) ? r.by_weekday : [wdOf(r.start_date || d)]; return ds.indexOf(wdOf(d)) >= 0; }
    if (r.freq === 'monthly') return d.slice(8) === (r.start_date || d).slice(8);
    return false;
  }
  function spawn(r) {
    const T = C.todayIso(), add = [];
    for (let i = 0; i <= HORIZON; i++) {
      const d = C.addDays(T, i);
      if (!occurs(r, d)) continue;
      if (C.blocksOf(d).some((x) => x.title === r.title)) continue;
      add.push({ date: d, start: r.time || '12:00', dur: r.duration_min || 45, title: r.title, type: r.type || 'task', origin: 'manual' });
    }
    if (add.length) C.bulkBlocks({ add, label: '按规则铺开：' + r.title + '（' + add.length + ' 次）' });
    return add.length;
  }
  function despawn(title) {
    const T = C.todayIso(), ids = [];
    for (let i = 0; i <= HORIZON; i++) {
      C.blocksOf(C.addDays(T, i)).forEach((x) => { if (x.title === title && x.status === 'planned') ids.push(x.id); });
    }
    if (ids.length) C.bulkBlocks({ removeIds: ids, label: '停用规则：' + title + '（撤下 ' + ids.length + ' 次）' });
    return ids.length;
  }
  function mkDef(input) {
    return Object.assign({
      id: rid(input.title || ''), kind: 'recurring', freq: 'weekly', interval: 1, by_weekday: [],
      start_date: C.todayIso(), until: null, active: true, type: 'task', time: null,
      duration_min: 45, timezone: 'Asia/Shanghai', time_mode: 'floating', source: 'user', note: null,
    }, input);
  }
  // 归纳出来的规则没有真实周期（只是从现有块反推的猜测），一律不可写
  const own = (id) => defs.find((x) => x.id === id) || null;
  S.createRule = guard((input) => { const d = mkDef(input); defs.push(d); saveDefs(); spawn(d); return d; });
  S.saveRulesBatch = guard((inputs) => (inputs || []).map((i) => { const d = mkDef(i); defs.push(d); saveDefs(); spawn(d); return d; }));
  S.patchRule = guard((id, ch) => {
    const d = own(id); if (!d) return { error: 'managed' };
    const oldTitle = d.title;
    if (ch.active === false) { Object.assign(d, ch); despawn(oldTitle); saveDefs(); return d; }
    Object.assign(d, ch);
    if (d.active === false) { saveDefs(); return d; }
    if ((ch.title && ch.title !== oldTitle) || ch.time || ch.duration_min || ch.type || ch.by_weekday || ch.freq) despawn(oldTitle);
    spawn(d); saveDefs(); return d;
  });
  S.deleteRule = guard((id) => {
    const d = own(id); if (!d) return { error: 'managed' };
    despawn(d.title); defs = defs.filter((x) => x.id !== d.id); saveDefs();
  });

  window.DcBridge = { syncAll };
})();
