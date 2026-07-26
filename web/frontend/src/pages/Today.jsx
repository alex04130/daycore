// Ported 1:1 from app/page-today.jsx. Differences from the prototype are the
// real async data calls: auto-plan/plan-text/plan-image hit the backend, and
// visible date ranges are ensured into the plan cache.
import React, { useState, useEffect, useRef, useMemo } from 'react';
import UI from '../boot/ds.js';
import IC from '../icons.js';
import S from '../store.js';
import I18N, { t } from '../i18n.js';
import * as api from '../api.js';
import { useStore, useLang, Sheet, Confirm, Segmented, Field, Calendar, WeekStrip, GenOverlay, useToast, TYPE_META, typeLabel } from '../ui.jsx';

const nowMin = () => { const d = new Date(); return d.getHours() * 60 + d.getMinutes(); };
const delay = (ms) => new Promise((r) => setTimeout(r, ms));

// ============ Auto-plan sheet ============
function AutoPlanSheet({ open, onClose, defaultDate, variant, onPlanned, goMaterials }) {
  const T = S.todayIso();
  const [range, setRange] = useState('today');
  const [custom, setCustom] = useState({ from: null, to: null });
  const [month, setMonth] = useState(T.slice(0, 7));
  const [instructions, setInstructions] = useState('');
  const [mode, setMode] = useState('keep_manual');
  const [phase, setPhase] = useState('form'); // form | gen | done
  const [err, setErr] = useState(null);
  useEffect(() => { if (open) { setPhase('form'); setErr(null); setRange(defaultDate === T ? 'today' : 'custom'); if (defaultDate !== T) setCustom({ from: defaultDate, to: defaultDate }); } }, [open]);

  const resolved = useMemo(() => {
    if (range === 'today') return { from: T, to: T };
    if (range === 'tomorrow') return { from: S.addDays(T, 1), to: S.addDays(T, 1) };
    if (range === 'd3') return { from: T, to: S.addDays(T, 2) };
    if (range === 'week') return { from: T, to: S.addDays(T, 6) };
    return custom.from ? { from: custom.from, to: custom.to || custom.from } : null;
  }, [range, custom, T]);

  const badges = useMemo(() => {
    const map = {};
    for (let i = -31; i < 62; i++) {
      const d = S.addDays(month + '-01', i);
      const nRules = S.state.rules.filter((r) => S.ruleOccursOn(r, d)).length;
      if (nRules) map[d] = { color: null };
    }
    return map;
  }, [month, open]);

  const st = S.state;
  const nDue = st.assignments.filter((a) => a.status === 'pending' && a.dueAt && S.dayDiff(a.dueAt.slice(0, 10), T) >= 0 && S.dayDiff(a.dueAt.slice(0, 10), T) <= 14).length;
  const nRules = st.rules.filter((r) => r.active).length;

  function pickDay(d) {
    if (!custom.from || (custom.from && custom.to)) setCustom({ from: d, to: null });
    else {
      let from = custom.from, to = d;
      if (to < from) { to = from; from = d; }
      if (S.dayDiff(to, from) > 6) to = S.addDays(from, 6);
      setCustom({ from, to });
    }
  }
  async function submit() {
    if (!resolved) return;
    setPhase('gen');
    const [res] = await Promise.all([
      S.autoPlan({ from: resolved.from, to: resolved.to, instructions, mode }),
      delay(2800), // let the phase animation breathe even on fast responses
    ]);
    if (res.error) { setPhase('form'); setErr(res.error); return; }
    setPhase('done');
  }
  const phases = [t('gen_status_1'), t('gen_status_2', { n: nDue }), t('gen_status_3'), t('gen_status_4', { date: resolved ? I18N.fmtDayLabel(resolved.from, T) : '' })];
  const rangeLabel = resolved ? (resolved.from === resolved.to
    ? t('ap_selected_one', { d: I18N.fmtDayLabel(resolved.from, T) })
    : t('ap_selected_range', { from: I18N.fmtDate(resolved.from, 'mdShort'), to: I18N.fmtDate(resolved.to, 'mdShort'), n: S.dayDiff(resolved.to, resolved.from) + 1 })) : null;

  return (
    <React.Fragment>
      <Sheet open={open && phase === 'form'} onClose={onClose} title={t('ap_title')}>
        {variant === 'brief' ? (
          <div className="dc-row-8" style={{ flexWrap: 'wrap', marginBottom: 18 }}>
            <UI.Chip variant="default">{'📚 ' + t('ap_day_badge_rules', { n: nRules })}</UI.Chip>
            <UI.Chip variant="default">{'⏰ ' + t('ap_day_badge_due', { n: nDue })}</UI.Chip>
            {st.keyFacts.slice(0, 2).map((f, i) => <UI.Chip key={i} variant="default">{'💡 ' + f}</UI.Chip>)}
          </div>
        ) : null}
        <Field label={t('ap_range')} sub={rangeLabel}>
          <Segmented small value={range} onChange={setRange} options={[
            { value: 'today', label: t('ap_range_today') }, { value: 'tomorrow', label: t('ap_range_tomorrow') },
            { value: 'd3', label: t('ap_range_3') }, { value: 'week', label: t('ap_range_week') },
            { value: 'custom', label: t('ap_custom') },
          ]} />
        </Field>
        {range === 'custom' ? (
          <div className="dc-field-row">
            <span className="dc-field-sub">{t('ap_pick_days')}</span>
            <Calendar month={month} onMonth={setMonth} range={custom} onPick={pickDay} min={T} max={S.addDays(T, 60)} badges={badges} />
          </div>
        ) : null}
        <Field label={t('ap_instructions')}>
          <UI.Textarea rows={2} placeholder={t('ap_instructions_ph')} value={instructions} onChange={(e) => setInstructions(e.target.value)} />
        </Field>
        <Field label={t('ap_mode')}>
          <div className="dc-stack-8">
            {[['keep_manual', 'ap_mode_keep', 'ap_mode_keep_sub'], ['replace_all', 'ap_mode_replace', 'ap_mode_replace_sub']].map(([v, l, s]) => (
              <button key={v} className="dc-set-row" style={{ border: '1.5px solid ' + (mode === v ? 'var(--color-primary)' : 'var(--color-border-custom)'), borderRadius: 16, background: mode === v ? 'color-mix(in srgb, var(--color-primary) 7%, transparent)' : 'none', width: '100%', textAlign: 'left' }} onClick={() => setMode(v)}>
                <div className="dc-set-main">
                  <div className="dc-set-title">{t(l)}</div>
                  <div className="dc-set-sub">{t(s)}</div>
                </div>
                {mode === v ? <IC.CheckCircle size={20} style={{ color: 'var(--color-primary)', flexShrink: 0 }} /> : <IC.Circle size={20} style={{ color: 'var(--color-text-muted)', flexShrink: 0 }} />}
              </button>
            ))}
          </div>
        </Field>
        {err ? (
          <div className="dc-plan-note" style={{ background: 'color-mix(in srgb, var(--color-states-warning) 12%, transparent)' }}>
            <IC.Inbox size={18} style={{ color: 'var(--color-states-warning)' }} />
            <span>
              {t(err === 'no_material' ? 'err_no_material' : err === 'range_too_large' ? 'err_range_too_large' : err === 'rate_limited' ? 'err_rate_limited' : 'err_generic')}
              {err === 'no_material' ? <button className="lnk" style={{ background: 'none', border: 'none', color: 'var(--color-primary)', fontWeight: 700, cursor: 'pointer', padding: '0 0 0 6px', font: 'inherit' }} onClick={() => { onClose(); goMaterials(); }}>{t('err_go_import')}</button> : null}
            </span>
          </div>
        ) : null}
        <UI.Button variant="primary" size="lg" fullWidth disabled={!resolved} onClick={submit}>
          <IC.Sparkles size={18} /> {t('ap_submit')}
        </UI.Button>
      </Sheet>
      <GenOverlay open={open && phase !== 'form'} phases={phases} done={phase === 'done'} onDone={() => { onPlanned(resolved); onClose(); }} />
    </React.Fragment>
  );
}

// ============ Block detail sheet ============
function BlockDetailSheet({ block, date, onClose, onEditRule }) {
  const toast = useToast();
  const [draft, setDraft] = useState(null);
  const [confirmDel, setConfirmDel] = useState(false);
  useEffect(() => { setDraft(block ? { title: block.title, type: block.type, time: block.time || '', duration_min: block.duration_min || '', time_mode: block.time_mode } : null); }, [block && block.id]);
  if (!block || !draft) return <Sheet open={false} onClose={onClose} />;
  const isRule = block.origin === 'rule' || !!block.rule_id;
  const rule = isRule ? S.state.rules.find((r) => r.id === block.rule_id) : null;
  const dirty = draft.title !== block.title || draft.type !== block.type || (draft.time || null) !== block.time || String(draft.duration_min || '') !== String(block.duration_min || '') || draft.time_mode !== block.time_mode;

  function save() {
    S.patchPlan(date, { action: 'update', match: { id: block.id }, changes: { title: draft.title.trim() || block.title, type: draft.type, time: draft.time || null, duration_min: draft.duration_min ? Number(draft.duration_min) : null, time_mode: draft.time_mode, origin: block.origin === 'auto' ? 'manual' : block.origin } });
    toast(t('bd_saved_toast'));
    onClose();
  }
  function toggleComplete() {
    S.patchPlan(date, { action: 'update', match: { id: block.id }, changes: { completed: !block.completed } });
    if (!block.completed) toast(t('bd_completed_toast'));
    onClose();
  }
  function remove() {
    S.patchPlan(date, { action: 'remove', match: { id: block.id } });
    toast(isRule ? t('bd_hidden_toast') : t('bd_deleted_toast'));
    onClose();
  }
  const originKey = block.origin === 'auto' ? 'bd_origin_auto' : isRule ? 'bd_origin_rule' : 'bd_origin_manual';
  return (
    <React.Fragment>
      <Sheet open={!!block} onClose={onClose} title={t('bd_title')}>
        <div className="dc-row-8" style={{ marginBottom: 16 }}>
          <UI.Badge tone="primary">{t(originKey)}</UI.Badge>
          {rule ? <span className="dc-field-sub">{t('bd_from_rule', { title: rule.title })}</span> : null}
        </div>
        <Field label={t('bd_name')}>
          <UI.Input value={draft.title} onChange={(e) => setDraft({ ...draft, title: e.target.value })} />
        </Field>
        <Field label={t('bd_type')}>
          <div className="dc-row-8" style={{ flexWrap: 'wrap' }}>
            {Object.keys(TYPE_META).map((ty) => (
              <UI.Chip key={ty} variant={(draft.type === ty) ? 'selected' : 'default'} onClick={() => setDraft({ ...draft, type: ty })}>{typeLabel(ty)}</UI.Chip>
            ))}
          </div>
        </Field>
        <div className="dc-grid-2">
          <Field label={t('bd_time')} sub={!draft.time ? t('bd_no_time') : null}>
            <UI.Input type="time" value={draft.time} onChange={(e) => setDraft({ ...draft, time: e.target.value })} />
          </Field>
          <Field label={t('bd_duration')}>
            <UI.Input type="number" min="5" step="5" value={draft.duration_min} onChange={(e) => setDraft({ ...draft, duration_min: e.target.value })} />
          </Field>
        </div>
        <Field inline label={t('bd_fixed')} sub={t('bd_fixed_sub')}>
          <button className={'dc-switch' + (draft.time_mode === 'fixed' ? ' is-on' : '')} role="switch" aria-checked={draft.time_mode === 'fixed'} onClick={() => setDraft({ ...draft, time_mode: draft.time_mode === 'fixed' ? 'floating' : 'fixed' })} />
        </Field>
        <div className="dc-stack-8">
          <div className="dc-sheet-actions" style={{ marginTop: 4 }}>
            <UI.Button variant="secondary" fullWidth onClick={toggleComplete}>
              <IC.CheckCircle size={17} /> {block.completed ? t('bd_uncomplete') : t('bd_completed')}
            </UI.Button>
            <UI.Button variant="primary" fullWidth disabled={!dirty} onClick={save}>{t('save')}</UI.Button>
          </div>
          {rule && onEditRule ? (
            <UI.Button variant="outline" fullWidth onClick={() => { onClose(); onEditRule(rule); }}>
              <IC.Repeat size={16} /> {t('bd_edit_rule')}
            </UI.Button>
          ) : null}
          <UI.Button variant="ghost" fullWidth className="dc-muted" onClick={() => (isRule ? remove() : setConfirmDel(true))}>
            {isRule ? <React.Fragment><IC.EyeOff size={16} /> {t('bd_hide_today')}</React.Fragment> : <React.Fragment><IC.Trash size={16} /> {t('bd_delete')}</React.Fragment>}
          </UI.Button>
          {isRule ? <p className="dc-field-sub" style={{ textAlign: 'center', margin: 0 }}>{t('bd_hide_sub')}</p> : null}
        </div>
      </Sheet>
      <Confirm open={confirmDel} onClose={() => setConfirmDel(false)} title={t('bd_delete')} danger confirmLabel={t('del')} onConfirm={remove} />
    </React.Fragment>
  );
}

// ============ Manual add sheet ============
function ManualAddSheet({ open, onClose, date }) {
  const toast = useToast();
  const [tab, setTab] = useState('text');
  const [text, setText] = useState('');
  const [target, setTarget] = useState(date);
  const [busy, setBusy] = useState(false);
  const [cands, setCands] = useState(null); // parsed blocks with .checked
  const [err, setErr] = useState(null);
  const [calOpen, setCalOpen] = useState(false);
  const [month, setMonth] = useState(date.slice(0, 7));
  const fileRef = useRef(null);
  useEffect(() => { if (open) { setTab('text'); setText(''); setCands(null); setErr(null); setTarget(date); setCalOpen(false); setMonth(date.slice(0, 7)); } }, [open]);
  const T = S.todayIso();

  async function parse() {
    setBusy(true); setErr(null);
    const res = await S.planFromText(text, target);
    setBusy(false);
    if (res.error) { setErr(res.error); return; }
    setCands(res.blocks.map((b) => ({ ...b, checked: true })));
  }
  async function parseImage(file) {
    setBusy(true); setErr(null);
    const res = await S.planFromImage(await api.fileToBase64(file), file.type || 'image/png', target);
    setBusy(false);
    if (res.error) { setErr(res.error); return; }
    setCands(res.blocks.map((b) => ({ ...b, checked: true })));
  }
  function confirmAdd() {
    const picked = cands.filter((c) => c.checked);
    picked.forEach((c) => { const { checked, ...b } = c; S.patchPlan(target, { action: 'add', block: { ...b, origin: 'manual' } }); });
    toast(t('di_added_toast', { n: picked.length }));
    onClose();
  }
  const errText = (code) => {
    const key = 'err_' + code;
    const s = t(key);
    return s === key ? t('err_generic') : s;
  };
  return (
    <Sheet open={open} onClose={onClose} title={t('di_title')}>
      {!cands ? (
        <React.Fragment>
          <div style={{ marginBottom: 16 }}>
            <Segmented value={tab} onChange={setTab} options={[{ value: 'text', label: t('di_tab_text') }, { value: 'image', label: t('di_tab_image') }]} />
          </div>
          <Field label={t('di_target')}>
            <div>
              <div className="dc-row-8" style={{ flexWrap: 'wrap' }}>
                {[T, S.addDays(T, 1)].map((d) => (
                  <UI.Chip key={d} variant={(target === d) ? 'selected' : 'default'} onClick={() => { setTarget(d); setCalOpen(false); }}>{I18N.fmtDayLabel(d, T)}</UI.Chip>
                ))}
                {date !== T && date !== S.addDays(T, 1) ? <UI.Chip variant={(target === date) ? 'selected' : 'default'} onClick={() => { setTarget(date); setCalOpen(false); }}>{I18N.fmtDate(date, 'mdShort')}</UI.Chip> : null}
                {target !== T && target !== S.addDays(T, 1) && target !== date ? <UI.Chip variant="selected">{I18N.fmtDayLabel(target, T)}</UI.Chip> : null}
                <UI.Chip variant="default" icon={<IC.CalendarDays size={14} />} onClick={() => { setMonth(target.slice(0, 7)); setCalOpen(!calOpen); }}>{t('di_pick_date')}</UI.Chip>
              </div>
              {calOpen ? (
                <div style={{ marginTop: 10 }}>
                  <Calendar month={month} onMonth={setMonth} value={target} min={T} onPick={(d) => { setTarget(d); setCalOpen(false); }} />
                </div>
              ) : null}
            </div>
          </Field>
          {tab === 'text' ? (
            <React.Fragment>
              <UI.Textarea rows={4} placeholder={t('di_text_ph')} value={text} onChange={(e) => setText(e.target.value)} />
              {err ? <p className="dc-auth-err" style={{ margin: '10px 0 0' }}>{errText(err)}</p> : null}
              <div className="dc-mt-20">
                <UI.Button variant="primary" size="lg" fullWidth disabled={busy || !text.trim()} onClick={parse}>
                  <IC.Wand size={17} /> {busy ? t('parsing') : t('di_parse')}
                </UI.Button>
              </div>
            </React.Fragment>
          ) : (
            <React.Fragment>
              <UI.EmptyState icon={<IC.Image size={30} />} title={t('di_image_hint')}
                action={<UI.Button variant="primary" disabled={busy} onClick={() => fileRef.current && fileRef.current.click()}>{busy ? t('shot_reading') : t('di_choose_image')}</UI.Button>} />
              {err ? <p className="dc-auth-err" style={{ margin: '10px 0 0', textAlign: 'center' }}>{errText(err)}</p> : null}
              <input ref={fileRef} type="file" accept="image/*" hidden onChange={(e) => { if (e.target.files.length) { parseImage(e.target.files[0]); e.target.value = ''; } }} />
            </React.Fragment>
          )}
        </React.Fragment>
      ) : (
        <React.Fragment>
          <p className="dc-field-sub" style={{ margin: '0 0 12px' }}>{t('candidates_sub')}</p>
          <div className="dc-stack-8">
            {cands.map((c, i) => (
              <button key={c.id || i} className="dc-asg-row dc-glass" style={{ width: '100%', textAlign: 'left', cursor: 'pointer', border: '1.5px solid ' + (c.checked ? 'var(--color-primary)' : 'var(--color-border-custom)') }}
                onClick={() => setCands(cands.map((x, j) => (j === i ? { ...x, checked: !x.checked } : x)))}>
                {c.checked ? <IC.CheckCircle size={19} style={{ color: 'var(--color-primary)', flexShrink: 0, marginTop: 2 }} /> : <IC.Circle size={19} style={{ color: 'var(--color-text-muted)', flexShrink: 0, marginTop: 2 }} />}
                <div className="dc-asg-main">
                  <div className="dc-asg-title">{c.title}</div>
                  <div className="dc-asg-meta">
                    <span>{c.time || t('bd_no_time')}</span>
                    {c.duration_min ? <span>{t('min_unit', { n: c.duration_min })}</span> : null}
                    <span>{typeLabel(c.type)}</span>
                  </div>
                </div>
              </button>
            ))}
          </div>
          <div className="dc-sheet-actions">
            <UI.Button variant="outline" fullWidth onClick={() => setCands(null)}>{t('back')}</UI.Button>
            <UI.Button variant="primary" fullWidth disabled={!cands.some((c) => c.checked)} onClick={confirmAdd}>{t('confirm_save_n', { n: cands.filter((c) => c.checked).length })}</UI.Button>
          </div>
        </React.Fragment>
      )}
    </Sheet>
  );
}

// ============ Today page ============
export default function TodayPage({ nav, tweaks }) {
  const st = useStore();
  useLang();
  const toast = useToast();
  const T = S.todayIso();
  const [date, setDate] = useState(T);
  const [apOpen, setApOpen] = useState(false);
  const [addOpen, setAddOpen] = useState(false);
  const [jumpOpen, setJumpOpen] = useState(false);
  const [jumpMonth, setJumpMonth] = useState(T.slice(0, 7));
  const [detail, setDetail] = useState(null);
  useEffect(() => { S.ensurePlans(date, date); }, [date]);
  useEffect(() => { if (jumpOpen) S.ensurePlans(S.addDays(jumpMonth + '-01', -7), S.addDays(jumpMonth + '-01', 38)); }, [jumpOpen, jumpMonth]);
  const plan = S.getPlan(date);
  const blocks = plan ? plan.blocks : [];
  const timed = blocks.filter((b) => b.time);
  const untimed = blocks.filter((b) => !b.time);
  const doneCount = blocks.filter((b) => b.completed).length;
  const hasAuto = plan && plan.sourceType === 'auto';
  const rulesOnly = plan && plan.sourceType === 'rules';
  const nm = nowMin();
  let nowInserted = false;

  const jumpBadges = useMemo(() => {
    const map = {};
    for (let i = -31; i < 62; i++) {
      const d = S.addDays(jumpMonth + '-01', i);
      const p = S.getPlan(d);
      if (p && p.blocks.length) map[d] = {};
    }
    return map;
  }, [jumpMonth, st]);

  function completeBlock(b) {
    S.patchPlan(date, { action: 'update', match: { id: b.id }, changes: { completed: !b.completed } });
    if (!b.completed) toast(t('bd_completed_toast'));
  }
  const renderBlock = (b) => (
    <div key={b.id} className="dc-block-wrap dc-block-tap" onClick={(e) => { if (e.target.closest('button')) return; setDetail(b); }}>
      <UI.TimeBlockCard time={b.time || t('block_tbd')} title={b.title} type={b.type} durationMin={b.duration_min || undefined}
        timeMode={b.time_mode} completed={b.completed} isAchievement={b.isAchievement}
        past={!!b.time && date === T && S.toMin(b.time) + (b.duration_min || 45) < nm}
        onComplete={() => completeBlock(b)} />
    </div>
  );

  return (
    <div className="dc-page-enter" data-screen-label="今日">
      <WeekStrip date={date} onDate={setDate} onJump={() => { setJumpMonth(date.slice(0, 7)); setJumpOpen(true); }} />
      <div className="dc-row-10" style={{ justifyContent: 'space-between', marginBottom: 14 }}>
        <div className="dc-row-8">
          <h2 style={{ font: '700 19px/1.3 inherit', margin: 0, fontFamily: 'inherit' }}>{I18N.fmtDayLabel(date, T)}</h2>
          {date !== T ? <UI.Chip variant="default" onClick={() => setDate(T)}>{t('back_to_today')}</UI.Chip> : null}
        </div>
        {blocks.length ? <span className="dc-progress-pill"><IC.CheckCircle size={13} /> {t('plan_progress', { done: doneCount, total: blocks.length })}</span> : null}
      </div>

      <button className="dc-hero-cta" onClick={() => setApOpen(true)}>
        <span className="ic"><IC.Sparkles size={22} /></span>
        <span style={{ flex: 1, minWidth: 0 }}>
          <span className="tt" style={{ display: 'block' }}>{t('autoplan_cta_title')}</span>
          <span className="ss" style={{ display: 'block' }}>{t('autoplan_cta_sub')}</span>
        </span>
        <IC.ChevronRight size={19} style={{ opacity: .8, flexShrink: 0 }} />
      </button>
      <div className="dc-today-actions">
        <UI.Button variant="outline" size="md" fullWidth onClick={() => setAddOpen(true)}><IC.Plus size={16} /> {t('manual_add')}</UI.Button>
      </div>

      {hasAuto && plan.note && tweaks.showAiNote ? (
        <div className="dc-plan-note"><IC.Sparkles size={16} /><span>{plan.note}</span></div>
      ) : null}
      {rulesOnly && blocks.length ? (
        <div className="dc-rules-hint"><IC.Repeat size={14} /> {t('rules_only_hint')}</div>
      ) : null}

      {!blocks.length ? (
        <UI.EmptyState icon={<IC.CalendarDays size={30} />} title={t('empty_title')} description={t('empty_desc')}
          action={<div className="dc-stack-8" style={{ minWidth: 220 }}>
            <UI.Button variant="primary" fullWidth onClick={() => nav.go('materials')}>{t('empty_import')}</UI.Button>
            <UI.Button variant="ghost" fullWidth onClick={() => setApOpen(true)}>{t('empty_autoplan')}</UI.Button>
          </div>} />
      ) : (
        <div className="dc-plan-list">
          {timed.map((b) => {
            const els = [];
            if (date === T && !nowInserted && S.toMin(b.time) > nm) {
              nowInserted = true;
              els.push(<div key="now" className="dc-now-line"><span className="lbl">{t('now_label')} {I18N.fmtTime(new Date())}</span><span className="line" /></div>);
            }
            els.push(renderBlock(b));
            return els;
          })}
          {date === T && !nowInserted && timed.length ? <div key="now-end" className="dc-now-line"><span className="lbl">{t('now_label')} {I18N.fmtTime(new Date())}</span><span className="line" /></div> : null}
          {untimed.length ? <div className="dc-group-label"><IC.Inbox size={13} /> {t('no_time_group')}</div> : null}
          {untimed.map(renderBlock)}
        </div>
      )}

      <AutoPlanSheet open={apOpen} onClose={() => setApOpen(false)} defaultDate={date} variant={tweaks.apVariant}
        goMaterials={() => nav.go('materials')}
        onPlanned={(r) => { if (r) setDate(r.from); }} />
      <ManualAddSheet open={addOpen} onClose={() => setAddOpen(false)} date={date} />
      <BlockDetailSheet block={detail} date={date} onClose={() => setDetail(null)} onEditRule={(rule) => nav.go('materials', { rules: true, editRule: rule.id })} />
      <Sheet open={jumpOpen} onClose={() => setJumpOpen(false)} title={t('month_pick_title')}>
        <Calendar month={jumpMonth} onMonth={setJumpMonth} value={date} onPick={(d) => { setDate(d); setJumpOpen(false); }} badges={jumpBadges} />
        <div className="dc-mt-20">
          <UI.Button variant="outline" fullWidth onClick={() => { setDate(T); setJumpOpen(false); }}>{t('back_to_today')}</UI.Button>
        </div>
      </Sheet>
    </div>
  );
}
