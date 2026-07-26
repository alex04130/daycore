/* Mood — check-in grid, reply card, exercises (breathe / stretch / ground), journal */
(function(){
const {Button,IconBtn,Icon,Textarea,Tag,Ring,useI18n,useAsync}=DcUI;
const {useState,useEffect,useRef,Fragment}=React;
DcI18n.add({
 'md.h':{zh:'心情',en:'Mood'},'md.q':{zh:'现在感觉怎么样？',en:'How do you feel right now?'},
 'md.note':{zh:'想说点什么吗？（可选）',en:'Anything to add? (optional)'},'md.save':{zh:'记录这一刻',en:'Log this moment'},
 'md.ex':{zh:'小练习',en:'Small exercises'},
 'md.ex.breathe':{zh:'呼吸 4-7-8',en:'Breathe 4-7-8'},'md.ex.breatheSub':{zh:'两分钟，让身体先松下来',en:'Two minutes to settle'},
 'md.ex.stretch':{zh:'舒展一下',en:'Stretch'},'md.ex.stretchSub':{zh:'60 秒肩颈放松',en:'60s neck & shoulders'},
 'md.ex.ground':{zh:'着地 5-4-3-2-1',en:'Ground 5-4-3-2-1'},'md.ex.groundSub':{zh:'把注意力带回当下',en:'Come back to now'},
 'md.history':{zh:'心情手账',en:'Journal'},
 'md.in':{zh:'吸气',en:'Breathe in'},'md.hold':{zh:'屏住',en:'Hold'},'md.out':{zh:'缓缓呼气',en:'Breathe out'},
 'md.round':{zh:'第 {n} 轮 · 建议 4 轮',en:'Round {n} · aim for 4'},
 'md.doneT':{zh:'做完啦',en:'All done'},'md.doneSub':{zh:'感觉好些了吗？慢慢来，不着急。',en:'Feeling a bit lighter? No rush.'},
 'md.next':{zh:'下一步',en:'Next'},'md.finish':{zh:'完成',en:'Finish'},
 'md.s1':{zh:'颈部缓慢左右转动',en:'Slow neck turns'},'md.s2':{zh:'双肩向后绕环',en:'Roll shoulders back'},'md.s3':{zh:'双手上举，向两侧伸展',en:'Reach up and to the sides'},
 'md.g5':{zh:'说出你能看到的 5 样东西',en:'Name 5 things you can see'},'md.g4':{zh:'4 样你能触摸到的东西',en:'4 things you can touch'},'md.g3':{zh:'3 种你能听到的声音',en:'3 sounds you can hear'},'md.g2':{zh:'2 种你能闻到的气味',en:'2 things you can smell'},'md.g1':{zh:'1 件让你感激的小事',en:'1 thing you are grateful for'},
 'md.m.happy':{zh:'开心',en:'Happy'},'md.m.calm':{zh:'平静',en:'Calm'},'md.m.excited':{zh:'兴奋',en:'Excited'},'md.m.grateful':{zh:'感激',en:'Grateful'},'md.m.okay':{zh:'一般',en:'Okay'},'md.m.bored':{zh:'无聊',en:'Bored'},'md.m.tired':{zh:'疲惫',en:'Tired'},'md.m.stressed':{zh:'压力',en:'Stressed'},'md.m.anxious':{zh:'焦虑',en:'Anxious'},'md.m.sad':{zh:'难过',en:'Sad'},'md.m.angry':{zh:'生气',en:'Angry'},'md.m.lonely':{zh:'孤单',en:'Lonely'},
});
const MOODS=[['happy','😊'],['calm','😌'],['excited','🤩'],['grateful','🥰'],['okay','🙂'],['bored','😑'],['tired','😪'],['stressed','😵‍💫'],['anxious','😰'],['sad','😢'],['angry','😠'],['lonely','🫥']];
const EMOJI=Object.fromEntries(MOODS);
function ExShell({onClose,children}){
  const {t}=useI18n();
  return <div className="dc4-exwrap" role="dialog" aria-modal="true">
    <IconBtn name="x" label={t('c.close')} solid onClick={onClose} style={{position:'absolute',top:'max(18px,env(safe-area-inset-top))',right:18}}/>
    {children}</div>}
function DoneView({onClose}){
  const {t}=useI18n();
  return <Fragment>
    <div className="dc4-orb" style={{width:120,height:120}}></div>
    <div style={{textAlign:'center'}}><div className="dc4-display">{t('md.doneT')}</div><div className="dc4-cap" style={{marginTop:8}}>{t('md.doneSub')}</div></div>
    <Button size="lg" onClick={onClose}>{t('md.finish')}</Button></Fragment>}
function Breathe({onClose}){
  const {t}=useI18n();
  const PH=[{k:'in',d:4},{k:'hold',d:7},{k:'out',d:8}];
  const[pi,setPi]=useState(0);const[left,setLeft]=useState(4);const[round,setRound]=useState(1);
  useEffect(()=>{let p=0,l=PH[0].d,r=1;
    const iv=setInterval(()=>{l--;if(l<=0){p=(p+1)%3;if(p===0)r++;l=PH[p].d;setPi(p);setRound(r)}setLeft(l)},1000);
    return()=>clearInterval(iv)},[]);
  if(round>4)return <ExShell onClose={onClose}><DoneView onClose={onClose}/></ExShell>;
  return <ExShell onClose={onClose}>
    <div className="dc4-orb" style={{transform:pi===2?'scale(1)':'scale(1.34)',transitionDuration:PH[pi].d+'s'}}></div>
    <div style={{textAlign:'center'}}>
      <div className="dc4-display" style={{fontSize:26}}>{t('md.'+PH[pi].k)}</div>
      <div className="dc4-stat" style={{fontSize:44,marginTop:6,color:'var(--dc-accent)'}}>{Math.max(left,0)||PH[pi].d}</div>
      <div className="dc4-cap-3" style={{marginTop:10}}>{t('md.round',{n:round})}</div>
    </div></ExShell>}
function Stretch({onClose}){
  const {t}=useI18n();
  const steps=['md.s1','md.s2','md.s3'];const D=20;
  const[si,setSi]=useState(0);const[el,setEl]=useState(0);
  useEffect(()=>{const iv=setInterval(()=>setEl(x=>x+1),1000);return()=>clearInterval(iv)},[]);
  const total=steps.length*D;
  if(el>=total)return <ExShell onClose={onClose}><DoneView onClose={onClose}/></ExShell>;
  const idx=Math.min(steps.length-1,Math.floor(el/D));const leftS=D-(el%D);
  return <ExShell onClose={onClose}>
    <Ring size={190} stroke={9} value={el/total}><div style={{textAlign:'center'}}><div className="dc4-stat" style={{fontSize:42}}>{leftS}</div><div className="dc4-cap-3">{idx+1} / {steps.length}</div></div></Ring>
    <div style={{textAlign:'center'}}><div className="dc4-h1">{t(steps[idx])}</div></div>
  </ExShell>}
function Ground({onClose}){
  const {t}=useI18n();
  const steps=['md.g5','md.g4','md.g3','md.g2','md.g1'];
  const[si,setSi]=useState(0);
  if(si>=steps.length)return <ExShell onClose={onClose}><DoneView onClose={onClose}/></ExShell>;
  return <ExShell onClose={onClose}>
    <div style={{display:'flex',gap:10}}>{steps.map((_,i)=><span key={i} style={{width:12,height:12,borderRadius:'50%',background:i<=si?'var(--dc-accent)':'color-mix(in srgb,var(--dc-ink) 14%,transparent)',boxShadow:i===si?'0 0 0 6px var(--dc-accent-soft)':'none',transition:'all .3s var(--dc-ease)'}}></span>)}</div>
    <div style={{textAlign:'center',maxWidth:340}}>
      <div className="dc4-stat" style={{color:'var(--dc-accent)'}}>{5-si}</div>
      <div className="dc4-h1" style={{marginTop:6}}>{t(steps[si])}</div>
    </div>
    <Button size="lg" onClick={()=>setSi(x=>x+1)}>{si===steps.length-1?t('md.finish'):t('md.next')}</Button>
  </ExShell>}
function Mood(){
  const {t}=useI18n();
  const moods=useAsync(()=>DcStore.listMoods(),[]);
  const[sel,setSel]=useState(null);const[note,setNote]=useState('');const[busy,setBusy]=useState(false);const[reply,setReply]=useState(null);
  const[ex,setEx]=useState(null);
  const noteRef=useRef(null);
  const submit=async()=>{setBusy(true);const r=await DcStore.addMood({mood:sel,note:note.trim()});setBusy(false);setReply(r);setSel(null);setNote('');moods.reload()};
  const exs=[{k:'breathe',icon:'heart'},{k:'stretch',icon:'dumbbell'},{k:'ground',icon:'flag'}];
  return <div className="dc4-stack dc4-casc" style={{gap:20}}>
    <header><h1 className="dc4-display">{t('md.h')}</h1><div className="dc4-cap" style={{marginTop:5}}>{t('md.q')}</div></header>
    {reply&&<div className="dc4-card" style={{display:'flex',gap:13,alignItems:'flex-start',border:'1.5px solid color-mix(in srgb,var(--dc-accent) 35%,transparent)'}}>
      <div className="dc4-orb" style={{width:38,height:38,flexShrink:0}}></div>
      <div style={{flex:1}}><div style={{fontSize:22,lineHeight:'26px',marginBottom:5}}>{EMOJI[reply.mood]}</div><div className="dc4-body">{reply.reply}</div></div>
      <IconBtn name="x" label={t('c.close')} onClick={()=>setReply(null)}/></div>}
    <section className="dc4-moodgrid">
      {MOODS.map(([k,e])=><button key={k} className={'dc4-moodtile'+(sel===k?' on':'')} onClick={()=>{setSel(sel===k?null:k);setReply(null)}}>
        <span className="e">{e}</span><span className="l">{t('md.m.'+k)}</span></button>)}
    </section>
    {sel&&<div className="dc4-card" ref={noteRef}>
      <Textarea rows={2} value={note} onChange={e=>setNote(e.target.value)} placeholder={t('md.note')}/>
      <div style={{display:'flex',justifyContent:'flex-end',marginTop:10}}><Button icon="heart" loading={busy} onClick={submit}>{t('md.save')}</Button></div></div>}
    <section>
      <div className="dc4-eyebrow" style={{marginBottom:10}}>{t('md.ex')}</div>
      <div className="dc4-grid3">
        {exs.map(x=><div key={x.k} className="dc4-card hover" style={{display:'flex',alignItems:'center',gap:12,padding:'14px 15px'}} onClick={()=>setEx(x.k)}>
          <span className="dc4-icobox"><Icon name={x.icon} size={17}/></span>
          <div style={{flex:1,minWidth:0}}><div style={{fontWeight:650,fontSize:14}}>{t('md.ex.'+x.k)}</div><div className="dc4-cap-3">{t('md.ex.'+x.k+'Sub')}</div></div>
          <Icon name="play" size={15} style={{color:'var(--dc-accent)'}}/></div>)}
      </div>
    </section>
    <section>
      <div className="dc4-eyebrow" style={{marginBottom:10}}>{t('md.history')}</div>
      {moods.loading?<DcUI.SkeletonCard lines={3}/>:<div className="dc4-card">
        {(moods.data||[]).slice(0,10).map(m=><div key={m.id} className="dc4-row" style={{alignItems:'flex-start'}}>
          <span style={{fontSize:24,lineHeight:'30px'}}>{EMOJI[m.mood]||'🙂'}</span>
          <div className="grow">
            <div className="s dc4-num" style={{marginBottom:1}}>{DcUI.fmtDate(m.date)} · {DcUI.fmtTs(m.ts)}</div>
            {m.note?<div className="t" style={{fontWeight:500}}>{m.note}</div>:null}
            <div className="s" style={{marginTop:2,color:'var(--dc-ink-2)'}}>{m.reply}</div>
          </div></div>)}
        {(moods.data||[]).length===0&&<div className="dc4-cap-3">—</div>}
      </div>}
    </section>
    {ex==='breathe'&&<Breathe onClose={()=>setEx(null)}/>}
    {ex==='stretch'&&<Stretch onClose={()=>setEx(null)}/>}
    {ex==='ground'&&<Ground onClose={()=>setEx(null)}/>}
  </div>}
DcPages.Mood=Mood;
})();
