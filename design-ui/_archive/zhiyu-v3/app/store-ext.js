/* DcStore extensions — inbox / imports / companion stream / mood / themes / memory / data */
(function(){
const S=window.DcStore;const st=()=>S._st();const uid=S.uid;
const wait=ms=>new Promise(r=>setTimeout(r,ms));
const nowIso=()=>new Date().toISOString();
const err=(code,msg)=>Promise.reject({error:code,message:msg||code});

/* ── memory ── */
Object.assign(S,{
 async listMemory(){await wait(60);return JSON.parse(JSON.stringify(st().memory))},
 async addMemory(text,source){await wait(90);st().memory.unshift({id:uid(),text,createdAt:S.todayIso(),source:source||'user'});S.save();return{}},
 async deleteMemory(id){await wait(70);st().memory=st().memory.filter(m=>m.id!==id);S.save();return{}}
});

/* ── inbox (quick capture) ── */
function parseCapture(text){
 const t=text.trim();
 const due=(()=>{const m=t.match(/(\d{1,2})月(\d{1,2})[日号]/);if(m){const d=new Date();d.setMonth(+m[1]-1,+m[2]);return S.iso(d)}
  if(/明天/.test(t))return S.addDays(S.todayIso(),1);if(/后天/.test(t))return S.addDays(S.todayIso(),2);
  const w=t.match(/下?周([一二三四五六日天])/);if(w){const map={'一':1,'二':2,'三':3,'四':4,'五':5,'六':6,'日':7,'天':7};const target=map[w[1]];let d=S.todayIso();const base=S.wd(d);let diff=target-base;if(/下周/.test(t))diff+=7;else if(diff<=0)diff+=7;return S.addDays(d,diff)}
  return null})();
 const time=t.match(/(\d{1,2})[:：](\d{2})/);
 if(/交|提交|截止|作业|due|report|论文|申请/.test(t))return{kind:'assignment',payload:{title:t.replace(/\s+/g,' ').slice(0,40),due:due||S.addDays(S.todayIso(),3),course:'',notes:t}};
 if(time&&due)return{kind:'block',payload:{date:due,start:time[1].padStart(2,'0')+':'+time[2],end:(String(+time[1]+1)).padStart(2,'0')+':'+time[2],title:t.slice(0,30),type:'custom'}};
 return{kind:'memory',payload:{text:t}};
}
Object.assign(S,{
 async listInbox(){await wait(60);return JSON.parse(JSON.stringify(st().inbox))},
 async quickCapture(text){await wait(700);const item={id:uid(),text,createdAt:nowIso(),status:'pending',parsed:parseCapture(text)};st().inbox.unshift(item);S.save();return JSON.parse(JSON.stringify(item))},
 async confirmInbox(id,edited){await wait(200);const it=st().inbox.find(x=>x.id===id);if(!it)return err('not_found');const p=edited||it.parsed;
  if(p.kind==='assignment')st().assignments.push({id:uid(),title:p.payload.title,course:p.payload.course||'',due:p.payload.due,status:'todo',notes:p.payload.notes||'',source:'capture',createdAt:S.todayIso()});
  else if(p.kind==='block')await S.upsertBlock({...p.payload,id:uid(),source:'manual',done:false,note:''});
  else st().memory.unshift({id:uid(),text:p.payload.text,createdAt:S.todayIso(),source:'capture'});
  it.status='confirmed';it.parsed=p;S.save();return{kind:p.kind}},
 async dismissInbox(id){await wait(80);const it=st().inbox.find(x=>x.id===id);if(it)it.status='dismissed';S.save();return{}}
});

/* ── imports ── */
function logImport(kind,summary,count){st().imports.unshift({id:uid(),kind,summary,count,createdAt:nowIso()});S.save()}
Object.assign(S,{
 async listImports(){await wait(60);return JSON.parse(JSON.stringify(st().imports))},
 async importCanvas(jsonText){await wait(900);let doc;try{doc=JSON.parse(jsonText)}catch(e){return err('invalid_value','JSON 无法解析')}
  const arr=Array.isArray(doc)?doc:(doc.assignments||[]);if(!arr.length)return err('invalid_value','没有找到 assignments 数组');
  let added=0,skipped=0;arr.forEach(x=>{const title=(x.course?x.course+' · ':'')+(x.name||x.title||'未命名作业');if(st().assignments.some(a=>a.title===title)){skipped++;return}const due=(x.due_at||x.due||'').slice(0,10)||S.addDays(S.todayIso(),7);st().assignments.push({id:uid(),title,course:x.course||'',due,status:'todo',notes:x.description||'',source:'canvas',createdAt:S.todayIso()});added++});
  logImport('canvas','Canvas 作业 '+arr.length+' 条（'+skipped+' 条已存在，跳过）',added);return{added,skipped}},
 async importICS(text){await wait(900);const events=[];const re=/BEGIN:VEVENT([\s\S]*?)END:VEVENT/g;let m;while((m=re.exec(text))){const b=m[1];const sm=b.match(/SUMMARY[^:]*:(.+)/);const dt=b.match(/DTSTART[^:]*:(\d{8})(T(\d{2})(\d{2}))?/);if(sm&&dt){const date=dt[1].slice(0,4)+'-'+dt[1].slice(4,6)+'-'+dt[1].slice(6,8);const start=dt[3]?dt[3]+':'+dt[4]:'09:00';events.push({title:sm[1].trim(),date,start})}}
  if(!events.length)return err('invalid_value','没有解析到 VEVENT');
  let added=0;for(const e of events){const end=(String(Math.min(23,+e.start.slice(0,2)+1))).padStart(2,'0')+e.start.slice(2);await S.upsertBlock({id:uid(),date:e.date,start:e.start,end,title:e.title,type:'custom',source:'manual',done:false,note:'来自 ICS 导入'});added++}
  logImport('ics','日历事件 '+added+' 条',added);return{added,skipped:0}},
 async importScreenshot(){await wait(2100);return{preview:[
  {name:'线性代数',teacher:'孙老师',room:'一教 210',day:2,start:'14:00',end:'15:40'},
  {name:'大学物理实验',teacher:'吴老师',room:'实验楼 B1',day:4,start:'14:00',end:'16:30'}]}},
 async applyImportedCourses(list){await wait(300);let added=0,skipped=0;list.forEach(c=>{if(st().courses.some(x=>x.name===c.name&&x.day===c.day)){skipped++;return}st().courses.push({...c,id:uid()});added++});logImport('screenshot','识别到课程 '+list.length+' 门（'+skipped+' 门已存在）',added);return{added,skipped}}
});

/* ── mood ── */
const MOOD_REPLY={happy:'为你开心！把这份好状态记下来啦，看看是什么让今天发光的 ✨',calm:'平静是很珍贵的状态，适合做点需要专注的事。',excited:'能量满满！要不要把这股劲头用在最想推进的那件事上？',grateful:'心怀感激的日子会过得更慢一点，也更暖一点。',tired:'累的时候不必硬撑。今晚的计划我会排松一些，早点休息。',anxious:'焦虑说明你在乎。深呼吸，我们可以把大任务拆成很小的一步。要不要试试呼吸练习？',sad:'抱抱你。难过的时候不用急着好起来，我在这儿陪着。',angry:'生气是正常的。等情绪落一落，我们再看看能改变什么。',bored:'无聊有时是大脑在要求新鲜感，去楼下走一圈？',stressed:'压力大的时候，先把今天必须做的挑出来，其余的都可以放到明天。',lonely:'孤单的时刻，跟我说说话也可以。你不是一个人。',okay:'普普通通的一天也值得记录。明天见 :)'};
Object.assign(S,{
 async listMoods(){await wait(60);return JSON.parse(JSON.stringify(st().moods))},
 async addMood({mood,note}){await wait(600);const reply=MOOD_REPLY[mood]||MOOD_REPLY.okay;const item={id:uid(),ts:nowIso(),date:S.todayIso(),mood,note:note||'',reply};st().moods.unshift(item);S.save();return JSON.parse(JSON.stringify(item))}
});

/* ── themes ── */
const MAP={'--primary':'--dc-accent','--accent':'--dc-accent-2','--bg-start':'--dc-bg','--text-primary':'--dc-ink','--text-secondary':'--dc-ink-2','--text-muted':'--dc-ink-3','--surface':'--dc-surface','--surface-hover':'--dc-surface-2','--success':'--dc-ok','--warning':'--dc-warn','--error':'--dc-err'};
function applyThemeVars(vars){let css='';for(const k in vars){const nk=k.startsWith('--dc-')?k:MAP[k];if(nk)css+=nk+':'+vars[k]+';'}
 let el=document.getElementById('dc4-theme-vars');if(!el){el=document.createElement('style');el.id='dc4-theme-vars';document.head.appendChild(el)}
 el.textContent=css?'html{'+css+'}':''}
function setTheme(key){const t=st();t.activeTheme=key;S.save();
 const custom=t.themes.find(x=>x.id===key);
 if(custom){document.documentElement.dataset.theme=custom.base||'sky';applyThemeVars(custom.vars)}
 else{document.documentElement.dataset.theme=['sky','sunset','night','nature'].includes(key)?key:'sky';applyThemeVars({})}}
function genPalette(desc){const d=desc.toLowerCase();let h=0;for(let i=0;i<desc.length;i++)h=(h*31+desc.charCodeAt(i))%360;
 if(/海|ocean|海洋|蓝/.test(d))h=205;else if(/森|林|forest|绿/.test(d))h=150;else if(/日落|夕阳|sunset|橙/.test(d))h=26;else if(/樱|粉|sakura|pink/.test(d))h=340;else if(/紫|薰衣草|lavender/.test(d))h=268;else if(/茶|咖啡|coffee|奶/.test(d))h=34;
 const dark=/夜|暗|深|dark|night|midnight/.test(d);
 const hsl=(hh,s,l)=>'hsl('+Math.round(hh)+' '+s+'% '+l+'%)';
 return dark?{base:'night',vars:{'--dc-accent':hsl(h,52,62),'--dc-accent-2':hsl((h+18)%360,60,74),'--dc-bg':hsl(h,16,10),'--dc-surface':hsl(h,14,15),'--dc-surface-2':hsl(h,13,19),'--dc-ink':hsl(h,20,93),'--dc-ink-2':hsl(h,12,70),'--dc-ink-3':hsl(h,9,48)}}
 :{base:'sky',vars:{'--dc-accent':hsl(h,58,44),'--dc-accent-2':hsl((h+20)%360,64,64),'--dc-bg':hsl(h,30,97),'--dc-surface':'#ffffff','--dc-surface-2':hsl(h,22,93),'--dc-ink':hsl(h,32,15),'--dc-ink-2':hsl(h,16,38),'--dc-ink-3':hsl(h,12,60)}}}
Object.assign(S,{
 applyThemeVars,setTheme,activeTheme:()=>st().activeTheme,
 listThemes(){return JSON.parse(JSON.stringify(st().themes))},
 async generateTheme(desc){await wait(1600);const p=genPalette(desc);return{name:desc.slice(0,12),desc,base:p.base,vars:p.vars}},
 async saveTheme(t){await wait(150);const item={id:uid(),name:t.name,desc:t.desc||'',base:t.base,vars:t.vars,createdAt:nowIso()};st().themes.push(item);S.save();return JSON.parse(JSON.stringify(item))},
 async renameTheme(id,name){await wait(100);const t=st().themes.find(x=>x.id===id);if(t)t.name=name;S.save();return{}},
 async deleteTheme(id){await wait(100);st().themes=st().themes.filter(x=>x.id!==id);if(st().activeTheme===id)setTheme('sky');S.save();return{}},
 initTheme(){setTheme(st().activeTheme||'sky')}
});

/* ── companion ── */
const chunks=s=>{const out=[];let i=0;while(i<s.length){const n=2+Math.floor(Math.random()*4);out.push(s.slice(i,i+n));i+=n}return out};
function buildReply(text){
 const t=text.toLowerCase();const name=st().profile.assistantName||'小日';
 const planIntent=/计划|安排|规划|排|plan|schedule|调整|挪|改到|取消|加/.test(t);
 const moodIntent=/难过|焦虑|累|压力|烦|开心|紧张|emo|sad|tired|anxious/.test(t);
 if(planIntent){
  const today=S.todayIso();const relax=/轻松|松|累|休息/.test(t);
  const card={id:uid(),status:'pending',title:relax?'把今天排松一点':'调整今天的计划',date:today,
   changes:relax?[{action:'remove',match:'task',label:'移除一节推进任务'},{action:'add',block:{date:today,start:'20:30',end:'21:10',title:'自由时间 · 放松一下',type:'break'}}]
   :[{action:'add',block:{date:today,start:'19:30',end:'20:20',title:'推进 · '+((st().assignments.find(a=>a.status!=='done')||{}).title||'待办任务').split(' · ').pop(),type:'task'}},{action:'add',block:{date:today,start:'21:40',end:'22:00',title:'散步放空',type:'break'}}],
   summary:relax?'减少一节任务，加一段自由时间':'加一节 50 分钟推进 + 一段散步'};
  return{pre:'收到～我看了下你今天的安排。',text:relax?'今天的节奏确实有点满。我建议把一节推进任务移走，晚上留一段完全自由的时间——休息也是计划的一部分。':'晚饭后有一段空档，很适合推进最近的作业；结束后散个步收收尾。你看这样调整可以吗？',card,tools:[{label:'读取今日计划与待办',result:'今日 '+(st().plan[today]||[]).length+' 个时间块 · '+st().assignments.filter(a=>a.status!=='done').length+' 项未完成'}]}}
 if(moodIntent)return{pre:'',text:'我听到了。'+(/累|tired/.test(t)?'连续赶进度真的很耗人，今晚就别再加任务了。':'这种感觉很正常，说出来就已经是在照顾自己了。')+'\n需要的话，心情页有个 4-7-8 呼吸练习，两分钟就能让身体松下来。我陪你。',tools:[]}
 if(/天气|weather/.test(t))return{pre:'',text:'我看了下：今天多云转晴，18-26°C，傍晚有点风——很适合把运动排在户外 🌤',tools:[{label:'查询天气',result:'多云转晴 · 18-26°C'}]}
 if(/你是谁|who are you|你叫/.test(t))return{pre:'',text:'我是'+name+'，你的日常陪伴助理。我记得你的课表、作业和习惯，可以帮你自动规划每一天，也随时可以聊聊天。',tools:[]}
 return{pre:'',text:'嗯嗯，我记下了。如果想让我动手调整计划，直接说「帮我把今晚排松一点」这类话就行；也可以去「今日」页一键自主规划。',tools:[]}
}
Object.assign(S,{
 async listThreads(){await wait(60);return JSON.parse(JSON.stringify(st().threads))},
 async newThread(){await wait(100);const th={id:uid(),title:'新的对话',createdAt:nowIso(),messages:[{id:uid(),role:'assistant',content:'这里是新的一页，想聊什么？',ts:nowIso()}]};st().threads.unshift(th);S.save();return JSON.parse(JSON.stringify(th))},
 async deleteThread(id){await wait(80);st().threads=st().threads.filter(x=>x.id!==id);S.save();return{}},
 sendMessage(threadId,text,onFrame){
  const th=st().threads.find(x=>x.id===threadId);if(!th)return Promise.reject({error:'not_found'});
  const um={id:uid(),role:'user',content:text,ts:nowIso()};th.messages.push(um);
  if(th.messages.filter(m=>m.role==='user').length===1)th.title=text.slice(0,16)||th.title;
  S.save();
  const r=buildReply(text);const am={id:uid(),role:'assistant',content:'',ts:nowIso(),card:null};
  return new Promise(res=>{
   let q=Promise.resolve();const step=(f,ms)=>{q=q.then(()=>new Promise(r2=>setTimeout(()=>{f();r2()},ms)))};
   step(()=>onFrame({t:'start'}),300);
   (r.tools||[]).forEach(tool=>{step(()=>onFrame({t:'tool_start',label:tool.label}),450);step(()=>onFrame({t:'tool_result',label:tool.label,result:tool.result}),750)});
   const full=(r.pre?r.pre+'\n':'')+r.text;
   chunks(full).forEach(c=>step(()=>{am.content+=c;onFrame({t:'delta',delta:c,content:am.content})},28));
   if(r.card)step(()=>{am.card=r.card;onFrame({t:'decision_card',card:r.card})},420);
   step(()=>{th.messages.push(am);S.save();onFrame({t:'done',message:JSON.parse(JSON.stringify(am))});res(am)},200);
  })
 },
 async applyDecision(threadId,msgId){await wait(300);const th=st().threads.find(x=>x.id===threadId);const m=th&&th.messages.find(x=>x.id===msgId);if(!m||!m.card)return err('not_found');
  const card=m.card;S.setOp('采纳建议：'+card.title,[card.date]);
  const arr=st().plan[card.date]=st().plan[card.date]||[];
  card.changes.forEach(ch=>{if(ch.action==='remove'){const i=arr.findIndex(b=>b.type===ch.match&&!b.locked&&b.source!=='manual');if(i>=0)arr.splice(i,1)}else if(ch.action==='add'){arr.push({...ch.block,id:uid(),source:'companion',done:false,note:ch.block.note||'来自陪伴建议',locked:false})}});
  arr.sort((a,b)=>a.start<b.start?-1:1);card.status='accepted';S.save();return{summary:card.summary}},
 async dismissDecision(threadId,msgId){await wait(80);const th=st().threads.find(x=>x.id===threadId);const m=th&&th.messages.find(x=>x.id===msgId);if(m&&m.card)m.card.status='dismissed';S.save();return{}}
});

/* ── data ── */
Object.assign(S,{
 exportData(){const t=st();const pick=({users,session,admin,...rest})=>rest;return JSON.stringify(pick(JSON.parse(JSON.stringify(t))),null,2)},
 async importData(json){await wait(300);let doc;try{doc=JSON.parse(json)}catch(e){return err('invalid_value','JSON 无法解析')}
  const t=st();['profile','courses','assignments','rules','plan','inbox','imports','threads','moods','memory','themes','activeTheme'].forEach(k=>{if(doc[k]!=null)t[k]=doc[k]});S.save();return{}},
 async resetAll(){try{localStorage.removeItem('dc4.state.v1')}catch(e){};return{}}
});
})();
