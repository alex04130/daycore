import { useCallback, useEffect, useState } from 'react';
import * as api from './api.js';

// Shared pieces every section needs, so that eight screens agree about what a
// loading state, an empty table and a refused write look like.

// useSection is the load-refetch-error cycle, written once.
//
// # Why the three failure kinds are distinguished here and not per screen
//
// A console has three completely different bad states and they need three
// different answers:
//
//   Unauthorized  the credential expired → the whole shell goes back to login
//   Degraded      storage is unavailable → THIS screen may still be useful
//   anything else → show what the server said
//
// Collapsing them is how a console ends up showing an empty table when the
// honest answer was "your session ended" — the failure mode that makes somebody
// think their data is gone.
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

// Screen wraps a section: title, the three bad states, and the content.
export function Screen({ title, sub, state, children, actions }) {
  return (
    <section className="screen">
      <header className="screen-head">
        <div>
          <h1>{title}</h1>
          {sub && <p className="sub">{sub}</p>}
        </div>
        {actions}
      </header>
      {state.status === 'loading' && <div className="placeholder">读取中…</div>}
      {state.status === 'degraded' && (
        <Notice kind="warn" title="存储不可用">
          这个部署是降级启动的：管理面在服务，其它端点一律 503。这一屏需要数据库，所以现在读不到。
          修好配置后重启进程即可 —— 降级是启动时决定的，不会自己恢复。
        </Notice>
      )}
      {state.status === 'error' && (
        <Notice kind="error" title="读取失败">
          {state.error}
        </Notice>
      )}
      {(state.status === 'ok' || state.status === 'refreshing') && children}
    </section>
  );
}

export function Notice({ kind = 'info', title, children }) {
  return (
    <div className={`notice ${kind}`}>
      {title && <strong>{title}</strong>}
      <div>{children}</div>
    </div>
  );
}

export function Empty({ children }) {
  return <div className="placeholder empty">{children}</div>;
}

// Field is one labelled value, with an optional explanation.
//
// `why` carries the reason a value is what it is — most usefully, why something
// is read-only. A greyed-out control with no explanation reads as a bug, and
// this console has a lot of deliberately read-only things.
export function Field({ label, why, children }) {
  return (
    <div className="field">
      <div className="field-label">
        {label}
        {why && (
          <span className="why" title={why}>
            ?
          </span>
        )}
      </div>
      <div className="field-value">{children}</div>
    </div>
  );
}

// Secret renders the only two things ever said about a credential.
//
// There is no third state and no masked value. A masked secret still tells you
// how long it is and whether it changed between two reads, and the console
// needs neither to do its job — so the API never sends it and this component
// could not render it if it wanted to.
export function Secret({ set, env }) {
  return (
    <span className={`secret ${set ? 'on' : 'off'}`}>
      {set ? '已配置' : '未配置'}
      {env && <code className="env">{env}</code>}
    </span>
  );
}

// Confirm is a destructive action that requires typing the target's name.
//
// Not a second click: a second click is muscle memory, and the actions this
// guards (deleting a user, dropping a row) have no undo. Typing the name is the
// cheapest gate that requires reading what is about to happen.
export function Confirm({ word, label, danger, onConfirm }) {
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState('');
  if (!open) {
    return (
      <button className="danger" onClick={() => setOpen(true)}>
        {label}
      </button>
    );
  }
  return (
    <div className="confirm">
      <div>{danger}</div>
      <label>
        输入 <code>{word}</code> 确认
        <input value={typed} onChange={(e) => setTyped(e.target.value)} autoFocus />
      </label>
      <div className="confirm-actions">
        <button
          className="danger"
          disabled={typed !== word}
          onClick={() => {
            setOpen(false);
            setTyped('');
            onConfirm();
          }}
        >
          {label}
        </button>
        <button
          className="linkish"
          onClick={() => {
            setOpen(false);
            setTyped('');
          }}
        >
          取消
        </button>
      </div>
    </div>
  );
}

// Copyable is a value whose whole purpose is to be pasted somewhere else — the
// OAuth callback URL above all, where a mismatch is the most common setup
// failure and the vendor's error message says nothing useful.
export function Copyable({ value }) {
  const [done, setDone] = useState(false);
  return (
    <span className="copyable">
      <code>{value}</code>
      <button
        className="linkish"
        onClick={() => {
          navigator.clipboard?.writeText(value).then(
            () => {
              setDone(true);
              setTimeout(() => setDone(false), 1200);
            },
            () => {},
          );
        }}
      >
        {done ? '已复制' : '复制'}
      </button>
    </span>
  );
}
