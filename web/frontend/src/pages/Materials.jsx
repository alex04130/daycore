// Ported 1:1 from app/page-materials.jsx, with real import pipelines and the
// §7 import-history section (every upload is archived server-side).
import React, { useState, useEffect, useRef } from 'react';
import UI from '../boot/ds.js';
import IC from '../icons.js';
import S from '../store.js';
import I18N, { t } from '../i18n.js';
import * as api from '../api.js';
import { useStore, Sheet, Confirm, Segmented, Field, Calendar, useToast, WeekdayPicker, typeLabel, TYPE_META } from '../ui.jsx';

// ---------- rule describe ----------
export function ruleDesc(r) {
  let s;
  if (r.kind === 'once') s = t('rl_desc_once', { date: I18N.fmtDate(r.date || r.start_date, 'mdShort') });
  else if (r.freq === 'daily') s = t('rl_desc_daily');
  else if (r.freq === 'every_n_days') s = t('rl_desc_every_n', { n: r.interval || 1 });
  else if (r.freq === 'monthly') s = t('rl_desc_monthly', { day: Number((r.start_date || '').split('-')[2] || 1) });
  else s = t('rl_desc_weekly', { days: I18N.joinWeekdays(r.by_weekday && r.by_weekday.length ? r.by_weekday : [S.weekdayOf(r.start_date || S.todayIso())]) });
  if (r.time) s += ' · ' + r.time;
  if (r.duration_min) s += ' · ' + t('min_short', { n: r.duration_min });
  if (r.until && r.kind === 'recurring') s += t('rl_desc_until', { date: I18N.fmtDate(r.until, 'mdShort') });
  return s;
}

// ---------- rule editor sheet ----------
export function RuleEditor({ rule, open, onClose }) {
  const toast = useToast();
  const empty = { title: '', type: 'task', time: '', duration_min: '', time_mode: 'floating', kind: 'recurring', date: S.todayIso(), freq: 'weekly', interval: 2, by_weekday: [S.weekdayOf(S.todayIso())], start_date: S.todayIso(), until: '' };
  const [d, setD] = useState(empty);
  const [dateSheet, setDateSheet] = useState(null); // 'date'|'start'|'until'
  const [month, setMonth] = useState(S.todayIso().slice(0, 7));
  const [confirmDel, setConfirmDel] = useState(false);
  useEffect(() => {
    if (open) setD(rule ? { title: rule.title, type: rule.type, time: rule.time || '', duration_min: rule.duration_min || '', time_mode: rule.time_mode, kind: rule.kind, date: rule.date || S.todayIso(), freq: rule.freq || 'weekly', interval: rule.interval || 2, by_weekday: rule.by_weekday || [], start_date: rule.start_date || S.todayIso(), until: rule.until || '' } : empty);
  }, [open, rule && rule.id]);

  async function save() {
    if (!d.title.trim()) { toast(t('rl_need_title')); return; }
    const payload = {
      title: d.title.trim(), type: d.type, time: d.time || null,
      duration_min: d.duration_min ? Number(d.duration_min) : null,
      time_mode: d.time_mode, kind: d.kind,
      date: d.kind === 'once' ? d.date : undefined,
      freq: d.kind === 'recurring' ? d.freq : undefined,
      interval: d.freq === 'every_n_days' || d.freq === 'weekly' || d.freq === 'monthly' ? Number(d.interval) || 1 : 1,
      by_weekday: d.freq === 'weekly' ? d.by_weekday : [],
      start_date: d.start_date, until: d.until || null,
    };
    try {
      if (rule) { await S.patchRule(rule.id, payload); toast(t('rl_saved')); }
      else { await S.createRule(Object.assign({ active: true, source: 'user' }, payload)); toast(t('rl_created')); }
      onClose();
    } catch (e) {
      toast(t('err_generic'));
    }
  }
  const dateBtn = (label, key, value, clearable) => (
    <Field label={label} sub={clearable && !value ? t('rl_until_forever') : null}>
      <div className="dc-row-8">
        <UI.Button variant="outline" onClick={() => { setMonth((value || S.todayIso()).slice(0, 7)); setDateSheet(key); }}>
          <IC.CalendarDays size={15} /> {value ? I18N.fmtDate(value, 'mdShort') : '—'}
        </UI.Button>
        {clearable && value ? <UI.IconButton label="clear" onClick={() => setD({ ...d, [key === 'until' ? 'until' : key === 'start' ? 'start_date' : 'date']: '' })}><IC.X size={15} /></UI.IconButton> : null}
      </div>
    </Field>
  );
  return (
    <React.Fragment>
      <Sheet open={open} onClose={onClose} title={rule ? t('rl_edit_title') : t('rl_new_title')}>
        <Field label={t('bd_name')}>
          <UI.Input placeholder={t('rl_name_ph')} value={d.title} onChange={(e) => setD({ ...d, title: e.target.value })} />
        </Field>
        <Field label={t('bd_type')}>
          <div className="dc-row-8" style={{ flexWrap: 'wrap' }}>
            {Object.keys(TYPE_META).map((ty) => <UI.Chip key={ty} variant={(d.type === ty) ? 'selected' : 'default'} onClick={() => setD({ ...d, type: ty })}>{typeLabel(ty)}</UI.Chip>)}
          </div>
        </Field>
        <Field label={t('rl_kind')}>
          <Segmented value={d.kind} onChange={(v) => setD({ ...d, kind: v })} options={[{ value: 'recurring', label: t('rl_kind_recur') }, { value: 'once', label: t('rl_kind_once') }]} />
        </Field>
        {d.kind === 'once' ? dateBtn(t('rl_date'), 'date', d.date) : (
          <React.Fragment>
            <Field label={t('rl_freq')}>
              <Segmented small value={d.freq} onChange={(v) => setD({ ...d, freq: v })} options={[
                { value: 'daily', label: t('freq_daily') }, { value: 'weekly', label: t('freq_weekly') },
                { value: 'monthly', label: t('freq_monthly') }, { value: 'every_n_days', label: t('freq_every_n') },
              ]} />
            </Field>
            {d.freq === 'weekly' ? <Field label={t('rl_weekdays')}><WeekdayPicker value={d.by_weekday} onChange={(v) => setD({ ...d, by_weekday: v })} /></Field> : null}
            {d.freq === 'every_n_days' ? <Field label={t('rl_interval_n')}><UI.Input type="number" min="1" style={{ maxWidth: 120 }} value={d.interval} onChange={(e) => setD({ ...d, interval: e.target.value })} /></Field> : null}
          </React.Fragment>
        )}
        <div className="dc-grid-2">
          <Field label={t('rl_time')}>
            <UI.Input type="time" value={d.time} onChange={(e) => setD({ ...d, time: e.target.value })} />
          </Field>
          <Field label={t('rl_duration')}>
            <UI.Input type="number" min="5" step="5" value={d.duration_min} onChange={(e) => setD({ ...d, duration_min: e.target.value })} />
          </Field>
        </div>
        {d.kind === 'recurring' ? (
          <div className="dc-grid-2">
            {dateBtn(t('rl_start'), 'start', d.start_date)}
            {dateBtn(t('rl_until'), 'until', d.until, true)}
          </div>
        ) : null}
        <Field inline label={t('bd_fixed')} sub={t('bd_fixed_sub')}>
          <button className={'dc-switch' + (d.time_mode === 'fixed' ? ' is-on' : '')} role="switch" aria-checked={d.time_mode === 'fixed'} onClick={() => setD({ ...d, time_mode: d.time_mode === 'fixed' ? 'floating' : 'fixed' })} />
        </Field>
        <div className="dc-sheet-actions">
          {rule ? <UI.Button variant="outline" fullWidth className="dc-muted" onClick={() => setConfirmDel(true)}><IC.Trash size={16} /> {t('del')}</UI.Button> : <UI.Button variant="outline" fullWidth onClick={onClose}>{t('cancel')}</UI.Button>}
          <UI.Button variant="primary" fullWidth onClick={save}>{t('save')}</UI.Button>
        </div>
      </Sheet>
      <Sheet open={!!dateSheet} onClose={() => setDateSheet(null)} title={t(dateSheet === 'until' ? 'rl_until' : dateSheet === 'start' ? 'rl_start' : 'rl_date')}>
        <Calendar month={month} onMonth={setMonth}
          value={dateSheet === 'until' ? d.until : dateSheet === 'start' ? d.start_date : d.date}
          onPick={(v) => { setD({ ...d, [dateSheet === 'until' ? 'until' : dateSheet === 'start' ? 'start_date' : 'date']: v }); setDateSheet(null); }} />
      </Sheet>
      <Confirm open={confirmDel} onClose={() => setConfirmDel(false)} title={t('rl_delete_confirm_title')} body={t('rl_delete_confirm')} danger confirmLabel={t('del')}
        onConfirm={async () => { await S.deleteRule(rule.id); toast(t('rl_deleted')); onClose(); }} />
    </React.Fragment>
  );
}

// ---------- candidates confirm sheet (ICS / screenshot) ----------
function CandidatesSheet({ open, onClose, cands, setCands }) {
  const toast = useToast();
  if (!cands) return <Sheet open={false} onClose={onClose} />;
  const n = cands.filter((c) => c.checked).length;
  return (
    <Sheet open={open} onClose={onClose} title={t('candidates_title', { n: cands.length })}>
      <div className="dc-row-10" style={{ justifyContent: 'space-between', marginBottom: 12 }}>
        <span className="dc-field-sub">{t('candidates_sub')}</span>
        <UI.Chip variant={(n === cands.length) ? 'selected' : 'default'} onClick={() => setCands(cands.map((c) => ({ ...c, checked: n !== cands.length })))}>{t('select_all')}</UI.Chip>
      </div>
      <div className="dc-stack-8">
        {cands.map((c, i) => (
          <button key={i} className="dc-asg-row dc-glass" style={{ width: '100%', textAlign: 'left', cursor: 'pointer', border: '1.5px solid ' + (c.checked ? 'var(--color-primary)' : 'var(--color-border-custom)') }}
            onClick={() => setCands(cands.map((x, j) => (j === i ? { ...x, checked: !x.checked } : x)))}>
            {c.checked ? <IC.CheckCircle size={19} style={{ color: 'var(--color-primary)', flexShrink: 0, marginTop: 2 }} /> : <IC.Circle size={19} style={{ color: 'var(--color-text-muted)', flexShrink: 0, marginTop: 2 }} />}
            <div className="dc-asg-main">
              <div className="dc-asg-title">{c.title}</div>
              <div className="dc-asg-meta"><span>{ruleDesc(c)}</span></div>
            </div>
          </button>
        ))}
      </div>
      <div className="dc-sheet-actions">
        <UI.Button variant="outline" fullWidth onClick={onClose}>{t('cancel')}</UI.Button>
        <UI.Button variant="primary" fullWidth disabled={!n} onClick={async () => {
          await S.saveRulesBatch(cands.filter((c) => c.checked).map(({ checked, ...r }) => r));
          toast(t('rules_saved_toast', { n })); onClose();
        }}>{t('confirm_save_n', { n })}</UI.Button>
      </div>
    </Sheet>
  );
}

// ---------- import token sheet ----------
function TokenSheet({ open, onClose }) {
  const st = useStore();
  const toast = useToast();
  const [confirmRotate, setConfirmRotate] = useState(false);
  return (
    <Sheet open={open} onClose={onClose} title={t('token_title')}>
      <p className="dc-confirm-body">{t('token_desc')}</p>
      {st.importToken ? (
        <div className="dc-token-box">
          <IC.Key size={16} style={{ color: 'var(--color-primary)', flexShrink: 0 }} />
          <span style={{ flex: 1 }}>{st.importToken}</span>
          <UI.IconButton label={t('copy')} onClick={() => { try { navigator.clipboard.writeText(st.importToken); } catch (e) {} toast(t('copied')); }}><IC.Copy size={16} /></UI.IconButton>
        </div>
      ) : (
        <div className="dc-token-box dc-muted">{t('token_none')}</div>
      )}
      <div className="dc-sheet-actions">
        {st.importToken
          ? <UI.Button variant="outline" fullWidth onClick={() => setConfirmRotate(true)}><IC.RefreshCw size={15} /> {t('token_rotate')}</UI.Button>
          : <UI.Button variant="primary" fullWidth onClick={() => S.genToken()}><IC.Key size={15} /> {t('token_generate')}</UI.Button>}
      </div>
      <div className="dc-mt-20" style={{ textAlign: 'center' }}>
        <a href="#" onClick={(e) => e.preventDefault()}><IC.Puzzle size={13} style={{ verticalAlign: -2 }} /> {t('ext_guide')}</a>
      </div>
      <Confirm open={confirmRotate} onClose={() => setConfirmRotate(false)} title={t('token_rotate')} body={t('token_rotate_confirm')} onConfirm={() => S.genToken()} />
    </Sheet>
  );
}

// ---------- assignments list ----------
function dueMeta(a) {
  if (!a.dueAt) return null;
  const T = S.todayIso();
  const d = a.dueAt.slice(0, 10);
  const diff = S.dayDiff(d, T);
  if (diff < 0) return { cls: 'urgent', label: t('asg_overdue_ndays', { n: -diff }) };
  if (diff === 0) return { cls: 'urgent', label: t('asg_due_today') };
  if (diff === 1) return { cls: 'urgent', label: t('asg_due_tomorrow') };
  if (diff <= 3) return { cls: 'soon', label: t('asg_due_ndays', { n: diff }) };
  return { cls: '', label: t('asg_due_ndays', { n: diff }) };
}
function AssignmentRow({ a, course }) {
  const toast = useToast();
  const due = dueMeta(a);
  const done = a.status === 'done' || a.submitted;
  return (
    <UI.GlassCard padding="none" className="dc-asg-row">
      <div className="dc-asg-main">
        <div className={'dc-asg-title' + (done ? ' is-done' : '')}>{a.title}</div>
        <div className="dc-asg-meta">
          {course ? <span className="dc-course-code" style={{ fontSize: 11 }}>{course.courseCode}</span> : null}
          {due ? <span className={'dc-due ' + due.cls}>{due.label}</span> : null}
          {a.pointsPossible ? <span>{t('asg_pts', { n: a.pointsPossible })}</span> : null}
          {a.graded ? <span>{t('asg_graded', { score: a.score + '/' + a.pointsPossible })}</span> : a.submitted ? <span>{t('asg_submitted')}</span> : null}
          {a.status === 'planned' ? <span style={{ color: 'var(--color-primary)' }}>{t('asg_planned_hint')}</span> : null}
        </div>
      </div>
      <div className="dc-asg-actions">
        {a.status !== 'done' && a.status !== 'dismissed' ? (
          <React.Fragment>
            <button className="dc-asg-icon-btn" title={t('asg_mark_done')} onClick={() => { S.patchAssignment(a.id, { status: 'done' }); toast(t('bd_completed_toast')); }}><IC.Check size={17} /></button>
            <button className="dc-asg-icon-btn" title={t('asg_dismiss')} onClick={() => S.patchAssignment(a.id, { status: 'dismissed' })}><IC.EyeOff size={16} /></button>
          </React.Fragment>
        ) : (
          <button className="dc-asg-icon-btn" title={t('asg_restore')} onClick={() => S.patchAssignment(a.id, { status: 'pending' })}><IC.RefreshCw size={15} /></button>
        )}
      </div>
    </UI.GlassCard>
  );
}

// ---------- Rules subpage ----------
function RulesView({ onBack, autoEditId }) {
  const st = useStore();
  const [editor, setEditor] = useState(null); // null | 'new' | rule
  useEffect(() => {
    if (autoEditId) { const r = st.rules.find((x) => x.id === autoEditId); if (r) setEditor(r); }
  }, [autoEditId]);
  const srcLabel = { user: 'rl_source_user', chat: 'rl_source_chat', ics: 'rl_source_ics', image: 'rl_source_image', canvas: 'rl_source_canvas' };
  return (
    <div className="dc-page-enter" data-screen-label="重复规则">
      <div className="dc-row-10" style={{ marginBottom: 4 }}>
        <UI.IconButton aria-label={t('back')} onClick={onBack} icon={<IC.ArrowLeft size={19} />} />
        <UI.SectionHeader title={t('rl_title')} subtitle={t('rl_sub')} className="" />
      </div>
      <div className="dc-mt-20 dc-stack-8">
        <UI.Button variant="secondary" fullWidth onClick={() => setEditor('new')}><IC.Plus size={16} /> {t('rl_new')}</UI.Button>
        {!st.rules.length ? (
          <UI.EmptyState icon={<IC.Repeat size={28} />} title={t('rl_empty_title')} description={t('rl_empty_desc')} />
        ) : st.rules.map((r) => (
          <UI.GlassCard key={r.id} padding="none" className={'dc-rule-row' + (r.active ? '' : ' is-paused')}>
            <div className="dc-rule-main" onClick={() => setEditor(r)}>
              <div className="dc-rule-title">
                {r.title}
                <UI.Badge tone="neutral">{t(srcLabel[r.source] || 'rl_source_user')}</UI.Badge>
                {!r.active ? <UI.Badge tone="neutral">{t('rl_paused')}</UI.Badge> : null}
              </div>
              <div className="dc-rule-desc">{ruleDesc(r)}</div>
            </div>
            <button className={'dc-switch' + (r.active ? ' is-on' : '')} role="switch" aria-checked={r.active} onClick={() => S.patchRule(r.id, { active: !r.active })} />
          </UI.GlassCard>
        ))}
      </div>
      <RuleEditor rule={editor === 'new' ? null : editor} open={!!editor} onClose={() => setEditor(null)} />
    </div>
  );
}

// ---------- Materials hub ----------
export default function MaterialsPage({ nav }) {
  const st = useStore();
  const toast = useToast();
  const [view, setView] = useState('hub');
  const [autoEditId, setAutoEditId] = useState(null);
  const [tokenOpen, setTokenOpen] = useState(false);
  const [cands, setCands] = useState(null);
  const [candsOpen, setCandsOpen] = useState(false);
  const [busy, setBusy] = useState(null); // 'canvas'|'ics'|'shot'
  const [asgFilter, setAsgFilter] = useState('pending');
  const canvasRef = useRef(null), icsRef = useRef(null), shotRef = useRef(null);

  useEffect(() => {
    const p = nav.params || {};
    if (p.rules) { setView('rules'); setAutoEditId(p.editRule || null); nav.clearParams && nav.clearParams(); }
  }, [nav.params]);

  if (view === 'rules') return <RulesView onBack={() => setView('hub')} autoEditId={autoEditId} />;

  async function importCanvas(file) {
    setBusy('canvas');
    const res = await S.importCanvas(await api.readFileText(file));
    setBusy(null);
    if (res.error) { toast(t('err_generic') + (res.message ? ' — ' + res.message : '')); return; }
    toast(t('import_success_canvas', { c: res.courses, a: res.assignments }));
    (res.warnings || []).forEach((w, i) => setTimeout(() => toast('⚠️ ' + w, { duration: 4200 }), 700 * (i + 1)));
  }
  async function importIcs(file) {
    setBusy('ics');
    const res = await S.icsCandidates(await api.readFileText(file));
    setBusy(null);
    if (res.error) { toast(t('err_generic')); return; }
    setCands(res.rules.map((c) => ({ ...c, checked: true })));
    setCandsOpen(true);
  }
  async function importShot(file) {
    setBusy('shot');
    const res = await S.shotCandidates(await api.fileToBase64(file), file.type || 'image/png');
    setBusy(null);
    if (res.error) { toast(t(res.error === 'not_a_timetable' ? 'err_not_a_timetable' : res.error === 'image_too_large' ? 'err_image_too_large' : 'err_generic')); return; }
    setCands(res.rules.map((c) => ({ ...c, checked: true })));
    setCandsOpen(true);
  }
  const fmtWhen = (isoStr) => I18N.fmtDayLabel(isoStr.slice(0, 10), S.todayIso()) + ' ' + I18N.fmtTime(new Date(isoStr));

  const activeRules = st.rules.filter((r) => r.active).length;
  const filters = ['pending', 'all', 'done', 'dismissed'];
  const filtered = st.assignments.filter((a) => {
    if (asgFilter === 'all') return true;
    if (asgFilter === 'pending') return a.status === 'pending' || a.status === 'planned';
    return a.status === asgFilter;
  }).sort((a, b) => (a.dueAt || '9999').localeCompare(b.dueAt || '9999'));

  const impSrcLabel = { canvas: 'imp_source_canvas', ics: 'imp_source_ics', image: 'imp_source_image' };

  return (
    <div className="dc-page-enter" data-screen-label="资料库">
      <UI.SectionHeader title={t('mt_title')} subtitle={t('mt_sub')} />
      <div className="dc-mt-20 dc-mat-grid">
        <UI.GlassCard className="dc-mat-card">
          <span className="dc-mat-ic"><IC.GraduationCap size={22} /></span>
          <div className="dc-mat-body">
            <h3 className="dc-mat-title">{t('mt_canvas_title')}</h3>
            <p className="dc-mat-desc">{t('mt_canvas_desc')}</p>
            <div className="dc-mat-actions">
              <UI.Button size="sm" variant="secondary" disabled={busy === 'canvas'} onClick={() => canvasRef.current.click()}><IC.FileJson size={14} /> {t('mt_upload_json')}</UI.Button>
              <UI.Button size="sm" variant="outline" onClick={() => setTokenOpen(true)}><IC.Zap size={14} /> {t('mt_token_setup')}</UI.Button>
            </div>
            <div className="dc-mat-meta"><IC.Clock size={12} /> {st.canvasImportedAt ? t('mt_last_import', { time: fmtWhen(st.canvasImportedAt) }) : t('mt_not_connected')}</div>
          </div>
        </UI.GlassCard>
        <UI.GlassCard className="dc-mat-card">
          <span className="dc-mat-ic"><IC.CalendarPlus size={22} /></span>
          <div className="dc-mat-body">
            <h3 className="dc-mat-title">{t('mt_ics_title')}</h3>
            <p className="dc-mat-desc">{t('mt_ics_desc')}</p>
            <div className="dc-mat-actions">
              <UI.Button size="sm" variant="secondary" disabled={busy === 'ics'} onClick={() => icsRef.current.click()}><IC.Upload size={14} /> {t('mt_upload_ics')}</UI.Button>
            </div>
          </div>
        </UI.GlassCard>
        <UI.GlassCard className="dc-mat-card">
          <span className="dc-mat-ic"><IC.Image size={22} /></span>
          <div className="dc-mat-body">
            <h3 className="dc-mat-title">{t('mt_shot_title')}</h3>
            <p className="dc-mat-desc">{t('mt_shot_desc')}</p>
            <div className="dc-mat-actions">
              <UI.Button size="sm" variant="secondary" disabled={busy === 'shot'} onClick={() => shotRef.current.click()}><IC.Upload size={14} /> {busy === 'shot' ? t('shot_reading') : t('mt_upload_image')}</UI.Button>
            </div>
          </div>
        </UI.GlassCard>
        <UI.GlassCard padding="none" className="dc-set-row" onClick={() => setView('rules')}>
          <span className="dc-set-ic"><IC.Repeat size={19} /></span>
          <div className="dc-set-main">
            <div className="dc-set-title">{t('mt_rules_entry')}</div>
            <div className="dc-set-sub">{t('mt_rules_entry_sub', { n: st.rules.length, m: activeRules })}</div>
          </div>
          <IC.ChevronRight size={18} style={{ color: 'var(--color-text-muted)' }} />
        </UI.GlassCard>
      </div>

      <div className="dc-mt-20">
        <UI.SectionHeader title={t('mt_assignments')} action={
          <div className="dc-row-8">
            {filters.map((f) => <UI.Chip key={f} variant={(asgFilter === f) ? 'selected' : 'default'} onClick={() => setAsgFilter(f)}>{t('asg_filter_' + (f === 'pending' ? 'pending' : f))}</UI.Chip>)}
          </div>
        } />
        <div className="dc-stack-8" style={{ marginTop: 12 }}>
          {!filtered.length ? <UI.EmptyState icon={<IC.Inbox size={26} />} title={t('asg_empty')} /> :
            filtered.map((a) => <AssignmentRow key={a.id} a={a} course={st.courses.find((c) => c.id === a.courseId)} />)}
        </div>
      </div>

      <div className="dc-mt-20">
        <UI.SectionHeader title={t('mt_courses')} />
        <div className="dc-stack-8" style={{ marginTop: 12 }}>
          {!st.courses.length ? <UI.EmptyState icon={<IC.BookOpen size={26} />} title={t('course_empty')} /> :
            st.courses.map((c) => (
              <UI.GlassCard key={c.id} padding="none" className="dc-course-row">
                <span className="dc-course-code">{c.courseCode}</span>
                <span className="dc-course-name">{c.name}</span>
                {c.currentScore != null ? <span className="dc-grade">{c.currentGrade}<small>{c.currentScore}</small></span> : <span className="dc-muted" style={{ fontSize: 12 }}>—</span>}
              </UI.GlassCard>
            ))}
        </div>
      </div>

      {st.importHistory.length ? (
        <div className="dc-mt-20">
          <UI.SectionHeader title={t('mt_import_history')} subtitle={t('mt_import_history_sub')} />
          <div className="dc-stack-8" style={{ marginTop: 12 }}>
            {st.importHistory.slice(0, 8).map((rec) => (
              <UI.GlassCard key={rec.id} padding="none" className="dc-asg-row">
                <span className="dc-set-ic">
                  {rec.source === 'canvas' ? <IC.GraduationCap size={17} /> : rec.source === 'ics' ? <IC.CalendarPlus size={17} /> : <IC.Image size={17} />}
                </span>
                <div className="dc-asg-main">
                  <div className="dc-asg-title" style={{ fontSize: 13.5 }}>{t(impSrcLabel[rec.source] || 'imp_source_canvas')} · {t('imp_items', { n: rec.items })}</div>
                  <div className="dc-asg-meta"><span>{fmtWhen(rec.createdAt)}</span>{rec.summary ? <span>{rec.summary}</span> : null}</div>
                </div>
              </UI.GlassCard>
            ))}
          </div>
        </div>
      ) : null}

      <input ref={canvasRef} type="file" accept=".json,application/json" hidden onChange={(e) => { if (e.target.files.length) { importCanvas(e.target.files[0]); e.target.value = ''; } }} />
      <input ref={icsRef} type="file" accept=".ics,text/calendar" hidden onChange={(e) => { if (e.target.files.length) { importIcs(e.target.files[0]); e.target.value = ''; } }} />
      <input ref={shotRef} type="file" accept="image/*" hidden onChange={(e) => { if (e.target.files.length) { importShot(e.target.files[0]); e.target.value = ''; } }} />
      <TokenSheet open={tokenOpen} onClose={() => setTokenOpen(false)} />
      <CandidatesSheet open={candsOpen} onClose={() => setCandsOpen(false)} cands={cands} setCands={setCands} />
    </div>
  );
}
