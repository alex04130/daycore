// Daycore Admin — mock server state behind the X-Admin-Token console
window.AdminStore = (function () {
  'use strict';
  const listeners = new Set();
  let version = 0;
  const emit = () => { version++; listeners.forEach((f) => f()); };
  const subscribe = (f) => { listeners.add(f); return () => listeners.delete(f); };
  const get = () => version;

  const NOW = Date.now();
  const ago = (min) => NOW - min * 60000;
  const p2 = (n) => String(n).padStart(2, '0');
  const fmtT = (ts) => { const d = new Date(ts); return `${p2(d.getMonth() + 1)}-${p2(d.getDate())} ${p2(d.getHours())}:${p2(d.getMinutes())}`; };
  const relT = (ts) => { const m = Math.max(1, Math.round((NOW - ts) / 60000)); if (m < 60) return m + ' 分钟前'; const h = Math.round(m / 60); if (h < 24) return h + ' 小时前'; return fmtT(ts); };

  // ---------- auth ----------
  const DEMO_TOKEN = 'daycore-admin';
  let token = null;
  try { token = sessionStorage.getItem('dc-admin-token'); } catch (e) {}
  const isAuthed = () => !!token;
  function login(tok) {
    if (tok !== DEMO_TOKEN) return { error: 'unauthorized' };
    token = tok; try { sessionStorage.setItem('dc-admin-token', tok); } catch (e) {}
    emit(); return { ok: true };
  }
  function logout() { token = null; try { sessionStorage.removeItem('dc-admin-token'); } catch (e) {} emit(); }

  // ---------- meta ----------
  const health = { ok: true, db: 'ok', version: '2.2.0-beta', channel: 'beta', env: 'production', uptime: '3 天 14 小时' };
  const versionInfo = { apiVersion: 2, apiMinor: 2, build: '2.2.0-beta', minClient: '2.0.0' };
  const stats = { users: 6, sessions: 14, aiCalls: 12480, tokenUsed: 8412030, feedbackUseful: 231, feedbackTotal: 252 };

  // ---------- prompts ----------
  const prompts = [
    { key: 'companion_system', name: '陪伴聊天', desc: 'companion SSE agent 的系统提示（11 个工具）' },
    { key: 'auto_plan', name: '自主规划', desc: 'POST /api/ai/auto-plan 的计划生成提示' },
    { key: 'mood_reply', name: '心情回应', desc: '心情打卡后的共情回复与练习邀请' },
    { key: 'inbox_classify', name: '随手记归类', desc: '随手记录 → 类别/结构化字段/建议动作' },
    { key: 'theme_generate', name: 'AI 主题', desc: '12 变量白名单内生成配色（POST /api/ai/theme）' },
    { key: 'travel_plan', name: '出行规划', desc: 'POST /api/ai/travel 行程草案生成' },
    { key: 'vision_food', name: '食物识别', desc: '饮食拍照的营养估计（vision）' },
    { key: 'vision_timetable', name: '课表识别', desc: '课表截图 → 候选时间块（vision）' },
  ];
  const promptDefaults = {
    companion_system: {
      'zh-CN': '你是 {{assistantName}}，一位温和、务实的日程陪伴助手。\n\n- 用户时区 {{timezone}}，今天是 {{today}}；会话语言 zh-CN。\n- 可用工具：get_weather / web_search / list_upcoming / plan_add / plan_update / plan_remove / rule_upsert / rule_remove / memory_add / memory_remove / propose_decision，单次回复最多 6 轮。\n- 改动计划前先看 list_upcoming；永不覆盖 origin=manual 的块。\n- 不确定时用自然语言追问；两个以上互斥选项用 propose_decision 发决策卡。\n- 值得长期记住的事实（作息偏好、过敏、常去地点）用 memory_add 落库。\n- 回复简短、贴近学生生活，不说教，不堆表情。',
      'en-US': 'You are {{assistantName}}, a warm, practical day-planning companion.\n\n- User timezone {{timezone}}; today is {{today}}; locale en-US.\n- Tools: get_weather / web_search / list_upcoming / plan_add / plan_update / plan_remove / rule_upsert / rule_remove / memory_add / memory_remove / propose_decision; max 6 rounds per reply.\n- Check list_upcoming before edits; never overwrite origin=manual blocks.\n- Ask when unsure; use propose_decision for mutually exclusive choices.\n- Keep replies short and concrete.',
    },
    auto_plan: {
      'zh-CN': '根据用户资料生成 {{from}} 至 {{to}} 的计划（JSON blocks）。\n\n输入：重复规则、临期作业（含 Canvas due）、记忆 facts、用户附加要求 {{instructions}}。\n约束：\n- mode=keep_manual 时保留所有 manual 块，只填空档；replace_all 时重排全部 auto 块。\n- 每天 6:30-23:30 之间安排；连续专注不超过 90 分钟，之间留 15 分钟休息。\n- 作业按截止日倒排，优先临期；饭点 (11:30-13:00 / 17:30-19:00) 留 meal 块。\n- 输出附一句 note 总结取舍理由，语气自然。',
      'en-US': 'Generate plan blocks (JSON) for {{from}}..{{to}} from rules, due assignments, memory facts and user instructions {{instructions}}.\n- keep_manual: fill gaps only; replace_all: rebuild auto blocks.\n- Schedule 6:30-23:30; ≤90min focus runs with 15min breaks; keep meal windows.\n- Add a one-line note explaining trade-offs.',
    },
    mood_reply: {
      'zh-CN': '用户刚完成一次心情打卡：{{mood}}（强度 {{intensity}}）。结合记忆 facts 给一段 2-3 句的贴身回应：\n- 先共情，再给一个可立刻做的小建议。\n- 负向情绪（焦虑/低落/烦躁/疲惫）时，附一句练习邀请（呼吸 4-7-8 / 伸展 / 着地 5-4-3-2-1 三选一）。\n- 检测到危机信号时只输出 crisis_detected 结构，语气温和，附求助热线。',
      'en-US': 'The user checked in mood {{mood}} (intensity {{intensity}}). Reply in 2-3 sentences: empathize first, then one tiny actionable suggestion. For negative moods, invite one exercise (breathing / stretch / grounding). On crisis signals output the crisis_detected structure only.',
    },
    inbox_classify: {
      'zh-CN': '把用户的随手记录归入启用类别之一：{{enabledCategories}}。\n\n输出 classification JSON：category / confidence / title / summary / structured / suggested_action。\n- diet：items[] 与营养估计（kcal/蛋白质）；health：symptoms + advice（保守自护建议，必须含就医提醒，不做诊断）；academic：course、due_at（识别到截止时间时 suggested_action=create_assignment）；travel：起止日期与目的地。\n- 置信度 < 0.55 时不要猜类别，返回关键词回退（understanding + suggestions）。',
      'en-US': 'Classify the note into one enabled category: {{enabledCategories}}. Output classification JSON (category / confidence / title / summary / structured / suggested_action). diet→items+nutrition; health→symptoms+conservative advice with a see-a-doctor reminder; academic→course+due_at (suggest create_assignment); travel→dates+destination. Below 0.55 confidence, fall back to keywords.',
    },
    theme_generate: {
      'zh-CN': '根据描述「{{description}}」生成一套主题，基准 {{base}}。\n\n只输出 12 个白名单变量：--primary --accent --bg-start --bg-end --text-primary --text-secondary --text-muted --surface --surface-hover --success --warning --error。\n- 值仅限颜色字面量（hex/rgb/rgba/hsl/hsla/transparent）。\n- 文字对比度 ≥ 4.5:1；深色底座（dark=true）时文字必须够亮。\n- 附 name（2-4 字中文名）。',
      'en-US': 'Generate a theme for "{{description}}" on base {{base}}. Output ONLY the 12 whitelisted vars (--primary … --error) as color literals. Keep text contrast ≥ 4.5:1; bright text on dark bases. Include a short name.',
    },
    travel_plan: {
      'zh-CN': '为目的地 {{destination}}（{{startDate}} ~ {{endDate}}）生成行程草案：\n- itinerary.days[]：每天 theme + items[]（时间 / 地点 / 备注，步行动线合理，一天不超过 4 个点）。\n- tips[]：3-5 条实用提示（交通卡、预约、天气）。\n- 结合用户备注 {{notes}} 与记忆 facts（饮食忌口、体力偏好）。',
      'en-US': 'Draft an itinerary for {{destination}} ({{startDate}}..{{endDate}}): days[] with theme + ≤4 items each (time/place/note, walkable order); 3-5 practical tips; respect user notes {{notes}} and memory facts.',
    },
    vision_food: {
      'zh-CN': '识别图片中的食物，输出 items[]（名称 / 份量估计 / kcal / 蛋白质g）与 total。\n- 不是食物时返回 not_food，并建议尝试课表识别（extract_image）。\n- 估计保守给区间中值；小吃按常见份量。',
      'en-US': 'Identify food in the image: items[] (name / portion / kcal / protein g) and total. If not food, return not_food and suggest timetable extraction. Estimate conservatively.',
    },
    vision_timetable: {
      'zh-CN': '从课表截图提取候选时间块：title / weekday(0=周日) / time / duration_min / location。\n- 无法可靠读出时间的行跳过，不要编造；整图不可读返回 unreadable_image；不是课表返回 not_a_timetable。',
      'en-US': 'Extract candidate blocks from the timetable screenshot: title / weekday (0=Sun) / time / duration_min / location. Skip unreadable rows — never invent; return unreadable_image or not_a_timetable when applicable.',
    },
  };
  let promptOvr = {};
  try { promptOvr = JSON.parse(localStorage.getItem('dc-admin-prompt-ovr') || '{}'); } catch (e) { promptOvr = {}; }
  const ovrKey = (k, l) => k + '@' + l;
  const persistOvr = () => { try { localStorage.setItem('dc-admin-prompt-ovr', JSON.stringify(promptOvr)); } catch (e) {} };
  const getPrompt = (k, l) => (ovrKey(k, l) in promptOvr) ? promptOvr[ovrKey(k, l)] : (promptDefaults[k] && promptDefaults[k][l]) || '';
  const isModified = (k, l) => l ? (ovrKey(k, l) in promptOvr) : ((ovrKey(k, 'zh-CN') in promptOvr) || (ovrKey(k, 'en-US') in promptOvr));
  function savePrompt(k, l, content) {
    if (content === promptDefaults[k][l]) delete promptOvr[ovrKey(k, l)];
    else promptOvr[ovrKey(k, l)] = content;
    persistOvr(); emit(); return { ok: true };
  }
  function resetPrompt(k, l) { delete promptOvr[ovrKey(k, l)]; persistOvr(); emit(); }

  // ---------- prompt placeholders (演示元数据；GET /api/admin/prompts 返回 placeholders[]) ----------
  const PH = {
    companion_system: [['assistantName', '助手昵称（设置页）'], ['today', '今天日期'], ['timezone', '用户时区'], ['locale', '会话语言'], ['l2Prompt', '用户自定义 L2 提示词（角色扮演 / 用户属性）'], ['memoryFacts', '记忆 facts 列表']],
    auto_plan: [['from', '起始日期'], ['to', '结束日期'], ['mode', 'keep_manual / replace_all'], ['rules', '重复规则 JSON'], ['assignments', '临期作业 JSON'], ['memoryFacts', '记忆 facts'], ['instructions', '用户附加要求'], ['l2Prompt', '用户 L2 提示词']],
    mood_reply: [['mood', '心情类型'], ['intensity', '强度 1-5'], ['note', '打卡备注'], ['memoryFacts', '记忆 facts'], ['l2Prompt', '用户 L2 提示词']],
    inbox_classify: [['text', '用户原文'], ['enabledCategories', '启用的类别'], ['today', '今天日期']],
    theme_generate: [['description', '用户描述'], ['base', '基准主题'], ['baseVars', '基准主题 12 变量当前值']],
    travel_plan: [['destination', '目的地'], ['startDate', '开始日期'], ['endDate', '结束日期'], ['notes', '用户备注'], ['memoryFacts', '记忆 facts'], ['l2Prompt', '用户 L2 提示词']],
    vision_food: [['locale', '会话语言']],
    vision_timetable: [['locale', '会话语言'], ['termStart', '学期开始日（推断周次）']],
  };
  const promptPlaceholders = (k) => PH[k] || [];

  // ---------- env config (.env → GET/PUT /api/admin/config) ----------
  const ENV_GROUPS = [
    { id: 'server', label: '服务', items: [
      { key: 'APP_ENV', type: 'select', opts: ['development', 'production'], v: 'production', d: '运行环境' },
      { key: 'HOST', type: 'text', v: '127.0.0.1', d: '监听接口；留空 = 全部。nginx 后用 127.0.0.1', restart: true },
      { key: 'PORT', type: 'number', v: '8080', d: '监听端口', restart: true },
      { key: 'STATIC_DIR', type: 'text', v: 'web/frontend/dist', d: '由 Go 服务的前端构建目录（SPA 回退）；缺失 = 仅 API', restart: true },
      { key: 'PUBLIC_BASE_URL', type: 'text', v: 'https://daycore.app', d: '对外基址（OAuth 回调）' },
      { key: 'ALLOWED_ORIGINS', type: 'text', v: '', d: '独立前端的 CORS 白名单（逗号分隔，仅开发）' },
    ] },
    { id: 'db', label: '数据库', items: [
      { key: 'DB_TYPE', type: 'select', opts: ['sqlite', 'postgres', 'mysql', 'mongodb'], v: 'sqlite', d: '已注册驱动', restart: true },
      { key: 'DB_DSN', type: 'text', v: 'file:daycore.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)', d: '连接串，格式随 DB_TYPE 变化', restart: true },
    ] },
    { id: 'auth', label: '认证', items: [
      { key: 'JWT_SECRET', type: 'secret', set: true, d: '签名密钥（openssl rand -hex 32）' },
      { key: 'COOKIE_SECRET', type: 'secret', set: true, d: 'Cookie 加密密钥' },
      { key: 'PASSWORD_PEPPER', type: 'secret', set: false, d: '可选的服务端哈希附加密钥' },
      { key: 'JWT_TTL', type: 'text', v: '720h', d: '登录态有效期' },
      { key: 'SECURE_COOKIES', type: 'bool', v: true, d: 'HTTPS / 生产环境开启' },
    ] },
    { id: 'ai', label: 'AI 与模型', items: [
      { key: 'DEFAULT_CHAT_MODEL', type: 'text', v: 'deepseek-chat', d: '须匹配 models.yaml 中的 id' },
      { key: 'DEFAULT_VISION_MODEL', type: 'text', v: '', d: '留空 = 目录中第一个 vision 模型' },
      { key: 'DEFAULT_PLANNER_MODEL', type: 'text', v: '', d: 'auto-plan 模型；留空 = 对话模型' },
      { key: 'AI_REQUEST_TIMEOUT', type: 'text', v: '120s', d: '单次 AI 请求超时' },
      { key: 'AGENT_MAX_ROUNDS', type: 'number', v: '6', d: '陪伴 agent 每条消息的工具轮数' },
      { key: 'MODELS_CONFIG', type: 'text', v: 'config/models.yaml', d: '模型目录文件路径', restart: true },
      { key: 'OAUTH_CONFIG', type: 'text', v: 'config/oauth.yaml', d: 'OAuth 配置文件路径', restart: true },
    ] },
    { id: 'keys', label: 'API 密钥', items: [
      { key: 'DEEPSEEK_API_KEY', type: 'secret', set: true, d: 'models.yaml 中 api_key_env 引用' },
      { key: 'OPENAI_API_KEY', type: 'secret', set: true, d: 'models.yaml 中 api_key_env 引用' },
      { key: 'ANTHROPIC_API_KEY', type: 'secret', set: false, d: 'models.yaml 中 api_key_env 引用' },
      { key: 'TAVILY_API_KEY', type: 'secret', set: false, d: '陪伴 agent 联网搜索（可选；回退 DuckDuckGo）' },
    ] },
    { id: 'limits', label: '限额与管理', items: [
      { key: 'AI_RATE_LIMIT_PER_MIN', type: 'number', v: '30', d: '每会话每分钟 AI 调用上限' },
      { key: 'MAX_IMAGE_BYTES', type: 'number', v: '8388608', d: '上传图片上限（8 MiB）' },
      { key: 'ADMIN_TOKEN', type: 'secret', set: true, d: '本控制台鉴权；留空 = 仅开发环境开放' },
    ] },
    { id: 'plan', label: '自主规划', items: [
      { key: 'AUTO_PLAN_MAX_DAYS', type: 'number', v: '7', d: '一次 /api/ai/auto-plan 覆盖的最大天数' },
      { key: 'ASSIGNMENT_LOOKAHEAD_DAYS', type: 'number', v: '14', d: '作业提前多少天计入 AI 上下文' },
    ] },
  ];
  const ENV_INDEX = {}; ENV_GROUPS.forEach((g) => g.items.forEach((it) => { ENV_INDEX[it.key] = it; }));
  let envOvr = {};
  try { envOvr = JSON.parse(localStorage.getItem('dc-admin-env') || '{}'); } catch (e) { envOvr = {}; }
  const envGroups = () => ENV_GROUPS;
  const envDef = (k) => ENV_INDEX[k];
  const envValue = (k) => { const d = ENV_INDEX[k]; if (!d) return ''; if (d.type === 'secret') return ''; return (k in envOvr) ? envOvr[k] : (d.v === undefined ? '' : d.v); };
  const envIsSet = (k) => { const d = ENV_INDEX[k]; return !!((k in envOvr && envOvr[k] !== '') || (d && d.set)); };
  function setEnvMany(map) {
    Object.keys(map).forEach((k) => { envOvr[k] = map[k]; });
    try { localStorage.setItem('dc-admin-env', JSON.stringify(envOvr)); } catch (e) {}
    emit(); return { saved: Object.keys(map), needsRestart: Object.keys(map).filter((k) => ENV_INDEX[k] && ENV_INDEX[k].restart) };
  }

  // ---------- models.yaml (GET/PUT /api/admin/models) ----------
  const MODEL_DEFAULTS = [
    { id: 'deepseek-chat', format: 'openai', base_url: 'https://api.deepseek.com', model: 'deepseek-chat', api_key_env: 'DEEPSEEK_API_KEY', vision: false, tools: true, stream: true, context_window: 65536 },
    { id: 'deepseek-reasoner', format: 'openai', base_url: 'https://api.deepseek.com', model: 'deepseek-reasoner', api_key_env: 'DEEPSEEK_API_KEY', vision: false, tools: false, stream: true, context_window: 65536 },
    { id: 'gpt-4o', format: 'openai', base_url: 'https://api.openai.com/v1', model: 'gpt-4o', api_key_env: 'OPENAI_API_KEY', vision: true, tools: true, stream: true, context_window: 128000 },
    { id: 'claude', format: 'anthropic', base_url: 'https://api.anthropic.com', model: 'claude-3-5-sonnet-latest', api_key_env: 'ANTHROPIC_API_KEY', vision: true, tools: true, stream: true, context_window: 200000 },
    { id: 'llava', format: 'ollama', base_url: 'http://localhost:11434', model: 'llava', api_key_env: '', vision: true, tools: false, stream: true, context_window: 8192 },
  ];
  let models;
  try { models = JSON.parse(localStorage.getItem('dc-admin-models') || 'null') || MODEL_DEFAULTS; } catch (e) { models = MODEL_DEFAULTS; }
  const persistModels = () => { try { localStorage.setItem('dc-admin-models', JSON.stringify(models)); } catch (e) {} };
  const modelsList = () => models;
  const modelDefaults = () => {
    const chat = envValue('DEFAULT_CHAT_MODEL') || (models[0] && models[0].id) || '';
    const visEnv = envValue('DEFAULT_VISION_MODEL');
    const firstVis = models.find((m) => m.vision);
    const planEnv = envValue('DEFAULT_PLANNER_MODEL');
    return { chat, vision: visEnv || (firstVis ? firstVis.id : ''), visionExplicit: !!visEnv, planner: planEnv || chat, plannerExplicit: !!planEnv };
  };
  function saveModel(m, isNew) {
    if (!m.id || !m.id.trim()) return { error: 'id_required' };
    if (isNew && models.some((x) => x.id === m.id)) return { error: 'id_exists' };
    const i = models.findIndex((x) => x.id === m.id);
    if (i >= 0) models[i] = m; else models.push(m);
    persistModels(); emit(); return { ok: true };
  }
  function deleteModel(id) {
    const d = modelDefaults();
    if (id === d.chat || id === d.vision || id === d.planner) return { error: 'is_default' };
    models = models.filter((m) => m.id !== id); persistModels(); emit(); return { ok: true };
  }

  // ---------- oauth.yaml (GET/PUT /api/admin/oauth) ----------
  const OAUTH_DEFAULTS = [
    { name: 'google', preset: true, client_id: '', client_secret: '' },
    { name: 'github', preset: true, client_id: 'Iv1.8a61f9b3a7aba766', client_secret: 'gh_s3cret_demo' },
  ];
  let oauth;
  try { oauth = JSON.parse(localStorage.getItem('dc-admin-oauth') || 'null') || OAUTH_DEFAULTS; } catch (e) { oauth = OAUTH_DEFAULTS; }
  const persistOauth = () => { try { localStorage.setItem('dc-admin-oauth', JSON.stringify(oauth)); } catch (e) {} };
  const oauthList = () => oauth;
  function saveProvider(p, isNew) {
    if (!p.name || !/^[a-z][a-z0-9_-]*$/.test(p.name)) return { error: 'bad_name' };
    if (isNew && oauth.some((x) => x.name === p.name)) return { error: 'name_exists' };
    const i = oauth.findIndex((x) => x.name === p.name);
    if (i >= 0) oauth[i] = p; else oauth.push(p);
    persistOauth(); emit(); return { ok: true };
  }
  function deleteProvider(name) { oauth = oauth.filter((p) => p.name !== name || p.preset); persistOauth(); emit(); }

  // ---------- ai logs ----------
  const KINDS = ['companion', 'auto_plan', 'mood', 'inbox_classify', 'theme', 'vision_food', 'travel'];
  const KIND_LABEL = { companion: '陪伴聊天', auto_plan: '自主规划', mood: '心情', inbox_classify: '随手记', theme: '主题', vision_food: '食物识别', travel: '出行' };
  const MODELS = { companion: 'deepseek-v3.2', auto_plan: 'deepseek-v3.2', mood: 'deepseek-v3.2', inbox_classify: 'deepseek-v3.2', theme: 'deepseek-v3.2', vision_food: 'qwen-vl-plus', travel: 'deepseek-v3.2' };
  const REQ_SAMPLES = ['这周三前把 Problem Set 8 排进日程', '帮我看看明天有什么空档', '今天有点累，别安排太满', '午饭拍照：食堂三楼的番茄炒蛋套餐', '把每周二的羽毛球改到周四晚上', '下周去杭州两天，帮我排个行程', '想要一套莫兰迪色系的主题', '范围 2026-07-15 ~ 2026-07-17，保留手动块'];
  const RESP_SAMPLES = ['排好了：明天 14:00–15:30 留给 Problem Set 8，微积分课后脑子还热着。', '明天 15:00–17:00 有空档，放一节复习正合适。', '好的，今天只保留三件必须做的事，其余顺延到明天。', '识别到：番茄炒蛋、米饭、紫菜汤，约 620 kcal、蛋白质 24g。', '已把「羽毛球」规则改为每周四 19:00–20:30。', '行程草案已生成：D1 西湖环线，D2 灵隐寺 + 龙井村。', '已生成主题「雾青」，主色 #7a9e9f，可在预览中查看。', '三天计划已生成：优先了周三截止的两份作业。'];
  const SESSIONS = ['sess_9f2k1x', 'sess_a81m3q', 'sess_x22d0p', 'u_01HYX2', 'sess_b55n8r', 'u_01J2K8'];
  const ailogs = [];
  (function () {
    let t = 14;
    for (let i = 0; i < 36; i++) {
      const kind = KINDS[(i * 3 + (i > 17 ? 1 : 0)) % KINDS.length];
      const fail = i === 4 || i === 17 || i === 29;
      const tin = 420 + ((i * 97) % 940);
      const tout = fail ? 0 : 110 + ((i * 53) % 460);
      const s = REQ_SAMPLES[i % REQ_SAMPLES.length];
      ailogs.push({
        id: 'log_' + (9412 - i * 7), ts: ago(t), kind, model: MODELS[kind],
        tin, tout, dur: fail ? 800 + ((i * 77) % 700) : 1200 + ((i * 211) % 2900),
        status: fail ? 'failed' : 'ok', err: fail ? (i === 4 ? 'rate_limited' : 'parse_error') : null,
        sessionId: SESSIONS[i % SESSIONS.length],
        req: JSON.stringify({ kind, locale: 'zh-CN', timezone: 'Asia/Shanghai', message: s, tools: kind === 'companion' ? 11 : undefined }, null, 2),
        resp: fail
          ? JSON.stringify({ error: i === 4 ? 'rate_limited' : 'parse_error', message: i === 4 ? 'AI 请求过频，请稍后再试' : '模型输出无法解析为合法 JSON' }, null, 2)
          : JSON.stringify({ text: RESP_SAMPLES[i % RESP_SAMPLES.length], usage: { prompt_tokens: tin, completion_tokens: tout } }, null, 2),
      });
      t += 9 + ((i * 13) % 42);
    }
  })();

  // ---------- users ----------
  let users = [
    { id: 'u_01HYX2', name: '陈雨', email: 'yu.chen@stu.pku.edu.cn', kind: 'registered', sessions: 2, plans: 34, materials: 41, lastActive: ago(12), createdAt: '2026-05-03' },
    { id: 'u_01J2K8', name: 'Zoe Lin', email: 'zoe.lin@gmail.com', kind: 'registered', sessions: 1, plans: 19, materials: 22, lastActive: ago(95), createdAt: '2026-05-21' },
    { id: 'u_01J409', name: '王思远', email: 'siyuan.w@outlook.com', kind: 'registered', sessions: 3, plans: 27, materials: 18, lastActive: ago(60 * 26), createdAt: '2026-06-02' },
    { id: 'u_01JA77', name: '李慕', email: 'mu.li@qq.com', kind: 'registered', sessions: 1, plans: 8, materials: 12, lastActive: ago(60 * 49), createdAt: '2026-06-18' },
    { id: 'u_01JB12', name: 'Hana Sato', email: 'hana.s@icloud.com', kind: 'registered', sessions: 1, plans: 5, materials: 6, lastActive: ago(60 * 72), createdAt: '2026-06-30' },
    { id: 'u_01JC55', name: '赵一鸣', email: 'yiming.zhao@163.com', kind: 'registered', sessions: 1, plans: 2, materials: 3, lastActive: ago(60 * 5), createdAt: '2026-07-11' },
    { id: 'sess_9f2k1x', name: null, email: null, kind: 'anon', sessions: 1, plans: 11, materials: 9, lastActive: ago(8), createdAt: '2026-07-06' },
    { id: 'sess_a81m3q', name: null, email: null, kind: 'anon', sessions: 1, plans: 4, materials: 2, lastActive: ago(140), createdAt: '2026-07-12' },
    { id: 'sess_x22d0p', name: null, email: null, kind: 'anon', sessions: 1, plans: 1, materials: 0, lastActive: ago(60 * 30), createdAt: '2026-07-14' },
    { id: 'sess_b55n8r', name: null, email: null, kind: 'anon', sessions: 1, plans: 0, materials: 1, lastActive: ago(60 * 90), createdAt: '2026-07-08' },
  ];
  function deleteUser(id) { users = users.filter((u) => u.id !== id); stats.users = users.filter((u) => u.kind === 'registered').length; emit(); }

  // ---------- db ----------
  let tables = [
    { name: 'users', rows: 6 }, { name: 'sessions', rows: 14 }, { name: 'plans', rows: 87 }, { name: 'time_blocks', rows: 412 },
    { name: 'schedule_rules', rows: 23 }, { name: 'assignments', rows: 58 }, { name: 'materials', rows: 96 }, { name: 'memory_facts', rows: 41 },
    { name: 'custom_themes', rows: 7 }, { name: 'chat_threads', rows: 19 }, { name: 'chat_messages', rows: 486 }, { name: 'ai_logs', rows: 1284 },
    { name: 'import_history', rows: 32 }, { name: 'ops_log', rows: 210 }, { name: 'decision_cards', rows: 18 },
  ];
  const rowsByTable = {
    users: users.filter((u) => u.kind === 'registered').slice(0, 4).map((u) => ({ id: u.id, email: u.email, name: u.name, token_version: 1, created_at: u.createdAt })),
    sessions: [
      { id: 'sess_9f2k1x', user_id: null, assistant_name: '小禾', theme: 'sky', language: 'zh-CN', created_at: '2026-07-06 09:12' },
      { id: 'sess_kk20pa', user_id: 'u_01HYX2', assistant_name: '小禾', theme: 'night', language: 'zh-CN', created_at: '2026-05-03 20:41' },
      { id: 'sess_a81m3q', user_id: null, assistant_name: 'Momo', theme: 'ct_9d2f1', language: 'zh-CN', created_at: '2026-07-12 13:02' },
      { id: 'sess_m30q8d', user_id: 'u_01J2K8', assistant_name: 'Sage', theme: 'nature', language: 'en-US', created_at: '2026-05-21 08:55' },
    ],
    plans: [
      { id: 'pl_88a1', session_id: 'sess_kk20pa', date: '2026-07-15', source_type: 'auto', blocks: 7, note: '优先排了周三截止的两份作业' },
      { id: 'pl_88a0', session_id: 'sess_kk20pa', date: '2026-07-14', source_type: 'auto', blocks: 6, note: null },
      { id: 'pl_7f2c', session_id: 'sess_9f2k1x', date: '2026-07-15', source_type: 'text', blocks: 3, note: null },
      { id: 'pl_7e91', session_id: 'sess_m30q8d', date: '2026-07-13', source_type: 'rules', blocks: 4, note: null },
    ],
    time_blocks: [
      { id: 'b_2201', plan_id: 'pl_88a1', time: '09:00', title: '微积分 II', type: 'appointment', origin: 'rule', completed: true },
      { id: 'b_2202', plan_id: 'pl_88a1', time: '14:00', title: 'Problem Set 8', type: 'task', origin: 'auto', completed: false },
      { id: 'b_2203', plan_id: 'pl_88a1', time: '19:00', title: '羽毛球', type: 'relax', origin: 'rule', completed: false },
      { id: 'b_2204', plan_id: 'pl_7f2c', time: null, title: '取快递', type: 'task', origin: 'manual', completed: false },
    ],
    schedule_rules: [
      { id: 'r_501', session_id: 'sess_kk20pa', title: '微积分 II', freq: 'weekly', by_weekday: '[1,3]', time: '09:00', active: true },
      { id: 'r_502', session_id: 'sess_kk20pa', title: '羽毛球', freq: 'weekly', by_weekday: '[4]', time: '19:00', active: true },
      { id: 'r_503', session_id: 'sess_9f2k1x', title: '给绿萝浇水', freq: 'every_n_days', interval: 3, time: null, active: true },
      { id: 'r_504', session_id: 'sess_m30q8d', title: 'Gym', freq: 'weekly', by_weekday: '[2,5]', time: '07:30', active: false },
    ],
    assignments: [
      { id: 'a_301', session_id: 'sess_kk20pa', title: 'Problem Set 8', due_at: '2026-07-17 23:59', status: 'pending', source: 'canvas' },
      { id: 'a_302', session_id: 'sess_kk20pa', title: 'Lab 6：哈希表', due_at: '2026-07-18 12:00', status: 'pending', source: 'canvas' },
      { id: 'a_303', session_id: 'sess_9f2k1x', title: '英语演讲初稿', due_at: '2026-07-21 23:59', status: 'pending', source: 'manual' },
      { id: 'a_304', session_id: 'sess_m30q8d', title: 'Essay: Urban Ecology', due_at: '2026-07-10 23:59', status: 'done', source: 'canvas' },
    ],
    materials: [
      { id: 'm_901', session_id: 'sess_kk20pa', category: 'diet', title: '午餐：番茄炒蛋套餐', created_at: '2026-07-15 12:24' },
      { id: 'm_902', session_id: 'sess_kk20pa', category: 'academic', title: '数值分析补充讲义', created_at: '2026-07-14 16:02' },
      { id: 'm_903', session_id: 'sess_9f2k1x', category: 'health', title: '最近睡前容易头疼', created_at: '2026-07-13 23:41' },
      { id: 'm_904', session_id: 'sess_m30q8d', category: 'travel', title: 'Hangzhou weekend', created_at: '2026-07-12 10:15' },
    ],
    memory_facts: [
      { id: 'f_101', session_id: 'sess_kk20pa', content: '早上 8 点前不安排学习任务', source: 'chat', created_at: '2026-06-20 21:10' },
      { id: 'f_102', session_id: 'sess_kk20pa', content: '对花生过敏', source: 'inbox', created_at: '2026-06-28 12:33' },
      { id: 'f_103', session_id: 'sess_9f2k1x', content: '宿舍 23:30 熄灯', source: 'chat', created_at: '2026-07-07 22:05' },
      { id: 'f_104', session_id: 'sess_m30q8d', content: 'Prefers gym before class', source: 'chat', created_at: '2026-06-01 09:40' },
    ],
    custom_themes: [
      { id: 'ct_9d2f1', session_id: 'sess_a81m3q', name: '雾青', base: 'sky', dark: false, vars: '12 vars' },
      { id: 'ct_8c110', session_id: 'sess_kk20pa', name: '暮山紫', base: 'night', dark: true, vars: '12 vars' },
      { id: 'ct_77e02', session_id: 'sess_m30q8d', name: 'Matcha', base: 'nature', dark: false, vars: '12 vars' },
    ],
    chat_threads: [
      { id: 't_61', session_id: 'sess_kk20pa', title: '这周的作业怎么排', message_count: 14, updated_at: '2026-07-15 10:22' },
      { id: 't_62', session_id: 'sess_9f2k1x', title: '考前焦虑', message_count: 22, updated_at: '2026-07-14 23:05' },
      { id: 't_63', session_id: 'sess_m30q8d', title: 'Hangzhou trip', message_count: 9, updated_at: '2026-07-12 11:30' },
    ],
    chat_messages: [
      { id: 'msg_8801', thread_id: 't_61', role: 'user', status: 'done', content: '这周三前把 Problem Set 8 排进日程', created_at: '2026-07-15 10:20' },
      { id: 'msg_8802', thread_id: 't_61', role: 'assistant', status: 'done', content: '排好了：明天 14:00–15:30 留给 Problem Set 8…', created_at: '2026-07-15 10:21' },
      { id: 'msg_8803', thread_id: 't_62', role: 'assistant', status: 'pending', content: '', created_at: '2026-07-15 11:02' },
    ],
    ai_logs: ailogs.slice(0, 4).map((l) => ({ id: l.id, session_id: l.sessionId, kind: l.kind, model: l.model, tokens_in: l.tin, tokens_out: l.tout, duration_ms: l.dur, status: l.status })),
    import_history: [
      { id: 'ih_21', session_id: 'sess_kk20pa', source: 'canvas', items: 18, created_at: '2026-07-10 09:00' },
      { id: 'ih_22', session_id: 'sess_kk20pa', source: 'ics', items: 12, created_at: '2026-07-02 14:20' },
      { id: 'ih_23', session_id: 'sess_9f2k1x', source: 'timetable_image', items: 9, created_at: '2026-07-06 09:30' },
    ],
    ops_log: [
      { id: 'op_991', session_id: 'sess_kk20pa', tool: 'plan_update', target_id: 'b_2202', summary: '更新了明天的计划', status: 'ok', created_at: '2026-07-15 10:21' },
      { id: 'op_992', session_id: 'sess_kk20pa', tool: 'rule_upsert', target_id: 'r_502', summary: '羽毛球改到周四 19:00', status: 'ok', created_at: '2026-07-14 20:11' },
      { id: 'op_993', session_id: 'sess_9f2k1x', tool: 'memory_add', target_id: 'f_103', summary: '记住：宿舍 23:30 熄灯', status: 'ok', created_at: '2026-07-07 22:05' },
    ],
    decision_cards: [
      { id: 'dc_71', session_id: 'sess_kk20pa', title: '明晚 19:00 有两件事', status: 'answered', choice: 'move_lab', created_at: '2026-07-14 18:40' },
      { id: 'dc_72', session_id: 'sess_9f2k1x', title: '空档要不要放复习', status: 'timeout', choice: 'timeout', created_at: '2026-07-13 15:02' },
    ],
  };
  const tableRows = (name) => rowsByTable[name] || [];
  function deleteRow(name, id) {
    if (!rowsByTable[name]) return;
    rowsByTable[name] = rowsByTable[name].filter((r) => r.id !== id);
    const t = tables.find((x) => x.name === name); if (t && t.rows > 0) t.rows--;
    emit();
  }

  // ---------- export / backup / import ----------
  function download(filename, text, type) {
    const blob = new Blob([text], { type: type || 'application/json' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob); a.download = filename;
    document.body.appendChild(a); a.click();
    setTimeout(() => { URL.revokeObjectURL(a.href); a.remove(); }, 400);
  }
  const stamp = () => { const d = new Date(); return `${d.getFullYear()}${p2(d.getMonth() + 1)}${p2(d.getDate())}-${p2(d.getHours())}${p2(d.getMinutes())}`; };
  const exportAll = () => download('daycore-export-' + stamp() + '.json', JSON.stringify({ exportVersion: 2, exportedAt: new Date().toISOString(), tables: rowsByTable }, null, 2));
  const backup = () => download('daycore-backup-' + stamp() + '.db', JSON.stringify({ format: 'sqlite-serialized-demo', pages: 412, tables: tables.map((t) => t.name) }), 'application/octet-stream');
  function importJson(file, cb) {
    const rd = new FileReader();
    rd.onload = () => {
      try { const j = JSON.parse(rd.result); cb({ ok: true, tables: Object.keys(j.tables || j).length }); }
      catch (e) { cb({ error: 'bad_json' }); }
    };
    rd.readAsText(file);
  }

  return {
    subscribe, get, emit,
    DEMO_TOKEN, isAuthed, login, logout,
    health, versionInfo, stats,
    prompts, getPrompt, savePrompt, resetPrompt, isModified, promptPlaceholders,
    envGroups, envDef, envValue, envIsSet, setEnvMany,
    modelsList, modelDefaults, saveModel, deleteModel,
    oauthList, saveProvider, deleteProvider,
    ailogs, KINDS, KIND_LABEL,
    usersList: () => users, deleteUser,
    tablesList: () => tables, tableRows, deleteRow,
    exportAll, backup, importJson,
    fmtT, relT,
  };
})();
