/* Settings — account / general / theme studio / memory / data / admin entry / about */
(function(){
const {Button,IconBtn,Icon,Field,Input,Textarea,Seg,Sheet,Tag,Avatar,Toggle,DirtyBar,useI18n,useAsync,errMsg}=DcUI;
const {useState,useEffect,useContext,Fragment}=React;
DcI18n.add({
 'st.h':{zh:'设置',en:'Settings'},
 'st.account':{zh:'账户',en:'Account'},'st.general':{zh:'通用',en:'General'},'st.lang':{zh:'语言',en:'Language'},
 'st.assistant':{zh:'助理昵称',en:'Assistant name'},'st.wake':{zh:'起床',en:'Wake'},'st.sleep':{zh:'入睡',en:'Sleep'},'st.focus':{zh:'高效时段',en:'Focus time'},
 'st.theme':{zh:'主题',en:'Theme'},'st.genTheme':{zh:'生成新主题',en:'New theme'},'st.genPh':{zh:'描述你想要的感觉，如「午夜的海」',en:'Describe a feeling, e.g. “midnight ocean”'},
 'st.preview':{zh:'预览中',en:'Previewing'},'st.themeName':{zh:'主题名称',en:'Theme name'},'st.rename':{zh:'重命名',en:'Rename'},
 'st.memory':{zh:'小日的记忆',en:'Memory'},'st.memSub':{zh:'这些偏好会进入规划与陪伴的上下文。',en:'These preferences feed planning & companion context.'},'st.memPh':{zh:'告诉小日一个要记住的事…',en:'Tell Daycore something to remember…'},
 'st.data':{zh:'数据',en:'Data'},'st.export':{zh:'导出 JSON',en:'Export JSON'},'st.import':{zh:'导入数据',en:'Import data'},'st.reset':{zh:'清空并重置',en:'Reset everything'},
 'st.resetMsg':{zh:'将删除本地全部数据并回到初始引导，无法撤销。',en:'Deletes all local data and restarts onboarding. Irreversible.'},
 'st.admin':{zh:'管理台',en:'Admin console'},'st.adminSub':{zh:'模型、环境变量、OAuth 与用户管理',en:'Models, environment, OAuth & users'},
 'st.about':{zh:'关于',en:'About'},'st.ver':{zh:'版本',en:'Version'},'st.channel':{zh:'渠道',en:'Channel'},
 'st.l2':{zh:'自定义提示词',en:'Custom prompt'},'st.l2Sub':{zh:'写给 AI 的长期设定：TA 扮演什么角色、你是谁、说话风格与禁忌。对话、规划与心情回应都会带上这段设定。',en:'Long-term instructions for the AI: its persona, who you are, tone and boundaries. Applied to chat, planning and mood replies.'},'st.l2Ph':{zh:'例：你是奶盖，一只毒舌但贴心的猫娘助手；我是大四学生，叫我阿宁；回复别超过三句。',en:'e.g. You are Mochi, a sharp-tongued but caring cat assistant; call me Ning; keep replies under three sentences.'},'st.l2Note':{zh:'{n} / 2000 字 · 离开输入框自动保存',en:'{n} / 2000 · autosaves on blur'},
 'st.tn.sky':{zh:'晴空',en:'Sky'},'st.tn.sunset':{zh:'落日',en:'Sunset'},'st.tn.night':{zh:'夜航',en:'Night'},'st.tn.nature':{zh:'苔径',en:'Nature'},
 'st.eg1':{zh:'午夜的海',en:'midnight ocean'},'st.eg2':{zh:'雨后的森林',en:'forest after rain'},'st.eg3':{zh:'焦糖奶茶',en:'caramel milk tea'},
});
const BUILTIN=[{k:'sky',bg:'#f5f7fa',ac:'#2f6bff'},{k:'sunset',bg:'#faf6f1',ac:'#e0632a'},{k:'night',bg:'#14161d',ac:'#4fc2ae'},{k:'nature',bg:'#f4f8f4',ac:'#2e9e63'}];
function Swatch({bg,ac,name,active,onClick,onMenu}){
  return <div style={{display:'flex',flexDirection:'column',alignItems:'center',gap:6,position:'relative',width:64}}>
    <button onClick={onClick} aria-label={name} style={{width:52,height:52,borderRadius:'50%',border:active?'2.5px solid var(--dc-accent)':'2px solid var(--dc-line)',background:'linear-gradient(135deg,'+bg+' 55%,'+ac+' 55%)',cursor:'pointer',boxShadow:'var(--dc-e1)',padding:0}}></button>
    <span className="dc4-cap-3" style={{fontSize:11.5,maxWidth:64,overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{name}</span>
    {onMenu?<button className="dc4-iconbtn" style={{position:'absolute',top:-7,right:-4,width:24,height:24,background:'var(--dc-surface)',boxShadow:'var(--dc-e1)',border:'1px solid var(--dc-line)'}} aria-label="menu" onClick={onMenu}><Icon name="dots" size={13}/></button>:null}
  </div>}
function Settings(){
  const {t,locale}=useI18n();const app=useContext(DcAppCtx);const u=app.session.user;
  const[prof,setProf]=useState({...app.session.profile});
  useEffect(()=>{setProf({...app.session.profile})},[app.session]);
  const orig=app.session.profile;
  const dirtyKeys=['assistantName','wake','sleep','focus'].filter(k=>prof[k]!==orig[k]);
  const[savingP,setSavingP]=useState(false);
  const setP=(k,v)=>setProf(x=>({...x,[k]:v}));
  const saveProf=async()=>{setSavingP(true);await DcStore.saveProfile({assistantName:prof.assistantName,wake:prof.wake,sleep:prof.sleep,focus:prof.focus});await app.refresh();setSavingP(false);DcUI.toast(t('c.saved'),'ok')};
  const[l2,setL2]=useState(orig.l2Prompt||'');
  useEffect(()=>{setL2(app.session.profile.l2Prompt||'')},[app.session]);
  const saveL2=async()=>{if(l2!==(orig.l2Prompt||'')){await DcStore.saveProfile({l2Prompt:l2});await app.refresh();DcUI.toast(t('c.saved'),'ok')}};
  const[,setThemesV]=useState(0);
  const customs=DcStore.listThemes();const active=DcStore.activeTheme();
  const[genOpen,setGenOpen]=useState(false);const[desc,setDesc]=useState('');const[genBusy,setGenBusy]=useState(false);
  const[preview,setPreview]=useState(null);
  const[menuTheme,setMenuTheme]=useState(null);const[rename,setRename]=useState('');
  const[importOpen,setImportOpen]=useState(false);const[importText,setImportText]=useState('');const[importBusy,setImportBusy]=useState(false);
  const gen=async()=>{if(!desc.trim())return;setGenBusy(true);const r=await DcStore.generateTheme(desc.trim());setGenBusy(false);setGenOpen(false);setDesc('');
    setPreview(r);document.documentElement.dataset.theme=r.base;DcStore.applyThemeVars(r.vars)};
  const savePreview=async()=>{const item=await DcStore.saveTheme(preview);DcStore.setTheme(item.id);setPreview(null);setThemesV(x=>x+1);DcUI.toast(t('c.saved'),'ok')};
  const cancelPreview=()=>{DcStore.setTheme(DcStore.activeTheme());setPreview(null)};
  const pick=k=>{DcStore.setTheme(k);setThemesV(x=>x+1)};
  const doRename=async()=>{if(rename.trim()){await DcStore.renameTheme(menuTheme.id,rename.trim())}setMenuTheme(null);setThemesV(x=>x+1)};
  const delTheme=async()=>{if(await DcUI.confirm({title:t('c.delete')+'「'+menuTheme.name+'」？',danger:true,confirmLabel:t('c.delete')})){await DcStore.deleteTheme(menuTheme.id);setMenuTheme(null);setThemesV(x=>x+1)}};
  const mem=useAsync(()=>DcStore.listMemory(),[]);
  const[memText,setMemText]=useState('');
  const addMem=async()=>{if(!memText.trim())return;await DcStore.addMemory(memText.trim());setMemText('');mem.reload()};
  const exportJson=()=>{const blob=new Blob([DcStore.exportData()],{type:'application/json'});const a=document.createElement('a');a.href=URL.createObjectURL(blob);a.download='daycore-data-'+DcStore.todayIso()+'.json';a.click();setTimeout(()=>URL.revokeObjectURL(a.href),4000)};
  const doImport=async()=>{setImportBusy(true);try{await DcStore.importData(importText);DcUI.toast(t('c.done'),'ok');setTimeout(()=>location.reload(),400)}catch(e){DcUI.toast((e&&e.message)||errMsg(e),'err');setImportBusy(false)}};
  const doReset=async()=>{if(await DcUI.confirm({title:t('st.reset'),message:t('st.resetMsg'),danger:true,confirmLabel:t('st.reset')})){await DcStore.resetAll();location.hash='#/onboarding';location.reload()}};
  const logout=async()=>{await DcStore.logout();await app.refresh();DcUI.toast(t('auth.logout'),'ok')};
  return <div className="dc4-stack dc4-casc" style={{gap:18}}>
    <header><h1 className="dc4-display">{t('st.h')}</h1></header>
    <section className="dc4-card">
      <div className="dc4-eyebrow" style={{marginBottom:12}}>{t('st.account')}</div>
      {u?<div style={{display:'flex',alignItems:'center',gap:13,flexWrap:'wrap'}}>
        <Avatar user={u} size="lg"/>
        <div style={{flex:1,minWidth:160}}>
          <div className="dc4-h2" style={{display:'flex',alignItems:'center',gap:8}}>{u.name}{u.role==='admin'&&<Tag tone="accent">admin</Tag>}</div>
          <div className="dc4-cap-3">{u.email}</div></div>
        <Button variant="sec" size="sm" icon="logout" onClick={logout}>{t('auth.logout')}</Button></div>
      :<div style={{display:'flex',alignItems:'center',gap:13,flexWrap:'wrap'}}>
        <Avatar user={null} size="lg"/>
        <div style={{flex:1,minWidth:160}}><div className="dc4-h2">{t('auth.anon')}</div><div className="dc4-cap-3">{t('auth.anonhint')}</div></div>
        <Button size="sm" onClick={app.openAuth}>{t('auth.login')} / {t('auth.register')}</Button></div>}
    </section>
    {u&&u.role==='admin'&&<section className="dc4-card hover" onClick={()=>dcNav('#/admin')} style={{display:'flex',alignItems:'center',gap:12}}>
      <span className="dc4-icobox"><Icon name="shield" size={17}/></span>
      <div style={{flex:1,minWidth:0}}><div style={{fontWeight:650,fontSize:14.5}}>{t('st.admin')}</div><div className="dc4-cap-3">{t('st.adminSub')}</div></div>
      <Icon name="chevron-right" size={16} style={{color:'var(--dc-ink-3)'}}/></section>}
    <section className="dc4-card">
      <div className="dc4-eyebrow" style={{marginBottom:12}}>{t('st.general')}</div>
      <div style={{display:'flex',flexDirection:'column',gap:13}}>
        <div className="dc4-row" style={{border:'none',padding:0}}><span className="grow dc4-body">{t('st.lang')}</span>
          <Seg value={locale} onChange={l=>DcI18n.setLocale(l)} options={[{value:'zh',label:'中文'},{value:'en',label:'EN'}]}/></div>
        <div className="dc4-grid2">
          <Field label={t('st.assistant')}><Input value={prof.assistantName||''} onChange={e=>setP('assistantName',e.target.value)}/></Field>
          <Field label={t('st.focus')}><Seg block value={prof.focus||'morning'} onChange={v=>setP('focus',v)} options={[{value:'morning',label:t('ob.morning')},{value:'afternoon',label:t('ob.afternoon')},{value:'evening',label:t('ob.evening')}]}/></Field>
        </div>
        <div className="dc4-grid2">
          <Field label={t('st.wake')}><Input type="time" value={prof.wake||'07:30'} onChange={e=>setP('wake',e.target.value)}/></Field>
          <Field label={t('st.sleep')}><Input type="time" value={prof.sleep||'23:30'} onChange={e=>setP('sleep',e.target.value)}/></Field>
        </div>
      </div>
    </section>
    <section className="dc4-card">
      <div className="dc4-eyebrow" style={{marginBottom:6}}>{t('st.l2')}</div>
      <div className="dc4-cap-3" style={{marginBottom:10}}>{t('st.l2Sub')}</div>
      <Textarea rows={4} maxLength={2000} value={l2} placeholder={t('st.l2Ph')} onChange={e=>setL2(e.target.value)} onBlur={saveL2}/>
      <div className="dc4-fhint" style={{marginTop:6}}>{t('st.l2Note',{n:l2.length})}</div>
    </section>
    <section className="dc4-card">
      <div className="dc4-eyebrow" style={{marginBottom:14}}>{t('st.theme')}</div>
      <div style={{display:'flex',gap:14,flexWrap:'wrap'}}>
        {BUILTIN.map(b=><Swatch key={b.k} bg={b.bg} ac={b.ac} name={t('st.tn.'+b.k)} active={active===b.k&&!preview} onClick={()=>pick(b.k)}/>)}
        {customs.map(c=><Swatch key={c.id} bg={c.vars['--dc-bg']||'#fff'} ac={c.vars['--dc-accent']||'#888'} name={c.name} active={active===c.id&&!preview} onClick={()=>pick(c.id)} onMenu={e=>{e.stopPropagation();setMenuTheme(c);setRename(c.name)}}/>)}
        <div style={{display:'flex',flexDirection:'column',alignItems:'center',gap:6,width:64}}>
          <button onClick={()=>setGenOpen(true)} aria-label={t('st.genTheme')} style={{width:52,height:52,borderRadius:'50%',border:'1.5px dashed color-mix(in srgb,var(--dc-ink) 25%,transparent)',background:'transparent',cursor:'pointer',display:'inline-flex',alignItems:'center',justifyContent:'center',color:'var(--dc-ink-2)'}}><Icon name="wand" size={19}/></button>
          <span className="dc4-cap-3" style={{fontSize:11.5}}>{t('st.genTheme')}</span>
        </div>
      </div>
    </section>
    <section className="dc4-card">
      <div className="dc4-eyebrow" style={{marginBottom:6}}>{t('st.memory')}</div>
      <div className="dc4-cap-3" style={{marginBottom:12}}>{t('st.memSub')}</div>
      <div style={{display:'flex',gap:9,marginBottom:6}}>
        <Input value={memText} onChange={e=>setMemText(e.target.value)} placeholder={t('st.memPh')} onKeyDown={e=>{if(e.key==='Enter'){e.preventDefault();addMem()}}}/>
        <Button icon="plus" disabled={!memText.trim()} onClick={addMem}>{t('c.add')}</Button></div>
      {(mem.data||[]).map(m=><div key={m.id} className="dc4-row">
        <span className="dc4-icobox" style={{background:'var(--dc-surface-2)',color:'var(--dc-ink-2)'}}><Icon name="sparkle" size={15}/></span>
        <div className="grow"><div className="t" style={{fontWeight:500}}>{m.text}</div><div className="s"><Tag tone={m.source==='companion'?'accent':''}>{t('mt.src.'+(m.source==='companion'?'companion':m.source==='capture'?'capture':'user'))}</Tag></div></div>
        <IconBtn name="trash" label={t('c.delete')} onClick={async()=>{await DcStore.deleteMemory(m.id);mem.reload()}}/></div>)}
    </section>
    <section className="dc4-card">
      <div className="dc4-eyebrow" style={{marginBottom:12}}>{t('st.data')}</div>
      <div style={{display:'flex',gap:9,flexWrap:'wrap'}}>
        <Button variant="sec" size="sm" icon="download" onClick={exportJson}>{t('st.export')}</Button>
        <Button variant="sec" size="sm" icon="upload" onClick={()=>setImportOpen(true)}>{t('st.import')}</Button>
        <Button variant="danger soft" size="sm" icon="trash" onClick={doReset}>{t('st.reset')}</Button>
      </div>
    </section>
    <section className="dc4-card">
      <div className="dc4-eyebrow" style={{marginBottom:10}}>{t('st.about')}</div>
      <DcUI.KV k={t('st.ver')} v="Daycore 4.0.0-beta"/>
      <DcUI.KV k={t('st.channel')} v="beta"/>
    </section>
    <DirtyBar count={dirtyKeys.length} saving={savingP} onSave={saveProf} onDiscard={()=>setProf({...orig})}/>
    {preview&&<div className="dc4-dirtybar"><span className="msg">{t('st.preview')} · {preview.name}</span>
      <Button variant="ghost" size="sm" onClick={cancelPreview}>{t('c.cancel')}</Button>
      <Button size="sm" onClick={savePreview}>{t('c.save')}</Button></div>}
    <Sheet open={genOpen} onClose={()=>setGenOpen(false)} title={t('st.genTheme')}>
      <div style={{display:'flex',flexDirection:'column',gap:13}}>
        <Input value={desc} onChange={e=>setDesc(e.target.value)} placeholder={t('st.genPh')} autoFocus onKeyDown={e=>{if(e.key==='Enter'){e.preventDefault();gen()}}}/>
        <div style={{display:'flex',gap:7,flexWrap:'wrap'}}>{['st.eg1','st.eg2','st.eg3'].map(k=><DcUI.Chip key={k} onClick={()=>setDesc(t(k))}>{t(k)}</DcUI.Chip>)}</div>
        <Button icon="wand" loading={genBusy} disabled={!desc.trim()} onClick={gen}>{t('st.genTheme')}</Button>
      </div></Sheet>
    <Sheet open={!!menuTheme} onClose={()=>setMenuTheme(null)} title={menuTheme?menuTheme.name:''}>
      {menuTheme&&<div style={{display:'flex',flexDirection:'column',gap:13}}>
        <Field label={t('st.themeName')}><Input value={rename} onChange={e=>setRename(e.target.value)}/></Field>
        <div style={{display:'flex',gap:9,justifyContent:'flex-end'}}>
          <Button variant="danger soft" onClick={delTheme}>{t('c.delete')}</Button>
          <Button onClick={doRename}>{t('st.rename')}</Button>
        </div></div>}</Sheet>
    <Sheet open={importOpen} onClose={()=>setImportOpen(false)} title={t('st.import')}>
      <div style={{display:'flex',flexDirection:'column',gap:13}}>
        <Textarea rows={7} value={importText} onChange={e=>setImportText(e.target.value)} placeholder='{"profile":{...},"courses":[...]}' style={{fontFamily:'var(--dc-mono)',fontSize:12.5}}/>
        <Button loading={importBusy} disabled={!importText.trim()} onClick={doImport}>{t('st.import')}</Button>
      </div></Sheet>
  </div>}
DcPages.Settings=Settings;
})();
