/* Admin users — roles & lifecycle */
(function(){
const {Button,IconBtn,Icon,Input,Select,Tag,useI18n,errMsg}=DcUI;
const {useState,useEffect,useContext}=React;
DcI18n.add({
 'au.q':{zh:'搜索邮箱或昵称…',en:'Search email or name…'},
 'au.user':{zh:'用户',en:'User'},'au.role':{zh:'角色',en:'Role'},'au.created':{zh:'注册',en:'Joined'},'au.active':{zh:'最近活跃',en:'Last active'},'au.sessions':{zh:'会话',en:'Sessions'},
 'au.role.user':{zh:'成员',en:'member'},'au.role.admin':{zh:'管理员',en:'admin'},
 'au.never':{zh:'从未',en:'never'},'au.you':{zh:'你',en:'you'},
 'au.delTitle':{zh:'删除用户？',en:'Delete user?'},'au.delMsg':{zh:'{email} 的账号与全部数据将被删除，无法恢复。',en:'{email} and all their data will be removed permanently.'},
 'au.justnow':{zh:'刚刚',en:'now'},
});
function AdminUsers(){
  const {t,locale}=useI18n();const app=useContext(DcAppCtx);const me=app.session.user;
  const[users,setUsers]=useState(null);const[q,setQ]=useState('');
  const load=async()=>{try{const r=await DcStore.adminUsers();setUsers(r.users)}catch(e){DcUI.toast(errMsg(e),'err')}};
  useEffect(()=>{load()},[]);
  if(!users)return <div className="dc4-stack"><DcUI.SkeletonCard lines={5}/></div>;
  const rel=ts=>{if(!ts)return t('au.never');const h=(Date.now()-new Date(ts).getTime())/36e5;
    if(h<1)return t('au.justnow');if(h<24)return Math.floor(h)+'h';return Math.floor(h/24)+'d'};
  const list=users.filter(u=>!q||(u.email+' '+u.name).toLowerCase().includes(q.toLowerCase()));
  const setRole=async(u,role)=>{if(role===u.role)return;
    try{await DcStore.adminPatchUser(u.id,{role});DcUI.toast(t('c.saved'),'ok')}catch(e){DcUI.toast(errMsg(e),'err')}
    load()};
  const del=async u=>{if(await DcUI.confirm({title:t('au.delTitle'),message:t('au.delMsg',{email:u.email}),danger:true,confirmLabel:t('c.delete')})){
    try{await DcStore.adminDeleteUser(u.id);DcUI.toast(t('c.deleted'),'ok')}catch(e){DcUI.toast(errMsg(e),'err')}
    load()}};
  return <div className="dc4-stack" style={{gap:14}}>
    <div style={{maxWidth:340}}><Input value={q} onChange={e=>setQ(e.target.value)} placeholder={t('au.q')}/></div>
    <div className="dc4-tablewrap">
      <table className="dc4-table"><thead><tr><th>{t('au.user')}</th><th>{t('au.role')}</th><th>{t('au.created')}</th><th>{t('au.active')}</th><th className="num">{t('au.sessions')}</th><th></th></tr></thead>
      <tbody>{list.map(u=><tr key={u.id}>
        <td><span style={{display:'inline-flex',alignItems:'center',gap:10}}>
          <span className="dc4-avatar" style={{width:30,height:30,fontSize:12.5,cursor:'default'}}>{(u.name||u.email).slice(0,1).toUpperCase()}</span>
          <span><span style={{fontWeight:600}}>{u.name}</span>{u.id===me.id&&<Tag tone="accent" >{t('au.you')}</Tag>}<br/><span className="dc4-cap-3" style={{fontSize:12}}>{u.email}</span></span></span></td>
        <td><Select value={u.role} onChange={e=>setRole(u,e.target.value)} options={[{value:'user',label:t('au.role.user')},{value:'admin',label:t('au.role.admin')}]} style={{width:118,height:32,fontSize:13}}/></td>
        <td className="dc4-num">{String(u.createdAt).slice(0,10)}</td>
        <td className="dc4-num">{rel(u.lastActiveAt)}</td>
        <td className="num dc4-num">{u.sessionCount}</td>
        <td><IconBtn name="trash" label={t('c.delete')} disabled={u.id===me.id} style={u.id===me.id?{opacity:.3,pointerEvents:'none'}:null} onClick={()=>del(u)}/></td>
      </tr>)}</tbody></table>
    </div>
  </div>}
DcPages.AdminUsers=AdminUsers;
})();
