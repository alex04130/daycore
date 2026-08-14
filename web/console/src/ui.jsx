import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react';
import * as api from './api.js';
import { AlertTriangle, X } from './icons.jsx';

// Shared pieces the eight screens use. Button/Input/Badge/Chip/Avatar/Seg/Sw
// re-create the DaycoreUI primitives the design prototype uses, with the same
// class names and (recreated) look; AdmConfirm/Drawer/ToastHost mirror
// admin-shell.jsx.

// useSection is the load-refetch-error cycle, written once. The three failure
// kinds are kept distinct: Unauthorized drops the shell back to the gate,
// Degraded is a state THIS screen may still be useful in, and anything else
// shows what the server said.
export function useSection(fetcher, { onUnauthorized } = {}) {
  const [state, setState] = useState({ status: 'loading' });

  const load = useCallback(() => {
    setState((s) => ({ ...s, status: s.data ? 'refreshing' : 'loading' }));
    fetcher()
      .then((data) => setState({ status: 'ok', data }))
      .catch((e) => {
        if (e instanceof api.Unauthorized) {
          onUnauthorized?.();
          return;
        }
        setState({ status: e instanceof api.Degraded ? 'degraded' : 'error', error: e.message });
      });
  }, [fetcher, onUnauthorized]);

  useEffect(load, [load]);
  return { ...state, reload: load };
}

// ── toast ────────────────────────────────────────────────────────────────────
const ToastCtx = createContext(() => {});
export function ToastHost({ children }) {
  const [msg, setMsg] = useState(null);
  const ref = useRef(null);
  const show = (m) => {
    setMsg(m);
    clearTimeout(ref.current);
    ref.current = setTimeout(() => setMsg(null), 2400);
  };
  return (
    <ToastCtx.Provider value={show}>
      {children}
      {msg ? <div className="adm-toast">{msg}</div> : null}
    </ToastCtx.Provider>
  );
}
export const useToast = () => useContext(ToastCtx);

// ── DaycoreUI primitives ─────────────────────────────────────────────────────
export function Button({ variant = 'primary', size = 'md', fullWidth, disabled, onClick, className, children, type = 'button' }) {
  return (
    <button
      type={type}
      className={'dc-btn dc-btn--' + variant + ' dc-btn--' + size + (fullWidth ? ' w-full' : '') + (className ? ' ' + className : '')}
      disabled={disabled}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

export function Input({ type = 'text', placeholder, value, onChange, onKeyDown, inputMode, disabled, autoFocus, spellCheck, className }) {
  return (
    <input
      type={type}
      className={'dc-field dc-input' + (className ? ' ' + className : '')}
      placeholder={placeholder}
      value={value}
      onChange={onChange}
      onKeyDown={onKeyDown}
      inputMode={inputMode}
      disabled={disabled}
      autoFocus={autoFocus}
      spellCheck={spellCheck}
    />
  );
}

export function Badge({ tone = 'warning', children }) {
  const cls = tone && tone !== 'warning' ? ' dc-badge--' + tone : '';
  return <span className={'dc-badge' + cls}>{children}</span>;
}

export function Chip({ variant = 'default', selected, onClick, children }) {
  const on = variant === 'selected' || selected;
  return (
    <button type="button" className={'dc-chip' + (on ? ' dc-chip--selected' : '')} onClick={onClick}>
      {children}
    </button>
  );
}

export function Avatar({ name, size = 30 }) {
  const initial = name ? name.trim().charAt(0).toUpperCase() : '?';
  return (
    <span className="dc-avatar" style={{ width: size, height: size, fontSize: Math.round(size * 0.42) }}>
      {initial}
    </span>
  );
}

export function Seg({ value, onChange, options }) {
  return (
    <div className="adm-seg">
      {options.map((o) => (
        <button key={o.value} type="button" className={value === o.value ? 'is-on' : ''} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function Sw({ on, onClick, label }) {
  return (
    <button
      type="button"
      className={'dc-switch' + (on ? ' is-on' : '')}
      role="switch"
      aria-checked={on}
      aria-label={label}
      onClick={onClick}
    />
  );
}

// ── overlays ─────────────────────────────────────────────────────────────────
export function AdmConfirm({ open, title, desc, confirmLabel, onConfirm, onClose }) {
  if (!open) return null;
  return (
    <div className="adm-modal-scrim" onClick={onClose}>
      <div className="adm-modal" onClick={(e) => e.stopPropagation()}>
        <div className="adm-modal-ic">
          <AlertTriangle size={22} />
        </div>
        <h3>{title}</h3>
        {desc ? <p>{desc}</p> : null}
        <div className="row">
          <Button variant="outline" fullWidth onClick={onClose}>
            取消
          </Button>
          <Button
            variant="primary"
            fullWidth
            className="adm-danger-btn"
            onClick={() => {
              onConfirm();
              onClose();
            }}
          >
            {confirmLabel || '确认'}
          </Button>
        </div>
      </div>
    </div>
  );
}

export function Drawer({ title, onClose, children }) {
  return (
    <>
      <div className="adm-scrim" onClick={onClose} />
      <div className="adm-drawer">
        <div className="adm-drawer-head">
          <h3>{title}</h3>
          <button type="button" className="adm-x" onClick={onClose}>
            <X size={17} />
          </button>
        </div>
        {children}
      </div>
    </>
  );
}

export function Empty({ children }) {
  return <div className="adm-empty">{children}</div>;
}

// Notice is a small inline block for the states the prototype expresses with a
// note or a pill — load errors, "not available", "read-only" explanations.
const NOTICE_TONES = {
  info: { border: 'rgba(167,139,250,.35)', bg: 'rgba(167,139,250,.08)', color: 'var(--color-text-primary)' },
  warn: { border: 'rgba(245,158,11,.4)', bg: 'rgba(245,158,11,.1)', color: 'var(--color-text-primary)' },
  error: { border: 'rgba(239,68,68,.4)', bg: 'rgba(239,68,68,.1)', color: 'var(--color-text-primary)' },
};
export function Notice({ kind = 'info', title, children }) {
  const t = NOTICE_TONES[kind] || NOTICE_TONES.info;
  return (
    <div
      className="adm-note"
      style={{
        border: '1px solid ' + t.border,
        background: t.bg,
        color: t.color,
        padding: '10px 14px',
        borderRadius: '12px',
        margin: '12px 0',
        fontSize: '12.5px',
        lineHeight: 1.6,
      }}
    >
      {title ? <strong style={{ display: 'block', marginBottom: 3 }}>{title}</strong> : null}
      <div>{children}</div>
    </div>
  );
}
