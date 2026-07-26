// Daycore v2.2 — quick-capture flow: capture box, AI confirm card, travel planner, material rows/detail
(function () {
  'use strict';
  const { useState, useEffect, useRef } = React;
  const UI = window.DaycoreUI;
  const IC = window.DcIcons;
  const S = window.DcStore;
  const { t } = window.I18N;
  const { Sheet, Confirm, Field, Calendar, useToast } = window.DcUI;

  const catMeta = (id) => {
    const c = S.CATEGORIES.find((x) => x.id === id) || S.CATEGORIES[0];
    return { id: c.id, Icon: IC[c.icon] || IC.NotebookPen, dot: c.dot, label: t('cat_' + c.id) };
  };
  const fmtWhen = (isoStr) => window.I18N.fmtDayLabel(isoStr.slice(0, 10), S.todayIso()) + ' ' + window.I18N.fmtTime(new Date(isoStr));

  function CatIc({ cat, size }) {
    const m = catMeta(cat);
    return (
      <span className="dc-cat-ic" style={{ color: m.dot, background: 'color-mix(in srgb, ' + m.dot + ' 13%, transparent)', width: size || 40, height: size || 40 }}>
        <m.Icon size={(size || 40) * 0.5} />
      </span>
    );
  }

  // ---------- capture box ----------
  function CaptureBox({ onResult }) {
    const [text, setText] = useState('');
    const [busy, setBusy] = useState(false);
    const fileRef = useRef(null);
    const taRef = useRef(null);
    function run(payload, raw) {
      setBusy(true);
      setTimeout(() => {
        const res = S.inboxProcess(payload);
        setBusy(false); setText('');
        if (taRef.current) taRef.current.style.height = 'auto';
        onResult(res, raw);
      }, 1300);
    }
    function send() {
      const v = text.trim();
      if (!v || busy) return;
      run({ text: v }, v);
    }
    return (
      <div className={'dc-capture' + (busy ? ' is-busy' : '')}>
        <textarea ref={taRef} rows={1} placeholder={t('cap_ph')} value={text} disabled={busy}
          onChange={(e) => { setText(e.target.value); e.target.style.height = 'auto'; e.target.style.height = Math.min(110, e.target.scrollHeight) + 'px'; }}
          onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); } }} />
        <div className="dc-cap-actions">
          <button className="dc-cap-cam" disabled={busy} aria-label={t('cap_camera')} onClick={() => fileRef.current.click()}>
            <IC.Camera size={18} /> <span>{t('cap_camera')}</span>
          </button>
          <div style={{ flex: 1 }} />
          {busy ? <span className="dc-cap-busy"><i className="dc-spin" />{t('cap_busy')}</span> : null}
          <button className="dc-send-btn" style={{ width: 36, height: 36 }} disabled={!text.trim() || busy} aria-label={t('cap_send')} onClick={send}>
            <IC.ArrowUp size={18} />
          </button>
        </div>
        <input ref={fileRef} type="file" accept="image/*" hidden onChange={(e) => { if (e.target.files.length) { run({ photo: true }, t('cap_photo_raw')); e.target.value = ''; } }} />
      </div>
    );
  }

  // ---------- structured preview ----------
  function KV({ k, v }) {
    return <div className="dc-kv-row"><span className="k">{k}</span><span className="v">{v}</span></div>;
  }
  function StructuredPreview({ cat, s, advice }) {
    if (!s && !advice) return null;
    return (
      <div className="dc-kv">
        {s && s.items ? (
          <React.Fragment>
            {s.items.map((it, i) => <KV key={i} k={it.name} v={'≈ ' + it.kcal + ' kcal'} />)}
            {s.total_kcal ? <div className="dc-kv-row is-total"><span className="k">{t('cc_diet_total')}</span><span className="v">≈ {s.total_kcal} kcal · {t('cc_diet_protein', { n: s.protein_g })}</span></div> : null}
          </React.Fragment>
        ) : null}
        {s && s.due_at ? <KV k={t('kv_due_at')} v={window.I18N.fmtDayLabel(s.due_at.slice(0, 10), S.todayIso()) + ' ' + s.due_at.slice(11, 16)} /> : null}
        {s && s.course ? <KV k={t('kv_course')} v={s.course} /> : null}
        {s && s.symptom ? <KV k={t('kv_symptom')} v={s.symptom + (s.since ? ' · ' + s.since : '')} /> : null}
        {s && s.destination ? <KV k={t('kv_destination')} v={s.destination + (s.start_date ? ' · ' + window.I18N.fmtDate(s.start_date, 'mdShort') + (s.end_date ? ' → ' + window.I18N.fmtDate(s.end_date, 'mdShort') : '') : '')} /> : null}
        {s && s.amount != null ? <KV k={t('kv_amount')} v={'¥ ' + s.amount} /> : null}
        {s && s.distance_km != null ? <KV k={t('kv_distance')} v={s.distance_km + ' km'} /> : null}
        {advice ? (
          <div className="dc-advice">
            <div className="dc-advice-head"><IC.HeartPulse size={15} /> {t('cc_health_advice')}</div>
            <p>{advice}</p>
            <p className="dc-disclaimer">{t('cc_health_disclaimer')}</p>
          </div>
        ) : null}
      </div>
    );
  }

  // ---------- itinerary preview ----------
  function Itinerary({ days, tips }) {
    return (
      <div className="dc-itin">
        {days.map((d, i) => (
          <div key={i} className="dc-itin-day">
            <div className="dc-itin-head">
              <span className="n">{t('tv_day', { n: i + 1 })}</span>
              <span className="th">{d.theme}</span>
              <span className="dt">{window.I18N.fmtDate(d.date, 'mdShort')}</span>
            </div>
            <ul>{d.items.map((it, j) => <li key={j}>{it}</li>)}</ul>
          </div>
        ))}
        {tips && tips.length ? (
          <div className="dc-itin-tips">
            <div className="dc-advice-head"><IC.Lightbulb size={15} /> {t('tv_tips')}</div>
            <ul>{tips.map((x, i) => <li key={i}>{x}</li>)}</ul>
          </div>
        ) : null}
      </div>
    );
  }

  // ---------- AI confirm card sheet ----------
  function ConfirmSheet({ open, onClose, payload, onPlanTrip, showConfidence, onSaved }) {
    const toast = useToast();
    const res = payload && payload.res;
    const cls = res && res.classification;
    const [cat, setCat] = useState('note');
    const [title, setTitle] = useState('');
    const [alsoNote, setAlsoNote] = useState(false);
    useEffect(() => {
      if (open && cls) { setCat(cls.category); setTitle(cls.title); setAlsoNote(false); }
    }, [open, payload]);
    if (!res) return null;

    // legacy fallback: no classification
    if (!cls) {
      return (
        <Sheet open={open} onClose={onClose} title={t('cc_fallback_title')}>
          <p className="dc-confirm-body">{res.understanding}</p>
          <div className="dc-row-8" style={{ flexWrap: 'wrap', marginBottom: 6 }}>
            {(res.suggestions || []).map((sg, i) => <UI.Chip key={i} variant="default" onClick={onClose}>{sg}</UI.Chip>)}
          </div>
          <p className="dc-field-sub">{t('cc_fallback_sub')}</p>
          <div className="dc-sheet-actions">
            <UI.Button variant="outline" fullWidth onClick={onClose}>{t('cc_discard')}</UI.Button>
            <UI.Button variant="primary" fullWidth onClick={() => { S.addMaterial({ category: 'note', title: (payload.text || '').slice(0, 18), text: payload.text }); toast(t('cc_saved_toast', { cat: t('cat_note') })); onSaved && onSaved('note'); onClose(); }}>{t('cc_save_note')}</UI.Button>
          </div>
        </Sheet>
      );
    }

    const meta = catMeta(cat);
    const asAssignment = cat === 'academic' && cls.suggested_action === 'create_assignment' && cls.structured && cls.structured.due_at;
    const enabledCats = S.listCategories().filter((c) => c.enabled);
    function commit() {
      const r = S.inboxCommit({ draftId: res.draftId, overrides: { category: cat, title: title.trim() || cls.title }, alsoKeepNote: alsoNote });
      if (r.type === 'assignment') toast(t('cc_asg_created_toast', { title: r.assignment.title }));
      else toast(t('cc_saved_toast', { cat: t('cat_' + r.material.category) }));
      onSaved && onSaved(cat);
      onClose();
    }
    return (
      <Sheet open={open} onClose={onClose} title={t('cc_title')}>
        <div className="dc-cc-head">
          <CatIc cat={cat} size={44} />
          <div style={{ flex: 1, minWidth: 0 }}>
            <UI.Input value={title} onChange={(e) => setTitle(e.target.value)} aria-label={t('cc_title_field')} />
            <div className="dc-cc-meta">
              <UI.Badge tone="primary" style={{ background: 'color-mix(in srgb, ' + meta.dot + ' 14%, transparent)', color: meta.dot }}>{meta.label}</UI.Badge>
              {cls.summary ? <span>{cls.summary}</span> : null}
              {showConfidence ? <span className="dc-conf">{t('cc_confidence', { p: Math.round(cls.confidence * 100) })}</span> : null}
            </div>
          </div>
        </div>

        <StructuredPreview cat={cat} s={cls.structured} advice={cls.advice} />

        {res.altTimetable ? (
          <button className="dc-alt-hint" onClick={() => { onClose(); toast(t('cc_alt_toast')); }}>
            <IC.Image size={15} /> {t('cc_alt_timetable')}
          </button>
        ) : null}

        <Field label={t('cc_change_cat')}>
          <div className="dc-row-8" style={{ flexWrap: 'wrap' }}>
            {enabledCats.map((c) => (
              <UI.Chip key={c.id} variant={(cat === c.id) ? 'selected' : 'default'} onClick={() => setCat(c.id)}>{t('cat_' + c.id)}</UI.Chip>
            ))}
          </div>
        </Field>

        {asAssignment ? (
          <div className="dc-cc-notice">
            <IC.GraduationCap size={16} />
            <div>
              <p>{t('cc_academic_notice')}</p>
              <label className="dc-cc-alsonote">
                <button className={'dc-switch' + (alsoNote ? ' is-on' : '')} role="switch" aria-checked={alsoNote} onClick={() => setAlsoNote(!alsoNote)} />
                <span>{t('cc_also_note')}</span>
              </label>
            </div>
          </div>
        ) : null}

        {cat === 'travel' && cls.suggested_action === 'plan_trip' ? (
          <UI.Button variant="secondary" fullWidth style={{ marginBottom: 4 }} onClick={() => { onClose(); onPlanTrip && onPlanTrip(cls.structured && cls.structured.destination); }}>
            <IC.Route size={16} /> {t('cc_gen_itinerary')}
          </UI.Button>
        ) : null}

        <div className="dc-sheet-actions">
          <UI.Button variant="outline" fullWidth onClick={onClose}>{t('cc_discard')}</UI.Button>
          <UI.Button variant="primary" fullWidth onClick={commit}>
            {asAssignment ? t('cc_save_assignment') : t('cc_save_to', { cat: meta.label })}
          </UI.Button>
        </div>
      </Sheet>
    );
  }

  // ---------- travel planner sheet ----------
  function TravelSheet({ open, onClose, presetDest, onSaved }) {
    const toast = useToast();
    const [dest, setDest] = useState('');
    const [range, setRange] = useState({ from: '', to: '' });
    const [notes, setNotes] = useState('');
    const [busy, setBusy] = useState(false);
    const [plan, setPlan] = useState(null); // {title, summary, itinerary, tips, draftId}
    const [calOpen, setCalOpen] = useState(false);
    const [month, setMonth] = useState(S.todayIso().slice(0, 7));
    useEffect(() => { if (open) { setDest(presetDest || ''); setRange({ from: '', to: '' }); setNotes(''); setPlan(null); setBusy(false); } }, [open]);
    function generate() {
      if (!dest.trim() || busy) return;
      setBusy(true); setPlan(null);
      setTimeout(() => {
        setBusy(false);
        setPlan(S.aiTravel({ destination: dest.trim(), startDate: range.from || undefined, endDate: range.to || undefined, notes }));
      }, 1700);
    }
    function save() {
      const r = S.inboxCommit({ draftId: plan.draftId });
      toast(t('cc_saved_toast', { cat: t('cat_travel') }));
      onSaved && onSaved(r);
      onClose();
    }
    return (
      <React.Fragment>
        <Sheet open={open && !calOpen} onClose={onClose} title={t('tv_title')} wide>
          {!plan ? (
            <React.Fragment>
              <Field label={t('tv_dest')}>
                <UI.Input placeholder={t('tv_dest_ph')} value={dest} onChange={(e) => setDest(e.target.value)} />
              </Field>
              <Field label={t('tv_dates')} sub={range.from ? null : t('tv_dates_sub')}>
                <UI.Button variant="outline" onClick={() => { setMonth((range.from || S.todayIso()).slice(0, 7)); setCalOpen(true); }}>
                  <IC.CalendarDays size={15} /> {range.from ? window.I18N.fmtDate(range.from, 'mdShort') + (range.to ? ' → ' + window.I18N.fmtDate(range.to, 'mdShort') : '') : '—'}
                </UI.Button>
              </Field>
              <Field label={t('tv_notes')}>
                <UI.Input placeholder={t('tv_notes_ph')} value={notes} onChange={(e) => setNotes(e.target.value)} />
              </Field>
              <UI.Button variant="primary" fullWidth disabled={!dest.trim() || busy} onClick={generate}>
                {busy ? <React.Fragment><i className="dc-spin dc-spin--lt" /> {t('tv_generating')}</React.Fragment> : <React.Fragment><IC.Sparkles size={16} /> {t('tv_generate')}</React.Fragment>}
              </UI.Button>
            </React.Fragment>
          ) : (
            <React.Fragment>
              <div className="dc-cc-head" style={{ marginBottom: 4 }}>
                <CatIc cat="travel" size={44} />
                <div>
                  <div style={{ fontWeight: 700, fontSize: 16 }}>{plan.title}</div>
                  <div className="dc-cc-meta"><span>{plan.summary}</span></div>
                </div>
              </div>
              <Itinerary days={plan.itinerary.days} tips={plan.tips} />
              <div className="dc-sheet-actions">
                <UI.Button variant="outline" fullWidth onClick={() => setPlan(null)}>{t('tv_retry')}</UI.Button>
                <UI.Button variant="primary" fullWidth onClick={save}>{t('tv_save')}</UI.Button>
              </div>
            </React.Fragment>
          )}
        </Sheet>
        <Sheet open={calOpen} onClose={() => setCalOpen(false)} title={t('tv_dates')}>
          <Calendar month={month} onMonth={setMonth} range={range} min={S.todayIso()}
            onPick={(d) => {
              if (!range.from || (range.from && range.to)) setRange({ from: d, to: '' });
              else if (d < range.from) setRange({ from: d, to: range.from });
              else { setRange({ from: range.from, to: d }); setCalOpen(false); }
            }} />
          <p className="dc-field-sub" style={{ textAlign: 'center', marginTop: 8 }}>{t('ap_pick_days_travel')}</p>
        </Sheet>
      </React.Fragment>
    );
  }

  // ---------- material row + detail ----------
  function MaterialRow({ m, onOpen }) {
    return (
      <UI.GlassCard padding="none" className="dc-matrow" onClick={() => onOpen(m)}>
        <CatIc cat={m.category} />
        <div className="dc-matrow-main">
          <div className="dc-matrow-title">{m.title}</div>
          <div className="dc-matrow-meta">
            <span className="cat" style={{ color: catMeta(m.category).dot }}>{catMeta(m.category).label}</span>
            {m.summary ? <span className="sum">{m.summary}</span> : null}
            <span>{fmtWhen(m.created_at)}</span>
          </div>
        </div>
        <IC.ChevronRight size={17} style={{ color: 'var(--color-text-muted)', flexShrink: 0 }} />
      </UI.GlassCard>
    );
  }

  function MaterialDetailSheet({ m, open, onClose }) {
    const toast = useToast();
    const [confirmDel, setConfirmDel] = useState(false);
    if (!m) return null;
    const s = m.structured;
    return (
      <React.Fragment>
        <Sheet open={open} onClose={onClose} title={t('md_detail_title')} wide>
          <div className="dc-cc-head">
            <CatIc cat={m.category} size={44} />
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontWeight: 700, fontSize: 16.5, lineHeight: 1.35 }}>{m.title}</div>
              <div className="dc-cc-meta">
                <UI.Badge tone="primary" style={{ background: 'color-mix(in srgb, ' + catMeta(m.category).dot + ' 14%, transparent)', color: catMeta(m.category).dot }}>{catMeta(m.category).label}</UI.Badge>
                <span>{fmtWhen(m.created_at)}</span>
                <span>{t(m.source === 'photo' ? 'md_source_photo' : 'md_source_inbox')}</span>
              </div>
            </div>
          </div>
          {m.text ? <p className="dc-md-text">{m.text}</p> : null}
          {s && s.itinerary ? <Itinerary days={s.itinerary} tips={s.tips} /> : <StructuredPreview cat={m.category} s={s} advice={m.advice} />}
          <div className="dc-sheet-actions">
            <UI.Button variant="outline" fullWidth className="dc-muted" onClick={() => setConfirmDel(true)}><IC.Trash size={16} /> {t('md_delete')}</UI.Button>
            <UI.Button variant="primary" fullWidth onClick={onClose}>{t('ok')}</UI.Button>
          </div>
        </Sheet>
        <Confirm open={confirmDel} onClose={() => setConfirmDel(false)} title={t('md_delete_confirm')} danger confirmLabel={t('del')}
          onConfirm={() => { S.deleteMaterial(m.id); toast(t('md_deleted')); onClose(); }} />
      </React.Fragment>
    );
  }

  window.DcCapture = { CaptureBox, ConfirmSheet, TravelSheet, MaterialRow, MaterialDetailSheet, CatIc, catMeta, Itinerary, StructuredPreview };
})();
