// Ported 1:1 from app/page-onboarding.jsx; imports are real now — each card
// opens a file picker and runs the same import pipeline as the Materials page
// (ICS / screenshot candidates are saved wholesale; editable later in Rules).
import React, { useState, useRef } from 'react';
import UI from '../boot/ds.js';
import IC from '../icons.js';
import S from '../store.js';
import { t } from '../i18n.js';
import * as api from '../api.js';
import { useStore, GenOverlay, useToast } from '../ui.jsx';

export default function Onboarding({ onDone }) {
  const st = useStore();
  const toast = useToast();
  const [step, setStep] = useState(0);
  const [imported, setImported] = useState({ canvas: !!st.canvasImportedAt, ics: false, shot: false });
  const [busy, setBusy] = useState(null);
  const [genOpen, setGenOpen] = useState(false);
  const [genDone, setGenDone] = useState(false);
  const canvasRef = useRef(null), icsRef = useRef(null), shotRef = useRef(null);

  async function importFile(kind, file) {
    if (!file || busy) return;
    setBusy(kind);
    try {
      if (kind === 'canvas') {
        const res = await S.importCanvas(await api.readFileText(file));
        if (res.error) throw res;
      } else if (kind === 'ics') {
        const res = await S.icsCandidates(await api.readFileText(file));
        if (res.error) throw res;
        if (res.rules.length) await S.saveRulesBatch(res.rules);
      } else {
        const res = await S.shotCandidates(await api.fileToBase64(file), file.type || 'image/png');
        if (res.error) throw res;
        if (res.rules.length) await S.saveRulesBatch(res.rules);
      }
      setImported((x) => ({ ...x, [kind]: true }));
    } catch (e) {
      toast(t(e.code === 'not_a_timetable' ? 'err_not_a_timetable' : 'err_generic'));
    } finally {
      setBusy(null);
    }
  }
  async function generate() {
    setGenOpen(true); setGenDone(false);
    const T = S.todayIso();
    const res = await S.autoPlan({ from: T, to: T, mode: 'keep_manual' });
    if (res.error) {
      setGenOpen(false);
      toast(t(res.error === 'no_material' ? 'err_no_material' : 'err_generic'));
      S.setOnboarded(true); onDone();
      return;
    }
    setGenDone(true);
  }
  const counts = {
    c: st.courses.length,
    a: st.assignments.filter((x) => x.status === 'pending' || x.status === 'planned').length,
    r: st.rules.filter((x) => x.active).length,
  };
  const hasAny = counts.c || counts.a || counts.r;
  const nDue = st.assignments.filter((a) => a.status === 'pending' && a.dueAt).length;
  const phases = [t('gen_status_1'), t('gen_status_2', { n: nDue }), t('gen_status_3'), t('gen_status_4', { date: t('today') })];

  const IMPORTS = [
    { k: 'canvas', icon: IC.GraduationCap, title: 'ob_import_canvas', sub: 'ob_import_canvas_sub', ref: canvasRef },
    { k: 'ics', icon: IC.CalendarPlus, title: 'ob_import_ics', sub: 'ob_import_ics_sub', ref: icsRef },
    { k: 'shot', icon: IC.Image, title: 'ob_import_shot', sub: 'ob_import_shot_sub', ref: shotRef },
  ];

  return (
    <div className="dc-ob" data-screen-label={'引导第' + (step + 1) + '步'}>
      {step > 0 ? <div className="dc-ob-back"><UI.IconButton label={t('back')} onClick={() => setStep(step - 1)}><IC.ArrowLeft size={20} /></UI.IconButton></div> : null}
      <div className="dc-ob-inner">
        {step === 0 ? (
          <React.Fragment>
            <div className="dc-ob-orb"><IC.Sparkles size={36} /></div>
            <h1 className="dc-ob-title">{t('ob_welcome_title')}</h1>
            <p className="dc-ob-sub">{t('ob_welcome_sub')}</p>
            <div className="dc-ob-actions">
              <UI.Button variant="primary" size="lg" fullWidth onClick={() => setStep(1)}>{t('ob_start')}</UI.Button>
            </div>
          </React.Fragment>
        ) : step === 1 ? (
          <React.Fragment>
            <h1 className="dc-ob-title" style={{ fontSize: 25 }}>{t('ob_step2_title')}</h1>
            <p className="dc-ob-sub" style={{ marginBottom: 24 }}>{t('ob_step2_sub')}</p>
            <div className="dc-stack-12">
              {IMPORTS.map(({ k, icon: Icon, title, sub, ref }) => (
                <UI.GlassCard key={k} padding="none" className="dc-ob-import-card" onClick={() => !imported[k] && !busy && ref.current && ref.current.click()}>
                  <span className="dc-mat-ic"><Icon size={21} /></span>
                  <div className="dc-set-main">
                    <div className="dc-set-title">{t(title)}</div>
                    <div className="dc-set-sub">{t(sub)}</div>
                  </div>
                  {busy === k ? <UI.Skeleton width={54} height={24} />
                    : imported[k] ? <UI.Badge tone="success">{t('ob_imported_badge')}</UI.Badge>
                    : <span className="chev"><IC.ChevronRight size={18} /></span>}
                </UI.GlassCard>
              ))}
            </div>
            <div className="dc-ob-actions dc-mt-20">
              <UI.Button variant="primary" size="lg" fullWidth onClick={() => setStep(2)}>{t('next')}</UI.Button>
              <button className="dc-ob-skip" onClick={() => setStep(2)}>{t('ob_skip')}</button>
            </div>
          </React.Fragment>
        ) : (
          <React.Fragment>
            <div className="dc-ob-orb" style={{ animation: 'none' }}><IC.CheckCircle size={36} /></div>
            <h1 className="dc-ob-title" style={{ fontSize: 25 }}>{t('ob_step3_title')}</h1>
            <p className="dc-ob-sub">{hasAny ? t('ob_step3_sub', counts) : t('ob_step3_sub_empty')}</p>
            <div className="dc-ob-actions">
              <UI.Button variant="primary" size="lg" fullWidth onClick={generate}><IC.Sparkles size={18} /> {t('ob_generate')}</UI.Button>
              <button className="dc-ob-skip" onClick={() => { S.setOnboarded(true); onDone(); }}>{t('ob_generate_skip')}</button>
            </div>
          </React.Fragment>
        )}
      </div>
      <div className="dc-ob-steps">{[0, 1, 2].map((i) => <i key={i} className={i <= step ? 'on' : ''} />)}</div>
      <GenOverlay open={genOpen} phases={phases} done={genDone} onDone={() => { S.setOnboarded(true); onDone(); }} />
      <input ref={canvasRef} type="file" accept=".json,application/json" hidden onChange={(e) => { if (e.target.files.length) { importFile('canvas', e.target.files[0]); e.target.value = ''; } }} />
      <input ref={icsRef} type="file" accept=".ics,text/calendar" hidden onChange={(e) => { if (e.target.files.length) { importFile('ics', e.target.files[0]); e.target.value = ''; } }} />
      <input ref={shotRef} type="file" accept="image/*" hidden onChange={(e) => { if (e.target.files.length) { importFile('shot', e.target.files[0]); e.target.value = ''; } }} />
    </div>
  );
}
