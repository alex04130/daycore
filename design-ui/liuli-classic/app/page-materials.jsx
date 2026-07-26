// Daycore v2.2 — 资料 page: quick capture first, then 记录 feed / 学业 / 数据源 sub-tabs
(function () {
  'use strict';
  const { useState, useEffect, useRef } = React;
  const UI = window.DaycoreUI;
  const IC = window.DcIcons;
  const S = window.DcStore;
  const { t } = window.I18N;
  const { useStore, Sheet, Field, Calendar, useToast } = window.DcUI;
  const MP = window.DcMatParts;
  const CF = window.DcCapture;

  // ---------- add-assignment sheet (manual deadline) ----------
  function AddAssignmentSheet({ open, onClose }) {
    const toast = useToast();
    const [title, setTitle] = useState('');
    const [date, setDate] = useState('');
    const [time, setTime] = useState('23:59');
    const [calOpen, setCalOpen] = useState(false);
    const [month, setMonth] = useState(S.todayIso().slice(0, 7));
    useEffect(() => { if (open) { setTitle(''); setDate(''); setTime('23:59'); } }, [open]);
    function save() {
      if (!title.trim()) return;
      S.createAssignment({ title: title.trim(), dueAt: date ? date + 'T' + time + ':00+08:00' : null });
      toast(t('ad_created'));
      onClose();
    }
    return (
      <React.Fragment>
        <Sheet open={open && !calOpen} onClose={onClose} title={t('ad_title')}>
          <Field label={t('bd_name')}>
            <UI.Input placeholder={t('ad_name_ph')} value={title} onChange={(e) => setTitle(e.target.value)} />
          </Field>
          <div className="dc-grid-2">
            <Field label={t('ad_due')}>
              <UI.Button variant="outline" onClick={() => { setMonth((date || S.todayIso()).slice(0, 7)); setCalOpen(true); }}>
                <IC.CalendarDays size={15} /> {date ? window.I18N.fmtDate(date, 'mdShort') : '—'}
              </UI.Button>
            </Field>
            <Field label={t('rl_time')}>
              <UI.Input type="time" value={time} onChange={(e) => setTime(e.target.value)} />
            </Field>
          </div>
          <div className="dc-sheet-actions">
            <UI.Button variant="outline" fullWidth onClick={onClose}>{t('cancel')}</UI.Button>
            <UI.Button variant="primary" fullWidth disabled={!title.trim()} onClick={save}>{t('save')}</UI.Button>
          </div>
        </Sheet>
        <Sheet open={calOpen} onClose={() => setCalOpen(false)} title={t('ad_due')}>
          <Calendar month={month} onMonth={setMonth} value={date} min={S.todayIso()} onPick={(d) => { setDate(d); setCalOpen(false); }} />
        </Sheet>
      </React.Fragment>
    );
  }

  // ---------- import history ----------
  function ImportHistory({ items }) {
    const meta = {
      canvas: { icon: IC.GraduationCap, key: 'ih_source_canvas' },
      ics: { icon: IC.CalendarPlus, key: 'ih_source_ics' },
      image: { icon: IC.Image, key: 'ih_source_image' },
    };
    if (!items.length) return <UI.EmptyState icon={<IC.History size={24} />} title={t('ih_empty')} />;
    return (
      <div className="dc-stack-8">
        {items.map((h) => {
          const m = meta[h.source] || meta.canvas;
          return (
            <UI.GlassCard key={h.id} padding="none" className="dc-hist-row">
              <span className="dc-set-ic" style={{ width: 34, height: 34 }}><m.icon size={17} /></span>
              <div className="dc-set-main">
                <div className="dc-set-title" style={{ fontSize: 14 }}>{t(m.key)} · {t('ih_items', { n: h.items })}</div>
                <div className="dc-set-sub">{window.I18N.fmtDayLabel(h.createdAt.slice(0, 10), S.todayIso())} {window.I18N.fmtTime(new Date(h.createdAt))}</div>
              </div>
              <IC.Check size={16} style={{ color: 'var(--color-states-success)' }} />
            </UI.GlassCard>
          );
        })}
      </div>
    );
  }

  // ---------- page ----------
  function MaterialsPage({ nav, tweaks }) {
    const st = useStore();
    const toast = useToast();
    const [view, setView] = useState('hub');
    const [autoEditId, setAutoEditId] = useState(null);
    const [sub, setSub] = useState('feed');
    // capture flow
    const [ccPayload, setCcPayload] = useState(null);
    const [ccOpen, setCcOpen] = useState(false);
    const [travelOpen, setTravelOpen] = useState(false);
    const [travelDest, setTravelDest] = useState('');
    // feed
    const [q, setQ] = useState('');
    const [cat, setCat] = useState('all');
    const [detail, setDetail] = useState(null);
    const [detailOpen, setDetailOpen] = useState(false);
    // academic
    const [asgFilter, setAsgFilter] = useState('pending');
    const [addAsgOpen, setAddAsgOpen] = useState(false);
    // sources
    const [tokenOpen, setTokenOpen] = useState(false);
    const [cands, setCands] = useState(null);
    const [candsOpen, setCandsOpen] = useState(false);
    const [busy, setBusy] = useState(null);
    const canvasRef = useRef(null), icsRef = useRef(null), shotRef = useRef(null);

    useEffect(() => {
      const p = nav.params || {};
      if (p.rules) { setView('rules'); setAutoEditId(p.editRule || null); nav.clearParams && nav.clearParams(); }
      else if (p.sub) { setSub(p.sub); nav.clearParams && nav.clearParams(); }
    }, [nav.params]);

    if (view === 'rules') return <MP.RulesView onBack={() => setView('hub')} autoEditId={autoEditId} />;

    function importCanvas() {
      setBusy('canvas');
      setTimeout(() => {
        const res = S.importCanvasDemo();
        setBusy(null);
        toast(t('import_success_canvas', { c: res.courses, a: res.assignments }));
        setTimeout(() => toast('⚠️ ' + t('import_warning_1'), { duration: 4200 }), 700);
      }, 1400);
    }
    function importIcs() {
      setBusy('ics');
      setTimeout(() => { setBusy(null); setCands(S.icsCandidatesDemo().map((c) => ({ ...c, checked: true }))); setCandsOpen(true); }, 1300);
    }
    function importShot() {
      setBusy('shot');
      setTimeout(() => { setBusy(null); setCands(S.shotCandidatesDemo().map((c) => ({ ...c, checked: true }))); setCandsOpen(true); }, 2000);
    }

    const enabledCats = S.listCategories().filter((c) => c.enabled);
    const feed = S.searchMaterials(q, cat);
    const activeRules = st.rules.filter((r) => r.active).length;
    const filters = ['pending', 'all', 'done', 'dismissed'];
    const filtered = st.assignments.filter((a) => {
      if (asgFilter === 'all') return true;
      if (asgFilter === 'pending') return a.status === 'pending' || a.status === 'planned';
      return a.status === asgFilter;
    }).sort((a, b) => (a.dueAt || '9999').localeCompare(b.dueAt || '9999'));
    const fmtTime = (isoStr) => window.I18N.fmtDayLabel(isoStr.slice(0, 10), S.todayIso()) + ' ' + window.I18N.fmtTime(new Date(isoStr));
    const hints = ['cap_hint_1', 'cap_hint_2', 'cap_hint_3'];

    return (
      <div className="dc-page-enter" data-screen-label="资料">
        <UI.SectionHeader title={t('mt_title')} subtitle={t('mt2_sub')} />

        {/* ---- universal quick capture ---- */}
        <div className="dc-mt-20">
          <CF.CaptureBox onResult={(res, raw) => { setCcPayload({ res, text: raw }); setCcOpen(true); }} />
          <div className="dc-cap-hints">
            {hints.map((h) => (
              <button key={h} className="dc-cap-hint" onClick={() => { setCcPayload({ res: S.inboxProcess({ text: t(h) }), text: t(h) }); setCcOpen(true); }}>{t(h)}</button>
            ))}
            {S.catEnabled('travel') ? (
              <button className="dc-cap-hint is-travel" onClick={() => { setTravelDest(''); setTravelOpen(true); }}>
                <IC.Plane size={13} /> {t('tv_entry')}
              </button>
            ) : null}
          </div>
        </div>

        <div className="dc-mt-20" style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center' }}>
          <UI.Button size="sm" variant="outline" disabled={!!busy} onClick={() => canvasRef.current && canvasRef.current.click()}>{busy === 'canvas' ? (window.I18N.lang === 'zh-CN' ? '导入中…' : 'Importing…') : (window.I18N.lang === 'zh-CN' ? '导入 Canvas 作业' : 'Import Canvas')}</UI.Button>
          <UI.Button size="sm" variant="outline" disabled={!!busy} onClick={() => icsRef.current && icsRef.current.click()}>{busy === 'ics' ? (window.I18N.lang === 'zh-CN' ? '导入中…' : 'Importing…') : (window.I18N.lang === 'zh-CN' ? '导入日历 ICS' : 'Import ICS')}</UI.Button>
          <UI.Button size="sm" variant="outline" disabled={!!busy} onClick={() => shotRef.current && shotRef.current.click()}>{busy === 'shot' ? (window.I18N.lang === 'zh-CN' ? '识别中…' : 'Reading…') : (window.I18N.lang === 'zh-CN' ? '识别课表截图' : 'Read timetable photo')}</UI.Button>
          <button className="dc-cap-hint" onClick={() => setSub('sources')}>{window.I18N.lang === 'zh-CN' ? '全部数据源 →' : 'All sources →'}</button>
        </div>

        <div className="dc-mt-20">
          <UI.Tabs value={sub} onValueChange={setSub} items={[
            { value: 'feed', label: t('mt_tab_feed') },
            { value: 'academic', label: t('mt_tab_academic') },
            { value: 'sources', label: t('mt_tab_sources') },
          ]} />
        </div>

        {/* ---- feed ---- */}
        {sub === 'feed' ? (
          <div className="dc-mt-20" data-screen-label="记录列表">
            <div className="dc-feed-search">
              <IC.Search size={16} />
              <input placeholder={t('feed_search_ph')} value={q} onChange={(e) => setQ(e.target.value)} />
              {q ? <button className="dc-feed-clear" aria-label={t('close')} onClick={() => setQ('')}><IC.X size={14} /></button> : null}
            </div>
            <div className="dc-cat-chips">
              <UI.Chip variant={(cat === 'all') ? 'selected' : 'default'} onClick={() => setCat('all')}>{t('cat_all')}</UI.Chip>
              {enabledCats.map((c) => {
                const n = st.materials.filter((m) => m.category === c.id).length;
                return <UI.Chip key={c.id} variant={(cat === c.id) ? 'selected' : 'default'} onClick={() => setCat(c.id)}>{t('cat_' + c.id)}{n ? ' ' + n : ''}</UI.Chip>;
              })}
            </div>
            <div className="dc-stack-8">
              {!feed.length ? (
                <UI.EmptyState icon={<IC.Inbox size={26} />} title={q ? t('feed_no_results') : t('feed_empty_title')} description={q ? null : t('feed_empty_desc')} />
              ) : feed.map((m) => <CF.MaterialRow key={m.id} m={m} onOpen={(x) => { setDetail(x); setDetailOpen(true); }} />)}
            </div>
          </div>
        ) : null}

        {/* ---- academic ---- */}
        {sub === 'academic' ? (
          <div className="dc-mt-20" data-screen-label="学业">
            <UI.SectionHeader title={t('mt_assignments')} action={
              <div className="dc-row-8">
                {filters.map((f) => <UI.Chip key={f} variant={(asgFilter === f) ? 'selected' : 'default'} onClick={() => setAsgFilter(f)}>{t('asg_filter_' + (f === 'pending' ? 'pending' : f))}</UI.Chip>)}
              </div>
            } />
            <div className="dc-stack-8" style={{ marginTop: 12 }}>
              <UI.Button variant="secondary" fullWidth onClick={() => setAddAsgOpen(true)}><IC.Plus size={16} /> {t('mt_add_deadline')}</UI.Button>
              {!filtered.length ? <UI.EmptyState icon={<IC.Inbox size={26} />} title={t('asg_empty')} /> :
                filtered.map((a) => <MP.AssignmentRow key={a.id} a={a} course={st.courses.find((c) => c.id === a.courseId)} />)}
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
          </div>
        ) : null}

        {/* ---- sources ---- */}
        {sub === 'sources' ? (
          <div className="dc-mt-20 dc-mat-grid" data-screen-label="数据源">
            <UI.GlassCard className="dc-mat-card">
              <span className="dc-mat-ic"><IC.GraduationCap size={22} /></span>
              <div className="dc-mat-body">
                <h3 className="dc-mat-title">{t('mt_canvas_title')}</h3>
                <p className="dc-mat-desc">{t('mt_canvas_desc')}</p>
                <div className="dc-mat-actions">
                  <UI.Button size="sm" variant="secondary" disabled={busy === 'canvas'} onClick={() => canvasRef.current.click()}><IC.FileJson size={14} /> {t('mt_upload_json')}</UI.Button>
                  <UI.Button size="sm" variant="outline" onClick={() => setTokenOpen(true)}><IC.Zap size={14} /> {t('mt_token_setup')}</UI.Button>
                </div>
                <div className="dc-mat-meta"><IC.Clock size={12} /> {st.canvasImportedAt ? t('mt_last_import', { time: fmtTime(st.canvasImportedAt) }) : t('mt_not_connected')}</div>
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
            <div className="dc-mt-20">
              <UI.SectionHeader title={t('ih_title')} subtitle={t('ih_sub')} />
              <div style={{ marginTop: 12 }}><ImportHistory items={st.importHistory} /></div>
            </div>
          </div>
        ) : null}

        <input ref={canvasRef} type="file" accept=".json,application/json" hidden onChange={(e) => { if (e.target.files.length) { importCanvas(); e.target.value = ''; } }} />
        <input ref={icsRef} type="file" accept=".ics,text/calendar" hidden onChange={(e) => { if (e.target.files.length) { importIcs(); e.target.value = ''; } }} />
        <input ref={shotRef} type="file" accept="image/*" hidden onChange={(e) => { if (e.target.files.length) { importShot(); e.target.value = ''; } }} />
        <MP.TokenSheet open={tokenOpen} onClose={() => setTokenOpen(false)} />
        <MP.CandidatesSheet open={candsOpen} onClose={() => setCandsOpen(false)} cands={cands} setCands={setCands} />
        <CF.ConfirmSheet open={ccOpen} onClose={() => setCcOpen(false)} payload={ccPayload}
          showConfidence={tweaks ? tweaks.showConfidence : true}
          onPlanTrip={(dest) => { setTravelDest(dest || ''); setTravelOpen(true); }}
          onSaved={(catId) => { if (catId !== 'academic') { setSub('feed'); setCat('all'); } else setSub('academic'); }} />
        <CF.TravelSheet open={travelOpen} onClose={() => setTravelOpen(false)} presetDest={travelDest} onSaved={() => { setSub('feed'); setCat('all'); }} />
        <CF.MaterialDetailSheet m={detail} open={detailOpen} onClose={() => setDetailOpen(false)} />
        <AddAssignmentSheet open={addAsgOpen} onClose={() => setAddAsgOpen(false)} />
      </div>
    );
  }

  window.DcPages = Object.assign(window.DcPages || {}, { MaterialsPage });
})();
