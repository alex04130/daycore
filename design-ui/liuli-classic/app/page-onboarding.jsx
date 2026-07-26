// Daycore v2 — 3-step onboarding: welcome → import (skippable) → first auto-plan
(function () {
  'use strict';
  const { useState } = React;
  const UI = window.DaycoreUI;
  const IC = window.DcIcons;
  const S = window.DcStore;
  const { t } = window.I18N;
  const { useStore, GenOverlay, useToast } = window.DcUI;

  function Onboarding({ onDone }) {
    const st = useStore();
    const toast = useToast();
    const [step, setStep] = useState(0);
    const [imported, setImported] = useState({ canvas: !!st.canvasImportedAt, ics: false, shot: false });
    const [busy, setBusy] = useState(null);
    const [genOpen, setGenOpen] = useState(false);
    const [genDone, setGenDone] = useState(false);

    function fakeImport(kind) {
      if (imported[kind] || busy) return;
      setBusy(kind);
      setTimeout(() => {
        if (kind === 'canvas') S.importCanvasDemo();
        else S.saveRulesBatch((kind === 'ics' ? S.icsCandidatesDemo() : S.shotCandidatesDemo()));
        setImported((x) => ({ ...x, [kind]: true }));
        setBusy(null);
      }, kind === 'shot' ? 1900 : 1300);
    }
    function generate() {
      setGenOpen(true); setGenDone(false);
      setTimeout(() => {
        const T = S.todayIso();
        S.autoPlan({ from: T, to: T, mode: 'keep_manual' });
        setGenDone(true);
      }, 3400);
    }
    const counts = {
      c: st.courses.length,
      a: st.assignments.filter((x) => x.status === 'pending' || x.status === 'planned').length,
      r: st.rules.filter((x) => x.active).length,
    };
    const hasAny = counts.c || counts.a || counts.r;
    const T = S.todayIso();
    const nDue = st.assignments.filter((a) => a.status === 'pending' && a.dueAt).length;
    const phases = [t('gen_status_1'), t('gen_status_2', { n: nDue }), t('gen_status_3'), t('gen_status_4', { date: t('today') })];

    const IMPORTS = [
      { k: 'canvas', icon: IC.GraduationCap, title: 'ob_import_canvas', sub: 'ob_import_canvas_sub' },
      { k: 'ics', icon: IC.CalendarPlus, title: 'ob_import_ics', sub: 'ob_import_ics_sub' },
      { k: 'shot', icon: IC.Image, title: 'ob_import_shot', sub: 'ob_import_shot_sub' },
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
                {IMPORTS.map(({ k, icon: Icon, title, sub }) => (
                  <UI.GlassCard key={k} padding="none" className="dc-ob-import-card" onClick={() => fakeImport(k)}>
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
      </div>
    );
  }

  window.DcPages = Object.assign(window.DcPages || {}, { Onboarding });
})();
