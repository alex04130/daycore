// Real data layer. Keeps the design prototype's DcStore interface (state +
// subscribe + sync getters over a cache) but every mutation talks to the Go
// backend per api/openapi.yaml. Pages stay 1:1 with the prototype.
import * as api from './api.js';
import I18N from './i18n.js';

// ---------- date helpers (identical to the prototype) ----------
const pad = (n) => String(n).padStart(2, '0');
const iso = (d) => d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate());
const todayIso = () => iso(new Date());
const addDays = (dateStr, n) => {
  const [y, m, d] = dateStr.split('-').map(Number);
  return iso(new Date(y, m - 1, d + n));
};
const dayDiff = (a, b) => Math.round((new Date(a) - new Date(b)) / 864e5);
const weekdayOf = (dateStr) => new Date(dateStr + 'T12:00:00').getDay(); // 0=Sun
const toMin = (t) => { const [h, m] = t.split(':').map(Number); return h * 60 + m; };
const toHM = (min) => pad(Math.floor(min / 60)) + ':' + pad(min % 60);
let idc = 1000;
const uid = (p) => (p || 'id') + '_' + (++idc) + '_' + Math.random().toString(36).slice(2, 6);
const TZ = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';

// API contract major this frontend was built against (GET /api/version).
// A mismatch sets state.apiMismatch so the UI can ask the user to upgrade.
const API_VERSION_EXPECTED = 1;

// clientDateCtx builds the {date, weekday, time, timezone} fields the AI
// endpoints expect from the client's clock.
function clientDateCtx() {
  const now = new Date();
  return {
    date: todayIso(),
    weekday: new Intl.DateTimeFormat(I18N.lang, { weekday: 'long' }).format(now),
    time: pad(now.getHours()) + ':' + pad(now.getMinutes()),
    timezone: TZ,
  };
}

// ---------- state ----------
const state = {
  ready: false,
  version: '',
  channel: '',
  apiMismatch: false, // true when the backend's apiVersion ≠ API_VERSION_EXPECTED
  session: { id: '', assistantName: 'Leo', currentTheme: 'sky', language: 'zh-CN', userId: null },
  user: null,
  plans: {},          // date -> DayPlan (server-merged blocks, visible only)
  plansLoaded: {},    // date -> true once covered by a range fetch
  rules: [],
  courses: [],
  assignments: [],
  customThemes: [],
  moods: [],
  chat: [],
  memoryFacts: [],    // [{id, fact, source, createdAt}]
  importHistory: [],  // [{id, source, items, summary, createdAt}]
  importToken: '',
  onboarded: (() => { try { return localStorage.getItem('daycore-onboarded') === '1'; } catch (_) { return false; } })(),
  get keyFacts() { return this.memoryFacts.map((f) => f.fact); }, // prototype-compatible view
  get canvasImportedAt() {
    const rec = this.importHistory.find((x) => x.source === 'canvas');
    return rec ? rec.createdAt : null;
  },
  get icsImportedAt() {
    const rec = this.importHistory.find((x) => x.source === 'ics' || x.source === 'image');
    return rec ? rec.createdAt : null;
  },
};

const subs = new Set();
function emit() { subs.forEach((fn) => fn(state)); }

// ---------- bootstrap ----------
async function bootstrap() {
  const sess = await api.post('/api/session/init');
  Object.assign(state.session, sess);
  if (sess.language) I18N.setLang(sess.language);
  try {
    // /api/version is the contract endpoint (apiVersion = breaking-change
    // major); healthz stays as fallback for older backends.
    const v = await api.get('/api/version');
    state.version = v.build || '';
    state.channel = v.channel || '';
    if (v.apiVersion && v.apiVersion !== API_VERSION_EXPECTED) {
      state.apiMismatch = true;
    }
  } catch (_) {
    try {
      const h = await api.get('/api/healthz');
      state.version = h.version || '';
      state.channel = h.channel || '';
    } catch (_) { /* version display is best-effort */ }
  }
  await refreshAll();
  state.ready = true;
  emit();
}

async function refreshAll() {
  const T = todayIso();
  const jobs = [
    loadRules(), loadCourses(), loadAssignments(), loadThemes(), loadMoods(),
    loadMemory(), loadImportHistory(), loadToken(), loadMe(), loadChat(),
    ensurePlans(addDays(T, -7), addDays(T, 13), true),
  ];
  await Promise.allSettled(jobs);
  emit();
}

const loadRules = async () => { const r = await api.get('/api/rules'); state.rules = r.rules || []; };
const loadCourses = async () => { const r = await api.get('/api/courses'); state.courses = r.courses || []; };
const loadAssignments = async () => { const r = await api.get('/api/assignments'); state.assignments = r.assignments || []; };
const loadThemes = async () => { const r = await api.get('/api/themes'); state.customThemes = r.themes || []; };
const loadMoods = async () => { state.moods = (await api.get('/api/mood?limit=20')) || []; };
const loadMemory = async () => { const r = await api.get('/api/memory'); state.memoryFacts = r.facts || []; };
const loadImportHistory = async () => { const r = await api.get('/api/import/history'); state.importHistory = r.imports || []; };
const loadToken = async () => { const r = await api.get('/api/import/token'); state.importToken = r.token || ''; };
const loadMe = async () => { try { const r = await api.get('/api/me'); state.user = r.user || null; } catch (_) { state.user = null; } };
const loadChat = async () => {
  try {
    const r = await api.get('/api/companion-history');
    state.chat = (r.history || []).map((m) => ({ role: m.role, content: m.content, timestamp: m.timestamp }));
    state.legacyKeyFacts = r.keyFacts || [];
  } catch (_) { state.chat = []; state.legacyKeyFacts = []; }
};

// ---------- plans ----------
function getPlan(dateStr) { return state.plans[dateStr] || null; }

let planFetch = Promise.resolve();
async function ensurePlans(from, to, force) {
  if (!force) {
    let covered = true;
    for (let d = from; d <= to; d = addDays(d, 1)) if (!state.plansLoaded[d]) { covered = false; break; }
    if (covered) return;
  }
  // serialize range fetches so overlapping requests don't interleave writes
  planFetch = planFetch.then(async () => {
    const plans = await api.get('/api/plan/range?from=' + from + '&to=' + to);
    for (let d = from; d <= to; d = addDays(d, 1)) { delete state.plans[d]; state.plansLoaded[d] = true; }
    (plans || []).forEach((p) => { state.plans[p.date] = p; });
    emit();
  }).catch(() => {});
  return planFetch;
}
function invalidatePlans() {
  const dates = Object.keys(state.plansLoaded);
  state.plans = {}; state.plansLoaded = {};
  if (dates.length) {
    dates.sort();
    return ensurePlans(dates[0], dates[dates.length - 1], true);
  }
}

async function patchPlan(dateStr, action) {
  const plan = await api.patch('/api/plan', { date: dateStr, action });
  state.plans[dateStr] = plan;
  state.plansLoaded[dateStr] = true;
  emit();
  return plan;
}

// ---------- rule expansion mirror (calendar badges only) ----------
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
    case 'monthly': return Number(dateStr.split('-')[2]) === Number(start.split('-')[2]);
    default: return false;
  }
}

// ---------- AI planning ----------
async function autoPlan({ from, to, instructions, mode }) {
  const ctx = clientDateCtx();
  try {
    const res = await api.post('/api/ai/auto-plan', {
      from, to, instructions: instructions || '', mode: mode || 'keep_manual',
      date: ctx.date, weekday: ctx.weekday, time: ctx.time, timezone: ctx.timezone,
    });
    (res.plans || []).forEach((p) => { state.plans[p.date] = p; state.plansLoaded[p.date] = true; });
    loadAssignments().then(emit).catch(() => {});
    emit();
    return res;
  } catch (e) {
    return { error: e.code || 'server_error', message: e.message };
  }
}

async function planFromText(text, targetDate) {
  const ctx = clientDateCtx();
  try {
    const res = await api.post('/api/ai/plan-text', {
      description: text, targetDate,
      date: ctx.date, weekday: ctx.weekday, time: ctx.time, timezone: ctx.timezone,
    });
    return { note: res.note, blocks: res.blocks || [] };
  } catch (e) {
    return { error: e.code || 'server_error', message: e.message };
  }
}

async function planFromImage(imageBase64, mimeType, targetDate) {
  const ctx = clientDateCtx();
  try {
    const res = await api.post('/api/ai/plan-image', {
      imageBase64, mimeType, targetDate,
      date: ctx.date, weekday: ctx.weekday, time: ctx.time, timezone: ctx.timezone,
    });
    return { note: res.note, blocks: res.blocks || [] };
  } catch (e) {
    return { error: e.code || 'server_error', message: e.message };
  }
}

// ---------- rules CRUD ----------
async function createRule(input) {
  const r = await api.post('/api/rules', { ...input, timezone: input.timezone || TZ });
  state.rules.unshift(r);
  invalidatePlans();
  emit();
  return r;
}
async function patchRule(id, changes) {
  const r = await api.patch('/api/rules/' + id, changes);
  state.rules = state.rules.map((x) => (x.id === id ? r : x));
  invalidatePlans();
  emit();
  return r;
}
async function deleteRule(id) {
  await api.del('/api/rules/' + id);
  state.rules = state.rules.filter((x) => x.id !== id);
  invalidatePlans();
  emit();
}
async function saveRulesBatch(inputs) {
  const res = await api.post('/api/rules/batch', {
    rules: inputs.map((r) => ({ ...r, timezone: r.timezone || TZ })),
  });
  await Promise.allSettled([loadRules(), loadImportHistory()]);
  invalidatePlans();
  emit();
  return res.rules || [];
}

// ---------- imports ----------
async function importCanvas(fileText) {
  let payload;
  try { payload = JSON.parse(fileText); } catch (_) {
    return { error: 'unsupported_export_version', message: 'not JSON' };
  }
  try {
    const res = await api.post('/api/import/canvas', payload);
    await Promise.allSettled([loadCourses(), loadAssignments(), loadImportHistory()]);
    emit();
    return res;
  } catch (e) {
    return { error: e.code || 'server_error', message: e.message };
  }
}
async function icsCandidates(icsText) {
  try {
    const res = await api.post('/api/import/ics', { icsText, preview: true, timezone: TZ });
    return { rules: res.rules || [], warnings: res.warnings || [] };
  } catch (e) {
    return { error: e.code || 'server_error', message: e.message };
  }
}
async function shotCandidates(imageBase64, mimeType) {
  const ctx = clientDateCtx();
  try {
    const res = await api.post('/api/ai/extract-schedule-image', {
      imageBase64, mimeType,
      date: ctx.date, weekday: ctx.weekday, time: ctx.time, timezone: ctx.timezone,
    });
    return { rules: (res.rules || []).map((r) => ({ ...r, source: 'image' })) };
  } catch (e) {
    return { error: e.code || 'server_error', message: e.message };
  }
}

// ---------- assignments ----------
async function patchAssignment(id, changes) {
  const a = await api.patch('/api/assignments/' + id, changes);
  state.assignments = state.assignments.map((x) => (x.id === id ? a : x));
  emit();
  return a;
}

// ---------- mood ----------
// The mood vocabulary comes from the backend registry, not from a list kept
// here. Keeping our own is what let the two drift into different vocabularies —
// and what made every check-in invisible to the companion, because we were
// storing "😊 开心" where the backend looks up an id.
let moodKindsCache = null;
async function moodKinds() {
  if (!moodKindsCache) {
    const res = await api.get('/api/mood/kinds');
    moodKindsCache = res.kinds || [];
  }
  return moodKindsCache;
}
async function moodAIResponse(moodText) {
  const res = await api.post('/api/ai/mood', { mood: moodText });
  return res.response || '';
}
async function addMood(entry) {
  const m = await api.post('/api/mood', { ...entry, theme: state.session.currentTheme });
  state.moods.unshift(m);
  emit();
  return m;
}
async function completeMoodExercise(id) {
  await api.patch('/api/mood', { id });
  state.moods = state.moods.map((m) => (m.id === id ? { ...m, exerciseCompleted: true } : m));
  emit();
}

// ---------- themes ----------
async function aiTheme(description, base, themeId) {
  try {
    return await api.post('/api/ai/theme', { description, base: base || '', themeId: themeId || '' });
  } catch (e) {
    return { error: e.code || 'server_error', message: e.message };
  }
}
async function createTheme(input) {
  const t = await api.post('/api/themes', input);
  state.customThemes.push(t);
  emit();
  return t;
}
async function patchTheme(id, changes) {
  const t = await api.patch('/api/themes/' + id, changes);
  state.customThemes = state.customThemes.map((x) => (x.id === id ? t : x));
  emit();
  return t;
}
async function deleteTheme(id) {
  const res = await api.del('/api/themes/' + id);
  state.customThemes = state.customThemes.filter((x) => x.id !== id);
  if (res.currentThemeReset) state.session.currentTheme = 'sky';
  emit();
  return res;
}

// ---------- session / auth ----------
async function patchSession(changes) {
  if (changes.currentTheme && Object.keys(changes).length === 1) {
    state.session.currentTheme = changes.currentTheme; // optimistic — theme flips instantly
    emit();
    api.post('/api/session/theme', { theme: changes.currentTheme }).catch(() => {});
    return state.session;
  }
  const sess = await api.patch('/api/session/settings', changes);
  Object.assign(state.session, sess);
  emit();
  return state.session;
}
async function register(f) {
  const r = await api.post('/api/auth/register', { email: f.email, password: f.password, name: f.name });
  state.user = r.user || r;
  emit();
  return state.user;
}
async function login(f) {
  const r = await api.post('/api/auth/login', { email: f.email, password: f.password });
  state.user = r.user || r;
  emit();
  return state.user;
}
async function logout() {
  await api.post('/api/auth/logout');
  state.user = null;
  emit();
}

async function genToken() {
  const r = await api.post('/api/import/token');
  state.importToken = r.token;
  emit();
  return r.token;
}

// ---------- memory (§7) ----------
async function addMemoryFact(fact, source) {
  const f = await api.post('/api/memory', { fact, source: source || 'chat' });
  state.memoryFacts.push(f);
  emit();
  return f;
}
async function deleteMemoryFact(id) {
  await api.del('/api/memory/' + id);
  state.memoryFacts = state.memoryFacts.filter((f) => f.id !== id);
  emit();
}
async function clearMemory() {
  const r = await api.del('/api/memory');
  state.memoryFacts = [];
  emit();
  return r;
}

// ---------- companion chat (SSE v2 agent stream) ----------
// reloadRules / reloadMemory: refresh caches after a server-side mutation the
// store didn't perform itself (agent tools, op reverts). Rule changes ripple
// into generated plans, hence the invalidatePlans.
const reloadRules = () => loadRules().then(() => { emit(); return invalidatePlans(); }).catch(() => {});
const reloadMemory = () => loadMemory().then(emit).catch(() => {});

// applyToolResult syncs caches from a tool_result frame so pages only render:
// mutating tools return fresh state inline (plans) or trigger a refetch
// (rules/memory); query tools leave caches alone.
function applyToolResult(f) {
  if (!f.ok) return;
  const tool = f.tool || '';
  if (tool.startsWith('plan_')) {
    if (f.data && f.data.plan) {
      state.plans[f.data.date] = f.data.plan;
      state.plansLoaded[f.data.date] = true;
      emit();
    }
  } else if (tool.startsWith('rule_')) {
    reloadRules();
  } else if (tool.startsWith('memory_')) {
    reloadMemory();
  }
}

// streamChat sends one user message through the agent stream; `on` receives
// the frame callbacks (see api.streamAgent). tool_result frames update the
// store caches before reaching the page.
let chatAbortCtrl = null;
function abortChat() {
  if (chatAbortCtrl) {
    chatAbortCtrl.abort();
    chatAbortCtrl = null;
  }
}
async function streamChat(message, conversationHistory, on) {
  abortChat();
  const ctrl = new AbortController();
  chatAbortCtrl = ctrl;
  try {
    await api.streamAgent('/api/ai/companion', {
      message,
      timezone: TZ,
      assistantName: state.session.assistantName,
      conversationHistory,
    }, {
      ...on,
      toolResult(f) { applyToolResult(f); on.toolResult && on.toolResult(f); },
    }, ctrl.signal);
  } finally {
    if (chatAbortCtrl === ctrl) chatAbortCtrl = null;
  }
}

let chatPersistTimer = null;
function persistChat(history) {
  state.chat = history;
  clearTimeout(chatPersistTimer);
  chatPersistTimer = setTimeout(() => {
    api.post('/api/companion-history', {
      history: state.chat, keyFacts: state.legacyKeyFacts || [],
    }).catch(() => {});
  }, 2000);
}

function setOnboarded(v) {
  state.onboarded = v;
  try { localStorage.setItem('daycore-onboarded', v ? '1' : '0'); } catch (_) {}
  emit();
}

const S = {
  get state() { return state; },
  subscribe(fn) { subs.add(fn); return () => subs.delete(fn); },
  emit, bootstrap, refreshAll,
  todayIso, addDays, dayDiff, weekdayOf, toMin, toHM, uid,
  clientDateCtx,
  getPlan, ensurePlans, patchPlan, invalidatePlans,
  autoPlan, planFromText, planFromImage,
  ruleOccursOn,
  createRule, patchRule, deleteRule, saveRulesBatch,
  importCanvas, icsCandidates, shotCandidates,
  patchAssignment,
  moodKinds, moodAIResponse, addMood, completeMoodExercise,
  aiTheme, createTheme, patchTheme, deleteTheme,
  patchSession, register, login, logout, genToken,
  addMemoryFact, deleteMemoryFact, clearMemory,
  streamChat, abortChat, persistChat, reloadRules, reloadMemory,
  setOnboarded,
};
export default S;
