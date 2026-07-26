// 顺流 — 零件：条目组件 / 插页 / 菜单 / 面板 / 撤销条
(function () {
  'use strict';
  const { useState, useEffect, useRef } = React;
  const C = window.DcCore;
  const Ic = ({ n, size, style }) => <span style={Object.assign({ display: 'inline-flex', lineHeight: 0 }, style)} dangerouslySetInnerHTML={{ __html: DcIcons.svg(n, { size: size || 15 }) }} />;
  function useCore() { const [, f] = React.useReducer((x) => x + 1, 0); useEffect(() => C.subscribe(f), []); return C; }

  // 撤销条（单条最新）
  const SB = { cur: null, subs: new Set(), t: null };
  function snack(o) { SB.cur = Object.assign({ id: C.uid('s') }, o); SB.subs.forEach((f) => f(SB.cur)); clearTimeout(SB.t); SB.t = setTimeout(() => { SB.cur = null; SB.subs.forEach((f) => f(null)); }, o.ttl || 5600); }
  window.flSnack = snack;
  function Snack() {
    const [s, setS] = useState(SB.cur);
    useEffect(() => { SB.subs.add(setS); return () => SB.subs.delete(setS); }, []);
    if (!s) return null;
    const close = () => { SB.cur = null; setS(null); };
    return <div className="fl-snack" data-screen-label="撤销条">
      <span>{s.label}{s.sub && <span className="sb">　{s.sub}</span>}</span>
      {s.onView && <button onClick={() => { s.onView(); close(); }}>去看看</button>}
      {s.opId && <button className="u" onClick={() => { if (C.undo(s.opId)) snack({ label: '已撤销', sub: '原来那笔也留着，随时能翻回去' }); else close(); }}>撤销</button>}
      <button onClick={close} style={{ padding: '5px 9px' }}>✕</button>
    </div>;
  }

  // 条目菜单
  function EntryMenu({ b, at, onClose, onNote }) {
    const core = useCore();
    const phase = core.phaseOf(b);
    const [confirm, setConfirm] = useState(null);
    const style = { left: Math.max(8, Math.min(at.x - 170, window.innerWidth - 200)), top: Math.min(at.y + 6, window.innerHeight - 300) };
    const done = (label, opId) => { snack({ label, opId }); onClose(); };
    const move1 = (confirmed) => {
      const to = { date: C.addDays(b.date, 1), start: b.start };
      if (confirmed) to._confirmed = true;
      const r = core.moveBlock(b.id, to);
      if (r && r.stone) return done('过去的记录不改写——用「重新安排」放个新的');
      if (r && r.locked === 'hard') return done(r.reason + '——改不了时间，但可以请假或标记冲突');
      if (r && r.locked === 'soft') { setConfirm(r.reason); return; }
      done('挪到明天了', r && r.opId);
    };
    return <React.Fragment>
      <div className="fl-veil" style={{ background: 'transparent' }} onClick={onClose}></div>
      <div className="fl-menu" style={style}>
        <div className="cap">{phase === 'stone' ? '已成记录 · 只能补记' : phase === 'recon' ? '刚过去 · 还能改' : b.origin === 'auto' ? '小禾放的' : '我放的'}</div>
        {phase !== 'stone' && <button onClick={() => done('完成：' + b.title, core.setStatus(b.id, 'done'))}><Ic n="check" />完成了</button>}
        {(phase === 'recon' || phase === 'stone') && !b.reconciled && <React.Fragment>
          <button onClick={() => done('记完成了', core.reconcile(b.id, 'done', ''))}><Ic n="check" />记完成了</button>
          <button onClick={() => done('记没做，没关系', core.reconcile(b.id, 'missed', ''))}><Ic n="minus" />记没做（没关系）</button>
          {phase === 'stone' && <button onClick={() => done('其实做了', core.reconcile(b.id, 'did', ''))}><Ic n="sparkle" />其实做了</button>}
        </React.Fragment>}
        {phase !== 'stone' && (b.lockLevel === 'hard'
          ? <React.Fragment>
            <div className="fl-menu-lock"><span className="fl-pin sm" aria-hidden="true"></span>{b.lockReason}——时间不由你定</div>
            <button onClick={() => { window.flPrefill && window.flPrefill('帮我请假：「' + b.title + '」'); onClose(); }}><Ic n="minus" />请假 / 不去</button>
            <button onClick={() => done('已标记冲突——小禾会绕开这段', core.setStatus(b.id, 'planned'))}><Ic n="sparkle" />标记冲突</button>
          </React.Fragment>
          : confirm
            ? <div className="fl-menu-confirm"><div className="q">{confirm}</div>
              <div className="row"><button className="y" onClick={() => move1(true)}>仍要改</button><button onClick={() => setConfirm(null)}>算了</button></div></div>
            : <React.Fragment>
              <button onClick={() => move1(false)}><Ic n="arrow-right" />挪到明天{b.lockLevel === 'soft' && <span className="fl-lk-dot" title={b.lockReason}></span>}</button>
              <button onClick={() => { window.flPrefill && window.flPrefill('把「' + b.title + '」挪到明天下午3点'); onClose(); }}><Ic n="pencil" />一句话改期…</button>
            </React.Fragment>)}
        <button onClick={() => { onNote(); onClose(); }}><Ic n="file" />注释</button>
        {phase === 'stone'
          ? <button onClick={() => { core.proposeReschedule(b.id); done('重新安排已投为提案——在流里等你点头'); }}><Ic n="refresh" />重新安排</button>
          : <button className="warn" onClick={() => done('移走了：' + b.title, core.removeBlock(b.id))}><Ic n="trash" />移走</button>}
      </div>
    </React.Fragment>;
  }

  // 块条目
  function BlockEntry({ b }) {
    const core = useCore();
    const [menu, setMenu] = useState(null);
    const [noting, setNoting] = useState(false);
    const [note, setNote] = useState(b.note || '');
    const phase = core.phaseOf(b);
    const tone = phase === 'stone' ? 'mute' : b.status === 'done' ? 'ok' : phase === 'now' ? 'acc' : phase === 'recon' && !b.reconciled ? 'warn' : b.date > C.todayIso() ? 'mute' : 'acc';
    const cls = 'fl-en tone-' + tone + ' ' + (phase === 'stone' ? 'stone' : phase === 'future' ? (b.date > C.todayIso() ? 'preview' : 'future') : phase === 'now' ? 'now-blk' : '');
    const end = C.toHM(C.toMin(b.start) + b.dur);
    return <div className={cls} id={'fl-b-' + b.id}>
      <span className="gut">{b.start}</span><span className="dot"></span>
      <div className={'fl-card' + (b.origin === 'auto' ? ' auto' : '') + (b.lockLevel === 'hard' ? ' lk-hard' : b.lockLevel === 'soft' ? ' lk-soft' : '')} title={b.lockReason || undefined} style={{ '--bc': 'var(--dc-' + (b.type === 'appointment' ? 'accent-2' : b.type === 'relax' ? 'ok' : b.type === 'meal' ? 'warn' : 'accent') + ')' }}>
        {b.lockLevel === 'hard' && <span className="fl-pin" aria-hidden="true"></span>}
        <button className="menu dc4-iconbtn" style={{ width: 30, height: 30 }} onClick={(e) => setMenu({ x: e.clientX, y: e.clientY })}><Ic n="dots" size={16} /></button>
        <div className="tt">{b.status === 'done' && <span className="ok"><Ic n="check" size={14} /></span>}<span style={b.status === 'done' ? { color: 'var(--dc-ink-2)' } : null}>{b.title}</span></div>
        <div className="meta">
          {phase === 'now' && <span className="fl-chip-now">正在</span>}
          {phase === 'stone' && <span className="fl-tag">记录</span>}
          {b.status === 'missed' && <span className="fl-tag miss">没做（没关系）</span>}
          <span>{b.start}–{end} · {b.dur} 分钟</span>
          <span>{b.origin === 'auto' ? '小禾排的' : '我放的'}</span>
        </div>
        {b.note && !noting && <div className="note">“{b.note}”</div>}
        {noting && <div style={{ display: 'flex', gap: 6, marginTop: 8 }}>
          <input className="dc4-input" style={{ height: 32, fontSize: 12.5 }} autoFocus value={note} onChange={(e) => setNote(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') { core.setNote(b.id, note); setNoting(false); snack({ label: '注释记下了' }); } if (e.key === 'Escape') setNoting(false); }} placeholder="加一句注释，回车保存" />
        </div>}
        {phase === 'now' && b.status !== 'done' && <div className="fl-doneb">
          <button className="dc4-btn sm" onClick={() => snack({ label: '完成：' + b.title, opId: core.setStatus(b.id, 'done') })}><Ic n="check" size={14} />完成了</button>
        </div>}
        {phase === 'recon' && !b.reconciled && <div className="fl-rechip">
          <button onClick={() => snack({ label: '记完成了', opId: core.reconcile(b.id, 'done', '') })}>完成了</button>
          <button onClick={() => snack({ label: '记没做，没关系', opId: core.reconcile(b.id, 'missed', '') })}>没做</button>
        </div>}
      </div>
      {menu && <EntryMenu b={b} at={menu} onClose={() => setMenu(null)} onNote={() => setNoting(true)} />}
    </div>;
  }

  // 提案插页
  function PropInsert({ p, onFollow }) {
    const core = useCore();
    if (p.state !== 'pending') return null;
    const left = Math.max(0, (p.expiresAt - C.nowMs()) / 3600e3);
    const ttl = (p.ttlRule === 'silent_accept' ? '没理它就当接受' : '没理它就作罢') + ' · 还剩 ' + (left >= 1 ? Math.round(left) + ' 小时' : Math.max(5, Math.round(left * 60)) + ' 分');
    const act = (fn, label) => { const r = fn(); snack({ label, opId: r && r.opId }); };
    const timed = p.kind === 'timed';
    return <div className={'fl-prop' + (timed ? ' ghosty' : '')}>
      <div className="kind"><Ic n="sparkle" size={12} />{timed ? '小禾想放进 ' + (p.start || '') : p.kind === 'decision' ? '小禾的提议 · 逐条点头' : '小禾给你留了张便签'}<span className="ttl">{ttl}</span></div>
      <h4>{p.title}</h4>
      <div className="why">{p.summary}——{p.reason}</div>
      {p.rows && <div className="rows">{p.rows.map((r) => <div key={r.id} className={'row ' + (r.state === 'accepted' ? 'acc' : r.state === 'rejected' ? 'rej' : '')}>
        <span className="lb">{r.label}</span>
        {r.state === 'pending' ? <React.Fragment>
          <button className="rb y" onClick={() => act(() => core.accept(p.id, r.id), '接受：' + r.label)}><Ic n="check" size={13} /></button>
          <button className="rb n" onClick={() => act(() => core.reject(p.id, r.id), '拒绝了一行')}><Ic n="x" size={12} /></button>
        </React.Fragment> : <Ic n={r.state === 'accepted' ? 'check' : 'x'} size={13} style={{ color: r.state === 'accepted' ? 'var(--dc-ok)' : 'var(--dc-ink-3)' }} />}
      </div>)}</div>}
      <div className="ev"><Ic n="eye" size={12} />{p.evidence}</div>
      <div className="acts">
        <button className="dc4-btn sm" onClick={() => act(() => core.accept(p.id), timed ? '放进流里了：' + p.title : '接受：' + p.title)}><Ic n="check" size={14} />{p.rows ? '全部接受' : timed ? '放进来' : '就这么办'}</button>
        <button className="dc4-btn sm sec" onClick={() => act(() => core.reject(p.id), '推开了——它会学乖')}>不了</button>
        <button className="dc4-btn sm ghost" onClick={() => { core.followUp(p.id, '为什么这样安排？'); onFollow && onFollow(); }}>追问</button>
      </div>
    </div>;
  }

  // L0 折叠小字组
  function L0Group({ g }) {
    const core = useCore();
    const [open, setOpen] = useState(false);
    return <div className="fl-l0">
      <button onClick={() => setOpen(!open)}><Ic n={open ? 'chevron-down' : 'chevron-right'} size={13} /><span className="n">{g.rows.length}</span>件小事悄悄做掉了 · 只记一笔，不打扰<span style={{ marginLeft: 'auto', fontVariantNumeric: 'tabular-nums' }}>{C.fmtHM(g.rows[0].at)}–{C.fmtHM(g.rows[g.rows.length - 1].at)}</span></button>
      {open && <div className="rows">{g.rows.map((o) => <div key={o.id} className="fl-line">
        <span className="ic"><Ic n="zap" size={12} /></span>
        <span className="lb">{o.label}<span className="sb">{C.fmtHM(o.at)} · 小禾</span></span>
        {o.undoable && !o.undone && <button className="un" onClick={() => { if (core.undo(o.id)) snack({ label: '已撤销' }); }}>撤销</button>}
      </div>)}</div>}
    </div>;
  }

  // 非块小账行
  const OP_IC = { add_wish: 'star', add_material: 'book', add_memory: 'sparkle', proposal_accept: 'check', proposal_reject: 'x', proposal_expire: 'moon', revert: 'undo', l0: 'zap' };
  function OpLine({ o }) {
    const core = useCore();
    const p = o.payload.type === 'proposal_expire' ? core.state.proposals.find((q) => q.id === o.payload.pid && q.state === 'expired') : null;
    return <div className={'fl-line' + (o.undone ? ' undone' : '')}>
      <span className="ic"><Ic n={OP_IC[o.payload.type] || 'pencil'} size={13} /></span>
      <span className="lb">{o.label}{o.sub && <span className="sb">{o.sub}</span>}</span>
      {o.agent && <span className="fl-tag">小禾</span>}
      {o.undoable && !o.undone && <button className="un" onClick={() => { if (core.undo(o.id)) snack({ label: '已撤销 · ' + o.label }); }}>撤销</button>}
      {p && <button className="un" onClick={() => { core.redeem(p.id); snack({ label: '捞回来了：' + p.title }); }}>捞回</button>}
    </div>;
  }

  function MoodEntry({ m }) {
    return <div className="fl-mood">
      <span className="e">{m.e}</span>
      <div className="bd"><b>{m.l}</b>{m.note && <span className="nt">　{m.note}</span>}</div>
      <span className="src">{m.source === 'agent' ? '小禾代记' : '我'}</span>
    </div>;
  }

  function BriefEntry() {
    const core = useCore();
    if (!core.briefVisible()) return null;
    const b = core.brief();
    return <div className="fl-brief" data-screen-label="晨导语">
      <h4><Ic n="sun" size={15} style={{ color: 'var(--dc-warn)' }} />{b.title}</h4>
      {b.lines.map((l, i) => <p key={i}>{l}</p>)}
      <div className="hint"><span>{b.hint}</span><button className="dc4-btn sm ghost" onClick={() => { core.dismissBrief(); snack({ label: '翻过去了——不反对就是接受' }); }}><Ic n="chevron-up" size={13} />翻过去</button></div>
    </div>;
  }

  // 心情 sheet
  function MoodSheet({ onClose }) {
    const core = useCore();
    const [sel, setSel] = useState(null);
    const [note, setNote] = useState('');
    return <React.Fragment>
      <div className="fl-veil" style={{ background: 'transparent' }} onClick={onClose}></div>
      <div className="fl-moodsheet" data-screen-label="心情打卡">
        <b style={{ fontSize: 13.5 }}>现在感觉怎么样？</b>
        <div className="fl-moodgrid">{C.MOODS.map((o) => <button key={o.l} className={sel && sel.l === o.l ? 'on' : ''} onClick={() => setSel(o)}><span className="e">{o.e}</span>{o.l}</button>)}</div>
        <input className="dc4-input" style={{ marginTop: 10, height: 34, fontSize: 12.5 }} placeholder="想说一句吗（可空）" value={note} onChange={(e) => setNote(e.target.value)} />
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 10 }}>
          <button className="dc4-btn sm ghost" onClick={onClose}>先不了</button>
          <button className="dc4-btn sm" disabled={!sel} onClick={() => { const r = core.moodRecord({ e: sel.e, l: sel.l, note, source: 'user' }); snack({ label: '心情写进流里了：' + sel.e + ' ' + sel.l, opId: r.opId }); onClose(); }}>记下</button>
        </div>
      </div>
    </React.Fragment>;
  }

  function AnswerOverlay({ a, onClose }) {
    if (!a) return null;
    return <div className="fl-over" onClick={onClose}>
      <div className="fl-ocard" onClick={(e) => e.stopPropagation()} data-screen-label="查询浮层">
        <h4><Ic n="search" size={15} style={{ color: 'var(--dc-accent)' }} />{a.title}</h4>
        <ul>{a.lines.map((l, i) => <li key={i}>{l}</li>)}</ul>
        <div className="acts"><button className="dc4-btn sm sec" onClick={onClose}>好（界面即答案）</button></div>
      </div>
    </div>;
  }

  function MoodCheck({ onMore, compact }) {
    const core = useCore();
    const mine = core.moodToday();
    const quick = [0, 1, 3, 5, 6, 8].map((i) => C.MOODS[i]);
    const rec = (o) => { const r = core.moodRecord({ e: o.e, l: o.l, note: '', source: 'user' }); if (r && r.opId) snack({ label: '心情记下了：' + o.e + ' ' + o.l, sub: '写进今天 · 可撤销', opId: r.opId }); };
    return <div className={'fl-moodcheck' + (compact ? ' compact' : '')} data-screen-label="心情打卡">
      <span className="q"><Ic n="smile" size={compact ? 14 : 15} style={{ color: 'var(--dc-accent)' }} />{mine ? '今天记过 ' + mine.e + '，再来一笔？' : '此刻，心情如何？'}</span>
      <div className="faces">
        {quick.map((o) => <button key={o.l} title={o.l} onClick={() => rec(o)}>{o.e}</button>)}
        <button className="more" title="更多心情 / 写一句" onClick={onMore}>{compact ? '···' : '写一句'}</button>
      </div>
    </div>;
  }

  Object.assign(window, { FL: { Ic, useCore, snack, Snack, BlockEntry, PropInsert, L0Group, OpLine, MoodEntry, MoodCheck, BriefEntry, MoodSheet, AnswerOverlay, EntryMenu } });
})();
