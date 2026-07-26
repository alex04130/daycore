// Daycore v2 — Mood page + 3 fullscreen exercises
(function () {
  'use strict';
  const { useState, useEffect, useRef } = React;
  const UI = window.DaycoreUI;
  const IC = window.DcIcons;
  const S = window.DcStore;
  const { t } = window.I18N;
  const { useStore, useToast } = window.DcUI;

  const MOODS = [
    ['😊', 'mood_great'], ['😌', 'mood_calm'], ['🤩', 'mood_excited'], ['🥰', 'mood_grateful'],
    ['😪', 'mood_tired'], ['😣', 'mood_stressed'], ['😰', 'mood_anxious'], ['😢', 'mood_sad'],
    ['😠', 'mood_angry'], ['😑', 'mood_bored'], ['🫥', 'mood_lonely'], ['🤒', 'mood_sick'],
  ];
  const NEGATIVE = { mood_tired: 'stretch', mood_stressed: 'breathing', mood_anxious: 'grounding', mood_sad: 'breathing', mood_angry: 'breathing', mood_lonely: 'grounding', mood_sick: 'stretch', mood_bored: 'stretch' };
  const AI_REPLIES = {
    'zh-CN': {
      positive: ['能量满满的一天！要不要把这份好状态用在最重要的那件事上？', '真好。记住此刻的感觉，它会是你以后的锚点。', '看到你状态不错，今天的计划就交给我啦。'],
      negative: ['辛苦了。感受本身没有对错，谢谢你愿意告诉我。', '我在呢。要不要先放下清单两分钟，只是陪自己待一会儿？', '这种时候不用逼自己高效。慢一点也可以。'],
    },
    'en-US': {
      positive: ['A full-battery day! Want to spend it on the thing that matters most?', 'Lovely. Remember this feeling — it can anchor you later.', 'Glad you feel good. Leave today’s plan to me.'],
      negative: ['That sounds heavy. Feelings aren’t right or wrong — thank you for telling me.', 'I’m here. Want to set the list aside for two minutes and just be?', 'No need to be productive right now. Slower is fine.'],
    },
  };

  // ============ Breathing (4-7-8, 3 rounds) ============
  function BreathingEx({ onDone, onClose }) {
    const PHASES = [['breath_inhale', 4], ['breath_hold', 7], ['breath_exhale', 8]];
    const ROUNDS = 3;
    const [state, setState] = useState({ round: -1, phase: 0, left: 0 }); // round -1 = ready
    useEffect(() => {
      let alive = true;
      let tm;
      function runPhase(round, phase) {
        if (!alive) return;
        if (round >= ROUNDS) { onDone(); return; }
        const [, secs] = PHASES[phase];
        setState({ round, phase, left: secs });
        let remain = secs;
        const tick = () => {
          if (!alive) return;
          remain -= 1;
          if (remain > 0) { setState((s) => ({ ...s, left: remain })); tm = setTimeout(tick, 1000); }
          else {
            const np = phase + 1;
            if (np < PHASES.length) runPhase(round, np);
            else runPhase(round + 1, 0);
          }
        };
        tm = setTimeout(tick, 1000);
      }
      const start = setTimeout(() => runPhase(0, 0), 2600);
      return () => { alive = false; clearTimeout(start); clearTimeout(tm); };
    }, []);
    const ready = state.round < 0;
    const phaseKey = ready ? null : PHASES[state.phase][0];
    const scale = ready ? 1 : state.phase === 0 ? 1.55 : state.phase === 1 ? 1.55 : 1;
    const dur = ready ? 1 : PHASES[state.phase][1];
    return (
      <div className="dc-ex-overlay" data-screen-label="呼吸练习">
        <div className="dc-ex-close"><UI.IconButton aria-label={t('close')} onClick={onClose} icon={<IC.X size={20} />} /></div>
        <p className="dc-ex-title">{t('ex_breathing')} · 4-7-8</p>
        <div className="dc-breath-circle">
          <div className="dc-breath-ring" style={{ transform: 'scale(' + (scale * 1.02) + ')', transition: 'transform ' + dur + 's ease-in-out' }} />
          <div className="dc-breath-core" style={{ transform: 'scale(' + scale + ')', transition: 'transform ' + dur + 's ease-in-out' }}>
            {ready ? <IC.Wind size={34} /> : state.left}
          </div>
        </div>
        <p className="dc-ex-phase">{ready ? t('breath_ready') : t(phaseKey)}</p>
        <p className="dc-ex-sub">{ready ? '' : t('breath_cycle', { i: state.round + 1, n: ROUNDS })}</p>
        <div className="dc-ex-progress">{Array.from({ length: ROUNDS }, (_, i) => <i key={i} className={!ready && i <= state.round ? 'on' : ''} />)}</div>
        <UI.Button variant="ghost" onClick={onClose}>{t('ex_skip')}</UI.Button>
      </div>
    );
  }

  // ============ Stretch (5 guided steps, 20s each with skip) ============
  function StretchEx({ onDone, onClose }) {
    const STEPS = ['stretch_step_1', 'stretch_step_2', 'stretch_step_3', 'stretch_step_4', 'stretch_step_5'];
    const SECS = 20;
    const [i, setI] = useState(0);
    const [left, setLeft] = useState(SECS);
    useEffect(() => {
      setLeft(SECS);
      const iv = setInterval(() => setLeft((x) => {
        if (x <= 1) { clearInterval(iv); if (i + 1 < STEPS.length) setI(i + 1); else onDone(); return 0; }
        return x - 1;
      }), 1000);
      return () => clearInterval(iv);
    }, [i]);
    const pct = 1 - left / SECS;
    return (
      <div className="dc-ex-overlay" data-screen-label="伸展练习">
        <div className="dc-ex-close"><UI.IconButton aria-label={t('close')} onClick={onClose} icon={<IC.X size={20} />} /></div>
        <p className="dc-ex-title">{t('ex_stretch')}</p>
        <div className="dc-stretch-fig" style={{ animation: 'dc-breathe 3s ease-in-out infinite' }}>
          <UI.ProgressRing size={150} strokeWidth={7} value={pct}>
            <IC.PersonStanding size={52} />
          </UI.ProgressRing>
        </div>
        <p className="dc-ex-phase">{t(STEPS[i])}</p>
        <p className="dc-ex-sub">{t('stretch_of', { i: i + 1, n: STEPS.length })} · {left}s</p>
        <div className="dc-ex-progress">{STEPS.map((_, j) => <i key={j} className={j <= i ? 'on' : ''} />)}</div>
        <div className="dc-row-8">
          <UI.Button variant="ghost" onClick={onClose}>{t('ex_skip')}</UI.Button>
          <UI.Button variant="secondary" onClick={() => (i + 1 < STEPS.length ? setI(i + 1) : onDone())}>{t('next')}</UI.Button>
        </div>
      </div>
    );
  }

  // ============ Grounding 5-4-3-2-1 (tap lights) ============
  function GroundingEx({ onDone, onClose }) {
    const STEPS = [['ground_5', 5], ['ground_4', 4], ['ground_3', 3], ['ground_2', 2], ['ground_1', 1]];
    const [i, setI] = useState(0);
    const [lit, setLit] = useState(0);
    const [key, count] = STEPS[i];
    function tap(j) {
      if (j !== lit) return;
      const n = lit + 1;
      if (n >= count) {
        setTimeout(() => {
          if (i + 1 < STEPS.length) { setI(i + 1); setLit(0); } else onDone();
        }, 450);
        setLit(n);
      } else setLit(n);
    }
    return (
      <div className="dc-ex-overlay" data-screen-label="着地练习">
        <div className="dc-ex-close"><UI.IconButton aria-label={t('close')} onClick={onClose} icon={<IC.X size={20} />} /></div>
        <p className="dc-ex-title">{t('ex_grounding')} · 5-4-3-2-1</p>
        <IC.Anchor size={44} style={{ color: 'var(--color-primary)' }} />
        <p className="dc-ex-phase" key={key}>{t(key)}</p>
        <p className="dc-ex-sub">{t('ground_tap_hint')}</p>
        <div className="dc-ground-lights">
          {Array.from({ length: count }, (_, j) => (
            <button key={i + '-' + j} className={'dc-ground-light' + (j < lit ? ' on' : '')} aria-label={'light ' + (j + 1)} onClick={() => tap(j)} />
          ))}
        </div>
        <div className="dc-ex-progress">{STEPS.map((_, j) => <i key={j} className={j <= i ? 'on' : ''} />)}</div>
        <UI.Button variant="ghost" onClick={onClose}>{t('ex_skip')}</UI.Button>
      </div>
    );
  }

  // ============ Mood page ============
  function MoodPage() {
    const st = useStore();
    const toast = useToast();
    const [picked, setPicked] = useState(null);   // mood key
    const [thinking, setThinking] = useState(false);
    const [reply, setReply] = useState(null);      // {text, exercise, checkinId}
    const [exercise, setExercise] = useState(null); // 'breathing'|'stretch'|'grounding' + checkinId
    const lang = window.I18N.lang;

    function pick(emoji, key) {
      if (thinking) return;
      setPicked(key); setReply(null); setThinking(true);
      const isNeg = !!NEGATIVE[key];
      const pool = AI_REPLIES[lang] || AI_REPLIES['zh-CN'];
      const text = (isNeg ? pool.negative : pool.positive)[Math.floor(Math.random() * 3)];
      const ex = NEGATIVE[key] || null;
      setTimeout(() => {
        const checkin = S.addMood({ mood: emoji + ' ' + t(key), aiResponse: text, exerciseOffered: ex });
        setThinking(false);
        setReply({ text, exercise: ex, checkinId: checkin.id });
      }, 1400);
    }
    function finishExercise() {
      if (exercise && exercise.checkinId) S.completeMoodExercise(exercise.checkinId);
      setExercise(null);
      toast(t('ex_completed_toast'));
    }
    const exLabel = { breathing: 'ex_breathing', stretch: 'ex_stretch', grounding: 'ex_grounding' };
    const exOffer = { breathing: 'md_offer_breathing', stretch: 'md_offer_stretch', grounding: 'md_offer_grounding' };
    const fmtWhen = (isoStr) => window.I18N.fmtDayLabel(isoStr.slice(0, 10), S.todayIso()) + ' ' + window.I18N.fmtTime(new Date(isoStr));

    return (
      <div className="dc-page-enter" data-screen-label="心情">
        <UI.SectionHeader title={t('md_title')} subtitle={t('md_sub')} />
        <div className="dc-mood-grid dc-mt-20">
          {MOODS.map(([emoji, key]) => (
            <UI.MoodTile key={key} emoji={emoji} label={t(key)} selected={picked === key} onClick={() => pick(emoji, key)} />
          ))}
        </div>

        {thinking ? (
          <div className="dc-mt-20"><UI.ChatBubble role="assistant"><UI.TypingDots /></UI.ChatBubble></div>
        ) : null}
        {reply ? (
          <div className="dc-mood-reply dc-mt-20">
            <UI.ChatBubble role="assistant">{reply.text}</UI.ChatBubble>
            {reply.exercise ? (
              <UI.GlassCard className="dc-mt-20" style={{ display: 'flex', alignItems: 'center', gap: 14 }}>
                <span className="dc-mat-ic">
                  {reply.exercise === 'breathing' ? <IC.Wind size={20} /> : reply.exercise === 'stretch' ? <IC.PersonStanding size={20} /> : <IC.Anchor size={20} />}
                </span>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div className="dc-set-title">{t(exLabel[reply.exercise])}</div>
                  <div className="dc-set-sub">{t(exOffer[reply.exercise])}</div>
                </div>
              </UI.GlassCard>
            ) : null}
            {reply.exercise ? (
              <div className="dc-sheet-actions" style={{ marginTop: 12 }}>
                <UI.Button variant="ghost" fullWidth onClick={() => setReply(null)}>{t('md_not_now')}</UI.Button>
                <UI.Button variant="primary" fullWidth onClick={() => setExercise({ kind: reply.exercise, checkinId: reply.checkinId })}>{t('md_start_exercise')}</UI.Button>
              </div>
            ) : null}
          </div>
        ) : null}

        {st.moods.length ? (
          <div className="dc-mt-20" style={{ marginTop: 34 }}>
            <UI.SectionHeader title={t('md_history')} />
            <div className="dc-stack-8" style={{ marginTop: 12 }}>
              {st.moods.slice(0, 6).map((m) => (
                <UI.GlassCard key={m.id} padding="none" className="dc-mood-history-row">
                  <span className="dc-mood-emoji">{m.mood.split(' ')[0]}</span>
                  <div className="dc-mood-hist-main">
                    <div className="dc-mood-hist-label">
                      {m.mood.split(' ').slice(1).join(' ')}
                      {m.exerciseCompleted ? <UI.Badge tone="success">{t('md_exercise_done_badge')}</UI.Badge> : null}
                    </div>
                    {m.aiResponse ? <div className="dc-mood-hist-resp">{m.aiResponse}</div> : null}
                    <div className="dc-mood-hist-time">{fmtWhen(m.createdAt)}</div>
                  </div>
                </UI.GlassCard>
              ))}
            </div>
          </div>
        ) : null}

        {exercise && exercise.kind === 'breathing' ? <BreathingEx onDone={finishExercise} onClose={() => setExercise(null)} /> : null}
        {exercise && exercise.kind === 'stretch' ? <StretchEx onDone={finishExercise} onClose={() => setExercise(null)} /> : null}
        {exercise && exercise.kind === 'grounding' ? <GroundingEx onDone={finishExercise} onClose={() => setExercise(null)} /> : null}
      </div>
    );
  }

  window.DcPages = Object.assign(window.DcPages || {}, { MoodPage });
})();
