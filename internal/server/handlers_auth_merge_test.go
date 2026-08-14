package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"daycore/internal/auth"
	"daycore/internal/config"
	"daycore/internal/domain"
	"daycore/internal/storage"
	_ "daycore/internal/storage/sqlstore"
)

// Signing in on a second device must fold the anonymous session's data into the
// canonical session. It did not: mergeSessionData copied a day_plan/rule/mood/
// import with the SOURCE row's id — but id is the PRIMARY KEY and that row still
// lives under the anonymous session, so every INSERT collided and the error was
// swallowed by the best-effort merge. The sign-in silently dropped the user's
// plan. The fix mints a fresh id (exactly like the materials/wishes/themes
// branches already did); this test fails on the old code because the canonical
// session ends up empty.
func TestMergeSessionDataCopiesAnonymousDataWithFreshIDs(t *testing.T) {
	store, err := storage.Open("sqlite", "file:"+t.TempDir()+"/merge.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}

	const anon, canon = "anon-sid", "canon-sid"
	ctx := context.Background()
	if _, err := store.Sessions().GetOrCreate(ctx, anon); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Sessions().GetOrCreate(ctx, canon); err != nil {
		t.Fatal(err)
	}

	// Seed the anonymous session with one of each merged entity.
	if _, err := store.DayPlans().Upsert(ctx, &domain.DayPlan{
		SessionID:  anon,
		Date:       "2026-08-14",
		SourceType: "text",
		Blocks:     []domain.TimeBlock{{Title: "写作业", Type: domain.BlockTask, Origin: domain.OriginManual}},
	}); err != nil {
		t.Fatal(err)
	}
	anonPlans, _ := store.DayPlans().Range(ctx, anon, "1970-01-01", "2099-12-31")
	if len(anonPlans) != 1 || anonPlans[0].ID == "" {
		t.Fatalf("seed day_plan missing or id empty: %+v", anonPlans)
	}

	if _, err := store.Rules().Create(ctx, &domain.ScheduleRule{
		SessionID: anon, Title: "数学", Type: domain.BlockTask, Kind: "recurring", Freq: "daily",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Moods().Create(ctx, &domain.MoodCheckin{SessionID: anon, Mood: "calm"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Memory().AddImport(ctx, &domain.ImportRecord{SessionID: anon, Source: "ics", Items: 3, Summary: "三节课"}); err != nil {
		t.Fatal(err)
	}

	s := New(Deps{
		Config: &config.Config{},
		Store:  store,
		Logger: slog.New(slog.NewTextHandler(new(strings.Builder), nil)),
	})
	if err := s.mergeSessionData(ctx, anon, canon); err != nil {
		t.Fatalf("mergeSessionData: %v", err)
	}

	// day_plans: the canonical session must now own a copy, with a fresh id.
	canonPlans, _ := store.DayPlans().Range(ctx, canon, "1970-01-01", "2099-12-31")
	if len(canonPlans) != 1 {
		t.Fatalf("canonical session has %d day_plans after merge, want 1", len(canonPlans))
	}
	if canonPlans[0].ID == anonPlans[0].ID {
		t.Errorf("merged day_plan kept the anonymous id %q; a fresh id must be minted", canonPlans[0].ID)
	}
	if len(canonPlans[0].Blocks) != 1 || canonPlans[0].Blocks[0].Title != "写作业" {
		t.Errorf("merged day_plan blocks wrong: %+v", canonPlans[0].Blocks)
	}

	if rules, _ := store.Rules().List(ctx, canon); len(rules) != 1 {
		t.Errorf("canonical session has %d rules after merge, want 1", len(rules))
	}
	if moods, _ := store.Moods().List(ctx, canon, 200); len(moods) != 1 {
		t.Errorf("canonical session has %d moods after merge, want 1", len(moods))
	}
	if imports, _ := store.Memory().ListImports(ctx, canon, 200); len(imports) != 1 {
		t.Errorf("canonical session has %d imports after merge, want 1", len(imports))
	}
}

// A login that presents a *different* anonymous session must fold it into the
// user's canonical data session. It did not: issueAndLink read its session via
// sessionIDFrom, which prefers the canonical data session (ctxDataSessionID) once
// the dc_auth cookie has already authenticated the user — so a user signing in
// on a second device saw "sid == dataSession" and silently skipped the merge.
// The fix reads the raw anonymous session (ctxSessionID) instead. This test
// simulates the authenticated-login context and asserts the anon data lands in
// the canonical session.
func TestIssueAndLinkMergesAnonymousSessionWhenAlreadyAuthenticated(t *testing.T) {
	store, err := storage.Open("sqlite", "file:"+t.TempDir()+"/merge-link.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	const anon, canon = "anon-sid", "canon-sid"
	if _, err := store.Sessions().GetOrCreate(ctx, anon); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Sessions().GetOrCreate(ctx, canon); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DayPlans().Upsert(ctx, &domain.DayPlan{
		SessionID: anon, Date: "2026-08-15", SourceType: "text",
		Blocks: []domain.TimeBlock{{Title: "写作业", Type: domain.BlockTask, Origin: domain.OriginManual}},
	}); err != nil {
		t.Fatal(err)
	}

	email := "merge-link@daycore.local"
	user, err := store.Users().Upsert(ctx, &domain.User{Email: &email})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Users().SetDataSession(ctx, user.ID, canon); err != nil {
		t.Fatal(err)
	}
	user.DataSessionID = canon

	s := New(Deps{
		Config:  &config.Config{},
		Store:   store,
		Logger:  slog.New(slog.NewTextHandler(new(strings.Builder), nil)),
		Tokens:  auth.NewTokenIssuer("merge-test-secret", time.Hour),
		Cookies: auth.NewCookieSigner("merge-test-cookie-secret"),
	})

	// The authenticated-login request context: dataSessionMW set the canonical
	// data session (the user is already authenticated), while sessionMW resolved
	// the anonymous session from X-Session-Token.
	req := httptest.NewRequest(http.MethodPost, "/api/v2/auth/login", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxSessionID, anon))
	req = req.WithContext(context.WithValue(req.Context(), ctxDataSessionID, canon))
	rec := httptest.NewRecorder()

	if _, err := s.issueAndLink(rec, req, user); err != nil {
		t.Fatalf("issueAndLink: %v", err)
	}

	plans, _ := store.DayPlans().Range(ctx, canon, "1970-01-01", "2099-12-31")
	if len(plans) != 1 {
		t.Fatalf("canonical session has %d day_plans after authenticated login, want 1", len(plans))
	}
	if len(plans[0].Blocks) != 1 || plans[0].Blocks[0].Title != "写作业" {
		t.Errorf("merged block wrong: %+v", plans[0].Blocks)
	}
}
