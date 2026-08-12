import { useCallback, useEffect, useState } from 'react';
import * as api from '@daycore/core';
import type { Assignment, Boot, Material, MaterialCategory, ScheduleRule } from '@daycore/core';
import { describeRule } from './rules';

// 资料：记下来的东西、还欠着的作业、每周都会发生的事。
//
// ⚠️ The rules section here is the one place this rebuild deliberately throws
// the prototype's largest single piece of logic away rather than porting it.
//
// The prototype's shared mock core had no rule ENTITY, so its bridge layer
// induced rules by scanning 28 days of blocks and grouping by title — plus a
// hard rule that induced ones could never be edited, because "a period guessed
// from existing blocks" expanded back out would conjure seven classes from two
// coincidences.
//
// The real backend has `ScheduleRule`, with recurrence, timezone, tombstones,
// and occurrence expansion already merged into GET /api/plan and /api/plan/range
// (internal/server/handlers_plan.go). Porting the induction would be
// reimplementing a workaround for a limitation that does not exist here — and
// worse, the two would then both expand, so every occurrence appears twice.

// ⚠️ Exported so src/locales.test.ts can check that every tab has a label.
// A `materials.tab.*` key is built from this array, so the pack is only
// verifiable against the array itself.
export const MATERIAL_TABS = ['notes', 'work', 'rules'] as const;
type Tab = (typeof MATERIAL_TABS)[number];

export function PageMaterials({ boot }: { boot: Boot }) {
  const t = boot.catalog.t;
  const locale = boot.catalog.locale;
  const [tab, setTab] = useState<Tab>('notes');
  const [materials, setMaterials] = useState<Material[]>([]);
  const [cats, setCats] = useState<MaterialCategory[]>([]);
  const [cat, setCat] = useState('');
  const [work, setWork] = useState<Assignment[]>([]);
  const [rules, setRules] = useState<ScheduleRule[]>([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const [m, c, a, r] = await Promise.all([
        api.materials(cat),
        api.materialCategories(),
        api.assignments(),
        api.rules(),
      ]);
      setMaterials(m.materials ?? []);
      setCats(c.categories ?? []);
      setWork(a.assignments ?? []);
      setRules(r.rules ?? []);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [cat]);

  useEffect(() => {
    void load();
  }, [load]);

  async function guard(run: () => Promise<unknown>) {
    setBusy(true);
    try {
      await run();
      await load();
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="lc-page">
      <header className="lc-head">
        <h1 className="lc-title">{t('nav.materials')}</h1>
      </header>

      <div className="lc-seg" role="tablist">
        {MATERIAL_TABS.map((id) => (
          <button
            key={id}
            role="tab"
            aria-selected={tab === id}
            className={'lc-segitem' + (tab === id ? ' on' : '')}
            onClick={() => setTab(id)}
          >
            {t(`materials.tab.${id}`)}
          </button>
        ))}
      </div>

      {error && <p className="lc-err">{error}</p>}

      {tab === 'notes' && (
        <>
          {/* ⚠️ The category list comes from the backend, never from a constant
              here. An operator's registry entry that this build had never heard
              of would otherwise be invisible — and its dot colour too, which is
              the bug the prototype shipped (five categories with bare hex codes
              that did not follow the theme). */}
          <div className="lc-seg">
            <button className={'lc-segitem' + (cat === '' ? ' on' : '')} onClick={() => setCat('')}>
              {t('materials.allCategories')}
            </button>
            {cats
              .filter((c) => c.enabled)
              .map((c) => (
                <button
                  key={c.id}
                  className={'lc-segitem' + (cat === c.id ? ' on' : '')}
                  onClick={() => setCat(c.id)}
                >
                  {c.name}
                </button>
              ))}
          </div>
          {materials.length === 0 ? (
            <p className="lc-empty">{t('materials.empty')}</p>
          ) : (
            <ul className="lc-list">
              {materials.map((m) => (
                <li key={m.id}>
                  <article className="lc-card">
                    <div className="lc-cardmain">
                      <span className="lc-cardtitle">{m.title}</span>
                    </div>
                    {m.summary && <p className="lc-sub">{m.summary}</p>}
                    <div className="lc-actrow">
                      <button
                        className="lc-btn sec"
                        disabled={busy}
                        onClick={() => void guard(() => api.deleteMaterial(m.id))}
                      >
                        {t('common.delete')}
                      </button>
                    </div>
                  </article>
                </li>
              ))}
            </ul>
          )}
        </>
      )}

      {tab === 'work' && (
        <>
          {work.length === 0 ? (
            <p className="lc-empty">{t('materials.noWork')}</p>
          ) : (
            <ul className="lc-list">
              {work.map((a) => (
                <li key={a.id}>
                  <article className={'lc-card' + (a.status === 'done' ? ' done' : '')}>
                    <div className="lc-cardmain">
                      <span className="lc-cardtitle">{a.title}</span>
                      {a.dueAt && (
                        <span className="lc-time">
                          {new Intl.DateTimeFormat(locale, {
                            month: 'short',
                            day: 'numeric',
                            hour: '2-digit',
                            minute: '2-digit',
                          }).format(new Date(a.dueAt))}
                        </span>
                      )}
                    </div>
                    {/* ⚠️ `planned` is still owed — it means "scheduled into a
                        day", not "finished". Filtering it out of the to-do list
                        is how a reader loses track of work they can still see
                        on 今日. */}
                    {a.status !== 'done' && (
                      <div className="lc-actrow">
                        <button
                          className="lc-btn pri"
                          disabled={busy}
                          onClick={() => void guard(() => api.patchAssignment(a.id, { status: 'done' }))}
                        >
                          {t('materials.markDone')}
                        </button>
                      </div>
                    )}
                  </article>
                </li>
              ))}
            </ul>
          )}
        </>
      )}

      {tab === 'rules' && (
        <>
          {rules.length === 0 ? (
            <p className="lc-empty">{t('materials.noRules')}</p>
          ) : (
            <ul className="lc-list">
              {rules.map((r) => {
                const parts = describeRule(r, locale);
                return (
                  <li key={r.id}>
                    <article className={'lc-card' + (r.active ? '' : ' done')}>
                      <div className="lc-cardmain">
                        <span className="lc-cardtitle">{r.title}</span>
                        {r.time && <span className="lc-time">{r.time}</span>}
                      </div>
                      {/* ⚠️ Parts are rendered as separate spans, never joined
                          with a string here. Concatenating them would put a
                          separator in the source — and the right separator is a
                          per-language decision that belongs in the pack. */}
                      <p className="lc-sub">
                        {parts.map((p) => (
                          <span key={p.key} className="lc-part">
                            {t(p.key, p.vars)}
                          </span>
                        ))}
                      </p>
                      <div className="lc-actrow">
                        <button
                          className="lc-btn sec"
                          disabled={busy}
                          onClick={() => void guard(() => api.patchRule(r.id, { active: !r.active }))}
                        >
                          {t(r.active ? 'rules.pause' : 'rules.resume')}
                        </button>
                        <button
                          className="lc-btn sec"
                          disabled={busy}
                          onClick={() => void guard(() => api.deleteRule(r.id))}
                        >
                          {t('common.delete')}
                        </button>
                      </div>
                    </article>
                  </li>
                );
              })}
            </ul>
          )}
          {/* Stated on the screen rather than only in a README, because the
              reader is standing exactly where the missing thing would be. */}
          <p className="lc-sub">{t('materials.importNote')}</p>
        </>
      )}
    </div>
  );
}
