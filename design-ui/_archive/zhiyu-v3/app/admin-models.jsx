/* Admin models — catalog table, editor sheet, defaults, prompts */
(function(){
const {Button,IconBtn,Icon,Field,Input,Select,Sheet,Tag,Toggle,Card,DirtyBar,useI18n,errMsg}=DcUI;
const {useState,useEffect,useContext,Fragment}=React;
DcI18n.add({
 'am.add':{zh:'添加模型',en:'Add model'},'am.defaults':{zh:'默认模型与参数',en:'Defaults & runtime'},
 'am.chat':{zh:'对话',en:'Chat'},'am.visionD':{zh:'视觉',en:'Vision'},'am.planner':{zh:'规划',en:'Planner'},
 'am.auto':{zh:'自动（第一个支持视觉的模型）',en:'Auto (first vision-capable)'},'am.same':{zh:'与对话相同',en:'Same as chat'},
 'am.timeout':{zh:'请求超时',en:'Timeout'},'am.rounds':{zh:'Agent 轮数上限',en:'Agent max rounds'},
 'am.caps':{zh:'能力',en:'Caps'},'am.ctx':{zh:'上下文',en:'Context'},'am.keyenv':{zh:'密钥环境变量',en:'Key env'},
 'am.test':{zh:'测试',en:'Test'},'am.pong':{zh:'连通 · {ms}ms',en:'OK · {ms}ms'},
 'am.edit':{zh:'编辑模型',en:'Edit model'},'am.id':{zh:'标识 id',en:'ID'},'am.format':{zh:'接口格式',en:'Format'},'am.model':{zh:'上游模型名',en:'Upstream model'},
 'am.enabled':{zh:'启用',en:'Enabled'},'am.stream':{zh:'流式',en:'Stream'},'am.tools':{zh:'工具',en:'Tools'},'am.vision':{zh:'视觉',en:'Vision'},
 'am.prompts':{zh:'提示词',en:'Prompts'},'am.updated':{zh:'更新于',en:'Updated'},
 'am.idExists':{zh:'该 id 已存在',en:'ID already exists'},
});
function ModelSheet({model,isNew,onClose,onSave,onDelete}){
  const {t}=useI18n();const[f,setF]=useState({...model});
  const set=(k,v)=>setF(x=>({...x,[k]:v}));
  return <Sheet open onClose={onClose} title={isNew?t('am.add'):t('am.edit')} footer={<Fragment>
      {!isNew&&<Button variant="danger soft" onClick={()=>onDelete(model.id)}>{t('c.delete')}</Button>}
      <span style={{flex:1}}></span><Button variant="ghost" onClick={onClose}>{t('c.cancel')}</Button>
      <Button disabled={!f.id||!f.model} onClick={()=>onSave(f)}>{t('c.save')}</Button></Fragment>}>
    <div style={{display:'flex',flexDirection:'column',gap:13}}>
      <div className="dc4-grid2">
        <Field label={t('am.id')} hint="a-z 0-9 . _ -"><Input value={f.id} disabled={!isNew} onChange={e=>set('id',e.target.value)} style={{fontFamily:'var(--dc-mono)',fontSize:13}}/></Field>
        <Field label={t('am.format')}><Select value={f.format} onChange={e=>set('format',e.target.value)} options={['openai','anthropic','ollama'].map(v=>({value:v,label:v}))}/></Field>
      </div>
      <Field label="Base URL"><Input value={f.baseUrl} onChange={e=>set('baseUrl',e.target.value)} style={{fontFamily:'var(--dc-mono)',fontSize:13}}/></Field>
      <div className="dc4-grid2">
        <Field label={t('am.model')}><Input value={f.model} onChange={e=>set('model',e.target.value)}/></Field>
        <Field label={t('am.keyenv')} hint={f.format==='ollama'?t('c.optional'):null}><Input value={f.apiKeyEnv} onChange={e=>set('apiKeyEnv',e.target.value)} placeholder="OPENAI_API_KEY" style={{fontFamily:'var(--dc-mono)',fontSize:13}}/></Field>
      </div>
      <div className="dc4-grid2">
        <Field label={t('am.ctx')}><Input type="number" value={f.contextWindow} onChange={e=>set('contextWindow',+e.target.value||0)}/></Field>
        <div className="dc4-field"><label className="dc4-label">{t('am.caps')}</label>
          <div style={{display:'flex',gap:7,flexWrap:'wrap'}}>
            {[['vision',t('am.vision')],['tools',t('am.tools')],['stream',t('am.stream')]].map(([k,l])=><DcUI.Chip key={k} on={!!f[k]} onClick={()=>set(k,!f[k])}>{l}</DcUI.Chip>)}
          </div></div>
      </div>
      <div className="dc4-row" style={{border:'none',padding:0}}><span className="grow dc4-body">{t('am.enabled')}</span><Toggle checked={!!f.enabled} onChange={v=>set('enabled',v)}/></div>
    </div></Sheet>}
function AdminModels(){
  const {t}=useI18n();const admin=useContext(DcPages.AdminCtx);
  const[doc,setDoc]=useState(null);const[orig,setOrig]=useState('');
  const[sheet,setSheet]=useState(null);const[testing,setTesting]=useState('');const[saving,setSaving]=useState(false);
  const[prompts,setPrompts]=useState(null);const[pSheet,setPSheet]=useState(null);const[pBody,setPBody]=useState('');const[pBusy,setPBusy]=useState(false);
  const load=async()=>{try{const r=await DcStore.adminGetModels();const d={models:r.models,defaults:r.defaults,requestTimeout:r.requestTimeout,agentMaxRounds:r.agentMaxRounds};setDoc(d);setOrig(JSON.stringify(d))}catch(e){DcUI.toast(errMsg(e),'err')}};
  useEffect(()=>{load();DcStore.adminPrompts().then(r=>setPrompts(r.prompts)).catch(()=>{})},[]);
  if(!doc)return <div className="dc4-stack"><DcUI.SkeletonCard lines={5}/></div>;
  const dirty=JSON.stringify(doc)!==orig;
  const setDefault=(k,v)=>setDoc(d=>({...d,defaults:{...d.defaults,[k]:v}}));
  const setModel=(id,patch)=>setDoc(d=>({...d,models:d.models.map(m=>m.id===id?{...m,...patch}:m)}));
  const saveSheet=f=>{setDoc(d=>{if(sheet.isNew){if(d.models.some(m=>m.id===f.id)){DcUI.toast(t('am.idExists'),'err');return d}return{...d,models:[...d.models,f]}}
    return{...d,models:d.models.map(m=>m.id===sheet.model.id?f:m)}});setSheet(null)};
  const delModel=id=>{setDoc(d=>({...d,models:d.models.filter(m=>m.id!==id)}));setSheet(null)};
  const test=async m=>{setTesting(m.id);try{
      const o=JSON.parse(orig).models.find(x=>x.id===m.id);
      const r=await DcStore.adminTestModel(o&&JSON.stringify(o)===JSON.stringify(m)?{id:m.id}:{config:m});
      r.ok?DcUI.toast(t('am.pong',{ms:r.latencyMs}),'ok'):DcUI.toast(r.error,'err')}
    catch(e){DcUI.toast((e&&e.message)||errMsg(e),'err')}finally{setTesting('')}};
  const save=async()=>{setSaving(true);try{const r=await DcStore.adminPutModels(doc);
      const d={models:r.models,defaults:r.defaults,requestTimeout:r.requestTimeout,agentMaxRounds:r.agentMaxRounds};
      setDoc(d);setOrig(JSON.stringify(d));DcUI.toast(t('c.saved'),'ok');(r.warnings||[]).forEach(w=>DcUI.toast(w,'info'));admin.reloadSys()}
    catch(e){DcUI.toast((e&&e.message)||errMsg(e),'err')}finally{setSaving(false)}};
  const dirtyCount=(()=>{if(!dirty)return 0;const o=JSON.parse(orig);let n=0;
    doc.models.forEach(m=>{const om=o.models.find(x=>x.id===m.id);if(!om||JSON.stringify(om)!==JSON.stringify(m))n++});
    n+=o.models.filter(m=>!doc.models.find(x=>x.id===m.id)).length;
    if(JSON.stringify(o.defaults)!==JSON.stringify(doc.defaults)||o.requestTimeout!==doc.requestTimeout||o.agentMaxRounds!==doc.agentMaxRounds)n++;
    return Math.max(1,n)})();
  const enabled=doc.models.filter(m=>m.enabled);
  return <div className="dc4-stack" style={{gap:16,paddingBottom:60}}>
    <div style={{display:'flex',justifyContent:'flex-end'}}><Button icon="plus" variant="sec" size="sm" onClick={()=>setSheet({isNew:true,model:{id:'',format:'openai',baseUrl:'',model:'',apiKeyEnv:'',vision:false,tools:true,stream:true,contextWindow:65536,enabled:true}})}>{t('am.add')}</Button></div>
    <div className="dc4-tablewrap">
      <table className="dc4-table"><thead><tr><th>ID</th><th>{t('am.format')}</th><th>{t('am.model')}</th><th>{t('am.keyenv')}</th><th>{t('am.caps')}</th><th className="num">{t('am.ctx')}</th><th>{t('am.enabled')}</th><th></th></tr></thead>
      <tbody>{doc.models.map(m=><tr key={m.id}>
        <td className="mono" style={{fontWeight:600}}>{m.id}</td>
        <td><Tag>{m.format}</Tag></td>
        <td className="mono">{m.model}</td>
        <td><span style={{display:'inline-flex',alignItems:'center',gap:6}}><span className={'dc4-dot '+(m.apiKeyEnv===''?'ok':m.apiKeySet?'ok':'warn')}></span><span className="dc4-mono">{m.apiKeyEnv||'—'}</span></span></td>
        <td><span style={{display:'inline-flex',gap:4}}>{m.vision&&<Tag tone="accent">V</Tag>}{m.tools&&<Tag>T</Tag>}{m.stream&&<Tag>S</Tag>}</span></td>
        <td className="num dc4-num">{(m.contextWindow/1000).toFixed(0)}k</td>
        <td><Toggle checked={m.enabled} onChange={v=>setModel(m.id,{enabled:v})}/></td>
        <td><span style={{display:'inline-flex',gap:4}}>
          <Button variant="ghost accent" size="sm" loading={testing===m.id} onClick={()=>test(m)}>{t('am.test')}</Button>
          <IconBtn name="pencil" label={t('c.edit')} onClick={()=>setSheet({isNew:false,model:m})}/></span></td>
      </tr>)}</tbody></table>
    </div>
    <Card icon="sliders" title={t('am.defaults')}>
      <div className="dc4-grid3">
        <Field label={t('am.chat')}><Select value={doc.defaults.chat} onChange={e=>setDefault('chat',e.target.value)} options={enabled.map(m=>({value:m.id,label:m.id}))}/></Field>
        <Field label={t('am.visionD')}><Select value={doc.defaults.vision} onChange={e=>setDefault('vision',e.target.value)} options={[{value:'',label:t('am.auto')}].concat(doc.models.filter(m=>m.vision).map(m=>({value:m.id,label:m.id})))}/></Field>
        <Field label={t('am.planner')}><Select value={doc.defaults.planner} onChange={e=>setDefault('planner',e.target.value)} options={[{value:'',label:t('am.same')}].concat(doc.models.map(m=>({value:m.id,label:m.id})))}/></Field>
      </div>
      <div className="dc4-grid2" style={{marginTop:13}}>
        <Field label={t('am.timeout')} hint="Go duration：120s / 2m"><Input value={doc.requestTimeout} onChange={e=>setDoc(d=>({...d,requestTimeout:e.target.value}))} style={{fontFamily:'var(--dc-mono)',fontSize:13}}/></Field>
        <Field label={t('am.rounds')}><Input type="number" min="1" value={doc.agentMaxRounds} onChange={e=>setDoc(d=>({...d,agentMaxRounds:+e.target.value||1}))}/></Field>
      </div>
    </Card>
    <Card icon="terminal" title={t('am.prompts')}>
      {(prompts||[]).map(p=><div key={p.key} className="dc4-row">
        <div className="grow"><div className="t">{p.label}</div><div className="s"><span className="dc4-mono">{p.key}</span> · {t('am.updated')} {p.updatedAt}</div></div>
        <IconBtn name="pencil" label={t('c.edit')} onClick={()=>{setPSheet(p);setPBody(p.body)}}/></div>)}
    </Card>
    <DirtyBar count={dirtyCount} saving={saving} onSave={save} onDiscard={()=>setDoc(JSON.parse(orig))}/>
    {sheet&&<ModelSheet model={sheet.model} isNew={sheet.isNew} onClose={()=>setSheet(null)} onSave={saveSheet} onDelete={delModel}/>}
    <Sheet open={!!pSheet} onClose={()=>setPSheet(null)} title={pSheet?pSheet.label:''} wide footer={pSheet?<Fragment>
        <Button variant="ghost" onClick={()=>setPSheet(null)}>{t('c.cancel')}</Button>
        <Button loading={pBusy} onClick={async()=>{setPBusy(true);await DcStore.adminSetPrompt(pSheet.key,pBody);const r=await DcStore.adminPrompts();setPrompts(r.prompts);setPBusy(false);setPSheet(null);DcUI.toast(t('c.saved'),'ok')}}>{t('c.save')}</Button></Fragment>:null}>
      {pSheet&&<DcUI.Textarea rows={10} value={pBody} onChange={e=>setPBody(e.target.value)} style={{fontFamily:'var(--dc-mono)',fontSize:12.5,lineHeight:'19px'}}/>}
    </Sheet>
  </div>}
DcPages.AdminModels=AdminModels;
})();
