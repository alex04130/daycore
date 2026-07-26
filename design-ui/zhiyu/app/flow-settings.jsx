// 顺流 — 设置面板（完整版）：纸色（4 内置 + AI 生成）/ 助手 / 主动关怀 / 渠道 / 类别 / 节律 / 场景 / 数据
(function () {
  'use strict';
  const { useState, useEffect } = React;
  const C = window.DcCore, FL = window.FL, Ic = FL.Ic;
  const BUILTIN = [
    { id: 'sky', name: '格线纸', sw: ['#2f6bff', '#f5f7fa'] },
    { id: 'sunset', name: '牛皮纸', sw: ['#e0632a', '#faf6f1'] },
    { id: 'night', name: '炭黑纸', sw: ['#4fc2ae', '#14161d'], dark: true },
    { id: 'nature', name: '行线本', sw: ['#2e9e63', '#f4f8f4'] },
  ];
  const swOf = (th) => { const v = th.variables || {}; return [v['--primary'] || '#888', v['--bg-start'] || '#eee']; };
  function Switch({ on, onToggle }) { return <button className={'fl-switch' + (on ? ' on' : '')} role="switch" aria-checked={on} onClick={onToggle}></button>; }
  const PREFS = [
    ['morningBrief', 'sun', '早间简报', '每天早上把今天的样子写进流里'],
    ['eveningReview', 'moon', '晚间回顾', '睡前对个账，顺便预告明天'],
    ['deadlineAlerts', 'bell', '临期提醒', '截止逼近时叫你一声（计入 ≤3 条/天）'],
    ['rollingReplan', 'refresh', '滚动重排', '计划被打乱时自动顺延调整'],
    ['gapSuggestions', 'sparkle', '空档建议', '出现空档时轻轻递一个提议'],
    ['autoPlan', 'zap', '自动规划', '允许在你入睡后的静默窗排好明天'],
    ['doNotDisturb', 'eye-off', '勿扰模式', '只留关怀与最紧急的截止'],
  ];
  const CATI = { note: 'pencil', food: 'utensils', health: 'heart', academic: 'book', travel: 'send', finance: 'database', fitness: 'dumbbell' };
  const CHM = { qq: ['QQ', 'chat'], telegram: ['Telegram', 'send'] };

  function Settings({ onClose }) {
    const core = FL.useCore();
    const cur = core.theme('zhiyu');
    const [name, setName] = useState(core.state.assistantName);
    const [l2, setL2] = useState(core.state.l2Prompt || '');
    const [aiDesc, setAiDesc] = useState('');
    const [busy, setBusy] = useState(false);
    const [err, setErr] = useState(null);
    const [preview, setPreview] = useState(null);
    const [renameId, setRenameId] = useState(null);
    const [renVal, setRenVal] = useState('');
    const [delId, setDelId] = useState(null);
    const [bindFor, setBindFor] = useState(null);
    const [unbind, setUnbind] = useState(null);
    useEffect(() => {
      const esc = (e) => e.key === 'Escape' && onClose();
      window.addEventListener('keydown', esc);
      return () => { window.removeEventListener('keydown', esc); if (window.__flPreview) { window.__flPreview = null; window.flApplyTheme(); } };
    }, []);
    const applyT = (id) => { core.setTheme('zhiyu', id); window.flApplyTheme(); };
    const gen = () => {
      const d = aiDesc.trim(); if (!d || busy) return;
      setBusy(true); setErr(null);
      setTimeout(() => {
        const res = core.aiTheme(d); setBusy(false);
        if (res.error) { setErr('这句听不出纸色——描述一种颜色或氛围，比如「深夜的海」「奶茶时光」'); return; }
        setPreview({ candidate: res, desc: d }); window.__flPreview = res; window.flApplyTheme();
      }, 1300);
    };
    const cancelP = () => { setPreview(null); window.__flPreview = null; window.flApplyTheme(); };
    const saveP = () => {
      const c = preview.candidate;
      const th = core.createTheme({ name: c.name, dark: c.dark, base: c.base, variables: c.variables });
      window.__flPreview = null; core.setTheme('zhiyu', th.id); window.flApplyTheme();
      setPreview(null); setAiDesc('');
      FL.snack({ label: '✓ 纸色「' + th.name + '」存好了，已应用' });
    };
    const saveName = () => { const v = name.trim(); if (v && v !== core.state.assistantName) { core.patch({ assistantName: v }); FL.snack({ label: '✓ TA 现在叫「' + v + '」了' }); } };
    const saveL2 = () => { if (l2 !== (core.state.l2Prompt || '')) { core.patch({ l2Prompt: l2 }); FL.snack({ label: '✓ 提示词存好了，立即生效' }); } };
    return <React.Fragment>
      <div className="fl-veil" onClick={onClose}></div>
      <div className="fl-panel" data-screen-label="设置面板">
        <div className="fl-phead"><h3><Ic n="sliders" size={16} style={{ color: 'var(--dc-accent)' }} />设置</h3><button className="dc4-iconbtn" onClick={onClose}><Ic n="x" size={16} /></button></div>
        <div className="fl-pbody">
          <div className="fl-sec">纸色 · 内置</div>
          <div className="fl-papers">
            {BUILTIN.map((b) => <button key={b.id} className={'fl-paper' + (cur === b.id ? ' on' : '')} onClick={() => applyT(b.id)}>
              <span className="sw" style={{ background: b.sw[1] }}><i style={{ background: b.sw[0] }}></i></span>
              <span className="nm">{b.name}{b.dark && <span className="dk">深色</span>}</span>
              {cur === b.id && <span className="ck"><Ic n="check" size={11} /></span>}
            </button>)}
          </div>
          <div className="fl-sec">纸色 · AI 调一张<span className="n">{core.state.customThemes.length}</span></div>
          {core.state.customThemes.map((th) => {
            const sw = swOf(th);
            return <div key={th.id} className={'fl-it' + (cur === th.id ? ' sel' : '')} style={cur === th.id ? { borderColor: 'var(--dc-accent)' } : null}>
              <span style={{ width: 26, height: 26, borderRadius: 8, flex: 'none', border: '1px solid rgba(0,0,0,.08)', background: 'linear-gradient(135deg,' + sw[1] + ' 46%,' + sw[0] + ' 54%)' }}></span>
              <div className="bd">
                {renameId === th.id
                  ? <input className="dc4-input" style={{ height: 32, fontSize: 12.5 }} autoFocus value={renVal} onChange={(e) => setRenVal(e.target.value)}
                    onKeyDown={(e) => { if (e.key === 'Enter' && renVal.trim()) { core.patchTheme(th.id, { name: renVal.trim() }); setRenameId(null); } if (e.key === 'Escape') setRenameId(null); }}
                    onBlur={() => { if (renVal.trim()) core.patchTheme(th.id, { name: renVal.trim() }); setRenameId(null); }} />
                  : <div className="t" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>{th.name}{th.dark && <span className="fl-tag">深色</span>}{cur === th.id && <Ic n="check" size={12} style={{ color: 'var(--dc-accent)' }} />}</div>}
                <div className="s">AI 生成 · 基于{(BUILTIN.find((b) => b.id === th.base) || { name: '格线纸' }).name}</div>
              </div>
              {cur !== th.id && <button className="x" style={{ opacity: 1, color: 'var(--dc-accent)', fontSize: 11.5, fontWeight: 650 }} onClick={() => applyT(th.id)}>应用</button>}
              <button className="x" title="重命名" onClick={() => { setRenameId(th.id); setRenVal(th.name); }}><Ic n="pencil" size={13} /></button>
              <button className="x" title="删除" style={delId === th.id ? { opacity: 1, color: 'var(--dc-err)', fontSize: 11, fontWeight: 700 } : null}
                onClick={() => { if (delId !== th.id) { setDelId(th.id); setTimeout(() => setDelId((d) => d === th.id ? null : d), 2600); return; } core.deleteTheme(th.id); window.flApplyTheme(); setDelId(null); FL.snack({ label: '✓ 删掉了纸色「' + th.name + '」' }); }}>
                {delId === th.id ? '确认？' : <Ic n="trash" size={13} />}</button>
            </div>;
          })}
          <div className="fl-it" style={{ flexDirection: 'column', alignItems: 'stretch', gap: 8 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 7, fontSize: 12.5, fontWeight: 650 }}><Ic n="wand" size={14} style={{ color: 'var(--dc-accent)' }} />描述一种感觉，调一张新纸</div>
            <input className="dc4-input" style={{ height: 38, fontSize: 13 }} placeholder="「深夜的海」「奶茶时光」「雨后苔原」…" value={aiDesc} onChange={(e) => setAiDesc(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && gen()} />
            {err && <div style={{ fontSize: 11.5, color: 'var(--dc-err)' }}>{err}</div>}
            <button className="dc4-btn" disabled={busy || !aiDesc.trim()} onClick={gen}><Ic n="sparkle" size={14} />{busy ? '正在调色…' : '生成并预览'}</button>
          </div>

          <div className="fl-sec">助手</div>
          <div className="fl-it" style={{ alignItems: 'center' }}>
            <Ic n="heart" size={15} style={{ color: 'var(--dc-accent)', flex: 'none' }} />
            <div className="bd"><div className="t">助手名字</div><div className="s">流里和聊天里 TA 都用这个名字</div></div>
            <input className="dc4-input" style={{ width: 110, height: 34, fontSize: 13, textAlign: 'right' }} value={name} onChange={(e) => setName(e.target.value)} onBlur={saveName} onKeyDown={(e) => e.key === 'Enter' && e.target.blur()} />
          </div>
          <div className="fl-it" style={{ flexDirection: 'column', alignItems: 'stretch', gap: 8 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 7, fontSize: 12.5, fontWeight: 650 }}><Ic n="sparkle" size={14} style={{ color: 'var(--dc-accent)' }} />自定义提示词</div>
            <div style={{ fontSize: 11.5, color: 'var(--dc-ink-3)' }}>写给 AI 的长期设定：角色、你是谁、说话风格与禁忌。对话、规划与关怀都会带上。</div>
            <textarea className="fl-ta" rows={4} maxLength={2000} placeholder="例：你是奶盖，一只毒舌但贴心的猫娘助手；我是大四学生，叫我阿宁；回复别超过三句。" value={l2} onChange={(e) => setL2(e.target.value)} onBlur={saveL2}></textarea>
            <div style={{ fontSize: 10.5, color: 'var(--dc-ink-3)' }}>{l2.length} / 2000 字 · 离开输入框自动保存</div>
          </div>

          <div className="fl-sec">主动关怀</div>
          {PREFS.map(([k, ic, t, s]) => <div key={k} className="fl-it" style={{ alignItems: 'center' }}>
            <Ic n={ic} size={15} style={{ color: 'var(--dc-accent)', flex: 'none' }} />
            <div className="bd"><div className="t">{t}</div><div className="s">{s}</div></div>
            <Switch on={!!core.state.prefs[k]} onToggle={() => core.setPref(k, !core.state.prefs[k])} />
          </div>)}

          <div className="fl-sec">通知渠道</div>
          {core.state.channels.map((ch) => {
            const [nm, ic] = CHM[ch.id] || [ch.id, 'link'];
            return <React.Fragment key={ch.id}>
              <div className="fl-it" style={{ alignItems: 'center' }}>
                <Ic n={ic} size={15} style={{ color: 'var(--dc-accent)', flex: 'none' }} />
                <div className="bd"><div className="t">{nm}{ch.bound ? ' · 已绑定' : ''}</div><div className="s">{ch.bound ? ch.externalId : '绑定后，简报和关怀也会送到这里'}</div></div>
                {ch.bound
                  ? <button className="dc4-btn sm sec" onClick={() => { if (unbind !== ch.id) { setUnbind(ch.id); setTimeout(() => setUnbind((u) => u === ch.id ? null : u), 2600); return; } core.unbindChannel(ch.id); setUnbind(null); FL.snack({ label: '✓ 解绑了 ' + nm }); }}>{unbind === ch.id ? '确认解绑？' : '解绑'}</button>
                  : <button className="dc4-btn sm" onClick={() => setBindFor({ id: ch.id, token: core.bindChannel(ch.id).token })}>绑定</button>}
              </div>
              {bindFor && bindFor.id === ch.id && <div className="fl-it" style={{ flexDirection: 'column', alignItems: 'stretch', gap: 6 }}>
                <div style={{ fontSize: 12, color: 'var(--dc-ink-2)' }}>把这串 Token 发给 {nm} 里的 Daycore 机器人，10 分钟内有效：</div>
                <div className="fl-token"><Ic n="key" size={14} style={{ color: 'var(--dc-accent)' }} /><span style={{ flex: 1 }}>{bindFor.token}</span>
                  <button className="x" style={{ opacity: 1 }} onClick={() => { try { navigator.clipboard.writeText(bindFor.token); } catch (e) { } FL.snack({ label: '✓ 复制好了' }); }}><Ic n="copy" size={13} /></button></div>
                <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
                  <button className="dc4-btn sm sec" onClick={() => setBindFor(null)}>取消</button>
                  <button className="dc4-btn sm" onClick={() => { core.verifyChannel(ch.id); setBindFor(null); FL.snack({ label: '✓ 绑定好了：' + nm, sub: '演示环境 · 模拟对方发来 Token' }); }}><Ic n="check" size={13} />模拟完成绑定</button>
                </div>
              </div>}
            </React.Fragment>;
          })}
          <div className="fl-line" style={{ color: 'var(--dc-ink-3)' }}><span className="lb">推送预算 ≤3 条/天——绑了渠道也一样遵守。</span></div>

          <div className="fl-sec">记录类别</div>
          {core.listCategories().map((c) => <div key={c.id} className="fl-it" style={{ alignItems: 'center' }}>
            <Ic n={CATI[c.id] || 'file'} size={15} style={{ color: 'var(--dc-accent)', flex: 'none' }} />
            <div className="bd"><div className="t">{c.label}</div>{c.locked && <div className="s">基础类别，始终开启</div>}</div>
            {c.locked ? <Ic n="check" size={14} style={{ color: 'var(--dc-ink-3)' }} /> : <Switch on={c.enabled} onToggle={() => core.setCategory(c.id, !c.enabled)} />}
          </div>)}

          <div className="fl-sec">作息节律</div>
          <div className="fl-it">
            <Ic n="moon" size={15} style={{ marginTop: 2, color: 'var(--dc-accent)' }} />
            <div className="bd"><div className="t">约 {core.state.rhythm.sleep} 睡 · {core.state.rhythm.wake} 起</div><div className="s">{core.state.rhythm.pinned ? '已钉住——按你说的来' : '学来的，不是设置的——auto-plan 跑在你入睡后的静默窗'}</div></div>
            <button className="dc4-btn sm sec" onClick={() => { core.state.rhythm.pinned = !core.state.rhythm.pinned; core.emit(); }}>{core.state.rhythm.pinned ? '取消钉住' : '钉住'}</button>
          </div>

          <div className="fl-sec">演示场景</div>
          <div className="fl-seg">{[['healthy', '健康日'], ['crisis', '危机日'], ['cold', '冷启动']].map(([v, l]) => <button key={v} className={core.state.scenario === v ? 'on' : ''} onClick={() => { core.scenarioSet(v); onClose(); }}>{l}</button>)}</div>
          <div className="fl-line" style={{ color: 'var(--dc-ink-3)' }}><span className="lb">换一个演示场景——琉璃和汀那边也会跟着变。</span></div>

          <div className="fl-sec">数据与关于</div>
          <div className="fl-it" style={{ alignItems: 'center', cursor: 'pointer' }} onClick={() => { core.scenarioSet(core.state.scenario); FL.snack({ label: '✓ 重新播种好了' }); }}>
            <Ic n="refresh" size={15} style={{ color: 'var(--dc-accent)', flex: 'none' }} />
            <div className="bd"><div className="t">重新播种当前场景</div><div className="s">把演示账本恢复到初始状态</div></div>
          </div>
          <div className="fl-line"><span className="lb">纸屿 · 顺流——一天是一篇你和{core.state.assistantName}共同书写的日志。这条流不是聊天史：它是账本的渲染，只有事实进流。<span className="sb" style={{ marginTop: 4 }}>每一步都记在流里，随时翻回去改 · 任何动作 3 秒内可撤</span></span></div>
          <div style={{ display: 'flex', gap: 8, marginTop: 6, paddingBottom: 8 }}>
            <a className="dc4-btn sm sec" href="../index.html"><Ic n="home" size={14} />回三前端门厅</a>
            <a className="dc4-btn sm sec" href="../liuli/admin.html"><Ic n="key" size={13} />管理控制台</a>
          </div>
        </div>
        {preview && <div className="fl-pfoot"><div className="fl-prevrow">
          <Ic n="palette" size={15} style={{ color: 'var(--dc-accent)', flex: 'none' }} />
          <span className="nm">预览中：{preview.candidate.name}</span>
          <button className="dc4-btn sm sec" onClick={gen} disabled={busy}>{busy ? '…' : '再试'}</button>
          <button className="dc4-btn sm sec" onClick={cancelP}>取消</button>
          <button className="dc4-btn sm" onClick={saveP}>保存并应用</button>
        </div></div>}
      </div>
    </React.Fragment>;
  }

  window.FLP = Object.assign(window.FLP || {}, { Settings });
})();
