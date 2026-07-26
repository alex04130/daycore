/* main — router, app shell, auth sheet, boot */
const AppCtx=React.createContext(null);
window.DcPages=window.DcPages||{};window.DcAppCtx=AppCtx;
(function(){
const {Bg,Button,IconBtn,Icon,Avatar,Sheet,Field,Input,Overlays,useI18n,errMsg,Empty}=DcUI;
const {useState,useEffect,useContext}=React;

function useRoute(){const[h,setH]=useState(location.hash||'#/today');
  useEffect(()=>{const f=()=>setH(location.hash||'#/today');window.addEventListener('hashchange',f);return()=>window.removeEventListener('hashchange',f)},[]);
  return h}
window.dcNav=h=>{if(location.hash!==h)location.hash=h;};

const TABS=[{k:'today',icon:'sun',path:'#/today'},{k:'materials',icon:'book',path:'#/materials'},{k:'companion',icon:'chat',path:'#/companion'},{k:'mood',icon:'heart',path:'#/mood'},{k:'settings',icon:'gear',path:'#/settings'}];

function AuthSheet({open,onClose}){
  const {t}=useI18n();const app=useContext(AppCtx);
  const[mode,setMode]=useState('login');const[email,setEmail]=useState('');const[pw,setPw]=useState('');const[name,setName]=useState('');
  const[busy,setBusy]=useState(false);const[oa,setOa]=useState('');const[error,setError]=useState(null);
  useEffect(()=>{if(open){setError(null);setBusy(false);setOa('')}},[open]);
  const submit=async e=>{e.preventDefault();setBusy(true);setError(null);
    try{mode==='login'?await DcStore.login(email,pw):await DcStore.register(email,pw,name);await app.refresh();DcUI.toast(t('auth.welcome'),'ok');onClose()}
    catch(err){setError(err)}finally{setBusy(false)}};
  const oauth=async p=>{setOa(p);setError(null);
    try{await DcStore.oauth(p);await app.refresh();DcUI.toast(t('auth.welcome'),'ok');onClose()}catch(err){setError(err);}finally{setOa('')}};
  return <Sheet open={open} onClose={onClose} title={mode==='login'?t('auth.login'):t('auth.create')}>
    <form onSubmit={submit} style={{display:'flex',flexDirection:'column',gap:14}}>
      {mode==='register'?<Field label={t('auth.name')}><Input value={name} onChange={e=>setName(e.target.value)} placeholder="Yun" autoComplete="nickname"/></Field>:null}
      <Field label={t('auth.email')}><Input type="email" required value={email} onChange={e=>setEmail(e.target.value)} placeholder="you@example.com" autoComplete="email"/></Field>
      <Field label={t('auth.password')} error={error?errMsg(error):null}><Input type="password" required value={pw} onChange={e=>setPw(e.target.value)} placeholder="••••••••" autoComplete={mode==='login'?'current-password':'new-password'}/></Field>
      <Button type="submit" loading={busy}>{mode==='login'?t('auth.login'):t('auth.register')}</Button>
      <button type="button" className="dc4-btn ghost accent sm" onClick={()=>{setMode(mode==='login'?'register':'login');setError(null)}}>{mode==='login'?t('auth.create'):t('auth.welcome')+' → '+t('auth.login')}</button>
      <div style={{display:'flex',alignItems:'center',gap:10}}><span style={{flex:1,height:1,background:'var(--dc-line)'}}></span><span className="dc4-cap-3">{t('auth.or')}</span><span style={{flex:1,height:1,background:'var(--dc-line)'}}></span></div>
      <div className="dc4-grid2">
        <Button type="button" variant="sec" onClick={()=>oauth('google')} loading={oa==='google'}><b style={{fontWeight:800}}>G</b>Google</Button>
        <Button type="button" variant="sec" onClick={()=>oauth('github')} loading={oa==='github'}><Icon name="terminal" size={16}/>GitHub</Button>
      </div>
      <p className="dc4-cap-3" style={{textAlign:'center'}}>{t('auth.demo')}</p>
    </form></Sheet>}

function Rail({route}){
  const {t}=useI18n();const app=useContext(AppCtx);const u=app.session.user;
  return <nav className="dc4-rail">
    <div className="logo"><span className="dc4-logo-dot"></span>Daycore</div>
    {TABS.map(x=><button key={x.k} className={'item'+(route.startsWith(x.path)?' on':'')} onClick={()=>dcNav(x.path)}><Icon name={x.icon} size={20}/>{t('tab.'+x.k)}</button>)}
    <div className="spacer"></div>
    <div className="user" onClick={()=>u?dcNav('#/settings'):app.openAuth()} role="button" tabIndex={0}>
      <Avatar user={u}/><div style={{flex:1,minWidth:0}}>
        <div style={{fontSize:13.5,fontWeight:650,overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{u?u.name:t('auth.anon')}</div>
        <div className="dc4-cap-3" style={{fontSize:11.5,overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{u?u.email:t('auth.anonhint')}</div>
      </div><Icon name="chevron-right" size={16} style={{color:'var(--dc-ink-3)'}}/></div>
  </nav>}

function TabBar({route}){
  const {t}=useI18n();
  return <nav className="dc4-tabbar">{TABS.map(x=><button key={x.k} className={'dc4-tab'+(route.startsWith(x.path)?' on':'')} onClick={()=>dcNav(x.path)} aria-label={t('tab.'+x.k)}><Icon name={x.icon} size={22}/><span>{t('tab.'+x.k)}</span></button>)}</nav>}

function TopBar(){
  const app=useContext(AppCtx);const u=app.session.user;
  return <div className="dc4-topbar"><div style={{display:'flex',alignItems:'center',gap:9,fontWeight:760,fontSize:17,letterSpacing:'-.01em'}}><span className="dc4-logo-dot" style={{width:24,height:24,borderRadius:'50%',background:'radial-gradient(circle at 32% 30%,var(--dc-accent-2),var(--dc-accent) 72%)',display:'inline-block'}}></span>Daycore</div>
    <Avatar user={u} onClick={()=>u?dcNav('#/settings'):app.openAuth()}/></div>}

function NotFound(){const {t}=useI18n();return <Empty icon="flag" title="404" desc={location.hash} action={<Button onClick={()=>dcNav('#/today')}>{t('c.back')}</Button>}/>}

function Root(){
  const route=useRoute();
  const[session,setSession]=useState(null);
  const[authOpen,setAuthOpen]=useState(false);
  const refresh=async()=>{const s=await DcStore.me();setSession(s);return s};
  useEffect(()=>{refresh()},[]);
  useEffect(()=>{if(!session)return;
    if(!session.onboarded&&!route.startsWith('#/onboarding')&&!route.startsWith('#/admin')){dcNav('#/onboarding')}
    else if(session.onboarded&&route.startsWith('#/onboarding')){dcNav('#/today')}},[session,route]);
  if(!session)return <div className="dc4-app"><Bg/></div>;
  const ctx={session,refresh,openAuth:()=>setAuthOpen(true)};
  let body;
  if(route.startsWith('#/admin')){const A=DcPages.AdminShell;body=A?<A route={route}/>:<NotFound/>}
  else if(route.startsWith('#/onboarding')){const O=DcPages.Onboarding;body=<div><Bg/>{O?<O/>:<NotFound/>}</div>}
  else{
    const map={'#/today':'Today','#/materials':'Materials','#/companion':'Companion','#/mood':'Mood','#/settings':'Settings'};
    const key=Object.keys(map).find(k=>route.startsWith(k));
    const P=key?DcPages[map[key]]:null;
    body=<div className="dc4-app"><Bg/><TopBar/><Rail route={route}/>
      <main className="dc4-page" data-screen-label={key?map[key]:'404'} key={key||route}>{P?<P/>:<NotFound/>}</main>
      <TabBar route={route}/></div>}
  return <AppCtx.Provider value={ctx}>{body}<AuthSheet open={authOpen} onClose={()=>setAuthOpen(false)}/><Overlays/></AppCtx.Provider>}

const boot=()=>{DcStore.initTheme();ReactDOM.createRoot(document.getElementById('root')).render(<Root/>)};
if(document.readyState==='loading')window.addEventListener('DOMContentLoaded',boot);else boot();
Object.assign(window,{DcRoot:Root});
})();
