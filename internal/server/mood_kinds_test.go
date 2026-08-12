package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"daycore/internal/config"
	"daycore/internal/domain"
	"daycore/internal/mood"
	"daycore/internal/storage"
	_ "daycore/internal/storage/sqlstore"
)

// A check-in must reach the mood window.
//
// It did not, for the whole life of the shipped frontend. Mood.jsx stored
// `emoji + " " + localizedName` ("😊 开心"); mood.Read resolves each row through
// MoodKindByID and skips what it cannot resolve; so every sample was dropped and
// moodHistoryContext told the companion "no check-ins" no matter how many the
// user had made. Nothing failed — the row saved, the mood page answered, and the
// only visible symptom was an assistant that never mentioned how you had been.
//
// This test is written end-to-end on purpose. A unit test of either half passes:
// the repository stored what it was given, and the window correctly skipped what
// it could not parse. Only the seam was wrong.
func TestMoodCheckinReachesTheWindow(t *testing.T) {
	s, sid := newMoodTestServer(t)
	ctx := context.Background()

	if w := s.moodWindow(ctx, sid); w.Known {
		t.Fatal("window claims to know something before any check-in")
	}

	rec := postMood(t, s, sid, `{"mood":"happy"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("posting a registry id was rejected: %d %s", rec.Code, rec.Body.String())
	}

	w := s.moodWindow(ctx, sid)
	if !w.Known {
		t.Fatal("the window did not see a check-in that was just written — the seam is broken again")
	}
	if w.Samples != 1 {
		t.Errorf("samples = %d, want 1", w.Samples)
	}
	if w.Score <= 0 {
		t.Errorf("score = %v for a positive-valence mood, want > 0", w.Score)
	}
	if w.LastKind != "happy" {
		t.Errorf("last kind = %q, want happy", w.LastKind)
	}
}

// The write side must reject what the window cannot read, so the two can never
// silently disagree again.
func TestMoodCheckinRejectsDisplayStrings(t *testing.T) {
	s, sid := newMoodTestServer(t)
	for _, bad := range []string{
		`{"mood":"😊 开心"}`,    // what the shipped frontend actually sent
		`{"mood":"😊 Happy"}`, // ...and its English form: the same feeling, a different value
		`{"mood":"great"}`,   // the frontend's own id, which the backend never had
		`{"mood":"开心"}`,      // the label without the emoji
	} {
		rec := postMood(t, s, sid, bad)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("accepted %s → %d; it would be stored and then silently skipped", bad, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "unknown_mood") {
			t.Errorf("rejected %s but not with unknown_mood: %s", bad, rec.Body.String())
		}
	}
}

// Whatever GET /api/mood/kinds serves must be exactly what POST accepts. A
// frontend that renders from the first and posts to the second cannot be wrong.
func TestMoodKindsAreExactlyWhatIsAccepted(t *testing.T) {
	s, sid := newMoodTestServer(t)

	req := httptest.NewRequest("GET", versionPath("/api/mood/kinds"), nil)
	req.Header.Set("X-Session-Token", sid)
	rec := httptest.NewRecorder()
	s.handleMoodKinds(rec, req.WithContext(withSession(req.Context(), sid)))
	if rec.Code != http.StatusOK {
		t.Fatalf("kinds: %d %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Kinds []struct {
			ID, Emoji, Name string
			Valence         *int `json:"valence"`
		} `json:"kinds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Kinds) != len(domain.MoodKinds()) {
		t.Fatalf("served %d kinds, registry has %d", len(got.Kinds), len(domain.MoodKinds()))
	}
	for _, k := range got.Kinds {
		if k.ID == "" || k.Emoji == "" || k.Name == "" {
			t.Errorf("incomplete kind: %+v", k)
		}
		if k.Name == k.ID {
			t.Errorf("%q rendered as its own id — the catalog lookup missed", k.ID)
		}
		if k.Valence != nil {
			t.Errorf("%q exposes valence; it is a coarse internal number and showing it invites a score", k.ID)
		}
		if rec := postMood(t, s, sid, `{"mood":"`+k.ID+`"}`); rec.Code != http.StatusOK {
			t.Errorf("kinds served %q but POST rejected it: %d", k.ID, rec.Code)
		}
	}
}

func newMoodTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	store, err := storage.Open("sqlite", "file:"+t.TempDir()+"/mood.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	const sid = "mood-test-sid"
	if _, err := store.Sessions().GetOrCreate(context.Background(), sid); err != nil {
		t.Fatal(err)
	}
	return New(Deps{
		Config: &config.Config{},
		Store:  store,
		Logger: slog.New(slog.NewTextHandler(new(strings.Builder), nil)),
	}), sid
}

func postMood(t *testing.T, s *Server, sid, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", versionPath("/api/mood"), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleMoodCreate(rec, req.WithContext(withSession(req.Context(), sid)))
	return rec
}

func withSession(ctx context.Context, sid string) context.Context {
	return context.WithValue(ctx, ctxSessionID, sid)
}

var _ = mood.TrendUnknown
