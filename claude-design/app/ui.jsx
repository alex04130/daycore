// Daycore v2 — shared UI: hooks, toast, sheets, calendar, overlays
(function () {
  'use strict';
  const { useState, useEffect, useRef, useCallback, useMemo, createContext, useContext } = React;
  const UI = window.DaycoreUI;
  const IC = window.DcIcons;
  const S = window.DcStore;
  const { t } = window.I18N;

  // ---------- hooks ----------
  function useStore() {
    const [, force] = useState(0);
    useEffect(() => S.subscribe(() => force((x) => x + 1)), []);
    return S.state;
  }
  function useLang() {
    const [lang, setLang] = useState(window.I18N.lang);
    useEffect(() => window.I18N.subscribe(setLang), []);
    return lang;
  }
  function useMedia(q) {
    const [m, setM] = useState(() => window.matchMedia(q).matches);
    useEffect(() => {
      const mq = window.matchMedia(q);
      const fn = () => setM(mq.matches);
      mq.addEventListener('change', fn);
      return () => mq.removeEventListener('change', fn);
    }, [q]);
    return m;
  }
  const useDesktop = () => useMedia('(min-width: 900px)');

  // ---------- theme application (handoff 增量01 §3) ----------
  const VAR_MAP = {
    '--primary': ['--color-primary'], '--accent': ['--color-accent'],
    '--bg-start': ['--color-bg-start'], '--bg-end': ['--color-bg-end'],
    '--text-primary': ['--color-text-primary'], '--text-secondary': ['--color-text-secondary'],
    '--text-muted': ['--color-text-muted'],
    '--surface': ['--color-surface'], '--surface-hover': ['--color-surface-hover'],
    '--success': ['--color-states-success'], '--warning': ['--color-states-warning'], '--error': ['--color-states-error'],
  };
  const ALL_MAPPED = Object.values(VAR_MAP).flat().concat(['--color-border-custom', '--tab-bar-bg', '--tab-bar-border']);
  function applyThemeVars(themeObj) {
    const root = document.documentElement;
    ALL_MAPPED.forEach((k) => root.style.removeProperty(k));
    if (!themeObj) return; // builtin — data-theme handles everything
    root.dataset.theme = themeObj.dark ? 'night' : (themeObj.base || 'sky');
    for (const [k, v] of Object.entries(themeObj.variables || {})) {
      (VAR_MAP[k] || []).forEach((real) => root.style.setProperty(real, v));
    }
    const p = themeObj.variables['--primary'];
    if (p) {
      root.style.setProperty('--color-border-custom', 'color-mix(in srgb, ' + p + ' 22%, transparent)');
      root.style.setProperty('--tab-bar-border', 'color-mix(in srgb, ' + p + ' 16%, transparent)');
    }
    const bg = themeObj.variables['--bg-start'];
    if (bg) root.style.setProperty('--tab-bar-bg', 'color-mix(in srgb, ' + bg + ' 86%, transparent)');
  }
  function applyCurrentTheme(state) {
    const cur = state.session.currentTheme;
    const builtin = ['sky', 'sunset', 'night', 'nature'];
    const root = document.documentElement;
    if (builtin.includes(cur)) { root.dataset.theme = cur; applyThemeVars(null); }
    else {
      const th = state.customThemes.find((x) => x.id === cur);
      if (th) applyThemeVars(th); else { root.dataset.theme = 'sky'; applyThemeVars(null); }
    }
  }

  // ---------- toast ----------
  const ToastCtx = createContext(null);
  function ToastProvider({ children }) {
    const [toasts, setToasts] = useState([]);
    const push = useCallback((msg, opts) => {
      const id = Math.random().toString(36).slice(2);
      setToasts((ts) => [...ts.slice(-2), { id, msg, action: opts && opts.action, onAction: opts && opts.onAction }]);
      setTimeout(() => setToasts((ts) => ts.filter((x) => x.id !== id)), opts && opts.duration || 3200);
    }, []);
    return (
      <ToastCtx.Provider value={push}>
        {children}
        <div className="dc-toast-wrap" aria-live="polite">
          {toasts.map((x) => (
            <div key={x.id} className="dc-toast">
              <span>{x.msg}</span>
              {x.action ? <button className="dc-toast-action" onClick={() => { x.onAction && x.onAction(); setToasts((ts) => ts.filter((y) => y.id !== x.id)); }}>{x.action}</button> : null}
            </div>
          ))}
        </div>
      </ToastCtx.Provider>
    );
  }
  const useToast = () => useContext(ToastCtx);

  // ---------- Sheet (BottomSheet on mobile; centered dialog on desktop via CSS) ----------
  function Sheet({ open, onClose, title, children, wide }) {
    return (
      <UI.BottomSheet open={open} onClose={onClose} title={title} className={'dc-app-sheet' + (wide ? ' dc-app-sheet--wide' : '')}>
        {children}
      </UI.BottomSheet>
    );
  }

  // ---------- Confirm ----------
  function Confirm({ open, onClose, title, body, confirmLabel, danger, onConfirm }) {
    return (
      <Sheet open={open} onClose={onClose} title={title}>
        {body ? <p className="dc-confirm-body">{body}</p> : null}
        <div className="dc-sheet-actions">
          <UI.Button variant="outline" fullWidth onClick={onClose}>{t('cancel')}</UI.Button>
          <UI.Button variant="primary" fullWidth className={danger ? 'dc-btn-danger' : ''} onClick={() => { onConfirm(); onClose(); }}>
            {confirmLabel || t('confirm')}
          </UI.Button>
        </div>
      </Sheet>
    );
  }

  // ---------- Segmented ----------
  function Segmented({ options, value, onChange, small }) {
    return (
      <div className={'dc-seg' + (small ? ' dc-seg--sm' : '')} role="radiogroup">
        {options.map((o) => (
          <button key={o.value} role="radio" aria-checked={value === o.value}
            className={'dc-seg-item' + (value === o.value ? ' is-on' : '')}
            onClick={() => onChange(o.value)}>
            {o.label}
          </button>
        ))}
      </div>
    );
  }

  // ---------- Field row (label + control) ----------
  function Field({ label, sub, children, inline }) {
    return (
      <div className={'dc-field-row' + (inline ? ' dc-field-row--inline' : '')}>
        <div className="dc-field-label">
          <span>{label}</span>
          {sub ? <span className="dc-field-sub">{sub}</span> : null}
        </div>
        {children}
      </div>
    );
  }

  // ---------- Calendar (month grid; single or range select) ----------
  function Calendar({ month, onMonth, value, range, onPick, min, max, badges }) {
    // month: 'YYYY-MM'; value: dateStr | null; range: {from,to} | null
    const [y, m] = month.split('-').map(Number);
    const first = new Date(y, m - 1, 1);
    const startPad = first.getDay(); // Sunday-led like by_weekday
    const daysIn = new Date(y, m, 0).getDate();
    const cells = [];
    for (let i = 0; i < startPad; i++) cells.push(null);
    for (let d = 1; d <= daysIn; d++) cells.push(y + '-' + String(m).padStart(2, '0') + '-' + String(d).padStart(2, '0'));
    const wds = window.I18N.weekdayNames('narrow');
    const T = S.todayIso();
    const inRange = (d) => range && range.from && range.to && d >= range.from && d <= range.to;
    const prevM = () => { const dt = new Date(y, m - 2, 1); onMonth(dt.getFullYear() + '-' + String(dt.getMonth() + 1).padStart(2, '0')); };
    const nextM = () => { const dt = new Date(y, m, 1); onMonth(dt.getFullYear() + '-' + String(dt.getMonth() + 1).padStart(2, '0')); };
    return (
      <div className="dc-cal">
        <div className="dc-cal-head">
          <UI.IconButton aria-label="prev" onClick={prevM} icon={<IC.ChevronLeft size={18} />} />
          <span className="dc-cal-title">{window.I18N.fmtDate(month + '-01', 'md').replace(/日|1(?=,|\s|$)/, '').replace(/\s?1$/, '') || month}</span>
          <UI.IconButton aria-label="next" onClick={nextM} icon={<IC.ChevronRight size={18} />} />
        </div>
        <div className="dc-cal-grid dc-cal-wd">{wds.map((w, i) => <span key={i}>{w}</span>)}</div>
        <div className="dc-cal-grid">
          {cells.map((d, i) => {
            if (!d) return <span key={'e' + i} />;
            const disabled = (min && d < min) || (max && d > max);
            const isSel = value === d || (range && (range.from === d || range.to === d));
            const cls = 'dc-cal-day' + (isSel ? ' is-sel' : '') + (inRange(d) && !isSel ? ' is-range' : '') + (d === T ? ' is-today' : '') + (disabled ? ' is-off' : '');
            const badge = badges && badges[d];
            return (
              <button key={d} className={cls} disabled={disabled} onClick={() => onPick(d)}>
                <span>{Number(d.split('-')[2])}</span>
                {badge ? <i className="dc-cal-dot" style={badge.color ? { background: badge.color } : null} /> : null}
              </button>
            );
          })}
        </div>
      </div>
    );
  }

  // ---------- WeekStrip (7-day pager + jump) ----------
  function WeekStrip({ date, onDate, onJump }) {
    const T = S.todayIso();
    const wd0 = new Date(date + 'T12:00:00');
    const start = S.addDays(date, -((wd0.getDay() + 6) % 7)); // Monday-led display
    const days = Array.from({ length: 7 }, (_, i) => S.addDays(start, i));
    const wds = window.I18N.weekdayNames('narrow');
    return (
      <div className="dc-weekstrip">
        <button className="dc-week-nav" aria-label="prev week" onClick={() => onDate(S.addDays(date, -7))}><IC.ChevronLeft size={16} /></button>
        <div className="dc-week-days">
          {days.map((d) => {
            const p = S.getPlan(d);
            const has = p && p.blocks.length > 0;
            const done = has && p.blocks.every((b) => b.completed);
            return (
              <button key={d} className={'dc-week-day' + (d === date ? ' is-sel' : '') + (d === T ? ' is-today' : '')} onClick={() => onDate(d)}>
                <span className="dc-week-wd">{wds[S.weekdayOf(d)]}</span>
                <span className="dc-week-num">{Number(d.split('-')[2])}</span>
                <i className={'dc-week-dot' + (has ? ' has' : '') + (done ? ' done' : '')} />
              </button>
            );
          })}
        </div>
        <button className="dc-week-nav" aria-label="next week" onClick={() => onDate(S.addDays(date, 7))}><IC.ChevronRight size={16} /></button>
        <button className="dc-week-nav dc-week-cal" aria-label={t('month_pick_title')} onClick={onJump}><IC.CalendarDays size={16} /></button>
      </div>
    );
  }

  // ---------- Generating overlay (breathing) ----------
  function GenOverlay({ open, phases, done, onDone }) {
    const [idx, setIdx] = useState(0);
    useEffect(() => {
      if (!open) { setIdx(0); return; }
      const timer = setInterval(() => setIdx((i) => Math.min(i + 1, phases.length - 1)), 950);
      return () => clearInterval(timer);
    }, [open, phases.length]);
    useEffect(() => {
      if (open && done) { const tm = setTimeout(onDone, 650); return () => clearTimeout(tm); }
    }, [open, done]);
    if (!open) return null;
    return (
      <div className="dc-gen-overlay" role="status">
        <div className={'dc-gen-orb' + (done ? ' is-done' : '')}>
          {done ? <IC.Check size={38} strokeWidth={2.5} /> : <IC.Sparkles size={34} />}
        </div>
        <p className="dc-gen-text" key={done ? 'done' : idx}>{done ? t('gen_done') : phases[idx]}</p>
        <div className="dc-gen-dots">{phases.map((_, i) => <i key={i} className={i <= idx || done ? 'on' : ''} />)}</div>
      </div>
    );
  }

  // ---------- type meta ----------
  const TYPE_META = {
    task: { icon: IC.Clock, dot: 'var(--color-primary)' },
    appointment: { icon: IC.Pin, dot: '#f97316' },
    break: { icon: IC.Coffee, dot: '#22c55e' },
    relax: { icon: IC.Leaf, dot: '#a78bfa' },
    meal: { icon: IC.Utensils, dot: '#f59e0b' },
  };
  const typeLabel = (type) => t('type_' + type);

  // ---------- weekday multiselect ----------
  function WeekdayPicker({ value, onChange }) {
    const wds = window.I18N.weekdayNames('narrow');
    const order = [1, 2, 3, 4, 5, 6, 0]; // display Mon-led
    return (
      <div className="dc-wd-picker">
        {order.map((i) => (
          <button key={i} className={'dc-wd-chip' + (value.includes(i) ? ' is-on' : '')}
            onClick={() => onChange(value.includes(i) ? value.filter((x) => x !== i) : [...value, i])}>
            {wds[i]}
          </button>
        ))}
      </div>
    );
  }

  window.DcUI = {
    useStore, useLang, useMedia, useDesktop,
    applyCurrentTheme, applyThemeVars,
    ToastProvider, useToast,
    Sheet, Confirm, Segmented, Field, Calendar, WeekStrip, GenOverlay, WeekdayPicker,
    TYPE_META, typeLabel,
  };
})();
