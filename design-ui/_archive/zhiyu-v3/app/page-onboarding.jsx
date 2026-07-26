/* Onboarding — 3 steps: welcome → materials (optional) → rhythm + first plan */
(function(){
const {Button,Icon,Field,Input,Textarea,Seg,useI18n,errMsg}=DcUI;
const {useState}=React;
DcI18n.add({
 'ob.hi':{zh:'你好呀',en:'Hello there'},'ob.tag':{zh:'把每一天交给会照顾你的计划',en:'Days that plan themselves, gently'},
 'ob.sub':{zh:'Daycore 会记住你的课表、作业和习惯，自动为你安排每一天——你只需要告诉它感受和想法。',en:'Daycore remembers your classes, deadlines and habits, then plans each day for you.'},
 'ob.start':{zh:'开始',en:'Get started'},'ob.callme':{zh:'怎么称呼你？',en:'What should we call you?'},
 'ob.import':{zh:'把资料交给我',en:'Hand over your materials'},'ob.importSub':{zh:'可以现在导入，也可以之后在「资料」页随时补充。这一步完全可以跳过。',en:'Import now, or add later on the Materials page. Totally skippable.'},
 'ob.canvas':{zh:'Canvas 作业',en:'Canvas'},'ob.canvasSub':{zh:'粘贴导出的 JSON',en:'Paste exported JSON'},
 'ob.ics':{zh:'日历 ICS',en:'Calendar ICS'},'ob.icsSub':{zh:'粘贴 .ics 内容',en:'Paste .ics text'},
 'ob.shot':{zh:'课表截图',en:'Timetable photo'},'ob.shotSub':{zh:'AI 识别课程',en:'AI reads courses'},
 'ob.demoNote':{zh:'演示数据已就位：5 门课程 · 5 项作业',en:'Demo data ready: 5 courses · 5 assignments'},
 'ob.rhythm':{zh:'你的作息',en:'Your rhythm'},'ob.rhythmSub':{zh:'告诉我大概的节奏，规划会顺着它来。',en:'Planning follows your natural rhythm.'},
 'ob.wake':{zh:'起床',en:'Wake'},'ob.sleep':{zh:'入睡',en:'Sleep'},
 'ob.focus':{zh:'什么时候效率最高？',en:'When do you focus best?'},'ob.morning':{zh:'上午',en:'Morning'},'ob.afternoon':{zh:'下午',en:'Afternoon'},'ob.evening':{zh:'晚上',en:'Evening'},
 'ob.go':{zh:'生成今天的计划',en:'Plan my day'},'ob.going':{zh:'正在为你安排今天…',en:'Planning your day…'},
 'ob.parse':{zh:'解析',en:'Parse'},'ob.parsed':{zh:'已导入 {n} 条',en:'{n} imported'},
});
function ImportSheetBody({kind,onDone}){
  const {t}=useI18n();const[text,setText]=useState('');const[busy,setBusy]=useState(false);const[err,setErr]=useState(null);const[shot,setShot]=useState(null);
  const run=async()=>{setBusy(true);setErr(null);
    try{
      if(kind==='canvas'){const r=await DcStore.importCanvas(text);onDone(r.added)}
      else if(kind==='ics'){const r=await DcStore.importICS(text);onDone(r.added)}
      else{const r=await DcStore.importScreenshot();setShot(r.preview)}
    }catch(e){setErr(e)}finally{setBusy(false)}};
  const applyShot=async()=>{setBusy(true);const r=await DcStore.applyImportedCourses(shot);setBusy(false);onDone(r.added)};
  if(kind==='shot'&&shot)return <div style={{display:'flex',flexDirection:'column',gap:10}}>
    {shot.map((c,i)=><div key={i} className="dc4-row"><span className="dc4-icobox"><Icon name="book" size={17}/></span><div className="grow"><div className="t">{c.name}</div><div className="s">{'周'+'一二三四五六日'[c.day-1]+' '+c.start+'–'+c.end+' · '+c.room}</div></div></div>)}
    <Button loading={busy} onClick={applyShot}>{t('c.confirm')}</Button></div>;
  return <div style={{display:'flex',flexDirection:'column',gap:12}}>
    {kind==='shot'
      ?<div className="dc4-empty" style={{padding:'20px 10px'}}><span className="ico"><Icon name="image" size={38}/></span><div className="dc4-cap-3">{DcI18n.locale==='zh'?'选择一张课表截图（演示环境将使用示例识别结果）':'Pick a timetable photo (demo uses a sample result)'}</div></div>
      :<Textarea rows={7} value={text} onChange={e=>setText(e.target.value)} placeholder={kind==='canvas'?'{"assignments":[{"name":"实验三","due_at":"2026-07-20","course":"数据结构"}]}':'BEGIN:VEVENT\nSUMMARY:班会\nDTSTART:20260722T190000\nEND:VEVENT'} className="dc4-mono" style={{fontFamily:'var(--dc-mono)',fontSize:12.5}}/>}
    {err?<div className="dc4-ferr">{errMsg(err)}</div>:null}
    <Button loading={busy} disabled={kind!=='shot'&&!text.trim()} onClick={run}>{kind==='shot'?(DcI18n.locale==='zh'?'开始识别':'Recognize'):t('ob.parse')}</Button>
  </div>}
function Onboarding(){
  const {t}=useI18n();const app=React.useContext(DcAppCtx);
  const[step,setStep]=useState(0);const[name,setName]=useState('');
  const[sheet,setSheet]=useState(null);const[importedN,setImportedN]=useState(0);
  const[wake,setWake]=useState('07:30');const[sleep,setSleep]=useState('23:30');const[focus,setFocus]=useState('morning');
  const[busy,setBusy]=useState(false);
  const finish=async()=>{setBusy(true);
    await DcStore.saveProfile({assistantName:'小日',wake,sleep,focus});
    if(name.trim())await DcStore.saveProfile({userName:name.trim()});
    await DcStore.autoPlan({dates:[DcStore.todayIso()],instruction:'',mode:'replace'});
    await DcStore.setOnboarded(true);await app.refresh();dcNav('#/today')};
  const cards=[{k:'canvas',icon:'file',t:t('ob.canvas'),s:t('ob.canvasSub')},{k:'ics',icon:'calendar',t:t('ob.ics'),s:t('ob.icsSub')},{k:'shot',icon:'image',t:t('ob.shot'),s:t('ob.shotSub')}];
  return <div className="dc4-ob" data-screen-label="Onboarding">
    <div className="inner" key={step}>
      {step===0&&<div style={{display:'contents'}}>
        <div className="dc4-orb breathe-idle" style={{width:120,height:120,margin:'0 auto 14px'}}></div>
        <div className="dc4-eyebrow accent">Daycore</div>
        <h1 className="dc4-display" style={{fontSize:36,lineHeight:'44px'}}>{t('ob.tag')}</h1>
        <p className="dc4-body" style={{color:'var(--dc-ink-2)'}}>{t('ob.sub')}</p>
        <Field label={t('ob.callme')}><Input value={name} onChange={e=>setName(e.target.value)} placeholder="Yun" style={{textAlign:'center'}}/></Field>
        <Button size="lg" variant="grad" onClick={()=>setStep(1)}>{t('ob.start')}</Button>
      </div>}
      {step===1&&<div style={{display:'contents'}}>
        <div className="dc4-eyebrow accent">1 / 2</div>
        <h1 className="dc4-display">{t('ob.import')}</h1>
        <p className="dc4-body" style={{color:'var(--dc-ink-2)'}}>{t('ob.importSub')}</p>
        <div style={{display:'flex',flexDirection:'column',gap:10,textAlign:'left'}}>
          {cards.map(c=><div key={c.k} className="dc4-card hover" style={{display:'flex',alignItems:'center',gap:12,padding:'13px 15px'}} onClick={()=>setSheet(c.k)}>
            <span className="dc4-icobox"><Icon name={c.icon} size={17}/></span>
            <div style={{flex:1}}><div style={{fontWeight:650,fontSize:14.5}}>{c.t}</div><div className="dc4-cap-3">{c.s}</div></div>
            <Icon name="chevron-right" size={16} style={{color:'var(--dc-ink-3)'}}/></div>)}
        </div>
        <p className="dc4-cap-3">{importedN>0?t('ob.parsed',{n:importedN}):t('ob.demoNote')}</p>
        <Button size="lg" onClick={()=>setStep(2)}>{t('c.next')}</Button>
        <button className="dc4-btn ghost sm" onClick={()=>setStep(2)}>{t('c.skip')}</button>
      </div>}
      {step===2&&<div style={{display:'contents'}}>
        <div className="dc4-eyebrow accent">2 / 2</div>
        <h1 className="dc4-display">{t('ob.rhythm')}</h1>
        <p className="dc4-body" style={{color:'var(--dc-ink-2)'}}>{t('ob.rhythmSub')}</p>
        <div className="dc4-grid2" style={{textAlign:'left'}}>
          <Field label={t('ob.wake')}><Input type="time" value={wake} onChange={e=>setWake(e.target.value)}/></Field>
          <Field label={t('ob.sleep')}><Input type="time" value={sleep} onChange={e=>setSleep(e.target.value)}/></Field>
        </div>
        <Field label={t('ob.focus')}><Seg block value={focus} onChange={setFocus} options={[{value:'morning',label:t('ob.morning')},{value:'afternoon',label:t('ob.afternoon')},{value:'evening',label:t('ob.evening')}]}/></Field>
        <Button size="lg" variant="grad" loading={busy} icon={busy?null:'sparkle'} onClick={finish}>{busy?t('ob.going'):t('ob.go')}</Button>
      </div>}
      <div className="dc4-obdots">{[0,1,2].map(i=><i key={i} className={i===step?'on':''}></i>)}</div>
    </div>
    <DcUI.Sheet open={!!sheet} onClose={()=>setSheet(null)} title={sheet?cards.find(c=>c.k===sheet).t:''}>
      {sheet?<ImportSheetBody kind={sheet} onDone={n=>{setImportedN(x=>x+n);setSheet(null);DcUI.toast(t('ob.parsed',{n}),'ok')}}/>:null}
    </DcUI.Sheet>
  </div>}
DcPages.Onboarding=Onboarding;
DcPages.ImportSheetBody=ImportSheetBody;
})();
