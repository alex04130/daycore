import { useCallback, useEffect, useMemo, useState } from 'react';
import * as api from '../api.js';
import { Notice, Screen, useSection } from '../ui.jsx';

// The prompt editor.
//
// # Editing a prompt changes what the assistant says to EVERY user
//
// That is why prompts.write is its own permission and not folded into any
// "write" bucket, and it is why this screen is more careful than the others.
// Three things it does that a plain textarea would not:
//
//  1. **Both locales are always in view.** A prompt exists per locale and the
//     pair must stay in step — a rule added to zh-CN and forgotten in en-US is
//     an assistant that behaves differently depending on who is talking to it,
//     and nothing anywhere reports that. The other tab is shown with a marker
//     when it also differs from its default.
//  2. **Placeholders are listed and checked.** The backend renders these as Go
//     templates; a `{{.Weather}}` that becomes `{{.Wether}}` is a render error
//     at request time, which means the feature stops working for everybody at
//     once. Save already refuses a template that will not parse (the server
//     validates), but a placeholder that parses and names nothing is silently
//     empty — so the ones present in the DEFAULT are listed, and any that the
//     draft has dropped are called out before saving.
//  3. **Reverting is one click and says what it restores.**
//
// # Boundary: the L1 hard boundaries are not here and must not be
//
// internal/ai/prompts/boundaries.json is disk + embedded only, with no database
// layer and no endpoint, deliberately: a boundary that can be edited from a
// console is a boundary that can be deleted from a console. It will not appear
// in this list because the API does not serve it. ⚠️ Do not "fix" that.
export function Prompts({ onUnauthorized }) {
  const load = useCallback(() => api.getPrompts(), []);
  const { reload, ...state } = useSection(load, { onUnauthorized });
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
    <Screen
      title="提示词"
      sub="改这里等于改助手对每一个用户说的话。两种语言必须一起改 —— 只改一边，助手就会因为用户说什么语言而行为不同。"
      state={state}
    >
      {state.data && (
        <div className="prompt-layout">
          <div className="prompt-list">
            {keys.map((k) => {
              const variants = byKey.get(k);
              const modified = variants.some((v) => v.hasOverride);
              return (
                <button
                  key={k}
                  className={`prompt-item ${k === active ? 'on' : ''}`}
                  onClick={() => setSelected(k)}
                >
                  <span className="prompt-key">{k}</span>
                  {modified && <span className="pill warn small">已改写</span>}
                </button>
              );
            })}
          </div>
          {active && (
            <Editor
              key={active}
              promptKey={active}
              locales={state.data.locales || []}
              variants={byKey.get(active)}
              onSaved={reload}
              onUnauthorized={onUnauthorized}
            />
          )}
        </div>
      )}
    </Screen>
  );
}

// placeholdersIn finds the {{...}} tokens a template uses.
//
// Deliberately textual rather than a real template parse: the console cannot
// import Go's parser, and the server already refuses a template that will not
// parse. What this catches is the other failure — a template that parses fine
// and quietly renders nothing because a field name was dropped or misspelt.
function placeholdersIn(text) {
  const out = new Set();
  for (const m of String(text || '').matchAll(/\{\{-?\s*([^}]*?)\s*-?\}\}/g)) {
    const body = m[1].trim();
    if (!body || body.startsWith('/*')) continue;
    out.add(body);
  }
  return out;
}

function Editor({ promptKey, locales, variants, onSaved, onUnauthorized }) {
  const [locale, setLocale] = useState(locales[0] || 'zh-CN');
  const [draft, setDraft] = useState('');
  const [loaded, setLoaded] = useState(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');
  const [note, setNote] = useState('');

  useEffect(() => {
    let live = true;
    setErr('');
    setNote('');
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
  const overridden = loaded && loaded.content !== loaded.default;

  // Placeholders the built-in default uses, and which of them the draft has
  // lost. The default is the reference because it is what the feature was
  // written against.
  const expected = useMemo(() => placeholdersIn(loaded?.default), [loaded]);
  const present = useMemo(() => placeholdersIn(draft), [draft]);
  const dropped = [...expected].filter((p) => !present.has(p));
  const added = [...present].filter((p) => !expected.has(p));

  const otherLocales = variants.filter((v) => v.locale !== locale);

  async function save() {
    setBusy(true);
    setErr('');
    setNote('');
    try {
      await api.putPrompt(promptKey, locale, draft);
      setNote('已保存，立即生效。');
      const p = await api.getPrompt(promptKey, locale);
      setLoaded(p);
      setDraft(p.content);
      onSaved();
    } catch (e) {
      setErr(e.message);
    } finally {
      setBusy(false);
    }
  }

  function revert() {
    if (loaded) setDraft(loaded.default);
  }

  return (
    <div className="prompt-editor">
      <div className="prompt-editor-head">
        <code>{promptKey}</code>
        <div className="locale-tabs">
          {locales.map((l) => {
            const v = variants.find((x) => x.locale === l);
            return (
              <button
                key={l}
                className={`chip ${l === locale ? 'on' : ''}`}
                onClick={() => setLocale(l)}
                title={v?.hasOverride ? '这个语言已经被改写过' : '这个语言还是内置默认'}
              >
                {l}
                {v?.hasOverride && <span className="dot-mod" />}
              </button>
            );
          })}
        </div>
      </div>

      {otherLocales.length > 0 && (
        <p className="sub">
          另一种语言：
          {otherLocales.map((v) => (
            <span key={v.locale} className="muted">
              {' '}
              {v.locale} {v.hasOverride ? '已改写' : '仍是默认'}
            </span>
          ))}
          。<strong>两边要一起改</strong> —— 只改一边，助手就会因为用户说什么语言而行为不同，而这件事没有任何东西会报错。
        </p>
      )}

      <textarea
        className="prompt-text"
        spellCheck={false}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
      />

      {expected.size > 0 && (
        <div className="ph-row">
          <span className="muted">默认模板用到的占位符：</span>
          {[...expected].map((p) => (
            <code key={p} className={`chip ${present.has(p) ? '' : 'missing'}`}>
              {'{{' + p + '}}'}
            </code>
          ))}
        </div>
      )}
      {dropped.length > 0 && (
        <Notice kind="warn" title="有占位符被删掉了">
          默认模板里的 <code>{dropped.map((p) => '{{' + p + '}}'). join(' ')}</code> 不在你的版本里。
          模板照样能解析、照样能保存 —— 但那部分数据就再也不会出现在提示词里，而这不会报任何错。
        </Notice>
      )}
      {added.length > 0 && (
        <Notice kind="info" title="新增了占位符">
          <code>{added.map((p) => '{{' + p + '}}'). join(' ')}</code> 不在默认模板里。
          后端只注入它知道的字段，名字对不上就渲染成空。
        </Notice>
      )}

      {err && <Notice kind="error">{err}</Notice>}
      {note && <Notice kind="info">{note}</Notice>}

      <div className="row-actions">
        <button disabled={busy || !dirty} onClick={save}>
          {busy ? '…' : '保存'}
        </button>
        {(dirty || overridden) && (
          <button className="linkish" onClick={revert}>
            填回内置默认
          </button>
        )}
        <span className="muted">
          {draft.length} 字{overridden ? ' · 当前是改写版' : ' · 当前是内置默认'}
        </span>
      </div>
    </div>
  );
}
