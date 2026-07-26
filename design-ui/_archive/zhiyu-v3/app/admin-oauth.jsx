/* Admin OAuth — presets + custom providers */
(function(){
const {Button,IconBtn,Icon,Field,Input,Toggle,Tag,Card,DirtyBar,SecretField,useI18n,errMsg}=DcUI;
const {useState,useEffect,Fragment}=React;
DcI18n.add({
 'ao.preset':{zh:'预设',en:'preset'},'ao.custom':{zh:'自定义',en:'custom'},
 'ao.cb':{zh:'回调地址（填到提供商控制台）',en:'Callback URL (register at provider)'},
 'ao.scopes':{zh:'Scopes（逗号分隔）',en:'Scopes (comma-separated)'},
 'ao.addCustom':{zh:'添加自定义提供商',en:'Add custom provider'},
 'ao.mapping':{zh:'Userinfo 字段映射',en:'Userinfo field mapping'},
 'ao.nameHint':{zh:'小写 slug，如 authentik',en:'lowercase slug, e.g. authentik'},
 'ao.secretKeep':{zh:'留空 = 保持不变',en:'leave empty to keep'},
});
function Provider({p,idx,secret,onPatch,onSecret,onRemove}){
  const {t}=useI18n();
  const set=(k,v)=>onPatch(idx,{[k]:v});
  return <Card title={<span style={{display:'inline-flex',alignItems:'center',gap:8}}>{p.name||'—'}<Tag tone={p.preset?'accent':''}>{t(p.preset?'ao.preset':'ao.custom')}</Tag></span>}
    icon="key" extra={<span style={{display:'inline-flex',alignItems:'center',gap:8}}>
      {!p.preset&&<IconBtn name="trash" label={t('c.delete')} onClick={()=>onRemove(idx)}/>}
      <Toggle checked={p.enabled} onChange={v=>set('enabled',v)}/></span>}>
    <div style={{display:'flex',flexDirection:'column',gap:13}}>
      {!p.preset&&<Field label="name" hint={t('ao.nameHint')}><Input value={p.name} onChange={e=>set('name',e.target.value.toLowerCase())} style={{fontFamily:'var(--dc-mono)',fontSize:13}}/></Field>}
      <Field label={t('ao.cb')}>
        <div style={{display:'flex',gap:8,alignItems:'center'}}>
          <Input value={p.callbackUrl||''} readOnly style={{fontFamily:'var(--dc-mono)',fontSize:12,color:'var(--dc-ink-2)'}}/>
          <IconBtn name="copy" solid label={t('c.copy')} onClick={()=>{navigator.clipboard&&navigator.clipboard.writeText(p.callbackUrl||'');DcUI.toast(t('c.copied'),'ok')}}/>
        </div></Field>
      <div className="dc4-grid2">
        <Field label="Client ID"><Input value={p.clientId} onChange={e=>set('clientId',e.target.value)} style={{fontFamily:'var(--dc-mono)',fontSize:13}}/></Field>
        <Field label="Client Secret" hint={secret!=null?t('ao.secretKeep'):null}>
          {p.clientSecretSet&&secret==null
            ?<SecretField isSet onReset={()=>onSecret(p.name,'')}/>
            :<Input type="password" value={secret||''} onChange={e=>onSecret(p.name,e.target.value)} placeholder={p.clientSecretSet?t('ao.secretKeep'):''}/>}
        </Field>
      </div>
      <Field label={t('ao.scopes')}><Input value={(p.scopes||[]).join(', ')} onChange={e=>set('scopes',e.target.value.split(',').map(s=>s.trim()).filter(Boolean))} style={{fontFamily:'var(--dc-mono)',fontSize:13}}/></Field>
      {!p.preset&&<Fragment>
        <div className="dc4-grid3">
          {['authUrl','tokenUrl','userinfoUrl'].map(k=><Field key={k} label={k}><Input value={p[k]||''} onChange={e=>set(k,e.target.value)} style={{fontFamily:'var(--dc-mono)',fontSize:12}}/></Field>)}
        </div>
        <div className="dc4-eyebrow">{t('ao.mapping')}</div>
        <div className="dc4-grid2">
          {['idField','emailField','nameField','avatarField'].map(k=><Field key={k} label={k}><Input value={p[k]||''} onChange={e=>set(k,e.target.value)} style={{fontFamily:'var(--dc-mono)',fontSize:12.5}} placeholder={k.replace('Field','')}/></Field>)}
        </div>
      </Fragment>}
    </div></Card>}
function AdminOauth(){
  const {t}=useI18n();
  const[provs,setProvs]=useState(null);const[orig,setOrig]=useState('');const[secrets,setSecrets]=useState({});const[saving,setSaving]=useState(false);
  const load=async()=>{try{const r=await DcStore.adminGetOauth();setProvs(r.providers);setOrig(JSON.stringify(r.providers));setSecrets({})}catch(e){DcUI.toast(errMsg(e),'err')}};
  useEffect(()=>{load()},[]);
  if(!provs)return <div className="dc4-stack"><DcUI.SkeletonCard lines={5}/></div>;
  const patch=(i,p)=>setProvs(x=>x.map((v,j)=>j===i?{...v,...p}:v));
  const remove=i=>setProvs(x=>x.filter((_,j)=>j!==i));
  const addCustom=()=>setProvs(x=>[...x,{name:'',preset:false,enabled:false,clientId:'',clientSecretSet:false,callbackUrl:'',scopes:['openid','email','profile'],authUrl:'',tokenUrl:'',userinfoUrl:'',idField:'',emailField:'',nameField:'',avatarField:''}]);
  const dirtyN=(()=>{let n=Object.keys(secrets).filter(k=>secrets[k]!=='').length;
    const o=JSON.parse(orig);provs.forEach(p=>{const op=o.find(x=>x.name===p.name);if(!op||JSON.stringify(op)!==JSON.stringify(p))n++});
    n+=o.filter(p=>!provs.find(x=>x.name===p.name)).length;return n})();
  const save=async()=>{setSaving(true);
    try{const r=await DcStore.adminPutOauth({providers:provs.map(p=>({...p,clientSecret:secrets[p.name]!=null?secrets[p.name]:''}))});
      setProvs(r.providers);setOrig(JSON.stringify(r.providers));setSecrets({});DcUI.toast(t('c.saved'),'ok')}
    catch(e){DcUI.toast((e&&e.message)||errMsg(e),'err')}finally{setSaving(false)}};
  return <div className="dc4-stack" style={{gap:16,paddingBottom:60}}>
    {provs.map((p,i)=><Provider key={p.name||'new'+i} p={p} idx={i} secret={secrets[p.name]!=null?secrets[p.name]:null} onPatch={patch} onSecret={(name,v)=>setSecrets(x=>({...x,[name]:v}))} onRemove={remove}/>)}
    <div><Button variant="sec" icon="plus" onClick={addCustom}>{t('ao.addCustom')}</Button></div>
    <DirtyBar count={dirtyN} saving={saving} onSave={save} onDiscard={load}/>
  </div>}
DcPages.AdminOauth=AdminOauth;
})();
