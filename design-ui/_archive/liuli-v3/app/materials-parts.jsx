// Daycore v2.2 — materials building blocks: rule editor/list, import candidates, token sheet, assignment row
(function () {
  'use strict';
  const { useState, useEffect, useRef, useMemo } = React;
  const UI = window.DaycoreUI;
  const IC = window.DcIcons;
  const S = window.DcStore;
  const { t } = window.I18N;
  const { useStore, Sheet, Confirm, Segmented, Field, Calendar, useToast, WeekdayPicker, typeLabel, TYPE_META } = window.DcUI;

  // ---------- rule describe ----------
  function ruleDesc(r) {
    let s;
    if (r.kind === 'once') s = t('rl_desc_once', { date: window.I18N.fmtDate(r.date || r.start_date, 'mdShort') });
    else if (r.freq === 'daily') s = t('rl_desc_daily');
    else if (r.freq === 'every_n_days') s = t('rl_desc_every_n', { n: r.interval || 1 });
    else if (r.freq === 'monthly') s = t('rl_desc_monthly', { day: Number((r.start_date || '').split('-')[2] || 1) });
    else s = t('rl_desc_weekly', { days: window.I18N.joinWeekdays(r.by_weekday && r.by_weekday.length ? r.by_weekday : [S.weekdayOf(r.start_date || S.todayIso())]) });
    if (r.time) s += ' · ' + r.time;
    if (r.duration_min) s += ' · ' + t('min_short', { n: r.duration_min });
    if (r.until && r.kind === 'recurring') s += t('rl_desc_until', { date: window.I18N.fmtDate(r.until, 'mdShort') });
    return s;
  }

  // ---------- rule editor sheet ----------
  function RuleEditor({ rule, open, onClose }) {
    const toast = useToast();
    const empty = { title: '', type: 'task', time: '', duration_min: '', time_mode: 'floating', kind: 'recurring', date: S.todayIso(), freq: 'weekly', interval: 2, by_weekday: [S.weekdayOf(S.todayIso())], start_date: S.todayIso(), until: '' };
    const [d, setD] = useState(empty);
    const [dateSheet, setDateSheet] = useState(null); // 'date'|'start'|'until'
    const [month, setMonth] = useState(S.todayIso().slice(0, 7));
    const [confirmDel, setConfirmDel] = useState(false);
    useEffect(() => {
      if (open) setD(rule ? { title: rule.title, type: rule.type, time: rule.time || '', duration_min: rule.duration_min || '', time_mode: rule.time_mode, kind: rule.kind, date: rule.date || S.todayIso(), freq: rule.freq || 'weekly', interval: rule.interval || 2, by_weekday: rule.by_weekday || [], start_date: rule.start_date || S.todayIso(), until: rule.until || '' } : empty);
    }, [open, rule && rule.id]);

    function save() {
      if (!d.title.trim()) { toast(t('rl_need_title')); return; }
      const payload = {
        title: d.title.trim(), type: d.type, time: d.time || null,
        duration_min: d.duration_min ? Number(d.duration_min) : null,
        time_mode: d.time_mode, kind: d.kind,
        date: d.kind === 'once' ? d.date : null,
        freq: d.kind === 'recurring' ? d.freq : undefined,
        interval: d.freq === 'every_n_days' || d.freq === 'weekly' || d.freq === 'monthly' ? Number(d.interval) || 1 : 1,
        by_weekday: d.freq === 'weekly' ? d.by_weekday : [],
        start_date: d.start_date, until: d.until || null,
      };
      if (rule) { S.patchRule(rule.id, payload); toast(t('rl_saved')); }
      else { S.createRule(Object.assign({ active: true, source: 'user' }, payload)); toast(t('rl_created')); }
      onClose();
    }
    const dateBtn = (label, key, value, clearable) => (
      <Field label={label} sub={clearable && !value ? t('rl_until_forever') : null}>
        <div className="dc-row-8">
          <UI.Button variant="outline" onClick={() => { setMonth((value || S.todayIso()).slice(0, 7)); setDateSheet(key); }}>
            <IC.CalendarDays size={15} /> {value ? window.I18N.fmtDate(value, 'mdShort') : '—'}
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
          onConfirm={() => { S.deleteRule(rule.id); toast(t('rl_deleted')); onClose(); }} />
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
          <UI.Button variant="primary" fullWidth disabled={!n} onClick={() => {
            S.saveRulesBatch(cands.filter((c) => c.checked).map(({ checked, ...r }) => r));
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

  // exports (v2.2: MaterialsPage itself lives in page-materials.jsx)
  window.DcMatParts = { RuleEditor, RulesView, CandidatesSheet, TokenSheet, AssignmentRow, ruleDesc, dueMeta };
  window.DcPages = Object.assign(window.DcPages || {}, { RuleEditor, ruleDesc });
})();
