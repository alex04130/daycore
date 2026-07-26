/* Materials — inbox capture / courses / assignments / rules / imports */
(function(){
const {Button,IconBtn,Icon,Field,Input,Textarea,Select,Seg,Sheet,Tag,Chip,Toggle,Empty,useI18n,useAsync,errMsg}=DcUI;
const {useState,useEffect,Fragment}=React;
DcI18n.add({
 'mt.h':{zh:'资料',en:'Materials'},'mt.sub':{zh:'课表、作业与约定，规划都从这里长出来',en:'Courses, deadlines and rules — planning grows from here'},
 'mt.tab.inbox':{zh:'收集箱',en:'Inbox'},'mt.tab.courses':{zh:'课程',en:'Courses'},'mt.tab.assignments':{zh:'作业',en:'Work'},'mt.tab.rules':{zh:'规则',en:'Rules'},'mt.tab.imports':{zh:'导入',en:'Import'},
 'mt.capPh':{zh:'随手记一句，小日来整理。例如「下周三交离散作业」…',en:'Jot anything — “discrete math due next Wed”…'},'mt.parse':{zh:'AI 解析',en:'Parse'},
 'mt.pending':{zh:'待确认',en:'To confirm'},'mt.confirm':{zh:'采纳',en:'Accept'},'mt.ignore':{zh:'忽略',en:'Dismiss'},
 'mt.k.assignment':{zh:'作业',en:'Assignment'},'mt.k.block':{zh:'日程',en:'Schedule'},'mt.k.memory':{zh:'记忆',en:'Memory'},
 'mt.emptyInbox':{zh:'收集箱是空的',en:'Inbox is clear'},'mt.emptyInboxSub':{zh:'脑子里闪过什么，先扔进来，小日会解析成作业、日程或记忆。',en:'Toss in stray thoughts; Daycore sorts them into work, schedule or memory.'},
 'mt.addCourse':{zh:'添加课程',en:'Add course'},'mt.editCourse':{zh:'编辑课程',en:'Edit course'},'mt.cname':{zh:'课程名',en:'Name'},'mt.day':{zh:'星期',en:'Day'},'mt.teacher':{zh:'老师',en:'Teacher'},'mt.room':{zh:'教室',en:'Room'},
 'mt.addA':{zh:'添加作业',en:'Add item'},'mt.editA':{zh:'编辑作业',en:'Edit item'},'mt.due':{zh:'截止',en:'Due'},'mt.course':{zh:'课程',en:'Course'},'mt.notes':{zh:'备注',en:'Notes'},
 'mt.st.todo':{zh:'待办',en:'To do'},'mt.st.doing':{zh:'进行中',en:'Doing'},'mt.st.done':{zh:'完成',en:'Done'},'mt.overdue':{zh:'已逾期',en:'Overdue'},
 'mt.f.open':{zh:'未完成',en:'Open'},'mt.f.all':{zh:'全部',en:'All'},
 'mt.rulePh':{zh:'例如：周日晚上不安排任务',en:'e.g. keep Sunday nights free'},'mt.ruleSub':{zh:'自主规划与陪伴建议都会遵守这些约定。',en:'Auto-plan and companion suggestions follow these.'},
 'mt.src.canvas':{zh:'Canvas',en:'Canvas'},'mt.src.manual':{zh:'手动',en:'manual'},'mt.src.companion':{zh:'陪伴',en:'companion'},'mt.src.capture':{zh:'收集箱',en:'inbox'},'mt.src.user':{zh:'我',en:'me'},'mt.src.screenshot':{zh:'截图',en:'photo'},'mt.src.ics':{zh:'ICS',en:'ICS'},
 'mt.history':{zh:'导入历史',en:'History'},'mt.importNow':{zh:'导入',en:'Import'},'mt.emptyDay':{zh:'这天没有课',en:'No class'},
});
const DAY_OPTS=()=>Array.from({length:7},(_,i)=>({value:i+1,label:(DcI18n.locale==='zh'?'周':'')+DcI18n.t('wd.'+(i+1))}));
function CourseSheet({course,onClose,onSaved}){
  const {t}=useI18n();const isNew=!course.id;
  const[f,setF]=useState({name:course.name||'',day:course.day||1,start:course.start||'08:00',end:course.end||'09:40',teacher:course.teacher||'',room:course.room||''});
  const set=(k,v)=>setF(x=>({...x,[k]:v}));const[busy,setBusy]=useState(false);
  const save=async()=>{setBusy(true);await DcStore.upsertCourse({...course,...f,day:+f.day});onSaved()};
  const del=async()=>{if(await DcUI.confirm({title:t('c.delete')+'「'+f.name+'」？',danger:true,confirmLabel:t('c.delete')})){await DcStore.deleteCourse(course.id);onSaved()}};
  return <Sheet open onClose={onClose} title={isNew?t('mt.addCourse'):t('mt.editCourse')} footer={<Fragment>
      {!isNew&&<Button variant="danger soft" onClick={del}>{t('c.delete')}</Button>}<span style={{flex:1}}></span>
      <Button variant="ghost" onClick={onClose}>{t('c.cancel')}</Button><Button loading={busy} disabled={!f.name.trim()||f.start>=f.end} onClick={save}>{t('c.save')}</Button></Fragment>}>
    <div style={{display:'flex',flexDirection:'column',gap:13}}>
      <Field label={t('mt.cname')}><Input value={f.name} onChange={e=>set('name',e.target.value)} autoFocus={isNew}/></Field>
      <div className="dc4-grid3">
        <Field label={t('mt.day')}><Select value={f.day} onChange={e=>set('day',e.target.value)} options={DAY_OPTS()}/></Field>
        <Field label={t('td.start')}><Input type="time" value={f.start} onChange={e=>set('start',e.target.value)}/></Field>
        <Field label={t('td.end')}><Input type="time" value={f.end} onChange={e=>set('end',e.target.value)}/></Field>
      </div>
      <div className="dc4-grid2">
        <Field label={t('mt.teacher')}><Input value={f.teacher} onChange={e=>set('teacher',e.target.value)}/></Field>
        <Field label={t('mt.room')}><Input value={f.room} onChange={e=>set('room',e.target.value)}/></Field>
      </div>
    </div></Sheet>}
function AssignmentSheet({a,onClose,onSaved}){
  const {t}=useI18n();const isNew=!a.id;
  const[f,setF]=useState({title:a.title||'',course:a.course||'',due:a.due||DcStore.addDays(DcStore.todayIso(),3),notes:a.notes||'',status:a.status||'todo'});
  const set=(k,v)=>setF(x=>({...x,[k]:v}));const[busy,setBusy]=useState(false);
  const save=async()=>{setBusy(true);await DcStore.upsertAssignment({...a,...f});onSaved()};
  const del=async()=>{if(await DcUI.confirm({title:t('c.delete')+'？',message:f.title,danger:true,confirmLabel:t('c.delete')})){await DcStore.deleteAssignment(a.id);onSaved()}};
  return <Sheet open onClose={onClose} title={isNew?t('mt.addA'):t('mt.editA')} footer={<Fragment>
      {!isNew&&<Button variant="danger soft" onClick={del}>{t('c.delete')}</Button>}<span style={{flex:1}}></span>
      <Button variant="ghost" onClick={onClose}>{t('c.cancel')}</Button><Button loading={busy} disabled={!f.title.trim()} onClick={save}>{t('c.save')}</Button></Fragment>}>
    <div style={{display:'flex',flexDirection:'column',gap:13}}>
      <Field label={t('td.title')}><Input value={f.title} onChange={e=>set('title',e.target.value)} autoFocus={isNew}/></Field>
      <div className="dc4-grid2">
        <Field label={t('mt.due')}><Input type="date" value={f.due} onChange={e=>set('due',e.target.value)}/></Field>
        <Field label={t('mt.course')}><Input value={f.course} onChange={e=>set('course',e.target.value)}/></Field>
      </div>
      <Field label={t('mt.st.todo')+' / '+t('mt.st.doing')+' / '+t('mt.st.done')}><Seg block value={f.status} onChange={v=>set('status',v)} options={['todo','doing','done'].map(v=>({value:v,label:t('mt.st.'+v)}))}/></Field>
      <Field label={t('mt.notes')}><Input value={f.notes} onChange={e=>set('notes',e.target.value)}/></Field>
    </div></Sheet>}
function ParsedCard({item,onDone}){
  const {t}=useI18n();const[p,setP]=useState(item.parsed||{kind:'memory',payload:{text:item.text}});const[busy,setBusy]=useState(false);
  const set=(k,v)=>setP(x=>({...x,payload:{...x.payload,[k]:v}}));
  const confirm=async()=>{setBusy(true);try{const r=await DcStore.confirmInbox(item.id,p);DcUI.toast(t('c.applied')+' · '+t('mt.k.'+r.kind),'ok');onDone()}catch(e){DcUI.toast(errMsg(e),'err');setBusy(false)}};
  return <div className="dc4-card" style={{border:'1.5px solid color-mix(in srgb,var(--dc-accent) 35%,transparent)'}}>
    <div className="dc4-cap-3" style={{marginBottom:9}}>“{item.text}”</div>
    <div style={{marginBottom:11}}><Tag tone="accent">{t('mt.k.'+p.kind)}</Tag></div>
    {p.kind==='assignment'&&<div className="dc4-grid2">
      <Field label={t('td.title')}><Input value={p.payload.title} onChange={e=>set('title',e.target.value)}/></Field>
      <Field label={t('mt.due')}><Input type="date" value={p.payload.due} onChange={e=>set('due',e.target.value)}/></Field></div>}
    {p.kind==='block'&&<div className="dc4-grid2">
      <Field label={t('td.title')}><Input value={p.payload.title} onChange={e=>set('title',e.target.value)}/></Field>
      <Field label={t('mt.due')}><Input type="date" value={p.payload.date} onChange={e=>set('date',e.target.value)}/></Field>
      <Field label={t('td.start')}><Input type="time" value={p.payload.start} onChange={e=>set('start',e.target.value)}/></Field>
      <Field label={t('td.end')}><Input type="time" value={p.payload.end} onChange={e=>set('end',e.target.value)}/></Field></div>}
    {p.kind==='memory'&&<Field label={t('mt.k.memory')}><Input value={p.payload.text} onChange={e=>set('text',e.target.value)}/></Field>}
    <div style={{display:'flex',gap:8,justifyContent:'flex-end',marginTop:13}}>
      <Button variant="ghost" size="sm" onClick={async()=>{await DcStore.dismissInbox(item.id);onDone()}}>{t('mt.ignore')}</Button>
      <Button size="sm" loading={busy} onClick={confirm}>{t('mt.confirm')}</Button></div></div>}
function InboxTab(){
  const {t}=useI18n();const inbox=useAsync(()=>DcStore.listInbox(),[]);
  const[text,setText]=useState('');const[busy,setBusy]=useState(false);
  const cap=async()=>{if(!text.trim())return;setBusy(true);await DcStore.quickCapture(text.trim());setText('');setBusy(false);inbox.reload()};
  const pending=(inbox.data||[]).filter(x=>x.status==='pending');
  return <div className="dc4-stack" style={{gap:14}}>
    <div className="dc4-card">
      <Textarea rows={2} value={text} onChange={e=>setText(e.target.value)} placeholder={t('mt.capPh')} onKeyDown={e=>{if(e.key==='Enter'&&!e.shiftKey){e.preventDefault();cap()}}}/>
      <div style={{display:'flex',justifyContent:'flex-end',marginTop:10}}><Button icon="wand" loading={busy} disabled={!text.trim()} onClick={cap}>{t('mt.parse')}</Button></div>
    </div>
    {inbox.loading?<DcUI.SkeletonCard lines={2}/>
    :pending.length===0?<div className="dc4-card"><Empty icon="sparkle" title={t('mt.emptyInbox')} desc={t('mt.emptyInboxSub')}/></div>
    :<div style={{display:'contents'}}><div className="dc4-eyebrow">{t('mt.pending')} · {pending.length}</div>
      {pending.map(item=><ParsedCard key={item.id} item={item} onDone={inbox.reload}/>)}</div>}
  </div>}
function CoursesTab(){
  const {t,locale}=useI18n();const c=useAsync(()=>DcStore.listCourses(),[]);const[sheet,setSheet]=useState(null);
  return <div className="dc4-stack" style={{gap:14}}>
    <div style={{display:'flex',justifyContent:'flex-end'}}><Button icon="plus" variant="sec" size="sm" onClick={()=>setSheet({})}>{t('mt.addCourse')}</Button></div>
    {c.loading?<DcUI.SkeletonCard lines={4}/>:
    <div className="dc4-card">{Array.from({length:7},(_,i)=>i+1).map(d=>{const list=(c.data||[]).filter(x=>x.day===d).sort((a,b)=>a.start<b.start?-1:1);
      return <div key={d} style={{marginBottom:d<7?14:0}}>
        <div className="dc4-eyebrow" style={{marginBottom:2}}>{(locale==='zh'?'周':'')+t('wd.'+d)}</div>
        {list.length===0?<div className="dc4-cap-3" style={{padding:'7px 0'}}>{t('mt.emptyDay')}</div>
        :list.map(x=><div key={x.id} className="dc4-row" style={{cursor:'pointer'}} onClick={()=>setSheet(x)}>
          <span className="dc4-icobox"><Icon name="book" size={16}/></span>
          <div className="grow"><div className="t">{x.name}</div><div className="s dc4-num">{x.start}–{x.end}{x.room?' · '+x.room:''}{x.teacher?' · '+x.teacher:''}</div></div>
          <Icon name="chevron-right" size={15} style={{color:'var(--dc-ink-3)'}}/></div>)}
      </div>})}</div>}
    {sheet&&<CourseSheet course={sheet} onClose={()=>setSheet(null)} onSaved={()=>{setSheet(null);c.reload()}}/>}
  </div>}
function AssignmentsTab(){
  const {t}=useI18n();const a=useAsync(()=>DcStore.listAssignments(),[]);
  const[filter,setFilter]=useState('open');const[sheet,setSheet]=useState(null);
  const today=DcStore.todayIso();
  const list=(a.data||[]).filter(x=>filter==='all'||x.status!=='done');
  const cycle=async x=>{const next={todo:'doing',doing:'done',done:'todo'}[x.status];await DcStore.setAssignmentStatus(x.id,next);a.reload()};
  return <div className="dc4-stack" style={{gap:14}}>
    <div style={{display:'flex',justifyContent:'space-between',alignItems:'center',gap:10}}>
      <Seg value={filter} onChange={setFilter} options={[{value:'open',label:t('mt.f.open')},{value:'all',label:t('mt.f.all')}]}/>
      <Button icon="plus" variant="sec" size="sm" onClick={()=>setSheet({})}>{t('mt.addA')}</Button></div>
    {a.loading?<DcUI.SkeletonCard lines={4}/>
    :list.length===0?<div className="dc4-card"><Empty icon="check" title={t('mt.f.open')+' · 0'} desc=""/></div>
    :<div className="dc4-card">{list.map(x=>{const overdue=x.due<today&&x.status!=='done';
      return <div key={x.id} className="dc4-row">
        <Chip on={x.status==='doing'} onClick={()=>cycle(x)} style={{minWidth:74,justifyContent:'center',...(x.status==='done'?{background:'var(--dc-ok-soft)',borderColor:'color-mix(in srgb,var(--dc-ok) 40%,transparent)',color:'var(--dc-ok)'}:null)}}>{t('mt.st.'+x.status)}</Chip>
        <div className="grow" style={{cursor:'pointer'}} onClick={()=>setSheet(x)}>
          <div className="t" style={x.status==='done'?{textDecoration:'line-through',color:'var(--dc-ink-3)'}:null}>{x.title}</div>
          <div className="s" style={{display:'flex',gap:7,alignItems:'center',flexWrap:'wrap'}}>
            <span className={overdue?'':'dc4-num'} style={overdue?{color:'var(--dc-err)',fontWeight:600}:null}>{overdue?t('mt.overdue')+' · ':''}{DcUI.fmtDate(x.due)}</span>
            {x.course?<span>· {x.course}</span>:null}<Tag>{t('mt.src.'+(x.source||'manual'))}</Tag></div></div>
        <Icon name="chevron-right" size={15} style={{color:'var(--dc-ink-3)'}}/></div>})}</div>}
    {sheet&&<AssignmentSheet a={sheet} onClose={()=>setSheet(null)} onSaved={()=>{setSheet(null);a.reload()}}/>}
  </div>}
function RulesTab(){
  const {t}=useI18n();const r=useAsync(()=>DcStore.listRules(),[]);
  const[text,setText]=useState('');const[busy,setBusy]=useState(false);
  const add=async()=>{if(!text.trim())return;setBusy(true);await DcStore.addRule(text.trim());setText('');setBusy(false);r.reload()};
  return <div className="dc4-stack" style={{gap:14}}>
    <div className="dc4-card">
      <div className="dc4-cap-3" style={{marginBottom:10}}>{t('mt.ruleSub')}</div>
      <div style={{display:'flex',gap:9}}>
        <Input value={text} onChange={e=>setText(e.target.value)} placeholder={t('mt.rulePh')} onKeyDown={e=>{if(e.key==='Enter'){e.preventDefault();add()}}}/>
        <Button icon="plus" loading={busy} disabled={!text.trim()} onClick={add}>{t('c.add')}</Button></div></div>
    {r.loading?<DcUI.SkeletonCard lines={4}/>:<div className="dc4-card">
      {(r.data||[]).map(x=><div key={x.id} className="dc4-row">
        <Toggle checked={x.active} onChange={async()=>{await DcStore.toggleRule(x.id);r.reload()}}/>
        <div className="grow"><div className="t" style={x.active?null:{color:'var(--dc-ink-3)'}}>{x.text}</div><div className="s"><Tag tone={x.source==='companion'?'accent':''}>{t('mt.src.'+(x.source==='user'?'user':'companion'))}</Tag></div></div>
        <IconBtn name="trash" label={t('c.delete')} onClick={async()=>{if(await DcUI.confirm({title:t('c.delete')+'？',message:x.text,danger:true,confirmLabel:t('c.delete')})){await DcStore.deleteRule(x.id);r.reload()}}}/>
      </div>)}</div>}
  </div>}
function HistoryList(){
  const {t}=useI18n();const h=useAsync(()=>DcStore.listImports(),[]);
  const ic={canvas:'file',ics:'calendar',screenshot:'image'};
  if(h.loading)return <DcUI.Skeleton h={60}/>;
  return <div>{(h.data||[]).length===0?<div className="dc4-cap-3">—</div>:(h.data||[]).map(x=><div key={x.id} className="dc4-row">
    <span className="dc4-icobox"><Icon name={ic[x.kind]||'download'} size={16}/></span>
    <div className="grow"><div className="t">{x.summary}</div><div className="s dc4-num">{x.createdAt.slice(0,16).replace('T',' ')}</div></div>
    <Tag tone="ok">+{x.count}</Tag></div>)}</div>}
function ImportBar({onChanged}){
  const {t}=useI18n();
  const[sheet,setSheet]=useState(null);const[histOpen,setHistOpen]=useState(false);
  const ISB=DcPages.ImportSheetBody;
  const cards=[{k:'canvas',icon:'file',t:t('ob.canvas')},{k:'ics',icon:'calendar',t:t('ob.ics')},{k:'shot',icon:'image',t:t('ob.shot')}];
  return <div className="dc4-card" style={{display:'flex',alignItems:'center',gap:8,flexWrap:'wrap',padding:'11px 15px'}}>
    <span className="dc4-eyebrow accent" style={{marginRight:2}}>{t('mt.importNow')}</span>
    {cards.map(c=><Chip key={c.k} onClick={()=>setSheet(c.k)}><Icon name={c.icon} size={14}/>{c.t}</Chip>)}
    <span style={{flex:1}}></span>
    <button className="dc4-btn ghost accent sm" style={{height:28,padding:'0 9px',fontSize:12.5}} onClick={()=>setHistOpen(true)}><Icon name="clock" size={14}/>{t('mt.history')}</button>
    <Sheet open={!!sheet} onClose={()=>setSheet(null)} title={sheet?cards.find(c=>c.k===sheet).t:''}>
      {sheet&&ISB?<ISB kind={sheet} onDone={n=>{setSheet(null);DcUI.toast(t('ob.parsed',{n}),'ok');onChanged&&onChanged()}}/>:null}
    </Sheet>
    <Sheet open={histOpen} onClose={()=>setHistOpen(false)} title={t('mt.history')}><HistoryList/></Sheet>
  </div>}
function Materials(){
  const {t}=useI18n();const[tab,setTab]=useState('inbox');const[ver,setVer]=useState(0);
  const Body={inbox:InboxTab,courses:CoursesTab,assignments:AssignmentsTab,rules:RulesTab}[tab];
  return <div className="dc4-stack dc4-casc" style={{gap:18}}>
    <header><h1 className="dc4-display">{t('mt.h')}</h1><div className="dc4-cap" style={{marginTop:5}}>{t('mt.sub')}</div></header>
    <ImportBar onChanged={()=>setVer(v=>v+1)}/>
    <Seg block value={tab} onChange={setTab} options={['inbox','courses','assignments','rules'].map(k=>({value:k,label:t('mt.tab.'+k)}))}/>
    <div key={tab+'-'+ver} className="dc4-casc"><Body/></div>
  </div>}
DcPages.Materials=Materials;
})();
