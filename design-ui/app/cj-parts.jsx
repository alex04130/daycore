// 长卷 — 共享零件：hook / toast 总线 / 提案堆叠 / 心情胶囊 / 导语 / 推送 / 浮层回答
(function () {
  'use strict';
  const { useState, useEffect, useRef } = React;
  const C = window.DcCore,IC = window.DcIcons;

  function useCore() {const [, f] = React.useReducer((x) => x + 1, 0);useEffect(() => C.subscribe(f), []);return C;}
  function useTick(ms) {const [, f] = React.useReducer((x) => x + 1, 0);useEffect(() => {const t = setInterval(f, ms);return () => clearInterval(t);}, [ms]);}

  // toast 总线（轻卡 + 撤销）
  const TB = { list: [], subs: new Set(), emit() {this.subs.forEach((f) => f([...this.list]));} };
  function cjToast(t) {const row = Object.assign({ id: C.uid('t') }, t);TB.list = [row, ...TB.list].slice(0, 4);TB.emit();if (t.ttl !== 0) setTimeout(() => cjDrop(row.id), t.ttl || 7000);return row.id;}
  function cjDrop(id) {TB.list = TB.list.filter((x) => x.id !== id);TB.emit();}
  window.cjToast = cjToast;

  function Toasts() {
    const [list, setList] = useState(TB.list);
    useEffect(() => {TB.subs.add(setList);return () => TB.subs.delete(setList);}, []);
    return <div className="cj-toasts">{list.map((t) =>
      <div key={t.id} className="cj-toast glass">
        <span className="ic">{t.kind === 'stone' ? <IC.Anchor size={15} /> : t.kind === 'info' ? <IC.Sparkles size={15} /> : <IC.CheckCircle size={15} />}</span>
        <div className="bd">
          <div className="lb">{t.label}</div>
          {t.sub && <div className="sb">{t.sub}</div>}
          <div className="acts">
            {t.opId && <button onClick={() => {if (C.undo(t.opId)) {cjDrop(t.id);cjToast({ label: '已撤销', sub: '账本只增不改——这是一笔反向补偿', kind: 'info' });}}}>撤销</button>}
            {t.onView && <button onClick={() => {t.onView();cjDrop(t.id);}}>去看看</button>}
            {t.onAct && <button onClick={() => {t.onAct();cjDrop(t.id);}}>{t.actLabel || '好'}</button>}
            <button className="mute" onClick={() => cjDrop(t.id)}>知道了</button>
          </div>
        </div>
      </div>)}
    </div>;
  }

  // 提案堆叠：只装 L2 · ≤3 · 底部露出下一张边缘
  function Stack({ onFollow, onGlance }) {
    const core = useCore();useTick(30e3);
    const [open, setOpen] = useState(false);
    const cards = core.stack();
    if (!cards.length) return null;
    const p = cards[0];
    const left = Math.max(0, Math.round((p.expiresAt - core.nowMs()) / 3600e3 * 10) / 10);
    const ttlText = (p.ttlRule === 'silent_accept' ? '沉默即接受' : '沉默即拒绝') + ' · ' + (left >= 1 ? Math.round(left) + 'h' : Math.max(5, Math.round(left * 60)) + 'm');
    const act = (fn, label) => {const r = fn();if (r) cjToast({ label, opId: r.opId });};
    if (!open) return <button className="cj-stack-pill glass" data-screen-label="提案入口" onClick={() => setOpen(true)}><span className="dot"></span><span className="n">{cards.length}</span><span className="lbl">件事等你点头</span></button>;
    return <div className="cj-stack" data-screen-label="提案堆叠">
      <div className="meta"><span className="n">{cards.length}</span>需要你点头的事<span style={{ flex: 1 }} /><span className="cj-l0">{ttlText}</span><button className="cls" title="收起" onClick={() => setOpen(false)}><IC.X size={12} /></button></div>
      <div className="cj-card glass" key={p.id}>
        <span className="ttl">{p.domain === 'care' ? '♡ 关怀' : ''}</span>
        <h4>{p.title}</h4>
        <div className="sum">{p.summary}</div>
        <div className="why">{p.reason}</div>
        {p.rows ? <div className="cj-rows">{p.rows.map((r) =>
          <div key={r.id} className={'cj-row ' + (r.state === 'accepted' ? 'acc' : r.state === 'rejected' ? 'rej' : '')}>
            <span className="lb">{r.label}</span>
            {r.state === 'pending' ? <React.Fragment>
              <button className="rb y" title="接受这行" onClick={() => act(() => core.accept(p.id, r.id), '接受：' + r.label)}><IC.Check size={14} /></button>
              <button className="rb n" title="拒绝这行" onClick={() => act(() => core.reject(p.id, r.id), '拒绝了一行')}><IC.X size={13} /></button>
            </React.Fragment> : <span style={{ color: r.state === 'accepted' ? 'var(--ok)' : 'var(--ink3)' }}>{r.state === 'accepted' ? <IC.Check size={14} /> : <IC.X size={13} />}</span>}
          </div>)}
        </div> : null}
        <div className="ev"><IC.Sparkles size={11} />{p.evidence}</div>
        <div className="cj-actions">
          {!p.rows && <button className="cj-btn pri" onClick={() => act(() => core.accept(p.id), '接受：' + p.title)}><IC.Check size={14} />接受</button>}
          {p.rows && <button className="cj-btn pri" onClick={() => act(() => core.accept(p.id), '全部接受')}>全部接受</button>}
          <button className="cj-btn sec" onClick={() => act(() => core.reject(p.id), '拒绝了——它会学乖一点')}>拒绝</button>
          <button className="cj-btn ghost" onClick={() => {core.followUp(p.id, '为什么这样安排？');onFollow && onFollow();}}>追问</button>
        </div>
      </div>
      {cards.length > 1 && <div className="cj-peek" title="下一张：" onClick={onGlance}></div>}
    </div>;
  }

  // 心情胶囊
  const ECHO = { '开心': '看得出来。这样的日子多来点。', '平静': '平静最养人，安排就保持原样。', '被爱': '记下了。今天适合早点收工。', '兴奋': '劲头正好——难的事要不要现在做？', '一般': '一般也挺好，不用勉强精彩。', '疲惫': '看到了。我把晚上的安排放轻了一点。', '压力大': '我听到了。要不要我把最近的事拆小一点？', '焦虑': '深呼吸。最要紧的只有一件，其他我盯着。', '低落': '嗯，先照顾自己。计划会迁就你。', '烦躁': '那今天就少排点事。', '不舒服': '健康优先，其余的我先顺延。', '失眠': '躺着也是休息。明早的安排我调晚了些。' };
  function MoodCapsule() {
    const core = useCore();
    const [open, setOpen] = useState(false);
    const [sel, setSel] = useState(null);
    const [note, setNote] = useState('');
    const [echo, setEcho] = useState(null);
    const m = core.anyMoodToday();
    const save = () => {if (!sel) return;const r = core.moodRecord({ e: sel.e, l: sel.l, note, source: 'user' });cjToast({ label: '心情记下了：' + sel.e + ' ' + sel.l, opId: r.opId });setEcho(ECHO[sel.l] || '记下了。');setSel(null);setNote('');setTimeout(() => {setEcho(null);setOpen(false);}, 3600);};
    return <div className="cj-mood" data-screen-label="心情胶囊">
      <button className="cj-moodcap glass" onClick={() => setOpen(!open)}>
        {m ? <React.Fragment><span className="e">{m.e}</span>{m.l}{m.source === 'agent' && <span style={{ fontSize: 10, color: 'var(--ink3)' }}>小禾代记</span>}</React.Fragment> :
        <React.Fragment><IC.Smile size={16} />今天的心情</React.Fragment>}
      </button>
      {open && <div className="cj-moodpick glass">
        <h4>{m ? '今天已打卡——想更新就再选一次' : '现在感觉怎么样？'}</h4>
        <div className="cj-moodgrid">{core.MOODS.map((o) =>
          <button key={o.l} className={sel && sel.l === o.l ? 'on' : ''} onClick={() => setSel(o)}><span className="e">{o.e}</span>{o.l}</button>)}
        </div>
        <input placeholder="想说一句吗（可空）" value={note} onChange={(e) => setNote(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && save()} />
        {echo && <div className="cj-echo">🌱 {echo}</div>}
        <div className="row">
          <button className="cj-btn ghost" onClick={() => setOpen(false)}>先不了</button>
          <button className="cj-btn pri" disabled={!sel} style={!sel ? { opacity: .4, pointerEvents: 'none' } : null} onClick={save}>记下</button>
        </div>
      </div>}
    </div>;
  }

  // 晨导语改为静默：不占 UI，内容留在伙伴抽屉里

  // 推送 chip（预算 ≤3/天）
  function PushChip() {
    const core = useCore();
    const [open, setOpen] = useState(false);
    const p = core.unreadPush();
    if (!p) return null;
    return <React.Fragment>
      <button className="cj-push glass" onClick={() => setOpen(!open)}><span className="dot" /><span className="t">{p.title}</span></button>
      {open && <div className="cj-push-card glass">
        <h4>{p.title}</h4><p>{p.body}</p>
        <div className="row">
          <span className="cj-l0" style={{ marginRight: 'auto' }}>推送预算 {core.pushBudget().used}/{core.pushBudget().max}</span>
          <button className="cj-btn sec" onClick={() => {core.markPushRead(p.id);setOpen(false);}}>知道了</button>
        </div>
      </div>}
    </React.Fragment>;
  }

  // 查到 → 浮层 + 视图变化
  function AnswerSheet({ a, onClose, onView }) {
    if (!a) return null;
    return <div className="cj-answer glass" data-screen-label="查询浮层">
      <h4><span className="ic"><IC.Sparkles size={16} /></span>{a.title}</h4>
      <ul>{a.lines.map((l, i) => <li key={i}>{l}</li>)}</ul>
      <div className="row">
        {a.view && <button className="cj-btn pri" onClick={() => {onView(a.view);onClose();}}>去看看</button>}
        <button className="cj-btn sec" onClick={onClose}>好</button>
      </div>
    </div>;
  }

  Object.assign(window, { CJ: { useCore, useTick, cjToast, cjDrop, Toasts, Stack, MoodCapsule, PushChip, AnswerSheet } });
})();