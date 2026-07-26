// 长卷 — 设置：助手 / 语言 / 主动关怀 / 渠道 / 类别 / 记忆 / 主题工作室（4 内置 + AI 生成）/ 数据 / 关于
(function () {
  'use strict';
  const { useState, useEffect, useRef } = React;
  const C = window.DcCore, IC = window.DcIcons, CJ = window.CJ;
  const BUILTIN = [
    { id: 'sky', name: '天空蓝', sw: ['#3b82f6', '#e0f2fe', '#bae6fd'] },
    { id: 'sunset', name: '暖橙日落', sw: ['#f97316', '#fff7ed', '#fed7aa'] },
    { id: 'night', name: '深夜紫', sw: ['#a78bfa', '#1e1b4b', '#312e81'], dark: true },
    { id: 'nature', name: '自然绿', sw: ['#16a34a', '#f0fdf4', '#bbf7d0'] },
  ];
  const swOf = (th) => { const v = th.variables || {}; return [v['--primary'] || '#888', v['--bg-start'] || '#eee', v['--bg-end'] || '#ddd']; };

  function Switch({ on, onToggle }) { return <button className={'cj-switch' + (on ? ' on' : '')} role="switch" aria-checked={on} onClick={onToggle}></button>; }
  function Row({ icon, t, s, children, onClick, href }) {
    const body = <React.Fragment><span className="ic">{icon}</span><div className="bd"><div className="t">{t}</div>{s && <div className="s">{s}</div>}</div>{children}</React.Fragment>;
    if (href) return <a className="cj-set-row glass" style={{ color: 'inherit', textDecoration: 'none', cursor: 'pointer' }} href={href}>{body}</a>;
    return <div className="cj-set-row glass" style={onClick ? { cursor: 'pointer' } : null} onClick={onClick}>{body}</div>;
  }
  function Group({ lab, sub, children }) {
    return <div className="cj-set-group"><div className="cj-set-lab">{lab}</div>{sub && <div style={{ fontSize: 11.5, color: 'var(--ink3)', margin: '-4px 0 10px' }}>{sub}</div>}{children}</div>;
  }

  // ── 主题工作室 ──
  function ThemeStudio() {
    const core = CJ.useCore();
    const cur = core.theme('liuli');
    const [aiDesc, setAiDesc] = useState('');
    const [aiBase, setAiBase] = useState('');
    const [busy, setBusy] = useState(false);
    const [err, setErr] = useState(null);
    const [editing, setEditing] = useState(null);   // 正在 AI 改的自定义主题
    const [preview, setPreview] = useState(null);   // {candidate, editingId, desc}
    const [menu, setMenu] = useState(null);         // {th, x, y, confirm}
    const [rename, setRename] = useState(null);     // {th, val}
    const longPress = useRef(null);
    const apply = (id) => { core.setTheme('liuli', id); window.cjApplyTheme(); };
    function generate(editingId, descOverride) {
      const desc = (descOverride !== undefined ? descOverride : aiDesc).trim();
      if (!desc) return;
      setBusy(true); setErr(null);
      setTimeout(() => {
        const res = core.aiTheme(desc, aiBase || undefined);
        setBusy(false);
        if (res.error) { setErr('这句听不出主题——描述一种颜色或氛围试试，比如「深夜的海」「奶茶」「雨后的苔原」'); return; }
        setPreview({ candidate: res, editingId: editingId || (editing && editing.id) || null, desc });
        window.__cjPreview = res; window.cjApplyTheme();
      }, 1400);
    }
    function cancelPreview() { setPreview(null); window.__cjPreview = null; window.cjApplyTheme(); }
    function savePreview() {
      const c = preview.candidate; let th;
      if (preview.editingId) th = core.patchTheme(preview.editingId, { name: c.name, dark: c.dark, base: c.base, variables: c.variables });
      else th = core.createTheme({ name: c.name, dark: c.dark, base: c.base, variables: c.variables });
      window.__cjPreview = null;
      core.setTheme('liuli', th.id); window.cjApplyTheme();
      setPreview(null); setEditing(null); setAiDesc('');
      CJ.cjToast({ label: '主题「' + th.name + '」存好了，已应用', sub: '不喜欢随时回设置换掉', kind: 'info' });
    }
    function openMenu(e, th) { e.preventDefault(); setMenu({ th, x: Math.min(e.clientX || 120, window.innerWidth - 190), y: Math.min(e.clientY || 200, window.innerHeight - 160), confirm: false }); }
    useEffect(() => {
      if (!menu) return;
      const close = () => setMenu(null);
      const t = setTimeout(() => { window.addEventListener('click', close); window.addEventListener('scroll', close, true); }, 50);
      return () => { clearTimeout(t); window.removeEventListener('click', close); window.removeEventListener('scroll', close, true); };
    }, [menu && menu.th.id]);
    const card = (id, name, sw, dark, th) => (
      <button key={id} className={'cj-theme-card' + (cur === id ? ' sel' : '')} onClick={() => apply(id)}
        onContextMenu={th ? (e) => openMenu(e, th) : undefined}
        onTouchStart={th ? (e) => { longPress.current = setTimeout(() => openMenu({ preventDefault() { }, clientX: e.touches[0].clientX, clientY: e.touches[0].clientY }, th), 550); } : undefined}
        onTouchEnd={th ? () => clearTimeout(longPress.current) : undefined}
        onTouchMove={th ? () => clearTimeout(longPress.current) : undefined}>
        <span className="cj-theme-sw" style={{ background: 'linear-gradient(135deg,' + sw[1] + ',' + sw[2] + ')' }}><span className="pill" style={{ background: sw[0] }}></span></span>
        <span className="cj-theme-name">{name}{dark && <span className="dk">深色</span>}</span>
        {cur === id && <span className="cj-theme-check"><IC.Check size={12} strokeWidth={3} /></span>}
        {th && <span className="dots" onClick={(e) => { e.stopPropagation(); openMenu(e, th); }}><IC.MoreHorizontal size={13} /></span>}
      </button>
    );
    return <React.Fragment>
      <div className="cj-theme-grid">
        {BUILTIN.map((b) => card(b.id, b.name, b.sw, b.dark))}
        {core.state.customThemes.map((th) => card(th.id, th.name, swOf(th), th.dark, th))}
      </div>
      {core.state.customThemes.length > 0 && <div style={{ fontSize: 11.5, color: 'var(--ink3)', margin: '8px 2px 0' }}>自定义主题：右键 / 长按可重命名、AI 改一版、删除</div>}
      <div className="cj-set-card glass" style={{ marginTop: 14 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10, fontSize: 13.5, fontWeight: 650 }}><IC.Wand size={16} style={{ color: 'var(--accent)' }} />AI 生成主题{editing && <span className="cj-chip on" style={{ marginLeft: 4 }} onClick={() => setEditing(null)}>正在改：{editing.name} ×</span>}</div>
        <input className="cj-ai-input" placeholder="描述一种感觉：「深夜的海」「奶茶时光」「薰衣草的梦」…" value={aiDesc}
          onChange={(e) => setAiDesc(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && !busy && generate()} />
        <div className="cj-ai-chips">
          <span style={{ fontSize: 11.5, color: 'var(--ink3)' }}>基于：</span>
          <button className={'cj-chip' + (aiBase === '' ? ' on' : '')} onClick={() => setAiBase('')}>不指定</button>
          {BUILTIN.map((b) => <button key={b.id} className={'cj-chip' + (aiBase === b.id ? ' on' : '')} onClick={() => setAiBase(b.id)}>{b.name}</button>)}
        </div>
        {err && <div className="cj-err">{err}</div>}
        <button className="cj-btn pri" style={{ width: '100%', marginTop: 12, height: 38 }} disabled={busy || !aiDesc.trim()} onClick={() => generate()}>
          <IC.Sparkles size={15} />{busy ? '正在调色…' : '生成并预览'}
        </button>
      </div>
      {preview && <div className="cj-previewbar glass" data-screen-label="主题预览条">
        <IC.Palette size={16} style={{ color: 'var(--accent)', flex: 'none' }} />
        <span className="nm">预览中：{preview.candidate.name}</span>
        <button onClick={() => generate(preview.editingId, preview.desc)} disabled={busy}>{busy ? '…' : '再试一次'}</button>
        <button onClick={cancelPreview}>取消</button>
        <button className="pri" onClick={savePreview}>{preview.editingId ? '保存修改' : '保存并应用'}</button>
      </div>}
      {menu && <div className="cj-ctx glass" style={{ left: menu.x, top: menu.y }} onClick={(e) => e.stopPropagation()}>
        <button onClick={() => { setRename({ th: menu.th, val: menu.th.name }); setMenu(null); }}><IC.Pencil size={14} />重命名</button>
        <button onClick={() => { setEditing({ id: menu.th.id, name: menu.th.name }); setAiBase(menu.th.base || ''); setMenu(null); CJ.cjToast({ label: '描述一句想怎么改，生成后会覆盖「' + menu.th.name + '」', kind: 'info' }); }}><IC.Wand size={14} />AI 改一版</button>
        <button style={{ color: '#dc2626' }} onClick={() => { if (!menu.confirm) { setMenu({ ...menu, confirm: true }); return; } const nm = menu.th.name; core.deleteTheme(menu.th.id); window.cjApplyTheme(); setMenu(null); CJ.cjToast({ label: '删掉了主题「' + nm + '」', kind: 'info' }); }}><IC.Trash size={14} />{menu.confirm ? '确认删除？' : '删除'}</button>
      </div>}
      {rename && <div className="cj-ctx glass" style={{ left: '50%', top: '40%', transform: 'translate(-50%,-50%)', width: 240, padding: 12 }}>
        <div style={{ fontSize: 12.5, fontWeight: 650, marginBottom: 8 }}>重命名主题</div>
        <input className="cj-ai-input" autoFocus value={rename.val} onChange={(e) => setRename({ ...rename, val: e.target.value })}
          onKeyDown={(e) => { if (e.key === 'Enter' && rename.val.trim()) { core.patchTheme(rename.th.id, { name: rename.val.trim() }); setRename(null); CJ.cjToast({ label: '改好了', kind: 'info' }); } if (e.key === 'Escape') setRename(null); }} />
        <div style={{ display: 'flex', gap: 6, marginTop: 10, justifyContent: 'flex-end' }}>
          <button className="cj-btn sec" style={{ height: 30 }} onClick={() => setRename(null)}>取消</button>
          <button className="cj-btn pri" style={{ height: 30 }} disabled={!rename.val.trim()} onClick={() => { core.patchTheme(rename.th.id, { name: rename.val.trim() }); setRename(null); CJ.cjToast({ label: '改好了', kind: 'info' }); }}>保存</button>
        </div>
      </div>}
    </React.Fragment>;
  }

  // ── 通知渠道 ──
  function Channels() {
    const core = CJ.useCore();
    const [bindFor, setBindFor] = useState(null); // {id, token}
    const [unbind, setUnbind] = useState(null);
    const META = { qq: ['QQ', IC.MessageCircle], telegram: ['Telegram', IC.Send] };
    return <React.Fragment>
      {core.state.channels.map((ch) => {
        const [name, Icon] = META[ch.id] || [ch.id, IC.Link];
        return <React.Fragment key={ch.id}>
          <Row icon={<Icon size={17} />} t={name + (ch.bound ? ' · 已绑定' : '')} s={ch.bound ? ch.externalId : '绑定后，简报和关怀也会发到这里'}>
            {ch.bound
              ? <button className="cj-btn sec" style={{ height: 30 }} onClick={() => { if (unbind !== ch.id) { setUnbind(ch.id); setTimeout(() => setUnbind((u) => u === ch.id ? null : u), 2600); return; } core.unbindChannel(ch.id); setUnbind(null); CJ.cjToast({ label: '解绑了 ' + name, kind: 'info' }); }}>{unbind === ch.id ? '确认解绑？' : '解绑'}</button>
              : <button className="cj-btn pri" style={{ height: 30 }} onClick={() => setBindFor({ id: ch.id, token: core.bindChannel(ch.id).token })}>绑定</button>}
          </Row>
          {bindFor && bindFor.id === ch.id && <div className="cj-set-card glass" style={{ marginTop: -2 }}>
            <div style={{ fontSize: 12.5, color: 'var(--ink2)', lineHeight: 1.6 }}>把这串 Token 发给 {name} 里的 Daycore 机器人，10 分钟内有效：</div>
            <div className="cj-token"><IC.Key size={15} style={{ color: 'var(--accent)' }} /><span style={{ flex: 1 }}>{bindFor.token}</span>
              <button className="cj-btn ghost" style={{ height: 26 }} onClick={() => { try { navigator.clipboard.writeText(bindFor.token); } catch (e) { } CJ.cjToast({ label: '复制好了', kind: 'info' }); }}><IC.Copy size={13} /></button></div>
            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
              <button className="cj-btn sec" style={{ height: 30 }} onClick={() => setBindFor(null)}>取消</button>
              <button className="cj-btn pri" style={{ height: 30 }} onClick={() => { core.verifyChannel(ch.id); setBindFor(null); CJ.cjToast({ label: '绑定好了：' + name, sub: '演示环境 · 模拟对方发来 Token' }); }}><IC.Check size={13} />模拟完成绑定</button>
            </div>
          </div>}
        </React.Fragment>;
      })}
      <div style={{ fontSize: 11.5, color: 'var(--ink3)', margin: '2px 2px 0' }}>一天最多找你 3 次——绑了渠道也一样。</div>
    </React.Fragment>;
  }

  // ── 记录类别 ──
  const CAT_ICON = { note: 'NotebookPen', food: 'Utensils', health: 'Heart', academic: 'GraduationCap', travel: 'Plane', finance: 'Wallet', fitness: 'Dumbbell' };
  function Categories() {
    const core = CJ.useCore();
    return <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(220px,1fr))', gap: 8 }}>
      {core.listCategories().map((c) => {
        const Icon = IC[CAT_ICON[c.id]] || IC.Circle;
        return <div key={c.id} className="cj-set-row glass" style={{ marginBottom: 0 }}>
          <span className="ic"><Icon size={16} /></span>
          <div className="bd"><div className="t" style={{ fontSize: 13 }}>{c.label}</div>{c.locked && <div className="s">基础类别，始终开启</div>}</div>
          {c.locked ? <IC.Check size={15} style={{ color: 'var(--ink3)' }} /> : <Switch on={c.enabled} onToggle={() => core.setCategory(c.id, !c.enabled)} />}
        </div>;
      })}
    </div>;
  }

  // ── 记忆 ──
  function Memories() {
    const core = CJ.useCore();
    const [draft, setDraft] = useState('');
    const [confirmClear, setConfirmClear] = useState(false);
    const add = () => { const t = draft.trim(); if (!t) return; const r = core.memoryAdd({ text: t, type: 'preference' }, false); setDraft(''); CJ.cjToast({ label: '记住了', opId: r.opId }); };
    return <React.Fragment>
      {core.state.memories.map((m) => <div key={m.id} className="cj-item">
        <span className="cj-kind mem">{m.type === 'preference' ? '偏好' : '惦记'}</span>
        <div className="bd"><div className="t">{m.text}</div></div>
        <button className="x" title="忘掉" onClick={() => { const opId = core.memoryRemove(m.id); CJ.cjToast({ label: '忘掉了', opId }); }}><IC.Trash size={13} /></button>
      </div>)}
      <div style={{ display: 'flex', gap: 8, marginTop: 4 }}>
        <input className="cj-ai-input" style={{ flex: 1 }} placeholder="补一条让 TA 记住的事…" value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && add()} />
        <button className="cj-btn sec" disabled={!draft.trim()} onClick={add} style={{ height: 40 }}><IC.Plus size={14} />记住</button>
      </div>
      {core.state.memories.length > 0 && <button className="cj-btn ghost" style={{ marginTop: 8 }} onClick={() => {
        if (!confirmClear) { setConfirmClear(true); setTimeout(() => setConfirmClear(false), 2600); return; }
        core.state.memories.slice().forEach((m) => core.memoryRemove(m.id)); setConfirmClear(false); CJ.cjToast({ label: '记忆清空了（每条都进了账本，可撤销）', kind: 'info' });
      }}><IC.Trash size={13} />{confirmClear ? '确认清空全部记忆？' : '清空记忆'}</button>}
    </React.Fragment>;
  }

  // ── 主动关怀 ──
  const PREFS = [
    ['morningBrief', 'Sun', '早间简报', '每天早上把今天的样子说给你听'],
    ['eveningReview', 'Moon', '晚间回顾', '睡前对个账，顺便预告明天'],
    ['deadlineAlerts', 'Bell', '临期提醒', '截止逼近时叫你一声（一天最多 3 次）'],
    ['rollingReplan', 'RefreshCw', '滚动重排', '计划被打乱时自动顺延调整'],
    ['gapSuggestions', 'Sparkles', '空档建议', '出现空档时轻轻递一个建议'],
    ['autoPlan', 'Zap', '自动规划', '允许在你睡后悄悄排好明天'],
    ['doNotDisturb', 'EyeOff', '勿扰模式', '只留关怀与最紧急的截止'],
  ];

  // ── 设置页 ──
  function SettingsPage() {
    const core = CJ.useCore();
    const [name, setName] = useState(core.state.assistantName);
    const [l2, setL2] = useState(core.state.l2Prompt || '');
    useEffect(() => setName(core.state.assistantName), [core.state.assistantName]);
    const saveName = () => { const v = name.trim(); if (v && v !== core.state.assistantName) { core.patch({ assistantName: v }); CJ.cjToast({ label: 'TA 现在叫「' + v + '」了', kind: 'info' }); } };
    const saveL2 = () => { if (l2 !== (core.state.l2Prompt || '')) { core.patch({ l2Prompt: l2 }); CJ.cjToast({ label: '提示词存好了，立即生效', kind: 'info' }); } };
    const scen = core.state.scenario;
    return <div className="cj-page" data-screen-label="设置视图">
      <div className="inner narrow">
        <h2 className="pt"><span className="ic"><IC.Settings size={19} /></span>设置<span className="pn">主题、助手与偏好——都在这</span></h2>

        <div className="cj-set-row glass" style={{ marginBottom: 26 }}>
          <span className="cj-avatar" style={{ width: 44, height: 44, fontSize: 16, flex: 'none' }}>{core.state.userName[0]}</span>
          <div className="bd"><div className="t" style={{ fontSize: 15 }}>{core.state.userName}</div><div className="s">匿名演示会话 · 账本存在本机，三个前端共用</div></div>
        </div>

        <Group lab="主题工作室" sub="换个心情换张皮——AI 生成的主题存下来就是你的">
          <ThemeStudio />
        </Group>

        <Group lab="助手">
          <Row icon={<IC.Heart size={17} />} t="助手名字" s="聊天里 TA 会用这个名字">
            <input className="cj-ai-input" style={{ width: 130, textAlign: 'right', height: 34 }} value={name} onChange={(e) => setName(e.target.value)} onBlur={saveName} onKeyDown={(e) => e.key === 'Enter' && e.target.blur()} />
          </Row>
          <div className="cj-set-card glass">
            <div style={{ display: 'flex', alignItems: 'center', gap: 7, fontSize: 13.5, fontWeight: 650 }}><IC.Sparkles size={14} style={{ color: 'var(--accent)' }} />自定义提示词</div>
            <div style={{ fontSize: 11.5, color: 'var(--ink3)', margin: '4px 0 10px' }}>写给 AI 的长期设定：TA 扮演什么角色、你是谁、说话风格与禁忌。所有对话、规划与关怀都会带上这段。</div>
            <textarea className="cj-ta" rows={4} maxLength={2000} placeholder="例：你是奶盖，一只毒舌但贴心的猫娘助手；我是大四学生，在准备秋招，叫我阿宁；回复别超过三句。" value={l2} onChange={(e) => setL2(e.target.value)} onBlur={saveL2}></textarea>
            <div style={{ fontSize: 11, color: 'var(--ink3)', marginTop: 6 }}>{l2.length} / 2000 字 · 离开输入框自动保存，立即生效</div>
          </div>
        </Group>

        <Group lab="语言">
          <Row icon={<IC.Globe size={17} />} t="界面语言" s="演示版先只有中文">
            <div className="cj-seg" style={{ width: 150 }}>
              <button className="on">中文</button>
              <button onClick={() => CJ.cjToast({ label: 'EN 文案还在路上——演示版先只有中文', kind: 'info' })}>EN</button>
            </div>
          </Row>
        </Group>

        <Group lab="主动关怀" sub="每一项都可以关——关掉后 TA 只在你开口时出现">
          {PREFS.map(([k, ic, t, s]) => { const Icon = IC[ic]; return <Row key={k} icon={<Icon size={16} />} t={t} s={s}><Switch on={!!core.state.prefs[k]} onToggle={() => core.setPref(k, !core.state.prefs[k])} /></Row>; })}
        </Group>

        <Group lab="通知渠道"><Channels /></Group>

        <Group lab="记录类别" sub="关闭的类别不会出现在归档的分类里；随手笔记始终开启"><Categories /></Group>

        <Group lab="记忆 · 关于你自己" sub="TA 长期记着的事——可见、可改、可撤"><Memories /></Group>

        <Group lab="演示与数据">
          <div className="cj-set-row glass">
            <span className="ic"><IC.Layers size={16} /></span>
            <div className="bd"><div className="t">演示场景</div><div className="s">写进共享账本，纸屿和汀同时生效</div></div>
            <div className="cj-seg" style={{ width: 230 }}>
              {[['healthy', '健康日'], ['crisis', '危机日'], ['cold', '冷启动']].map(([v, l]) => <button key={v} className={scen === v ? 'on' : ''} onClick={() => { core.scenarioSet(v); CJ.cjToast({ label: '切到「' + l + '」了', kind: 'info' }); }}>{l}</button>)}
            </div>
          </div>
          <Row icon={<IC.RefreshCw size={16} />} t="重新播种当前场景" s="把演示账本恢复到这个场景的初始状态" onClick={() => { core.scenarioSet(scen); CJ.cjToast({ label: '重新播种好了', kind: 'info' }); }}><IC.ChevronRight size={16} style={{ color: 'var(--ink3)' }} /></Row>
          <Row icon={<IC.Key size={16} />} t="管理控制台" s="模型、密钥、数据与用户管理（需管理员）" href="admin.html"><IC.ExternalLink size={14} style={{ color: 'var(--ink3)' }} /></Row>
          <Row icon={<IC.ExternalLink size={16} />} t="回三前端门厅" s="琉璃 · 纸屿 · 汀——同一本账，三种活法" href="../index.html"><IC.ChevronRight size={16} style={{ color: 'var(--ink3)' }} /></Row>
        </Group>

        <div className="cj-about glass" style={{ borderRadius: 'var(--r-md)', padding: '18px 16px' }}>
          <b>Daycore · 琉璃「长卷」</b><br />每一步都记在账本里 · 任何 AI 动作 3 秒可撤<br /><a href="admin.html">管理控制台 ↗</a>
        </div>
      </div>
    </div>;
  }

  Object.assign(window, { CJSettings: { SettingsPage, BUILTIN } });
})();
