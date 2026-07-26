/* Admin data — AI logs / audit / database */
(function(){
const {Button,IconBtn,Icon,Seg,Sheet,Tag,useI18n,useAsync,errMsg}=DcUI;
const {useState,Fragment}=React;
DcI18n.add({
 'ad.ai':{zh:'AI 日志',en:'AI logs'},'ad.audit':{zh:'审计',en:'Audit'},'ad.db':{zh:'数据库',en:'Database'},
 'ad.backup':{zh:'备份下载',en:'Download backup'},'ad.browse':{zh:'浏览',en:'Browse'},
 'ad.table':{zh:'表',en:'Table'},'ad.rows':{zh:'行数',en:'Rows'},'ad.size':{zh:'大小',en:'Size'},
 'ad.time':{zh:'时间',en:'Time'},'ad.user':{zh:'用户',en:'User'},'ad.kind':{zh:'类型',en:'Kind'},'ad.model':{zh:'模型',en:'Model'},'ad.lat':{zh:'延迟',en:'Latency'},'ad.tok':{zh:'Tokens 入/出',en:'Tokens in/out'},'ad.preview':{zh:'内容',en:'Preview'},
 'ad.actor':{zh:'操作者',en:'Actor'},'ad.action':{zh:'动作',en:'Action'},'ad.target':{zh:'对象',en:'Target'},'ad.result':{zh:'结果',en:'Result'},
 'ad.first14':{zh:'仅展示前若干行',en:'showing first rows only'},
});
const fmtDT=ts=>{const d=new Date(ts);const p=n=>String(n).padStart(2,'0');return p(d.getMonth()+1)+'-'+p(d.getDate())+' '+p(d.getHours())+':'+p(d.getMinutes())};
function AILogs(){
  const {t}=useI18n();const r=useAsync(()=>DcStore.adminAILogs(),[]);
  if(r.loading)return <DcUI.SkeletonCard lines={6}/>;
  return <div className="dc4-tablewrap">
    <table className="dc4-table"><thead><tr><th>{t('ad.time')}</th><th>{t('ad.user')}</th><th>{t('ad.kind')}</th><th>{t('ad.model')}</th><th className="num">{t('ad.lat')}</th><th className="num">{t('ad.tok')}</th><th>{t('ad.preview')}</th></tr></thead>
    <tbody>{(r.data.logs||[]).map(x=><tr key={x.id}>
      <td className="dc4-num">{fmtDT(x.ts)}</td>
      <td className="dc4-cap-3" style={{fontSize:12.5}}>{x.user}</td>
      <td><Tag tone={x.kind==='auto-plan'?'accent':''}>{x.kind}</Tag></td>
      <td className="mono">{x.model}</td>
      <td className="num dc4-num">{x.latencyMs}ms</td>
      <td className="num dc4-num">{x.tokensIn}/{x.tokensOut}</td>
      <td><span style={{display:'inline-flex',alignItems:'center',gap:7,maxWidth:230}}>
        <span className={'dc4-dot '+(x.status==='ok'?'ok':'err')}></span>
        <span style={{overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap',color:x.status==='ok'?'inherit':'var(--dc-err)'}}>{x.preview}</span></span></td>
    </tr>)}</tbody></table></div>}
function Audit(){
  const {t}=useI18n();const r=useAsync(()=>DcStore.adminAudit(),[]);
  if(r.loading)return <DcUI.SkeletonCard lines={4}/>;
  return <div className="dc4-tablewrap">
    <table className="dc4-table"><thead><tr><th>{t('ad.time')}</th><th>{t('ad.actor')}</th><th>{t('ad.action')}</th><th>{t('ad.target')}</th><th>{t('ad.result')}</th></tr></thead>
    <tbody>{(r.data.audit||[]).map(x=><tr key={x.id}>
      <td className="dc4-num">{fmtDT(x.ts)}</td><td className="dc4-cap-3" style={{fontSize:12.5}}>{x.actor}</td>
      <td><Tag tone="accent">{x.action}</Tag></td><td style={{maxWidth:260,overflow:'hidden',textOverflow:'ellipsis'}}>{x.target}</td>
      <td><Tag tone={x.result==='ok'?'ok':'err'}>{x.result}</Tag></td></tr>)}</tbody></table></div>}
function DB(){
  const {t}=useI18n();const r=useAsync(()=>DcStore.adminDBTables(),[]);
  const[tbl,setTbl]=useState(null);const[rows,setRows]=useState(null);const[busy,setBusy]=useState(false);
  const browse=async name=>{setTbl(name);setRows(null);const d=await DcStore.adminDBBrowse(name);setRows(d)};
  const backup=async()=>{setBusy(true);try{const b=await DcStore.adminDBBackup();
      const blob=new Blob([b.blob],{type:'application/json'});const a=document.createElement('a');a.href=URL.createObjectURL(blob);a.download=b.filename;a.click();setTimeout(()=>URL.revokeObjectURL(a.href),4000)}
    catch(e){DcUI.toast(errMsg(e),'err')}finally{setBusy(false)}};
  if(r.loading)return <DcUI.SkeletonCard lines={5}/>;
  return <div className="dc4-stack" style={{gap:14}}>
    <div style={{display:'flex',justifyContent:'flex-end'}}><Button variant="sec" size="sm" icon="download" loading={busy} onClick={backup}>{t('ad.backup')}</Button></div>
    <div className="dc4-tablewrap">
      <table className="dc4-table"><thead><tr><th>{t('ad.table')}</th><th className="num">{t('ad.rows')}</th><th className="num">{t('ad.size')}</th><th></th></tr></thead>
      <tbody>{(r.data.tables||[]).map(x=><tr key={x.name}>
        <td className="mono" style={{fontWeight:600}}>{x.name}</td>
        <td className="num dc4-num">{x.rows}</td>
        <td className="num dc4-num">{DcUI.fmtBytes(x.bytes)}</td>
        <td><Button variant="ghost accent" size="sm" onClick={()=>browse(x.name)}>{t('ad.browse')}</Button></td></tr>)}</tbody></table>
    </div>
    <Sheet open={!!tbl} onClose={()=>setTbl(null)} title={tbl||''} wide>
      {rows?<Fragment><div className="dc4-tablewrap" style={{boxShadow:'none'}}>
        <table className="dc4-table"><thead><tr>{rows.columns.map(c=><th key={c}>{c}</th>)}</tr></thead>
        <tbody>{rows.rows.map((row,i)=><tr key={i}>{row.map((cell,j)=><td key={j} className="mono" style={{maxWidth:200,overflow:'hidden',textOverflow:'ellipsis'}}>{String(cell)}</td>)}</tr>)}</tbody></table></div>
        <div className="dc4-cap-3" style={{marginTop:8}}>{rows.rows.length} · {t('ad.first14')}</div></Fragment>
      :<DcUI.Skeleton h={120}/>}
    </Sheet>
  </div>}
function AdminData(){
  const {t}=useI18n();const[tab,setTab]=useState('ai');
  const Body={ai:AILogs,audit:Audit,db:DB}[tab];
  return <div className="dc4-stack" style={{gap:14}}>
    <Seg value={tab} onChange={setTab} options={[{value:'ai',label:t('ad.ai')},{value:'audit',label:t('ad.audit')},{value:'db',label:t('ad.db')}]}/>
    <div key={tab}><Body/></div>
  </div>}
DcPages.AdminData=AdminData;
})();
