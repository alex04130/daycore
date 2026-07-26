/* Admin env — grouped entries, secrets, dirty bar, restart flags */
(function(){
const {Button,Icon,Input,Select,Toggle,Tag,Card,DirtyBar,SecretField,useI18n,errMsg}=DcUI;
const {useState,useEffect,useContext}=React;
DcI18n.add({
 'ae.g.server':{zh:'服务',en:'Server'},'ae.g.database':{zh:'数据库',en:'Database'},'ae.g.auth':{zh:'认证与安全',en:'Auth & security'},'ae.g.ai':{zh:'AI',en:'AI'},'ae.g.limits':{zh:'限额与杂项',en:'Limits & misc'},'ae.g.weather':{zh:'天气',en:'Weather'},'ae.g.channels':{zh:'外部通道',en:'Channels'},
 'ae.hot':{zh:'热生效',en:'hot'},'ae.restart':{zh:'重启生效',en:'restart'},'ae.envlock':{zh:'由部署环境锁定',en:'locked by deploy env'},
 'ae.newSecret':{zh:'输入新值；留空保存 = 清除',en:'New value; save empty = clear'},
 'ae.savedN':{zh:'已保存 {n} 项',en:'Saved {n} keys'},'ae.default':{zh:'默认',en:'default'},
});
const ORDER=['server','database','auth','ai','limits','weather','channels'];
function EnvRow({e,val,dirty,editingSecret,onChange,onResetSecret}){
  const {t}=useI18n();
  let control;
  if(e.isSecret&&!editingSecret)control=<SecretField isSet={e.isSet} onReset={e.editable?onResetSecret:null}/>;
  else if(e.type==='bool')control=<Toggle checked={val==='true'} onChange={v=>onChange(v?'true':'false')}/>;
  else if(e.type==='select')control=<Select value={val} onChange={ev=>onChange(ev.target.value)} options={e.options.map(o=>({value:o,label:o}))} style={{width:210}}/>;
  else control=<Input value={val} onChange={ev=>onChange(ev.target.value)} disabled={!e.editable} placeholder={e.isSecret?t('ae.newSecret'):''} style={{width:'min(300px,72vw)',fontFamily:'var(--dc-mono)',fontSize:12.5}}/>;
  return <div className="dc4-row" style={{flexWrap:'wrap',rowGap:8,alignItems:'flex-start'}}>
    <div className="grow" style={{minWidth:220}}>
      <div className="t" style={{display:'flex',alignItems:'center',gap:7}}>{e.label}
        {dirty&&<span className="dc4-dot accent"></span>}
        {!e.editable&&<span title={t('ae.envlock')} style={{display:'inline-flex',color:'var(--dc-ink-3)'}}><Icon name="lock" size={13}/></span>}</div>
      <div className="s" style={{display:'flex',gap:6,alignItems:'center',flexWrap:'wrap',marginTop:2}}>
        <span className="dc4-mono">{e.key}</span>
        {e.restartRequired?<Tag>{t('ae.restart')}</Tag>:<Tag tone="ok">{t('ae.hot')}</Tag>}
        {e.source==='default'&&<Tag>{t('ae.default')}</Tag>}</div>
      {e.description?<div className="s" style={{marginTop:3,maxWidth:420}}>{e.description}</div>:null}
    </div>
    <div style={{flex:'none',paddingTop:2}}>{control}</div>
  </div>}
function AdminEnv(){
  const {t}=useI18n();const admin=useContext(DcPages.AdminCtx);
  const[entries,setEntries]=useState(null);const[edits,setEdits]=useState({});const[saving,setSaving]=useState(false);
  const load=async()=>{try{const r=await DcStore.adminGetEnv();setEntries(r.entries)}catch(e){DcUI.toast(errMsg(e),'err')}};
  useEffect(()=>{load()},[]);
  if(!entries)return <div className="dc4-stack"><DcUI.SkeletonCard lines={6}/></div>;
  const save=async()=>{setSaving(true);
    try{const r=await DcStore.adminPutEnv(edits);DcUI.toast(t('ae.savedN',{n:r.applied.length}),'ok');setEdits({});await load();admin.reloadSys()}
    catch(e){DcUI.toast((e&&e.message)||errMsg(e),'err')}finally{setSaving(false)}};
  return <div className="dc4-stack" style={{gap:16,paddingBottom:60}}>
    {ORDER.map(g=>{const list=entries.filter(e=>e.group===g);if(!list.length)return null;
      return <Card key={g} title={t('ae.g.'+g)}>
        {list.map(e=><EnvRow key={e.key} e={e} dirty={e.key in edits} editingSecret={e.isSecret&&(e.key in edits)}
          val={e.key in edits?edits[e.key]:(e.isSecret?'':e.value)}
          onChange={v=>setEdits(x=>({...x,[e.key]:v}))}
          onResetSecret={()=>setEdits(x=>({...x,[e.key]:''}))}/>)}
      </Card>})}
    <DirtyBar count={Object.keys(edits).length} saving={saving} onSave={save} onDiscard={()=>setEdits({})}/>
  </div>}
DcPages.AdminEnv=AdminEnv;
})();
