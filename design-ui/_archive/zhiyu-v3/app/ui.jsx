/* DcUI — Paper Isle component library (React on dc4-* classes) */
const {useState,useEffect,useRef,useMemo,useCallback,Fragment}=React;
function Icon({name,size=20,cls,style}){return <span style={{display:'inline-flex',lineHeight:0,...style}} className={cls} dangerouslySetInnerHTML={{__html:DcIcons.svg(name,{size})}}/>}
function Bg(){return <div className="dc4-bg" aria-hidden="true"></div>}
function Spinner({size=16}){return <Icon name="refresh" size={size} cls="dc4-spin"/>}
function Button({variant='',size='',icon,loading,children,className='',...rest}){
  return <button className={('dc4-btn '+variant+' '+size+' '+className).trim()} disabled={loading||rest.disabled} {...rest}>
    {loading?<Spinner/>:icon?<Icon name={icon} size={size==='sm'?15:17}/>:null}{children}</button>}
function IconBtn({name,label,solid,size=18,className='',...rest}){return <button className={'dc4-iconbtn '+(solid?'solid ':'')+className} aria-label={label} title={label} {...rest}><Icon name={name} size={size}/></button>}
function Field({label,error,hint,children}){return <div className="dc4-field">{label?<label className="dc4-label">{label}</label>:null}{children}{error?<div className="dc4-ferr">{error}</div>:hint?<div className="dc4-fhint">{hint}</div>:null}</div>}
function Input(props){return <input {...props} className={'dc4-input '+(props.className||'')}/>}
function Textarea(props){return <textarea {...props} className={'dc4-textarea '+(props.className||'')}/>}
function Select({options,...props}){return <select {...props} className={'dc4-select '+(props.className||'')}>{options.map(o=><option key={o.value} value={o.value}>{o.label}</option>)}</select>}
function Toggle({checked,onChange,label}){return <button type="button" role="switch" aria-checked={!!checked} aria-label={label} className="dc4-toggle" onClick={()=>onChange(!checked)}><span className="knob"></span></button>}
function Seg({options,value,onChange,block,className=''}){return <div className={'dc4-seg '+(block?'block ':'')+className} role="tablist">{options.map(o=><button key={o.value} role="tab" aria-selected={value===o.value} className={value===o.value?'on':''} onClick={()=>onChange(o.value)}>{o.label}</button>)}</div>}
function Chip({on,children,...rest}){return <button type="button" className={'dc4-chip'+(on?' on':'')} {...rest}>{children}</button>}
function Tag({tone='',dot,children}){return <span className={'dc4-tag '+tone}>{dot?<span className={'dc4-dot '+tone}></span>:null}{children}</span>}
function Dot({tone=''}){return <span className={'dc4-dot '+tone}></span>}
function Card({icon,iconTone='',title,extra,children,className='',pad0,hover,onClick,style}){
  return <section className={'dc4-card '+(pad0?'pad0 ':'')+(hover?'hover ':'')+className} onClick={onClick} style={style}>
    {(title||icon||extra)?<header className="dc4-cardhead" style={pad0?{padding:'15px 17px 0'}:null}>{icon?<span className={'dc4-icobox '+iconTone}><Icon name={icon} size={18}/></span>:null}<span className="ttl">{title}</span>{extra}</header>:null}
    {children}</section>}
function KV({k,v,mono=true}){return <div className="dc4-kv"><span className="k">{k}</span><span className={mono?'v':'v'} style={mono?null:{fontFamily:'inherit',fontSize:'13.5px'}}>{v}</span></div>}
function Skeleton({h=16,w='100%',r=10,style}){return <div className="dc4-skel" style={{height:h,width:w,borderRadius:r,...style}}></div>}
function SkeletonCard({lines=3}){return <div className="dc4-card"><Skeleton h={18} w="40%"/><div style={{height:12}}></div>{Array.from({length:lines}).map((_,i)=><div key={i} style={{marginBottom:8}}><Skeleton h={13} w={(90-i*18)+'%'}/></div>)}</div>}
function Empty({icon='sparkle',title,desc,action}){return <div className="dc4-empty"><span className="ico"><Icon name={icon} size={40}/></span><div className="dc4-h2">{title}</div>{desc?<div className="dc4-cap-3" style={{maxWidth:340}}>{desc}</div>:null}{action?<div style={{marginTop:10}}>{action}</div>:null}</div>}
function Ring({size=64,stroke=5,value=0,tone,children}){
  const r=(size-stroke)/2,c=2*Math.PI*r;
  return <span className="dc4-ringwrap" style={{width:size,height:size}}>
    <svg width={size} height={size} style={{transform:'rotate(-90deg)'}}>
      <circle cx={size/2} cy={size/2} r={r} fill="none" stroke="var(--dc-surface-2)" strokeWidth={stroke}/>
      <circle cx={size/2} cy={size/2} r={r} fill="none" stroke={tone||'var(--dc-accent)'} strokeWidth={stroke} strokeLinecap="round" strokeDasharray={c} strokeDashoffset={c*(1-Math.min(1,Math.max(0,value)))} style={{transition:'stroke-dashoffset .6s var(--dc-ease)'}}/>
    </svg><span className="in">{children}</span></span>}
function Avatar({user,size='',onClick}){
  const txt=user?(user.name||user.email||'?').trim().slice(0,1).toUpperCase():null;
  return <button type="button" className={'dc4-avatar '+size} onClick={onClick} aria-label={user?user.name:DcI18n.t('auth.login')}>{user?txt:<Icon name="user" size={size==='lg'?22:17}/>}</button>}
function SecretField({isSet,onReset,resetLabel}){
  return <span className="dc4-secret"><span>{isSet?'••••••••':'—'}</span><Tag tone={isSet?'ok':''} dot>{DcI18n.t(isSet?'c.set':'c.unset')}</Tag>{onReset?<Button variant="ghost accent" size="sm" onClick={onReset}>{resetLabel||DcI18n.t('c.reset')}</Button>:null}</span>}
function Typing(){return <span className="dc4-typing"><i></i><i></i><i></i></span>}
function Sheet({open,onClose,title,children,wide,footer,back}){
  useEffect(()=>{if(!open)return;const h=e=>{if(e.key==='Escape')onClose&&onClose()};window.addEventListener('keydown',h);return()=>window.removeEventListener('keydown',h)},[open]);
  if(!open)return null;
  return <div className="dc4-veil" onMouseDown={e=>{if(e.target===e.currentTarget&&onClose)onClose()}}>
    <div className={'dc4-sheet'+(wide?' wide':'')} role="dialog" aria-modal="true" aria-label={typeof title==='string'?title:undefined}>
      <div className="dc4-grab"></div>
      {title!=null?<div className="dc4-sheet-head">{back?<IconBtn name="arrow-left" label={DcI18n.t('c.back')} onClick={back}/>:null}<span className="ttl">{title}</span><IconBtn name="x" label={DcI18n.t('c.close')} onClick={onClose}/></div>:null}
      {children}
      {footer?<div style={{display:'flex',gap:10,justifyContent:'flex-end',marginTop:20}}>{footer}</div>:null}
    </div></div>}
function DirtyBar({count,onSave,onDiscard,saving}){
  if(!count)return null;
  return <div className="dc4-dirtybar"><span className="msg dc4-num">{DcI18n.t('c.dirty',{n:count})}</span><Button variant="ghost" size="sm" onClick={onDiscard}>{DcI18n.t('c.discard')}</Button><Button size="sm" loading={saving} onClick={onSave}>{DcI18n.t('c.save')}</Button></div>}
/* toast + confirm host */
function Overlays(){
  const[toasts,setToasts]=useState([]);const[cf,setCf]=useState(null);
  useEffect(()=>{
    const ht=e=>{const t={id:Math.random(),...e.detail};setToasts(x=>[...x.slice(-2),t]);setTimeout(()=>setToasts(x=>x.filter(y=>y.id!==t.id)),2800)};
    const hc=e=>setCf(e.detail);
    window.addEventListener('dc4-toast',ht);window.addEventListener('dc4-confirm',hc);
    return()=>{window.removeEventListener('dc4-toast',ht);window.removeEventListener('dc4-confirm',hc)}},[]);
  return <div style={{display:'contents'}}>
    <div className="dc4-toasts">{toasts.map(t=><div key={t.id} className={'dc4-toast '+(t.type||'info')}><Icon name={t.type==='ok'?'check':t.type==='err'?'alert':'info'} size={17}/><span>{t.msg}</span></div>)}</div>
    {cf?<div className="dc4-veil" style={{alignItems:'center'}} onMouseDown={e=>{if(e.target===e.currentTarget){cf.resolve(false);setCf(null)}}}>
      <div className="dc4-sheet" style={{maxWidth:400,borderRadius:20,padding:'22px 24px'}} role="alertdialog" aria-modal="true">
        <div className="dc4-h1" style={{marginBottom:8}}>{cf.title}</div>
        {cf.message?<div className="dc4-body" style={{color:'var(--dc-ink-2)',marginBottom:18}}>{cf.message}</div>:null}
        <div style={{display:'flex',gap:10,justifyContent:'flex-end'}}>
          <Button variant="ghost" onClick={()=>{cf.resolve(false);setCf(null)}}>{cf.cancelLabel||DcI18n.t('c.cancel')}</Button>
          <Button variant={cf.danger?'danger':''} onClick={()=>{cf.resolve(true);setCf(null)}}>{cf.confirmLabel||DcI18n.t('c.confirm')}</Button>
        </div></div></div>:null}
  </div>}
/* hooks */
function useI18n(){const[,f]=useState(0);useEffect(()=>DcI18n.onChange(()=>f(x=>x+1)),[]);return{t:DcI18n.t,pick:DcI18n.pick,locale:DcI18n.locale}}
function useAsync(fn,deps){
  const[s,setS]=useState({loading:true,data:null,error:null});const[n,setN]=useState(0);
  useEffect(()=>{let on=true;setS(x=>({...x,loading:true,error:null}));
    fn().then(d=>on&&setS({loading:false,data:d,error:null})).catch(e=>on&&setS({loading:false,data:null,error:e}));
    return()=>{on=false}},[...(deps||[]),n]);
  return{...s,reload:()=>setN(x=>x+1)}}
function errMsg(e){if(!e)return DcI18n.t('err.unknown');const k='err.'+(e.error||'unknown');const s=DcI18n.t(k);return s===k?(e.message||DcI18n.t('err.unknown')):s}
function ErrorStrip({error,onRetry}){return <div className="dc4-card" style={{display:'flex',alignItems:'center',gap:10,borderColor:'color-mix(in srgb,var(--dc-err) 30%,transparent)'}}><Icon name="alert" size={18} style={{color:'var(--dc-err)'}}/><span className="dc4-body" style={{flex:1}}>{errMsg(error)}</span>{onRetry?<Button variant="sec" size="sm" onClick={onRetry}>{DcI18n.t('c.retry')}</Button>:null}</div>}
function injectCss(id,css){const eid='v4-css-'+id;if(document.getElementById(eid))return;const el=document.createElement('style');el.id=eid;el.textContent=css;document.head.appendChild(el)}
function fmtDate(isoStr,locale){const d=new Date(isoStr+'T12:00:00');const t=DcStore.todayIso();
  if(isoStr===t)return DcI18n.t('c.today');if(isoStr===DcStore.addDays(t,1))return DcI18n.t('c.tomorrow');if(isoStr===DcStore.addDays(t,-1))return DcI18n.t('c.yesterday');
  return (locale||DcI18n.locale)==='zh'?(d.getMonth()+1)+'月'+d.getDate()+'日 '+DcI18n.t('wd.p')+DcI18n.t('wd.'+DcStore.wd(isoStr)):d.toLocaleDateString('en-US',{month:'short',day:'numeric',weekday:'short'})}
function fmtTs(ts){const d=new Date(ts);return String(d.getHours()).padStart(2,'0')+':'+String(d.getMinutes()).padStart(2,'0')}
function fmtBytes(b){return b>1048576?(b/1048576).toFixed(1)+' MB':b>1024?Math.round(b/1024)+' KB':b+' B'}
function fmtUptime(sec){const h=Math.floor(sec/3600),m=Math.floor(sec%3600/60);return h>0?h+'h '+m+'m':m+'m'}
window.DcUI={Icon,Bg,Spinner,Button,IconBtn,Field,Input,Textarea,Select,Toggle,Seg,Chip,Tag,Dot,Card,KV,Skeleton,SkeletonCard,Empty,Ring,Avatar,SecretField,Typing,Sheet,DirtyBar,Overlays,useI18n,useAsync,errMsg,ErrorStrip,injectCss,fmtDate,fmtTs,fmtBytes,fmtUptime,
  toast(msg,type){window.dispatchEvent(new CustomEvent('dc4-toast',{detail:{msg,type:type||'info'}}))},
  confirm(opts){return new Promise(resolve=>window.dispatchEvent(new CustomEvent('dc4-confirm',{detail:{...opts,resolve}})))}
};
