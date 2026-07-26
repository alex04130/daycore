// 汀 — 壳：单件流状态机（导语 → 提案 → 心情帧 → 此刻/空档/收尾）
(function () {
  'use strict';
  const { useState, useEffect, useRef } = React;
  const C = window.DcCore, TG = window.TG, Ic = TG.Ic;

  function useSwipeUp(onFire) {
    const [dy, setDy] = useState(0);
    const down = (e) => {
      if (e.target.closest('button') || e.target.closest('input')) return;
      const y0 = e.clientY;
      const mv = (ev) => setDy(Math.min(0, ev.clientY - y0));
      const up = (ev) => {
        window.removeEventListener('pointermove', mv); window.removeEventListener('pointerup', up);
        const d = ev.clientY - y0; setDy(0);
        if (d < -90) onFire();
      };
      window.addEventListener('pointermove', mv); window.addEventListener('pointerup', up);
    };
    return [dy, down];
  }
  const swipeStyle = (dy) => ({ transform: 'translateY(' + dy + 'px)', opacity: 1 + dy / 260, transition: dy ? 'none' : 'transform .25s cubic-bezier(.22,.9,.24,1), opacity .25s' });

  function App() {
    const core = TG.useCore(); TG.useTick(30e3);
    const [sheet, setSheet] = useState(null);
    const [peek, setPeek] = useState(false);
    const [mood, setMood] = useState(false);
    const skipped = useRef(new Set());
    const [, force] = React.useReducer((x) => x + 1, 0);
    const theme = core.theme('ting') || 'night';
    useEffect(() => { document.documentElement.setAttribute('data-tg', theme); }, [theme]);
    useEffect(() => { window.tgOpenCompanionHint = () => setSheet('why'); return () => { window.tgOpenCompanionHint = null; }; }, []);

    const T = C.todayIso();
    const flow = core.flowNow();
    const queue = [...core.stack(), ...core.ghostsOf(T)].filter((p) => p.state === 'pending' && !skipped.current.has(p.id));
    const brief = core.briefVisible();
    const complete = (b) => {
      const opId = core.setStatus(b.id, 'done');
      TG.tgUndo({ label: '完成：' + b.title + '——送走了', opId });
      if (!core.anyMoodToday()) setMood(true);
    };
    const [dy, downCur] = useSwipeUp(() => { if (flow.current) complete(flow.current); });
    const [bdy, downBrief] = useSwipeUp(() => core.dismissBrief());

    // ── faces ──
    let face = null, faceKey = '';
    if (brief) {
      const b = core.brief();
      faceKey = 'brief';
      face = <div className="tg-card" onPointerDown={downBrief} style={swipeStyle(bdy)} data-screen-label="导语面">
        <div className="tg-eyebrow"><Ic n="sun" size={13} />今天，先看几句<span className="ln"></span></div>
        <div className="tg-title sm">{b.title}</div>
        <div className="tg-sub tg-brieflines">{b.lines.map((l, i) => <p key={i}>{l}</p>)}</div>
        <div className="tg-actrow">
          <button className="tg-btn sec" onClick={() => core.dismissBrief()}><Ic n="chevron-up" size={14} />上滑即走</button>
        </div>
        <div className="tg-metarow">{b.hint}</div>
      </div>;
    } else if (queue.length) {
      faceKey = 'prop-' + queue[0].id;
      face = <TG.PropCard p={queue[0]} queueN={queue.length} onSkipAll={() => { queue.forEach((p) => skipped.current.add(p.id)); force(); }} />;
    } else if (flow.current) {
      const b = flow.current;
      const s = C.toMin(b.start), pct = Math.max(2, Math.min(100, Math.round((C.nowMin() - s) / b.dur * 100)));
      faceKey = 'cur-' + b.id;
      face = <div className="tg-card" onPointerDown={downCur} style={swipeStyle(dy)} data-screen-label="此刻卡">
        <div className="tg-eyebrow"><Ic n="clock" size={13} />此刻 · {b.start}–{C.toHM(s + b.dur)}<span className="ln"></span><span className="tg-qbadge">{pct}%</span></div>
        <div className="tg-title">{b.title}</div>
        {b.note && <div className="tg-note">“{b.note}”</div>}
        <div className="tg-metarow">
          <span className={'tg-origin ' + b.origin}>{b.origin === 'manual' ? '我放的' : '小禾排的'}</span>
          <span>还剩 {Math.max(0, s + b.dur - C.nowMin())} 分钟</span>
        </div>
        <div className="tg-prog"><i style={{ width: pct + '%' }}></i></div>
        <div className="tg-actrow">
          <button className="tg-btn pri" onClick={() => complete(b)}><Ic n="check" size={15} />完成了</button>
          <button className="tg-btn sec" onClick={() => setSheet('act')}>没做 / 换一个…</button>
        </div>
      </div>;
    } else if (flow.next && !flow.nextTomorrow) {
      faceKey = 'gap';
      face = <div className="tg-card" data-screen-label="空档卡">
        <div className="tg-eyebrow mute"><Ic n="moon" size={13} />空档<span className="ln"></span></div>
        <div className="tg-title md">现在空着，<br />空着不是欠着。</div>
        <div className="tg-sub">{flow.gapMin} 分钟后才有下一件。想干嘛干嘛——要是想让我填点什么，说一句就行。</div>
        <div className="tg-actrow">
          <button className="tg-btn pri" onClick={() => setSheet('capture')}><Ic n="chat" size={15} />说一句</button>
          {skipped.current.size > 0 && <button className="tg-btn sec" onClick={() => { skipped.current.clear(); force(); }}>再看看提案</button>}
        </div>
      </div>;
    } else if (flow.next && flow.nextTomorrow) {
      faceKey = 'done';
      face = <div className="tg-card" data-screen-label="收尾卡">
        <div className="tg-eyebrow mute"><Ic n="moon" size={13} />今天到这儿<span className="ln"></span></div>
        <div className="tg-title md">没有下一件了。</div>
        <div className="tg-sub">没做完的不积债——要紧的明天会自己浮上来。睡前想到什么，丢给我就好。</div>
        <div className="tg-actrow"><button className="tg-btn pri" onClick={() => setSheet('capture')}><Ic n="chat" size={15} />说一句</button></div>
      </div>;
    } else {
      faceKey = 'empty';
      face = <div className="tg-card" data-screen-label="空白之汀">
        <div className="tg-eyebrow mute"><Ic n="moon" size={13} />此刻<span className="ln"></span></div>
        <div className="tg-title md">现在没有安排。<br />这不是欠着。</div>
        <div className="tg-sub">说一句「明天上午十点开会」或者「想学吉他」，我来接住剩下的。什么都不说，今天就这样也很好。</div>
        <div className="tg-actrow"><button className="tg-btn pri" onClick={() => setSheet('capture')}><Ic n="chat" size={15} />说一句</button></div>
      </div>;
    }

    const showSwipeHint = faceKey.startsWith('cur') || faceKey === 'brief';
    const ex = core.state.exchange;
    return <div className="tg-app">
      <div className="tg-frame" data-screen-label="汀 · 单件流">
        <TG.UndoBar />
        <header className="tg-top">
          <span className="tg-clock">{C.toHM(C.nowMin())}</span>
          <button className="tg-l0" onClick={() => setSheet('ledger')} title="点开看看账本">悄悄做了 {core.l0Today()} 件</button>
          <button className="tg-dots" onClick={() => setMood(true)} title="记一下心情"><Ic n="smile" size={17} /></button>
          <button className="tg-dots" onClick={() => setSheet('menu')}><Ic n="dots" size={18} /></button>
        </header>
        <div className="tg-handle" onClick={() => setPeek(true)}><span className="bar"></span>下拉看全天</div>
        <TG.PushStrip />
        <main className="tg-main">
          {face}
          {showSwipeHint && <div className="tg-swipehint"><span className="up"><Ic n="chevron-up" size={15} /></span>{faceKey === 'brief' ? '上滑即走' : '上滑 = 完成，送走它'}</div>}
          {showSwipeHint && <div className="tg-keyhint"><span><kbd>Enter</kbd>{faceKey === 'brief' ? '知道了' : '完成 / 就这么办'}</span><span><kbd>Esc</kbd>推开</span><span><kbd>↓</kbd>看全天</span></div>}
        </main>
        {flow.next && faceKey !== 'empty' && <button className="tg-next" onClick={() => setPeek(true)} data-screen-label="下一件 peek">
          <span className="k">{flow.nextTomorrow ? '明天' : '下一件'}</span>
          <span className="t">{flow.next.title}</span>
          <span className="tm">{flow.next.start}</span>
        </button>}
        <footer className="tg-foot">
          <div className="tg-caps" onClick={() => setSheet('capture')} data-screen-label="收集胶囊">
            随手说一句 · 两秒记下<span className="mic"><Ic n="chat" size={16} /></span>
          </div>
        </footer>
        {peek && <TG.Peek onClose={() => setPeek(false)} onLedger={() => { setPeek(false); setSheet('ledger'); }} onOutlook={() => { setPeek(false); setSheet('outlook'); }} />}
        {mood && <React.Fragment>
          <div className="tg-veil" onClick={() => setMood(false)}></div>
          <div className="tg-sheet" data-screen-label="心情打卡"><div className="tg-sbody"><TG.MoodFrame onDone={() => setMood(false)} /></div></div>
        </React.Fragment>}
        {sheet === 'ledger' && <TG.LedgerSheet onClose={() => setSheet(null)} />}
        {sheet === 'outlook' && <TG.OutlookSheet onClose={() => setSheet(null)} />}
        {sheet === 'menu' && <TG.MenuSheet onClose={() => setSheet(null)} />}
        {sheet === 'capture' && <TG.CaptureSheet onClose={() => setSheet(null)} />}
        {sheet === 'why' && <TG.Sheet title="为什么这样安排" icon="eye" onClose={() => setSheet(null)} label="追问 sheet">
          {ex && ex.snapshot ? <React.Fragment>
            <div className="tg-cap">{ex.snapshot.title}</div>
            <div className="tg-sub" style={{ marginTop: 4 }}>{ex.snapshot.reason}</div>
            <div className="tg-li" style={{ marginTop: 8 }}><Ic n="eye" size={13} style={{ marginTop: 3 }} /><span className="lb sb">{ex.snapshot.evidence}</span></div>
          </React.Fragment> : <div className="tg-sub">上下文快照过期了（10 分钟）——重新在提案上点追问。</div>}
          <div style={{ fontSize: 11.5, color: 'var(--ink3)', margin: '14px 0 8px', lineHeight: 1.7 }}>汀刻意不带聊天史。要继续聊，切到任一姊妹前端的「伙伴」——Exchange 快照已带上（同一账本，同一契约）。</div>
          <div className="tg-actrow" style={{ marginTop: 4 }}>
            <a className="tg-btn sec" href="../liuli/index.html">去琉璃的伙伴</a>
            <a className="tg-btn sec" href="../zhiyu/index.html">去纸屿的伙伴</a>
          </div>
        </TG.Sheet>}
        {sheet === 'act' && flow.current && <TG.Sheet title={flow.current.title} icon="dots" onClose={() => setSheet(null)} label="块动作 sheet">
          <button className="tg-menuitem" onClick={() => { const opId = core.setStatus(flow.current.id, 'missed'); TG.tgUndo({ label: '标记没做——没关系，不积债', opId }); setSheet(null); }}><span className="ic"><Ic n="minus" size={15} /></span>没做，先送走（不扣分，没有红色）</button>
          <button className="tg-menuitem" onClick={() => { const r = core.moveBlock(flow.current.id, { date: C.addDays(T, 1), start: flow.current.start }); TG.tgUndo({ label: '推给明天了', opId: r && r.opId }); setSheet(null); }}><span className="ic"><Ic n="arrow-right" size={15} /></span>换一个——这件推给明天</button>
          <button className="tg-menuitem" onClick={() => { const opId = core.removeBlock(flow.current.id); TG.tgUndo({ label: '移走了：' + flow.current.title, opId }); setSheet(null); }}><span className="ic"><Ic n="trash" size={15} /></span>移走（撤销随时在账本里）</button>
        </TG.Sheet>}
      </div>
      <div className="tg-desk">汀 · 此刻 —— 一次只回答一个问题：现在做什么</div>
    </div>;
  }

  ReactDOM.createRoot(document.getElementById('root')).render(<App />);
})();

// 桌面端键盘：Enter=主动作，Esc=次动作，↓=全天
(function () {
  if (window.__tgKeys) return; window.__tgKeys = true;
  document.addEventListener('keydown', (e) => {
    const t = e.target;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
    const q = (s) => document.querySelector(s);
    if (e.key === 'Enter' || e.key === 'ArrowUp') { const b = q('.tg-main .tg-btn.pri'); if (b) { e.preventDefault(); b.click(); } }
    else if (e.key === 'Escape') { const b = q('.tg-sheet .tg-dots, .tg-main .tg-btn.sec'); if (b) { e.preventDefault(); b.click(); } }
    else if (e.key === 'ArrowDown') { const b = q('.tg-handle'); if (b) { e.preventDefault(); b.click(); } }
  });
})();
