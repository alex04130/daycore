// 顺流 — 侧入口面板（伙伴/资料/展望/设置）+ 桌面常驻上下文栏
(function () {
  'use strict';
  const { useState, useEffect, useRef } = React;
  const C = window.DcCore, FL = window.FL, Ic = FL.Ic;

  function Panel({ title, icon, onClose, children, label, footer }) {
    useEffect(() => { const esc = (e) => e.key === 'Escape' && onClose(); window.addEventListener('keydown', esc); return () => window.removeEventListener('keydown', esc); }, []);
    return <React.Fragment>
      <div className="fl-veil" onClick={onClose}></div>
      <div className="fl-panel" data-screen-label={label}>
        <div className="fl-phead"><h3><Ic n={icon} size={16} style={{ color: 'var(--dc-accent)' }} />{title}</h3><button className="dc4-iconbtn" onClick={onClose}><Ic n="x" size={16} /></button></div>
        <div className="fl-pbody">{children}</div>
        {footer && <div className="fl-pfoot">{footer}</div>}
      </div>
    </React.Fragment>;
  }

  function Materials({ onClose }) {
    const core = FL.useCore();
    const fileRef = useRef(null);
    return <Panel title="资料" icon="book" onClose={onClose} label="资料面板">
      <div className="fl-it" style={{ borderStyle: 'dashed', cursor: 'pointer' }} onClick={() => fileRef.current.click()}>
        <Ic n="upload" size={16} style={{ color: 'var(--dc-accent)', marginTop: 2 }} />
        <div className="bd"><div className="t">丢个文件进来</div><div className="s">拖进来就自动收好，翻资料随时能改</div></div>
      </div>
      <input ref={fileRef} type="file" style={{ display: 'none' }} onChange={(e) => { const f = e.target.files[0]; if (!f) return; const r = core.materialAdd({ title: f.name, body: '上传的文件 · ' + Math.round(f.size / 1024) + ' KB', kind: 'life', source: 'upload' }, true); FL.snack({ label: '归档了：' + f.name, opId: r.opId }); e.target.value = ''; }} />
      <div className="fl-sec">资料库<span className="n">{core.state.materials.length}</span></div>
      {core.state.materials.map((m) => <div key={m.id} className="fl-it">
        <span className="fl-tag" style={{ marginTop: 2 }}>{m.kind === 'academic' ? '学业' : '生活'}</span>
        <div className="bd"><div className="t">{m.title}</div>{m.body && <div className="s">{m.body}</div>}</div>
        <button className="x" onClick={() => FL.snack({ label: '移出资料：' + m.title, opId: core.materialRemove(m.id) })}><Ic n="trash" size={14} /></button>
      </div>)}
      <div className="fl-sec">记忆 · 关于你自己<span className="n">{core.state.memories.length}</span></div>
      {core.state.memories.map((m) => <div key={m.id} className="fl-it">
        <span className="fl-tag" style={{ marginTop: 2, color: 'var(--dc-accent)', background: 'var(--dc-accent-soft)' }}>{m.type === 'preference' ? '偏好' : '惦记'}</span>
        <div className="bd"><div className="t">{m.text}</div></div>
        <button className="x" onClick={() => FL.snack({ label: '忘掉了', opId: core.memoryRemove(m.id) })}><Ic n="trash" size={14} /></button>
      </div>)}
    </Panel>;
  }

  function Outlook({ onClose }) {
    const core = FL.useCore();
    const [wish, setWish] = useState('');
    const addWish = () => { const t = wish.trim(); if (!t) return; const r = core.wishAdd(t, null); FL.snack({ label: r.dup ? '已经有一条了，帮你合并' : '记进「想做的事」了', opId: r.opId }); setWish(''); };
    return <Panel title="展望" icon="zap" onClose={onClose} label="展望面板" footer={<div style={{ display: 'flex', gap: 8 }}>
      <input className="dc4-input" style={{ height: 40, fontSize: 13 }} placeholder="想做点什么：想学 / 想去 / 想买…" value={wish} onChange={(e) => setWish(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && addWish()} />
      <button className="dc4-btn" style={{ flex: 'none' }} onClick={addWish}><Ic n="plus" size={15} />记下</button>
    </div>}>
      <div className="fl-sec">快到的截止<span className="n">{core.radar().length}</span></div>
      <div className="fl-radar">{core.radar().map((a) => <div key={a.id} className="fl-it">
        <span className="fl-u" data-u={a.urgency}></span>
        <div className="bd"><div className="t">{a.title}</div><div className="s">{a.course}</div></div>
        <span className="fl-dl">{a.dueLabel}</span>
      </div>)}</div>
      <div className="fl-sec">想做的事<span className="n">{core.state.wishes.filter((w) => w.status === 'active').length}</span></div>
      {core.state.wishes.filter((w) => w.status === 'active').map((w) => <div key={w.id} className="fl-it">
        <Ic n="star" size={14} style={{ color: 'var(--dc-warn)', marginTop: 3 }} />
        <div className="bd"><div className="t">{w.text}</div><div className="s">{w.effort ? '约 ' + w.effort + ' 分钟 · ' : ''}有空档时小禾会提上来问你</div></div>
        <button className="x" title="达成" style={{ color: 'var(--dc-ok)' }} onClick={() => core.wishSet(w.id, 'done')}><Ic n="check" size={14} /></button>
        <button className="x" title="放下" onClick={() => core.wishSet(w.id, 'dropped')}><Ic n="x" size={13} /></button>
      </div>)}
    </Panel>;
  }

  function Companion({ onClose }) {
    const core = FL.useCore();
    const [text, setText] = useState('');
    const body = useRef(null), fr = useRef(null);
    const onAttach = (e) => {
      const f = e.target.files[0]; if (!f) return; e.target.value = '';
      const r = core.materialAdd({ title: f.name, body: '上传的文件 · ' + Math.round(f.size / 1024) + ' KB', kind: 'life', source: 'upload' }, true);
      core.state.chat.push({ role: 'user', content: '［附件］' + f.name, at: C.nowMs() });
      core.state.chat.push({ role: 'assistant', content: '收到，先替你收进资料库了。要我从里面读出课表、截止或重点，说一声就行。', at: C.nowMs() + 1, tools: [{ label: '收进资料 · 可撤销', opId: r.opId }] });
      core.emit();
    };
    useEffect(() => {
      const ex = core.consumeExchange();
      if (ex) {
        core.state.chat.push({ role: 'user', content: ex.q || '为什么这样安排？', at: C.nowMs(), exch: true });
        core.state.chat.push({ role: 'assistant', content: ex.snapshot ? '「' + ex.snapshot.title + '」是这么来的：' + ex.snapshot.reason : '我在，带着刚才那条的上下文。', at: C.nowMs() + 1, tools: ex.snapshot && ex.snapshot.evidence ? [{ label: ex.snapshot.evidence }] : null });
        core.emit();
      }
    }, []);
    useEffect(() => { if (body.current) body.current.scrollTop = body.current.scrollHeight; }, [core.state.chat.length, core.state.typing]);
    const send = () => { const t = text.trim(); if (!t) return; core.chatSend(t); setText(''); };
    const P = (pid) => core.state.proposals.find((p) => p.id === pid);
    return <React.Fragment>
      <div className="fl-veil" onClick={onClose}></div>
      <div className="fl-panel" data-screen-label="伙伴面板">
        <div className="fl-phead"><h3><Ic n="chat" size={16} style={{ color: 'var(--dc-accent)' }} />伙伴 · {core.state.assistantName}</h3><button className="dc4-iconbtn" onClick={onClose}><Ic n="x" size={16} /></button></div>
        <div className="fl-pbody" ref={body}>
          <div className="fl-chat">
            {core.state.chat.map((m, i) => <React.Fragment key={i}>
              {m.exch && <div className="fl-exch">带着刚才在流里的上下文过来 · 10 分钟内有效</div>}
              <div className={'fl-msg ' + (m.role === 'user' ? 'user' : 'ai')}>
                {m.content}
                {m.tools && <div className="fl-toolrow">{m.tools.map((t, j) => <span key={j} className="fl-toolchip"><Ic n="wand" size={11} />{t.label}{t.opId && <button onClick={() => { if (core.undo(t.opId)) FL.snack({ label: '已撤销' }); }}>撤销</button>}</span>)}</div>}
                {m.proposalId && P(m.proposalId) && <div style={{ marginTop: 8 }}><FL.PropInsert p={P(m.proposalId)} onFollow={() => { }} /></div>}
              </div>
            </React.Fragment>)}
            {core.state.typing && <div className="fl-msg ai" style={{ color: 'var(--dc-ink-3)' }}>{core.state.assistantName}正在想…</div>}
          </div>
        </div>
        <div style={{ display: 'flex', gap: 8, padding: '10px 16px 16px', flex: 'none', alignItems: 'center' }}>
          <button className="dc4-iconbtn solid" title="附件——上传即 intent" onClick={() => fr.current.click()}><Ic n="link" size={16} /></button>
          <input ref={fr} type="file" style={{ display: 'none' }} onChange={onAttach} />
          <input className="dc4-input" style={{ borderRadius: 999 }} placeholder={'跟' + core.state.assistantName + '说点什么…'} value={text} onChange={(e) => setText(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && send()} />
          <button className="dc4-btn" style={{ borderRadius: 999, width: 44, padding: 0, flex: 'none' }} onClick={send}><Ic n="send" size={16} /></button>
        </div>
      </div>
    </React.Fragment>;
  }

  function Rail({ onMore }) {
    const core = FL.useCore();
    const n = core.l0Today();
    return <aside className="fl-rail" data-screen-label="边栏 · 手记">
      <FL.MoodCheck compact onMore={onMore} />
      <div className="fl-rcard">
        <h5>今天的安静</h5>
        <p className="fl-rnote">{n > 0 ? '小禾悄悄替你打理了 ' + n + ' 件小事——都在流里，随时能翻回去。' : '还没替你动什么手——需要时我会悄悄来，不打扰你。'}</p>
      </div>
      <div className="fl-rcard">
        <h5>周信</h5>
        <p className="fl-rnote">周日晚上，这一周的河、心情与后悔会写成一封散文体的信——不是饼图。</p>
      </div>
    </aside>;
  }

  Object.assign(window, { FLP: Object.assign(window.FLP || {}, { Materials, Outlook, Companion, Rail }) });
})();
