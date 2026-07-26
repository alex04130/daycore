// 汀 — 零件：撤销条 / 整屏提案卡 / 心情过渡帧 / 抽屉与 sheet / 下一件 peek
(function () {
  'use strict';
  const { useState, useEffect, useRef } = React;
  const C = window.DcCore;
  const Ic = ({ n, size, style }) => <span style={Object.assign({ display: 'inline-flex', lineHeight: 0 }, style)} dangerouslySetInnerHTML={{ __html: DcIcons.svg(n, { size: size || 15 }) }} />;
  function useCore() { const [, f] = React.useReducer((x) => x + 1, 0); useEffect(() => C.subscribe(f), []); return C; }
  function useTick(ms) { const [, f] = React.useReducer((x) => x + 1, 0); useEffect(() => { const t = setInterval(f, ms); return () => clearInterval(t); }, [ms]); }

  const UB = { cur: null, subs: new Set(), t: null };
  function tgUndo(o) { UB.cur = Object.assign({ id: C.uid('u') }, o); UB.subs.forEach((f) => f(UB.cur)); clearTimeout(UB.t); UB.t = setTimeout(() => { UB.cur = null; UB.subs.forEach((f) => f(null)); }, o.ttl || 5200); }
  window.tgUndo = tgUndo;
  function UndoBar() {
    const [s, setS] = useState(UB.cur);
    useEffect(() => { UB.subs.add(setS); return () => UB.subs.delete(setS); }, []);
    if (!s) return null;
    return <div className="tg-undo" data-screen-label="撤销条">
      <span>{s.label}</span>
      {s.opId && <button onClick={() => { if (C.undo(s.opId)) tgUndo({ label: '已撤销——原来那笔也留着' }); }}>撤销</button>}
      <button className="x" onClick={() => { UB.cur = null; setS(null); }}>✕</button>
    </div>;
  }

  function Sheet({ title, icon, onClose, children, label }) {
    return <React.Fragment>
      <div className="tg-veil" onClick={onClose}></div>
      <div className="tg-sheet" data-screen-label={label}>
        <div className="tg-shead"><h3><span className="ic"><Ic n={icon} size={15} /></span>{title}</h3><button className="tg-dots" onClick={onClose}><Ic n="x" size={16} /></button></div>
        <div className="tg-sbody">{children}</div>
      </div>
    </React.Fragment>;
  }

  // 整屏提案（一滑接受、一滑推开）
  function PropCard({ p, queueN, onSkipAll }) {
    const core = useCore(); useTick(30e3);
    const [dx, setDx] = useState(0);
    const ref = useRef(null);
    if (!p || p.state !== 'pending') return null;
    const left = Math.max(0, (p.expiresAt - C.nowMs()) / 3600e3);
    const ttl = (p.ttlRule === 'silent_accept' ? '没理它就当接受' : '没理它就作罢') + ' · ' + (left >= 1 ? Math.round(left) + 'h' : Math.max(5, Math.round(left * 60)) + 'm');
    const act = (fn, label) => { const r = fn(); tgUndo({ label, opId: r && r.opId }); };
    const accept = () => act(() => core.accept(p.id), (p.kind === 'timed' ? '放进日程了：' : '接受：') + p.title);
    const reject = () => act(() => core.reject(p.id), '推开了——它会学乖一点');
    const down = (e) => {
      if (e.target.closest('button')) return;
      const x0 = e.clientX; ref.current = x0;
      const mv = (ev) => setDx(ev.clientX - x0);
      const up = (ev) => {
        window.removeEventListener('pointermove', mv); window.removeEventListener('pointerup', up);
        const d = ev.clientX - x0; setDx(0);
        if (d > 90) accept(); else if (d < -90) reject();
      };
      window.addEventListener('pointermove', mv); window.addEventListener('pointerup', up);
    };
    return <div className="tg-card" onPointerDown={down} style={{ transform: 'translateX(' + dx + 'px) rotate(' + dx / 26 + 'deg)', transition: dx ? 'none' : 'transform .25s cubic-bezier(.22,.9,.24,1)' }}>
      <div className="tg-prop" data-screen-label="整屏提案">
        <span className="ttl">{ttl}</span>
        <div className="tg-eyebrow"><Ic n="sparkle" size={13} />{p.kind === 'timed' ? '这段空着 · ' + (p.start || '') : p.domain === 'care' ? '关怀 · 不是训斥' : '需要你点头'}<span className="ln"></span>{queueN > 1 && <span className="tg-qbadge">还有 {queueN - 1} 张</span>}</div>
        <div className="tg-title sm">{p.title}</div>
        <div className="why">{p.summary}——{p.reason}</div>
        {p.rows && <div className="tg-rows">{p.rows.map((r) => <div key={r.id} className={'tg-row ' + (r.state === 'accepted' ? 'acc' : r.state === 'rejected' ? 'rej' : '')}>
          <span className="lb">{r.label}</span>
          {r.state === 'pending' ? <React.Fragment>
            <button className="rb y" onClick={() => act(() => core.accept(p.id, r.id), '接受：' + r.label)}><Ic n="check" size={14} /></button>
            <button className="rb n" onClick={() => act(() => core.reject(p.id, r.id), '拒绝了一行')}><Ic n="x" size={13} /></button>
          </React.Fragment> : <Ic n={r.state === 'accepted' ? 'check' : 'x'} size={13} style={{ color: r.state === 'accepted' ? 'var(--accent)' : 'var(--ink3)' }} />}
        </div>)}</div>}
        <div className="ev"><Ic n="eye" size={12} />{p.evidence}</div>
        <div className="tg-actrow">
          <button className="tg-btn pri" onClick={accept}><Ic n="check" size={15} />{p.rows ? '全部接受' : '就这么办'}</button>
          <button className="tg-btn sec" onClick={reject}>推开</button>
          <button className="tg-btn ghost" onClick={() => { core.followUp(p.id, '为什么这样安排？'); window.tgOpenCompanionHint && window.tgOpenCompanionHint(); }}>追问</button>
          <button className="tg-btn ghost" onClick={onSkipAll}>先不看</button>
        </div>
        <div className="tg-sidecue l" style={{ opacity: dx < -30 ? 1 : 0 }}>推开 ←</div>
        <div className="tg-sidecue r" style={{ opacity: dx > 30 ? 1 : 0 }}>→ 接受</div>
      </div>
    </div>;
  }

  // 心情过渡帧（一次点击完成，可跳过）
  const QUICK_MOODS = [['😊', '开心'], ['😌', '平静'], ['🤩', '兴奋'], ['😐', '一般'], ['😪', '疲惫'], ['😣', '压力大']];
  function MoodFrame({ onDone }) {
    const core = useCore();
    return <div data-screen-label="心情过渡帧">
      <div className="tg-eyebrow mute">此刻<span className="ln"></span></div>
      <div className="tg-title sm">顺手记一下：现在感觉？</div>
      <div className="tg-sub">一次点选就够——也可以直接跳过，不欠任何东西。</div>
      <div className="tg-moodrow">{QUICK_MOODS.map(([e, l]) => <button key={l} onClick={() => { const r = core.moodRecord({ e, l, note: '', source: 'user' }); tgUndo({ label: '心情记下了：' + e + ' ' + l, opId: r.opId }); onDone(); }}><span className="e">{e}</span>{l}</button>)}</div>
      <div className="tg-actrow" style={{ justifyContent: 'center' }}><button className="tg-btn ghost" onClick={onDone}>跳过</button></div>
    </div>;
  }

  // 顶部 peek：今日迷你轴（只读为主）
  function Peek({ onClose, onLedger, onOutlook }) {
    const core = useCore();
    const T = C.todayIso();
    const rows = [
      ...core.blocksOf(T).map((b) => ({ k: 'b', at: C.toMin(b.start), b })),
      ...core.ghostsOf(T).map((g) => ({ k: 'g', at: C.toMin(g.start), g })),
    ].sort((a, b) => a.at - b.at);
    const n = C.nowMin();
    let nowDrawn = false;
    return <React.Fragment>
      <div className="tg-veil" onClick={onClose}></div>
      <div className="tg-peek" data-screen-label="下拉全天 peek">
        <div className="tg-shead"><h3><span className="ic"><Ic n="calendar" size={15} /></span>{C.relDay(T)} · {C.fmtDate(T, true)}</h3><span style={{ fontSize: 10.5, color: 'var(--ink3)' }}>全天一览 · 看完收起</span></div>
        <div className="tg-axis">
          {rows.length === 0 && <div className="tg-sub" style={{ textAlign: 'center', padding: '18px 0' }}>今天还是空的——不是坏事。</div>}
          {rows.map((r, i) => {
            const mark = !nowDrawn && r.at > n; if (mark) nowDrawn = true;
            const row = r.k === 'b'
              ? (() => { const ph = core.phaseOf(r.b); return <div className={'tg-ax' + (ph === 'stone' || ph === 'recon' ? ' past' : '')}>
                  <span className="tm">{r.b.start}</span>
                  <span className={'d ' + (ph === 'now' ? 'now' : r.b.status === 'done' ? 'done' : r.b.status === 'missed' ? 'miss' : ph === 'stone' ? 'stone' : '')}></span>
                  <span className="t">{r.b.title}</span>
                  {ph === 'now' && <span className="tg-qbadge" style={{ color: 'var(--accent)' }}>此刻</span>}
                </div>; })()
              : <div className="tg-ax ghost"><span className="tm">{r.g.start}</span><span className="d"></span><span className="t">想放 · {r.g.title}</span></div>;
            return <React.Fragment key={i}>
              {mark && <div className="tg-axnow"><span className="chip">{C.toHM(n)}</span><span className="ln"></span></div>}
              {row}
            </React.Fragment>;
          })}
          {!nowDrawn && rows.length > 0 && <div className="tg-axnow"><span className="chip">{C.toHM(n)}</span><span className="ln"></span></div>}
        </div>
        <div className="tg-peekfoot">
          <button className="tg-btn sec" style={{ flex: 1 }} onClick={onLedger}><Ic n="book" size={14} />足迹 · 账本</button>
          <button className="tg-btn sec" style={{ flex: 1 }} onClick={onOutlook}><Ic n="zap" size={14} />展望 · 雷达</button>
          <button className="tg-btn ghost" onClick={onClose}><Ic n="chevron-up" size={15} />收起</button>
        </div>
      </div>
    </React.Fragment>;
  }

  function LedgerSheet({ onClose }) {
    const core = useCore();
    return <Sheet title="足迹 · 账本" icon="book" onClose={onClose} label="账本 sheet">
      <div style={{ fontSize: 11, color: 'var(--ink3)', marginBottom: 6 }}>这里只增不减——撤销、后悔都各记一笔</div>
      {core.ledgerByDay(4).map((day) => <div key={day.date}>
        <div className="tg-cap">{C.relDay(day.date)}</div>
        {day.rows.map((o) => {
          const p = o.payload.type === 'proposal_expire' ? core.state.proposals.find((q) => q.id === o.payload.pid && q.state === 'expired') : null;
          return <div key={o.id} className={'tg-li' + (o.silent ? ' silent' : '') + (o.undone ? ' undone' : '')}>
            <span className="tm">{C.fmtHM(o.at)}</span>
            <div className="bd"><div className="lb">{o.label}</div>{o.sub && <div className="sb">{o.sub}</div>}</div>
            <span className={'who' + (o.agent ? '' : ' me')}>{o.agent ? '小禾' : '我'}</span>
            {o.undoable && !o.undone && <button className="un" onClick={() => { if (core.undo(o.id)) tgUndo({ label: '已撤销 · ' + o.label }); }}>撤销</button>}
            {p && <button className="un" onClick={() => { core.redeem(p.id); tgUndo({ label: '捞回来了：' + p.title }); }}>捞回</button>}
          </div>;
        })}
      </div>)}
    </Sheet>;
  }

  function OutlookSheet({ onClose }) {
    const core = useCore();
    return <Sheet title="展望 · 快到的截止" icon="zap" onClose={onClose} label="展望 sheet">
      {core.radar().map((a) => <div key={a.id} className="tg-li">
        <span className="tg-u" data-u={a.urgency}></span>
        <div className="bd"><div className="lb">{a.title}</div><div className="sb">{a.course}</div></div>
        <span className="sb" style={{ fontVariantNumeric: 'tabular-nums', fontWeight: 650, color: a.urgency === 2 ? 'var(--warm)' : 'var(--ink2)' }}>{a.dueLabel}</span>
      </div>)}
      <div style={{ fontSize: 11, color: 'var(--ink3)', margin: '8px 0 4px', textAlign: 'center' }}>远期就看这张清单</div>
      <div className="tg-cap">想做的事</div>
      {core.state.wishes.filter((w) => w.status === 'active').map((w) => <div key={w.id} className="tg-li">
        <Ic n="star" size={13} style={{ color: 'var(--warm)', marginTop: 3 }} />
        <div className="bd"><div className="lb">{w.text}</div><div className="sb">{w.effort ? '约 ' + w.effort + ' 分钟 · ' : ''}有空档时会整屏浮上来问你</div></div>
        <button className="un" style={{ color: 'var(--accent)' }} onClick={() => core.wishSet(w.id, 'done')}>达成</button>
        <button className="un" style={{ color: 'var(--ink3)' }} onClick={() => core.wishSet(w.id, 'dropped')}>放下</button>
      </div>)}
    </Sheet>;
  }

  function MenuSheet({ onClose }) {
    const core = useCore();
    const theme = core.theme('ting') || 'night';
    return <Sheet title="汀 · 此刻" icon="moon" onClose={onClose} label="菜单 sheet">
      <div className="tg-cap">水色</div>
      <div className="tg-seg">{[['night', '夜汀'], ['dawn', '晨汀']].map(([v, l]) => <button key={v} className={theme === v ? 'on' : ''} onClick={() => core.setTheme('ting', v)}>{l}</button>)}</div>
      <div className="tg-cap">演示场景</div>
      <div className="tg-seg">{[['healthy', '健康日'], ['crisis', '危机日'], ['cold', '冷启动']].map(([v, l]) => <button key={v} className={core.state.scenario === v ? 'on' : ''} onClick={() => { core.scenarioSet(v); onClose(); }}>{l}</button>)}</div>
      <div className="tg-li"><Ic n="moon" size={13} style={{ marginTop: 3, color: 'var(--accent)' }} /><div className="bd"><div className="lb">节律：约 {core.state.rhythm.sleep} 睡 · {core.state.rhythm.wake} 起</div><div className="sb">熙太久了我会劰你歇歇，语气是担心不是训斥</div></div></div>
      <a className="tg-menuitem" href="../liuli/index.html"><span className="ic"><Ic n="layout" size={15} /></span>换到 琉璃 · 长卷（同一账号，同一账本）</a>
      <a className="tg-menuitem" href="../zhiyu/index.html"><span className="ic"><Ic n="file" size={15} /></span>换到 纸屿 · 顺流</a>
      <a className="tg-menuitem" href="../index.html"><span className="ic"><Ic n="home" size={15} /></span>回三前端门厅</a>
      <div style={{ fontSize: 11.5, color: 'var(--ink3)', marginTop: 12, lineHeight: 1.7 }}>汀：水边的小平地。画面永远只回答一个问题——现在做什么。给「连日历都不想看」的人。</div>
    </Sheet>;
  }

  // 收集胶囊 sheet（语音优先的视觉）
  function CaptureSheet({ onClose }) {
    const core = useCore();
    const [text, setText] = useState('');
    const [ans, setAns] = useState(null);
    const fr = useRef(null);
    const onAttach = (e) => {
      const f = e.target.files[0]; if (!f) return; e.target.value = '';
      const r = core.materialAdd({ title: f.name, body: '上传的文件 · ' + Math.round(f.size / 1024) + ' KB', kind: 'life', source: 'upload' }, true);
      tgUndo({ label: '✓ 收好了：' + f.name, opId: r.opId });
      onClose();
    };
    const submit = (t0) => {
      const t = (t0 || text).trim(); if (!t) return;
      const r = core.captureIntent(t);
      setText('');
      if (r && r.kind === 'answer') { setAns(r); return; }
      if (r) tgUndo({ label: '✓ ' + r.label, opId: r.opId });
      onClose();
    };
    return <Sheet title="说一句就走" icon="chat" onClose={onClose} label="收集 sheet">
      {ans ? <div>
        <div className="tg-cap">{ans.title}</div>
        {ans.lines.map((l, i) => <div key={i} className="tg-li"><span className="lb">{l}</span></div>)}
        <div className="tg-actrow"><button className="tg-btn sec" onClick={onClose}>好（界面即答案）</button></div>
      </div> : <React.Fragment>
        <div className="tg-inrow">
          <button className="tg-btn sec" style={{ width: 46, padding: 0, borderRadius: 14, flex: 'none' }} title="附件" onClick={() => fr.current.click()}><Ic n="link" size={16} /></button>
          <input ref={fr} type="file" style={{ display: 'none' }} onChange={onAttach} />
          <input autoFocus placeholder="杂念、心情、安排——丢进来就行" value={text} onChange={(e) => setText(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && submit()} />
          <button className="tg-btn pri" style={{ width: 46, padding: 0, borderRadius: 14 }} title="发送" onClick={() => submit()}><Ic n="send" size={16} /></button>
        </div>
        <div className="tg-quick">
          {['明天上午十点 复习微积分', '想学一点吉他', '压力好大，明天还要考试', '最近的截止是什么时候？'].map((q) => <button key={q} onClick={() => submit(q)}>{q}</button>)}
        </div>
        <div style={{ fontSize: 11, color: 'var(--ink3)', marginTop: 12, textAlign: 'center' }}>随手说一句 · 放哪由小禾决定，错了随手撤</div>
      </React.Fragment>}
    </Sheet>;
  }

  function PushStrip() {
    const core = useCore();
    const p = core.unreadPush();
    if (!p) return null;
    return <div className="tg-push" data-screen-label="推送条">
      <span className="dot"></span>
      <div className="bd"><b>{p.title}</b><div className="s">{p.body}</div></div>
      <button onClick={() => core.markPushRead(p.id)}>知道了</button>
    </div>;
  }

  Object.assign(window, { TG: { Ic, useCore, useTick, tgUndo, UndoBar, Sheet, PropCard, MoodFrame, Peek, LedgerSheet, OutlookSheet, MenuSheet, CaptureSheet, PushStrip } });
})();
