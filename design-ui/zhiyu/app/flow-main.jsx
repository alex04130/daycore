// 顺流 — 壳：把账本渲染成一条叙事流（过去↑ · 现在 · 未来↓）
(function () {
  'use strict';
  const { useState, useEffect, useRef } = React;
  const C = window.DcCore, FL = window.FL, FLP = window.FLP, Ic = FL.Ic;
  const dayOf = (ms) => { const d = new Date(ms); return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0'); };

  function compose(core, earlier) {
    const T = C.todayIso(), Y = C.addDays(T, -1), TM = C.addDays(T, 1);
    const items = [];
    const push = (at, kind, data, key) => items.push({ at, kind, data, key });
    if (earlier > 0) (core.state.riverSeed || []).filter((r) => r.date < Y).slice(-earlier).forEach((r) => push(C.dayStartMs(r.date) + 12 * 3600e3, 'river', r, 'r' + r.date));
    [Y, T, TM].forEach((d) => {
      push(C.dayStartMs(d), 'head', d, 'h' + d);
      core.blocksOf(d).forEach((b) => push(C.dayStartMs(d) + C.toMin(b.start) * 60e3, 'block', b, 'b' + b.id));
      core.ghostsOf(d).forEach((g) => push(C.dayStartMs(d) + C.toMin(g.start) * 60e3, 'prop', g, 'p' + g.id));
    });
    if (core.briefVisible()) push(C.dayStartMs(T) + C.toMin(core.state.rhythm.wake || '07:30') * 60e3, 'brief', null, 'brief');
    core.stack().forEach((p) => {
      let at = C.nowMs() + 90e3;
      const r0 = p.rows && p.rows.find((r) => r.date && r.start);
      if (r0 && r0.date >= T) at = Math.max(at, C.dayStartMs(r0.date) + C.toMin(r0.start) * 60e3 - 1);
      push(at, 'prop', p, 'p' + p.id);
    });
    core.state.proposals.filter((p) => p.state === 'expired' && p.expiresAt >= C.dayStartMs(Y)).forEach((p) => push(Math.min(p.expiresAt, C.nowMs() - 1), 'wither', p, 'w' + p.id));
    core.state.moods.filter((m) => m.at >= C.dayStartMs(Y)).forEach((m) => push(m.at, 'mood', m, 'm' + m.id));
    const HIDE = { add_block: 1, move_block: 1, status: 1, reconcile: 1, note: 1, remove_block: 1, add_mood: 1 };
    const silent = [];
    core.state.ops.filter((o) => o.at >= C.dayStartMs(Y)).forEach((o) => { if (o.silent) silent.push(o); else if (!HIDE[o.payload.type]) push(o.at, 'op', o, 'o' + o.id); });
    silent.sort((a, b) => a.at - b.at);
    let g = null, gi = 0;
    silent.forEach((o) => {
      if (!g || o.at - g.rows[g.rows.length - 1].at > 150 * 60e3 || dayOf(o.at) !== dayOf(g.rows[0].at)) { g = { rows: [o] }; push(o.at, 'l0', g, 'l0' + (gi++)); }
      else g.rows.push(o);
    });
    push(C.nowMs(), 'now', null, 'now');
    push(C.nowMs() + 1, 'moodcheck', null, 'moodcheck');
    const W = { head: 0, river: 1, brief: 1, now: 0 };
    items.sort((a, b) => a.at - b.at || (W[a.kind] || 2) - (W[b.kind] || 2));
    return items;
  }

  function App() {
    const core = FL.useCore();
    const [, tick] = React.useReducer((x) => x + 1, 0);
    useEffect(() => { const t = setInterval(tick, 45e3); return () => clearInterval(t); }, []);
    const [earlier, setEarlier] = useState(0);
    const [panel, setPanel] = useState(null);
    const [answer, setAnswer] = useState(null);
    const [moodSheet, setMoodSheet] = useState(false);
    const [back, setBack] = useState(0);
    const [text, setText] = useState('');
    const scRef = useRef(null), inRef = useRef(null), frRef = useRef(null), booted = useRef(false);
    useEffect(() => { window.flApplyTheme(); });
    useEffect(() => { window.flPrefill = (t) => { setText(t); if (inRef.current) inRef.current.focus(); }; return () => { window.flPrefill = null; }; }, []);
    const toNow = (smooth) => {
      const el = document.getElementById('fl-nowline'), sc = scRef.current; if (!el || !sc) return;
      const top = el.getBoundingClientRect().top - sc.getBoundingClientRect().top + sc.scrollTop - sc.clientHeight * .45;
      sc.scrollTo({ top: Math.max(0, top), behavior: smooth ? 'smooth' : 'auto' });
    };
    useEffect(() => { if (!booted.current) { booted.current = true; setTimeout(() => toNow(false), 60); } });
    useEffect(() => { booted.current = false; }, [core.state.scenario]);
    const onScroll = () => {
      const el = document.getElementById('fl-nowline'), sc = scRef.current; if (!el || !sc) return;
      const d = el.getBoundingClientRect().top - sc.getBoundingClientRect().top - sc.clientHeight * .5;
      setBack(Math.abs(d) > 460 ? (d < 0 ? -1 : 1) : 0);
    };
    const submit = () => {
      const t = text.trim(); if (!t) return;
      const r = core.captureIntent(t);
      setText(''); if (inRef.current) inRef.current.focus();
      if (!r) return;
      if (r.kind === 'answer') { setAnswer(r); return; }
      if (r.kind === 'executed') {
        FL.snack({ label: '✓ ' + r.label + '——已写进流里', sub: r.sub, opId: r.opId });
        setTimeout(() => {
          const el = document.getElementById('fl-b-' + r.blockId), sc = scRef.current;
          if (el && sc) sc.scrollTo({ top: el.getBoundingClientRect().top - sc.getBoundingClientRect().top + sc.scrollTop - sc.clientHeight * .4, behavior: 'smooth' });
        }, 120);
        return;
      }
      const viewMap = { wish: 'outlook', material: 'materials', memory: 'materials' };
      FL.snack({ label: '✓ ' + r.label, sub: r.sub, opId: r.opId, onView: viewMap[r.kind] ? () => setPanel(viewMap[r.kind]) : null });
    };
    const onAttach = (e) => {
      const f = e.target.files[0]; if (!f) return; e.target.value = '';
      const r = core.materialAdd({ title: f.name, body: '上传的文件 · ' + Math.round(f.size / 1024) + ' KB', kind: 'life', source: 'upload' }, true);
      FL.snack({ label: '✓ 收好了：' + f.name, sub: '先替你收进资料了', opId: r.opId, onView: () => setPanel('materials') });
    };
    const items = compose(core, earlier);
    const T = C.todayIso();
    const seedLeft = (core.state.riverSeed || []).filter((r) => r.date < C.addDays(T, -1)).length - earlier;
    const scen = core.state.scenario;
    const render = (it) => {
      switch (it.kind) {
        case 'head': {
          const past = it.data < T, future = it.data > T;
          return <div className="fl-day" data-screen-label={'日分隔 ' + C.relDay(it.data)}>
            <span className="lb">{C.relDay(it.data)}<span className="w">{C.fmtDate(it.data, true)}</span></span>
            {past && <span className="rec"><Ic n="lock" size={11} />已经翻篇 · 只作记录</span>}
            {future && <span className="rec"><Ic n="eye" size={11} />预告 · 都还能改</span>}
            <span className="ln"></span>
          </div>;
        }
        case 'river': return <div className="fl-en tone-mute"><span className="gut">{C.fmtDate(it.data.date)}</span><span className="dot"></span>
          <div className="fl-river"><span className="e">{it.data.mood || '·'}</span><span className="bar" style={{ width: 30 + it.data.count * 8 }}></span><span>{it.data.count} 笔 · 都在河里，周信会提起</span></div></div>;
        case 'block': return <FL.BlockEntry b={it.data} />;
        case 'prop': return <div className="fl-en tone-acc"><span className="gut">{it.data.start || C.fmtHM(it.at)}</span><span className="dot"></span><FL.PropInsert p={it.data} onFollow={() => setPanel('companion')} /></div>;
        case 'wither': return <div className="fl-en tone-mute"><span className="gut">{C.fmtHM(it.at)}</span><span className="dot"></span>
          <div className="fl-wither"><Ic n="moon" size={12} />这条提议过了：{it.data.title}<button onClick={() => { core.redeem(it.data.id); FL.snack({ label: '捞回来了：' + it.data.title }); }}>捞回</button></div></div>;
        case 'mood': return <div className="fl-en tone-warn"><span className="gut">{C.fmtHM(it.at)}</span><span className="dot"></span><FL.MoodEntry m={it.data} /></div>;
        case 'op': return <div className="fl-en tone-mute"><span className="gut">{C.fmtHM(it.at)}</span><span className="dot"></span><FL.OpLine o={it.data} /></div>;
        case 'l0': return <div className="fl-en tone-mute"><span className="gut">{C.fmtHM(it.data.rows[0].at)}</span><span className="dot"></span><FL.L0Group g={it.data} /></div>;
        case 'brief': return <div className="fl-en tone-warn"><span className="gut">导语</span><span className="dot"></span><FL.BriefEntry /></div>;
        case 'moodcheck': return <div className="fl-moodwrap"><FL.MoodCheck onMore={() => setMoodSheet(true)} /></div>;
        case 'now': return <div className="fl-now" id="fl-nowline" data-screen-label="现在线"><span className="chip">现在 · {C.toHM(C.nowMin())}</span><span className="ln"></span></div>;
        default: return null;
      }
    };
    return <div className="fl-app" data-screen-label="顺流主界面">
      <div className="dc4-bg"></div>
      <div className="fl-col">
        <div className="fl-scroll" ref={scRef} onScroll={onScroll}>
          <div className="fl-cover">
            <div className="brand">
              <span className="mark"><Ic n="book" size={17} /></span>
              <div><h1>纸屿 · 顺流</h1><div className="sub">一天是一篇你和{core.state.assistantName}一起写的日志</div></div>
              <span className="date">{C.fmtDate(C.todayIso(), true)}</span>
            </div>
          </div>
          {seedLeft > 0 && <div className="fl-more"><button className="dc4-btn sm ghost" onClick={() => setEarlier(earlier + 5)}><Ic n="chevron-up" size={13} />展开更早的 {Math.min(5, seedLeft)} 天</button></div>}
          <div className="fl-stream">
            {items.map((it) => <React.Fragment key={it.key}>{render(it)}</React.Fragment>)}
            <div className="fl-line" style={{ justifyContent: 'center', padding: '18px 0 0', color: 'var(--dc-ink-3)' }}>再往后还没长出来——小禾会在你睡后悄悄把明天排好</div>
          </div>
        </div>
        {back !== 0 && <button className="fl-back" onClick={() => toNow(true)}><Ic n={back < 0 ? 'chevron-down' : 'chevron-up'} size={14} />回到现在</button>}
        <div className="fl-inputwrap">
          <div className="fl-inputbar" data-screen-label="流入口">
            <input ref={inRef} placeholder={scen === 'cold' ? '第一句就有用：「周三下午三点有课」' : '说一句，小禾替你接住——2 秒就够'} value={text} onChange={(e) => setText(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && submit()} />
            <button className="ib" title="附件" onClick={() => frRef.current.click()}><Ic n="link" size={16} /></button>
            <input ref={frRef} type="file" style={{ display: 'none' }} onChange={onAttach} />
            <button className="send" disabled={!text.trim()} onClick={submit}><Ic n="arrow-up" size={16} /></button>
          </div>
          <div className="fl-hint">说一句就记下 · 每件事都能撤销</div>
        </div>
      </div>
      <FLP.Rail onMore={() => setMoodSheet(true)} />
      <nav className="fl-tabs" data-screen-label="侧入口">
        {[['companion', 'chat', '伙伴'], ['materials', 'book', '资料'], ['outlook', 'zap', '展望'], ['settings', 'sliders', '设置']].map(([id, ic, lb]) => (
          <button key={id} className={'fl-tab' + (panel === id ? ' on' : '')} onClick={() => setPanel(panel === id ? null : id)}><Ic n={ic} size={14} />{lb}</button>))}
      </nav>
      {panel === 'materials' && <FLP.Materials onClose={() => setPanel(null)} />}
      {panel === 'outlook' && <FLP.Outlook onClose={() => setPanel(null)} />}
      {panel === 'companion' && <FLP.Companion onClose={() => setPanel(null)} />}
      {panel === 'settings' && <FLP.Settings onClose={() => setPanel(null)} />}
      {moodSheet && <FL.MoodSheet onClose={() => setMoodSheet(false)} />}
      <FL.AnswerOverlay a={answer} onClose={() => setAnswer(null)} />
      <FL.Snack />
    </div>;
  }

  ReactDOM.createRoot(document.getElementById('root')).render(<App />);
})();
