/* DcI18n — zh-CN / en-US. t(key,vars) / pick({zh,en}) / add({key:{zh,en}}) / setLocale */
(function(){
const M={};
const S={
'app.name':{zh:'Daycore',en:'Daycore'},
'tab.today':{zh:'今日',en:'Today'},'tab.materials':{zh:'资料',en:'Materials'},'tab.companion':{zh:'陪伴',en:'Companion'},'tab.mood':{zh:'心情',en:'Mood'},'tab.settings':{zh:'设置',en:'Settings'},
'c.save':{zh:'保存',en:'Save'},'c.cancel':{zh:'取消',en:'Cancel'},'c.confirm':{zh:'确认',en:'Confirm'},'c.delete':{zh:'删除',en:'Delete'},'c.edit':{zh:'编辑',en:'Edit'},'c.add':{zh:'添加',en:'Add'},'c.close':{zh:'关闭',en:'Close'},'c.back':{zh:'返回',en:'Back'},'c.next':{zh:'下一步',en:'Next'},'c.skip':{zh:'跳过',en:'Skip'},'c.done':{zh:'完成',en:'Done'},'c.retry':{zh:'重试',en:'Retry'},'c.copy':{zh:'复制',en:'Copy'},'c.copied':{zh:'已复制',en:'Copied'},'c.search':{zh:'搜索',en:'Search'},'c.more':{zh:'更多',en:'More'},'c.undo':{zh:'撤销',en:'Undo'},'c.discard':{zh:'放弃',en:'Discard'},'c.saved':{zh:'已保存',en:'Saved'},'c.loading':{zh:'加载中…',en:'Loading…'},'c.enabled':{zh:'已启用',en:'Enabled'},'c.disabled':{zh:'已停用',en:'Disabled'},'c.on':{zh:'开',en:'On'},'c.off':{zh:'关',en:'Off'},'c.optional':{zh:'可选',en:'Optional'},'c.today':{zh:'今天',en:'Today'},'c.tomorrow':{zh:'明天',en:'Tomorrow'},'c.yesterday':{zh:'昨天',en:'Yesterday'},'c.deleted':{zh:'已删除',en:'Deleted'},'c.applied':{zh:'已应用',en:'Applied'},'c.dirty':{zh:'{n} 项未保存',en:'{n} unsaved'},'c.set':{zh:'已设置',en:'Set'},'c.unset':{zh:'未设置',en:'Not set'},'c.reset':{zh:'重设',en:'Reset'},'c.test':{zh:'测试',en:'Test'},'c.open':{zh:'打开',en:'Open'},'c.all':{zh:'全部',en:'All'},'c.custom':{zh:'自定义',en:'Custom'},'c.none':{zh:'无',en:'None'},'c.min':{zh:'分钟',en:'min'},
'wd.1':{zh:'一',en:'Mon'},'wd.2':{zh:'二',en:'Tue'},'wd.3':{zh:'三',en:'Wed'},'wd.4':{zh:'四',en:'Thu'},'wd.5':{zh:'五',en:'Fri'},'wd.6':{zh:'六',en:'Sat'},'wd.7':{zh:'日',en:'Sun'},'wd.p':{zh:'周',en:''},
'auth.login':{zh:'登录',en:'Log in'},'auth.register':{zh:'注册',en:'Sign up'},'auth.logout':{zh:'退出登录',en:'Log out'},'auth.email':{zh:'邮箱',en:'Email'},'auth.password':{zh:'密码',en:'Password'},'auth.name':{zh:'昵称',en:'Name'},'auth.or':{zh:'或使用第三方账号',en:'or continue with'},'auth.google':{zh:'使用 Google 登录',en:'Continue with Google'},'auth.github':{zh:'使用 GitHub 登录',en:'Continue with GitHub'},'auth.demo':{zh:'演示账号：demo@daycore.app / demo1234 · 管理员：admin@daycore.app / admin123',en:'Demo: demo@daycore.app / demo1234 · Admin: admin@daycore.app / admin123'},'auth.welcome':{zh:'欢迎回来',en:'Welcome back'},'auth.create':{zh:'创建账号',en:'Create account'},'auth.anon':{zh:'未登录',en:'Not signed in'},'auth.anonhint':{zh:'本地体验中 · 登录后可同步',en:'Local session · sign in to sync'},'auth.loggedin':{zh:'已登录',en:'Signed in'},'auth.oauthing':{zh:'正在跳转 {p}…',en:'Redirecting to {p}…'},
'err.unauthorized':{zh:'请先登录',en:'Please sign in first'},'err.forbidden':{zh:'没有权限执行此操作',en:'You do not have permission'},'err.invalid_credentials':{zh:'邮箱或密码不正确',en:'Wrong email or password'},'err.email_taken':{zh:'该邮箱已被注册',en:'Email already registered'},'err.weak_password':{zh:'密码至少 8 位',en:'Password must be ≥ 8 chars'},'err.invalid_email':{zh:'邮箱格式不正确',en:'Invalid email'},'err.last_admin':{zh:'不能移除最后一位管理员',en:'Cannot remove the last admin'},'err.self_delete':{zh:'不能删除自己的账号',en:'You cannot delete yourself'},'err.unknown_key':{zh:'包含未知配置键',en:'Unknown config key'},'err.invalid_value':{zh:'存在不合法的值',en:'Invalid value'},'err.invalid_models':{zh:'模型配置校验未通过',en:'Model config failed validation'},'err.invalid_oauth':{zh:'OAuth 配置校验未通过',en:'OAuth config failed validation'},'err.not_found':{zh:'目标不存在',en:'Not found'},'err.network':{zh:'网络异常，请重试',en:'Network error, try again'},'err.unknown':{zh:'出了点问题，请重试',en:'Something went wrong'},
'greet.morning':{zh:'早上好',en:'Good morning'},'greet.noon':{zh:'中午好',en:'Good afternoon'},'greet.afternoon':{zh:'下午好',en:'Good afternoon'},'greet.evening':{zh:'晚上好',en:'Good evening'},'greet.night':{zh:'夜深了',en:'Late night'},
'adm.title':{zh:'管理台',en:'Console'},'adm.backapp':{zh:'返回应用',en:'Back to app'},'adm.overview':{zh:'总览',en:'Overview'},'adm.models':{zh:'模型设置',en:'Models'},'adm.env':{zh:'环境变量',en:'Environment'},'adm.oauth':{zh:'OAuth',en:'OAuth'},'adm.users':{zh:'用户',en:'Users'},'adm.data':{zh:'日志与数据',en:'Logs & Data'},'adm.restart':{zh:'需重启',en:'Restart required'},'adm.restartTip':{zh:'以下配置修改需要重启服务后生效：{keys}',en:'Restart the server to apply: {keys}'},'adm.forbidden':{zh:'此区域仅管理员可见',en:'Admins only'},'adm.forbiddenSub':{zh:'请使用管理员账号登录后再访问管理台。',en:'Sign in with an admin account to open the console.'},
};
let locale=(function(){try{return localStorage.getItem('dc4.locale')||(navigator.language||'zh').toLowerCase().startsWith('zh')?'zh':'en'}catch(e){return 'zh'}})();
try{const s=localStorage.getItem('dc4.locale');if(s)locale=s}catch(e){}
const subs=new Set();
Object.assign(M,S);
window.DcI18n={
  get locale(){return locale},
  setLocale(l){locale=l;try{localStorage.setItem('dc4.locale',l)}catch(e){};document.documentElement.lang=l==='zh'?'zh-CN':'en';subs.forEach(f=>f(l))},
  onChange(f){subs.add(f);return()=>subs.delete(f)},
  add(map){Object.assign(M,map)},
  pick(o){if(o==null)return'';if(typeof o==='string')return o;return o[locale]!=null?o[locale]:(o.zh!=null?o.zh:o.en)},
  t(k,vars){const e=M[k];let s=e?(e[locale]!=null?e[locale]:e.zh):k;if(vars)for(const v in vars)s=s.split('{'+v+'}').join(String(vars[v]));return s}
};
document.documentElement.lang=locale==='zh'?'zh-CN':'en';
})();
