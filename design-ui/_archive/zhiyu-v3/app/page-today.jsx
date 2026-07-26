/* Today — greeting hero + auto-plan CTA + week strip + timeline */
(function(){
const {Button,IconBtn,Icon,Field,Input,Textarea,Seg,Sheet,Tag,Ring,Empty,useI18n,useAsync,errMsg}=DcUI;
const {useState,useEffect,useContext,Fragment}=React;
DcI18n.add({
 'td.plan':{zh:'自主规划',en:'Auto-plan'},'td.planSub':{zh:'让小日为你安排',en:'Let Daycore arrange it'},
 'td.progress':{zh:'今日进度',en:'Today'},'td.doneOf':{zh:'{d} / {n} 完成',en:'{d} of {n} done'},
 'td.range':{zh:'规划范围',en:'Range'},'td.r.today':{zh:'今天',en:'Today'},'td.r.tomorrow':{zh:'明天',en:'Tomorrow'},'td.r.3':{zh:'未来 3 天',en:'Next 3 days'},'td.r.7':{zh:'未来 7 天',en:'Next 7 days'},'td.r.custom':{zh:'自定义',en:'Custom'},
 'td.mode':{zh:'已有安排',en:'Existing blocks'},'td.m.keep':{zh:'保留手动安排',en:'Keep manual'},'td.m.replace':{zh:'全部重排',en:'Replan all'},
 'td.instr':{zh:'想怎么安排？',en:'Any instructions?'},'td.instrPh':{zh:'例如：今晚想早睡；给复习多留点时间；周三下午空出来…',en:'e.g. early night today; more review time…'},
 'td.go':{zh:'开始规划',en:'Plan it'},'td.planning':{zh:'规划中',en:'Planning'},
 'td.p1':{zh:'读取课表与作业…',en:'Reading courses & assignments…'},'td.p2':{zh:'结合你的规则与作息…',en:'Applying your rules & rhythm…'},'td.p3':{zh:'生成时间块…',en:'Laying out time blocks…'},
 'td.now':{zh:'现在',en:'now'},'td.empty':{zh:'这一天还是空白',en:'A blank day'},'td.emptySub':{zh:'点击上方「自主规划」，或手动添加一个时间块。',en:'Use Auto-plan above, or add a block by hand.'},
 'td.addBlock':{zh:'添加时间块',en:'Add block'},'td.editBlock':{zh:'编辑时间块',en:'Edit block'},
 'td.title':{zh:'标题',en:'Title'},'td.start':{zh:'开始',en:'Start'},'td.end':{zh:'结束',en:'End'},'td.note':{zh:'备注',en:'Note'},'td.type':{zh:'类型',en:'Type'},
 'td.t.course':{zh:'课程',en:'Class'},'td.t.task':{zh:'任务',en:'Task'},'td.t.meal':{zh:'餐',en:'Meal'},'td.t.break':{zh:'休息',en:'Break'},'td.t.exercise':{zh:'运动',en:'Move'},'td.t.custom':{zh:'其他',en:'Other'},
 'td.lock':{zh:'锁定（规划时不移动）',en:'Lock (auto-plan keeps it)'},
 'td.src.auto':{zh:'自动',en:'auto'},'td.src.manual':{zh:'手动',en:'manual'},'td.src.companion':{zh:'陪伴',en:'companion'},
 'td.undone':{zh:'已撤销',en:'Undone'},'td.backToday':{zh:'回到今天',en:'Back to today'},
});
const TYPE_BAR={course:'var(--dc-accent)',task:'var(--dc-warn)',meal:'var(--dc-ink-3)',break:'var(--dc-ok)',exercise:'var(--dc-accent-2)',custom:'var(--dc-ink-2)'};
const nowMin=()=>{const d=new Date();return d.getHours()*60+d.getMinutes()};
const tm=s=>{const[a,b]=s.split(':');return +a*60+ +b};

function BlockSheet({block,date,onClose,onSaved}){
  const {t}=useI18n();const isNew=!block.id;
  const[f,setF]=useState({title:typeof block.title==='string'?block.title:'',start:block.start||'19:00',end:block.end||'20:00',note:block.note||'',type:block.type||'custom',locked:!!block.locked});
  const[busy,setBusy]=useState(false);
  const set=(k,v)=>setF(x=>({...x,[k]:v}));
  const save=async()=>{if(!f.title.trim()||f.start>=f.end)return;setBusy(true);
    await DcStore.upsertBlock({...block,...f,date,title:f.title.trim()});setBusy(false);onSaved()};
  const del=async()=>{if(await DcUI.confirm({title:t('c.delete')+'「'+f.title+'」？',danger:true,confirmLabel:t('c.delete')})){await DcStore.deleteBlock(date,block.id);onSaved()}};
  return <Sheet open onClose={onClose} title={isNew?t('td.addBlock'):t('td.editBlock')} footer={<Fragment>
      {!isNew&&<Button variant="danger soft" onClick={del}>{t('c.delete')}</Button>}
      <span style={{flex:1}}></span><Button variant="ghost" onClick={onClose}>{t('c.cancel')}</Button>
      <Button loading={busy} disabled={!f.title.trim()||f.start>=f.end} onClick={save}>{t('c.save')}</Button></Fragment>}>
    <div style={{display:'flex',flexDirection:'column',gap:14}}>
      <Field label={t('td.title')}><Input value={f.title} onChange={e=>set('title',e.target.value)} autoFocus={isNew}/></Field>
      <div className="dc4-grid2">
        <Field label={t('td.start')}><Input type="time" value={f.start} onChange={e=>set('start',e.target.value)}/></Field>
        <Field label={t('td.end')} error={f.start>=f.end?(DcI18n.locale==='zh'?'需晚于开始时间':'Must be after start'):null}><Input type="time" value={f.end} onChange={e=>set('end',e.target.value)}/></Field>
      </div>
      <Field label={t('td.type')}><div style={{display:'flex',gap:7,flexWrap:'wrap'}}>{Object.keys(TYPE_BAR).map(k=><DcUI.Chip key={k} on={f.type===k} onClick={()=>set('type',k)}><span className="dc4-dot" style={{background:TYPE_BAR[k]}}></span>{t('td.t.'+k)}</DcUI.Chip>)}</div></Field>
      <Field label={t('td.note')}><Input value={f.note} onChange={e=>set('note',e.target.value)}/></Field>
      <div className="dc4-row" style={{border:'none',padding:0}}><span className="grow dc4-body">{t('td.lock')}</span><DcUI.Toggle checked={f.locked} onChange={v=>set('locked',v)}/></div>
    </div></Sheet>}

function PlanSheet({onClose,onDone}){
  const {t}=useI18n();
  const[range,setRange]=useState('today');const[from,setFrom]=useState(DcStore.todayIso());const[to,setTo]=useState(DcStore.addDays(DcStore.todayIso(),2));
  const[mode,setMode]=useState('keep');const[instr,setInstr]=useState('');
  const[phase,setPhase]=useState(0);const[busy,setBusy]=useState(false);
  const dates=()=>{const t0=DcStore.todayIso();
    if(range==='today')return[t0];if(range==='tomorrow')return[DcStore.addDays(t0,1)];
    if(range==='3'||range==='7'){const n=+range;return Array.from({length:n},(_,i)=>DcStore.addDays(t0,i))}
    const out=[];let d=from;let g=0;while(d<=to&&g<14){out.push(d);d=DcStore.addDays(d,1);g++}return out};
  const go=async()=>{setBusy(true);setPhase(1);
    const t1=setTimeout(()=>setPhase(2),900);const t2=setTimeout(()=>setPhase(3),1800);
    try{const r=await DcStore.autoPlan({dates:dates(),instruction:instr,mode});
      clearTimeout(t1);clearTimeout(t2);DcUI.toast(r.summary,'ok');onDone(dates()[0])}
    catch(e){DcUI.toast(errMsg(e),'err');setBusy(false);setPhase(0)}};
  return <Sheet open onClose={busy?null:onClose} title={t('td.plan')}>
    {busy?<div style={{display:'flex',flexDirection:'column',alignItems:'center',gap:18,padding:'26px 0 30px'}}>
      <div className="dc4-orb breathe-idle" style={{width:96,height:96}}></div>
      <div className="dc4-h2">{t('td.planning')}…</div>
      <div className="dc4-cap-3" key={phase}>{t('td.p'+Math.max(1,phase))}</div>
    </div>
    :<div style={{display:'flex',flexDirection:'column',gap:16}}>
      <Field label={t('td.range')}>
        <div style={{display:'flex',gap:7,flexWrap:'wrap'}}>
          {['today','tomorrow','3','7','custom'].map(k=><DcUI.Chip key={k} on={range===k} onClick={()=>setRange(k)}>{t('td.r.'+k)}</DcUI.Chip>)}
        </div></Field>
      {range==='custom'&&<div className="dc4-grid2">
        <Field label={t('td.start')}><Input type="date" value={from} onChange={e=>setFrom(e.target.value)}/></Field>
        <Field label={t('td.end')}><Input type="date" value={to} onChange={e=>setTo(e.target.value)}/></Field></div>}
      <Field label={t('td.mode')}><Seg block value={mode} onChange={setMode} options={[{value:'keep',label:t('td.m.keep')},{value:'replace',label:t('td.m.replace')}]}/></Field>
      <Field label={t('td.instr')}><Textarea rows={3} value={instr} onChange={e=>setInstr(e.target.value)} placeholder={t('td.instrPh')}/></Field>
      <Button size="lg" variant="grad" icon="sparkle" onClick={go}>{t('td.go')}</Button>
    </div>}</Sheet>}

function Today(){
  const {t,locale}=useI18n();const app=useContext(DcAppCtx);
  const[date,setDate]=useState(DcStore.todayIso());
  const[weekOff,setWeekOff]=useState(0);
  const plan=useAsync(()=>DcStore.listPlan(date),[date]);
  const[sheet,setSheet]=useState(null);const[planSheet,setPlanSheet]=useState(false);
  const[undoable,setUndoable]=useState(!!DcStore.lastOpLabel());
  const today=DcStore.todayIso();
  const monday=(()=>{let d=today;while(DcStore.wd(d)!==1)d=DcStore.addDays(d,-1);return DcStore.addDays(d,weekOff*7)})();
  const week=Array.from({length:7},(_,i)=>DcStore.addDays(monday,i));
  const blocks=plan.data||[];
  const done=blocks.filter(b=>b.done).length;
  const hour=new Date().getHours();
  const greet=hour<5?'greet.night':hour<11?'greet.morning':hour<13?'greet.noon':hour<18?'greet.afternoon':'greet.evening';
  const uname=(app.session.user&&app.session.user.name)||(app.session.profile&&app.session.profile.userName)||(locale==='zh'?'朋友':'friend');
  const nm=nowMin();
  let nowIdx=-1;if(date===today){nowIdx=blocks.findIndex(b=>tm(b.start)>nm);if(nowIdx===-1)nowIdx=blocks.length}
  const toggle=async b=>{await DcStore.toggleDone(date,b.id);plan.reload()};
  const undo=async()=>{try{await DcStore.undoLast();setUndoable(false);plan.reload();DcUI.toast(t('td.undone'),'ok')}catch(e){}};
  return <div className="dc4-stack dc4-casc">
    <header style={{display:'flex',alignItems:'center',gap:16,flexWrap:'wrap'}}>
      <div style={{flex:1,minWidth:220}}>
        <div className="dc4-eyebrow accent">{DcUI.fmtDate(today)} · {new Date().getFullYear()}</div>
        <h1 className="dc4-display" style={{marginTop:4}}>{t(greet)}，{uname}</h1>
        <div className="dc4-cap" style={{marginTop:6,display:'flex',alignItems:'center',gap:8}}>
          <span className="dc4-num">{t('td.doneOf',{d:done,n:blocks.length})}</span>
          {undoable&&<button className="dc4-btn ghost accent sm" style={{height:26,padding:'0 9px',fontSize:12}} onClick={undo}><Icon name="undo" size={13}/>{t('c.undo')} · {DcStore.lastOpLabel()}</button>}
        </div>
      </div>
      <div style={{display:'flex',alignItems:'center',gap:16}}>
        <Ring size={62} stroke={5} value={blocks.length?done/blocks.length:0}><span className="dc4-num" style={{fontSize:14,fontWeight:760}}>{blocks.length?Math.round(done/blocks.length*100)+'%':'—'}</span></Ring>
        <Button size="lg" variant="grad" icon="sparkle" onClick={()=>setPlanSheet(true)}>{t('td.plan')}</Button>
      </div>
    </header>
    <section className="dc4-card" style={{padding:'10px 12px'}}>
      <div style={{display:'flex',alignItems:'center',gap:6}}>
        <IconBtn name="chevron-left" label="prev week" onClick={()=>setWeekOff(x=>x-1)}/>
        <div className="dc4-weekstrip" style={{flex:1}}>
          {week.map(d=><button key={d} className={'dc4-day'+(d===date?' on':'')+(d===today?' today':'')} onClick={()=>setDate(d)}>
            <span className="dw">{(locale==='zh'?t('wd.p'):'')+t('wd.'+DcStore.wd(d))}</span><span className="dn">{+d.slice(8)}</span>
          </button>)}
        </div>
        <IconBtn name="chevron-right" label="next week" onClick={()=>setWeekOff(x=>x+1)}/>
      </div>
      {(date!==today||weekOff!==0)&&<div style={{textAlign:'center',marginTop:2}}><button className="dc4-btn ghost accent sm" style={{height:26,fontSize:12}} onClick={()=>{setDate(today);setWeekOff(0)}}>{t('td.backToday')}</button></div>}
    </section>
    <section data-screen-label="timeline">
      {plan.loading?<DcUI.SkeletonCard lines={4}/>
      :blocks.length===0?<div className="dc4-card"><Empty icon="sun" title={t('td.empty')} desc={t('td.emptySub')} action={<Button icon="plus" variant="sec" onClick={()=>setSheet({date})}>{t('td.addBlock')}</Button>}/></div>
      :<div className="dc4-tl">
        {blocks.map((b,i)=><div style={{display:'contents'}} key={b.id}>
          {i===nowIdx&&<div className="dc4-nowline"><span className="dotp"></span><span className="dc4-eyebrow accent" style={{letterSpacing:'.04em'}}>{t('td.now')} {DcUI.fmtTs(new Date())}</span><span className="ln"></span></div>}
          <div className="dc4-block">
            <div className="time"><span className="dc4-num">{b.start}</span><span className="end dc4-num">{b.end}</span></div>
            <div className="body"><div className="dc4-blockcard" onClick={()=>setSheet(b)}>
              <span className="bar" style={{background:TYPE_BAR[b.type]||TYPE_BAR.custom}}></span>
              <div style={{flex:1,minWidth:0}}>
                <div className="t" style={{fontWeight:650,fontSize:14.5,display:'flex',alignItems:'center',gap:7}}>{typeof b.title==='string'?b.title:DcI18n.pick(b.title)}{b.locked&&<Icon name="lock" size={13} style={{color:'var(--dc-ink-3)'}}/>}</div>
                {(b.note||b.source!=='auto')&&<div className="dc4-cap-3" style={{display:'flex',gap:7,alignItems:'center',marginTop:2}}>{b.note?<span style={{overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{b.note}</span>:null}{b.source!=='auto'&&<Tag tone={b.source==='companion'?'accent':''}>{t('td.src.'+b.source)}</Tag>}</div>}
              </div>
              <button className={'dc4-check'+(b.done?' on':'')} aria-label="done" onClick={e=>{e.stopPropagation();toggle(b)}}><Icon name="check" size={14}/></button>
            </div></div>
          </div>
        </div>)}
        {nowIdx===blocks.length&&date===today&&<div className="dc4-nowline"><span className="dotp"></span><span className="dc4-eyebrow accent">{t('td.now')} {DcUI.fmtTs(new Date())}</span><span className="ln"></span></div>}
        <div style={{paddingLeft:66}}><Button variant="ghost accent" icon="plus" size="sm" onClick={()=>setSheet({date})}>{t('td.addBlock')}</Button></div>
      </div>}
    </section>
    {sheet&&<BlockSheet block={sheet} date={date} onClose={()=>setSheet(null)} onSaved={()=>{setSheet(null);plan.reload()}}/>}
    {planSheet&&<PlanSheet onClose={()=>setPlanSheet(false)} onDone={d=>{setPlanSheet(false);setUndoable(true);setDate(d);plan.reload()}}/>}
  </div>}
DcPages.Today=Today;
})();
