/* Admin shell — guard, dark sidebar, topbar w/ restart badge, Overview */
(function(){
const {Button,IconBtn,Icon,Tag,Card,KV,Empty,Skeleton,useI18n,useAsync}=DcUI;
const {useState,useEffect,useContext}=React;
const AdminCtx=React.createContext(null);DcPages.AdminCtx=AdminCtx;
DcI18n.add({
 'adm.sec.overview':{zh:'总览',en:'Overview'},'adm.sec.models':{zh:'模型',en:'Models'},'adm.sec.env':{zh:'环境变量',en:'Environment'},'adm.sec.oauth':{zh:'OAuth',en:'OAuth'},'adm.sec.users':{zh:'用户',en:'Users'},'adm.sec.data':{zh:'日志与数据',en:'Logs & Data'},
 'adm.cap.sys':{zh:'系统',en:'System'},'adm.cap.cfg':{zh:'配置',en:'Config'},'adm.cap.ops':{zh:'运营',en:'Operations'},
 'adm.restartN':{zh:'需重启 · {n} 项',en:'Restart · {n}'},'adm.restarted':{zh:'我已重启服务',en:'I restarted the server'},
 'adm.restartDesc':{zh:'以下键已写入 .env，重启服务后生效：',en:'Written to .env; effective after restart:'},
 'adm.stat.users':{zh:'用户',en:'Users'},'adm.stat.sessions':{zh:'活跃会话',en:'Sessions'},'adm.stat.materials':{zh:'资料条目',en:'Materials'},'adm.stat.assignments':{zh:'作业',en:'Assignments'},
 'adm.sysinfo':{zh:'系统信息',en:'System'},'adm.activity':{zh:'近 7 天活动',en:'Last 7 days'},'adm.modelUse':{zh:'模型用量',en:'Model usage'},
 'adm.ver':{zh:'版本',en:'Version'},'adm.uptime':{zh:'运行时长',en:'Uptime'},'adm.db':{zh:'数据库',en:'Database'},'adm.baseurl':{zh:'公开地址',en:'Public URL'},'adm.cfgfiles':{zh:'配置文件',en:'Config files'},
 'adm.msgs':{zh:'消息',en:'msgs'},'adm.plans':{zh:'规划',en:'plans'},'adm.moodsN':{zh:'心情',en:'moods'},'adm.calls':{zh:'次',en:'calls'},
});
const NAV={'':'home',models:'cpu',env:'sliders',oauth:'key',users:'users',data:'database'};
function RestartBadge({sys,onCleared}){
  const {t}=useI18n();
  return <button className="dc4-chip" style={{borderColor:'color-mix(in srgb,var(--dc-warn) 45%,transparent)',background:'var(--dc-warn-soft)',color:'color-mix(in srgb,var(--dc-warn) 80%,var(--dc-ink))'}}
    onClick={async()=>{if(await DcUI.confirm({title:t('adm.restart'),message:t('adm.restartDesc')+' '+sys.restartKeys.join(', '),confirmLabel:t('adm.restarted')})){await DcStore.adminClearRestart();onCleared();DcUI.toast(t('c.done'),'ok')}}}>
    <Icon name="alert" size={14}/>{t('adm.restartN',{n:sys.restartKeys.length})}</button>}
function Overview(){
  const {t}=useI18n();const ctx=useContext(AdminCtx);const sys=ctx.sys;
  const stats=useAsync(()=>DcStore.adminStats(),[]);
  if(!sys)return <div className="dc4-stack"><DcUI.SkeletonCard lines={2}/><DcUI.SkeletonCard lines={4}/></div>;
  const S=stats.data;
  const maxM=S?Math.max.apply(null,S.series.map(d=>d.messages).concat([1])):1;
  return <div className="dc4-stack dc4-casc" style={{gap:16}}>
    <div className="dc4-statgrid">
      {[['users','adm.stat.users'],['sessions','adm.stat.sessions'],['materials','adm.stat.materials'],['assignments','adm.stat.assignments']].map(([k,l])=>
        <Card key={k}><div className="dc4-eyebrow">{t(l)}</div><div className="dc4-stat" style={{marginTop:6}}>{sys.counts[k]}</div></Card>)}
    </div>
    <div className="dc4-grid2">
      <Card icon="server" title={t('adm.sysinfo')} extra={<Tag tone={sys.env==='production'?'ok':'warn'}>{sys.env}</Tag>}>
        <KV k={t('adm.ver')} v={sys.version+' · '+sys.goVersion}/>
        <KV k={t('adm.db')} v={sys.dbType+' · '+DcUI.fmtBytes(sys.dbSizeBytes)}/>
        <KV k={t('adm.uptime')} v={DcUI.fmtUptime(sys.uptimeSec)}/>
        <KV k={t('adm.baseurl')} v={sys.publicBaseUrl}/>
        <KV k={t('adm.cfgfiles')} v={sys.configPaths.env+' · '+sys.configPaths.models+' · '+sys.configPaths.oauth}/>
      </Card>
      <Card icon="activity" title={t('adm.activity')}>
        {S?<div>
          <div style={{display:'flex',alignItems:'flex-end',gap:8,height:118,padding:'6px 2px 0'}}>
            {S.series.map(d=><div key={d.date} style={{flex:1,display:'flex',flexDirection:'column',alignItems:'center',gap:5,justifyContent:'flex-end'}}>
              <div style={{width:'100%',maxWidth:34,height:Math.max(6,d.messages/maxM*88),borderRadius:7,background:'linear-gradient(180deg,var(--dc-accent-2),var(--dc-accent))',opacity:.92}} title={d.date+' · '+d.messages}></div>
              <span className="dc4-cap-3" style={{fontSize:10.5}}>{+d.date.slice(8)}</span></div>)}
          </div>
          <div className="dc4-cap-3" style={{marginTop:8}} >{t('adm.msgs')} {S.series.reduce((s,d)=>s+d.messages,0)} · {t('adm.plans')} {S.series.reduce((s,d)=>s+d.plans,0)} · {t('adm.moodsN')} {S.series.reduce((s,d)=>s+d.moods,0)}</div>
        </div>:<Skeleton h={118}/>}
      </Card>
    </div>
    <Card icon="cpu" title={t('adm.modelUse')}>
      {S?S.models.map(m=>{const max=Math.max.apply(null,S.models.map(x=>x.calls));
        return <div key={m.id} className="dc4-row">
          <span className="dc4-mono" style={{width:150,flex:'none'}}>{m.id}</span>
          <div className="grow"><div style={{height:8,borderRadius:4,background:'var(--dc-surface-2)',overflow:'hidden'}}><div style={{width:(m.calls/max*100)+'%',height:'100%',borderRadius:4,background:'var(--dc-accent)'}}></div></div></div>
          <span className="dc4-cap-3 dc4-num" style={{width:130,textAlign:'right',flex:'none'}}>{m.calls} {t('adm.calls')} · {Math.round(m.tokens/1000)}k tok</span></div>})
      :<Skeleton h={60}/>}
    </Card>
  </div>}
function AdminShell({route}){
  const {t}=useI18n();const app=useContext(DcAppCtx);
  const[open,setOpen]=useState(false);const[bump,setBump]=useState(0);
  const u=app.session.user;const isAdmin=!!(u&&u.role==='admin');
  const sys=useAsync(()=>isAdmin?DcStore.adminSystem():Promise.resolve(null),[isAdmin,bump]);
  const sec=(route.split('/')[2]||'').split('?')[0];
  useEffect(()=>{setOpen(false)},[route]);
  if(!isAdmin)return <div className="dc4-app" style={{paddingLeft:20}}><DcUI.Bg/>
    <div style={{maxWidth:440,margin:'12vh auto 0'}}><div className="dc4-card">
      <Empty icon="shield" title={t('adm.forbidden')} desc={t('adm.forbiddenSub')} action={<div style={{display:'flex',gap:10}}>
        {!u&&<Button onClick={app.openAuth}>{t('auth.login')}</Button>}
        <Button variant="sec" onClick={()=>dcNav('#/today')}>{t('adm.backapp')}</Button></div>}/>
    </div></div></div>;
  const secKey=Object.prototype.hasOwnProperty.call(NAV,sec)?sec:'';
  const Sub={'':Overview,models:DcPages.AdminModels,env:DcPages.AdminEnv,oauth:DcPages.AdminOauth,users:DcPages.AdminUsers,data:DcPages.AdminData}[secKey]||Overview;
  const item=k=><button key={k} className={'item'+(secKey===k?' on':'')} onClick={()=>dcNav('#/admin'+(k?'/'+k:''))}><Icon name={NAV[k]} size={18}/>{t('adm.sec.'+(k||'overview'))}</button>;
  return <AdminCtx.Provider value={{sys:sys.data,reloadSys:()=>setBump(x=>x+1)}}>
    <div className="dc4-adm"><DcUI.Bg/>
      {open&&<div style={{position:'fixed',inset:0,zIndex:65,background:'rgba(10,12,18,.4)'}} onClick={()=>setOpen(false)}></div>}
      <aside className={'dc4-adm-side'+(open?' open':'')}>
        <div className="logo"><span className="dc4-logo-dot" style={{width:24,height:24}}></span>Daycore<span style={{opacity:.55,fontWeight:550,marginLeft:2}}>{t('adm.title')}</span></div>
        <div className="cap">{t('adm.cap.sys')}</div>
        {item('')}
        <div className="cap">{t('adm.cap.cfg')}</div>
        {['models','env','oauth'].map(item)}
        <div className="cap">{t('adm.cap.ops')}</div>
        {['users','data'].map(item)}
        <div className="spacer"></div>
        <button className="item" onClick={()=>dcNav('#/today')}><Icon name="arrow-left" size={18}/>{t('adm.backapp')}</button>
      </aside>
      <div className="dc4-adm-main">
        <header className="dc4-adm-top">
          <IconBtn name="menu" label="menu" className="dc4-adm-burger" onClick={()=>setOpen(true)}/>
          <span className="dc4-h2">{t('adm.sec.'+(secKey||'overview'))}</span>
          <span style={{flex:1}}></span>
          {sys.data&&sys.data.needsRestart&&<RestartBadge sys={sys.data} onCleared={()=>setBump(x=>x+1)}/>}
          <Tag tone="accent">beta</Tag>
        </header>
        <div className="dc4-adm-body" data-screen-label={'Admin '+(secKey||'overview')} key={secKey}>{Sub?<Sub/>:null}</div>
      </div>
    </div></AdminCtx.Provider>}
DcPages.AdminShell=AdminShell;
})();
