/* DcStore core — mock backend on localStorage. All methods return Promises mirroring the v4 API. */
(function(){
const KEY='dc4.state.v1';
const uid=()=>Math.random().toString(36).slice(2,10);
const pad=n=>String(n).padStart(2,'0');
const iso=d=>d.getFullYear()+'-'+pad(d.getMonth()+1)+'-'+pad(d.getDate());
const todayIso=()=>iso(new Date());
const addDays=(isoStr,n)=>{const d=new Date(isoStr+'T12:00:00');d.setDate(d.getDate()+n);return iso(d)};
const wd=isoStr=>{const d=new Date(isoStr+'T12:00:00').getDay();return d===0?7:d};
const tm=s=>{const[a,b]=s.split(':');return +a*60+ +b};
const mt=m=>pad(Math.floor(m/60)%24)+':'+pad(m%60);
const wait=ms=>new Promise(r=>setTimeout(r,ms+Math.random()*ms*.4));
const err=(code,msg)=>Promise.reject({error:code,message:msg||code});
const clone=o=>JSON.parse(JSON.stringify(o));

let st=null,saveT=null;
function save(){clearTimeout(saveT);saveT=setTimeout(()=>{try{localStorage.setItem(KEY,JSON.stringify(st))}catch(e){}},120)}
function load(){try{const raw=localStorage.getItem(KEY);if(raw){st=JSON.parse(raw);return true}}catch(e){}return false}

/* ── seed ── */
function seed(){
const t=todayIso();
st={
 v:1,session:{userId:null,onboarded:false},
 users:[
  {id:'u_demo',email:'demo@daycore.app',password:'demo1234',name:'黎清',role:'user',createdAt:'2026-06-02T09:12:00Z'},
  {id:'u_admin',email:'admin@daycore.app',password:'admin123',name:'Yun',role:'admin',createdAt:'2026-05-20T02:00:00Z'},
  {id:'u_kite',email:'kite@stu.pku.edu.cn',password:'x',name:'风筝',role:'user',createdAt:'2026-06-18T13:40:00Z'},
  {id:'u_mo',email:'mo@outlook.com',password:'x',name:'Mo',role:'user',createdAt:'2026-07-01T08:05:00Z'}],
 profile:{assistantName:'小日',wake:'07:30',sleep:'23:30',focus:'morning',l2Prompt:''},
 courses:[
  {id:'c1',name:'数据结构',teacher:'王老师',room:'理教 204',day:1,start:'08:00',end:'09:40'},
  {id:'c2',name:'离散数学',teacher:'李老师',room:'二教 415',day:2,start:'10:10',end:'11:50'},
  {id:'c3',name:'大学英语',teacher:'Ms. Ellis',room:'外语楼 302',day:3,start:'14:00',end:'15:40'},
  {id:'c4',name:'操作系统',teacher:'赵老师',room:'理教 108',day:4,start:'08:00',end:'09:40'},
  {id:'c5',name:'体育（羽毛球）',teacher:'陈老师',room:'综合馆',day:5,start:'16:00',end:'17:30'}],
 assignments:[
  {id:'a1',title:'数据结构 · 实验三（AVL 树）',course:'数据结构',due:addDays(t,2),status:'doing',notes:'提交到 OJ',source:'canvas',createdAt:addDays(t,-4)},
  {id:'a2',title:'离散数学 · 习题集 §7',course:'离散数学',due:addDays(t,1),status:'todo',notes:'',source:'canvas',createdAt:addDays(t,-3)},
  {id:'a3',title:'英语口语展示准备',course:'大学英语',due:addDays(t,5),status:'todo',notes:'3 分钟自我陈述',source:'manual',createdAt:addDays(t,-2)},
  {id:'a4',title:'操作系统 · 阅读第 4 章',course:'操作系统',due:addDays(t,3),status:'todo',notes:'',source:'companion',createdAt:addDays(t,-1)},
  {id:'a5',title:'周报（社团）',course:'',due:addDays(t,-1),status:'done',notes:'',source:'manual',createdAt:addDays(t,-6)}],
 rules:[
  {id:'r1',text:'23:30 前睡觉，睡前一小时不安排学习任务',active:true,source:'user',createdAt:addDays(t,-9)},
  {id:'r2',text:'每周至少运动三次，每次 30 分钟以上',active:true,source:'companion',createdAt:addDays(t,-7)},
  {id:'r3',text:'午饭后留 20 分钟散步，不排任务',active:true,source:'user',createdAt:addDays(t,-5)},
  {id:'r4',text:'周五晚上是自由时间，不排学习',active:false,source:'user',createdAt:addDays(t,-3)}],
 plan:{},inbox:[
  {id:'ib1',text:'下周三之前把奖学金申请表交到院办 312',createdAt:addDays(t,-1)+'T20:14:00',status:'pending',parsed:{kind:'assignment',payload:{title:'奖学金申请表 · 交到院办 312',due:addDays(t,5),course:'',notes:'下周三之前把奖学金申请表交到院办 312'}}}],
 imports:[{id:'im1',kind:'canvas',summary:'Canvas 作业 6 条（2 条已存在，跳过）',count:4,createdAt:addDays(t,-4)+'T10:02:00'}],
 threads:[{id:'th1',title:'和小日的对话',createdAt:addDays(t,-2)+'T09:00:00',messages:[
  {id:'m1',role:'assistant',content:'早呀，我是小日 ☁️ 课表和作业我都记住了。想调整计划、倒倒苦水，或者只是聊聊天，都可以找我。',ts:addDays(t,-2)+'T09:00:00'}]}],
 moods:[
  {id:'md1',ts:addDays(t,-3)+'T22:10:00',date:addDays(t,-3),mood:'calm',note:'把实验课补完了，松了口气',reply:'辛苦啦，补完的感觉是不是很踏实？今晚早点休息。'},
  {id:'md2',ts:addDays(t,-2)+'T21:40:00',date:addDays(t,-2),mood:'tired',note:'',reply:'累的时候不必硬撑，明天的计划我会排松一点。'},
  {id:'md3',ts:addDays(t,-1)+'T23:02:00',date:addDays(t,-1),mood:'happy',note:'羽毛球赢了两局！',reply:'哇，运动的快乐是真实的！记得拉伸～'}],
 memory:[
  {id:'me1',text:'早上效率最高，重要任务尽量排上午',createdAt:addDays(t,-9),source:'companion'},
  {id:'me2',text:'不吃香菜；食堂偏好二楼面档',createdAt:addDays(t,-6),source:'user'},
  {id:'me3',text:'期末周前会比较焦虑，需要更多鼓励',createdAt:addDays(t,-4),source:'companion'}],
 themes:[],activeTheme:'sky',
 lastOp:null,
 admin:null
};
save();
}
if(!load())seed();
if(!st.admin&&window.DcAdminSeed)st.admin=window.DcAdminSeed();

/* ── helpers ── */
const cur=()=>st.session.userId?st.users.find(u=>u.id===st.session.userId)||null:null;
const pubUser=u=>u?{id:u.id,email:u.email,name:u.name,role:u.role,createdAt:u.createdAt}:null;
function snapshotDates(dates){const s={};dates.forEach(d=>s[d]=clone(st.plan[d]||[]));return s}
function setOp(label,dates){st.lastOp={label,snap:snapshotDates(dates),ts:Date.now()};save()}

/* ── auto-plan generator ── */
function genDay(date,instruction,mode){
 const insr=(instruction||'').toLowerCase();
 const p=st.profile;let wake=tm(p.wake),sleep=tm(p.sleep);
 if(/早睡|early sleep|早点睡/.test(insr))sleep-=45;
 const kept=(mode==='keep')?(st.plan[date]||[]).filter(b=>b.source==='manual'||b.locked):[];
 const blocks=[...kept.map(clone)];
 const overlaps=(s,e)=>blocks.some(b=>tm(b.start)<e&&tm(b.end)>s);
 const push=(s,e,titleObj,type,note)=>{if(s>=e||overlaps(s,e))return null;const b={id:uid(),date,start:mt(s),end:mt(e),title:titleObj,note:note||'',type,source:'auto',done:false,locked:false};blocks.push(b);return b};
 push(wake,wake+30,'洗漱 & 早餐','meal');
 st.courses.filter(c=>c.day===wd(date)).forEach(c=>push(tm(c.start),tm(c.end),c.name,'course',(c.room||'')+(c.teacher?' · '+c.teacher:'')));
 let lun=720;while(overlaps(lun,lun+40)&&lun<840)lun+=10;push(lun,lun+40,'午餐','meal');
 if(st.rules.some(r=>r.active&&/散步/.test(r.text)))push(lun+40,lun+60,'午后散步','break');
 let din=1080;while(overlaps(din,din+40)&&din<1200)din+=10;push(din,din+40,'晚餐','meal');
 const relax=/轻松|放松|休息|light|easy/.test(insr);
 const exam=/考试|复习|exam|review/.test(insr);
 const dueSoon=st.assignments.filter(a=>a.status!=='done'&&a.due>=date&&tm('00:01')&&(new Date(a.due)-new Date(date))/864e5<=5).sort((a,b)=>a.due<b.due?-1:1);
 let quota=relax?1:(exam?3:2);
 const gaps=()=>{const occ=blocks.map(b=>[tm(b.start),tm(b.end)]).sort((a,b)=>a[0]-b[0]);const g=[];let c=Math.max(wake+30,tm('08:00'));const lim=sleep-60;occ.forEach(([s,e])=>{if(s-c>=55)g.push([c,s]);c=Math.max(c,e)});if(lim-c>=55)g.push([c,lim]);return g};
 for(const a of dueSoon){if(quota<=0)break;const g=gaps()[0];if(!g)break;const s=g[0]+5,e=Math.min(s+50,g[1]);if(push(s,e,'推进 · '+a.title.split(' · ').pop(),'task','截止 '+a.due)){quota--}}
 if(exam){const g=gaps()[0];if(g)push(g[0]+5,Math.min(g[0]+65,g[1]),'复习冲刺','task')}
 const wantsEx=/运动|锻炼|exercise|跑步/.test(insr)||st.rules.some(r=>r.active&&/运动/.test(r.text));
 if(wantsEx&&!blocks.some(b=>b.type==='exercise'||/体育/.test(String(b.title)))){const g=gaps().find(g=>g[0]>=960)||gaps()[0];if(g)push(Math.max(g[0],1020),Math.min(Math.max(g[0],1020)+35,g[1]),'运动 30 分钟','exercise')}
 if(relax){const g=gaps()[0];if(g)push(g[0]+5,Math.min(g[0]+65,g[1]),'自由时间 · 做点喜欢的事','break')}
 push(sleep-40,sleep-15,'晚间复盘 & 准备明天','custom');
 blocks.sort((a,b)=>tm(a.start)-tm(b.start));
 return blocks;
}

/* ── public API ── */
window.DcStore={
 _st:()=>st,save,uid,iso,todayIso,addDays,wd,
 subscribeSaved(){},
 /* session & auth */
 async me(){await wait(60);const u=cur();return{user:pubUser(u),onboarded:st.session.onboarded,profile:clone(st.profile)}},
 async login(email,password){await wait(500);const u=st.users.find(x=>x.email===email.trim().toLowerCase());if(!u||u.password!==password)return err('invalid_credentials');st.session.userId=u.id;save();return{user:pubUser(u)}},
 async register(email,password,name){await wait(600);email=email.trim().toLowerCase();if(!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email))return err('invalid_email');if(password.length<8)return err('weak_password');if(st.users.some(x=>x.email===email))return err('email_taken');const first=st.users.length===0;const u={id:uid(),email,password,name:name||email.split('@')[0],role:first?'admin':'user',createdAt:new Date().toISOString()};st.users.push(u);st.session.userId=u.id;save();return{user:pubUser(u)}},
 async oauth(provider){await wait(1400);const email=provider+'.user@daycore.app';let u=st.users.find(x=>x.email===email);if(!u){u={id:uid(),email,name:provider==='google'?'G. Chen':'octo-dev',role:'user',createdAt:new Date().toISOString(),password:uid()};st.users.push(u)}st.session.userId=u.id;save();return{user:pubUser(u)}},
 async logout(){await wait(150);st.session.userId=null;save();return{}},
 async setOnboarded(v){st.session.onboarded=v!==false;save();return{}},
 async saveProfile(patch){await wait(120);Object.assign(st.profile,patch);save();return clone(st.profile)},
 /* plan */
 async listPlan(date){await wait(90);return clone(st.plan[date]||[])},
 async planRange(from,days){await wait(90);const o={};for(let i=0;i<days;i++){const d=addDays(from,i);o[d]=clone(st.plan[d]||[])}return o},
 async upsertBlock(b){await wait(100);const date=b.date;const arr=st.plan[date]=st.plan[date]||[];const i=arr.findIndex(x=>x.id===b.id);if(i>=0)arr[i]={...arr[i],...b};else arr.push({...b,id:b.id||uid(),source:b.source||'manual',done:!!b.done});arr.sort((a,c)=>tm(a.start)-tm(c.start));save();return clone(arr)},
 async deleteBlock(date,id){await wait(80);st.plan[date]=(st.plan[date]||[]).filter(b=>b.id!==id);save();return{}},
 async toggleDone(date,id){await wait(50);const b=(st.plan[date]||[]).find(x=>x.id===id);if(b)b.done=!b.done;save();return clone(b)},
 async toggleLock(date,id){await wait(50);const b=(st.plan[date]||[]).find(x=>x.id===id);if(b)b.locked=!b.locked;save();return clone(b)},
 async autoPlan({dates,instruction,mode}){
  await wait(400);
  setOp('自主规划（'+dates.length+' 天）',dates);
  const days=[];
  for(const d of dates){await wait(420);const blocks=genDay(d,instruction,mode);st.plan[d]=blocks;days.push({date:d,blocks:clone(blocks)})}
  save();
  const n=days.reduce((s,x)=>s+x.blocks.length,0);
  return{days,summary:'为 '+dates.length+' 天生成了 '+n+' 个时间块'+(mode==='keep'?'（保留了手动安排）':'')}
 },
 async undoLast(){await wait(120);if(!st.lastOp)return err('not_found','nothing to undo');for(const d in st.lastOp.snap)st.plan[d]=st.lastOp.snap[d];const label=st.lastOp.label;st.lastOp=null;save();return{label}},
 lastOpLabel(){return st.lastOp?st.lastOp.label:null},
 setOp,
 /* courses / assignments / rules */
 async listCourses(){await wait(60);return clone(st.courses)},
 async upsertCourse(c){await wait(100);const i=st.courses.findIndex(x=>x.id===c.id);if(i>=0)st.courses[i]={...st.courses[i],...c};else st.courses.push({...c,id:uid()});save();return clone(st.courses)},
 async deleteCourse(id){await wait(80);st.courses=st.courses.filter(c=>c.id!==id);save();return{}},
 async listAssignments(){await wait(60);return clone(st.assignments).sort((a,b)=>a.due<b.due?-1:1)},
 async upsertAssignment(a){await wait(100);const i=st.assignments.findIndex(x=>x.id===a.id);if(i>=0)st.assignments[i]={...st.assignments[i],...a};else st.assignments.push({...a,id:uid(),source:a.source||'manual',createdAt:todayIso()});save();return{}},
 async setAssignmentStatus(id,status){await wait(60);const a=st.assignments.find(x=>x.id===id);if(a)a.status=status;save();return{}},
 async deleteAssignment(id){await wait(80);st.assignments=st.assignments.filter(a=>a.id!==id);save();return{}},
 async listRules(){await wait(60);return clone(st.rules)},
 async addRule(text,source){await wait(100);st.rules.unshift({id:uid(),text,active:true,source:source||'user',createdAt:todayIso()});save();return clone(st.rules)},
 async toggleRule(id){await wait(60);const r=st.rules.find(x=>x.id===id);if(r)r.active=!r.active;save();return{}},
 async deleteRule(id){await wait(80);st.rules=st.rules.filter(r=>r.id!==id);save();return{}}
};
})();
