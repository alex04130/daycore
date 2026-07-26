/* DcStore admin — mock of the v4 admin API (mirrors api/openapi-v4.yaml shapes) */
(function(){
const S=window.DcStore;const st=()=>S._st();const uid=S.uid;
const wait=ms=>new Promise(r=>setTimeout(r,ms+Math.random()*ms*.3));
const err=(code,msg)=>Promise.reject({error:code,message:msg||code});
const MASK='••••••••';
const SECRET=k=>/(_API_KEY|_SECRET)$/.test(k)||['JWT_SECRET','COOKIE_SECRET','PASSWORD_PEPPER','ADMIN_TOKEN','ONEBOT_TOKEN'].includes(k);
const HOT=['AI_RATE_LIMIT_PER_MIN','TAVILY_API_KEY','QWEATHER_API_KEY','OPENWEATHERMAP_API_KEY','WEATHER_PROVIDER'];
const E=(key,group,label,description,type,def,extra)=>Object.assign({key,group,label,description,type,options:null,def,raw:null,envLock:false,restartRequired:!HOT.includes(key)},extra||{});

function seedAdmin(){
 const t=S.todayIso();const ad=S.addDays;
 const env=[
  E('APP_ENV','server','运行环境','development 下有 dev fail-safe（仅 loopback、弱密钥告警）','select','development',{options:['development','production']}),
  E('HOST','server','监听地址','留空 = 监听全部网卡；nginx 反代时建议 127.0.0.1','string',''),
  E('PORT','server','端口','HTTP 监听端口','int','8080',{raw:'8080'}),
  E('STATIC_DIR','server','静态目录','Go 直接服务的前端构建产物目录（SPA fallback）','string','web/frontend/dist'),
  E('PUBLIC_BASE_URL','server','公开地址','用于拼 OAuth 回调等绝对地址','string','http://localhost:8080',{raw:'http://localhost:8080'}),
  E('ALLOWED_ORIGINS','server','CORS 白名单','逗号分隔；同源部署留空','string',''),
  E('DB_TYPE','database','数据库类型','','select','sqlite',{options:['sqlite','postgres','mysql','mongodb'],raw:'sqlite'}),
  E('DB_DSN','database','连接串','按 DB_TYPE 语法填写','string','file:daycore.db?_pragma=busy_timeout(5000)',{raw:'file:daycore.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)'}),
  E('JWT_SECRET','auth','JWT 密钥','生产环境必填强随机值','string','',{raw:'k9…demo'}),
  E('COOKIE_SECRET','auth','Cookie 密钥','','string','',{raw:'c3…demo'}),
  E('PASSWORD_PEPPER','auth','密码 Pepper','可选的服务端附加盐','string',''),
  E('JWT_TTL','auth','登录有效期','Go duration，如 720h','duration','720h'),
  E('SECURE_COOKIES','auth','Secure Cookie','HTTPS 部署时开启','bool','false'),
  E('ADMIN_EMAILS','auth','管理员邮箱','逗号分隔；注册时命中自动授予 admin','string','',{raw:'admin@daycore.app'}),
  E('DEFAULT_CHAT_MODEL','ai','默认对话模型','必须是模型目录中的 id','string','deepseek-chat',{raw:'deepseek-chat'}),
  E('DEFAULT_VISION_MODEL','ai','默认视觉模型','留空 = 自动选第一个支持视觉的模型','string',''),
  E('DEFAULT_PLANNER_MODEL','ai','默认规划模型','自主规划所用；留空 = 用对话模型','string',''),
  E('AI_REQUEST_TIMEOUT','ai','AI 请求超时','','duration','120s'),
  E('AGENT_MAX_ROUNDS','ai','Agent 最大轮数','陪伴 agent 单条消息的工具调用轮数上限','int','6'),
  E('DEEPSEEK_API_KEY','ai','DeepSeek API Key','','string','',{raw:'sk-demo'}),
  E('OPENAI_API_KEY','ai','OpenAI API Key','','string',''),
  E('ANTHROPIC_API_KEY','ai','Anthropic API Key','','string',''),
  E('TAVILY_API_KEY','ai','Tavily 搜索 Key','陪伴联网搜索；缺省回退 DuckDuckGo','string',''),
  E('AI_RATE_LIMIT_PER_MIN','limits','AI 限速 / 分钟','热应用，保存立即生效','int','30',{raw:'30'}),
  E('MAX_IMAGE_BYTES','limits','图片大小上限','字节','int','8388608'),
  E('ADMIN_TOKEN','limits','管理 Token','旧版 X-Admin-Token 轨；留空则仅 JWT 轨可用','string',''),
  E('AUTO_PLAN_MAX_DAYS','limits','自主规划最大天数','单次 /api/ai/auto-plan 覆盖上限','int','7',{raw:'7'}),
  E('ASSIGNMENT_LOOKAHEAD_DAYS','limits','作业前瞻天数','AI 上下文里「临近作业」的范围','int','14'),
  E('WEATHER_PROVIDER','weather','天气源','','select','open-meteo',{options:['open-meteo','qweather','openweathermap','wttrin']}),
  E('QWEATHER_API_KEY','weather','和风天气 Key','','string',''),
  E('OPENWEATHERMAP_API_KEY','weather','OpenWeatherMap Key','','string',''),
  E('ONEBOT_WS_URL','channels','OneBot WS 地址','QQ 通道；留空关闭','string',''),
  E('ONEBOT_TOKEN','channels','OneBot Token','','string',''),
  E('WORKER_DEFAULT_TZ','channels','推送默认时区','','string','Asia/Shanghai')
 ];
 const logs=[];const users=['demo@daycore.app','kite@stu.pku.edu.cn','mo@outlook.com','admin@daycore.app'];const kinds=['chat','chat','auto-plan','mood','chat','vision'];
 for(let i=0;i<26;i++){const d=new Date(Date.now()-i*137*60000);logs.push({id:'lg'+i,ts:d.toISOString(),user:users[i%4],model:i%5===3?'deepseek-reasoner':'deepseek-chat',kind:kinds[i%6],latencyMs:400+((i*997)%2400),tokensIn:220+((i*313)%1800),tokensOut:80+((i*211)%900),status:i%9===7?'error':'ok',preview:i%9===7?'upstream timeout after 120s':['帮我把今晚排松一点','为 3 天生成计划','今天有点累','识别课程表截图','明天的安排是什么','把运动挪到傍晚'][i%6]})}
 return{
  startedAt:Date.now()-1000*60*60*13-1000*60*22,needsRestart:false,restartKeys:[],env,
  models:{models:[
   {id:'deepseek-chat',format:'openai',baseUrl:'https://api.deepseek.com',model:'deepseek-chat',apiKeyEnv:'DEEPSEEK_API_KEY',vision:false,tools:true,stream:true,contextWindow:65536,enabled:true},
   {id:'deepseek-reasoner',format:'openai',baseUrl:'https://api.deepseek.com',model:'deepseek-reasoner',apiKeyEnv:'DEEPSEEK_API_KEY',vision:false,tools:false,stream:true,contextWindow:65536,enabled:true},
   {id:'gpt-4o',format:'openai',baseUrl:'https://api.openai.com/v1',model:'gpt-4o',apiKeyEnv:'OPENAI_API_KEY',vision:true,tools:true,stream:true,contextWindow:128000,enabled:true},
   {id:'claude',format:'anthropic',baseUrl:'https://api.anthropic.com',model:'claude-3-5-sonnet-latest',apiKeyEnv:'ANTHROPIC_API_KEY',vision:true,tools:true,stream:true,contextWindow:200000,enabled:false},
   {id:'llava',format:'ollama',baseUrl:'http://localhost:11434',model:'llava',apiKeyEnv:'',vision:true,tools:false,stream:true,contextWindow:8192,enabled:false}],
   defaults:{chat:'deepseek-chat',vision:'',planner:'deepseek-reasoner'},requestTimeout:'120s',agentMaxRounds:6},
  oauth:{providers:[
   {name:'google',preset:true,enabled:false,clientId:'',clientSecret:'',scopes:['openid','email','profile'],authUrl:'',tokenUrl:'',userinfoUrl:'',idField:'',emailField:'',nameField:'',avatarField:''},
   {name:'github',preset:true,enabled:true,clientId:'Iv1.8f24aa01d3b2c977',clientSecret:'ghp_secret_demo',scopes:['read:user','user:email'],authUrl:'',tokenUrl:'',userinfoUrl:'',idField:'',emailField:'',nameField:'',avatarField:''}]},
  prompts:[
   {key:'companion.system',label:'陪伴 · 系统提示词',updatedAt:ad(t,-6),body:'你是 {assistant_name}，用户的日常陪伴助理。语气温暖、简短、不说教。你可以读取用户的课表、作业、规则与计划；当用户希望调整安排时，输出 <plan_update> 决策卡而不是直接改动。尊重用户规则（rules），冲突时先询问。'},
   {key:'planner.system',label:'自主规划 · 系统提示词',updatedAt:ad(t,-6),body:'你是日程规划器。输入：日期范围、课表、作业（含截止）、用户规则、偏好（作息/专注时段）与自由指令。输出 JSON 时间块数组。原则：先固定课程，再安排临近截止的作业推进（≤50 分钟/节），保留三餐与休息；mode=keep 时不得移动 source=manual 或 locked 的块。'},
   {key:'mood.reply',label:'心情 · 回应提示词',updatedAt:ad(t,-12),body:'用户完成一次心情打卡。用 1-2 句温暖、具体、不评判的话回应；若情绪低落，轻轻提及可用的呼吸/着地练习，不强推。'},
   {key:'inbox.parse',label:'随手记 · 解析提示词',updatedAt:ad(t,-12),body:'把用户的随手一句话解析为结构化对象：assignment（含 title/due/course）、block（date/start/end/title）或 memory。无法确定日期时给出最可能猜测并标注 confidence。'}],
  ailogs:logs,
  audit:[{id:uid(),ts:new Date(Date.now()-864e5).toISOString(),actor:'admin@daycore.app',action:'models_put',target:'defaults.planner → deepseek-reasoner',result:'ok'}],
  userMeta:{u_demo:{lastActiveAt:new Date(Date.now()-36e5).toISOString(),sessionCount:5},u_admin:{lastActiveAt:new Date().toISOString(),sessionCount:2},u_kite:{lastActiveAt:ad(t,-2)+'T10:11:00Z',sessionCount:1},u_mo:{lastActiveAt:null,sessionCount:0}}
 };
}
if(!st().admin){st().admin=seedAdmin();S.save()}
const A=()=>st().admin;

function guard(){const u=st().session.userId&&st().users.find(x=>x.id===st().session.userId);if(!u)return{error:'unauthorized',message:'sign in first'};if(u.role!=='admin')return{error:'forbidden',message:'admins only'};return null}
function audit(action,target,result){A().audit.unshift({id:uid(),ts:new Date().toISOString(),actor:(st().users.find(u=>u.id===st().session.userId)||{}).email||'admin-token',action,target,result:result||'ok'});A().audit=A().audit.slice(0,60);S.save()}
const isDur=v=>/^([0-9]+(\.[0-9]+)?(ns|us|ms|s|m|h))+$/.test(v);
const envView=e=>({key:e.key,group:e.group,label:e.label,description:e.description,type:e.type,options:e.options,isSecret:SECRET(e.key),isSet:e.raw!=null&&e.raw!=='',source:e.envLock?'env':(e.raw!=null&&e.raw!==''?'file':'default'),editable:!e.envLock,restartRequired:e.restartRequired,value:SECRET(e.key)?(e.raw?MASK:''):(e.raw!=null&&e.raw!==''?e.raw:e.def)});
const maskId=s=>!s?'':(s.length<8?'••••':s.slice(0,4)+'…'+s.slice(-4));
const keySet=envKey=>{if(!envKey)return true;const e=A().env.find(x=>x.key===envKey);return!!(e&&e.raw)};

Object.assign(S,{
 async adminSystem(){await wait(150);const g=guard();if(g)return Promise.reject(g);const a=A();const t=st();
  const val=k=>{const e=a.env.find(x=>x.key===k);return e?(e.raw||e.def):''};
  return{version:'4.0.0-beta',channel:'beta',apiVersion:4,apiMinor:0,goVersion:'go1.24.3',env:val('APP_ENV'),uptimeSec:Math.floor((Date.now()-a.startedAt)/1000),dbType:val('DB_TYPE'),dbSizeBytes:1327104+t.assignments.length*4096,staticDir:val('STATIC_DIR'),publicBaseUrl:val('PUBLIC_BASE_URL'),configPaths:{env:'.env',models:'config/models.yaml',oauth:'config/oauth.yaml'},needsRestart:a.needsRestart,restartKeys:[...a.restartKeys],counts:{users:t.users.length,sessions:8,materials:t.courses.length+t.assignments.length+t.imports.length,assignments:t.assignments.length}}},
 async adminGetEnv(){await wait(200);const g=guard();if(g)return Promise.reject(g);return{entries:A().env.map(envView)}},
 async adminPutEnv(entries){await wait(500);const g=guard();if(g)return Promise.reject(g);const a=A();
  for(const k in entries){const e=a.env.find(x=>x.key===k);if(!e)return err('unknown_key','未知键：'+k);if(e.envLock)return err('invalid_value',k+' 被部署环境锁定');const v=String(entries[k]);
   if(v!==''){if(e.type==='int'&&!/^\d+$/.test(v))return err('invalid_value',k+' 必须是整数');if(k==='PORT'&&(+v<1||+v>65535))return err('invalid_value','PORT 必须在 1-65535');if(e.type==='duration'&&!isDur(v))return err('invalid_value',k+' 不是合法 duration（如 120s / 720h）');if(e.type==='bool'&&!['true','false'].includes(v))return err('invalid_value',k+' 必须是 true/false');if(e.type==='select'&&!e.options.includes(v))return err('invalid_value',k+' 必须是 '+e.options.join(' / '))}}
  const applied=[];const hot=[];
  for(const k in entries){const e=a.env.find(x=>x.key===k);const v=String(entries[k]);e.raw=v===''?null:v;applied.push(k);if(HOT.includes(k))hot.push(k);else if(!a.restartKeys.includes(k))a.restartKeys.push(k)}
  a.needsRestart=a.restartKeys.length>0;S.save();audit('env_put',applied.join(', '));
  return{applied,needsRestart:a.needsRestart,restartKeys:[...a.restartKeys]}},
 async adminGetModels(){await wait(200);const g=guard();if(g)return Promise.reject(g);const m=A().models;return{models:m.models.map(x=>({...x,apiKeySet:keySet(x.apiKeyEnv)})),defaults:{...m.defaults},requestTimeout:m.requestTimeout,agentMaxRounds:m.agentMaxRounds}},
 async adminPutModels(doc){await wait(600);const g=guard();if(g)return Promise.reject(g);
  const ids=new Set();const warnings=[];
  if(!doc.models||!doc.models.length)return err('invalid_models','至少保留一个模型');
  for(const m of doc.models){if(!m.id||!/^[a-z0-9._-]+$/.test(m.id))return err('invalid_models','模型 id 不合法：'+(m.id||'(空)'));if(ids.has(m.id))return err('invalid_models','模型 id 重复：'+m.id);ids.add(m.id);
   if(!['openai','anthropic','ollama'].includes(m.format))return err('invalid_models',m.id+'：未注册的 format '+m.format);
   if(m.format!=='ollama'&&!m.apiKeyEnv)return err('invalid_models',m.id+'：远程格式必须指定 apiKeyEnv');
   if(m.apiKeyEnv&&!keySet(m.apiKeyEnv))warnings.push(m.id+'：环境变量 '+m.apiKeyEnv+' 尚未设置')}
  const chat=doc.defaults&&doc.defaults.chat;const ce=doc.models.find(m=>m.id===chat);
  if(!chat||!ce)return err('invalid_models','defaults.chat 必须指向已存在的模型');
  if(!ce.enabled)return err('invalid_models','defaults.chat 指向的模型未启用');
  for(const dk of['vision','planner']){const v=doc.defaults[dk];if(v&&!doc.models.find(m=>m.id===v))return err('invalid_models','defaults.'+dk+' 指向不存在的模型')}
  if(!isDur(doc.requestTimeout||''))return err('invalid_models','requestTimeout 不是合法 duration');
  if(!(doc.agentMaxRounds>=1))return err('invalid_models','agentMaxRounds 需 ≥ 1');
  A().models={models:doc.models.map(({apiKeySet,...m})=>m),defaults:{...doc.defaults},requestTimeout:doc.requestTimeout,agentMaxRounds:doc.agentMaxRounds};S.save();audit('models_put',doc.models.length+' models; chat='+chat);
  return{...await S.adminGetModels(),warnings}},
 async adminTestModel(x){await wait(300);const g=guard();if(g)return Promise.reject(g);
  let m=null;if(x.id){m=A().models.models.find(e=>e.id===x.id);if(!m)return err('not_found','未知模型 id')}else if(x.config){m=x.config;if(!m.baseUrl||!/^https?:\/\//.test(m.baseUrl))return{ok:false,latencyMs:12,reply:'',error:'baseUrl 不是合法 URL',errorCode:'bad_config'}}else return err('invalid_value','需要 id 或 config');
  const lat=500+Math.floor(Math.random()*900);await wait(lat);
  if(m.format==='ollama')return{ok:false,latencyMs:3000+lat,reply:'',error:'connect ECONNREFUSED '+m.baseUrl,errorCode:'network'};
  if(!keySet(m.apiKeyEnv))return{ok:false,latencyMs:lat,reply:'',error:'401 Unauthorized from upstream',errorCode:'auth'};
  return{ok:true,latencyMs:lat,reply:'pong',error:'',errorCode:''}},
 async adminGetOauth(){await wait(200);const g=guard();if(g)return Promise.reject(g);const base=(A().env.find(e=>e.key==='PUBLIC_BASE_URL')||{}).raw||'http://localhost:8080';
  return{providers:A().oauth.providers.map(p=>({name:p.name,preset:p.preset,enabled:p.enabled,clientId:maskId(p.clientId),clientSecretSet:!!p.clientSecret,callbackUrl:base+'/api/auth/oauth/'+p.name+'/callback',scopes:[...(p.scopes||[])],authUrl:p.preset?'':p.authUrl,tokenUrl:p.preset?'':p.tokenUrl,userinfoUrl:p.preset?'':p.userinfoUrl,idField:p.preset?'':p.idField,emailField:p.preset?'':p.emailField,nameField:p.preset?'':p.nameField,avatarField:p.preset?'':p.avatarField}))}},
 async adminPutOauth(doc){await wait(500);const g=guard();if(g)return Promise.reject(g);const names=new Set();
  for(const p of doc.providers){if(!p.name||!/^[a-z0-9-]+$/.test(p.name))return err('invalid_oauth','提供商名称必须是小写 slug');if(names.has(p.name))return err('invalid_oauth','提供商重名：'+p.name);names.add(p.name);
   const preset=['google','github'].includes(p.name);
   if(p.enabled){if(!p.clientId)return err('invalid_oauth',p.name+'：clientId 必填');
    if(!preset&&(!p.authUrl||!p.tokenUrl||!p.userinfoUrl||!p.idField||!p.emailField||!p.nameField||!p.avatarField))return err('invalid_oauth',p.name+'：自定义提供商必须补全 URL 与字段映射')}}
  const old=A().oauth.providers;
  A().oauth.providers=doc.providers.map(p=>{const prev=old.find(o=>o.name===p.name);const preset=['google','github'].includes(p.name);
   return{name:p.name,preset,enabled:!!p.enabled,clientId:p.clientId&&p.clientId.includes('…')?(prev?prev.clientId:''):(p.clientId||''),clientSecret:p.clientSecret===''?(prev?prev.clientSecret:''):(p.clientSecret||''),scopes:p.scopes&&p.scopes.length?p.scopes:(prev?prev.scopes:[]),authUrl:p.authUrl||'',tokenUrl:p.tokenUrl||'',userinfoUrl:p.userinfoUrl||'',idField:p.idField||'',emailField:p.emailField||'',nameField:p.nameField||'',avatarField:p.avatarField||''}});
  S.save();audit('oauth_put',doc.providers.map(p=>p.name+(p.enabled?'(on)':'(off)')).join(', '));
  return S.adminGetOauth()},
 async adminUsers(){await wait(200);const g=guard();if(g)return Promise.reject(g);const meta=A().userMeta;
  return{users:st().users.map(u=>({id:u.id,email:u.email,name:u.name,role:u.role,createdAt:u.createdAt,lastActiveAt:(meta[u.id]||{}).lastActiveAt||null,sessionCount:(meta[u.id]||{}).sessionCount||0}))}},
 async adminPatchUser(id,patch){await wait(300);const g=guard();if(g)return Promise.reject(g);const u=st().users.find(x=>x.id===id);if(!u)return err('not_found');
  if(patch.role&&patch.role!==u.role){if(u.role==='admin'&&st().users.filter(x=>x.role==='admin').length<=1)return err('last_admin');u.role=patch.role}
  S.save();audit('user_patch',u.email+' → '+u.role);return{user:{id:u.id,email:u.email,name:u.name,role:u.role,createdAt:u.createdAt}}},
 async adminDeleteUser(id){await wait(300);const g=guard();if(g)return Promise.reject(g);const u=st().users.find(x=>x.id===id);if(!u)return err('not_found');
  if(u.id===st().session.userId)return err('self_delete');
  if(u.role==='admin'&&st().users.filter(x=>x.role==='admin').length<=1)return err('last_admin');
  st().users=st().users.filter(x=>x.id!==id);S.save();audit('user_delete',u.email);return{}},
 async adminPrompts(){await wait(150);const g=guard();if(g)return Promise.reject(g);return{prompts:JSON.parse(JSON.stringify(A().prompts))}},
 async adminSetPrompt(key,body){await wait(300);const g=guard();if(g)return Promise.reject(g);const p=A().prompts.find(x=>x.key===key);if(!p)return err('not_found');p.body=body;p.updatedAt=S.todayIso();S.save();audit('prompt_set',key);return{}},
 async adminStats(){await wait(250);const g=guard();if(g)return Promise.reject(g);const days=[];for(let i=6;i>=0;i--){const d=S.addDays(S.todayIso(),-i);days.push({date:d,messages:3+((i*7+3)%14),plans:i%2===0?2:1,moods:(i*5+2)%4})}
  return{series:days,models:[{id:'deepseek-chat',calls:186,tokens:412000},{id:'deepseek-reasoner',calls:42,tokens:158000},{id:'gpt-4o',calls:9,tokens:31000}]}},
 async adminAILogs(){await wait(250);const g=guard();if(g)return Promise.reject(g);return{logs:JSON.parse(JSON.stringify(A().ailogs))}},
 async adminAudit(){await wait(150);const g=guard();if(g)return Promise.reject(g);return{audit:JSON.parse(JSON.stringify(A().audit))}},
 async adminDBTables(){await wait(200);const g=guard();if(g)return Promise.reject(g);const t=st();
  return{tables:[{name:'users',rows:t.users.length,bytes:16384},{name:'sessions',rows:8,bytes:8192},{name:'courses',rows:t.courses.length,bytes:12288},{name:'assignments',rows:t.assignments.length,bytes:20480},{name:'plan_blocks',rows:Object.values(t.plan).reduce((s,a)=>s+a.length,0),bytes:24576},{name:'messages',rows:t.threads.reduce((s,x)=>s+x.messages.length,0),bytes:40960},{name:'moods',rows:t.moods.length,bytes:8192},{name:'memory',rows:t.memory.length,bytes:4096},{name:'admin_audit',rows:A().audit.length,bytes:4096}]}},
 async adminDBBrowse(name){await wait(250);const g=guard();if(g)return Promise.reject(g);const t=st();
  const V={users:()=>({columns:['id','email','name','role','created_at'],rows:t.users.map(u=>[u.id,u.email,u.name,u.role,u.createdAt])}),
   courses:()=>({columns:['id','name','day','start','end','room'],rows:t.courses.map(c=>[c.id,c.name,c.day,c.start,c.end,c.room])}),
   assignments:()=>({columns:['id','title','due','status','source'],rows:t.assignments.map(a=>[a.id,a.title,a.due,a.status,a.source])}),
   plan_blocks:()=>({columns:['id','date','start','end','title','type','source'],rows:[].concat(...Object.values(t.plan).map(arr=>arr.map(b=>[b.id,b.date,b.start,b.end,typeof b.title==='string'?b.title:'',b.type,b.source]))).slice(0,14)}),
   moods:()=>({columns:['id','date','mood','note'],rows:t.moods.map(m=>[m.id,m.date,m.mood,m.note])}),
   memory:()=>({columns:['id','text','source','created_at'],rows:t.memory.map(m=>[m.id,m.text,m.source,m.createdAt])}),
   admin_audit:()=>({columns:['ts','actor','action','target','result'],rows:A().audit.map(a=>[a.ts,a.actor,a.action,a.target,a.result])}),
   messages:()=>({columns:['id','thread','role','content'],rows:[].concat(...t.threads.map(th=>th.messages.map(m=>[m.id,th.id,m.role,String(m.content).slice(0,40)]))).slice(0,14)}),
   sessions:()=>({columns:['id','user','created_at','last_seen'],rows:[['s1','u_admin',S.addDays(S.todayIso(),-1),'today'],['s2','u_demo',S.addDays(S.todayIso(),-2),S.addDays(S.todayIso(),-1)]]})};
  return(V[name]||V.users)()},
 async adminDBBackup(){await wait(700);const g=guard();if(g)return Promise.reject(g);audit('db_backup','all tables');return{blob:S.exportData(),filename:'daycore-backup-'+S.todayIso()+'.json'}},
 async adminClearRestart(){const g=guard();if(g)return Promise.reject(g);A().needsRestart=false;A().restartKeys=[];S.save();return{}}
});
})();
