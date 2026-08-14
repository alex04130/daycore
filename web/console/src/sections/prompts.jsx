import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import * as api from '../api.js';
import * as I from '../icons.jsx';
import { Button, Empty, Notice, Seg, useSection, useToast } from '../ui.jsx';

// The prompt editor, in the prototype's two-pane form. Two things the real
// backend does NOT provide are handled honestly rather than faked:
//
//   - name / description per key — the API returns key/locale/content only, so
//     the list shows the key itself.
//   - a curated placeholder list — the console derives the field placeholders
//     ({{.Field}}) from the built-in default template, which is the reference
//     the feature was written against.
export function Prompts({ onUnauthorized }) {
  const load = useCallback(() => api.getPrompts(), []);
  const state = useSection(load, { onUnauthorized });
  const [selected, setSelected] = useState(null);

  const byKey = useMemo(() => {
    const m = new Map();
    for (const p of state.data?.prompts || []) {
      if (!m.has(p.key)) m.set(p.key, []);
      m.get(p.key).push(p);
    }
    return m;
  }, [state.data]);

  const keys = [...byKey.keys()];
  const active = selected && byKey.has(selected) ? selected : keys[0];

  return (
    <div data-screen-label="Prompt 管理">
      <header className="adm-head">
        <div>
          <h1 className="adm-title">Prompt 管理</h1>
          <p className="adm-sub">
            改动保存后立即生效，按 locale 分别存储 · <span className="mono">PUT /api/admin/prompts/{'{key}'}?locale=</span>
          </p>
        </div>
      </header>

      {state.status === 'loading' && <div className="adm-empty">读取中…</div>}
      {state.status === 'error' && <Notice kind="error">{state.error}</Notice>}

      {state.data && (
        <div className="adm-prompts">
          <div className="adm-plist">
            {keys.map((k) => {
              const variants = byKey.get(k);
              const modified = variants.some((v) => v.hasOverride);
              return (
                <button
                  key={k}
                  type="button"
                  className={'adm-pitem' + (k === active ? ' is-active' : '')}
                  onClick={() => setSelected(k)}
                >
                  <span className="nm">
                    {k}
                    {modified ? <i className="mdot" title="已修改"></i> : null}
                  </span>
                </button>
              );
            })}
          </div>
          {active && (
            <PromptEditor
              key={active}
              promptKey={active}
              locales={state.data.locales || []}
              variants={byKey.get(active)}
              onSaved={state.reload}
              onUnauthorized={onUnauthorized}
            />
          )}
        </div>
      )}
    </div>
  );
}

const LOCALE_LABEL = { 'zh-CN': '中文', 'en-US': 'English' };
const localeLabel = (l) => LOCALE_LABEL[l] || l;

// Field placeholders the built-in default uses, e.g. {{.Date}} / {{.Weekday}}.
function placeholdersIn(text) {
  const out = [];
  const seen = new Set();
  for (const m of String(text || '').matchAll(/.[A-Za-z_]w*/g)) {
    if (!seen.has(m[0])) {
      seen.add(m[0]);
      out.push(m[0]);
    }
  }
  return out;
}

function PromptEditor({ promptKey, locales, variants, onSaved, onUnauthorized }) {
  const toast = useToast();
  const [locale, setLocale] = useState(locales[0] || 'zh-CN');
  const [draft, setDraft] = useState('');
  const [loaded, setLoaded] = useState(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');
  const taRef = useRef(null);

  useEffect(() => {
    let live = true;
    setErr('');
    api
      .getPrompt(promptKey, locale)
      .then((p) => {
        if (!live) return;
        setLoaded(p);
        setDraft(p.content);
      })
      .catch((e) => {
        if (e instanceof api.Unauthorized) onUnauthorized?.();
        else if (live) setErr(e.message);
      });
    return () => {
      live = false;
    };
  }, [promptKey, locale, onUnauthorized]);

  const dirty = loaded && draft !== loaded.content;
  const modified = loaded && loaded.content !== loaded.default;
  const placeholders = useMemo(() => placeholdersIn(loaded?.default), [loaded]);

  function insertPh(name) {
    const token = '{{' + name + '}}';
    const ta = taRef.current;
    if (!ta) {
      setDraft(draft + token);
      return;
    }
    const s = ta.selectionStart == null ? draft.length : ta.selectionStart;
    const e2 = ta.selectionEnd == null ? s : ta.selectionEnd;
    setDraft(draft.slice(0, s) + token + draft.slice(e2));
    requestAnimationFrame(() => {
      ta.focus();
      ta.selectionStart = ta.selectionEnd = s + token.length;
    });
  }

  async function save() {
    setBusy(true);
    setErr('');
    try {
      await api.putPrompt(promptKey, locale, draft);
      const p = await api.getPrompt(promptKey, locale);
      setLoaded(p);
      setDraft(p.content);
      onSaved();
      toast('已保存，立即生效');
    } catch (e) {
      if (e instanceof api.Unauthorized) return onUnauthorized?.();
      setErr(e.message);
    } finally {
      setBusy(false);
    }
  }

  function revert() {
    if (loaded) setDraft(loaded.default);
  }

  const localesForSeg = locales.map((l) => ({ value: l, label: localeLabel(l) }));

  return (
    <div className="adm-card adm-editor">
      <div className="adm-card-head" style={{ marginBottom: 6 }}>
        <I.FileText size={16} /> {promptKey}
        <span style={{ marginLeft: 'auto' }}>
          <Seg value={locale} onChange={setLocale} options={localesForSeg} />
        </span>
      </div>
      <p className="adm-sub" style={{ margin: '0 0 12px' }}>
        {modified ? '当前为修改版' : '当前为内置默认'}
      </p>
      <textarea
        ref={taRef}
        spellCheck={false}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
      ></textarea>

      {placeholders.length > 0 && (
        <>
          <p className="adm-ph-label">可用占位符（点击插入，后端运行时注入）</p>
          <div className="adm-ph-row">
            {placeholders.map((name) => (
              <button key={name} type="button" className="adm-ph-chip" title={name} onClick={() => insertPh(name)}>
                {'{{' + name + '}}'}
              </button>
            ))}
          </div>
        </>
      )}

      {err && <Notice kind="error">{err}</Notice>}

      <div className="adm-editor-foot">
        <span>{draft.length} 字 · {'{{占位符}}'} 由后端注入</span>
        <span className="sp"></span>
        {(modified || dirty) && (
          <Button variant="ghost" onClick={revert}>
            还原默认
          </Button>
        )}
        <Button variant="primary" disabled={!dirty || busy} onClick={save}>
          {busy ? '保存中…' : '保存'}
        </Button>
      </div>
    </div>
  );
}
