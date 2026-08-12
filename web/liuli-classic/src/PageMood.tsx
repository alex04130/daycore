import { useCallback, useEffect, useState } from 'react';
import * as api from '@daycore/core';
import type { Boot, MoodCheckin, MoodKind } from '@daycore/core';

// 心情：记一笔，然后被回应一句。
//
// # ⚠️ What this page deliberately does NOT do
//
// The prototype offered a breathing/stretching/grounding exercise automatically
// after a negative check-in, driven by a hard-coded list of which moods are
// "negative".
//
// That list cannot exist here, and the reason is a decision the backend already
// made: `MoodKind.Valence` is `json:"-"`, and the handler says why — "never
// shown to the user … 'your week was -4' is precisely the shame the product's
// 底色 forbids". `/api/ai/mood` returns `{response}` and nothing else, and the
// `exerciseOffered` field on a check-in is whatever the CLIENT wrote there.
//
// So a frontend that decides which moods are bad is reconstructing, on the
// client, a judgement the product refuses to render — with its own worse copy
// of the data. That is the definition of 不做假的.
//
// What is honest instead: the three exercises live here permanently and the
// reader picks one whenever they want. `exerciseOffered` then records what they
// actually chose, which is a true statement about a real event.

// ⚠️ Exported for the same reason as MATERIAL_TABS — see src/locales.test.ts.
export const EXERCISES = ['breathe', 'stretch', 'ground'] as const;

export function PageMood({ boot }: { boot: Boot }) {
  const t = boot.catalog.t;
  const locale = boot.catalog.locale;
  const [kinds, setKinds] = useState<MoodKind[]>([]);
  const [history, setHistory] = useState<MoodCheckin[]>([]);
  const [reply, setReply] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [note, setNote] = useState('');

  const load = useCallback(async () => {
    try {
      const [k, h] = await Promise.all([api.moodKinds(), api.moodHistory(6)]);
      setKinds(k.kinds ?? []);
      // ⚠️ A bare array, unlike every other list endpoint. `?? []` rather than
      // `h.checkins` — which is what the first draft of the client wrapper had,
      // and which would have rendered an empty history forever.
      setHistory(h ?? []);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function record(kind: MoodKind) {
    setBusy(true);
    setReply('');
    // ⚠️ Read the note into a local before any await. The first version cleared
    // the field and then read state inside the callback, which by then was the
    // cleared one — every note silently arrived empty.
    const text = note.trim();
    try {
      // ⚠️ Submit the registry ID, never the emoji or the label. The shipped
      // frontend sent "😊 开心" and the backend could not resolve a single
      // check-in — so every one of them was invisible to the companion, with
      // nothing anywhere reporting it.
      await api.recordMood(kind.id, text);
      setNote('');
      const r = await api.askMoodReply(kind.id, text);
      setReply(r.response);
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
        <h1 className="lc-title">{t('nav.mood')}</h1>
      </header>

      {error && <p className="lc-err">{error}</p>}

      {/* ⚠️ The grid is rendered from the backend's registry, not from a list
          here. The shipped frontend kept its own and the two drifted into
          different vocabularies — four ids named differently for the same
          emoji, and four that existed on only one side. */}
      <div className="lc-moods">
        {kinds.map((k) => (
          <button key={k.id} className="lc-mood" disabled={busy} onClick={() => void record(k)}>
            <span className="lc-moodemoji" aria-hidden="true">{k.emoji}</span>
            <span className="lc-moodname">{k.name}</span>
          </button>
        ))}
      </div>

      <input
        className="lc-input"
        value={note}
        onChange={(e) => setNote(e.target.value)}
        placeholder={t('mood.notePlaceholder')}
        aria-label={t('mood.notePlaceholder')}
      />

      {reply && (
        <article className="lc-card">
          <p>{reply}</p>
        </article>
      )}

      <section className="lc-card">
        <h2 className="lc-cardtitle">{t('mood.exercises')}</h2>
        <p className="lc-sub">{t('mood.exercisesNote')}</p>
        <div className="lc-actrow">
          {EXERCISES.map((id) => (
            <button key={id} className="lc-btn sec">
              {t(`mood.exercise.${id}`)}
            </button>
          ))}
        </div>
      </section>

      {history.length > 0 && (
        <ul className="lc-list">
          {history.map((h) => (
            <li key={h.id}>
              <article className="lc-card">
                <div className="lc-cardmain">
                  <span className="lc-cardtitle">{kinds.find((k) => k.id === h.mood)?.name ?? h.mood}</span>
                  <span className="lc-time">
                    {new Intl.DateTimeFormat(locale, { month: 'short', day: 'numeric' }).format(
                      new Date(h.createdAt),
                    )}
                  </span>
                </div>
                {h.note && <p className="lc-sub">{h.note}</p>}
              </article>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
