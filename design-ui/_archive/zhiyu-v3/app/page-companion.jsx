/* Companion — streaming chat with tool strips & decision cards */
(function(){
const {Button,IconBtn,Icon,Sheet,Tag,Typing,Spinner,useI18n,errMsg}=DcUI;
const {useState,useEffect,useRef,Fragment}=React;
DcI18n.add({
 'cp.ph':{zh:'和小日说点什么…',en:'Say something…'},
 'cp.new':{zh:'新对话',en:'New chat'},'cp.threads':{zh:'对话',en:'Conversations'},
 'cp.accept':{zh:'采纳',en:'Apply'},'cp.later':{zh:'先不用',en:'Not now'},
 'cp.accepted':{zh:'已采纳',en:'Applied'},'cp.dismissed':{zh:'好的，先不动',en:'OK, unchanged'},
 'cp.viewToday':{zh:'查看今日',en:'View Today'},
 'cp.sug1':{zh:'帮我把今晚排松一点',en:'Make tonight lighter'},'cp.sug2':{zh:'明天怎么安排比较好？',en:'Plan my tomorrow'},'cp.sug3':{zh:'我最近有点累',en:'I feel tired lately'},
 'cp.online':{zh:'在这儿',en:'here with you'},
});
function DecisionCard({card,busy,onAccept,onDismiss}){
  const {t}=useI18n();
  return <div className="dc4-decision">
    <div style={{display:'flex',alignItems:'center',gap:9,marginBottom:9}}>
      <span className="dc4-icobox" style={{width:28,height:28,borderRadius:8}}><Icon name="wand" size={15}/></span>
      <span style={{fontWeight:650,fontSize:14.5,flex:1,minWidth:0}}>{card.title}</span><Tag tone="accent">{DcUI.fmtDate(card.date)}</Tag></div>
    <div style={{display:'flex',flexDirection:'column',gap:5,marginBottom:11}}>
      {card.changes.map((ch,i)=><div key={i} className="dc4-cap" style={{display:'flex',gap:8,alignItems:'baseline'}}>
        <span className="dc4-mono" style={{color:ch.action==='add'?'var(--dc-ok)':'var(--dc-err)',fontWeight:700}}>{ch.action==='add'?'+':'−'}</span>
        <span className="dc4-num">{ch.action==='add'?ch.block.start+'–'+ch.block.end+' · '+ch.block.title:(ch.label||'')}</span></div>)}
    </div>
    {card.status==='pending'&&onAccept?<div style={{display:'flex',gap:8,justifyContent:'flex-end'}}>
      <Button size="sm" variant="ghost" disabled={busy} onClick={onDismiss}>{t('cp.later')}</Button>
      <Button size="sm" loading={busy} onClick={onAccept}>{t('cp.accept')}</Button></div>
    :card.status==='accepted'?<div style={{display:'flex',alignItems:'center',gap:8,justifyContent:'flex-end'}}><Tag tone="ok" dot>{t('cp.accepted')}</Tag><Button size="sm" variant="ghost accent" onClick={()=>dcNav('#/today')}>{t('cp.viewToday')}</Button></div>
    :card.status==='dismissed'?<div style={{textAlign:'right'}}><Tag>{t('cp.dismissed')}</Tag></div>:null}
  </div>}
function ToolStrip({tool}){
  return <div className="dc4-procstrip">{tool.done?<Icon name="check" size={14} style={{color:'var(--dc-ok)'}}/>:<Spinner size={13}/>}<span>{tool.label}{tool.done&&tool.result?' — '+tool.result:''}</span></div>}
function Companion(){
  const {t}=useI18n();const app=React.useContext(DcAppCtx);
  const[threads,setThreads]=useState(null);const[tid,setTid]=useState(null);
  const[msgs,setMsgs]=useState([]);const[live,setLive]=useState(null);const[input,setInput]=useState('');
  const[listOpen,setListOpen]=useState(false);const[busyCard,setBusyCard]=useState('');
  const scrollRef=useRef(null);
  const aname=(app.session.profile&&app.session.profile.assistantName)||'小日';
  const load=async pickId=>{const th=await DcStore.listThreads();setThreads(th);
    const cur=th.find(x=>x.id===(pickId||tid))||th[0];
    if(cur){setTid(cur.id);setMsgs(cur.messages)}else{setTid(null);setMsgs([])}};
  useEffect(()=>{load()},[]);
  useEffect(()=>{const el=scrollRef.current;if(el)el.scrollTop=el.scrollHeight},[msgs.length,live&&live.content,live&&live.tools&&live.tools.length]);
  const send=async raw=>{const text=(raw!=null?raw:input).trim();if(!text||live||!tid)return;setInput('');
    setMsgs(m=>[...m,{id:'u'+Date.now(),role:'user',content:text,ts:new Date().toISOString()}]);
    setLive({tools:[],content:'',card:null});
    try{await DcStore.sendMessage(tid,text,f=>{
      if(f.t==='tool_start')setLive(l=>l&&{...l,tools:[...l.tools,{label:f.label,done:false}]});
      else if(f.t==='tool_result')setLive(l=>l&&{...l,tools:l.tools.map(x=>x.label===f.label?{...x,done:true,result:f.result}:x)});
      else if(f.t==='delta')setLive(l=>l&&{...l,content:f.content});
      else if(f.t==='decision_card')setLive(l=>l&&{...l,card:f.card});
      else if(f.t==='done'){setMsgs(m=>[...m,f.message]);setLive(null)}
    })}catch(e){setLive(null);DcUI.toast(errMsg(e),'err')}};
  const decide=async(m,ok)=>{setBusyCard(m.id);
    try{if(ok){const r=await DcStore.applyDecision(tid,m.id);DcUI.toast(r.summary||t('cp.accepted'),'ok')}else await DcStore.dismissDecision(tid,m.id);
      setMsgs(x=>x.map(y=>y.id===m.id?{...y,card:{...y.card,status:ok?'accepted':'dismissed'}}:y))}
    catch(e){DcUI.toast(errMsg(e),'err')}finally{setBusyCard('')}};
  const newThread=async()=>{const th=await DcStore.newThread();await load(th.id);setListOpen(false)};
  const delThread=async th=>{if(await DcUI.confirm({title:t('c.delete')+'「'+th.title+'」？',danger:true,confirmLabel:t('c.delete')})){await DcStore.deleteThread(th.id);await load(th.id===tid?null:tid)}};
  const curThread=(threads||[]).find(x=>x.id===tid);
  return <div className="dc4-chatpage">
    <header style={{display:'flex',alignItems:'center',gap:13,marginBottom:14,flex:'none'}}>
      <div className="dc4-orb breathe-idle" style={{width:46,height:46,flexShrink:0}}></div>
      <div style={{flex:1,minWidth:0}}>
        <h1 className="dc4-h1">{aname}</h1>
        <div className="dc4-cap-3" style={{display:'flex',alignItems:'center',gap:6}}><span className="dc4-dot ok"></span>{t('cp.online')}{curThread?' · '+curThread.title:''}</div>
      </div>
      <IconBtn name="plus" solid label={t('cp.new')} onClick={newThread}/>
      <IconBtn name="dots" solid label={t('cp.threads')} onClick={async()=>{const th=await DcStore.listThreads();setThreads(th);setListOpen(true)}}/>
    </header>
    <div className="dc4-chat dc4-chatscroll" ref={scrollRef}>
      {msgs.map(m=><div style={{display:'contents'}} key={m.id}>
        <div className={'dc4-bubble '+(m.role==='user'?'user':'ai')}>{m.content}</div>
        {m.card?<DecisionCard card={m.card} busy={busyCard===m.id} onAccept={()=>decide(m,true)} onDismiss={()=>decide(m,false)}/>:null}
      </div>)}
      {live&&<div style={{display:'contents'}}>
        {live.tools.map((tool,i)=><ToolStrip key={i} tool={tool}/>)}
        {live.content?<div className="dc4-bubble ai">{live.content}</div>:live.tools.every(x=>x.done)?<div className="dc4-bubble ai"><Typing/></div>:null}
        {live.card?<DecisionCard card={live.card} busy={true}/>:null}
      </div>}
      {!live&&msgs.length<=1&&<div style={{display:'flex',gap:8,flexWrap:'wrap',marginTop:6}}>
        {['cp.sug1','cp.sug2','cp.sug3'].map(k=><DcUI.Chip key={k} onClick={()=>send(t(k))}>{t(k)}</DcUI.Chip>)}
      </div>}
    </div>
    <div className="dc4-composer inflow">
      <textarea rows={1} value={input} placeholder={t('cp.ph')} onChange={e=>setInput(e.target.value)}
        onKeyDown={e=>{if(e.key==='Enter'&&!e.shiftKey){e.preventDefault();send()}}}
        onInput={e=>{e.target.style.height='auto';e.target.style.height=Math.min(120,e.target.scrollHeight)+'px'}}></textarea>
      <button className="dc4-sendbtn" aria-label="send" disabled={!input.trim()||!!live} onClick={()=>send()}><Icon name="send" size={17}/></button>
    </div>
    <Sheet open={listOpen} onClose={()=>setListOpen(false)} title={t('cp.threads')}>
      <div style={{display:'flex',flexDirection:'column',gap:4}}>
        {(threads||[]).map(th=><div key={th.id} className="dc4-row" style={{cursor:'pointer'}} onClick={()=>{setTid(th.id);setMsgs(th.messages);setListOpen(false)}}>
          <span className="dc4-icobox" style={th.id===tid?null:{background:'var(--dc-surface-2)',color:'var(--dc-ink-3)'}}><Icon name="chat" size={16}/></span>
          <div className="grow"><div className="t">{th.title}</div><div className="s dc4-num">{th.messages.length} · {th.createdAt.slice(5,10)}</div></div>
          <IconBtn name="trash" label={t('c.delete')} onClick={e=>{e.stopPropagation();delThread(th)}}/>
        </div>)}
        <Button variant="sec" icon="plus" onClick={newThread} style={{marginTop:8}}>{t('cp.new')}</Button>
      </div></Sheet>
  </div>}
DcPages.Companion=Companion;
})();
