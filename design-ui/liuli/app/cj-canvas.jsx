// 长卷 — 时间画布：日画布（拖拽 + 引力井 + 虚影 + 石化） / 周透镜 / 块浮层
(function () {
  'use strict';
  const { useState, useEffect, useRef } = React;
  const C = window.DcCore, IC = window.DcIcons, CJ = window.CJ;
  const DAY0 = 360, DAY1 = 1440, PXM = 1.05, H = (DAY1 - DAY0) * PXM;
  const yOf = (min) => (min - DAY0) * PXM;
  const snap5 = (m) => Math.round(m / 5) * 5;
  const TYPE_ICON = { task: 'Zap', appointment: 'CalendarDays', relax: 'Leaf', meal: 'Utensils', break: 'Coffee' };

  function laneize(items) {
    const arr = items.map((o) => ({ ...o, s: C.toMin(o.start), e: C.toMin(o.start) + (o.dur || 45) })).sort((a, b) => a.s - b.s || b.e - a.e);
    const lanes = [];
    arr.forEach((it) => { let i = 0; while ((lanes[i] || []).some((o) => o.s < it.e && it.s < o.e)) i++; (lanes[i] = lanes[i] || []).push(it); it.lane = i; });
    return arr;
  }

  // ── 块详情浮层 ──
  function BlockPop({ pop, onClose }) {
    const core = CJ.useCore();
    const [note, setNote] = useState('');
    useEffect(() => { setNote((pop && pop.obj.note) || ''); }, [pop && pop.obj.id]);
    if (!pop) return null;
    const { kind, x, y } = pop;
    const style = { left: Math.max(10, Math.min(x, window.innerWidth - 316)), top: Math.max(10, Math.min(y, window.innerHeight - 360)) };
    const done = (label, opId, extra) => { CJ.cjToast(Object.assign({ label, opId }, extra)); onClose(); };
    if (kind === 'ghost') {
      const p = core.state.proposals.find((q) => q.id === pop.obj.id);
      if (!p || p.state !== 'pending') return null;
      return <div className="cj-pop glass" style={style} data-screen-label="虚影浮层">
        <button className="close" onClick={onClose}><IC.X size={15} /></button>
        <h4>{p.title}</h4>
        <div className="meta"><span className="org auto">小禾想放的 · 等你点头</span>{C.relDay(p.date)} {p.start} · {p.dur} 分钟</div>
        <div className="why" style={{ marginTop: 10 }}>{p.reason}</div>
        <div className="ev" style={{ marginTop: 7, fontSize: 10.5, color: 'var(--ink3)', display: 'flex', gap: 5, alignItems: 'center' }}><IC.Sparkles size={11} />{p.evidence}</div>
        <div className="grp">
          <button className="cj-btn pri" onClick={() => { const r = core.accept(p.id); done('放上了：' + p.title, r && r.opId); }}><IC.Check size={14} />就这么办</button>
          <button className="cj-btn sec" onClick={() => { const r = core.reject(p.id); done('划走了——它会学乖', r && r.opId); }}>不了</button>
          <button className="cj-btn ghost" onClick={() => { core.followUp(p.id, '为什么想排这个？'); window.cjOpenDrawer && window.cjOpenDrawer('companion'); onClose(); }}>追问</button>
        </div>
      </div>;
    }
    const b = core.state.blocks.find((q) => q.id === pop.obj.id);
    if (!b) return null;
    const phase = core.phaseOf(b);
    const end = C.toHM(C.toMin(b.start) + b.dur);
    const saveNote = () => { if (note !== (b.note || '')) { core.setNote(b.id, note); CJ.cjToast({ label: '注释记下了', kind: 'info' }); } };
    const recon = (v, lb) => { const opId = core.reconcile(b.id, v, ''); done('回顾：' + b.title + ' · ' + lb, opId); };
    return <div className="cj-pop glass" style={style} data-screen-label="块浮层">
      <button className="close" onClick={onClose}><IC.X size={15} /></button>
      <h4>{b.title}</h4>
      <div className="meta">
        <span className={'org ' + b.origin}>{b.origin === 'manual' ? '我放的' : '小禾排的'}</span>
        {C.relDay(b.date)} {b.start}–{end}
        {phase === 'stone' && <span className="org" style={{ background: 'color-mix(in srgb,var(--stone) 18%,transparent)', color: 'var(--stone)' }}>记录</span>}
        {b.status === 'done' && <span style={{ color: 'var(--ok)', display: 'inline-flex', gap: 3, alignItems: 'center' }}><IC.Check size={12} />完成</span>}
        {b.status === 'missed' && <span>没做（没关系）</span>}
      </div>
      {(phase === 'future' || phase === 'now') && b.lockLevel === 'hard' && <React.Fragment>
        <div className="stoneline"><span className="cj-pin sm" aria-hidden="true"></span>{b.lockReason}——时间不由你定，但你可以不去。</div>
        <div className="grp">
          <button className="cj-btn pri" onClick={() => done('完成：' + b.title, core.setStatus(b.id, 'done'))}><IC.Check size={14} />完成了</button>
          <button className="cj-btn sec" onClick={() => { core.followUp && core.followUp(null, '帮我请假：' + b.title); done('去跟小禾说请假的事'); }}>请假 / 不去</button>
          <button className="cj-btn ghost" onClick={() => done('已标记冲突——小禾会绕开这段')}>标记冲突</button>
        </div>
      </React.Fragment>}
      {(phase === 'future' || phase === 'now') && b.lockLevel !== 'hard' && <div className="grp">
        <button className="cj-btn pri" onClick={() => done('完成：' + b.title, core.setStatus(b.id, 'done'))}><IC.Check size={14} />完成了</button>
        <button className="cj-btn sec" onClick={() => { const to = { date: C.addDays(b.date, 1), start: b.start }; let r = core.moveBlock(b.id, to); if (r && r.locked === 'soft') { if (!window.confirm(r.reason + '——这是和别人约好的时间，仍要改？')) return; to._confirmed = true; r = core.moveBlock(b.id, to); } done('挪到明天了', (r || {}).opId); }}>放到明天</button>
        <button className="cj-btn sec" onClick={() => { const ns = C.toHM(Math.min(1380, C.toMin(b.start) + 30)); done('往后挪了 30 分钟', (core.moveBlock(b.id, { date: b.date, start: ns }) || {}).opId); }}>+30 分钟</button>
        <button className="cj-btn ghost" onClick={() => done('移走了：' + b.title, core.removeBlock(b.id))}><IC.Trash size={13} />移走</button>
      </div>}
      {b.lockLevel && phase !== 'stone' && <div className="grp">
        <button className="cj-btn sec" onClick={() => { window.cjPrefill && window.cjPrefill('帮我请假：「' + b.title + '」'); core.followUp && core.followUp(null, '帮我请假：' + b.title); done('请假的事交给小禾了'); }}><span className="cj-lock sm" aria-hidden="true"></span>请假 / 不去</button>
        <button className="cj-btn ghost" onClick={() => { core.proposeReschedule(b.id); done('已标记冲突——小禾会绕开这段'); }}>标记冲突</button>
      </div>}
      {phase !== 'stone' && <div className="cj-marks">
        {[['done', '完成了'], ['missed', '没做（没关系）']].map(([m, lb]) => {
          const on = b.status === m;
          return <button key={m} className={'cj-mk' + (on ? ' on' : '')} onClick={() => { const r = core.setMark(b.id, on ? 'none' : m); done(on ? '取消了' : lb, r && r.opId); }}>{lb}</button>;
        })}
      </div>}
      {phase === 'recon' && b.status === 'planned' && <div className="cj-lapse">
        <div className="q"><IC.Sparkles size={12} />这段过去了，还没标——它该去哪儿？</div>
        <div className="grp">
          <button className="cj-btn sec" onClick={() => { core.wishAdd(b.title, b.dur); const opId = core.removeBlock(b.id); done('收进「想做的事」', opId); }}>放回想做的事</button>
          <button className="cj-btn sec" onClick={() => { const r = core.moveBlock(b.id, { date: C.addDays(b.date, 1), start: b.start, _confirmed: true }); done('明天同一时段再来', (r || {}).opId); }}>明天再排</button>
          <button className="cj-btn ghost" onClick={() => { const r = core.setMark(b.id, 'missed'); done('就这样过去——没关系', r && r.opId); }}>就这样过去</button>
        </div>
      </div>}
      {phase === 'recon' && <React.Fragment>
        <div className="stoneline"><IC.Clock size={13} />刚过去，还能改——顺手记一下它怎么了？</div>
        <div className="grp">
          <button className="cj-btn pri" onClick={() => recon('done', '完成了')}>完成了</button>
          <button className="cj-btn sec" onClick={() => recon('missed', '没做（没关系）')}>没做</button>
          <button className="cj-btn ghost" onClick={() => done('移走了：' + b.title, core.removeBlock(b.id))}>移走</button>
        </div>
      </React.Fragment>}
      {phase === 'stone' && <React.Fragment>
        <div className="stoneline"><IC.Anchor size={13} />已经翻篇成记录——过去不改写，想补就用「重新安排」。</div>
        {!b.reconciled && <div className="grp">
          <button className="cj-btn pri" onClick={() => recon('done', '完成了')}>完成了</button>
          <button className="cj-btn sec" onClick={() => recon('missed', '没做（没关系）')}>没做</button>
          <button className="cj-btn sec" onClick={() => recon('did', '其实做了')}>其实做了</button>
        </div>}
        <div className="grp">
          <button className="cj-btn sec" onClick={() => { core.proposeReschedule(b.id); done('重新安排送出了', null, { kind: 'info', sub: '原来那件留着 + 明天同时段放个新的，等你点头' }); }}><IC.Repeat size={13} />重新安排</button>
        </div>
      </React.Fragment>}
      <input className="note" placeholder="加一句注释…" value={note} onChange={(e) => setNote(e.target.value)} onBlur={saveNote} onKeyDown={(e) => e.key === 'Enter' && e.target.blur()} />
    </div>;
  }

  // ── 日画布 ──
  function DayCanvas({ date, highlightId }) {
    const core = CJ.useCore(); CJ.useTick(30e3);
    const wrap = useRef(null);
    const [drag, setDrag] = useState(null);
    const [pop, setPop] = useState(null);
    const dragRef = useRef(null);
    const T = C.todayIso();
    const _all = laneize([...core.blocksOf(date).map((o) => ({ ...o, _g: 0 })), ...core.ghostsOf(date).map((o) => ({ ...o, _g: 1 }))]);
    const blocks = _all.filter((o) => !o._g);
    const ghosts = _all.filter((o) => o._g);
    useEffect(() => {
      if (!wrap.current) return;
      const target = date === T ? yOf(Math.max(DAY0, C.nowMin())) - wrap.current.clientHeight * .38 : yOf(8 * 60) - 60;
      wrap.current.scrollTop = Math.max(0, target + 86);
    }, [date]);
    useEffect(() => { const esc = (e) => e.key === 'Escape' && setPop(null); window.addEventListener('keydown', esc); return () => window.removeEventListener('keydown', esc); }, []);

    const wellsRef = useRef({});
    const hotWell = (x, y) => {
      for (const k of Object.keys(wellsRef.current)) {
        const el = wellsRef.current[k]; if (!el) continue;
        const r = el.getBoundingClientRect();
        const pad = window.innerWidth < 700 ? -10 : 18;
        if (x >= r.left - pad && x <= r.right + pad && y >= r.top - pad && y <= r.bottom + pad) return k;
      }
      return null;
    };
    const onDown = (b) => (e) => {
      if (e.button !== undefined && e.button !== 0) return;
      const phase = core.phaseOf(b);
      e.currentTarget.setPointerCapture(e.pointerId);
      dragRef.current = { id: b.id, y0: e.clientY, x0: e.clientX, moved: false, stone: phase === 'stone', hard: b.lockLevel === 'hard' };
      const lpAt = { x: e.clientX, y: e.clientY };
      dragRef.current.lp = setTimeout(() => {
        const d = dragRef.current; if (!d || d.moved) return;
        d.opened = true;
        if (navigator.vibrate) navigator.vibrate(12);
        setDrag(null); setPop({ kind: 'block', obj: b, x: lpAt.x + 14, y: lpAt.y - 40 });
      }, 460);
      const move = (ev) => {
        const d = dragRef.current; if (!d) return;
        const dy = ev.clientY - d.y0;
        if (!d.moved && Math.abs(dy) < 5 && Math.abs(ev.clientX - d.x0) < 5) return;
        if (d.lp) { clearTimeout(d.lp); d.lp = null; }
        if (d.opened) return;
        if (d.hard) {
          if (!d.warned && Math.abs(dy) > 10) {
            d.warned = true;
            const el = document.getElementById('cj-blk-' + d.id) || null;
            if (el) { el.classList.remove('tug'); void el.offsetWidth; el.classList.add('tug'); setTimeout(() => el.classList.remove('tug'), 460); }
            CJ.cjToast({ label: (b.lockReason || '这一项的时间不由你定') + '——改不了时间', sub: '想不去？点开它选「请假 / 标记冲突」', kind: 'stone' });
          }
          setDrag({ id: d.id, dy: Math.max(-7, Math.min(7, dy)), hot: null });
          return;
        }
        if (d.stone) { if (!d.warned && Math.abs(dy) > 14) { d.warned = true; CJ.cjToast({ label: '过去的记录不改写', sub: '想补的话，点开用「重新安排」放个新的', kind: 'stone' }); } return; }
        d.moved = true;
        setDrag({ id: d.id, dy, hot: hotWell(ev.clientX, ev.clientY) });
      };
      const up = (ev) => {
        window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up);
        const d = dragRef.current; dragRef.current = null;
        if (!d) return;
        if (d.lp) clearTimeout(d.lp);
        if (d.hard) { setDrag(null); return; }
        if (d.opened) { setDrag(null); return; }
        if (!d.moved) { setDrag(null); const r = e.currentTarget.getBoundingClientRect ? null : null; setPop({ kind: 'block', obj: b, x: ev.clientX + 14, y: ev.clientY - 40 }); return; }
        const hot = hotWell(ev.clientX, ev.clientY);
        setDrag(null);
        if (hot === 'rearrange') { core.proposeReschedule(b.id); CJ.cjToast({ label: '交给小禾重排了', sub: '原来那件留着——它会在流里投个新时间问你' }); return; }
        if (hot === 'archive') { core.materialAdd({ title: b.title, body: '从画布收起 · 原定 ' + C.relDay(b.date) + ' ' + b.start, kind: 'life' }); const opId = core.removeBlock(b.id); CJ.cjToast({ label: '收进资料库，画布上移走了', sub: '想反悔就撤销', opId }); return; }
        if (hot === 'park') { core.wishAdd(b.title, b.dur); const opId = core.removeBlock(b.id); CJ.cjToast({ label: '收进「想做的事」', sub: '哪天有空档我会提上来问你', opId }); return; }
        if (hot === 'next') {
          const nd = C.addDays(b.date, 1);
          const r = core.moveBlock(b.id, { date: nd, start: b.start });
          if (r && r.stone) CJ.cjToast({ label: '过去的记录不改写', sub: '想补的话，用「重新安排」放个新的', kind: 'stone' });
          else CJ.cjToast({ label: '交给' + C.relDay(nd) + '了', opId: r && r.opId });
          return;
        }
        const dy = ev.clientY - d.y0;
        const ns = Math.max(DAY0, Math.min(DAY1 - b.dur, snap5(C.toMin(b.start) + dy / PXM)));
        if (ns !== C.toMin(b.start)) {
          const r = core.moveBlock(b.id, { date: b.date, start: C.toHM(ns) });
          if (r && r.stone) CJ.cjToast({ label: '过去的记录不改写', kind: 'stone' });
          else if (r && r.opId) CJ.cjToast({ label: '挪到 ' + C.toHM(ns) + ' 了', opId: r.opId });
        }
      };
      window.addEventListener('pointermove', move); window.addEventListener('pointerup', up);
    };

    const hours = []; for (let h = 6; h <= 23; h++) hours.push(h);
    const nowY = yOf(Math.max(DAY0, Math.min(DAY1, C.nowMin())));
    const dragging = !!drag;
    return <React.Fragment>
      <div className="cj-scroll" ref={wrap}>
        <div className="cj-day" data-screen-label={'日画布 ' + C.relDay(date)}>
          <div className="cj-col" style={{ height: H }}>
            {hours.map((h) => <div key={h} className="cj-hour" style={{ top: yOf(h * 60) }}><span className="hl">{String(h).padStart(2, '0')}:00</span><span className="ln" /></div>)}
            {date === T && <div className="cj-now" style={{ top: nowY }}><span className="chip">{C.toHM(C.nowMin())}</span><span className="ln" /></div>}
            {!blocks.length && !ghosts.length && <div className="cj-empty" style={{ top: date === T ? Math.max(80, nowY - 40) : yOf(10 * 60) }}>
              <b>{date === T ? '今天还是空的' : C.relDay(date) + ' 还没有安排'}</b>
              <span>{date === T ? '不是坏事——说明还没人替你着急。在下面说一句「明天上午十点开会」试试，或者什么都不说也很好。' : '想放点什么，跟下面的输入框说一句就行。'}</span>
            </div>}
            {blocks.map((b) => {
              const phase = core.phaseOf(b);
              const isDrag = drag && drag.id === b.id;
              const h = Math.max(30, b.dur * PXM - 2);
              const compact = h < 48;
              return <div key={b.id} id={'cj-blk-' + b.id}
                title={b.start + '–' + C.toHM(b.s + b.dur) + ' ' + b.title + (b.status === 'missed' ? ' · 没做（没关系）' : b.status === 'done' ? ' · 完成' : '')}
                className={'cj-blk ' + b.origin + ' ' + phase + (compact ? ' compact' : '') + (b.status === 'done' ? ' done' : '') + (isDrag ? ' dragging' : '') + (b.lane > 0 ? ' stacked' : '') + (highlightId === b.id ? ' hl' : '') + (b.lockLevel === 'hard' ? ' lk-hard' : b.lockLevel === 'soft' ? ' lk-soft' : '')}
                style={{ top: yOf(b.s) + (isDrag ? drag.dy : 0), height: h, left: 46 + b.lane * 18, right: 6, '--lane': b.lane, '--bc': 'var(--c-' + b.type + ')' }}
                onPointerDown={onDown(b)}
                onContextMenu={(e) => { e.preventDefault(); e.stopPropagation(); dragRef.current = null; setDrag(null); setPop({ kind: 'block', obj: b, x: e.clientX + 14, y: e.clientY - 40 }); }}>
                {b.lockLevel && <span className={'cj-lock ' + b.lockLevel} title={b.lockReason || ''} aria-hidden="true"></span>}
                {!compact && <div className="tm">{b.start}–{C.toHM(b.s + b.dur)}{b.note ? <IC.Pencil size={10} /> : null}</div>}
                <div className="tt">{b.title}</div>
                {phase === 'stone' && <span className="rec">记录</span>}
                {b.status === 'done' && phase === 'stone' && <span className="st"><IC.CheckCircle size={compact ? 12 : 14} /></span>}
                {b.status === 'missed' && phase === 'stone' && <span className="st miss">没做</span>}
                {phase !== 'stone' && <button className={'cj-tick ' + (b.status === 'done' ? 'on' : b.status === 'missed' ? 'off' : '')}
                  title={b.status === 'done' ? '已完成 · 点一下取消' : b.status === 'missed' ? '记的是没做 · 点一下改成完成' : '点一下记完成'}
                  onPointerDown={(e) => { e.stopPropagation(); e.preventDefault(); }}
                  onClick={(e) => { e.stopPropagation(); const next = b.status === 'done' ? 'none' : 'done'; const r = core.setMark(b.id, next); CJ.cjToast({ label: next === 'done' ? '记完成了' : '取消了', sub: '随时能再改', opId: r && r.opId }); }}>
                  {b.status === 'done' ? <IC.Check size={13} /> : b.status === 'missed' ? <span className="dash"></span> : null}</button>}
                {!compact && h >= 52 && <div className="cj-tags" onPointerDown={(e) => e.stopPropagation()}>
                  {[b.origin === 'auto' ? '小禾排的' : '我放的',
                    b.lockLevel === 'hard' ? '课表固定' : b.lockLevel === 'soft' ? '约好的' : null,
                    phase === 'recon' && !b.reconciled ? '待回顾' : b.note ? '有注释' : null].filter(Boolean).slice(0, 3)
                    .map((t) => <span key={t} className="cj-tg">{t}</span>)}
                </div>}
                {false && <div className="cj-reconrow">
                </div>}
              </div>;
            })}
            {ghosts.map((g) => <div key={g.id} className="cj-ghost" style={{ top: yOf(g.s), height: Math.max(30, g.dur * PXM - 2), left: 46 + g.lane * 18, right: 6, zIndex: 14 + g.lane, '--bc': 'var(--c-' + (g.btype || 'task') + ')' }}
              onClick={(e) => setPop({ kind: 'ghost', obj: g, x: e.clientX + 14, y: e.clientY - 40 })}>
              <span className="q">想放这儿?</span>
              <div className="tm">{g.start} · {g.dur} 分钟</div>
              <div className="tt">{g.title}</div>
            </div>)}
          </div>
        </div>
      </div>
      <div className={'cj-wells' + (dragging ? ' live' : '')}>
        <div ref={(el) => wellsRef.current.rearrange = el} className={'cj-well br' + (drag && drag.hot === 'rearrange' ? ' hot' : '')}><span className="halo"><IC.Sparkles size={17} /></span>小禾<br />重排</div>
        <div ref={(el) => wellsRef.current.archive = el} className={'cj-well tl' + (drag && drag.hot === 'archive' ? ' hot' : '')}><span className="halo"><IC.BookOpen size={17} /></span>资料<br />收好</div>
        <div ref={(el) => wellsRef.current.park = el} className={'cj-well bl' + (drag && drag.hot === 'park' ? ' hot' : '')}><span className="halo"><IC.Anchor size={17} /></span>想做的事<br />改天再说</div>
        <div ref={(el) => wellsRef.current.next = el} className={'cj-well tr' + (drag && drag.hot === 'next' ? ' hot' : '')}><span className="halo"><IC.ChevronRight size={17} /></span>交给{C.relDay(C.addDays(date, 1))}</div>
      </div>
      <BlockPop pop={pop} onClose={() => setPop(null)} />
    </React.Fragment>;
  }

  // ── 周透镜（捏合到周为止） ──
  function WeekLens({ anchor, onPick }) {
    const core = CJ.useCore();
    const T = C.todayIso();
    const [y, m, d] = anchor.split('-').map(Number);
    const dow = (new Date(y, m - 1, d).getDay() + 6) % 7; // 周一起
    const days = Array.from({ length: 7 }, (_, i) => C.addDays(anchor, i - dow));
    const pct = (min) => ((min - DAY0) / (DAY1 - DAY0) * 100) + '%';
    return <div className="cj-scroll"><div className="cj-week" data-screen-label="周透镜">
      {days.map((ds) => {
        const bs = core.blocksOf(ds), gs = core.ghostsOf(ds);
        return <div key={ds} className={'cj-wcol glass' + (ds === T ? ' today' : '')} onClick={() => onPick(ds)}>
          <h5>{C.weekdayShort(ds)}</h5>
          <div className="n">{Number(ds.split('-')[2])}</div>
          <div className="cj-wbody">
            {bs.map((b) => { const s = C.toMin(b.start); const ph = core.phaseOf(b); return <div key={b.id} title={b.start + ' ' + b.title} className={'cj-wblk ' + (ph === 'stone' ? 'stonew' : b.origin)} style={{ top: pct(Math.max(DAY0, s)), height: 'max(5px, calc(' + (b.dur / (DAY1 - DAY0) * 100) + '% - 2px))', '--bc': 'var(--c-' + b.type + ')' }} />; })}
            {gs.map((g) => <div key={g.id} title={'想放：' + g.title} className="cj-wblk ghostw" style={{ top: pct(C.toMin(g.start)), height: Math.max(6, g.dur / (DAY1 - DAY0) * 100) + '%', '--bc': 'var(--c-' + (g.btype || 'task') + ')' }} />)}
          </div>
        </div>;
      })}
    </div></div>;
  }

  Object.assign(window, { CJCanvas: { DayCanvas, WeekLens } });
})();
