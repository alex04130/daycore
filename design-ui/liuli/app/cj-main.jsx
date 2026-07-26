// 长卷 — 壳：五席书签 = 内容区 tab（镜头移动）；底部输入恒定，语境注入 agent
(function () {
  'use strict';
  const { useState, useEffect, useRef } = React;
  const C = window.DcCore, IC = window.DcIcons, CJ = window.CJ, CV = window.CJCanvas, DR = window.CJDrawers, ST = window.CJSettings;

  const PLACEHOLDER = {
    today: '跟我说句话，随手记点什么——2 秒就够',
    materials: '在这页说的，会顺手收进资料',
    outlook: '许个愿、问问截止都行',
    trace: '问问过去，或者直接说',
    companion: '跟小禾聊，这里会留着聊天记录',
    settings: '设置页也能说话——换主题用下面的工作室',
  };
  const CTX = { today: '今天', materials: '资料', outlook: '展望', trace: '足迹', companion: '伙伴', settings: '设置' };

  function App() {
    const core = CJ.useCore(); CJ.useTick(60e3);
    const [date, setDate] = useState(C.todayIso());
    const [mode, setMode] = useState('day');
    const [view, setView] = useState('today');
    const [menu, setMenu] = useState(false);
    const [answer, setAnswer] = useState(null);
    const [hl, setHl] = useState(null);
    const [text, setText] = useState('');
    const inputRef = useRef(null), fileRef = useRef(null);
    const theme = core.theme('liuli') || 'sky';
    useEffect(() => { window.cjApplyTheme(); });
    useEffect(() => { window.cjOpenDrawer = (d) => setView(d); return () => { window.cjOpenDrawer = null; }; }, []);
    useEffect(() => {
      const key = (e) => {
        if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') return;
        if (view === 'today') { if (e.key === 'ArrowLeft') setDate((d) => C.addDays(d, -1)); if (e.key === 'ArrowRight') setDate((d) => C.addDays(d, 1)); }
        if (e.key === 'Escape') { setMenu(false); setAnswer(null); }
      };
      window.addEventListener('keydown', key); return () => window.removeEventListener('keydown', key);
    }, [view]);

    const T = C.todayIso();
    const goHome = () => { setMenu(false); setView('today'); setMode('day'); setDate(T); };
    const goDay = (d) => { setView('today'); setMode('day'); setDate(d); };
    const jumpView = (v) => { if (v === 'today') goHome(); else if (CTX[v]) setView(v); };
    const submit = () => {
      const t = text.trim(); if (!t) return;
      setText(''); if (inputRef.current) inputRef.current.focus();
      if (view === 'companion') { core.chatSend(t); return; }
      const r = core.captureIntent(t, { focus: view === 'materials' ? 'archive' : view });
      if (!r) return;
      if (r.kind === 'answer') { setAnswer(r); return; }
      if (r.kind === 'executed') {
        goDay(r.date || T);
        setHl(r.blockId); setTimeout(() => setHl(null), 1800);
        CJ.cjToast({ label: r.label, sub: r.sub, opId: r.opId });
        return;
      }
      const viewMap = { wish: 'outlook', material: 'materials', memory: 'materials', mood: null };
      CJ.cjToast({ label: r.label, sub: r.sub, opId: r.opId, onView: viewMap[r.kind] && view !== viewMap[r.kind] ? () => setView(viewMap[r.kind]) : null });
    };
    const onAttach = (e) => {
      const f = e.target.files[0]; if (!f) return; e.target.value = '';
      const r = core.materialAdd({ title: f.name, body: '上传的文件 · ' + Math.round(f.size / 1024) + ' KB', kind: 'life', source: 'upload' }, true);
      if (view === 'companion') {
        core.state.chat.push({ role: 'user', content: '［附件］' + f.name, at: C.nowMs() });
        core.state.chat.push({ role: 'assistant', content: '收到，先替你收进资料库了。要我从里面读出课表、截止或重点，说一声就行。', at: C.nowMs() + 1, tools: [{ label: '收进资料 · 可撤销', opId: r.opId }] });
        core.emit();
      } else CJ.cjToast({ label: '收进资料库了：' + f.name, sub: '先替你收好了', opId: r.opId, onView: view !== 'materials' ? () => setView('materials') : null });
    };

    const seats = [
      { id: 'materials', label: '资料', icon: <IC.BookOpen size={20} /> },
      { id: 'trace', label: '足迹', icon: <IC.Anchor size={20} />, badge: core.l0Today() || null, quiet: true },
      { id: 'today', label: '今天', icon: <IC.Sun size={22} /> },
      { id: 'outlook', label: '展望', icon: <IC.Zap size={20} /> },
      { id: 'companion', label: '伙伴', icon: <IC.MessageHeart size={20} /> },
    ];
    const offToday = mode !== 'day' || date !== T;
    const nb = view === 'today' && mode === 'day' ? core.blocksOf(date).length : 0;
    const scen = core.state.scenario;
    return <div className="cj-app" data-screen-label="长卷">
      <div className="cj-orbs"></div><div className="cj-grain"></div>
      <div className="cj-stage">
        {view === 'today' && <React.Fragment key={'t' + date + scen}>
          {mode === 'day'
            ? <CV.DayCanvas key={date} date={date} highlightId={hl} />
            : <CV.WeekLens anchor={date} onPick={goDay} />}
          {mode === 'day' && null}
        </React.Fragment>}
        {view === 'materials' && <DR.Materials key={'m' + scen} />}
        {view === 'outlook' && <DR.Outlook key={'o' + scen} onGoDay={goDay} />}
        {view === 'trace' && <DR.Trace key={'r' + scen} />}
        {view === 'companion' && <DR.Companion key={'c' + scen} />}
        {view === 'settings' && <ST.SettingsPage key={'st' + scen} />}
      </div>
      <header className="cj-top">
        <CJ.PushChip />
        {view === 'today' && <CJ.Stack onFollow={() => setView('companion')} onGlance={() => { }} />}
        <span className="cj-sp"></span>
        {view === 'today' ? <div className="cj-nav">
          <button className="cj-navbtn" onClick={() => setDate(C.addDays(date, mode === 'week' ? -7 : -1))}><IC.ChevronLeft size={17} /></button>
          <div className={'cj-title' + (mode === 'week' ? ' wk' : '')}><span className="d">{mode === 'week' ? '这一周' : C.relDay(date)}</span><span className="w">{mode === 'week' ? '最多看到一周' : C.fmtDate(date, true)}{mode !== 'week' && <span className="cnt">{nb ? ' · ' + nb + ' 件安排' : ' · 空白一天'}</span>}</span></div>
          <button className="cj-navbtn" onClick={() => setDate(C.addDays(date, mode === 'week' ? 7 : 1))}><IC.ChevronRight size={17} /></button>
          {offToday && <button className="cj-pill glass on" onClick={goHome}><IC.Sun size={14} />回到今天</button>}
        </div> : <div className="cj-title"><span className="d">{CTX[view]}</span><span className="w">{C.relDay(T)} · {C.fmtDate(T, true)}</span></div>}
        <span className="cj-sp"></span>
        {view === 'today' && mode === 'day' && <CJ.MoodCapsule />}
        <span className="cj-clock glass" title="现在时刻">{C.toHM(C.nowMin())}</span>
        {view === 'today' && <button className={'cj-pill glass' + (mode === 'week' ? ' on' : '')} onClick={() => setMode(mode === 'week' ? 'day' : 'week')}><IC.Layers size={14} />{mode === 'week' ? '回到日' : '周'}</button>}
        <button className="cj-avatar" onClick={() => setMenu(!menu)}>{core.state.userName[0]}</button>
      </header>
      {menu && <React.Fragment>
        <div className="cj-veil" style={{ background: 'transparent', backdropFilter: 'none' }} onClick={() => setMenu(false)}></div>
        <div className="cj-menu glass" data-screen-label="头像菜单">
          <div className="who"><span className="cj-avatar">{core.state.userName[0]}</span><div><b>{core.state.userName}</b><span>琉璃 · 长卷 — 时间是一块画布</span></div></div>
          <div className="lab">主题</div>
          <div className="cj-mini-themes" style={{ flexWrap: 'wrap' }}>
            {ST.BUILTIN.map((b) => <button key={b.id} title={b.name} className={theme === b.id ? 'on' : ''} style={{ background: 'linear-gradient(145deg,' + b.sw[1] + ',' + b.sw[2] + ')', '--sw': b.sw[0] }} onClick={() => { core.setTheme('liuli', b.id); window.cjApplyTheme(); }}></button>)}
            {core.state.customThemes.map((th) => { const v = th.variables || {}; return <button key={th.id} title={th.name + '（AI 生成）'} className={theme === th.id ? 'on' : ''} style={{ background: 'linear-gradient(145deg,' + (v['--bg-start'] || '#eee') + ',' + (v['--bg-end'] || '#ddd') + ')', '--sw': v['--primary'] || '#888' }} onClick={() => { core.setTheme('liuli', th.id); window.cjApplyTheme(); }}></button>; })}
          </div>
          <div className="lab">演示场景</div>
          <div className="cj-seg">{[['healthy', '健康日'], ['crisis', '危机日'], ['cold', '冷启动']].map(([v, l]) => <button key={v} className={scen === v ? 'on' : ''} onClick={() => { core.scenarioSet(v); setMenu(false); goHome(); }}>{l}</button>)}</div>
          <button className="mrow" style={{ marginTop: 10 }} onClick={() => { setMenu(false); setView('settings'); }}><span className="ic"><IC.Settings size={15} /></span>设置 · 主题工作室与偏好</button>
          <a className="hub" href="../index.html"><IC.ExternalLink size={13} />回三前端门厅 · Daycore Hub</a>
        </div>
      </React.Fragment>}
      <CJ.AnswerSheet a={answer} onClose={() => setAnswer(null)} onView={jumpView} />
      <CJ.Toasts />
      <footer className="cj-bottom">
        <div className="cj-inputbar glass" data-screen-label="恒定输入">
          <span className="ctx" title="在这一页说话，小禾知道你在看什么">{CTX[view]}</span>
          <input ref={inputRef} placeholder={scen === 'cold' && view === 'today' ? '第一句话就有用：「周三下午三点有课」' : (view === 'companion' ? '跟' + core.state.assistantName + '聊，这里留着聊天记录' : PLACEHOLDER[view])} value={text}
            onChange={(e) => setText(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && submit()} />
          <button className="mic" title="附件" onClick={() => fileRef.current.click()}><IC.Link size={15} /></button>
          <button className="mic" title="语音（演示）"><IC.Mic size={16} /></button>
          <button className="send" disabled={!text.trim()} onClick={submit}><IC.ArrowUp size={16} /></button>
          <input ref={fileRef} type="file" style={{ display: 'none' }} onChange={onAttach} />
        </div>
        <nav className="cj-dock glass" data-screen-label="五席书签">
          <div className="cj-rail-logo"><span className="dot"></span>琉璃 · 长卷</div>
          {seats.map((s) => s.id === 'today'
            ? <button key={s.id} className={'cj-seat home' + (view === 'today' && !offToday ? '' : ' off')} onClick={goHome}>{s.icon}{s.label}</button>
            : <button key={s.id} className={'cj-seat' + (view === s.id ? ' on' : '')} onClick={() => setView(view === s.id ? 'today' : s.id)}>
              {s.badge ? <span className={'bdg' + (s.quiet ? ' q' : '')}>{s.badge}</span> : null}{s.icon}{s.label}
            </button>)}
          <button className="cj-rail-user" onClick={() => setMenu(!menu)}>
            <span className="cj-avatar" style={{ width: 32, height: 32, fontSize: 12, flex: 'none' }}>{core.state.userName[0]}</span>
            <span className="meta"><span className="name">{core.state.userName}</span><span className="sub">{scen === 'healthy' ? '健康日' : scen === 'crisis' ? '危机日' : '冷启动'} · 设置与主题</span></span>
            <IC.Settings size={15} style={{ color: 'var(--ink3)', flex: 'none' }} />
          </button>
        </nav>
      </footer>
    </div>;
  }

  ReactDOM.createRoot(document.getElementById('root')).render(<App />);
})();
