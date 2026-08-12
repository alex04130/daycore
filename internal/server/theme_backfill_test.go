package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"daycore/internal/ai"
	"daycore/internal/config"
	"daycore/internal/domain"
)

// backfillServer is a server whose model answers from a script, so the sweep
// can be driven end to end.
func backfillServer(t *testing.T, script []string) (*Server, *int) {
	t.Helper()
	base, _ := newAgentTestServer(t)
	srv := fakeOpenAI(t, script)
	calls := 0
	counting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Redirect(w, r, srv.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(counting.Close)

	path := t.TempDir() + "/models.yaml"
	yaml := "models:\n  - id: fake\n    format: openai\n    base_url: " + counting.URL + "\n    model: fake\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := ai.LoadCatalog(path, "fake", "", "")
	if err != nil {
		t.Fatal(err)
	}
	prompts, err := ai.NewPromptService(base.store.Prompts())
	if err != nil {
		t.Fatal(err)
	}
	s := New(Deps{
		Config:  &config.Config{AgentMaxRounds: 3, AIRequestTimeout: 10 * time.Second},
		Store:   base.store,
		Catalog: cat,
		Prompts: prompts,
		Logger:  base.log,
	})
	return s, &calls
}

// A family with a widened token space, a theme missing one of the new tokens,
// and an operator who asked.
func seedBackfill(t *testing.T, s *Server, missingKind string) (domain.FrontendFamily, domain.CustomTheme) {
	t.Helper()
	ctx := context.Background()
	fam := domain.FrontendFamily{ID: "liuli", Tokens: []domain.TokenSpec{
		{Name: "--primary", Kind: "color"},
		{Name: "--glass-alpha", Kind: missingKind},
	}}
	if err := s.store.Frontends().UpsertFamily(ctx, fam); err != nil {
		t.Fatal(err)
	}
	th, err := s.store.Themes().Create(ctx, &domain.CustomTheme{
		SessionID: "sid1", FamilyID: "liuli", Name: "夜", Dark: true,
		Variables: map[string]string{"--primary": "#a78bfa"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	return *got, *th
}

func request(t *testing.T, s *Server, famID string) {
	t.Helper()
	ctx := context.Background()
	fam, err := s.store.Frontends().GetFamily(ctx, famID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	fam.BackfillRequestedAt = &now
	if err := s.store.Frontends().UpsertFamily(ctx, *fam); err != nil {
		t.Fatal(err)
	}
}

// The sweep fills what is missing, keeps what is there, and stops asking.
func TestABackfillFillsTheGapAndThenStops(t *testing.T) {
	s, calls := backfillServer(t, []string{`"{\"variables\":{\"--glass-alpha\":\"0.72\"}}"`})
	_, th := seedBackfill(t, s, "ratio")
	request(t, s, "liuli")
	ctx := context.Background()

	s.sweepThemeBackfills(ctx)

	got, err := s.store.Themes().Get(ctx, "sid1", th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Variables["--glass-alpha"] != "0.72" {
		t.Errorf("the missing token was not filled: %v", got.Variables)
	}
	// ⚠️ MERGED, never replaced. A backfill that overwrote a palette somebody
	// chose would be silent and unundoable.
	if got.Variables["--primary"] != "#a78bfa" {
		t.Errorf("the backfill changed a value the user chose: %v", got.Variables)
	}

	// A second sweep must not pay again — the family's request is cleared once
	// a full pass finds nothing left.
	fam, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	if fam.BackfillRequestedAt != nil {
		t.Error("the request is still armed after everything was filled; the sweep will keep scanning forever")
	}
	before := *calls
	s.sweepThemeBackfills(ctx)
	if *calls != before {
		t.Errorf("a finished backfill called the model again (%d → %d)", before, *calls)
	}
}

// ⚠️ Nothing happens without a request. The widening that makes themes
// incomplete is a routine deploy; spending on it automatically would be
// spending because somebody shipped a frontend.
func TestNothingIsSpentWithoutAnOperatorAsking(t *testing.T) {
	s, calls := backfillServer(t, []string{`"{\"variables\":{\"--glass-alpha\":\"0.72\"}}"`})
	_, th := seedBackfill(t, s, "ratio")
	ctx := context.Background()

	s.sweepThemeBackfills(ctx)
	if *calls != 0 {
		t.Errorf("the sweep spent %d model calls nobody asked for", *calls)
	}
	got, _ := s.store.Themes().Get(ctx, "sid1", th.ID)
	if _, filled := got.Variables["--glass-alpha"]; filled {
		t.Error("a theme was backfilled with no request")
	}
}

// A value the model returns that does not fit its token's kind is dropped, not
// stored — the same floor every other write passes.
func TestAModelValueThatDoesNotFitItsKindIsNotStored(t *testing.T) {
	s, _ := backfillServer(t, []string{
		`"{\"variables\":{\"--glass-alpha\":\"rgba(255,255,255,0.7)\"}}"`, // a colour, for a ratio
	})
	_, th := seedBackfill(t, s, "ratio")
	request(t, s, "liuli")
	ctx := context.Background()

	s.sweepThemeBackfills(ctx)

	got, _ := s.store.Themes().Get(ctx, "sid1", th.ID)
	if v, filled := got.Variables["--glass-alpha"]; filled {
		t.Errorf("a value of the wrong kind was stored: %q", v)
	}
	// ⚠️ And the request stays armed, because the theme is still incomplete.
	// Clearing it here would mean the console said "done" about a family that is
	// not done.
	fam, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	if fam.BackfillRequestedAt == nil {
		t.Error("the request was cleared even though the theme is still missing a token")
	}
}

// The occurrence is claimed before the call, so a failing theme stops costing
// money after JobMaxAttempts instead of on every sweep forever.
func TestAThemeThatKeepsFailingStopsCostingMoney(t *testing.T) {
	s, calls := backfillServer(t, []string{`"not json at all"`})
	seedBackfill(t, s, "ratio")
	request(t, s, "liuli")
	ctx := context.Background()

	for i := 0; i < 6; i++ {
		s.sweepThemeBackfills(ctx)
	}
	if *calls > domain.JobMaxAttempts {
		t.Errorf("a permanently failing theme was paid for %d times; the cap is %d",
			*calls, domain.JobMaxAttempts)
	}
	if *calls == 0 {
		t.Fatal("it never tried at all — this test would pass for the wrong reason")
	}
	runs, _ := s.store.JobRuns().List(ctx, "sid1", 10)
	if len(runs) == 0 || runs[0].Job != domain.JobThemeBackfill {
		t.Errorf("nothing recorded the attempts: %+v", runs)
	}
	if runs[0].Status != domain.JobFailed || runs[0].Error == "" {
		t.Errorf("the failure was not recorded with a reason: %+v", runs[0])
	}
}

// Widening the family again is a NEW occurrence, not a collision with the
// finished one — otherwise the second token could never be filled.
func TestWideningAgainIsANewOccurrence(t *testing.T) {
	s, _ := backfillServer(t, []string{
		`"{\"variables\":{\"--glass-alpha\":\"0.72\"}}"`,
		`"{\"variables\":{\"--edge\":\"hairline\"}}"`,
	})
	_, th := seedBackfill(t, s, "ratio")
	request(t, s, "liuli")
	ctx := context.Background()
	s.sweepThemeBackfills(ctx)

	fam, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	fam.Tokens = append(fam.Tokens, domain.TokenSpec{Name: "--edge", Kind: "one-of[hairline, none]"})
	nowAt := time.Now().UTC()
	fam.BackfillRequestedAt = &nowAt
	// UpdatedAt moves on write, which is what makes the run key different.
	if err := s.store.Frontends().UpsertFamily(ctx, *fam); err != nil {
		t.Fatal(err)
	}
	s.sweepThemeBackfills(ctx)

	got, _ := s.store.Themes().Get(ctx, "sid1", th.ID)
	if got.Variables["--edge"] != "hairline" {
		t.Errorf("the second widening was never filled — the run key collided with the finished one: %v", got.Variables)
	}
	if got.Variables["--glass-alpha"] != "0.72" {
		t.Errorf("the first fill was lost: %v", got.Variables)
	}
}

// The price is a read; spending is not.
func TestThePriceIsReportedBeforeAnybodyPresses(t *testing.T) {
	s, calls := backfillServer(t, []string{`"{\"variables\":{}}"`})
	fam, _ := seedBackfill(t, s, "ratio")
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := s.store.Themes().Create(ctx, &domain.CustomTheme{
			SessionID: fmt.Sprintf("sid%d", i+2), FamilyID: "liuli", Name: "t",
			Variables: map[string]string{"--primary": "#fff"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	// One theme that is already complete must not be counted.
	if _, err := s.store.Themes().Create(ctx, &domain.CustomTheme{
		SessionID: "sid9", FamilyID: "liuli", Name: "complete",
		Variables: map[string]string{"--primary": "#fff", "--glass-alpha": "0.5"},
	}); err != nil {
		t.Fatal(err)
	}

	n, capped, err := s.themeBackfillCount(ctx, fam, 500)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("the price is %d themes, want 4 (the complete one must not be counted)", n)
	}
	if capped {
		t.Error("a four-theme family reported as capped")
	}
	if *calls != 0 {
		t.Errorf("asking the price spent %d model calls", *calls)
	}
	// The cap is reported rather than hidden: "500" and "at least 500" are
	// different sentences to somebody deciding whether to press the button.
	if n, capped, _ := s.themeBackfillCount(ctx, fam, 2); n != 2 || !capped {
		t.Errorf("the cap was not reported: n=%d capped=%v", n, capped)
	}
}

// Stopping is a stop, not an undo — and it says so.
func TestStoppingDoesNotUndoWhatWasAlreadyFilled(t *testing.T) {
	s, _ := backfillServer(t, []string{`"{\"variables\":{\"--glass-alpha\":\"0.72\"}}"`})
	_, th := seedBackfill(t, s, "ratio")
	ctx := context.Background()
	// Two themes, a budget of one, so the first sweep leaves work behind.
	second, err := s.store.Themes().Create(ctx, &domain.CustomTheme{
		SessionID: "sid2", FamilyID: "liuli", Name: "另一套",
		Variables: map[string]string{"--primary": "#fff"},
	})
	if err != nil {
		t.Fatal(err)
	}
	request(t, s, "liuli")
	fam, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	if _, _, _, err := s.backfillFamily(ctx, *fam, 1); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRequest(http.MethodDelete, versionPath("/api/admin/frontends/families/liuli/backfill"), strings.NewReader(""))
	w := httptest.NewRecorder()
	rec.SetPathValue("id", "liuli")
	s.handleAdminBackfillStop(w, rec)
	if w.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "alreadyFilledStayFilled") {
		t.Errorf("the response does not say a stop is not an undo: %s", w.Body)
	}

	filled := 0
	for _, id := range []struct{ sid, id string }{{"sid1", th.ID}, {"sid2", second.ID}} {
		got, _ := s.store.Themes().Get(ctx, id.sid, id.id)
		if _, ok := got.Variables["--glass-alpha"]; ok {
			filled++
		}
	}
	if filled != 1 {
		t.Errorf("%d of 2 themes are filled; the stop should have left exactly the one already done", filled)
	}
	fam, _ = s.store.Frontends().GetFamily(ctx, "liuli")
	if fam.BackfillRequestedAt != nil {
		t.Error("the stop did not disarm the request")
	}
}

// ⚠️ The operator's spend does not land on the user's account.
//
// The ledger row still carries the session — that is what the money bought —
// but the three-hour window on the session row is "what is this ACCOUNT doing",
// and a backfill is not something the account did. Without this split, pressing
// the button on a family with two thousand themes spikes two thousand innocent
// accounts at once, and whoever goes looking for the cause finds them instead
// of the deploy.
func TestABackfillDoesNotLandOnTheUsersUsageCounters(t *testing.T) {
	s, _ := backfillServer(t, []string{`"{\"variables\":{\"--glass-alpha\":\"0.72\"}}"`})
	seedBackfill(t, s, "ratio")
	request(t, s, "liuli")
	ctx := context.Background()

	if _, err := s.store.Sessions().GetOrCreate(ctx, "sid1"); err != nil {
		t.Fatal(err)
	}
	before, _ := s.store.Sessions().Get(ctx, "sid1")

	s.sweepThemeBackfills(ctx)

	after, _ := s.store.Sessions().Get(ctx, "sid1")
	if after.Usage.TotalCalls != before.Usage.TotalCalls {
		t.Errorf("the account's lifetime calls moved from %d to %d for a call it did not make",
			before.Usage.TotalCalls, after.Usage.TotalCalls)
	}
	if after.Usage.FastCalls != before.Usage.FastCalls {
		t.Errorf("the account's three-hour window moved for a call it did not make")
	}

	// …and the ledger DOES have it, under its own endpoint, so the cost is still
	// attributable to the theme it bought.
	logs, err := s.store.AILogs().List(ctx, domain.AILogFilter{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range logs {
		if l.Endpoint == epThemeBackfill {
			found = true
			if l.SessionID != "sid1" {
				t.Errorf("the ledger row is attributed to %q, losing the link to the theme", l.SessionID)
			}
		}
		if l.Endpoint == epThemeGen {
			t.Error("a backfill was logged as an ordinary theme generation; the two cannot be told apart")
		}
	}
	if !found {
		t.Error("the backfill left no ledger row at all")
	}
}

// ⚠️ A backfill that cannot make progress is called off, not left armed.
//
// Without this the family stays armed FOREVER: the sweep rescans it every
// minute, the console says 补算进行中 forever, and nothing anywhere says it is
// stuck. Two ordinary ways to get there — a model that refuses this prompt, or
// a token whose kind nothing it writes can satisfy.
func TestAStuckBackfillIsCalledOffRatherThanArmedForever(t *testing.T) {
	s, calls := backfillServer(t, []string{`"not json at all"`})
	seedBackfill(t, s, "ratio")
	request(t, s, "liuli")
	ctx := context.Background()

	for i := 0; i < ThemeBackfillDryPasses+2; i++ {
		s.sweepThemeBackfills(ctx)
	}
	fam, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	if fam.BackfillRequestedAt != nil {
		t.Error("a backfill that can never make progress is still armed; it will rescan every minute forever")
	}
	// And it stopped paying long before it gave up — the attempts cap, not the
	// dry-pass counter, is what bounds the money.
	if *calls > domain.JobMaxAttempts {
		t.Errorf("it paid %d times before giving up; the cap is %d", *calls, domain.JobMaxAttempts)
	}
}

// …but a blip is ridden out rather than turned into a cancelled job.
func TestATransientFailureDoesNotCallOffTheBackfill(t *testing.T) {
	s, _ := backfillServer(t, []string{`"not json"`})
	seedBackfill(t, s, "ratio")
	request(t, s, "liuli")
	ctx := context.Background()

	s.sweepThemeBackfills(ctx)
	fam, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	if fam.BackfillRequestedAt == nil {
		t.Error("one failed pass called off the whole backfill")
	}
}

// ⚠️ The budget bounds MONEY, including calls that fail.
//
// The bug this pins: `spent` counted only themes that came out COMPLETE, so a
// model that failed — or that filled some tokens and not others — left it at
// zero, the ceiling never applied, and one sweep called the model once for
// every theme in the family. That is exactly the case where a ceiling matters
// most: a misbehaving provider burning through two thousand themes back to
// back, saturating its own rate limit and starving every interactive request
// behind it.
//
// Found by an adversarial review pass, not by the tests written alongside the
// code — those all used a single theme, where "budget" and "completions" are
// indistinguishable.
func TestTheBudgetBoundsFailingCallsToo(t *testing.T) {
	for _, c := range []struct {
		name   string
		script []string
	}{
		{"every call fails", []string{`"not json"`}},
		{"every call is refused by validation", []string{`"{\"variables\":{\"--glass-alpha\":\"#ffffff\"}}"`}},
		{"every call succeeds", []string{`"{\"variables\":{\"--glass-alpha\":\"0.5\"}}"`}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, calls := backfillServer(t, c.script)
			fam, _ := seedBackfill(t, s, "ratio")
			ctx := context.Background()
			for i := 0; i < 9; i++ {
				if _, err := s.store.Themes().Create(ctx, &domain.CustomTheme{
					SessionID: fmt.Sprintf("sid%d", i+2), FamilyID: "liuli", Name: "t",
					Variables: map[string]string{"--primary": "#fff"},
				}); err != nil {
					t.Fatal(err)
				}
			}
			// Ten themes needing work, a budget of three.
			spent, _, remaining, err := s.backfillFamily(ctx, fam, 3)
			if err != nil {
				t.Fatal(err)
			}
			if *calls > 3 {
				t.Errorf("budget 3 but %d model calls — the ceiling does not apply to this outcome", *calls)
			}
			if spent != *calls {
				t.Errorf("reported spend %d but made %d calls", spent, *calls)
			}
			// The countdown is the whole family, not just the part it got to.
			if remaining < 7 {
				t.Errorf("remaining=%d; the scan must finish so the operator sees a real countdown", remaining)
			}
		})
	}
}

// The dry counter resets on progress, or a family that is slowly working
// through a backlog gets called off after ten passes for no reason.
func TestSlowProgressDoesNotCountAsStuck(t *testing.T) {
	// First call fills, second is garbage: one completion, one failure, work
	// left over — the shape of a pass that IS making progress.
	s, _ := backfillServer(t, []string{
		`"{\"variables\":{\"--glass-alpha\":\"0.5\"}}"`,
		`"not json"`,
	})
	seedBackfill(t, s, "ratio")
	ctx := context.Background()
	if _, err := s.store.Themes().Create(ctx, &domain.CustomTheme{
		SessionID: "sid2", FamilyID: "liuli", Name: "另一套",
		Variables: map[string]string{"--primary": "#fff"},
	}); err != nil {
		t.Fatal(err)
	}
	request(t, s, "liuli")

	s.backfillDry["liuli"] = ThemeBackfillDryPasses - 1
	s.sweepThemeBackfills(ctx)

	if n := s.backfillDry["liuli"]; n != 0 {
		t.Errorf("a pass that completed a theme left the stuck counter at %d; it must reset on progress", n)
	}
	fam, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	if fam.BackfillRequestedAt == nil {
		t.Error("a backfill that was making progress was called off")
	}
}

// ⚠️ A handshake must not reset the attempts cap.
//
// The occurrence used to be keyed on fam.UpdatedAt, and the handshake calls
// UpsertFamily on EVERY connection — so updated_at moved whenever any frontend
// loaded a page. Each move minted a fresh job_runs occurrence, which reset the
// attempts counter, which meant a theme that could never be filled was paid for
// again and again forever, with every job_run showing one clean attempt so
// nothing looked wrong.
//
// Found by an adversarial review pass.
func TestAHandshakeDoesNotResetTheAttemptsCap(t *testing.T) {
	s, calls := backfillServer(t, []string{`"not json"`})
	seedBackfill(t, s, "ratio")
	request(t, s, "liuli")
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		s.sweepThemeBackfills(ctx)
		// A frontend connects between sweeps — the ordinary case, once per page
		// load. It declares nothing new, so the token space is unchanged.
		handshake(t, s, `{"familyId":"liuli","buildHash":"web-1","theme":{"tokens":[
			{"name":"--primary","kind":"color"},
			{"name":"--glass-alpha","kind":"ratio"}]}}`)
	}
	if *calls > domain.JobMaxAttempts {
		t.Errorf("a page load reset the attempts cap: %d calls, cap is %d", *calls, domain.JobMaxAttempts)
	}

	// …but genuinely widening the space IS a new occurrence, or the new token
	// could never be filled.
	fam, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	before := fam.TokenSpaceHash()
	fam.Tokens = append(fam.Tokens, domain.TokenSpec{Name: "--edge", Kind: "one-of[hairline, none]"})
	if fam.TokenSpaceHash() == before {
		t.Error("widening the token space did not change its hash; the new token can never be backfilled")
	}
	// A rename does not.
	fam2, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	fam2.DisplayName = "琉璃"
	if fam2.TokenSpaceHash() != before {
		t.Error("renaming a family changed its token-space hash")
	}
	// A token whose KIND changed is a different value to generate, so it does.
	fam3, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	fam3.Tokens[1].Kind = "length"
	if fam3.TokenSpaceHash() == before {
		t.Error("changing a token's kind did not change the hash; the old value is never regenerated")
	}
	// Order must not matter — the union is built from an unordered merge.
	fam4, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	fam4.Tokens[0], fam4.Tokens[1] = fam4.Tokens[1], fam4.Tokens[0]
	if fam4.TokenSpaceHash() != before {
		t.Error("reordering the same tokens changed the hash; every merge would re-run every theme")
	}
}

// ⚠️ A user's edit during the model call is not reverted.
//
// The model call takes seconds and Update writes the whole variables map, so
// merging into the snapshot read before the call would silently revert whatever
// the user did in that window — the exact outcome the merge exists to prevent,
// arrived at from the other side. There is no undo for a theme.
//
// Found by an adversarial review pass.
func TestAUserEditDuringTheModelCallSurvives(t *testing.T) {
	s, _ := backfillServer(t, []string{`"{\"variables\":{\"--glass-alpha\":\"0.72\"}}"`})
	fam, th := seedBackfill(t, s, "ratio")
	ctx := context.Background()

	// The user recolours the theme, and adds the very token being backfilled,
	// while the sweep holds its snapshot.
	edited := map[string]string{"--primary": "#22c55e", "--extra": "#000"}
	if _, err := s.store.Themes().Update(ctx, "sid1", th.ID,
		domain.CustomThemeUpdate{Variables: &edited}); err != nil {
		t.Fatal(err)
	}

	// th is the STALE copy, which is exactly what the sweep is holding.
	s.fillOneTheme(ctx, fam, th, fam.MissingTokens(th.Variables))

	got, _ := s.store.Themes().Get(ctx, "sid1", th.ID)
	if got.Variables["--primary"] != "#22c55e" {
		t.Errorf("the backfill reverted the user's edit: --primary is %q, they set #22c55e", got.Variables["--primary"])
	}
	if _, ok := got.Variables["--extra"]; !ok {
		t.Error("the backfill dropped a variable the user added during the call")
	}
	if got.Variables["--glass-alpha"] != "0.72" {
		t.Errorf("the backfill did not fill the missing token: %v", got.Variables)
	}
}

// A user who filled the token themselves keeps their value — they chose it, the
// model guessed it.
func TestTheUsersOwnValueBeatsTheModels(t *testing.T) {
	s, _ := backfillServer(t, []string{`"{\"variables\":{\"--glass-alpha\":\"0.72\"}}"`})
	fam, th := seedBackfill(t, s, "ratio")
	ctx := context.Background()
	mine := map[string]string{"--primary": "#a78bfa", "--glass-alpha": "0.31"}
	if _, err := s.store.Themes().Update(ctx, "sid1", th.ID,
		domain.CustomThemeUpdate{Variables: &mine}); err != nil {
		t.Fatal(err)
	}

	_, complete := s.fillOneTheme(ctx, fam, th, fam.MissingTokens(th.Variables))

	got, _ := s.store.Themes().Get(ctx, "sid1", th.ID)
	if got.Variables["--glass-alpha"] != "0.31" {
		t.Errorf("the model overwrote the value the user chose: %q", got.Variables["--glass-alpha"])
	}
	if !complete {
		t.Error("a theme the user completed themselves was not reported complete; the family never finishes")
	}
}

// ⚠️ A request made while the sweep was running is not thrown away.
//
// There is no compare-and-set on families, so the sweep's re-read before
// clearing is the whole defence. Without the comparison, an operator who
// pressed the button again — because the token space widened again — has their
// request silently discarded, and they are watching a screen that says it is
// running.
func TestARequestMadeDuringTheSweepIsNotDiscarded(t *testing.T) {
	s, _ := backfillServer(t, []string{`"{\"variables\":{}}"`})
	seedBackfill(t, s, "ratio")
	ctx := context.Background()

	// What the sweep read when it started.
	old := time.Now().UTC().Add(-time.Hour)
	stale := domain.FrontendFamily{ID: "liuli", BackfillRequestedAt: &old}

	// What is in the database now: the operator pressed it again mid-pass.
	newer := time.Now().UTC()
	cur, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	cur.BackfillRequestedAt = &newer
	if err := s.store.Frontends().UpsertFamily(ctx, *cur); err != nil {
		t.Fatal(err)
	}

	s.clearBackfillIfUnchanged(ctx, stale)

	got, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	if got.BackfillRequestedAt == nil {
		t.Fatal("a request made while the sweep was running was discarded; the operator is watching a screen that says it is running")
	}
	if got.BackfillRequestedAt.UnixMilli() != newer.UnixMilli() {
		t.Errorf("the stored request is %v, want the newer one %v", got.BackfillRequestedAt, newer)
	}

	// …and the ordinary case still clears, or a finished backfill sweeps forever.
	//
	// ⚠️ Passing `*cur` — a family carrying a time this test MADE, not one it
	// read — is deliberate. The stores keep epoch millis, so time.Equal is false
	// against the truncated copy and a comparison written that way silently
	// never clears. This line is what says the comparison is millisecond-based.
	s.clearBackfillIfUnchanged(ctx, *cur)
	if got, _ := s.store.Frontends().GetFamily(ctx, "liuli"); got.BackfillRequestedAt != nil {
		t.Error("a finished backfill was left armed; it will rescan every minute forever")
	}
}
