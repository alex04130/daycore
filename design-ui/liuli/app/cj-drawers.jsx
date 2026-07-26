// 长卷 — 书签视图（正常 tab 切换：内容区动画，不是弹窗）：资料 / 展望 / 足迹 / 伙伴
(function () {
  'use strict';
  const { useState, useEffect, useRef } = React;
  const C = window.DcCore, IC = window.DcIcons, CJ = window.CJ;

  function Page({ title, icon, children, narrow, label, note }) {
    return <div className="cj-page" data-screen-label={label}>
      <div className={'inner' + (narrow ? ' narrow' : '')}>
        <h2 className="pt"><span className="ic">{icon}</span>{title}{note && <span className="pn">{note}</span>}</h2>
        {children}
      </div>
    </div>;
  }

  // ── 资料 ──
  function Materials() {
    const core = CJ.useCore();
    const fileRef = useRef(null);
    const KIND = { academic: ['学业', ''], life: ['生活', 'life'] };
    return <Page title="资料" icon={<IC.BookOpen size={19} />} label="资料视图" note="拖进来自动收好，翻资料随时能改">
      <div className="cj-grid2">
        <div>
          <div className="cj-item" style={{ borderStyle: 'dashed', cursor: 'pointer' }} onClick={() => fileRef.current.click()}>
            <span style={{ color: 'var(--accent)', marginTop: 2 }}><IC.Upload size={16} /></span>
            <div className="bd"><div className="t">丢个文件进来</div><div className="s">拖进来就自动收好，需要时我帮你读出重点</div></div>
          </div>
          <input ref={fileRef} type="file" style={{ display: 'none' }} onChange={(e) => { const f = e.target.files[0]; if (!f) return; const r = core.materialAdd({ title: f.name, body: '上传的文件 · ' + Math.round(f.size / 1024) + ' KB', kind: 'life', source: 'upload' }, true); CJ.cjToast({ label: '收进资料库了：' + f.name, opId: r.opId }); e.target.value = ''; }} />
          <div className="cj-sec">资料库<span className="n">{core.state.materials.length}</span></div>
          {core.state.materials.map((m) => <div key={m.id} className="cj-item">
            <span className={'cj-kind ' + (KIND[m.kind] ? KIND[m.kind][1] : '')}>{(KIND[m.kind] || ['其他'])[0]}</span>
            <div className="bd"><div className="t">{m.title}</div>{m.body && <div className="s">{m.body}</div>}</div>
            <button className="x" title="移出" onClick={() => { const opId = core.materialRemove(m.id); CJ.cjToast({ label: '移出资料：' + m.title, opId }); }}><IC.Trash size={13} /></button>
          </div>)}
        </div>
        <div>
          <div className="cj-sec">记忆 · 关于你自己<span className="n">{core.state.memories.length}</span></div>
          {core.state.memories.map((m) => <div key={m.id} className="cj-item">
            <span className="cj-kind mem">{m.type === 'preference' ? '偏好' : '惦记'}</span>
            <div className="bd"><div className="t">{m.text}</div></div>
            <button className="x" title="忘掉" onClick={() => { const opId = core.memoryRemove(m.id); CJ.cjToast({ label: '忘掉了', opId }); }}><IC.Trash size={13} /></button>
          </div>)}
          <div className="cj-l0" style={{ marginTop: 14 }}>都看得见、改得动、撤得回</div>
        </div>
      </div>
    </Page>;
  }

  // ── 展望 ──
  function Outlook({ onGoDay }) {
    const core = CJ.useCore();
    const [wish, setWish] = useState('');
    const radar = core.radar();
    const tm = C.addDays(C.todayIso(), 1);
    const tomorrow = core.blocksOf(tm), tGhosts = core.ghostsOf(tm);
    const addWish = () => { const t = wish.trim(); if (!t) return; const r = core.wishAdd(t, null); CJ.cjToast({ label: r.dup ? '已经有一条了，帮你合并' : '记进想做清单了', opId: r.opId }); setWish(''); };
    return <Page title="展望" icon={<IC.Zap size={19} />} label="展望视图" note="远期就看这张截止清单">
      <div className="cj-grid2">
        <div>
          <div className="cj-sec">快到的截止<span className="n">{radar.length}</span></div>
          <div className="cj-radar">
            {radar.map((a) => <div key={a.id} className="cj-ritem" data-u={a.urgency}>
              <span className="u" data-u={a.urgency}></span>
              <div className="bd"><div className="t">{a.title}</div><div className="c">{a.course}</div></div>
              <span className="dl">{a.dueLabel}</span>
            </div>)}
          </div>
        </div>
        <div>
          <div className="cj-sec">明天一瞥</div>
          {tomorrow.length === 0 && !tGhosts.length && <div className="cj-item"><div className="bd"><div className="s">明天还是空的，今晚我会悄悄排好一版。</div></div></div>}
          {tomorrow.map((b) => <div key={b.id} className="cj-op" style={{ cursor: 'pointer' }} onClick={() => onGoDay(tm)}>
            <span className="tm">{b.start}</span><div className="bd"><div className="lb">{b.title}</div></div>
            <span className="who">{b.origin === 'auto' ? '小禾' : '我'}</span>
          </div>)}
          {tGhosts.map((g) => <div key={g.id} className="cj-op" style={{ cursor: 'pointer' }} onClick={() => onGoDay(tm)}>
            <span className="tm">{g.start}</span><div className="bd"><div className="lb" style={{ color: 'var(--ink3)' }}>{g.title}</div><div className="sb">想放 · 等你点头</div></div>
          </div>)}
          <div className="cj-sec" style={{ marginTop: 18 }}>想做的事<span className="n">{core.state.wishes.filter((w) => w.status === 'active').length}</span></div>
          {core.state.wishes.filter((w) => w.status === 'active').map((w) => <div key={w.id} className="cj-item">
            <div className="bd"><div className="t">{w.text}</div><div className="s">{w.effort ? '约 ' + w.effort + ' 分钟 · ' : ''}有空档时会提上来</div></div>
            <button className="x" title="达成了" onClick={() => core.wishSet(w.id, 'done')} style={{ color: 'var(--ok)' }}><IC.Check size={14} /></button>
            <button className="x" title="放下" onClick={() => core.wishSet(w.id, 'dropped')}><IC.X size={13} /></button>
          </div>)}
          <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
            <input placeholder="想做点什么：想学 / 想去 / 想买…" value={wish} onChange={(e) => setWish(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && addWish()}
              style={{ flex: 1, height: 36, borderRadius: 10, border: '1px solid var(--line)', background: 'var(--glass2)', padding: '0 12px', fontSize: 12.5 }} />
            <button className="cj-btn sec" onClick={addWish}><IC.Plus size={14} /></button>
          </div>
        </div>
      </div>
    </Page>;
  }

  // ── 足迹 ──
  function Trace() {
    const core = CJ.useCore();
    const [hideSilent, setHideSilent] = useState(false);
    const river = core.river();
    const maxN = Math.max(...river.map((r) => r.count), 1);
    const rhythm = core.state.rhythm;
    const pinRhythm = () => { core.state.rhythm.pinned = !core.state.rhythm.pinned; core.emit(); CJ.cjToast({ label: core.state.rhythm.pinned ? '节律钉住了——我不再自作主张调整' : '恢复学习你的节律', kind: 'info' }); };
    return <Page title="足迹" icon={<IC.Anchor size={19} />} label="足迹视图" note="近景是账本，远景是河">
      <div className="cj-grid2">
        <div>
          <div className="cj-sec">远景 · 十五天的河</div>
          <div className="cj-river">
            {river.map((r) => <div key={r.date} className={'cj-rday' + (r.date === C.todayIso() ? ' today' : '')} data-tone={r.tone} title={C.fmtDate(r.date, true) + ' · ' + r.count + ' 笔'}>
              <span className="e">{r.mood || ''}</span>
              <span className="bar" style={{ height: 8 + r.count / maxN * 44 }}></span>
              <span className="d">{Number(r.date.split('-')[2])}</span>
            </div>)}
          </div>
          <div className="cj-item" style={{ marginTop: 14 }}>
            <span style={{ color: 'var(--accent)', marginTop: 2 }}><IC.Moon size={15} /></span>
            <div className="bd"><div className="t">作息节律：约 {rhythm.sleep} 睡 · {rhythm.wake} 起</div>
              <div className="s">{rhythm.pinned ? '已钉住——按你说的来' : rhythm.learning ? '学来的，不是设置的——我在你睡后悄悄排好明天' : '还没学到数据，先用默认时刻表'}</div></div>
            <button className="cj-btn sec" style={{ height: 28, fontSize: 11 }} onClick={pinRhythm}><IC.Pin size={12} />{rhythm.pinned ? '取消钉住' : '钉住'}</button>
          </div>
          <div className="cj-item" style={{ marginTop: 14 }}>
            <span style={{ color: 'var(--warm)', marginTop: 2 }}><IC.Heart size={15} /></span>
            <div className="bd"><div className="t">周日有一封周信</div><div className="s">散文，不是饼图——这周的河、心情与后悔，都会写进去。</div></div>
          </div>
        </div>
        <div>
          <div className="cj-sec">近景 · 账本
            <span style={{ flex: 1 }}></span>
            <button className="cj-pill" style={{ height: 24, fontSize: 11 }} onClick={() => setHideSilent(!hideSilent)}>{hideSilent ? '显示静默' : '折叠静默'}</button>
          </div>
          {core.ledgerByDay(7).map((day) => <div key={day.date}>
            <div className="cj-sec" style={{ letterSpacing: 0, fontWeight: 650 }}>{C.relDay(day.date)}</div>
            {day.rows.filter((o) => !hideSilent || !o.silent).map((o) => {
              const expiredP = o.payload.type === 'proposal_expire' ? core.state.proposals.find((p) => p.id === o.payload.pid && p.state === 'expired') : null;
              return <div key={o.id} className={'cj-op' + (o.silent ? ' silent' : '') + (o.undone ? ' undone' : '')}>
                <span className="tm">{C.fmtHM(o.at)}</span>
                <div className="bd"><div className="lb">{o.label}</div>{o.sub && <div className="sb">{o.sub}</div>}</div>
                <span className={'who' + (o.agent ? '' : ' me')}>{o.agent ? '小禾' : '我'}</span>
                {o.undoable && !o.undone && <button className="un" onClick={() => { if (core.undo(o.id)) CJ.cjToast({ label: '已撤销 · ' + o.label, sub: '也记成了一笔', kind: 'info' }); }}>撤销</button>}
                {expiredP && <button className="un" style={{ opacity: 1 }} onClick={() => { core.redeem(expiredP.id); CJ.cjToast({ label: '捞回来了：' + expiredP.title, kind: 'info' }); }}>捞回</button>}
              </div>;
            })}
          </div>)}
        </div>
      </div>
    </Page>;
  }

  // ── 伙伴（唯一的对话流；输入用底部恒定输入框） ──
  function Companion() {
    const core = CJ.useCore();
    const body = useRef(null);
    useEffect(() => {
      const ex = core.consumeExchange();
      if (ex) {
        core.state.chat.push({ role: 'user', content: ex.q || '为什么这样安排？', at: C.nowMs(), exch: true });
        if (ex.snapshot) core.state.chat.push({ role: 'assistant', content: '「' + ex.snapshot.title + '」是这么来的：' + ex.snapshot.reason, at: C.nowMs() + 1, tools: ex.snapshot.evidence ? [{ label: ex.snapshot.evidence }] : null });
        else core.state.chat.push({ role: 'assistant', content: '我在。上一屏的事我都带着上下文呢，直接问。', at: C.nowMs() + 1 });
        core.emit();
      }
    }, []);
    useEffect(() => { if (body.current) body.current.scrollTop = body.current.scrollHeight; }, [core.state.chat.length, core.state.typing]);
    const P = (pid) => core.state.proposals.find((p) => p.id === pid);
    return <div className="cj-page" ref={body} data-screen-label="伙伴视图">
      <div className="inner narrow">
        <h2 className="pt"><span className="ic"><IC.MessageHeart size={19} /></span>伙伴 · {core.state.assistantName}<span className="pn">这里留着聊天记录 · 直接在下面说</span></h2>
        <div className="cj-chat">
          {core.state.chat.map((m, i) => <React.Fragment key={i}>
            {m.exch && <div className="cj-exch">接着刚才那条聊 · 10 分钟内有效</div>}
            <div className={'cj-msg ' + (m.role === 'user' ? 'user' : 'ai')}>
              {m.content}
              {m.tools && <div className="cj-tools">{m.tools.map((t, j) => <span key={j} className="cj-tool"><IC.Wand size={10} />{t.label}{t.opId && <button onClick={() => { if (core.undo(t.opId)) CJ.cjToast({ label: '已撤销', kind: 'info' }); }}>撤销</button>}</span>)}</div>}
              {m.proposalId && P(m.proposalId) && P(m.proposalId).state === 'pending' && <div className="cj-rows">
                {(P(m.proposalId).rows || []).map((r) => <div key={r.id} className={'cj-row ' + (r.state === 'accepted' ? 'acc' : r.state === 'rejected' ? 'rej' : '')}>
                  <span className="lb">{r.label}</span>
                  {r.state === 'pending' && <React.Fragment>
                    <button className="rb y" onClick={() => { const x = core.accept(m.proposalId, r.id); CJ.cjToast({ label: '接受：' + r.label, opId: x && x.opId }); }}><IC.Check size={13} /></button>
                    <button className="rb n" onClick={() => core.reject(m.proposalId, r.id)}><IC.X size={12} /></button>
                  </React.Fragment>}
                </div>)}
              </div>}
              {m.proposalId && P(m.proposalId) && P(m.proposalId).state !== 'pending' && <div className="cj-tools"><span className="cj-tool">提议 · {P(m.proposalId).state === 'accepted' ? '已接受' : P(m.proposalId).state === 'rejected' ? '已谢绝' : '已过期（可捞回）'}</span></div>}
            </div>
          </React.Fragment>)}
          {core.state.typing && <div className="cj-typing glass" style={{ borderRadius: 16 }}><i></i><i></i><i></i></div>}
        </div>
      </div>
    </div>;
  }

  Object.assign(window, { CJDrawers: { Materials, Outlook, Trace, Companion } });
})();
